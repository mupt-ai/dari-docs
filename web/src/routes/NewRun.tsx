import { useEffect, useMemo, useRef, useState, type ChangeEvent, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ChevronDown, FolderOpen, Info, Loader2, Pencil, Plus, Trash2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { getRunConfig, type RunConfig } from "@/lib/billing";
import {
  createRunFromFolder,
  type BrowserSourceFile,
  type RuntimeSecretInput,
} from "@/lib/runs";
import { cn, formatCents } from "@/lib/utils";

type RunMode = "check" | "optimize";
type DocsSourceMode = "folder" | "link";

type SelectedSourceFile = BrowserSourceFile & {
  size: number;
};

type SkippedUploadFile = {
  path: string;
  reason: string;
  size: number;
};

const skipUploadDirs = new Set([
  ".git",
  "node_modules",
  ".dari-docs",
  ".next",
  "dist",
  "build",
  "coverage",
  ".turbo",
]);

const defaultTask =
  "Install the SDK and make a first API call based only on these docs.";

export default function NewRun() {
  const navigate = useNavigate();
  const [config, setConfig] = useState<RunConfig | null>(null);
  const [configError, setConfigError] = useState<string | null>(null);
  const [mode, setMode] = useState<RunMode>("check");
  const [taskItems, setTaskItems] = useState<string[]>([defaultTask]);
  const [taskDraft, setTaskDraft] = useState("");
  const [editingTaskIndex, setEditingTaskIndex] = useState<number | null>(null);
  const [editingTaskDraft, setEditingTaskDraft] = useState("");
  const [browserFiles, setBrowserFiles] = useState<File[]>([]);
  const [publicDocsURL, setPublicDocsURL] = useState("");
  const [docsSourceMode, setDocsSourceMode] = useState<DocsSourceMode>("folder");
  const [testerLLMIDs, setTesterLLMIDs] = useState<string[]>([]);
  const [editorLLMID, setEditorLLMID] = useState("");
  const [includeText, setIncludeText] = useState("");
  const [excludeText, setExcludeText] = useState("");
  const [runtimeSecrets, setRuntimeSecrets] = useState<RuntimeSecretInput[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const taskDraftRef = useRef<HTMLTextAreaElement | null>(null);
  const editingTaskRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    let canceled = false;
    async function load() {
      try {
        const nextConfig = await getRunConfig();
        if (canceled) return;
        setConfig(nextConfig);
        setTesterLLMIDs(defaultTesterLLMIDs(nextConfig));
        setEditorLLMID(nextConfig.default_llm_id ?? "");
      } catch (error) {
        if (!canceled) {
          setConfigError(error instanceof Error ? error.message : String(error));
        }
      }
    }
    void load();
    return () => {
      canceled = true;
    };
  }, []);

  const tasks = useMemo(() => parseTaskInputs(taskItems), [taskItems]);
  const includeGlobs = useMemo(() => parsePatternLines(includeText), [includeText]);
  const excludeGlobs = useMemo(() => parsePatternLines(excludeText), [excludeText]);
  const publicDocURLs = useMemo(() => parsePatternLines(publicDocsURL), [publicDocsURL]);
  const taskLimit = config?.max_tasks_per_run ?? 3;
  const taskByteLimit = config?.max_task_bytes ?? 10000;
  const liveVerify = runtimeSecrets.length > 0;
  const selectedFolder = useMemo(() => selectedFolderLabel(browserFiles), [browserFiles]);
  const { selected: selectedFiles, skipped: skippedFiles } = useMemo(
    () =>
      config
        ? selectBrowserFiles(browserFiles, config, includeGlobs, excludeGlobs)
        : { selected: [], skipped: [] },
    [browserFiles, config, excludeGlobs, includeGlobs]
  );
  const selectedBytes = useMemo(
    () => selectedFiles.reduce((sum, item) => sum + item.size, 0),
    [selectedFiles]
  );
  const activeSelectedFiles = docsSourceMode === "folder" ? selectedFiles : [];
  const activeSelectedBytes = docsSourceMode === "folder" ? selectedBytes : 0;
  const activePublicDocURLs = docsSourceMode === "link" ? publicDocURLs : [];
  const testerSessionCount = tasks.length * testerLLMIDs.length;
  const estimatedReserve = config
    ? testerSessionCount * config.tester_session_reserve_cents +
      (mode === "optimize" ? config.editor_session_reserve_cents : 0)
    : 0;
  const startRunDisabledReason = submitting
    ? ""
    : activeSelectedFiles.length === 0 && activePublicDocURLs.length === 0
      ? "Choose a docs folder or enter a public docs URL."
      : tasks.length === 0
        ? "Add at least one task."
        : testerLLMIDs.length === 0
          ? "Select at least one tester model."
          : "";
  const canStartRun = !submitting && startRunDisabledReason === "";

  const onFolderChange = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.currentTarget.files ?? []);
    setBrowserFiles(files);
  };

  const clearFolder = () => {
    setBrowserFiles([]);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  const selectFolderSource = () => {
    setDocsSourceMode("folder");
    setPublicDocsURL("");
  };

  const selectLinkSource = () => {
    setDocsSourceMode("link");
    clearFolder();
  };

  const updateTaskDraft = (value: string) => {
    setTaskDraft(truncateToUTF8Bytes(value, taskByteLimit));
  };

  const confirmTaskDraft = () => {
    const nextTask = taskDraft.trim();
    if (!nextTask || taskItems.length >= taskLimit) {
      setTaskDraft("");
      resetTextareaHeight(taskDraftRef.current);
      return;
    }
    setTaskItems((current) => [...current, nextTask]);
    setTaskDraft("");
    resetTextareaHeight(taskDraftRef.current);
  };

  const removeTask = (index: number) => {
    setTaskItems((current) => current.filter((_, itemIndex) => itemIndex !== index));
    if (editingTaskIndex === index) {
      setEditingTaskIndex(null);
      setEditingTaskDraft("");
    }
  };

  const startEditingTask = (index: number) => {
    setEditingTaskIndex(index);
    setEditingTaskDraft(taskItems[index] ?? "");
    requestAnimationFrame(() => {
      if (editingTaskRef.current) {
        resizeTextarea(editingTaskRef.current);
        editingTaskRef.current.focus();
      }
    });
  };

  const updateEditingTaskDraft = (value: string) => {
    setEditingTaskDraft(truncateToUTF8Bytes(value, taskByteLimit));
  };

  const saveEditingTask = () => {
    if (editingTaskIndex === null) return;
    const nextTask = editingTaskDraft.trim();
    const index = editingTaskIndex;
    setTaskItems((current) => {
      if (!nextTask) {
        return current.filter((_, itemIndex) => itemIndex !== index);
      }
      return current.map((task, itemIndex) => (itemIndex === index ? nextTask : task));
    });
    setEditingTaskIndex(null);
    setEditingTaskDraft("");
  };

  const cancelEditingTask = () => {
    setEditingTaskIndex(null);
    setEditingTaskDraft("");
  };

  const addSecret = () => {
    setRuntimeSecrets((current) => [...current, { name: "", value: "" }]);
  };

  const updateSecret = (index: number, key: keyof RuntimeSecretInput, value: string) => {
    setRuntimeSecrets((current) =>
      current.map((secret, itemIndex) =>
        itemIndex === index ? { ...secret, [key]: value } : secret
      )
    );
  };

  const removeSecret = (index: number) => {
    setRuntimeSecrets((current) => current.filter((_, itemIndex) => itemIndex !== index));
  };

  const toggleTesterLLM = (llmID: string) => {
    setTesterLLMIDs((current) =>
      current.includes(llmID)
        ? current.filter((value) => value !== llmID)
        : [...current, llmID]
    );
  };

  const submit = async () => {
    if (!config) return;
    setSubmitError(null);
    const validation = validateRunForm({
      config,
      tasks,
      selectedFiles: activeSelectedFiles,
      selectedBytes: activeSelectedBytes,
      testerLLMIDs,
      mode,
      editorLLMID,
      liveVerify,
      runtimeSecrets,
      publicDocURLs: activePublicDocURLs,
    });
    if (validation) {
      setSubmitError(validation);
      return;
    }
    setSubmitting(true);
    try {
      const response = await createRunFromFolder({
        mode,
        tasks,
        files: activeSelectedFiles,
        publicDocURLs: activePublicDocURLs,
        testerLLMIDs,
        editorLLMID: mode === "optimize" ? editorLLMID : undefined,
        includeGlobs: docsSourceMode === "folder" ? includeGlobs : [],
        excludeGlobs: docsSourceMode === "folder" ? excludeGlobs : [],
        liveVerify,
        runtimeSecrets: liveVerify ? runtimeSecrets : undefined,
      });
      navigate(`/runs/${response.run_id}`);
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : String(error));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="px-6 py-6">
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <Link to="/runs" className="text-xs uppercase tracking-widest text-muted-foreground hover:text-foreground">
            Runs
          </Link>
          <h1 className="mt-2 text-xl font-medium">New Run</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            Upload a local docs folder or collect public docs from a URL, then start a check or optimize run.
          </p>
        </div>
      </div>

      {configError && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {configError}
        </div>
      )}
      {submitError && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {submitError}
        </div>
      )}

      {!config ? (
        <div className="text-sm text-muted-foreground">Loading Run Configuration...</div>
      ) : (
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
          <div className="flex flex-col gap-6">
            <section className="border border-border bg-card p-4">
              <div className="mb-3 flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-sm font-medium">Docs Source: Folder Or Link</h2>
                </div>
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                <Button
                  type="button"
                  variant={docsSourceMode === "folder" ? "default" : "outline"}
                  onClick={selectFolderSource}
                >
                  Docs Folder
                </Button>
                <Button
                  type="button"
                  variant={docsSourceMode === "link" ? "default" : "outline"}
                  onClick={selectLinkSource}
                >
                  Public Docs URL
                </Button>
              </div>

              {docsSourceMode === "folder" ? (
                <div className="mt-4">
                  {browserFiles.length === 0 ? (
                    <label className="flex min-h-36 cursor-pointer flex-col items-center justify-center border border-dashed border-border bg-background px-4 py-6 text-center hover:border-muted-foreground/60">
                      <FolderOpen className="mb-3 h-6 w-6 text-muted-foreground" />
                      <span className="text-sm font-medium">Choose Docs Folder</span>
                      <span className="mt-1 text-xs text-muted-foreground">
                        Files are staged only for this run.
                      </span>
                      <input
                        {...directoryInputProps}
                        ref={fileInputRef}
                        type="file"
                        multiple
                        className="sr-only"
                        onChange={onFolderChange}
                      />
                    </label>
                  ) : (
                    <div className="border border-border bg-background p-4">
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <div className="flex min-w-0 items-center gap-3">
                          <div className="flex h-10 w-10 shrink-0 items-center justify-center border border-border bg-card">
                            <FolderOpen className="h-5 w-5 text-muted-foreground" />
                          </div>
                          <div className="min-w-0">
                            <div className="truncate text-sm font-medium">{selectedFolder}</div>
                            <div className="mt-1 text-xs text-muted-foreground">
                              {browserFiles.length} raw files selected
                            </div>
                          </div>
                        </div>
                        <div className="flex shrink-0 items-center">
                          <Button type="button" variant="outline" size="icon" onClick={clearFolder}>
                            <X className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                    </div>
                  )}
                  <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
                    <span className="border border-border bg-background px-2 py-1">
                      {selectedFiles.length} Selected
                    </span>
                    <span className="border border-border bg-background px-2 py-1">
                      {formatBytes(selectedBytes)} / {formatBytes(config.bundle_max_uncompressed_bytes)}
                    </span>
                    <span className="border border-border bg-background px-2 py-1">
                      {skippedFiles.length} Skipped
                    </span>
                  </div>
                  {skippedFiles.length > 0 && (
                    <details className="mt-3 border border-border bg-background p-3 text-xs">
                      <summary className="cursor-pointer text-muted-foreground">
                        Show skipped files
                      </summary>
                      <div className="mt-3 max-h-40 overflow-auto">
                        {skippedFiles.slice(0, 50).map((file) => (
                          <div key={`${file.path}:${file.reason}`} className="flex justify-between gap-3 py-1">
                            <span className="min-w-0 truncate">{file.path}</span>
                            <span className="shrink-0 text-muted-foreground">{file.reason}</span>
                          </div>
                        ))}
                      </div>
                    </details>
                  )}
                  <details className="group mt-4 border border-border bg-background p-3">
                    <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-sm font-medium">
                      Options
                      <ChevronDown className="h-4 w-4 text-muted-foreground transition-transform group-open:rotate-180" />
                    </summary>
                    <div className="mt-4 grid gap-4 lg:grid-cols-2">
                      <div>
                        <label className="mb-2 flex items-center gap-1.5 text-xs uppercase tracking-widest text-muted-foreground">
                          <span>Include Patterns</span>
                          <InfoTooltip>
                            By default, docs/source files are included: Markdown, JSON,
                            YAML, TOML, CSS, JavaScript, TypeScript, README.md,
                            docs.json, mint.json, OpenAPI files, and llms.txt.
                            Include patterns add files outside those defaults, but
                            do not override built-in skipped folders such as
                            node_modules, .git, dist, and build.
                          </InfoTooltip>
                        </label>
                        <textarea
                          value={includeText}
                          onChange={(event) => setIncludeText(event.target.value)}
                          placeholder={"examples/**/*.py\nscripts/**"}
                          className="min-h-28 w-full border border-border bg-card px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground hover:border-muted-foreground/60 focus:border-brand"
                        />
                      </div>
                      <div>
                        <label className="mb-2 flex items-center gap-1.5 text-xs uppercase tracking-widest text-muted-foreground">
                          <span>Exclude Patterns</span>
                          <InfoTooltip>
                            By default, generated or dependency-heavy folders are
                            skipped: .git, node_modules, .dari-docs, .next, dist,
                            build, coverage, and .turbo. Exclude patterns win over
                            defaults and include patterns.
                          </InfoTooltip>
                        </label>
                        <textarea
                          value={excludeText}
                          onChange={(event) => setExcludeText(event.target.value)}
                          placeholder={"generated/**\n*.lock"}
                          className="min-h-28 w-full border border-border bg-card px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground hover:border-muted-foreground/60 focus:border-brand"
                        />
                      </div>
                    </div>
                  </details>
                </div>
              ) : (
                <div className="mt-4">
                  <label className="mb-2 block text-sm font-medium">
                    Public Docs URL
                  </label>
                  <Input
                    value={publicDocsURL}
                    onChange={(event) => setPublicDocsURL(event.target.value)}
                    placeholder="https://docs.dari.dev/llms.txt"
                  />
                </div>
              )}
            </section>

            <section className="border border-border bg-card p-4">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-sm font-medium">Tasks</h2>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Add up to {config.max_tasks_per_run} tasks.
                  </p>
                </div>
              </div>
              <div className="mt-4 flex flex-col gap-3">
                {taskItems.map((task, index) => (
                  <div key={`${index}:${task}`} className="group/task flex items-center gap-3 border border-border bg-background p-3">
                    <div className="min-w-0 flex-1">
                      <div className="mb-1 flex items-center justify-between gap-3">
                        <span className="text-xs uppercase tracking-widest text-muted-foreground">
                          Task {index + 1}
                        </span>
                      </div>
                      {editingTaskIndex === index ? (
                        <div className="grid gap-2">
                          <textarea
                            ref={editingTaskRef}
                            value={editingTaskDraft}
                            maxLength={config.max_task_bytes}
                            rows={1}
                            onChange={(event) => {
                              updateEditingTaskDraft(event.target.value);
                              resizeTextarea(event.currentTarget);
                            }}
                            onInput={(event) => resizeTextarea(event.currentTarget)}
                            onKeyDown={(event) => {
                              if (event.key === "Enter" && !event.shiftKey) {
                                event.preventDefault();
                                saveEditingTask();
                              }
                              if (event.key === "Escape") {
                                event.preventDefault();
                                cancelEditingTask();
                              }
                            }}
                            className="max-h-40 min-h-10 resize-none overflow-hidden border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground hover:border-muted-foreground/60 focus:border-brand"
                          />
                          <div className="flex justify-end gap-2">
                            <Button type="button" variant="ghost" size="sm" onClick={cancelEditingTask}>
                              Cancel
                            </Button>
                            <Button type="button" variant="outline" size="sm" onClick={saveEditingTask}>
                              Save
                            </Button>
                          </div>
                        </div>
                      ) : (
                        <div className="whitespace-pre-wrap break-words text-sm text-foreground">{task}</div>
                      )}
                    </div>
                    {editingTaskIndex !== index && (
                      <div className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover/task:opacity-100 group-focus-within/task:opacity-100">
                        <Button type="button" variant="ghost" size="icon" onClick={() => startEditingTask(index)}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button type="button" variant="ghost" size="icon" onClick={() => removeTask(index)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    )}
                  </div>
                ))}
                {taskItems.length < config.max_tasks_per_run && (
                  <div className="border border-border bg-background p-3">
                    <div className="mb-2 flex items-center justify-between gap-3">
                      <label htmlFor="task-draft" className="text-xs uppercase tracking-widest text-muted-foreground">
                        New Task
                      </label>
                    </div>
                    <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
                      <textarea
                        id="task-draft"
                        ref={taskDraftRef}
                        value={taskDraft}
                        maxLength={config.max_task_bytes}
                        rows={1}
                        onChange={(event) => {
                          updateTaskDraft(event.target.value);
                          resizeTextarea(event.currentTarget);
                        }}
                        onInput={(event) => resizeTextarea(event.currentTarget)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter" && !event.shiftKey) {
                            event.preventDefault();
                            confirmTaskDraft();
                          }
                        }}
                        placeholder="Type a task and press Enter"
                        className="max-h-40 min-h-10 resize-none overflow-hidden border border-border bg-card px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground hover:border-muted-foreground/60 focus:border-brand"
                      />
                      <Button
                        type="button"
                        variant="outline"
                        onClick={confirmTaskDraft}
                        disabled={!taskDraft.trim()}
                        className="self-center"
                      >
                        Add
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            </section>

            <section className="border border-border bg-card p-4">
              <h2 className="text-sm font-medium">Models</h2>
              <div className="mt-4">
                <div className="mb-2 flex flex-wrap items-baseline gap-x-2 gap-y-1 text-xs uppercase tracking-widest text-muted-foreground">
                  <span>Tester Models</span>
                  <span className="normal-case tracking-normal">- choices are used for every task</span>
                </div>
                <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                  {config.allowed_llm_ids.map((llmID) => {
                    const selected = testerLLMIDs.includes(llmID);
                    return (
                      <button
                        key={llmID}
                        type="button"
                        aria-pressed={selected}
                        onClick={() => toggleTesterLLM(llmID)}
                        className={cn(
                          "relative min-h-12 border py-2 pl-3 pr-9 text-left text-sm transition-colors",
                          selected
                            ? "border-brand bg-brand/10 text-foreground"
                            : "border-border bg-background text-muted-foreground hover:border-muted-foreground/60 hover:text-foreground"
                        )}
                      >
                        <span className="block font-medium">{llmID}</span>
                        <span
                          className={cn(
                            "absolute right-3 top-1/2 h-2.5 w-2.5 -translate-y-1/2 rounded-full border",
                            selected ? "border-brand bg-brand" : "border-muted-foreground/60"
                          )}
                        />
                      </button>
                    );
                  })}
                </div>
                {mode === "optimize" && (
                  <div className="mt-4">
                  <label htmlFor="editor-llm" className="mb-2 block text-xs uppercase tracking-widest text-muted-foreground">
                    Editor Model
                  </label>
                  <select
                    id="editor-llm"
                    value={editorLLMID}
                    onChange={(event) => setEditorLLMID(event.target.value)}
                    className="w-full border border-border bg-background px-3 py-2 text-sm text-foreground outline-none transition-colors hover:border-muted-foreground/60 focus:border-brand"
                  >
                    {config.allowed_llm_ids.map((llmID) => (
                      <option key={llmID} value={llmID}>
                        {llmID}
                      </option>
                    ))}
                  </select>
                  </div>
                )}
              </div>
            </section>

            <section className="border border-border bg-card p-4">
              <h2 className="text-sm font-medium">Live Verification Secrets</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Pass runtime product/API secrets only for runs that need to exercise private or authenticated workflows.
              </p>
              <div className="mt-4 flex flex-col gap-2">
                {runtimeSecrets.length > 0 && (
                  <>
                    {runtimeSecrets.map((secret, index) => (
                      <div key={index} className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
                        <Input
                          value={secret.name}
                          onChange={(event) => updateSecret(index, "name", event.target.value)}
                          placeholder="TEST_SECRET_KEY"
                        />
                        <Input
                          value={secret.value}
                          onChange={(event) => updateSecret(index, "value", event.target.value)}
                          placeholder="Value"
                          type="password"
                        />
                        <Button type="button" variant="outline" size="icon" onClick={() => removeSecret(index)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    ))}
                  </>
                )}
                <Button type="button" variant="outline" size="sm" onClick={addSecret} className="w-fit">
                  <Plus className="mr-1.5 h-3.5 w-3.5" />
                  Add Secret
                </Button>
              </div>
            </section>
          </div>

          <aside className="h-fit xl:sticky xl:top-6">
              <div className="inline-flex w-full border border-border bg-card">
                <ModeButton active={mode === "check"} onClick={() => setMode("check")}>
                  Check
                </ModeButton>
                <ModeButton active={mode === "optimize"} onClick={() => setMode("optimize")}>
                  Optimize
                </ModeButton>
              </div>
            <section className="mt-4 border border-border bg-card p-4">
              <h2 className="text-sm font-medium">Summary</h2>
              <div className="mt-4 grid gap-3 text-sm">
                <SummaryRow label="Mode" value={mode === "optimize" ? "Optimize" : "Check"} />
                <SummaryRow label="Tasks" value={String(tasks.length)} />
                <SummaryRow label="Tester Models" value={String(testerLLMIDs.length)} />
                <SummaryRow label="Tester Sessions" value={String(testerSessionCount)} />
                {mode === "optimize" && <SummaryRow label="Editor Sessions" value="1" />}
                <SummaryRow label="Upload" value={formatBytes(selectedBytes)} />
                <SummaryRow
                  label={
                    <span className="inline-flex items-center gap-1.5">
                      Reserved
                      <span className="group relative inline-flex">
                        <Info className="h-3.5 w-3.5 text-muted-foreground" />
                        <span className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 hidden w-56 -translate-x-1/2 border border-border bg-[#111111] px-3 py-2 text-xs normal-case tracking-normal text-foreground shadow-lg group-hover:block">
                          Final charge reconciles to actual Dari session cost after completion.
                        </span>
                      </span>
                    </span>
                  }
                  value={formatCents(estimatedReserve)}
                />
              </div>
            </section>
              <div className="group/start relative mt-5">
                <Button
                  type="button"
                  className={cn(
                    "w-full",
                    !canStartRun &&
                      "cursor-not-allowed border-border bg-[#171717] text-muted-foreground hover:bg-[#171717]"
                  )}
                  style={
                    !canStartRun
                      ? {
                          backgroundImage:
                            "repeating-linear-gradient(135deg, rgba(255,255,255,0.10) 0, rgba(255,255,255,0.10) 1px, transparent 1px, transparent 7px)",
                        }
                      : undefined
                  }
                  onClick={() => {
                    if (!canStartRun) return;
                    void submit();
                  }}
                  disabled={submitting}
                  aria-disabled={!canStartRun}
                >
                  {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  {submitting ? "Starting..." : "Start Run"}
                </Button>
                {!canStartRun && startRunDisabledReason && (
                  <span className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 hidden w-64 -translate-x-1/2 border border-border bg-[#111111] px-3 py-2 text-xs text-foreground shadow-lg group-hover/start:block">
                    {startRunDisabledReason}
                  </span>
                )}
              </div>
          </aside>
        </div>
      )}
    </div>
  );
}

const directoryInputProps = {
  webkitdirectory: "",
  directory: "",
} as Record<string, string>;

function ModeButton({
  active,
  children,
  onClick,
}: {
  active: boolean;
  children: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "h-9 flex-1 px-4 text-sm transition-colors",
        active ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-accent hover:text-foreground"
      )}
    >
      {children}
    </button>
  );
}

function SummaryRow({ label, value }: { label: ReactNode; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border pb-2 last:border-b-0 last:pb-0">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{value}</span>
    </div>
  );
}

function InfoTooltip({ children }: { children: ReactNode }) {
  return (
    <span className="group/tooltip relative inline-flex">
      <Info className="h-3.5 w-3.5 text-muted-foreground" />
      <span className="pointer-events-none absolute bottom-full left-1/2 z-20 mb-2 hidden w-72 -translate-x-1/2 border border-border bg-[#111111] px-3 py-2 text-xs normal-case leading-5 tracking-normal text-foreground shadow-lg group-hover/tooltip:block">
        {children}
      </span>
    </span>
  );
}

function validateRunForm({
  config,
  tasks,
  selectedFiles,
  selectedBytes,
  testerLLMIDs,
  mode,
  editorLLMID,
  liveVerify,
  runtimeSecrets,
  publicDocURLs,
}: {
  config: RunConfig;
  tasks: string[];
  selectedFiles: SelectedSourceFile[];
  selectedBytes: number;
  testerLLMIDs: string[];
  mode: RunMode;
  editorLLMID: string;
  liveVerify: boolean;
  runtimeSecrets: RuntimeSecretInput[];
  publicDocURLs: string[];
}): string | null {
  if (selectedFiles.length === 0 && publicDocURLs.length === 0) {
    return "Choose a docs folder or enter a public docs URL.";
  }
  if (mode === "optimize" && publicDocURLs.length > 0) {
    return "Public docs URLs support Check only. Use local docs files for Optimize.";
  }
  const invalidURL = publicDocURLs.find((value) => !isPublicDocsURL(value));
  if (invalidURL) {
    return `Public docs URL must be an HTTP or HTTPS URL: ${invalidURL}`;
  }
  if (selectedBytes > config.bundle_max_uncompressed_bytes) {
    return `Selected files exceed the ${formatBytes(config.bundle_max_uncompressed_bytes)} upload limit.`;
  }
  if (tasks.length === 0) {
    return "Add at least one task.";
  }
  if (tasks.length > config.max_tasks_per_run) {
    return `Managed runs support at most ${config.max_tasks_per_run} tasks.`;
  }
  const encoder = new TextEncoder();
  const oversizedTask = tasks.findIndex((task) => encoder.encode(task).length > config.max_task_bytes);
  if (oversizedTask >= 0) {
    return `Task ${oversizedTask + 1} exceeds the ${config.max_task_bytes} byte task limit.`;
  }
  if (testerLLMIDs.length === 0) {
    return "Select at least one tester model.";
  }
  const allowed = new Set(config.allowed_llm_ids);
  if (testerLLMIDs.some((llmID) => !allowed.has(llmID))) {
    return "Tester models include an unsupported model.";
  }
  if (mode === "optimize" && !allowed.has(editorLLMID)) {
    return "Select a supported editor model.";
  }
  if (liveVerify) {
    const completeSecrets = runtimeSecrets.filter((secret) => secret.name.trim() || secret.value);
    if (completeSecrets.length === 0) {
      return "Add at least one runtime secret or turn off live verification.";
    }
    if (completeSecrets.some((secret) => !secret.name.trim() || !secret.value)) {
      return "Runtime secrets require both a name and value.";
    }
  }
  return null;
}

function parseTaskInputs(values: string[]): string[] {
  return values.map((value) => value.trim()).filter(Boolean);
}

function parsePatternLines(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith("#"));
}

function isPublicDocsURL(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

function selectedFolderLabel(files: File[]): string {
  if (files.length === 0) return "No folder selected";
  const firstPath = browserFilePath(files[0]);
  const firstSegment = firstPath.split("/").filter(Boolean)[0];
  if (!firstSegment) return "Selected folder";
  const allShareRoot = files.every((file) => browserFilePath(file).split("/").filter(Boolean)[0] === firstSegment);
  return allShareRoot ? firstSegment : "Selected files";
}

function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

function truncateToUTF8Bytes(value: string, maxBytes: number): string {
  if (maxBytes <= 0 || utf8ByteLength(value) <= maxBytes) return value;
  let out = "";
  let bytes = 0;
  for (const char of value) {
    const nextBytes = utf8ByteLength(char);
    if (bytes + nextBytes > maxBytes) break;
    out += char;
    bytes += nextBytes;
  }
  return out;
}

function resizeTextarea(el: HTMLTextAreaElement) {
  el.style.height = "auto";
  el.style.height = `${el.scrollHeight}px`;
}

function resetTextareaHeight(el: HTMLTextAreaElement | null) {
  if (!el) return;
  requestAnimationFrame(() => {
    el.style.height = "auto";
  });
}

function selectBrowserFiles(
  files: File[],
  config: RunConfig,
  includeGlobs: string[],
  excludeGlobs: string[]
): { selected: SelectedSourceFile[]; skipped: SkippedUploadFile[] } {
  const withPaths = stripCommonRoot(
    files.map((file) => ({
      file,
      path: browserFilePath(file),
    }))
  );
  const selected: SelectedSourceFile[] = [];
  const skipped: SkippedUploadFile[] = [];
  const seen = new Set<string>();

  for (const item of withPaths) {
    const path = normalizeBrowserPath(item.path);
    if (!path || seen.has(path)) {
      skipped.push({ path: item.path || item.file.name, reason: "duplicate", size: item.file.size });
      continue;
    }
    seen.add(path);
    const skipReason = uploadSkipReason(path, item.file, config, includeGlobs, excludeGlobs);
    if (skipReason) {
      skipped.push({ path, reason: skipReason, size: item.file.size });
      continue;
    }
    selected.push({ path, file: item.file, size: item.file.size });
  }
  selected.sort((a, b) => a.path.localeCompare(b.path));
  skipped.sort((a, b) => a.path.localeCompare(b.path));
  return { selected, skipped };
}

function browserFilePath(file: File): string {
  const withRelativePath = file as File & { webkitRelativePath?: string };
  return withRelativePath.webkitRelativePath || file.name;
}

function stripCommonRoot<T extends { path: string }>(items: T[]): T[] {
  const firstSegments = items
    .map((item) => item.path.split("/").filter(Boolean))
    .filter((segments) => segments.length > 1)
    .map((segments) => segments[0]);
  if (firstSegments.length !== items.length) return items;
  const root = firstSegments[0];
  if (!root || firstSegments.some((segment) => segment !== root)) return items;
  return items.map((item) => ({
    ...item,
    path: item.path.split("/").slice(1).join("/"),
  }));
}

function normalizeBrowserPath(path: string): string {
  return path.replace(/\\/g, "/").replace(/^\.\/+/, "").replace(/^\/+/, "").trim();
}

function uploadSkipReason(
  path: string,
  file: File,
  config: RunConfig,
  includeGlobs: string[],
  excludeGlobs: string[]
): string | null {
  const segments = path.split("/").filter(Boolean);
  if (segments.length === 0) return "invalid path";
  if (segments.some((segment) => segment === ".." || segment === ".")) return "invalid path";
  if (segments.some((segment) => skipUploadDirs.has(segment))) return "ignored directory";
  if (file.size > config.bundle_max_file_bytes) {
    return `over ${formatBytes(config.bundle_max_file_bytes)}`;
  }
  if (matchesAnyPattern(excludeGlobs, path)) return "excluded";
  if (!looksLikeDocsPath(path) && !matchesAnyPattern(includeGlobs, path)) {
    return "unsupported";
  }
  return null;
}

const defaultDocsExts = new Set([
  ".md",
  ".mdx",
  ".txt",
  ".json",
  ".yml",
  ".yaml",
  ".toml",
  ".css",
  ".js",
  ".jsx",
  ".ts",
  ".tsx",
]);

const defaultDocsNames = new Set([
  "mint.json",
  "docs.json",
  "openapi.json",
  "openapi.yaml",
  "README",
  "README.md",
  "llms.txt",
  "llms-full.txt",
]);

const defaultClaudeTesterLLMIDs = ["claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7"];

function defaultTesterLLMIDs(config: RunConfig): string[] {
  const allowed = new Set(config.allowed_llm_ids);
  const configuredDefaults = config.default_feedback_llm_ids.filter((llmID) => allowed.has(llmID));
  if (configuredDefaults.length > 0) return configuredDefaults;
  return defaultClaudeTesterLLMIDs.filter((llmID) => allowed.has(llmID));
}

function looksLikeDocsPath(filePath: string): boolean {
  const name = filePath.split("/").pop() ?? filePath;
  if (defaultDocsNames.has(name)) return true;
  const dot = name.lastIndexOf(".");
  return dot >= 0 && defaultDocsExts.has(name.slice(dot).toLowerCase());
}

function matchesAnyPattern(patterns: string[], rel: string): boolean {
  const normalized = normalizeBrowserPath(rel);
  return patterns.some((pattern) => matchesPattern(pattern, normalized));
}

function matchesPattern(pattern: string, rel: string): boolean {
  const normalized = normalizeBundlePattern(pattern);
  if (!normalized) return false;
  if (normalized.endsWith("/**")) {
    const prefix = normalized.slice(0, -3);
    if (rel === prefix || rel.startsWith(`${prefix}/`)) return true;
  }
  const rx = new RegExp(`^${globRegExp(normalized)}$`);
  if (normalized.includes("/")) {
    return rx.test(rel);
  }
  return rel.split("/").some((segment) => rx.test(segment));
}

function normalizeBundlePattern(pattern: string): string {
  return normalizeBrowserPath(pattern).replace(/\/+$/, "");
}

function globRegExp(pattern: string): string {
  let out = "";
  for (let i = 0; i < pattern.length; i++) {
    const char = pattern[i];
    if (char === "*") {
      if (pattern[i + 1] === "*") {
        if (pattern[i + 2] === "/") {
          out += "(?:.*/)?";
          i += 2;
        } else {
          out += ".*";
          i++;
        }
      } else {
        out += "[^/]*";
      }
    } else if (char === "?") {
      out += "[^/]";
    } else {
      out += char.replace(/[|\\{}()[\]^$+*?.]/g, "\\$&");
    }
  }
  return out;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KiB", "MiB", "GiB"];
  let value = bytes / 1024;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unitIndex]}`;
}

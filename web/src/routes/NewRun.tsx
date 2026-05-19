import { useEffect, useMemo, useState, type ChangeEvent, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ChevronDown, FolderOpen, Loader2, Plus, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { getRunConfig, type RunConfig } from "@/lib/billing";
import {
  createRunFromFolder,
  type BrowserSourceFile,
  type RuntimeSecretInput,
} from "@/lib/runs";
import { cn, formatCents } from "@/lib/utils";

type RunMode = "check" | "optimize";

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
  const [taskText, setTaskText] = useState(defaultTask);
  const [browserFiles, setBrowserFiles] = useState<File[]>([]);
  const [testerLLMIDs, setTesterLLMIDs] = useState<string[]>([]);
  const [editorLLMID, setEditorLLMID] = useState("");
  const [includeText, setIncludeText] = useState("");
  const [excludeText, setExcludeText] = useState("");
  const [liveVerify, setLiveVerify] = useState(false);
  const [runtimeSecrets, setRuntimeSecrets] = useState<RuntimeSecretInput[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);

  useEffect(() => {
    let canceled = false;
    async function load() {
      try {
        const nextConfig = await getRunConfig();
        if (canceled) return;
        setConfig(nextConfig);
        setTesterLLMIDs(nextConfig.default_feedback_llm_ids ?? []);
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

  const tasks = useMemo(() => parseTasks(taskText), [taskText]);
  const includeGlobs = useMemo(() => parsePatternLines(includeText), [includeText]);
  const excludeGlobs = useMemo(() => parsePatternLines(excludeText), [excludeText]);
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
  const testerSessionCount = tasks.length * Math.max(1, testerLLMIDs.length);
  const estimatedReserve = config
    ? testerSessionCount * config.tester_session_reserve_cents +
      (mode === "optimize" ? config.editor_session_reserve_cents : 0)
    : 0;

  const onFolderChange = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.currentTarget.files ?? []);
    setBrowserFiles(files);
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
      selectedFiles,
      selectedBytes,
      testerLLMIDs,
      mode,
      editorLLMID,
      liveVerify,
      runtimeSecrets,
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
        files: selectedFiles,
        testerLLMIDs,
        editorLLMID: mode === "optimize" ? editorLLMID : undefined,
        includeGlobs,
        excludeGlobs,
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
            Upload a local docs folder, set the managed run options, and start the same check or optimize flow used by the CLI.
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
              <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <h2 className="text-sm font-medium">Mode</h2>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Optimize runs the tester sessions first, then starts one editor session.
                  </p>
                </div>
                <div className="inline-flex border border-border">
                  <ModeButton active={mode === "check"} onClick={() => setMode("check")}>
                    Check
                  </ModeButton>
                  <ModeButton active={mode === "optimize"} onClick={() => setMode("optimize")}>
                    Optimize
                  </ModeButton>
                </div>
              </div>
            </section>

            <section className="border border-border bg-card p-4">
              <div className="mb-3 flex items-start justify-between gap-4">
                <div>
                  <h2 className="text-sm font-medium">Docs Folder</h2>
                  <p className="mt-1 text-xs text-muted-foreground">
                    The service builds the final docs bundle with the same filtering rules as the CLI.
                  </p>
                </div>
              </div>
              <label className="flex min-h-36 cursor-pointer flex-col items-center justify-center border border-dashed border-border bg-background px-4 py-6 text-center hover:border-muted-foreground/60">
                <FolderOpen className="mb-3 h-6 w-6 text-muted-foreground" />
                <span className="text-sm font-medium">Choose Docs Folder</span>
                <span className="mt-1 text-xs text-muted-foreground">
                  Files are staged only for this run.
                </span>
                <input
                  {...directoryInputProps}
                  type="file"
                  multiple
                  className="sr-only"
                  onChange={onFolderChange}
                />
              </label>
              <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
                <span className="border border-border bg-background px-2 py-1">
                  {selectedFiles.length} Selected
                </span>
                <span className="border border-border bg-background px-2 py-1">
                  {formatBytes(selectedBytes)} Uploaded
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
            </section>

            <section className="border border-border bg-card p-4">
              <h2 className="text-sm font-medium">Tasks</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Enter one task per paragraph or line. Managed runs support up to {config.max_tasks_per_run} tasks.
              </p>
              <textarea
                value={taskText}
                onChange={(event) => setTaskText(event.target.value)}
                className="mt-3 min-h-36 w-full border border-border bg-background px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground hover:border-muted-foreground/60 focus:border-brand"
              />
              <div className="mt-2 text-xs text-muted-foreground">
                {tasks.length} {tasks.length === 1 ? "task" : "tasks"} parsed
              </div>
            </section>

            <section className="border border-border bg-card p-4">
              <h2 className="text-sm font-medium">Models</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Tester model choices are used for every task. The editor model is used only for optimize runs.
              </p>
              <div className="mt-4 grid gap-4 lg:grid-cols-2">
                <div>
                  <div className="mb-2 text-xs uppercase tracking-widest text-muted-foreground">
                    Tester Models
                  </div>
                  <div className="grid gap-2">
                    {config.allowed_llm_ids.map((llmID) => (
                      <label key={llmID} className="flex items-center gap-2 text-sm">
                        <Checkbox
                          checked={testerLLMIDs.includes(llmID)}
                          onChange={() => toggleTesterLLM(llmID)}
                        />
                        <span>{llmID}</span>
                      </label>
                    ))}
                  </div>
                </div>
                <div className={mode === "optimize" ? "" : "opacity-50"}>
                  <label htmlFor="editor-llm" className="mb-2 block text-xs uppercase tracking-widest text-muted-foreground">
                    Editor Model
                  </label>
                  <select
                    id="editor-llm"
                    value={editorLLMID}
                    onChange={(event) => setEditorLLMID(event.target.value)}
                    disabled={mode !== "optimize"}
                    className="w-full border border-border bg-background px-3 py-2 text-sm text-foreground outline-none transition-colors hover:border-muted-foreground/60 focus:border-brand disabled:cursor-not-allowed"
                  >
                    {config.allowed_llm_ids.map((llmID) => (
                      <option key={llmID} value={llmID}>
                        {llmID}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            </section>

            <details className="group border border-border bg-card p-4">
              <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-sm font-medium">
                Advanced Options
                <ChevronDown className="h-4 w-4 text-muted-foreground transition-transform group-open:rotate-180" />
              </summary>
              <div className="mt-4 grid gap-4 lg:grid-cols-2">
                <div>
                  <label className="mb-2 block text-xs uppercase tracking-widest text-muted-foreground">
                    Include Patterns
                  </label>
                  <textarea
                    value={includeText}
                    onChange={(event) => setIncludeText(event.target.value)}
                    placeholder="examples/**/*.py"
                    className="min-h-28 w-full border border-border bg-background px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground hover:border-muted-foreground/60 focus:border-brand"
                  />
                </div>
                <div>
                  <label className="mb-2 block text-xs uppercase tracking-widest text-muted-foreground">
                    Exclude Patterns
                  </label>
                  <textarea
                    value={excludeText}
                    onChange={(event) => setExcludeText(event.target.value)}
                    placeholder="generated/**"
                    className="min-h-28 w-full border border-border bg-background px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground hover:border-muted-foreground/60 focus:border-brand"
                  />
                </div>
              </div>
              <div className="mt-5 border-t border-border pt-4">
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={liveVerify}
                    onChange={(event) => setLiveVerify(event.currentTarget.checked)}
                  />
                  <span>Pass runtime secrets for live verification</span>
                </label>
                {liveVerify && (
                  <div className="mt-3 flex flex-col gap-2">
                    {runtimeSecrets.map((secret, index) => (
                      <div key={index} className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
                        <Input
                          value={secret.name}
                          onChange={(event) => updateSecret(index, "name", event.target.value)}
                          placeholder="STRIPE_TEST_SECRET_KEY"
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
                    <Button type="button" variant="outline" size="sm" onClick={addSecret} className="w-fit">
                      <Plus className="mr-1.5 h-3.5 w-3.5" />
                      Add Secret
                    </Button>
                  </div>
                )}
              </div>
            </details>
          </div>

          <aside className="h-fit border border-border bg-card p-4">
            <h2 className="text-sm font-medium">Summary</h2>
            <div className="mt-4 grid gap-3 text-sm">
              <SummaryRow label="Mode" value={mode === "optimize" ? "Optimize" : "Check"} />
              <SummaryRow label="Tasks" value={String(tasks.length)} />
              <SummaryRow label="Tester Sessions" value={String(testerSessionCount)} />
              {mode === "optimize" && <SummaryRow label="Editor Sessions" value="1" />}
              <SummaryRow label="Upload" value={formatBytes(selectedBytes)} />
              <SummaryRow label="Reserved" value={formatCents(estimatedReserve)} />
            </div>
            <Button
              type="button"
              className="mt-5 w-full"
              onClick={() => void submit()}
              disabled={submitting}
            >
              {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {submitting ? "Starting..." : "Start Run"}
            </Button>
            <p className="mt-3 text-xs text-muted-foreground">
              Final charge reconciles to actual Dari session cost after completion.
            </p>
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
        "h-9 px-4 text-sm transition-colors",
        active ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-accent hover:text-foreground"
      )}
    >
      {children}
    </button>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border pb-2 last:border-b-0 last:pb-0">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{value}</span>
    </div>
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
}): string | null {
  if (selectedFiles.length === 0) {
    return "Choose a docs folder with at least one uploadable file.";
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

function parseTasks(value: string): string[] {
  const normalized = value.replace(/\r\n/g, "\n").trim();
  if (!normalized) return [];
  const chunks = normalized.includes("\n\n")
    ? normalized.split(/\n\s*\n/g)
    : normalized.split("\n");
  return chunks
    .map((chunk) => chunk.replace(/^\s*(?:[-*]|\d+[.)])\s+/, "").trim())
    .filter(Boolean);
}

function parsePatternLines(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith("#"));
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

function looksLikeDocsPath(filePath: string): boolean {
  const name = filePath.split("/").pop() ?? filePath;
  if (defaultDocsNames.has(name)) return true;
  const dot = name.lastIndexOf(".");
  return dot >= 0 && defaultDocsExts.has(name.slice(dot));
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
  const target = normalized.includes("/") ? rel : (rel.split("/").pop() ?? rel);
  return new RegExp(`^${globRegExp(normalized)}$`).test(target);
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

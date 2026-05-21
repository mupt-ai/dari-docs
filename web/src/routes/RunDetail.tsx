import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ChevronDown, ChevronRight, Copy, Download, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  downloadUpdatedDocs,
  formatLLMID,
  getRun,
  getRunSessionTranscript,
  isActiveRun,
  type RunSession,
  type RunSessionTranscript,
  type RunStatus,
} from "@/lib/runs";
import { formatCents, formatDate, formatDuration, toTitleCase } from "@/lib/utils";
import { StatusBadge } from "@/routes/Runs";

export default function RunDetail() {
  const { runId } = useParams();
  const [run, setRun] = useState<RunStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [copyFeedbackError, setCopyFeedbackError] = useState<string | null>(null);
  const [downloading, setDownloading] = useState(false);
  const [copiedFeedback, setCopiedFeedback] = useState(false);
  const copiedFeedbackTimerRef = useRef<number | null>(null);
  const [selectedTaskIndex, setSelectedTaskIndex] = useState(1);
  const [selectedResultKey, setSelectedResultKey] = useState("");

  const refresh = useCallback(async (quiet = false) => {
    if (!runId) return;
    if (quiet) setRefreshing(true);
    else setLoading(true);
    setError(null);
    try {
      setRun(await getRun(runId));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [runId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!run || !isActiveRun(run.status)) return;
    const id = window.setInterval(() => {
      void refresh(true);
    }, 7000);
    return () => window.clearInterval(id);
  }, [refresh, run]);

  const handleCopyAllFeedback = async () => {
    if (!run) return;
    const text = allFeedbackMarkdown(run);
    if (!text) {
      setCopyFeedbackError("No Feedback Available To Copy.");
      return;
    }
    setCopyFeedbackError(null);
    try {
      await copyTextToClipboard(text);
      setCopiedFeedback(true);
      if (copiedFeedbackTimerRef.current !== null) {
        window.clearTimeout(copiedFeedbackTimerRef.current);
      }
      copiedFeedbackTimerRef.current = window.setTimeout(() => {
        setCopiedFeedback(false);
        copiedFeedbackTimerRef.current = null;
      }, 1800);
    } catch (e) {
      setCopyFeedbackError(e instanceof Error ? e.message : String(e));
    }
  };

  useEffect(() => {
    return () => {
      if (copiedFeedbackTimerRef.current !== null) {
        window.clearTimeout(copiedFeedbackTimerRef.current);
      }
    };
  }, []);

  const handleDownload = async () => {
    if (!run) return;
    setDownloading(true);
    setDownloadError(null);
    try {
      const blob = await downloadUpdatedDocs(run.id);
      const url = window.URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `${run.id}-updated-docs.zip`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.URL.revokeObjectURL(url);
    } catch (e) {
      setDownloadError(e instanceof Error ? e.message : String(e));
    } finally {
      setDownloading(false);
    }
  };

  useEffect(() => {
    if (!run) return;
    const groups = taskGroups(run);
    const selectedGroup = groups.find((group) => group.taskIndex === selectedTaskIndex) ?? groups[0];
    if (!selectedGroup) return;
    if (selectedGroup.taskIndex !== selectedTaskIndex) {
      setSelectedTaskIndex(selectedGroup.taskIndex);
    }
    if (selectedGroup.results.length > 0 && !selectedGroup.results.some((result) => result.key === selectedResultKey)) {
      setSelectedResultKey(selectedGroup.results[0].key);
    }
  }, [run, selectedTaskIndex, selectedResultKey]);

  return (
    <div className="px-6 py-6">
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <Link to="/runs" className="text-xs uppercase tracking-widest text-muted-foreground hover:text-foreground">
            Runs
          </Link>
          <h1 className="mt-2 break-all text-xl font-medium">{runId}</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {run && allFeedbackMarkdown(run) && (
            <Button type="button" variant="outline" size="sm" onClick={handleCopyAllFeedback}>
              <Copy className="mr-1.5 h-3.5 w-3.5" />
              {copiedFeedback ? "Copied" : "Copy All Feedback"}
            </Button>
          )}
          {run?.updated_docs_available && (
            <Button type="button" variant="outline" size="sm" onClick={handleDownload} disabled={downloading}>
              <Download className="mr-1.5 h-3.5 w-3.5" />
              {downloading ? "Downloading..." : "Updated Docs"}
            </Button>
          )}
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void refresh(true)}
            disabled={refreshing}
          >
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            Refresh
          </Button>
        </div>
      </div>

      {error && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {error}
        </div>
      )}
      {downloadError && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {downloadError}
        </div>
      )}
      {copyFeedbackError && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {copyFeedbackError}
        </div>
      )}

      {loading && run === null ? (
        <div className="text-sm text-muted-foreground">Loading Run...</div>
      ) : run ? (
        <div className="flex flex-col gap-6">
          <div className="grid gap-4 md:grid-cols-5">
            <Summary label="Status" value={<StatusBadge status={run.status} />} />
            <Summary label="Type" value={<span>{toTitleCase(run.mode)}</span>} />
            <Summary label="Source" value={<span>{toTitleCase(run.source ?? "cli")}</span>} />
            <Summary label="Cost" value={<span>{formatCents(run.charged_cents)}{run.estimated ? " Est." : ""}</span>} />
            <Summary label="Completed" value={<span>{formatDate(run.completed_at)}</span>} />
          </div>

          {run.error && (
            <section className="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive-foreground">
              {run.error}
            </section>
          )}

          <TaskResults
            run={run}
            selectedTaskIndex={selectedTaskIndex}
            selectedResultKey={selectedResultKey}
            onSelectTask={(taskIndex) => {
              const group = taskGroups(run).find((item) => item.taskIndex === taskIndex);
              setSelectedTaskIndex(taskIndex);
              setSelectedResultKey(group?.results[0]?.key ?? "");
            }}
            onSelectResult={setSelectedResultKey}
          />

          {run.mode === "optimize" && (
            <section className="border border-border bg-card p-4">
              <SessionHeader
                label="Editor Session"
                session={editorSession(run.sessions)}
                fallbackStatus={editorFallbackStatus(run)}
              />
              {editorSession(run.sessions) && (
                <div className="mt-3">
                  <SessionTranscriptToggle runId={run.id} session={editorSession(run.sessions)!} />
                </div>
              )}
            </section>
          )}

        </div>
      ) : null}
    </div>
  );
}

type TaskResult = {
  key: string;
  llmID: string;
  session?: RunSession;
  feedback?: string;
};

type TaskGroup = {
  taskIndex: number;
  task: string;
  results: TaskResult[];
};

function taskGroups(run: RunStatus): TaskGroup[] {
  const tasks = run.tasks ?? [];
  const groups = new Map<number, TaskGroup>();
  const plannedLLMIDs = plannedTesterLLMIDs(run);
  const sessionsByKey = new Map<string, RunSession>();
  const feedbackByKey = completedFeedbackByKey(run);

  tasks.forEach((task, index) => {
    const taskIndex = index + 1;
    groups.set(taskIndex, {
      taskIndex,
      task,
      results: plannedLLMIDs.map((llmID) => ({
        key: taskResultKey(taskIndex, llmID),
        llmID,
        session: undefined,
        feedback: undefined,
      })),
    });
  });

  const testerSessions = (run.sessions ?? []).filter((session) => session.kind === "tester");
  testerSessions.forEach((session) => {
    sessionsByKey.set(taskResultKey(session.task_index || 1, session.llm_id), session);
  });

  groups.forEach((group) => {
    group.results = group.results.map((result) => ({
      ...result,
      session: sessionsByKey.get(result.key),
      feedback: feedbackByKey.get(result.key),
    }));
  });

  testerSessions.forEach((session) => {
    const taskIndex = Math.max(1, session.task_index || 1);
    const group = groups.get(taskIndex) ?? {
      taskIndex,
      task: tasks[taskIndex - 1] ?? `Task ${taskIndex}`,
      results: [],
    };
    const key = taskResultKey(taskIndex, session.llm_id);
    if (!group.results.some((result) => result.key === key)) {
      group.results.push({ key, llmID: session.llm_id, session, feedback: feedbackByKey.get(key) });
    }
    groups.set(taskIndex, group);
  });

  return Array.from(groups.values()).sort((a, b) => a.taskIndex - b.taskIndex);
}

function plannedTesterLLMIDs(run: RunStatus): string[] {
  const fromRun = uniqueStrings(
    (run.llms ?? [])
      .filter((item) => item.role === "tester")
      .map((item) => item.llm_id)
  );
  if (fromRun.length > 0) return fromRun;
  return uniqueStrings(
    (run.sessions ?? [])
      .filter((session) => session.kind === "tester")
      .map((session) => session.llm_id)
  );
}

function completedFeedbackByKey(run: RunStatus): Map<string, string> {
  const out = new Map<string, string>();
  const completedSessions = (run.sessions ?? [])
    .filter((session) => session.kind === "tester" && session.status === "completed");
  completedSessions.forEach((session, index) => {
    const feedback = run.feedback_reports?.[index];
    if (feedback) {
      out.set(taskResultKey(session.task_index || 1, session.llm_id), feedback);
    }
  });
  return out;
}

function taskResultKey(taskIndex: number, llmID: string): string {
  return `tester:${taskIndex}:${llmID.trim() || "default"}`;
}

function uniqueStrings(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of values) {
    const value = raw.trim();
    if (!value || seen.has(value)) continue;
    seen.add(value);
    out.push(value);
  }
  return out;
}

function allFeedbackMarkdown(run: RunStatus): string {
  const groups = taskGroups(run);
  const sections: string[] = [];

  for (const group of groups) {
    const feedbackResults = group.results.filter((result) => result.feedback?.trim());
    if (feedbackResults.length === 0) continue;

    const taskParts = [`## Task ${group.taskIndex}`];
    const task = group.task.trim();
    if (task) taskParts.push(task);

    for (const result of feedbackResults) {
      taskParts.push(`### ${formatLLMID(result.llmID)} Feedback`);
      taskParts.push(result.feedback!.trim());
    }
    sections.push(taskParts.join("\n\n"));
  }

  if (sections.length === 0) return "";

  const meta = [
    "# Dari Docs Feedback",
    `Run: ${run.id}`,
    `Status: ${toTitleCase(run.status)}`,
    `Type: ${toTitleCase(run.mode)}`,
  ];
  if (run.completed_at) meta.push(`Completed: ${formatDate(run.completed_at)}`);
  return `${meta.join("\n")}\n\n${sections.join("\n\n")}\n`;
}

async function copyTextToClipboard(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall through to the textarea fallback for browsers that expose but block Clipboard API.
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  textarea.style.top = "0";
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand("copy");
  textarea.remove();
  if (!copied) {
    throw new Error("Could Not Copy Feedback To Clipboard.");
  }
}

function TaskResults({
  run,
  selectedTaskIndex,
  selectedResultKey,
  onSelectTask,
  onSelectResult,
}: {
  run: RunStatus;
  selectedTaskIndex: number;
  selectedResultKey: string;
  onSelectTask: (taskIndex: number) => void;
  onSelectResult: (key: string) => void;
}) {
  const groups = taskGroups(run);
  const selectedGroup = groups.find((group) => group.taskIndex === selectedTaskIndex) ?? groups[0];
  const selectedResult = selectedGroup?.results.find((result) => result.key === selectedResultKey) ?? selectedGroup?.results[0];
  const selectedResultStatus = sessionStatus(selectedResult?.session, run.status);

  if (!selectedGroup) return null;

  return (
    <section className="border border-border bg-card p-4">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0 flex-1">
          <label htmlFor="task-select" className="text-xs uppercase tracking-widest text-muted-foreground">
            Task
          </label>
          <div className="relative mt-2">
            <select
              id="task-select"
              value={selectedGroup.taskIndex}
              onChange={(event) => onSelectTask(Number(event.target.value))}
              className="w-full appearance-none border border-border bg-background py-2 pl-3 pr-10 text-sm text-foreground outline-none transition-colors hover:border-muted-foreground/60 focus:border-brand"
            >
              {groups.map((group) => (
                <option key={group.taskIndex} value={group.taskIndex}>
                  Task {group.taskIndex}
                </option>
              ))}
            </select>
            <ChevronDown className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          </div>
        </div>
      </div>

      <div className="mb-4 border border-border bg-background p-3 text-sm">
        <div className="mb-2 text-xs uppercase tracking-widest text-muted-foreground">Prompt</div>
        <div className="whitespace-pre-wrap">{selectedGroup.task}</div>
      </div>

      {selectedGroup.results.length > 0 ? (
        <>
          <div className="mb-4 flex min-h-9 items-end gap-6 overflow-x-auto overflow-y-hidden border-b border-border">
            {selectedGroup.results.map((result) => {
              const active = result.key === (selectedResult?.key ?? "");
              return (
                <button
                  key={result.key}
                  type="button"
                  onClick={() => onSelectResult(result.key)}
                  className={`-mb-px whitespace-nowrap border-b-2 px-1 pb-2 text-sm transition-colors ${
                    active
                      ? "border-brand text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {formatLLMID(result.llmID)}
                </button>
              );
            })}
          </div>

          <div className="border border-border bg-background text-sm">
            <div className="border-b border-border bg-card/40 px-3 py-2.5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate font-medium">{formatLLMID(selectedResult?.llmID)} Feedback</div>
                  <div className="mt-1 text-xs text-muted-foreground">{runMetaLine(run)}</div>
                </div>
                <SessionStatus status={selectedResultStatus} />
              </div>
            </div>
            <div className="p-3">
              {selectedResult?.feedback ? (
                <RawFeedback text={selectedResult.feedback} />
              ) : (
                <div className="text-muted-foreground">No Feedback Available.</div>
              )}
              {selectedResult?.session && (
                <div className="mt-3">
                  <SessionTranscriptToggle runId={run.id} session={selectedResult.session} />
                </div>
              )}
            </div>
          </div>
        </>
      ) : (
        <div className="border border-border bg-background p-4 text-sm text-muted-foreground">
          No Model Results For This Task Yet.
        </div>
      )}
    </section>
  );
}

function isLiveSessionStatus(status: string): boolean {
  return status === "starting" || status === "running" || status === "queued" || status === "uploading";
}

function SessionTranscriptToggle({ runId, session }: { runId: string; session: RunSession }) {
  const [open, setOpen] = useState(false);
  const [transcript, setTranscript] = useState<RunSessionTranscript | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const currentRequestKeyRef = useRef(`${runId}\u0000${session.id}`);
  const activeRequestRef = useRef<AbortController | null>(null);

  useEffect(() => {
    currentRequestKeyRef.current = `${runId}\u0000${session.id}`;
    activeRequestRef.current?.abort();
    activeRequestRef.current = null;
    setTranscript(null);
    setError(null);
    setLoading(false);

    return () => {
      activeRequestRef.current?.abort();
      activeRequestRef.current = null;
    };
  }, [runId, session.id]);

  const refreshTranscript = useCallback(async (quiet = false) => {
    if (!session.id || activeRequestRef.current) return;

    const requestKey = `${runId}\u0000${session.id}`;
    const controller = new AbortController();
    activeRequestRef.current = controller;

    if (!quiet) setLoading(true);
    setError(null);
    try {
      const nextTranscript = await getRunSessionTranscript(runId, session.id, { signal: controller.signal });
      if (currentRequestKeyRef.current === requestKey) {
        setTranscript(nextTranscript);
      }
    } catch (e) {
      if (!controller.signal.aborted && currentRequestKeyRef.current === requestKey) {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      if (activeRequestRef.current === controller) {
        activeRequestRef.current = null;
      }
      if (currentRequestKeyRef.current === requestKey) {
        setLoading(false);
      }
    }
  }, [runId, session.id]);

  useEffect(() => {
    if (!open) return;
    void refreshTranscript();
  }, [open, refreshTranscript]);

  useEffect(() => {
    if (!open || !isLiveSessionStatus(session.status)) return;
    const id = window.setInterval(() => {
      void refreshTranscript(true);
    }, 5000);
    return () => window.clearInterval(id);
  }, [open, refreshTranscript, session.status]);

  return (
    <div className="border border-border bg-card/60">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-muted-foreground hover:bg-muted/40"
      >
        {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        <span>{open ? "Hide Session" : "Show Session"}</span>
        <code className="ml-auto truncate font-mono text-foreground">{session.id}</code>
      </button>
      {open && (
        <div className="border-t border-border p-3">
          {error && (
            <div className="mb-3 border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive-foreground">
              {error}
            </div>
          )}
          {loading && transcript === null ? (
            <div className="text-sm text-muted-foreground">Loading Session...</div>
          ) : transcript ? (
            <TranscriptView transcript={transcript} />
          ) : !error ? (
            <div className="text-sm text-muted-foreground">No Session Loaded.</div>
          ) : null}
        </div>
      )}
    </div>
  );
}

function TranscriptView({ transcript }: { transcript: RunSessionTranscript }) {
  const items = transcriptItems(transcript);
  if (items.length === 0) {
    return <JsonBlock value={transcript} emptyText="No Transcript Events Yet." />;
  }
  return (
    <div className="space-y-3">
      {items.map((item, index) => (
        <TranscriptItemCard key={transcriptItemKey(item, index)} item={item} />
      ))}
    </div>
  );
}

function TranscriptItemCard({ item }: { item: Record<string, unknown> }) {
  const type = typeof item.type === "string" ? item.type : "event";
  const title = transcriptItemTitle(type);
  const status = typeof item.status === "string" ? item.status : "";
  const createdAt = typeof item.created_at === "string" ? item.created_at : "";
  const content = Array.isArray(item.content) ? item.content : [];
  const errorMessage = typeof item.error_message === "string" ? item.error_message : "";

  return (
    <div className="border border-border bg-background">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-muted/30 px-3 py-2">
        <span className="text-xs uppercase tracking-widest text-muted-foreground">{title}</span>
        <span className="text-xs text-muted-foreground">
          {[status, createdAt ? formatDate(createdAt) : ""].filter(Boolean).join(" · ")}
        </span>
      </div>
      <div className="space-y-2 px-3 py-3">
        {content.length > 0 ? (
          content.map((part, index) => <TranscriptPart key={index} part={part} />)
        ) : type === "tool_call" ? (
          <JsonBlock value={toolCallSummary(item)} />
        ) : (
          <JsonBlock value={item} />
        )}
        {errorMessage && (
          <div className="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive-foreground">
            {errorMessage}
          </div>
        )}
      </div>
    </div>
  );
}

function TranscriptPart({ part }: { part: unknown }) {
  if (typeof part === "object" && part !== null) {
    const record = part as Record<string, unknown>;
    if (record.type === "text" && typeof record.text === "string") {
      return <div className="whitespace-pre-wrap text-sm text-foreground">{record.text}</div>;
    }
    if (record.type === "thinking" && typeof record.thinking === "string") {
      return (
        <details className="border border-border">
          <summary className="cursor-pointer px-3 py-2 text-xs text-muted-foreground">Thinking</summary>
          <pre className="whitespace-pre-wrap border-t border-border px-3 py-2 font-mono text-xs text-muted-foreground">
            {record.thinking}
          </pre>
        </details>
      );
    }
    if (typeof record.file_id === "string") {
      return (
        <div className="text-xs text-muted-foreground">
          {String(record.type ?? "file")} <code className="text-foreground">{record.file_id}</code>
        </div>
      );
    }
  }
  return <JsonBlock value={part} />;
}

function transcriptItems(transcript: RunSessionTranscript): Record<string, unknown>[] {
  const timeline = transcript.timeline;
  if (typeof timeline === "object" && timeline !== null) {
    const items = (timeline as { items?: unknown }).items;
    if (Array.isArray(items)) {
      return items.filter((item): item is Record<string, unknown> => typeof item === "object" && item !== null);
    }
  }
  return [];
}

function transcriptItemKey(item: Record<string, unknown>, index: number): string {
  const id = item.id;
  return typeof id === "string" && id ? id : String(index);
}

function transcriptItemTitle(type: string): string {
  if (type === "user_message") return "User";
  if (type === "assistant_message") return "Assistant";
  if (type === "tool_call") return "Tool Call";
  return toTitleCase(type.replace(/_/g, " "));
}

function toolCallSummary(item: Record<string, unknown>): Record<string, unknown> {
  return {
    tool_name: item.tool_name,
    arguments: item.arguments,
    result: item.result,
    is_error: item.is_error,
  };
}

function JsonBlock({ value, emptyText = "No Details." }: { value: unknown; emptyText?: string }) {
  const text = safeStringify(value);
  if (!text || text === "{}") return <div className="text-sm text-muted-foreground">{emptyText}</div>;
  return <pre className="whitespace-pre-wrap break-all font-mono text-xs text-foreground">{text}</pre>;
}

function safeStringify(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function editorSession(sessions: RunSession[] | undefined): RunSession | undefined {
  return sessions?.find((session) => session.kind === "editor");
}

function editorFallbackStatus(run: RunStatus): string {
  if (run.status === "failed" || run.status === "completed") return run.status;
  const testers = run.sessions?.filter((session) => session.kind === "tester") ?? [];
  const taskCount = run.task_count || run.tasks?.length || 0;
  if (taskCount === 0 || testers.length < taskCount) return "waiting";
  if (testers.some((session) => session.status !== "completed")) return "waiting";
  return "queued";
}

function runMetaLine(run: RunStatus): string {
  const parts = [formatDate(run.completed_at ?? run.created_at)];
  parts.push(`${formatCents(run.charged_cents)}${run.estimated ? " Est." : ""}`);
  const end = run.completed_at ?? (isActiveRun(run.status) ? new Date().toISOString() : null);
  if (end) {
    const duration = formatDuration(run.created_at, end);
    if (duration) parts.push(`${duration} Total`);
  }
  return parts.join(" · ");
}

function SessionHeader({
  label,
  session,
  fallbackStatus,
}: {
  label: string;
  session?: RunSession;
  fallbackStatus: string;
}) {
  return (
    <div className="flex min-h-8 flex-wrap items-center justify-between gap-2">
      <div className="inline-flex min-w-0 items-center gap-2 text-sm font-medium">
        <span>{label}</span>
        <SessionModel session={session} />
      </div>
      <SessionMeta session={session} fallbackStatus={fallbackStatus} />
    </div>
  );
}

function SessionModel({ session }: { session?: RunSession }) {
  const llmID = formatLLMID(session?.llm_id);
  if (llmID === "-") return null;
  return <span className="min-w-0 truncate text-muted-foreground">- {llmID}</span>;
}

function sessionStatus(session: RunSession | undefined, fallbackStatus: string): string {
  return session?.status ?? fallbackStatus;
}

function SessionMeta({
  session,
  fallbackStatus,
}: {
  session?: RunSession;
  fallbackStatus: string;
}) {
  return (
    <div className="flex flex-wrap items-center justify-end gap-2 text-xs">
      {session?.completed_at && (
        <span className="text-muted-foreground">Completed {formatDate(session.completed_at)}</span>
      )}
      <SessionStatus status={sessionStatus(session, fallbackStatus)} />
    </div>
  );
}

function SessionStatus({ status }: { status: string }) {
  const failed = status === "failed";
  const waiting = status === "waiting" || status === "queued";
  const active = isActiveRun(status) && !waiting;
  return (
    <span
      className={
        failed
          ? "inline-flex min-w-20 justify-center border border-destructive/60 bg-destructive/10 px-2 py-1 text-xs text-destructive-foreground"
          : active
            ? "inline-flex min-w-20 justify-center border border-brand/60 bg-brand/10 px-2 py-1 text-xs text-brand"
            : waiting
              ? "inline-flex min-w-20 justify-center border border-border bg-muted/30 px-2 py-1 text-xs text-muted-foreground"
              : "inline-flex min-w-20 justify-center border border-border bg-muted/40 px-2 py-1 text-xs text-muted-foreground"
      }
    >
      {toTitleCase(status)}
    </span>
  );
}

function Summary({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="border border-border bg-card p-4">
      <div className="text-xs uppercase tracking-widest text-muted-foreground">{label}</div>
      <div className="mt-2 text-sm">{value}</div>
    </div>
  );
}

function RawFeedback({ text }: { text: string }) {
  return <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-foreground">{text}</pre>;
}

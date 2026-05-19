import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import ReactMarkdown from "react-markdown";
import { Download, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { downloadUpdatedDocs, formatLLMID, getRun, isActiveRun, type RunSession, type RunStatus } from "@/lib/runs";
import { formatCents, formatDate, toTitleCase } from "@/lib/utils";
import { StatusBadge } from "@/routes/Runs";

export default function RunDetail() {
  const { runId } = useParams();
  const [run, setRun] = useState<RunStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [downloading, setDownloading] = useState(false);
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

      {loading && run === null ? (
        <div className="text-sm text-muted-foreground">Loading Run...</div>
      ) : run ? (
        <div className="flex flex-col gap-6">
          <div className="grid gap-4 md:grid-cols-4">
            <Summary label="Status" value={<StatusBadge status={run.status} />} />
            <Summary label="Type" value={<span>{toTitleCase(run.mode)}</span>} />
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

  if (!selectedGroup) return null;

  return (
    <section className="border border-border bg-card p-4">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div className="min-w-0 flex-1">
          <label htmlFor="task-select" className="text-xs uppercase tracking-widest text-muted-foreground">
            Task
          </label>
          <select
            id="task-select"
            value={selectedGroup.taskIndex}
            onChange={(event) => onSelectTask(Number(event.target.value))}
            className="mt-2 w-full border border-border bg-background px-3 py-2 text-sm text-foreground outline-none transition-colors hover:border-muted-foreground/60 focus:border-brand"
          >
            {groups.map((group) => (
              <option key={group.taskIndex} value={group.taskIndex}>
                Task {group.taskIndex}
              </option>
            ))}
          </select>
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

          <div className="border border-border bg-background p-3 text-sm">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div className="font-medium">{formatLLMID(selectedResult?.llmID)}</div>
              <SessionMeta session={selectedResult?.session} fallbackStatus={run.status} />
            </div>
            {selectedResult?.feedback ? (
              <Markdown text={selectedResult.feedback} />
            ) : (
              <div className="text-muted-foreground">No Feedback Available.</div>
            )}
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

function Markdown({ text }: { text: string }) {
  return (
    <div className="prose prose-invert max-w-none text-sm prose-p:my-2 prose-pre:border prose-pre:border-border prose-pre:bg-background prose-pre:p-3">
      <ReactMarkdown skipHtml>{text}</ReactMarkdown>
    </div>
  );
}

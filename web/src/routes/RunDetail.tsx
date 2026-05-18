import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import ReactMarkdown from "react-markdown";
import { Download, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { downloadUpdatedDocs, formatLLMID, getRun, isActiveRun, type RunSession, type RunStatus } from "@/lib/runs";
import { formatCents, formatDate } from "@/lib/utils";
import { LLMSummary, StatusBadge } from "@/routes/Runs";

export default function RunDetail() {
  const { runId } = useParams();
  const [run, setRun] = useState<RunStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [downloadError, setDownloadError] = useState<string | null>(null);
  const [downloading, setDownloading] = useState(false);

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
              {downloading ? "Downloading..." : "Updated docs"}
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
        <div className="text-sm text-muted-foreground">loading run...</div>
      ) : run ? (
        <div className="flex flex-col gap-6">
          <div className="grid gap-4 md:grid-cols-4">
            <Summary label="Status" value={<StatusBadge status={run.status} />} />
            <Summary label="Type" value={<span className="capitalize">{run.mode}</span>} />
            <Summary label="Cost" value={<span>{formatCents(run.charged_cents)}{run.estimated ? " est." : ""}</span>} />
            <Summary label="Completed" value={<span>{formatDate(run.completed_at)}</span>} />
          </div>

          <section className="border border-border bg-card p-4">
            <div className="mb-3 text-sm font-medium">Run context</div>
            <div className="grid gap-4 text-sm md:grid-cols-3">
              <div>
                <div className="text-xs uppercase tracking-widest text-muted-foreground">Created</div>
                <div className="mt-1">{formatDate(run.created_at)}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-widest text-muted-foreground">Reserved</div>
                <div className="mt-1">{formatCents(run.reserved_cents)}</div>
              </div>
              <div>
                <div className="text-xs uppercase tracking-widest text-muted-foreground">LLMs</div>
                <div className="mt-1 text-muted-foreground">
                  <LLMSummary llms={run.llms} />
                </div>
              </div>
            </div>
            {run.error && (
              <div className="mt-4 border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive-foreground">
                {run.error}
              </div>
            )}
          </section>

          <section className="border border-border bg-card p-4">
            <div className="mb-3 text-sm font-medium">Tasks</div>
            <div className="flex flex-col gap-3">
              {run.tasks?.map((task, index) => (
                <div key={`${index}:${task}`} className="border border-border bg-background p-3 text-sm">
                  <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                    <div className="text-xs uppercase tracking-widest text-muted-foreground">
                      Task {index + 1}
                    </div>
                    <SessionSummary session={sessionForTask(run.sessions, index + 1)} fallbackStatus={run.status} />
                  </div>
                  <div className="whitespace-pre-wrap">{task}</div>
                  {run.feedback_reports?.[index] && (
                    <div className="mt-4 border-t border-border pt-4">
                      <div className="mb-2 text-xs uppercase tracking-widest text-muted-foreground">
                        Feedback
                      </div>
                      <Markdown text={run.feedback_reports[index]} />
                    </div>
                  )}
                </div>
              ))}
            </div>
          </section>

          {run.mode === "optimize" && (
            <section className="border border-border bg-card p-4">
              <div className="mb-3 text-sm font-medium">Editor session</div>
              <SessionSummary session={editorSession(run.sessions)} fallbackStatus={editorFallbackStatus(run)} />
            </section>
          )}

          {run.aggregate_feedback && (
            <section className="border border-border bg-card p-4">
              <div className="mb-3 text-sm font-medium">Aggregate feedback</div>
              <Markdown text={run.aggregate_feedback} />
            </section>
          )}
        </div>
      ) : null}
    </div>
  );
}

function sessionForTask(sessions: RunSession[] | undefined, taskIndex: number): RunSession | undefined {
  return sessions?.find((session) => session.kind === "tester" && session.task_index === taskIndex);
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

function SessionSummary({
  session,
  fallbackStatus,
}: {
  session?: RunSession;
  fallbackStatus: string;
}) {
  const status = session?.status ?? fallbackStatus;
  const llmID = formatLLMID(session?.llm_id);
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <SessionStatus status={status} />
      {llmID !== "-" && <span className="text-muted-foreground">{llmID}</span>}
      {session?.completed_at && (
        <span className="text-muted-foreground">completed {formatDate(session.completed_at)}</span>
      )}
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
      {status}
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

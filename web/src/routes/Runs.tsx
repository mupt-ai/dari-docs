import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowDown, ArrowUp, Check, Copy } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { firstLine, formatCents, formatDuration, formatRelativeTime, toTitleCase } from "@/lib/utils";
import {
  isActiveRun,
  listRuns,
  type RunLLM,
  type RunListItem,
  type RunSort,
  type SortDirection,
} from "@/lib/runs";

const columns: Array<{ key: RunSort; label: string; align?: "right" }> = [
  { key: "task", label: "Task" },
  { key: "mode", label: "Type" },
  { key: "llms", label: "Models" },
  { key: "cost", label: "Cost", align: "right" },
  { key: "created_at", label: "When" },
];

export default function Runs() {
  const [runs, setRuns] = useState<RunListItem[] | null>(null);
  const [sort, setSort] = useState<RunSort>("created_at");
  const [direction, setDirection] = useState<SortDirection>("desc");
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (quiet = false) => {
    if (!quiet) {
      setLoading(true);
    }
    setError(null);
    try {
      const runResp = await listRuns({ sort, direction });
      setRuns(runResp.runs);
      setNextCursor(runResp.next_cursor ?? null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [direction, sort]);

  const loadMore = async () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    setError(null);
    try {
      const runResp = await listRuns({ sort, direction, cursor: nextCursor });
      setRuns((prev) => [...(prev ?? []), ...runResp.runs]);
      setNextCursor(runResp.next_cursor ?? null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoadingMore(false);
    }
  };

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const hasActiveRuns = useMemo(
    () => runs?.some((run) => isActiveRun(run.status)) ?? false,
    [runs]
  );

  useEffect(() => {
    if (!hasActiveRuns) return;
    const id = window.setInterval(() => {
      void refresh(true);
    }, 7000);
    return () => window.clearInterval(id);
  }, [hasActiveRuns, refresh]);

  const handleSort = (next: RunSort) => {
    if (next === sort) {
      setDirection((value) => (value === "asc" ? "desc" : "asc"));
    } else {
      setSort(next);
      setDirection(next === "created_at" || next === "cost" ? "desc" : "asc");
    }
  };

  return (
    <div className="px-6 py-6">
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-medium">Runs</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Managed documentation checks and revisions from the CLI.
          </p>
        </div>
      </div>

      {error && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {error}
        </div>
      )}

      {loading && runs === null ? (
        <div className="text-sm text-muted-foreground">Loading Runs...</div>
      ) : runs && runs.length === 0 ? (
        <EmptyRuns />
      ) : (
        <div className="flex flex-col gap-3">
          <div className="overflow-hidden border border-border">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[720px] border-collapse text-sm">
                <thead className="bg-muted/40 text-xs uppercase tracking-widest text-muted-foreground">
                  <tr>
                    {columns.map((column) => (
                      <th
                        key={column.key}
                        className={column.align === "right" ? "px-3 py-2 text-right font-medium" : "px-3 py-2 text-left font-medium"}
                      >
                        <button
                          type="button"
                          onClick={() => handleSort(column.key)}
                          className={column.align === "right" ? "ml-auto inline-flex items-center gap-1 hover:text-foreground" : "inline-flex items-center gap-1 hover:text-foreground"}
                        >
                          {column.label}
                          <SortIcon active={sort === column.key} direction={direction} />
                        </button>
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {runs?.map((run) => (
                    <tr key={run.id} className="border-t border-border hover:bg-accent/40">
                      <td className={cn("max-w-[360px] px-3 py-3", statusBorderClass(run.status))}>
                        <RunWorkload run={run} />
                      </td>
                      <td className="px-3 py-3">{toTitleCase(run.mode)}</td>
                      <td className="px-3 py-3">
                        <LLMBadges llms={run.llms} />
                      </td>
                      <td className="px-3 py-3 text-right">
                        {formatCents(run.charged_cents)}
                        {run.estimated && <span className="ml-1 text-xs text-muted-foreground">Est.</span>}
                      </td>
                      <td className="px-3 py-3">
                        <RunWhen run={run} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          {nextCursor && (
            <div className="flex justify-center">
              <Button type="button" variant="outline" size="sm" onClick={loadMore} disabled={loadingMore}>
                {loadingMore ? "Loading..." : "Load More"}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function statusBorderClass(status: string): string {
  if (isActiveRun(status)) return "border-l-[3px] border-l-brand";
  if (status === "failed") return "border-l-[3px] border-l-destructive/80";
  return "border-l-[3px] border-l-transparent";
}

function SortIcon({ active, direction }: { active: boolean; direction: SortDirection }) {
  const className = active ? "h-3 w-3 shrink-0" : "h-3 w-3 shrink-0 opacity-0";
  return direction === "asc" ? <ArrowUp className={className} /> : <ArrowDown className={className} />;
}

function EmptyRuns() {
  return (
    <div className="border border-border bg-card p-6">
      <div className="text-sm font-medium">No Managed Runs Yet</div>
      <p className="mt-2 text-sm text-muted-foreground">
        Run a check from your docs repo. The run will appear here while it is queued, running, and completed.
      </p>
      <div className="mt-4 flex flex-col gap-3">
        <CopyableCommand command="curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | bash" />
        <CopyableCommand command="dari-docs auth login" />
        <CopyableCommand command={'dari-docs check . --managed --task "Install the SDK and make a first API call"'} />
      </div>
    </div>
  );
}

function CopyableCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="group relative overflow-hidden border border-border bg-background">
      <pre className="overflow-x-auto p-3 pr-12 text-xs text-muted-foreground">
        <code>{command}</code>
      </pre>
      <button
        type="button"
        aria-label="Copy Command"
        onClick={copy}
        className="absolute right-2 top-1/2 inline-flex h-7 w-7 -translate-y-1/2 items-center justify-center border border-border bg-card text-muted-foreground opacity-100 hover:bg-accent hover:text-foreground focus:opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
      >
        {copied ? <Check className="h-3.5 w-3.5 text-brand" /> : <Copy className="h-3.5 w-3.5" />}
      </button>
    </div>
  );
}

export function StatusBadge({ status }: { status: string }) {
  const active = isActiveRun(status);
  const failed = status === "failed";
  return (
    <span
      className={
        failed
          ? "inline-flex min-w-24 justify-center border border-destructive/60 bg-destructive/10 px-2 py-1 text-xs text-destructive-foreground"
          : active
            ? "inline-flex min-w-24 justify-center border border-brand/60 bg-brand/10 px-2 py-1 text-xs text-brand"
            : "inline-flex min-w-24 justify-center border border-border bg-muted/40 px-2 py-1 text-xs text-muted-foreground"
      }
    >
      {toTitleCase(status)}
    </span>
  );
}

function abbreviateLLMID(id: string): string {
  const s = id.replace(/^(anthropic|openai)\//, "");
  const claude = s.match(/^claude-(opus|sonnet|haiku)-(\d+)-(\d+)/i);
  if (claude) {
    return `${claude[1].charAt(0).toUpperCase()}${claude[1].slice(1)} ${claude[2]}.${claude[3]}`;
  }
  const gpt = s.match(/^gpt-(.+)/i);
  if (gpt) return `GPT-${gpt[1]}`;
  return s.length > 14 ? `${s.slice(0, 13)}…` : s;
}

function LLMBadges({ llms }: { llms: RunLLM[] }) {
  if (!llms || llms.length === 0) return <span className="text-muted-foreground">-</span>;
  const unique = [...new Set(llms.map((l) => l.llm_id))];
  const visible = unique.slice(0, 2);
  const overflow = unique.length - visible.length;
  return (
    <div className="flex flex-wrap items-center gap-1">
      {visible.map((id) => (
        <span key={id} className="border border-border bg-muted/40 px-1.5 py-0.5 text-xs text-muted-foreground">
          {abbreviateLLMID(id)}
        </span>
      ))}
      {overflow > 0 && (
        <span className="border border-border bg-muted/20 px-1.5 py-0.5 text-xs text-muted-foreground/60">
          +{overflow}
        </span>
      )}
    </div>
  );
}

function RunWhen({ run }: { run: RunListItem }) {
  const relTime = formatRelativeTime(run.created_at);
  const active = isActiveRun(run.status);
  const failed = run.status === "failed";
  const duration =
    run.completed_at && !active && !failed
      ? formatDuration(run.created_at, run.completed_at)
      : null;
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-muted-foreground">{relTime}</span>
      {active ? (
        <span className="text-xs text-brand">{toTitleCase(run.status)}…</span>
      ) : failed ? (
        <span className="text-xs text-destructive-foreground">Failed</span>
      ) : duration ? (
        <span className="text-xs text-muted-foreground/60">ran {duration}</span>
      ) : null}
    </div>
  );
}

function RunWorkload({ run }: { run: RunListItem }) {
  const headline = firstLine(run.tasks[0] ?? "") || run.id;
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Link to={`/runs/${run.id}`} className="block truncate text-foreground hover:text-brand">
        {headline}
      </Link>
      {run.task_count > 1 && (
        <span className="w-fit border border-border bg-muted/30 px-1.5 py-0.5 text-xs text-muted-foreground">
          {run.task_count}-Task Batch
        </span>
      )}
    </div>
  );
}

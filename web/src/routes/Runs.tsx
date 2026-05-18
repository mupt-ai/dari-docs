import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowDown, ArrowUp, Check, Copy, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { getBalance } from "@/lib/billing";
import { firstLine, formatCents, formatDate } from "@/lib/utils";
import {
  formatLLMID,
  isActiveRun,
  listRuns,
  type RunListItem,
  type RunSort,
  type SortDirection,
} from "@/lib/runs";

const columns: Array<{ key: RunSort; label: string; align?: "right" }> = [
  { key: "status", label: "Status" },
  { key: "mode", label: "Type" },
  { key: "task", label: "Tasks" },
  { key: "cost", label: "Cost", align: "right" },
  { key: "llms", label: "LLMs" },
  { key: "created_at", label: "Created" },
  { key: "completed_at", label: "Completed" },
];

export default function Runs() {
  const [runs, setRuns] = useState<RunListItem[] | null>(null);
  const [balance, setBalance] = useState<number | null>(null);
  const [sort, setSort] = useState<RunSort>("created_at");
  const [direction, setDirection] = useState<SortDirection>("desc");
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async (quiet = false) => {
    if (quiet) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);
    try {
      const [runResp, balanceResp] = await Promise.all([
        listRuns({ sort, direction }),
        getBalance(),
      ]);
      setRuns(runResp.runs);
      setNextCursor(runResp.next_cursor ?? null);
      setBalance(balanceResp.balance_cents);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
      setRefreshing(false);
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
      setDirection(next === "created_at" || next === "completed_at" || next === "cost" ? "desc" : "asc");
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
        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex h-8 items-center justify-center gap-2 whitespace-nowrap border border-border bg-transparent px-3 text-xs font-medium text-foreground">
            <span>Balance</span>
            <span>{balance === null ? "-" : formatCents(balance)}</span>
          </div>
          <Button type="button" variant="outline" size="sm" asChild>
            <Link to="/billing">Purchase credits</Link>
          </Button>
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

      {loading && runs === null ? (
        <div className="text-sm text-muted-foreground">loading runs...</div>
      ) : runs && runs.length === 0 ? (
        <EmptyRuns />
      ) : (
        <div className="flex flex-col gap-3">
          <div className="overflow-hidden border border-border">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[980px] border-collapse text-sm">
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
                          {sort === column.key ? (
                            direction === "asc" ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />
                          ) : null}
                        </button>
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {runs?.map((run) => (
                    <tr key={run.id} className="border-t border-border hover:bg-accent/40">
                      <td className="px-3 py-3">
                        <StatusBadge status={run.status} />
                      </td>
                      <td className="px-3 py-3 capitalize">{run.mode}</td>
                      <td className="max-w-[360px] px-3 py-3">
                        <RunWorkload run={run} />
                      </td>
                      <td className="px-3 py-3 text-right">
                        {formatCents(run.charged_cents)}
                        {run.estimated && <span className="ml-1 text-xs text-muted-foreground">est.</span>}
                      </td>
                      <td className="max-w-[260px] px-3 py-3 text-xs text-muted-foreground">
                        <LLMSummary llms={run.llms} />
                      </td>
                      <td className="px-3 py-3 text-muted-foreground">{formatDate(run.created_at)}</td>
                      <td className="px-3 py-3 text-muted-foreground">{formatDate(run.completed_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          {nextCursor && (
            <div className="flex justify-center">
              <Button type="button" variant="outline" size="sm" onClick={loadMore} disabled={loadingMore}>
                {loadingMore ? "Loading..." : "Load more"}
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function EmptyRuns() {
  return (
    <div className="border border-border bg-card p-6">
      <div className="text-sm font-medium">No managed runs yet</div>
      <p className="mt-2 text-sm text-muted-foreground">
        Run a check from your docs repo. The run will appear here while it is queued, running, and completed.
      </p>
      <div className="mt-4 flex flex-col gap-3">
        <CopyableCommand command="brew install mupt-ai/tap/dari-docs" />
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
        aria-label="Copy command"
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
      {status}
    </span>
  );
}

export function LLMSummary({ llms }: { llms: Array<{ role: string; llm_id: string; count: number }> }) {
  if (!llms || llms.length === 0) return <span>-</span>;
  return (
    <span className="flex flex-col gap-1">
      {llms.map((item) => (
        <span key={`${item.role}:${item.llm_id}`}>
          {formatRole(item.role)}: {formatLLMID(item.llm_id)}
          {item.count > 1 ? ` x${item.count}` : ""}
        </span>
      ))}
    </span>
  );
}

function RunWorkload({ run }: { run: RunListItem }) {
  const headline = firstLine(run.tasks[0] ?? "") || run.id;
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Link to={`/runs/${run.id}`} className="block truncate text-foreground hover:text-brand">
        {headline}
      </Link>
      <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
        <span className="border border-border bg-muted/30 px-1.5 py-0.5">
          {run.task_count > 1 ? `${run.task_count}-task batch` : "1 task"}
        </span>
        <span>
          {run.mode === "optimize" ? "tester sessions + editor" : "tester sessions"}
        </span>
      </div>
    </div>
  );
}

function formatRole(role: string): string {
  if (role === "tester") return "Tester";
  if (role === "editor") return "Editor";
  return role;
}

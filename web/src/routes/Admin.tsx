import { FormEvent, useEffect, useState, type ReactNode } from "react";
import { ArrowLeft, ListChecks, Plus, Search, UserRound } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import {
  getAdminUserDetail,
  grantAdminCredits,
  searchAdminUsers,
  type AdminUserDetail,
  type AdminUserSummary,
} from "@/lib/admin";
import { formatCents, formatDate, toTitleCase } from "@/lib/utils";

const searchDebounceMs = 250;
const minimumSkeletonMs = 250;

type GrantTarget = {
  id: string;
  email: string;
  displayName: string | null;
};

export default function Admin() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<AdminUserSummary[]>([]);
  const [detail, setDetail] = useState<AdminUserDetail | null>(null);
  const [searching, setSearching] = useState(false);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const [grantOpen, setGrantOpen] = useState(false);

  const showingDetail = loadingDetail || detail !== null;
  const selected = detail?.user ?? null;
  const grantTarget = selected
    ? {
        id: selected.id,
        email: selected.email,
        displayName: selected.display_name,
      }
    : null;

  useEffect(() => {
    if (showingDetail) {
      setSearching(false);
      return;
    }

    const trimmed = query.trim();
    if (trimmed.length < 2) {
      setResults([]);
      setHasSearched(false);
      setSearching(false);
      return;
    }

    const controller = new AbortController();
    setSearching(true);
    setHasSearched(true);
    const timer = window.setTimeout(() => {
      Promise.all([searchAdminUsers(trimmed), minimumSkeletonDelay()])
        .then(([next]) => {
          if (!controller.signal.aborted) setResults(next.users);
        })
        .catch((e) => {
          if (!controller.signal.aborted) {
            setError(e instanceof Error ? e.message : String(e));
          }
        })
        .finally(() => {
          if (!controller.signal.aborted) setSearching(false);
        });
    }, searchDebounceMs);

    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [query, showingDetail]);

  const selectUser = async (user: AdminUserSummary) => {
    setError(null);
    setInfo(null);
    setDetail(null);
    setLoadingDetail(true);
    try {
      const [next] = await Promise.all([
        getAdminUserDetail(user.id),
        minimumSkeletonDelay(),
      ]);
      setDetail(next);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoadingDetail(false);
    }
  };

  const refreshDetail = async () => {
    if (!detail) return;
    setDetail(await getAdminUserDetail(detail.user.id));
  };

  const backToSearch = () => {
    setDetail(null);
    setLoadingDetail(false);
    setGrantOpen(false);
  };

  const handleGranted = (target: GrantTarget, amountCents: number) => {
    setError(null);
    setInfo(`Granted ${formatCents(amountCents)} to ${displayUser(target)}.`);
    setGrantOpen(false);
    void refreshDetail().catch((e) => {
      setError(e instanceof Error ? e.message : String(e));
    });
  };

  return (
    <div className="px-6 py-6">
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          {showingDetail ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={backToSearch}
              className="-ml-3 mb-3 gap-2"
            >
              <ArrowLeft className="h-4 w-4" />
              Back to search
            </Button>
          ) : null}
          <h1 className="text-xl font-medium">Admin</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {showingDetail
              ? "Inspect a user, credits, and the agent runs they started."
              : "Search for a user, select them, then inspect usage and grant credits."}
          </p>
        </div>
        {selected ? (
          <Button
            type="button"
            onClick={() => setGrantOpen(true)}
            className="gap-2"
          >
            <Plus className="h-4 w-4" />
            Grant credits
          </Button>
        ) : null}
      </div>

      {error ? (
        <Banner variant="error" onDismiss={() => setError(null)}>
          {error}
        </Banner>
      ) : null}
      {info ? (
        <Banner variant="success" onDismiss={() => setInfo(null)}>
          {info}
        </Banner>
      ) : null}

      {loadingDetail ? (
        <DetailSkeleton />
      ) : detail ? (
        <UserDetail detail={detail} />
      ) : (
        <SearchView
          query={query}
          onQueryChange={setQuery}
          results={results}
          searching={searching}
          hasSearched={hasSearched}
          onSelectUser={selectUser}
        />
      )}

      {grantTarget ? (
        <GrantSheet
          open={grantOpen}
          onOpenChange={setGrantOpen}
          target={grantTarget}
          onGranted={handleGranted}
        />
      ) : null}
    </div>
  );
}

function SearchView({
  query,
  onQueryChange,
  results,
  searching,
  hasSearched,
  onSelectUser,
}: {
  query: string;
  onQueryChange: (query: string) => void;
  results: AdminUserSummary[];
  searching: boolean;
  hasSearched: boolean;
  onSelectUser: (user: AdminUserSummary) => void;
}) {
  return (
    <>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <Search
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Search email, display name, user ID, or auth subject"
            autoFocus
            className="pl-9"
          />
        </div>
      </div>

      {searching ? (
        <SearchSkeleton />
      ) : results.length > 0 ? (
        <div className="overflow-hidden border border-border bg-card text-card-foreground">
          <ul className="divide-y divide-border">
            {results.map((user) => (
              <li
                key={user.id}
                className="transition-colors hover:bg-muted/40"
              >
                <button
                  type="button"
                  onClick={() => void onSelectUser(user)}
                  className="flex w-full items-center gap-6 px-4 py-3 text-left"
                >
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">
                      {user.display_name ?? user.email}
                    </div>
                    <div className="mt-0.5 truncate text-xs text-muted-foreground">
                      {user.display_name !== null ? (
                        <>
                          <span>{user.email}</span>
                          <span className="px-1.5">·</span>
                          <span className="font-mono">{user.id}</span>
                        </>
                      ) : (
                        <span className="font-mono">{user.id}</span>
                      )}
                    </div>
                  </div>
                  <div className="hidden shrink-0 items-center gap-6 sm:flex">
                    <RowStat label="Runs" value={formatCount(user.run_count)} />
                    <RowStat
                      label="Spent"
                      value={formatCents(user.credit_spent_cents)}
                    />
                    <RowStat
                      label="Credits"
                      value={formatCents(user.balance_cents)}
                    />
                  </div>
                </button>
              </li>
            ))}
          </ul>
        </div>
      ) : hasSearched ? (
        <EmptyState>No matching users.</EmptyState>
      ) : (
        <EmptyState>
          Type at least 2 characters to search. Results are limited so the admin
          page never loads every user.
        </EmptyState>
      )}
    </>
  );
}

function RowStat({
  label,
  value,
  width = "w-24",
}: {
  label: string;
  value: string;
  width?: string;
}) {
  return (
    <div className={`${width} text-right`}>
      <div className="text-sm tabular-nums text-foreground">{value}</div>
      <div className="text-[10px] uppercase tracking-widest text-muted-foreground">
        {label}
      </div>
    </div>
  );
}

function EmptyState({ children }: { children: ReactNode }) {
  return (
    <div className="border border-dashed border-border px-4 py-6 text-center text-sm text-muted-foreground">
      {children}
    </div>
  );
}

function UserDetail({ detail }: { detail: AdminUserDetail }) {
  const user = detail.user;
  const headline = user.display_name ?? user.email;
  const hasDisplayName = user.display_name !== null;
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex-row items-center gap-2 space-y-0">
          <UserRound className="h-4 w-4 text-muted-foreground" />
          <CardTitle className="text-base">User information</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="text-lg font-medium">{headline}</div>
          <div className="grid gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
            {hasDisplayName ? (
              <Detail label="Email" value={user.email} />
            ) : null}
            <Detail label="User ID" value={user.id} mono />
            <Detail label="Created" value={formatDate(user.created_at)} />
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
            <Metric label="Remaining credits" value={formatCents(user.balance_cents)} />
            <Metric label="Spent" value={formatCents(user.credit_spent_cents)} />
            <Metric label="Granted" value={formatCents(user.credit_granted_cents)} />
            <Metric label="Runs" value={formatCount(user.run_count)} />
            <Metric label="Active runs" value={formatCount(user.active_run_count)} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center gap-2 space-y-0">
          <ListChecks className="h-4 w-4 text-muted-foreground" />
          <CardTitle className="flex-1 text-base">Agents ran</CardTitle>
          <span className="tabular-nums text-xs text-muted-foreground">
            {detail.runs.length}
          </span>
        </CardHeader>
        <CardContent className="p-0">
          {detail.runs.length === 0 ? (
            <div className="px-6 py-4 text-sm text-muted-foreground">
              No agent runs yet.
            </div>
          ) : (
            <ul className="divide-y divide-border border-t border-border">
              {detail.runs.map((run) => (
                <li
                  key={run.id}
                  className="flex items-center gap-6 px-4 py-3"
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium">
                        {toTitleCase(run.mode)}
                      </span>
                      <span className="shrink-0 border border-border px-1.5 py-0.5 text-[10px] uppercase tracking-widest text-muted-foreground">
                        {run.status}
                      </span>
                    </div>
                    <div className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                      {run.id}
                    </div>
                  </div>
                  <div className="hidden shrink-0 items-center gap-6 sm:flex">
                    <RowStat
                      label="Tasks"
                      value={formatCount(run.task_count)}
                    />
                    <RowStat
                      label="Charged"
                      value={formatCents(run.charged_cents)}
                    />
                    <RowStat
                      label="Created"
                      value={formatDate(run.created_at)}
                      width="w-32"
                    />
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Detail({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <div className="text-xs uppercase tracking-widest text-muted-foreground">
        {label}
      </div>
      <div
        className={
          "mt-1 break-words" + (mono ? " font-mono text-xs" : " text-sm")
        }
      >
        {value}
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border border-border p-3">
      <div className="text-xs uppercase tracking-widest text-muted-foreground">
        {label}
      </div>
      <div className="mt-2 break-words text-lg font-medium tabular-nums">
        {value}
      </div>
    </div>
  );
}

function Banner({
  variant,
  children,
  onDismiss,
}: {
  variant: "error" | "success";
  children: ReactNode;
  onDismiss: () => void;
}) {
  const tone =
    variant === "error"
      ? "border-destructive/50 bg-destructive/10"
      : "border-brand/60 bg-brand/10";
  return (
    <div
      className={`mb-4 flex items-start justify-between gap-3 border ${tone} p-3 text-sm`}
    >
      <div className="flex-1">{children}</div>
      <button
        type="button"
        onClick={onDismiss}
        className="text-xs uppercase tracking-widest text-muted-foreground hover:text-foreground"
      >
        Dismiss
      </button>
    </div>
  );
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-40" />
        </CardHeader>
        <CardContent className="space-y-6">
          <Skeleton className="h-6 w-48" />
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-20 w-full" />
            ))}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-32" />
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <Skeleton className="h-44 w-full" />
          <Skeleton className="h-44 w-full" />
          <Skeleton className="h-44 w-full" />
        </CardContent>
      </Card>
    </div>
  );
}

function SearchSkeleton() {
  return (
    <div className="overflow-hidden border border-border bg-card text-card-foreground">
      <ul className="divide-y divide-border">
        {[0, 1, 2].map((i) => (
          <li key={i} className="px-4 py-3">
            <div className="flex items-center gap-6">
              <div className="min-w-0 flex-1 space-y-2">
                <Skeleton className="h-4 w-40" />
                <Skeleton className="h-3 w-56" />
              </div>
              <div className="hidden shrink-0 items-center gap-6 sm:flex">
                <Skeleton className="h-8 w-24" />
                <Skeleton className="h-8 w-24" />
                <Skeleton className="h-8 w-24" />
              </div>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

function GrantSheet({
  open,
  onOpenChange,
  target,
  onGranted,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  target: GrantTarget;
  onGranted: (target: GrantTarget, amountCents: number) => void;
}) {
  const [amountUsd, setAmountUsd] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [sheetError, setSheetError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setAmountUsd("");
      setNote("");
      setSheetError(null);
    }
  }, [open]);

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setSheetError(null);
    try {
      const grant = await grantAdminCredits({
        user_id: target.id,
        amount_usd: amountUsd.trim(),
        note: note.trim() || null,
      });
      onGranted(target, grant.amount_cents);
    } catch (err) {
      setSheetError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="flex flex-col gap-6">
        <SheetHeader>
          <SheetTitle>Grant credits</SheetTitle>
          <SheetDescription>
            Add managed run credits to {displayUser(target)}.
          </SheetDescription>
        </SheetHeader>

        <form
          onSubmit={handleSubmit}
          className="flex flex-1 flex-col gap-5 overflow-y-auto"
        >
          <Field label="User">
            <div>
              <div>{displayUser(target)}</div>
              <div className="font-mono text-xs text-muted-foreground">
                {target.id}
              </div>
            </div>
          </Field>

          <Field label="Amount (USD)" htmlFor="grant-amount">
            <Input
              id="grant-amount"
              value={amountUsd}
              onChange={(e) => setAmountUsd(e.target.value)}
              placeholder="0.00"
              inputMode="decimal"
              required
              autoFocus
            />
          </Field>

          <Field label="Note" htmlFor="grant-note" optional>
            <Input
              id="grant-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="e.g. Support credit"
            />
          </Field>

          {sheetError ? (
            <div className="border border-destructive/50 bg-destructive/10 p-3 text-sm">
              {sheetError}
            </div>
          ) : null}

          <SheetFooter className="mt-auto pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={busy}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !amountUsd.trim()}>
              {busy ? "Granting…" : "Grant credits"}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}

function Field({
  label,
  htmlFor,
  optional,
  children,
}: {
  label: string;
  htmlFor?: string;
  optional?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <label
        htmlFor={htmlFor}
        className="text-xs uppercase tracking-widest text-muted-foreground"
      >
        {label}
        {optional ? (
          <span className="ml-1 lowercase tracking-normal text-muted-foreground/70">
            (optional)
          </span>
        ) : null}
      </label>
      {children}
    </div>
  );
}

function displayUser(user: GrantTarget): string {
  return user.displayName ?? user.email;
}

function formatCount(value: number): string {
  return value.toLocaleString();
}

function minimumSkeletonDelay(): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, minimumSkeletonMs);
  });
}

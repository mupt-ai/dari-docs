import { useEffect, useState, type FormEvent } from "react";
import { Check, ChevronDown, ChevronRight, Copy, MoreVertical, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  createToken,
  listTokens,
  revokeToken,
  type AuthTokenInfo,
} from "@/lib/tokens";
import { formatDate } from "@/lib/utils";

const scopeOptions = [
  ["managed:read", "Read account, balance, tokens, and run history."],
  ["managed:check", "Start managed docs checks."],
  ["managed:optimize", "Start managed docs optimizations."],
  ["managed:billing", "Create checkout sessions."],
  ["managed:tokens", "Manage automation tokens."],
] as const;

const defaultScopes = ["managed:read", "managed:check", "managed:optimize"];

export default function Tokens() {
  const [tokens, setTokens] = useState<AuthTokenInfo[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(defaultScopes);
  const [scopesOpen, setScopesOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [issued, setIssued] = useState<AuthTokenInfo | null>(null);
  const [copied, setCopied] = useState(false);
  const [pendingRevoke, setPendingRevoke] = useState<AuthTokenInfo | null>(null);
  const [revoking, setRevoking] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  const refresh = async () => {
    setError(null);
    try {
      setTokens(automationTokens(await listTokens()));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  const handleCreate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || scopes.length === 0) return;
    setCreating(true);
    setCreateError(null);
    try {
      const created = await createToken(trimmed, scopes);
      setIssued(created);
      setTokens((prev) => {
        const listed = tokenWithoutSecret(created);
        return prev ? [listed, ...prev] : [listed];
      });
      setName("");
      setScopes(defaultScopes);
      setScopesOpen(false);
    } catch (e) {
      setCreateError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const toggleScope = (scope: string) => {
    setScopes((prev) =>
      prev.includes(scope) ? prev.filter((item) => item !== scope) : [...prev, scope]
    );
  };

  const copyIssued = async () => {
    if (!issued?.token) return;
    try {
      await navigator.clipboard.writeText(`DARI_DOCS_TOKEN=${issued.token}`);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  const confirmRevoke = async () => {
    if (!pendingRevoke) return;
    const target = pendingRevoke;
    setRevoking(true);
    setRevokeError(null);
    try {
      await revokeToken(target.id);
      setTokens((prev) => prev?.filter((token) => token.id !== target.id) ?? prev);
      setPendingRevoke(null);
    } catch (e) {
      setRevokeError(e instanceof Error ? e.message : String(e));
    } finally {
      setRevoking(false);
    }
  };

  return (
    <div className="px-6 py-6">
      <div className="mb-6">
        <h1 className="text-xl font-medium">Tokens</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Automation tokens for CI and scripts.
        </p>
      </div>

      {error && (
        <div className="mb-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
          {error}
        </div>
      )}

      <section className="mb-6 border border-border bg-card p-6">
        <div className="mb-4">
          <div className="text-sm font-medium">Create token</div>
          <div className="mt-1 text-sm text-muted-foreground">
            The full token is shown once. Store it in your CI secret store.
          </div>
        </div>
        <form onSubmit={handleCreate} className="flex flex-col gap-4">
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Name, e.g. github-actions"
            maxLength={80}
            className="max-w-md"
            disabled={creating}
          />
          <div className="border border-border bg-background">
            <button
              type="button"
              onClick={() => setScopesOpen((value) => !value)}
              className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-accent"
            >
              <span className="flex items-center gap-2">
                {scopesOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                Token scopes
              </span>
              <span className="text-xs text-muted-foreground">
                {scopes.length} selected
              </span>
            </button>
            {scopesOpen && (
              <div className="grid gap-2 border-t border-border p-3 md:grid-cols-2">
                {scopeOptions.map(([scope, description]) => (
                  <label
                    key={scope}
                    className="flex cursor-pointer gap-3 border border-border bg-card p-3 text-sm hover:bg-accent"
                  >
                    <input
                      type="checkbox"
                      checked={scopes.includes(scope)}
                      onChange={() => toggleScope(scope)}
                      className="mt-1"
                    />
                    <span>
                      <span className="block font-mono text-xs">{scope}</span>
                      <span className="mt-1 block text-xs text-muted-foreground">{description}</span>
                    </span>
                  </label>
                ))}
              </div>
            )}
          </div>
          {createError && (
            <div className="border border-destructive/50 bg-destructive/10 p-3 text-xs text-destructive-foreground">
              {createError}
            </div>
          )}
          <div>
            <Button type="submit" disabled={creating || !name.trim() || scopes.length === 0}>
              {creating ? "Creating..." : "Create token"}
            </Button>
          </div>
        </form>
      </section>

      {issued?.token && (
        <section className="mb-6 border border-brand/60 bg-card p-6">
          <div className="mb-4 flex items-start justify-between gap-3">
            <div>
              <div className="text-sm font-medium">Copy your token now</div>
              <div className="mt-1 text-sm text-muted-foreground">
                This is the only time the full value will be shown.
              </div>
            </div>
            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => setIssued(null)}
              className="inline-flex h-7 w-7 items-center justify-center text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <div className="flex items-stretch gap-2">
            <div className="min-w-0 flex-1 select-all border border-border bg-background px-3 py-2 font-mono text-xs">
              <code className="break-all">DARI_DOCS_TOKEN={issued.token}</code>
            </div>
            <Button type="button" variant="outline" size="sm" onClick={copyIssued} className="h-auto">
              {copied ? <Check className="mr-1.5 h-3 w-3 text-brand" /> : <Copy className="mr-1.5 h-3 w-3" />}
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>
        </section>
      )}

      <ConfirmDialog
        open={pendingRevoke !== null}
        onOpenChange={(open) => {
          if (!open && !revoking) {
            setPendingRevoke(null);
            setRevokeError(null);
          }
        }}
        title={pendingRevoke ? `Revoke token "${pendingRevoke.name ?? pendingRevoke.id}"?` : "Revoke token?"}
        description="Any CI job or script using this token will fail immediately. This cannot be undone."
        confirmLabel="Revoke"
        variant="destructive"
        confirming={revoking}
        onConfirm={confirmRevoke}
        error={revokeError}
      />

      <section className="border border-border">
        <div className="border-b border-border bg-muted/40 px-4 py-3 text-xs uppercase tracking-widest text-muted-foreground">
          Active automation tokens
        </div>
        {tokens === null ? (
          <div className="p-4 text-sm text-muted-foreground">loading tokens...</div>
        ) : tokens.length === 0 ? (
          <div className="p-4 text-sm text-muted-foreground">No active automation tokens.</div>
        ) : (
          <div className="divide-y divide-border">
            {tokens.map((token) => (
              <div key={token.id} className="flex items-start justify-between gap-4 p-4">
                <div className="min-w-0">
                  <div className="text-sm font-medium">{token.name || token.id}</div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {token.id} · {token.kind} · created {formatDate(token.created_at)}
                  </div>
                  <div className="mt-2 flex flex-wrap gap-1">
                    {token.scopes.map((scope) => (
                      <span key={scope} className="border border-border bg-background px-2 py-1 font-mono text-[11px] text-muted-foreground">
                        {scope}
                      </span>
                    ))}
                  </div>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" variant="ghost" size="icon" aria-label="Token actions">
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onSelect={() => setPendingRevoke(token)}>
                      Revoke
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function automationTokens(tokens: AuthTokenInfo[]): AuthTokenInfo[] {
  return tokens.filter((token) => token.kind === "automation");
}

function tokenWithoutSecret(token: AuthTokenInfo): AuthTokenInfo {
  const listed = { ...token };
  delete listed.token;
  return listed;
}

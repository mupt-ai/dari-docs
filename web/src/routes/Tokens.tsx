import { useEffect, useState, type FormEvent } from "react";
import { Check, ChevronDown, ChevronRight, Copy, MoreVertical, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
  const [copyError, setCopyError] = useState<string | null>(null);
  const [pendingRevoke, setPendingRevoke] = useState<AuthTokenInfo | null>(null);
  const [revoking, setRevoking] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    listTokens()
      .then((items) => {
        if (!cancelled) setTokens(automationTokens(items));
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      cancelled = true;
    };
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

  const handleCopy = async () => {
    if (!issued?.token) return;
    const didCopy = await copyTextToClipboard(`DARI_DOCS_TOKEN=${issued.token}`);
    if (!didCopy) {
      setCopied(false);
      setCopyError("Copy failed. Select the token and copy it manually.");
      return;
    }
    setCopyError(null);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
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

  const handleRevokeDialogOpenChange = (open: boolean) => {
    if (!open && !revoking) {
      setPendingRevoke(null);
      setRevokeError(null);
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

      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Create Token</CardTitle>
          <CardDescription>
            The full token is shown once. Store it in your CI secret store.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleCreate} className="flex flex-col gap-4">
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Label (e.g. github-actions)"
              maxLength={80}
              className="max-w-sm"
              disabled={creating}
            />
            <div className="border border-border bg-background">
              <button
                type="button"
                onClick={() => setScopesOpen((value) => !value)}
                className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-accent"
              >
                <span className="flex items-center gap-2">
                  {scopesOpen ? (
                    <ChevronDown className="h-4 w-4" />
                  ) : (
                    <ChevronRight className="h-4 w-4" />
                  )}
                  Token Scopes
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
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {description}
                        </span>
                      </span>
                    </label>
                  ))}
                </div>
              )}
            </div>
            {createError && (
              <p className="text-xs text-destructive-foreground">{createError}</p>
            )}
            <div>
              <Button
                type="submit"
                disabled={creating || !name.trim() || scopes.length === 0}
              >
                {creating ? "Creating…" : "Create Token"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {issued?.token && (
        <Card className="mb-6 border-brand/60">
          <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
            <div className="flex flex-col gap-1.5">
              <CardTitle>Copy Your Token Now</CardTitle>
              <CardDescription>
                This is the only time the full value will be shown. Store it
                somewhere safe — you won't be able to retrieve it again.
              </CardDescription>
            </div>
            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => {
                setIssued(null);
                setCopied(false);
                setCopyError(null);
              }}
              className="-mr-1 -mt-1 inline-flex h-7 w-7 shrink-0 items-center justify-center text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <X className="h-4 w-4" />
            </button>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-2">
              <div className="flex items-stretch gap-2">
                <div
                  role="textbox"
                  aria-label="New token"
                  tabIndex={0}
                  className="min-w-0 flex-1 select-all rounded-md border border-border bg-card px-3 py-2 font-mono text-xs text-foreground outline-none focus-visible:ring-1 focus-visible:ring-ring"
                >
                  <code className="break-all">
                    DARI_DOCS_TOKEN={issued.token}
                  </code>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleCopy}
                  className="h-auto shrink-0"
                >
                  {copied ? (
                    <>
                      <Check className="mr-1.5 h-3 w-3 text-brand" />
                      Copied
                    </>
                  ) : (
                    <>
                      <Copy className="mr-1.5 h-3 w-3" />
                      Copy
                    </>
                  )}
                </Button>
              </div>
              {copyError && (
                <p className="text-xs text-muted-foreground">{copyError}</p>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      <ConfirmDialog
        open={pendingRevoke !== null}
        onOpenChange={handleRevokeDialogOpenChange}
        title={
          pendingRevoke
            ? `Revoke Token "${pendingRevoke.name ?? pendingRevoke.id}"?`
            : "Revoke Token?"
        }
        description="Any CI job or script using this token will fail immediately. This cannot be undone."
        confirmLabel="Revoke"
        cancelLabel="Cancel"
        variant="destructive"
        confirming={revoking}
        onConfirm={confirmRevoke}
        error={revokeError}
      />

      {tokens === null && !error ? (
        <div className="text-sm text-muted-foreground">loading…</div>
      ) : tokens && tokens.length === 0 ? (
        <div className="text-sm text-muted-foreground">
          No automation tokens yet. Create one above to get started.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {tokens?.map((token) => (
            <Card key={token.id}>
              <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
                <div className="flex flex-col gap-1.5">
                  <CardTitle>{token.name || token.id}</CardTitle>
                  <CardDescription>
                    created {token.created_at ? formatDate(token.created_at) : "—"}
                  </CardDescription>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    aria-label="Token actions"
                    className="-mr-1 -mt-1 inline-flex h-7 w-7 shrink-0 items-center justify-center text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                  >
                    <MoreVertical className="h-4 w-4" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onSelect={() => setPendingRevoke(token)}
                      className="text-destructive-foreground focus:bg-destructive/10 focus:text-destructive-foreground"
                    >
                      Revoke
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </CardHeader>
              <CardContent className="flex flex-col gap-1 text-xs text-muted-foreground">
                {token.token_prefix && (
                  <div>
                    <span className="uppercase tracking-widest">prefix</span>{" "}
                    <code className="text-foreground">{token.token_prefix}…</code>
                  </div>
                )}
                <div>
                  <span className="uppercase tracking-widest">last used</span>{" "}
                  <span className="text-foreground">
                    {token.last_used_at ? formatDate(token.last_used_at) : "never"}
                  </span>
                </div>
                <div className="mt-2 flex flex-wrap gap-1">
                  {token.scopes.map((scope) => (
                    <span
                      key={scope}
                      className="border border-border bg-background px-2 py-1 font-mono text-[11px] text-muted-foreground"
                    >
                      {scope}
                    </span>
                  ))}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

async function copyTextToClipboard(value: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return true;
    } catch {
      // Fall back below for browsers that block the async Clipboard API.
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.top = "0";
  textarea.style.left = "0";
  textarea.style.opacity = "0";

  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();

  try {
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}

function automationTokens(tokens: AuthTokenInfo[]): AuthTokenInfo[] {
  return tokens.filter((token) => token.kind === "automation");
}

function tokenWithoutSecret(token: AuthTokenInfo): AuthTokenInfo {
  const listed = { ...token };
  delete listed.token;
  return listed;
}

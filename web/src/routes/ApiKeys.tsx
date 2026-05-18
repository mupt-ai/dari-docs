import { useEffect, useState, type FormEvent } from "react";
import { Check, Copy, MoreVertical, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  createAPIKey,
  listAPIKeys,
  revokeAPIKey,
  type APIKeyInfo,
} from "@/lib/api-keys";
import { cn, formatDate } from "@/lib/utils";

const scopeOptions = [
  ["managed:read", "Read account, balance, API keys, and run history."],
  ["managed:check", "Start managed docs checks."],
  ["managed:optimize", "Start managed docs optimizations."],
  ["managed:billing", "Create checkout sessions."],
  ["managed:tokens", "Manage API keys."],
] as const;

const defaultScopes = ["managed:read", "managed:check", "managed:optimize"];

export default function ApiKeys() {
  const [apiKeys, setAPIKeys] = useState<APIKeyInfo[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(defaultScopes);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [issued, setIssued] = useState<APIKeyInfo | null>(null);
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState<string | null>(null);
  const [pendingRevoke, setPendingRevoke] = useState<APIKeyInfo | null>(null);
  const [revoking, setRevoking] = useState(false);
  const [revokeError, setRevokeError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    listAPIKeys()
      .then((items) => {
        if (!cancelled) setAPIKeys(automationAPIKeys(items));
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
      const created = await createAPIKey(trimmed, scopes);
      setIssued(created);
      setAPIKeys((prev) => {
        const listed = apiKeyWithoutSecret(created);
        return prev ? [listed, ...prev] : [listed];
      });
      setName("");
      setScopes(defaultScopes);
      setShowCreateForm(false);
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
    const didCopy = await copyTextToClipboard(`DARI_DOCS_API_KEY=${issued.token}`);
    if (!didCopy) {
      setCopied(false);
      setCopyError("Copy failed. Select the API key and copy it manually.");
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
      await revokeAPIKey(target.id);
      setAPIKeys((prev) => prev?.filter((apiKey) => apiKey.id !== target.id) ?? prev);
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
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-medium">API keys</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Programmatic access for CI and scripts.
          </p>
        </div>
        {!showCreateForm ? (
          <Button type="button" onClick={() => setShowCreateForm(true)}>
            Create Key
          </Button>
        ) : null}
      </div>

      {error ? (
        <StatusBanner variant="error" message={error} onDismiss={() => setError(null)} />
      ) : null}

      {showCreateForm ? (
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Create API key</CardTitle>
            <CardDescription>
              Issue a new key for CI, scripts, or other programmatic clients.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form id="api-key-create-form" onSubmit={handleCreate} className="space-y-4">
              <div className="flex items-start gap-2">
                <Input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Label (e.g. GitHub Actions)"
                  maxLength={80}
                  className="max-w-sm"
                  disabled={creating}
                  autoFocus
                />
                <Button
                  type="submit"
                  disabled={creating || !name.trim() || scopes.length === 0}
                >
                  {creating ? "Creating…" : "Create Key"}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => {
                    setShowCreateForm(false);
                    setCreateError(null);
                  }}
                  disabled={creating}
                  className="text-muted-foreground hover:text-foreground"
                >
                  Cancel
                </Button>
              </div>

              <div>
                <div className="mb-2 flex items-baseline justify-between gap-3">
                  <div className="text-xs uppercase tracking-widest text-muted-foreground">
                    Scopes
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {scopes.length} selected
                  </div>
                </div>
                <div className="grid gap-2 md:grid-cols-2">
                  {scopeOptions.map(([scope, description]) => {
                    const checked = scopes.includes(scope);
                    return (
                      <label
                        key={scope}
                        className={cn(
                          "flex cursor-pointer gap-3 border border-border bg-background p-3 text-sm transition-colors hover:bg-accent/50",
                          checked && "bg-muted/40"
                        )}
                      >
                        <Checkbox
                          checked={checked}
                          onChange={() => toggleScope(scope)}
                          className="mt-0.5"
                          disabled={creating}
                        />
                        <span className="min-w-0">
                          <span className="block font-mono text-xs text-foreground">
                            {scope}
                          </span>
                          <span className="mt-1 block text-xs text-muted-foreground">
                            {description}
                          </span>
                        </span>
                      </label>
                    );
                  })}
                </div>
              </div>

              {createError ? (
                <p className="text-xs text-destructive-foreground">{createError}</p>
              ) : null}
            </form>
          </CardContent>
        </Card>
      ) : null}

      {issued?.token ? (
        <Card className="mb-6 border-brand/60">
          <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
            <div className="flex flex-col gap-1.5">
              <CardTitle>Copy your key now</CardTitle>
              <CardDescription>
                This is the only time the full key will be shown. Store it somewhere safe.
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
                  aria-label="New API key"
                  tabIndex={0}
                  className="min-w-0 flex-1 select-all border border-border bg-card px-3 py-2 font-mono text-xs text-foreground outline-none focus-visible:ring-1 focus-visible:ring-ring"
                >
                  <code className="break-all">DARI_DOCS_API_KEY={issued.token}</code>
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
              {copyError ? (
                <p className="text-xs text-muted-foreground">{copyError}</p>
              ) : null}
            </div>
          </CardContent>
        </Card>
      ) : null}

      {apiKeys === null && !error ? (
        <div className="text-sm text-muted-foreground">loading…</div>
      ) : apiKeys && apiKeys.length === 0 ? (
        <div className="text-sm text-muted-foreground">
          No API keys yet. Create one to get started.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {apiKeys?.map((apiKey) => (
            <Card key={apiKey.id}>
              <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
                <div className="flex min-w-0 flex-col gap-1.5">
                  <CardTitle className="truncate">{apiKey.name || apiKey.id}</CardTitle>
                  <CardDescription>
                    created {apiKey.created_at ? formatDate(apiKey.created_at) : "—"}
                  </CardDescription>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    aria-label="API key actions"
                    className="-mr-1 -mt-1 inline-flex h-7 w-7 shrink-0 items-center justify-center text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                  >
                    <MoreVertical className="h-4 w-4" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      onSelect={() => setPendingRevoke(apiKey)}
                      className="text-destructive-foreground focus:bg-destructive/10 focus:text-destructive-foreground"
                    >
                      Revoke
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </CardHeader>
              <CardContent className="flex flex-col gap-1 text-xs text-muted-foreground">
                {apiKey.token_prefix ? (
                  <div>
                    <span className="uppercase tracking-widest">prefix</span>{" "}
                    <code className="text-foreground">{apiKey.token_prefix}…</code>
                  </div>
                ) : null}
                <div>
                  <span className="uppercase tracking-widest">last used</span>{" "}
                  <span className="text-foreground">
                    {apiKey.last_used_at ? formatDate(apiKey.last_used_at) : "never"}
                  </span>
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {apiKey.scopes.map((scope) => (
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

      <ConfirmDialog
        open={pendingRevoke !== null}
        onOpenChange={handleRevokeDialogOpenChange}
        title={
          pendingRevoke
            ? `Revoke API key "${pendingRevoke.name ?? pendingRevoke.id}"?`
            : "Revoke API key?"
        }
        description="Any CI job or script using this API key will fail immediately. This cannot be undone."
        confirmLabel="Revoke"
        cancelLabel="Cancel"
        variant="destructive"
        confirming={revoking}
        onConfirm={confirmRevoke}
        error={revokeError}
      />
    </div>
  );
}

function StatusBanner({
  variant,
  message,
  onDismiss,
}: {
  variant: "error" | "info";
  message: string;
  onDismiss: () => void;
}) {
  return (
    <div
      className={cn(
        "mb-6 flex items-center gap-3 border p-3 text-sm",
        variant === "error"
          ? "border-destructive/50 bg-destructive/10"
          : "border-border bg-muted/40"
      )}
    >
      <span className="min-w-0 flex-1 break-words">{message}</span>
      <button
        type="button"
        onClick={onDismiss}
        aria-label="Dismiss"
        className="inline-flex h-5 w-5 shrink-0 items-center justify-center text-muted-foreground hover:text-foreground"
      >
        <X className="h-3.5 w-3.5" />
      </button>
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

function automationAPIKeys(apiKeys: APIKeyInfo[]): APIKeyInfo[] {
  return apiKeys.filter((apiKey) => apiKey.kind === "automation");
}

function apiKeyWithoutSecret(apiKey: APIKeyInfo): APIKeyInfo {
  const listed = { ...apiKey };
  delete listed.token;
  return listed;
}

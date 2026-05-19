import { useEffect, useState, type FormEvent } from "react";
import { Check, Copy, MoreVertical, Plus, X } from "lucide-react";
import * as DialogPrimitive from "@radix-ui/react-dialog";

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

  const handleCancelCreate = () => {
    if (creating) return;
    setShowCreateForm(false);
    setName("");
    setScopes(defaultScopes);
    setCreateError(null);
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
          <h1 className="text-xl font-medium">API Keys</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Programmatic access for CI and scripts.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setShowCreateForm(true)}
          className="shrink-0 bg-white text-black hover:bg-white/90"
        >
          <Plus className="mr-1 h-4 w-4" />
          Create Key
        </Button>
      </div>

      {error ? (
        <StatusBanner variant="error" message={error} onDismiss={() => setError(null)} />
      ) : null}

      <CreateKeyDialog
        open={showCreateForm}
        onOpenChange={(open) => {
          if (open) {
            setShowCreateForm(true);
          } else {
            handleCancelCreate();
          }
        }}
        name={name}
        onNameChange={setName}
        scopes={scopes}
        onToggleScope={toggleScope}
        creating={creating}
        createError={createError}
        onSubmit={handleCreate}
      />

      {issued?.token ? (
        <Card className="mb-6 border-brand/60">
          <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
            <div className="flex flex-col gap-1.5">
              <CardTitle>Copy Your Key Now</CardTitle>
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
                  aria-label="New API Key"
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
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : apiKeys && apiKeys.length === 0 ? (
        <div className="text-sm text-muted-foreground">
          No API Keys Yet. Use Create Key to get started.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {apiKeys?.map((apiKey) => (
            <Card key={apiKey.id}>
              <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
                <div className="flex min-w-0 flex-col gap-1.5">
                  <CardTitle className="truncate">{apiKey.name || apiKey.id}</CardTitle>
                  <CardDescription>
                    Created {apiKey.created_at ? formatDate(apiKey.created_at) : "—"}
                  </CardDescription>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    aria-label="API Key Actions"
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
                    <span className="uppercase tracking-widest">Prefix</span>{" "}
                    <code className="text-foreground">{apiKey.token_prefix}…</code>
                  </div>
                ) : null}
                <div>
                  <span className="uppercase tracking-widest">Last Used</span>{" "}
                  <span className="text-foreground">
                    {apiKey.last_used_at ? formatDate(apiKey.last_used_at) : "Never"}
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
            ? `Revoke API Key "${pendingRevoke.name ?? pendingRevoke.id}"?`
            : "Revoke API Key?"
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

function CreateKeyDialog({
  open,
  onOpenChange,
  name,
  onNameChange,
  scopes,
  onToggleScope,
  creating,
  createError,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  name: string;
  onNameChange: (value: string) => void;
  scopes: string[];
  onToggleScope: (scope: string) => void;
  creating: boolean;
  createError: string | null;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/70" />
        <DialogPrimitive.Content className="fixed left-1/2 top-1/2 z-50 max-h-[calc(100vh-2rem)] w-[calc(100vw-2rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto border border-border bg-card p-6 shadow-lg focus:outline-none">
          <div className="mb-5 flex items-start justify-between gap-4">
            <div>
              <DialogPrimitive.Title className="text-base font-medium text-foreground">
                Create API Key
              </DialogPrimitive.Title>
              <DialogPrimitive.Description className="mt-2 text-sm text-muted-foreground">
                Issue a new key for CI, scripts, or other programmatic clients.
              </DialogPrimitive.Description>
            </div>
            <button
              type="button"
              aria-label="Close"
              onClick={() => onOpenChange(false)}
              disabled={creating}
              className="-mr-2 -mt-2 inline-flex h-8 w-8 shrink-0 items-center justify-center text-muted-foreground hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <form id="api-key-create-form" onSubmit={onSubmit} className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="api-key-name" className="text-sm font-medium">
                Label
              </label>
              <Input
                id="api-key-name"
                value={name}
                onChange={(event) => onNameChange(event.target.value)}
                placeholder="Label (e.g. GitHub Actions)"
                maxLength={80}
                disabled={creating}
                autoFocus
              />
            </div>

            <div>
              <div className="mb-2 flex items-baseline justify-between gap-3">
                <div className="text-xs uppercase tracking-widest text-muted-foreground">
                  Scopes
                </div>
                <div className="text-xs text-muted-foreground">
                  {scopes.length} Selected
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
                        checked && "bg-muted/40",
                        creating && "cursor-not-allowed opacity-60"
                      )}
                    >
                      <Checkbox
                        checked={checked}
                        onChange={() => onToggleScope(scope)}
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

            <p className="text-xs leading-5 text-muted-foreground">
              The full key will only be shown once after creation.
            </p>

            <div className="flex justify-end gap-2 pt-1">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => onOpenChange(false)}
                disabled={creating}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                size="sm"
                disabled={creating || !name.trim() || scopes.length === 0}
              >
                {creating ? "Creating…" : "Create Key"}
              </Button>
            </div>
          </form>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
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

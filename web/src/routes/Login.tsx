import { useState } from "react";

import { Button } from "@/components/ui/button";
import { signInWithGoogle } from "@/lib/supabase";

export default function Login() {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleSignIn = async () => {
    setError(null);
    setBusy(true);
    try {
      const redirectTo = `${window.location.origin}/auth/callback`;
      await signInWithGoogle(redirectTo);
    } catch (e) {
      setBusy(false);
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="w-full max-w-md border border-border bg-card p-10">
        <div className="mb-8">
          <div className="flex items-center gap-2 text-sm uppercase tracking-widest text-muted-foreground">
            <span className="inline-flex h-6 w-6 items-center justify-center rounded-[50%] border border-white/40">
              <img src="/dari-logo.svg" alt="" className="h-4 w-4" />
            </span>
            Dari Docs
          </div>
          <h1 className="mt-2 text-2xl font-medium">Sign in</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Sign in with Google to view managed docs runs, billing, and automation tokens.
          </p>
        </div>
        <Button className="w-full" onClick={handleSignIn} disabled={busy}>
          {busy ? "Redirecting..." : "Continue with Google"}
        </Button>
        {error && (
          <div className="mt-6 border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive-foreground">
            {error}
          </div>
        )}
      </div>
    </div>
  );
}

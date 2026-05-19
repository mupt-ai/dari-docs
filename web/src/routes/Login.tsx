import { type FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  getSupabaseClient,
  signInWithGoogle,
  signInWithMagicLink,
} from "@/lib/supabase";

export default function Login() {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState<"google" | "email" | null>(null);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const client = await getSupabaseClient();
        const { data } = await client.auth.getSession();
        if (cancelled || !data.session) return;
        navigate("/runs", { replace: true });
      } catch (e) {
        if (cancelled) return;
        setError(e instanceof Error ? e.message : String(e));
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [navigate]);

  const redirectTo = `${window.location.origin}/auth/callback`;

  const handleGoogleSignIn = async () => {
    setError(null);
    setMessage(null);
    setBusy("google");
    try {
      await signInWithGoogle(redirectTo);
    } catch (e) {
      setBusy(null);
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const handleMagicLink = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedEmail = email.trim();
    if (!trimmedEmail) {
      setError("Enter your email address.");
      return;
    }
    setError(null);
    setMessage(null);
    setBusy("email");
    try {
      await signInWithMagicLink(trimmedEmail, redirectTo);
      setMessage("Check your email for a Dari sign-in link.");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6 py-12">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center justify-center gap-2.5 text-sm tracking-wide">
          <span className="inline-flex h-9 w-9 items-center justify-center rounded-[50%] border border-white/40">
            <img src="/dari-logo.svg" alt="" className="h-7 w-7" />
          </span>
          <span className="text-foreground">dari-docs</span>
        </div>

        <div className="border border-border bg-card p-8">
          <div className="mb-6 space-y-1.5">
            <h1 className="text-lg font-medium tracking-tight">Sign In</h1>
            <p className="text-sm text-muted-foreground">
              Manage your Dari docs runs.
            </p>
          </div>

          <Button
            className="w-full"
            onClick={handleGoogleSignIn}
            disabled={busy !== null}
          >
            {busy === "google" ? "Redirecting…" : "Continue with Google"}
          </Button>

          <div className="my-5 flex items-center gap-3 text-xs text-muted-foreground">
            <div className="h-px flex-1 bg-border" />
            <span>Or</span>
            <div className="h-px flex-1 bg-border" />
          </div>

          <form onSubmit={handleMagicLink} className="space-y-4">
            <div className="space-y-2">
              <label
                className="block text-xs font-medium tracking-wide text-foreground"
                htmlFor="email"
              >
                Email
              </label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="you@example.com"
                disabled={busy !== null}
                autoComplete="email"
              />
            </div>
            <Button
              className="w-full"
              type="submit"
              variant="outline"
              disabled={busy !== null}
            >
              {busy === "email" ? "Sending…" : "Email Me a Magic Link"}
            </Button>
          </form>

          {message ? (
            <div className="mt-5 border border-border bg-muted/40 px-3 py-2.5 text-xs text-muted-foreground">
              {message}
            </div>
          ) : null}
          {error ? (
            <div className="mt-5 border border-destructive/30 bg-destructive/10 px-3 py-2.5 text-xs text-destructive-foreground">
              {error}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

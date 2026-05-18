import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

import { getSupabaseClient } from "@/lib/supabase";

export default function AuthCallback() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const client = await getSupabaseClient();
        const { data, error: sbError } = await client.auth.getSession();
        if (cancelled) return;
        if (sbError) {
          setError(sbError.message);
          return;
        }
        navigate(data.session ? "/runs" : "/login", { replace: true });
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [navigate]);

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center px-6">
        <div className="max-w-md border border-destructive/50 bg-destructive/10 p-6 text-sm">
          <div className="mb-2 font-medium">Sign-in failed</div>
          <div className="text-destructive-foreground">{error}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-screen items-center justify-center text-muted-foreground">
      signing in...
    </div>
  );
}

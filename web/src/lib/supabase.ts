import { createClient, type SupabaseClient } from "@supabase/supabase-js";

import { API_URL } from "@/lib/env";

type AuthConfig = {
  supabase_url: string;
  supabase_publishable_key: string;
  providers: string[];
};

let clientPromise: Promise<SupabaseClient> | null = null;

async function fetchAuthConfig(): Promise<AuthConfig> {
  const resp = await fetch(`${API_URL}/v1/auth/config`, {
    headers: { Accept: "application/json" },
  });
  if (!resp.ok) {
    throw new Error(
      `Failed to fetch auth config: ${resp.status} ${resp.statusText}`
    );
  }
  return (await resp.json()) as AuthConfig;
}

export function getSupabaseClient(): Promise<SupabaseClient> {
  if (clientPromise) return clientPromise;
  clientPromise = fetchAuthConfig().then((cfg) =>
    createClient(cfg.supabase_url, cfg.supabase_publishable_key, {
      auth: {
        flowType: "pkce",
        detectSessionInUrl: true,
        persistSession: true,
        autoRefreshToken: true,
      },
    })
  ).catch((err) => {
    clientPromise = null;
    throw err;
  });
  return clientPromise;
}

export async function signInWithGoogle(redirectTo: string) {
  const client = await getSupabaseClient();
  const { error } = await client.auth.signInWithOAuth({
    provider: "google",
    options: { redirectTo },
  });
  if (error) throw error;
}

export async function signOutSupabase() {
  const client = await getSupabaseClient();
  await client.auth.signOut();
}

import { useEffect, useState } from "react";
import type { Session, User } from "@supabase/supabase-js";

import { API_URL } from "@/lib/env";
import { getSupabaseClient, signOutSupabase } from "@/lib/supabase";

const LEGACY_MANAGED_TOKEN_KEY = "dariDocs.managedToken";
const LEGACY_MANAGED_PROFILE_KEY = "dariDocs.managedProfile";

export type ManagedProfile = {
  email: string;
  displayName: string | null;
  isAdmin: boolean;
};

export type AuthState =
  | { status: "loading" }
  | { status: "signed_out" }
  | { status: "signed_in"; session: Session; profile: ManagedProfile };

export function clearLegacyManagedSession(): void {
  try {
    window.localStorage.removeItem(LEGACY_MANAGED_TOKEN_KEY);
    window.localStorage.removeItem(LEGACY_MANAGED_PROFILE_KEY);
  } catch {
    // Ignore private-mode storage failures.
  }
}

function legacyManagedToken(): string | null {
  try {
    const token = window.localStorage.getItem(LEGACY_MANAGED_TOKEN_KEY);
    return token && token.trim() !== "" ? token : null;
  } catch {
    return null;
  }
}

async function revokeLegacyManagedToken(token: string): Promise<void> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 3000);
  try {
    await fetch(`${API_URL}/v1/auth/logout`, {
      method: "POST",
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${token}`,
      },
      signal: controller.signal,
    });
  } catch {
    // Logout should still clear local browser auth if legacy token revocation fails.
  } finally {
    window.clearTimeout(timeout);
  }
}

function profileFromUser(user: User): ManagedProfile {
  const metadata = user.user_metadata ?? {};
  return {
    email: user.email ?? "",
    displayName:
      stringMetadata(metadata.full_name) ??
      stringMetadata(metadata.name) ??
      stringMetadata(metadata.display_name),
    isAdmin: false,
  };
}

type ManagedAccountResponse = {
  email?: string;
  is_admin?: boolean;
};

async function fetchManagedAccount(
  accessToken: string
): Promise<ManagedAccountResponse | null> {
  try {
    const resp = await fetch(`${API_URL}/v1/me`, {
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${accessToken}`,
      },
    });
    if (!resp.ok) return null;
    return (await resp.json()) as ManagedAccountResponse;
  } catch {
    return null;
  }
}

function stringMetadata(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed === "" ? null : trimmed;
}

export async function logoutManaged(): Promise<void> {
  const legacyToken = legacyManagedToken();
  if (legacyToken) {
    await revokeLegacyManagedToken(legacyToken);
  }
  clearLegacyManagedSession();
  try {
    await signOutSupabase();
  } catch {
    // Local auth state will settle signed out when Supabase storage is cleared.
  }
}

export function useAuthState(): AuthState {
  const [state, setState] = useState<AuthState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    let unsubscribe: (() => void) | null = null;

    function sync(session: Session | null) {
      if (!session) {
        clearLegacyManagedSession();
        if (!cancelled) setState({ status: "signed_out" });
        return;
      }
      if (!cancelled) {
        const baseProfile = profileFromUser(session.user);
        setState({
          status: "signed_in",
          session,
          profile: baseProfile,
        });
        void fetchManagedAccount(session.access_token).then((account) => {
          if (cancelled || !account) return;
          setState((prev) => {
            if (
              prev.status !== "signed_in" ||
              prev.session.access_token !== session.access_token
            ) {
              return prev;
            }
            return {
              ...prev,
              profile: {
                ...prev.profile,
                email: account.email ?? prev.profile.email,
                isAdmin: account.is_admin === true,
              },
            };
          });
        });
      }
    }

    (async () => {
      try {
        const client = await getSupabaseClient();
        const { data } = await client.auth.getSession();
        sync(data.session);
        const sub = client.auth.onAuthStateChange((_event, session) => {
          sync(session);
        });
        unsubscribe = () => sub.data.subscription.unsubscribe();
      } catch {
        if (!cancelled) setState({ status: "signed_out" });
      }
    })();

    return () => {
      cancelled = true;
      if (unsubscribe) unsubscribe();
    };
  }, []);

  return state;
}

import { useEffect, useState } from "react";
import type { Session, User } from "@supabase/supabase-js";

import { getSupabaseClient, signOutSupabase } from "@/lib/supabase";

const LEGACY_MANAGED_TOKEN_KEY = "dariDocs.managedToken";
const LEGACY_MANAGED_PROFILE_KEY = "dariDocs.managedProfile";

export type ManagedProfile = {
  email: string;
  displayName: string | null;
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

function profileFromUser(user: User): ManagedProfile {
  const metadata = user.user_metadata ?? {};
  return {
    email: user.email ?? "",
    displayName:
      stringMetadata(metadata.full_name) ??
      stringMetadata(metadata.name) ??
      stringMetadata(metadata.display_name),
  };
}

function stringMetadata(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed === "" ? null : trimmed;
}

export async function logoutManaged(): Promise<void> {
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
        setState({
          status: "signed_in",
          session,
          profile: profileFromUser(session.user),
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

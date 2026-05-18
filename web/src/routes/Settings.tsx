import { useOutletContext } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { logoutManaged } from "@/lib/auth";
import type { AppContext } from "@/routes/AppLayout";

export default function Settings() {
  const { profile } = useOutletContext<AppContext>();

  return (
    <div className="px-6 py-6">
      <div className="mb-6">
        <h1 className="text-xl font-medium">Settings</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Account settings for Dari Docs.
        </p>
      </div>

      <section className="max-w-xl border border-border bg-card p-6">
        <div className="text-sm font-medium">Account</div>
        <dl className="mt-4 flex flex-col gap-3 text-sm">
          <div className="flex items-center justify-between gap-4 border-t border-border pt-3">
            <dt className="text-muted-foreground">Email</dt>
            <dd className="min-w-0 truncate">{profile.email}</dd>
          </div>
        </dl>
        <Button
          type="button"
          variant="outline"
          className="mt-6"
          onClick={() => {
            void logoutManaged();
          }}
        >
          Log out
        </Button>
      </section>
    </div>
  );
}

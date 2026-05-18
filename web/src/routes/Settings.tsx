import { useOutletContext } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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

      <div className="flex flex-col gap-6">
        <Card>
          <CardHeader>
            <CardTitle>Account</CardTitle>
            <CardDescription>
              The Dari Docs account you're currently signed in to.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-xs text-muted-foreground">
            <div>
              <span className="uppercase tracking-widest">email</span>{" "}
              <span className="text-foreground">{profile.email}</span>
            </div>
            {profile.displayName && (
              <div>
                <span className="uppercase tracking-widest">name</span>{" "}
                <span className="text-foreground">{profile.displayName}</span>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Session</CardTitle>
            <CardDescription>
              Sign out of Dari Docs on this browser.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                void logoutManaged();
              }}
            >
              Log Out
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

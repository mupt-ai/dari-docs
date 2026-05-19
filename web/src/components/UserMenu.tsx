import { useEffect, useRef, useState } from "react";
import { LogOut } from "lucide-react";

import { cn } from "@/lib/utils";

type Props = {
  email: string;
  displayName: string | null;
  onSignOut: () => void;
};

export default function UserMenu({ email, displayName, onSignOut }: Props) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleOutside = (event: MouseEvent) => {
      if (
        rootRef.current &&
        !rootRef.current.contains(event.target as Node)
      ) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleOutside);
    return () => document.removeEventListener("mousedown", handleOutside);
  }, [open]);

  const initial = (displayName || email || "?").trim().charAt(0).toUpperCase();

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        className={cn(
          "flex h-9 w-9 items-center justify-center border border-border bg-card text-sm font-medium text-brand",
          "hover:bg-accent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        )}
      >
        {initial}
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 top-full z-20 mt-2 w-72 border border-border bg-card shadow-lg"
        >
          <div className="flex items-center gap-3 border-b border-border p-3">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center border border-border bg-background text-base font-medium text-brand">
              {initial}
            </div>
            <div className="min-w-0">
              <div className="truncate text-sm">
                {displayName ?? email.split("@")[0]}
              </div>
              <div className="truncate text-xs text-muted-foreground">
                {email}
              </div>
            </div>
          </div>
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              onSignOut();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <LogOut className="h-4 w-4" />
            Sign Out
          </button>
        </div>
      )}
    </div>
  );
}

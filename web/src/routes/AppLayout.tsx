import { type ComponentType } from "react";
import { Link, NavLink, Outlet, useMatch } from "react-router-dom";
import {
  CreditCard,
  KeyRound,
  ListChecks,
  Settings as SettingsIcon,
} from "lucide-react";

import UserMenu from "@/components/UserMenu";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { logoutManaged, type ManagedProfile } from "@/lib/auth";

export type AppContext = {
  profile: ManagedProfile;
};

export default function AppLayout({ profile }: { profile: ManagedProfile }) {
  return (
    <SidebarProvider className="h-svh overflow-hidden">
      <Sidebar className="z-30 border-r border-border">
        <SidebarHeader className="flex h-14 shrink-0 flex-row items-center border-b border-border px-4">
          <Link
            to="/runs"
            className="flex items-center gap-2 text-sm font-medium tracking-tight"
          >
            <span className="inline-flex h-6 w-6 items-center justify-center rounded-[50%] border border-white/40">
              <img src="/dari-logo.svg" alt="" className="h-4 w-4" />
            </span>
            Dari Docs
          </Link>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Dashboard</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarNavItem to="/runs" label="Runs" icon={ListChecks} />
                <SidebarNavItem to="/billing" label="Billing" icon={CreditCard} />
                <SidebarNavItem to="/tokens" label="Tokens" icon={KeyRound} />
                <SidebarNavItem to="/settings" label="Settings" icon={SettingsIcon} />
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          <SidebarGroup>
            <SidebarGroupLabel>Resources</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarExternalNavItem
                  href="https://github.com/mupt-ai/dari-docs"
                  label="dari-docs"
                  icon={GitHubIcon}
                />
                <SidebarExternalNavItem
                  href="https://dari.dev"
                  label="dari.dev"
                  icon={DariMarkIcon}
                />
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
      </Sidebar>
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center justify-between border-b border-border px-6">
          <SidebarTrigger className="-ml-2" />
          <UserMenu
            email={profile.email}
            displayName={profile.displayName}
            onSignOut={() => {
              void logoutManaged();
            }}
          />
        </header>
        <main className="flex-1 overflow-auto">
          <Outlet context={{ profile } satisfies AppContext} />
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}

function GitHubIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={className}
      fill="currentColor"
    >
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.11.79-.25.79-.56v-2.12c-3.2.7-3.87-1.36-3.87-1.36-.52-1.33-1.28-1.68-1.28-1.68-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.76 2.7 1.25 3.36.96.1-.75.4-1.25.73-1.54-2.55-.29-5.23-1.28-5.23-5.68 0-1.25.45-2.28 1.19-3.08-.12-.29-.52-1.46.11-3.04 0 0 .97-.31 3.17 1.18A10.9 10.9 0 0 1 12 6.09c.98 0 1.96.13 2.88.39 2.2-1.49 3.17-1.18 3.17-1.18.63 1.58.23 2.75.11 3.04.74.8 1.19 1.83 1.19 3.08 0 4.42-2.69 5.38-5.25 5.67.41.36.78 1.06.78 2.14v3.12c0 .31.21.68.8.56A11.51 11.51 0 0 0 23.5 12C23.5 5.65 18.35.5 12 .5Z" />
    </svg>
  );
}

function DariMarkIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="160 320 700 360"
      aria-hidden="true"
      className={className}
      fill="currentColor"
    >
      <path d="m 227.1744,643.26658 c 81.9542,-142.22535 126.12577,-261.01869 234.53618,-263.51921 100.27816,6.92634 172.97587,162.6855 217.37594,263.51921 H 790.02886 C 708.72492,525.47047 593.70414,341.71238 463.12269,339.90771 297.07876,334.95263 226.62841,652.33298 227.1744,643.26658 Z" />
      <path d="m 657.9333,552.76478 h 177.77926 v -65.43353 l -224.0784,18.71143 44.16137,15.72708 z" />
      <rect width="14.019739" height="89.34613" x="327.44406" y="446.84949" />
      <rect width="13.920205" height="129.37703" x="368.88443" y="403.28497" />
      <rect width="13.886437" height="144.71237" x="413.17789" y="382.23361" />
      <rect width="13.886437" height="144.71237" x="462.11267" y="378.20102" />
      <path d="m 525.7832,410.10938 c 0.14632,0.10293 0.29263,0.20634 0.43946,0.31054 v -0.31054 z m -13.44726,0.42187 v 118.44141 h 13.88672 V 423.49805 c -4.56972,-4.5372 -9.1975,-8.86768 -13.88672,-12.9668 z" />
      <path d="m 593.5625,507.76758 -329.94531,33.8418 -85.6836,11.15624 h 444.8711 C 613.24085,537.56109 603.51308,522.439 593.5625,507.76758 Z" />
    </svg>
  );
}

type IconComponent = ComponentType<{ className?: string }>;

function SidebarNavItem({
  to,
  label,
  icon: Icon,
}: {
  to: string;
  label: string;
  icon?: IconComponent;
}) {
  const match = useMatch({ path: to, end: false });
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        asChild
        isActive={Boolean(match)}
        className="data-[active=true]:text-brand"
      >
        <NavLink to={to}>
          {Icon && <Icon className="h-4 w-4" />}
          <span>{label}</span>
        </NavLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function SidebarExternalNavItem({
  href,
  label,
  icon: Icon,
}: {
  href: string;
  label: string;
  icon?: IconComponent;
}) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild>
        <a href={href} target="_blank" rel="noreferrer noopener">
          {Icon && <Icon className="h-4 w-4" />}
          <span>{label}</span>
        </a>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

import { apiFetch } from "@/lib/api";

export type AdminUserSummary = {
  id: string;
  email: string;
  display_name: string | null;
  created_at: string;
  free_credit_granted_at: string | null;
  balance_cents: number;
  credit_granted_cents: number;
  credit_spent_cents: number;
  run_count: number;
  active_run_count: number;
  token_count: number;
  max_tasks_per_run_override: number | null;
  max_active_runs_per_user_override: number | null;
  effective_max_tasks_per_run: number;
  effective_max_active_runs_per_user: number;
};

export type AdminRunSummary = {
  id: string;
  mode: string;
  source: string;
  status: string;
  tester_agent_id: string | null;
  editor_agent_id: string | null;
  task_count: number;
  reserved_cents: number;
  charged_cents: number;
  created_at: string;
  completed_at: string | null;
};

export type AdminAPITokenSummary = {
  id: string;
  name: string;
  kind: string;
  token_prefix: string;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
};

export type AdminUserSearch = {
  users: AdminUserSummary[];
};

export type AdminUserDetail = {
  user: AdminUserSummary;
  tokens: AdminAPITokenSummary[];
  runs: AdminRunSummary[];
};

export type AdminCreditGrant = {
  id: string;
  user_id: string;
  amount_cents: number;
  amount_usd: string;
  note: string | null;
  created_at: string;
  created_by_user_id: string;
};

export async function searchAdminUsers(query: string): Promise<AdminUserSearch> {
  const params = new URLSearchParams({ q: query });
  return apiFetch<AdminUserSearch>(
    `/v1/admin/users/search?${params.toString()}`
  );
}

export async function getAdminUserDetail(
  userId: string
): Promise<AdminUserDetail> {
  return apiFetch<AdminUserDetail>(
    `/v1/admin/users/${encodeURIComponent(userId)}`
  );
}

export async function grantAdminCredits(params: {
  user_id: string;
  amount_usd: string;
  note?: string | null;
}): Promise<AdminCreditGrant> {
  return apiFetch<AdminCreditGrant>("/v1/admin/credits", {
    method: "POST",
    body: params,
  });
}

export async function updateAdminUserLimits(
  userId: string,
  params: {
    max_tasks_per_run: number | null;
    max_active_runs_per_user: number | null;
  }
): Promise<AdminUserSummary> {
  return apiFetch<AdminUserSummary>(
    `/v1/admin/users/${encodeURIComponent(userId)}/limits`,
    {
      method: "PATCH",
      body: params,
    }
  );
}

import { apiFetch } from "@/lib/api";

export type AdminUserSummary = {
  id: string;
  email: string;
  display_name: string | null;
  created_at: string;
  balance_cents: number;
  credit_granted_cents: number;
  credit_spent_cents: number;
  run_count: number;
  active_run_count: number;
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

export type AdminUserSearch = {
  users: AdminUserSummary[];
};

export type AdminUserDetail = {
  user: AdminUserSummary;
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

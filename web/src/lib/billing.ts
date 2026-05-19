import { apiFetch } from "@/lib/api";

export type BalanceResponse = {
  email: string;
  balance_cents: number;
  credit_granted_cents?: number;
  credit_spent_cents?: number;
  token: {
    id: string;
    name?: string;
    kind: string;
    token_prefix?: string;
    scopes: string[];
  };
};

export type RunConfig = {
  free_credit_cents: number;
  tester_session_reserve_cents: number;
  editor_session_reserve_cents: number;
  service_fee_cents: number;
  max_tasks_per_run: number;
  max_task_bytes: number;
  max_active_runs_per_user: number;
  max_bundle_bytes: number;
  bundle_max_uncompressed_bytes: number;
  bundle_max_file_bytes: number;
};

export type CheckoutResponse = {
  checkout_session_id: string;
  checkout_url: string;
};

export type BillingConfig = {
  min_checkout_cents: number;
  default_checkout_cents: number;
  max_checkout_cents: number;
};

export async function getBalance(): Promise<BalanceResponse> {
  return apiFetch<BalanceResponse>("/v1/billing/balance");
}

export async function getBillingConfig(): Promise<BillingConfig> {
  return apiFetch<BillingConfig>("/v1/billing/config");
}

export async function getRunConfig(): Promise<RunConfig> {
  return apiFetch<RunConfig>("/v1/runs/config");
}

export async function createCheckout(amountCents: number): Promise<CheckoutResponse> {
  return apiFetch<CheckoutResponse>("/v1/billing/checkout", {
    method: "POST",
    body: { amount_cents: amountCents },
  });
}

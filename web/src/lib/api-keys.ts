import { apiFetch } from "@/lib/api";

export type APIKeyInfo = {
  id: string;
  name?: string;
  kind: string;
  token_prefix?: string;
  scopes: string[];
  token?: string;
  created_at?: string;
  last_used_at?: string | null;
  expires_at?: string | null;
  revoked_at?: string | null;
};

type APIKeyListResponse = {
  tokens: APIKeyInfo[];
};

export async function listAPIKeys(): Promise<APIKeyInfo[]> {
  const resp = await apiFetch<APIKeyListResponse>("/v1/auth/tokens");
  return resp.tokens;
}

export async function createAPIKey(
  name: string,
  scopes: string[]
): Promise<APIKeyInfo> {
  return apiFetch<APIKeyInfo>("/v1/auth/tokens", {
    method: "POST",
    body: { name, scopes },
  });
}

export async function revokeAPIKey(id: string): Promise<{ revoked: boolean }> {
  return apiFetch<{ revoked: boolean }>(`/v1/auth/tokens/${id}/revoke`, {
    method: "POST",
  });
}

import { apiFetch } from "@/lib/api";

export type AuthTokenInfo = {
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

type TokenListResponse = {
  tokens: AuthTokenInfo[];
};

export async function listTokens(): Promise<AuthTokenInfo[]> {
  const resp = await apiFetch<TokenListResponse>("/v1/auth/tokens");
  return resp.tokens;
}

export async function createToken(
  name: string,
  scopes: string[]
): Promise<AuthTokenInfo> {
  return apiFetch<AuthTokenInfo>("/v1/auth/tokens", {
    method: "POST",
    body: { name, scopes },
  });
}

export async function revokeToken(id: string): Promise<{ revoked: boolean }> {
  return apiFetch<{ revoked: boolean }>(`/v1/auth/tokens/${id}/revoke`, {
    method: "POST",
  });
}

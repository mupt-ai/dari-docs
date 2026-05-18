import { clearLegacyManagedSession } from "@/lib/auth";
import { API_URL } from "@/lib/env";
import { getSupabaseClient } from "@/lib/supabase";

export class ApiError extends Error {
  readonly status: number;
  readonly detail: string;

  constructor(status: number, detail: string) {
    super(`HTTP ${status}: ${detail}`);
    this.status = status;
    this.detail = detail;
  }
}

type RequestOptions = {
  method?: string;
  body?: unknown;
  signal?: AbortSignal;
};

async function currentAccessToken(): Promise<string | null> {
  const client = await getSupabaseClient();
  const { data } = await client.auth.getSession();
  return data.session?.access_token ?? null;
}

async function refreshAccessToken(): Promise<string | null> {
  const client = await getSupabaseClient();
  const { data, error } = await client.auth.refreshSession();
  if (error) return null;
  return data.session?.access_token ?? null;
}

export async function apiFetch<T>(
  path: string,
  opts: RequestOptions = {}
): Promise<T> {
  const resp = await apiRequest(path, opts);

  if (resp.status === 204) {
    return undefined as T;
  }
  return (await resp.json()) as T;
}

export async function apiBlob(
  path: string,
  opts: RequestOptions = {}
): Promise<Blob> {
  const resp = await apiRequest(path, opts);
  return await resp.blob();
}

async function apiRequest(
  path: string,
  opts: RequestOptions = {}
): Promise<Response> {
  const url = `${API_URL}${path}`;
  const method = opts.method ?? "GET";

  const doRequest = async (token: string | null): Promise<Response> => {
    const headers: Record<string, string> = {
      Accept: "application/json",
    };
    if (opts.body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    return fetch(url, {
      method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: opts.signal,
    });
  };

  let token = await currentAccessToken();
  let resp = await doRequest(token);

  if (resp.status === 401 && token) {
    const refreshed = await refreshAccessToken();
    if (refreshed && refreshed !== token) {
      token = refreshed;
      resp = await doRequest(token);
    }
  }

  if (resp.status === 401) {
    clearLegacyManagedSession();
  }

  if (!resp.ok) {
    let detail = resp.statusText;
    try {
      const body = (await resp.json()) as { error?: string; detail?: string };
      detail = body.error ?? body.detail ?? detail;
    } catch {
      // Keep statusText when the response is not JSON.
    }
    throw new ApiError(resp.status, detail);
  }

  return resp;
}

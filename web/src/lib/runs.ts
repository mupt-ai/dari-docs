import { apiBlob, apiFetch } from "@/lib/api";

export type RunLLM = {
  role: string;
  llm_id: string;
  count: number;
};

export type RunListItem = {
  id: string;
  mode: "check" | "optimize";
  status: "uploading" | "queued" | "starting" | "running" | "completed" | "failed";
  tasks: string[];
  task_count: number;
  created_at: string;
  completed_at?: string | null;
  reserved_cents: number;
  charged_cents: number;
  estimated: boolean;
  error?: string;
  llms: RunLLM[];
  updated_docs_available: boolean;
};

export type RunListResponse = {
  runs: RunListItem[];
  next_cursor?: string;
};

export type RunStatus = RunListItem & {
  feedback_reports?: string[];
  aggregate_feedback?: string;
};

export type RunSort = "status" | "mode" | "task" | "cost" | "llms" | "created_at" | "completed_at";
export type SortDirection = "asc" | "desc";

export async function listRuns(params: {
  sort: RunSort;
  direction: SortDirection;
  limit?: number;
  cursor?: string;
}): Promise<RunListResponse> {
  const query = new URLSearchParams({
    sort: params.sort,
    direction: params.direction,
    limit: String(params.limit ?? 50),
  });
  if (params.cursor) query.set("cursor", params.cursor);
  return apiFetch<RunListResponse>(`/v1/runs?${query.toString()}`);
}

export async function getRun(id: string): Promise<RunStatus> {
  return apiFetch<RunStatus>(`/v1/runs/${encodeURIComponent(id)}`);
}

export async function downloadUpdatedDocs(id: string): Promise<Blob> {
  return apiBlob(`/v1/runs/${encodeURIComponent(id)}/updated-docs.zip`);
}

export function isActiveRun(status: string): boolean {
  return status === "uploading" || status === "queued" || status === "starting" || status === "running";
}

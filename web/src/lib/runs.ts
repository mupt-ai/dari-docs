import { apiBlob, apiFetch } from "@/lib/api";

export type RunLLM = {
  role: string;
  llm_id: string;
  count: number;
};

export type RunSession = {
  kind: "tester" | "editor" | string;
  task_index: number;
  status: "uploading" | "queued" | "starting" | "running" | "completed" | "failed" | string;
  llm_id: string;
  created_at: string;
  completed_at?: string | null;
};

export type RunListItem = {
  id: string;
  mode: "check" | "optimize";
  source?: "cli" | "web";
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
  sessions: RunSession[];
  feedback_reports?: string[];
  aggregate_feedback?: string;
};

export type RunSort = "status" | "mode" | "task" | "cost" | "llms" | "created_at" | "completed_at";
export type SortDirection = "asc" | "desc";

export type BrowserSourceFile = {
  path: string;
  file: File;
};

export type RuntimeSecretInput = {
  name: string;
  value: string;
};

export type CreateManagedRunInput = {
  mode: "check" | "optimize";
  tasks: string[];
  files: BrowserSourceFile[];
  testerLLMIDs: string[];
  editorLLMID?: string;
  includeGlobs?: string[];
  excludeGlobs?: string[];
  liveVerify?: boolean;
  runtimeSecrets?: RuntimeSecretInput[];
};

export type CreateRunResponse = {
  run_id: string;
  status: string;
};

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

export async function createRunFromFolder(input: CreateManagedRunInput): Promise<CreateRunResponse> {
  const form = new FormData();
  form.set("mode", input.mode);
  form.set("tasks_json", JSON.stringify(input.tasks));
  form.set("feedback_llm_ids_json", JSON.stringify(input.testerLLMIDs));
  if (input.mode === "optimize" && input.editorLLMID) {
    form.set("editor_llm_id", input.editorLLMID);
  }
  if (input.includeGlobs && input.includeGlobs.length > 0) {
    form.set("bundle_include_json", JSON.stringify(input.includeGlobs));
  }
  if (input.excludeGlobs && input.excludeGlobs.length > 0) {
    form.set("bundle_exclude_json", JSON.stringify(input.excludeGlobs));
  }
  if (input.liveVerify && input.runtimeSecrets && input.runtimeSecrets.length > 0) {
    const secrets: Record<string, string> = {};
    for (const secret of input.runtimeSecrets) {
      const name = secret.name.trim();
      if (name) {
        secrets[name] = secret.value;
      }
    }
    form.set("live_verify", "true");
    form.set("runtime_secrets_json", JSON.stringify(secrets));
  }
  form.set(
    "source_files_json",
    JSON.stringify({
      files: input.files.map((item) => ({ path: item.path })),
    })
  );
  for (const item of input.files) {
    form.append("source_file", item.file, item.file.name);
  }
  return apiFetch<CreateRunResponse>("/v1/runs", {
    method: "POST",
    body: form,
  });
}

export async function downloadUpdatedDocs(id: string): Promise<Blob> {
  return apiBlob(`/v1/runs/${encodeURIComponent(id)}/updated-docs.zip`);
}

export function isActiveRun(status: string): boolean {
  return status === "uploading" || status === "queued" || status === "starting" || status === "running";
}

export function formatLLMID(llmID?: string | null): string {
  const normalized = llmID?.trim();
  if (!normalized || normalized === "default") return "-";
  return normalized;
}

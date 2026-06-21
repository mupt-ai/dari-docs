import type { FlueContext, FlueHarness, WorkflowRouteHandler } from '@flue/runtime';
import * as v from 'valibot';
import editorAgent from '../../agents/docs-editor-agent';

export const route: WorkflowRouteHandler = async (_c, next) => next();

type DocFile = { path: string; content: string };
type EditPayload = {
  files?: DocFile[];
  feedback?: string;
  liveVerify?: boolean;
  runtimeSecrets?: Record<string, string>;
};

const editedDocsResult = v.object({
  changelog: v.string(),
  files: v.array(v.object({
    path: v.string(),
    content: v.string(),
  })),
});

export async function run({ init, payload }: FlueContext<EditPayload, Record<string, string | undefined>>) {
  applyRuntimeSecrets(payload?.runtimeSecrets);
  const feedback = requireString(payload?.feedback, 'feedback');
  const files = payload?.files ?? [];
  const harness = await init(editorAgent);
  await seedDocs(harness, files);
  await harness.shell('mkdir -p attempt updated-docs');

  const session = await harness.session();
  const result = await session.prompt(editorPrompt(feedback, payload), { result: editedDocsResult });
  if (result.data.files.length > 0 || files.length === 0) {
    return result.data;
  }

  const retry = await session.prompt(
    'You returned zero files. For optimize, provide at least one complete repo-relative file to create or overwrite. If source truth is missing, write the best documentation structure you can with explicit TODO(owner) placeholders instead of returning only a changelog.',
    { result: editedDocsResult },
  );
  return retry.data;
}

async function seedDocs(harness: FlueHarness, files: DocFile[]) {
  await harness.shell('rm -rf input-docs updated-docs && mkdir -p input-docs/files updated-docs');
  for (const file of files) {
    const rel = safeRelativePath(file.path);
    const fullPath = `input-docs/files/${rel}`;
    await harness.fs.mkdir(dirname(fullPath), { recursive: true });
    await harness.fs.writeFile(fullPath, file.content);
  }
}

function editorPrompt(feedback: string, payload: EditPayload | undefined): string {
  const live = payload?.liveVerify
    ? `Live verification is enabled. Runtime secrets, if provided, are available as environment variables. Secret names: ${Object.keys(payload.runtimeSecrets ?? {}).sort().join(', ') || 'none'}. Never print secret values.`
    : 'Live verification is disabled unless the docs provide a safe no-credential smoke test.';
  return `You are a documentation editor. Original docs are available under input-docs/files/. Tester feedback is below.\n\n${feedback}\n\nApply documentation improvements that address concrete blockers and confusing spots. Do not invent product facts. If source truth is missing, leave a clear TODO(owner) note. ${live}\n\nReturn structured data with:\n- changelog: a concise summary of changed files and unresolved items\n- files: every proposed file to write, as repo-relative path plus complete file content\n\nFor optimize, returning only a changelog is not useful. If the feedback identifies blockers in an existing docs file, include that updated file in files with the complete new content. File paths must be repo-relative (for example README.md), not workspace paths such as input-docs/files/README.md. Only include files that should be created or overwritten by the user. Do not include secrets.`;
}

function applyRuntimeSecrets(secrets: Record<string, string> | undefined) {
  const names = Object.keys(secrets ?? {}).filter(Boolean).sort();
  process.env.DARI_DOCS_RUNTIME_SECRET_NAMES = names.join(',');
  for (const name of names) {
    process.env[name] = secrets?.[name] ?? '';
  }
}

function requireString(value: unknown, name: string): string {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`payload.${name} must be a non-empty string`);
  }
  return value;
}

function safeRelativePath(value: string): string {
  const cleaned = value.replaceAll('\\\\', '/').replace(/^\/+/, '');
  if (!cleaned || cleaned === '.' || cleaned.includes('\0') || cleaned.split('/').includes('..')) {
    throw new Error(`unsafe docs path: ${value}`);
  }
  return cleaned;
}

function dirname(value: string): string {
  const idx = value.lastIndexOf('/');
  return idx <= 0 ? '.' : value.slice(0, idx);
}

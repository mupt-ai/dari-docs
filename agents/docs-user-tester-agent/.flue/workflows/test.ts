import type { FlueContext, FlueHarness, WorkflowRouteHandler } from '@flue/runtime';
import * as v from 'valibot';
import testerAgent from '../../agents/docs-user-tester-agent';

export const route: WorkflowRouteHandler = async (_c, next) => next();

type DocFile = { path: string; content: string };
type TestPayload = {
  task?: string;
  files?: DocFile[];
  publicDocUrls?: string[];
  liveVerify?: boolean;
  runtimeSecrets?: Record<string, string>;
};

const feedbackResult = v.object({
  feedback: v.string(),
});

export async function run({ init, payload }: FlueContext<TestPayload, Record<string, string | undefined>>) {
  applyRuntimeSecrets(payload?.runtimeSecrets);
  const task = requireString(payload?.task, 'task');
  const harness = await init(testerAgent);
  await seedDocs(harness, payload?.files ?? []);
  await harness.shell('mkdir -p attempt');

  const session = await harness.session();
  const result = await session.prompt(testerPrompt(task, payload), { result: feedbackResult });
  return result.data;
}

async function seedDocs(harness: FlueHarness, files: DocFile[]) {
  await harness.shell('rm -rf input-docs && mkdir -p input-docs/files');
  for (const file of files) {
    const rel = safeRelativePath(file.path);
    const fullPath = `input-docs/files/${rel}`;
    await harness.fs.mkdir(dirname(fullPath), { recursive: true });
    await harness.fs.writeFile(fullPath, file.content);
  }
}

function testerPrompt(task: string, payload: TestPayload | undefined): string {
  const urls = (payload?.publicDocUrls ?? []).filter(Boolean);
  const source = urls.length
    ? `Use these public docs URLs as source material:\n${urls.map((url) => `- ${url}`).join('\n')}\n\nLocal docs may also be available under input-docs/files/.`
    : 'Use the local docs under input-docs/files/ as source material.';
  const live = payload?.liveVerify
    ? `Live verification is enabled. Runtime secrets, if provided, are available as environment variables. Secret names: ${Object.keys(payload.runtimeSecrets ?? {}).sort().join(', ') || 'none'}. Never print secret values.`
    : 'Live verification is disabled unless the docs provide a safe no-credential smoke test.';
  return `You are a developer trying to complete this task using supplied docs:\n\n${task}\n\n${source}\n\nActually try the task in attempt/ in the current workspace. ${live}\n\nReturn concise feedback as structured data. Mention what you tried, whether it worked, where you got stuck, and the smallest docs changes that would have helped. Do not score the docs.`;
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

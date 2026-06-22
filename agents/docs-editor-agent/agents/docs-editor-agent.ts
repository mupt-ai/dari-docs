import { mkdir } from 'node:fs/promises';
import path from 'node:path';
import { configureProvider, createAgent } from '@flue/runtime';
import { local } from '@flue/runtime/node';
import systemPrompt from '../prompts/system.md?raw';
import documentationEditor from '../skills/documentation-editor/SKILL.md' with { type: 'skill' };

const DEFAULT_MODEL = 'anthropic/claude-sonnet-4-6';
const WORKSPACE_ROOT = '/tmp/dari-docs-workspaces';
const PROVIDER_DEFAULT_SECRET_NAMES: Record<string, string> = {
  anthropic: 'ANTHROPIC_API_KEY',
  openai: 'OPENAI_API_KEY',
  openrouter: 'OPENROUTER_API_KEY',
};

type RuntimeEnv = Record<string, string | undefined>;
type AgentPayload = { model?: string };

export default createAgent<AgentPayload, RuntimeEnv>(async ({ id, env, payload }) => {
  configureProviderSecrets(env);
  const workspace = path.join(WORKSPACE_ROOT, safePathPart(id));
  await mkdir(workspace, { recursive: true });

  return {
    model: configuredModel(env, payload),
    instructions: systemPrompt,
    skills: [documentationEditor],
    cwd: workspace,
    sandbox: local({ cwd: workspace, env: runtimeSecretEnv(env) }),
  };
});

function configuredModel(env: RuntimeEnv | undefined, payload: AgentPayload | undefined): string {
  const requestedModel = normalizeModel(payload?.model);
  if (requestedModel) return requestedModel;
  const envModel = normalizeModel(envValue(env, 'DARI_DOCS_DEFAULT_MODEL'));
  return envModel || DEFAULT_MODEL;
}

function configureProviderSecrets(env: RuntimeEnv | undefined) {
  for (const [provider, secretName] of Object.entries(PROVIDER_DEFAULT_SECRET_NAMES)) {
    if (!secretName) continue;
    const apiKey = envValue(env, secretName);
    if (!apiKey) continue;
    configureProvider(provider, { apiKey });
  }
}

function normalizeModel(value: string | undefined): string {
  const model = value?.trim() ?? '';
  if (!model) return '';
  if (model.includes('/')) return model;
  if (model.startsWith('claude-')) return `anthropic/${model}`;
  if (model.startsWith('gpt-') || /^o\d/.test(model)) return `openai/${model}`;
  return model;
}

function runtimeSecretEnv(env: RuntimeEnv | undefined): Record<string, string> {
  const names = envValue(env, 'DARI_DOCS_RUNTIME_SECRET_NAMES')
    .split(',')
    .map((name) => name.trim())
    .filter(Boolean);
  const out: Record<string, string> = {};
  for (const name of names) {
    const value = envValue(env, name);
    if (value) out[name] = value;
  }
  return out;
}

function envValue(env: RuntimeEnv | undefined, name: string): string {
  return env?.[name] ?? process.env[name] ?? '';
}

function safePathPart(value: string): string {
  const cleaned = value.replace(/[^a-zA-Z0-9._-]/g, '-').replace(/^-+|-+$/g, '');
  return cleaned || 'session';
}

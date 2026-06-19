import { mkdir } from 'node:fs/promises';
import path from 'node:path';
import { configureProvider, createAgent } from '@flue/runtime';
import { local } from '@flue/runtime/node';
import systemPrompt from '../prompts/system.md?raw';
import documentationEditor from '../skills/documentation-editor/SKILL.md' with { type: 'skill' };

const DEFAULT_MODEL = 'anthropic/claude-sonnet-4-6';
const WORKSPACE_ROOT = '/tmp/dari-docs-workspaces';
const PROVIDER_SECRET_NAME_ENVS: Record<string, string> = {
  anthropic: 'DARI_DOCS_ANTHROPIC_API_KEY_SECRET_NAME',
  openai: 'DARI_DOCS_OPENAI_API_KEY_SECRET_NAME',
  openrouter: 'DARI_DOCS_OPENROUTER_API_KEY_SECRET_NAME',
};

type RuntimeEnv = Record<string, string | undefined>;

export default createAgent<unknown, RuntimeEnv>(async ({ id, env }) => {
  configureProviderSecrets(env);
  const workspace = path.join(WORKSPACE_ROOT, safePathPart(id));
  await mkdir(workspace, { recursive: true });

  return {
    model: configuredModel(env),
    instructions: systemPrompt,
    skills: [documentationEditor],
    cwd: workspace,
    sandbox: local({ cwd: workspace }),
  };
});

function configuredModel(env: RuntimeEnv | undefined): string {
  const model = envValue(env, 'DARI_DOCS_DEFAULT_MODEL').trim();
  return model || DEFAULT_MODEL;
}

function configureProviderSecrets(env: RuntimeEnv | undefined) {
  for (const [provider, envName] of Object.entries(PROVIDER_SECRET_NAME_ENVS)) {
    const secretName = envValue(env, envName).trim();
    if (!secretName) continue;
    const apiKey = envValue(env, secretName);
    if (!apiKey) continue;
    configureProvider(provider, { apiKey });
  }
}

function envValue(env: RuntimeEnv | undefined, name: string): string {
  return env?.[name] ?? process.env[name] ?? '';
}

function safePathPart(value: string): string {
  const cleaned = value.replace(/[^a-zA-Z0-9._-]/g, '-').replace(/^-+|-+$/g, '');
  return cleaned || 'session';
}

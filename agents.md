# Bundled Flue Apps

`dari-docs` ships two Flue app templates under `agents/`:

```text
agents/docs-user-tester-agent/
agents/docs-editor-agent/
```

`dari-docs init` extracts them into the user's repo:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

They are ordinary Flue projects. Each app has `package.json`, `flue.config.ts`, a TypeScript agent entrypoint under `agents/`, prompts, skills, and one HTTP workflow under `.flue/workflows/`.

- `docs-user-tester-agent` exposes `POST /workflows/test?wait=result`.
- `docs-editor-agent` exposes `POST /workflows/edit?wait=result`.

Users run or deploy these apps with Bun, then pass the base URLs to `dari-docs check` and `dari-docs optimize` with `--tester-url` and `--editor-url`.

## Model Configuration

The templates put the default model in `agents/<name>.ts` and default to `anthropic/claude-sonnet-4-6`. Set `ANTHROPIC_API_KEY` in the app deployment environment, or edit the model/provider code and set the provider key required by that provider.

`dari-docs` can request a per-run model in the workflow payload with `--llm`, `--feedback-llm`, and `--editor-llm`. A model string can use the provider/model format expected by Flue, such as `anthropic/claude-sonnet-4-6`; the bundled agents also normalize common short IDs such as `claude-sonnet-4-6` and `gpt-5.5`.

## Runtime Product/API Secrets

Runtime product/API keys are separate from model provider credentials. `dari-docs --live-verify --secret-env NAME` reads local environment variables and sends them in the workflow payload for that run only. The bundled workflows expose those values to the sandbox environment and instruct agents not to print secret values.

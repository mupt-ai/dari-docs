# Bundled Agents

`dari-docs` ships two self-managed Flue agent templates plus a Modal deploy file under `agents/`:

```text
agents/modal_app.py
agents/docs-user-tester-agent/
agents/docs-editor-agent/
```

`dari-docs init` extracts them into the user's repo:

```text
.dari-docs/agents/modal_app.py
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

They are ordinary Flue projects with visible source files. Each agent has `package.json`, `bun.lock`, `flue.config.ts`, root `app.ts`, a TypeScript agent entrypoint under `agents/`, one workflow under `workflows/`, prompts, and skills. There is no hidden `.flue/` directory in the bundled templates or initialized output.

- `docs-user-tester-agent` exposes `POST /workflows/test?wait=result`.
- `docs-editor-agent` exposes `POST /workflows/edit?wait=result`.

The intended deployment is Modal:

```bash
uvx modal deploy .dari-docs/agents/modal_app.py
```

`modal_app.py` deploys the tester and editor as separate Modal gateway endpoints. The gateway functions do not run the agents directly; each workflow request creates a fresh `modal.Sandbox`, starts the matching Flue server inside that sandbox, forwards the workflow call, and terminates the sandbox after the result. With `dari-docs check --parallel 30`, the CLI can keep many sandbox-backed tester workflows in flight and aggregate them after completion.

## Model Configuration

The templates put the default model in `agents/<name>.ts` and default to `anthropic/claude-sonnet-4-6`. Put model provider keys in the Modal secret, or edit the model/provider code and set the provider key required by that provider.

`dari-docs` can request a per-run model in the workflow payload with `--llm`, `--feedback-llm`, and `--editor-llm`. A model string can use the provider/model format expected by Flue, such as `anthropic/claude-sonnet-4-6`; the bundled agents also normalize common short IDs such as `claude-sonnet-4-6` and `gpt-5.5`.

## Runtime Product/API Secrets

Runtime product/API keys are separate from model provider credentials. `dari-docs --live-verify --secret-env NAME` reads local environment variables and sends them in the workflow payload for that run only. The bundled workflows expose those values to the sandbox environment and instruct agents not to print secret values.

## Hosted Managed Agents

The source agent directories also contain `dari.yml` manifests for the production managed offering:

```text
agents/docs-user-tester-agent/dari.yml
agents/docs-editor-agent/dari.yml
```

Those manifests are not embedded into `dari-docs init` output. They are deployed by maintainers with `dari deploy ... --agent-id "$MANAGED_*_AGENT_ID"`. They use `sandbox.provider: modal`, omit `sandbox.provider_api_key_secret`, and omit LLM `api_key_secret` fields so hosted managed runs use platform-managed Modal and model-provider credentials.

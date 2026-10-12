# Bundled Harbor Runner

`dari-docs` ships a Python runner plus a Modal deploy file under `agents/`:

```text
agents/modal_app.py
agents/harbor_runner.py
agents/requirements.txt
```

`dari-docs init` extracts them into the user's repo:

```text
.dari-docs/agents/modal_app.py
.dari-docs/agents/harbor_runner.py
.dari-docs/agents/requirements.txt
```

`harbor_runner.py` serves the tester and editor workflows on top of [Harbor](https://docs.harborframework.com). `requirements.txt` pins `harbor[modal]==0.24.0`.

- The tester service exposes `POST /workflows/test?wait=result`.
- The editor service exposes `POST /workflows/edit?wait=result`.

Both calls block until the result is ready. An asynchronous `/runs` API is planned; do not document or depend on its shape until it lands.

Run a service locally after creating a virtual environment and installing `requirements.txt`:

```bash
python harbor_runner.py --service tester --port 8000
python harbor_runner.py --service editor --port 8001
```

The intended deployment is Modal:

```bash
uvx modal deploy .dari-docs/agents/modal_app.py
```

`modal_app.py` deploys the tester and editor as separate Modal endpoints and reads model provider keys from the `dari-docs-model-providers` secret (override with `DARI_DOCS_MODAL_SECRET`). Each workflow request runs as a Harbor trial in a fresh sandbox. The tester agent writes `feedback.md`; the editor agent writes `updated-docs/files/` and `changelog.md`. The runner collects those artifacts and validates them on the trusted host before the sandbox is torn down. With `dari-docs check --parallel 30`, the CLI keeps many tester trials in flight and aggregates them after completion.

If `DARI_DOCS_RUNNER_TOKEN` is set in the runner environment, requests must send `Authorization: Bearer <token>`. The CLI reads the token from the variable named by `--runner-token-env`.

## Harness, Model, And Sandbox

The defaults are the `claude-code` harness, the `anthropic/claude-sonnet-4-6` model, and the `modal` sandbox. Per run, the CLI can select:

- `--harness` and `--editor-harness`: a Harbor agent name or a custom `module:Class` adapter.
- `--llm`, `--feedback-llm`, `--editor-llm`: the model, in Harbor's `provider/model` form.
- `--sandbox`: a Harbor environment type or a custom `module:Class` adapter.
- `--agent-kwarg key=value`, `--sandbox-kwarg key=value`: constructor options.
- `--agent-env NAME=value`: non-secret agent environment variables.

Not every harness, model, and sandbox combination works; the default combination is the tested path.

Harbor's trial lifecycle hooks are not surfaced through the blocking endpoints. Live trajectory streaming in Harbor is limited to Claude Code and Codex.

## Runtime Product/API Secrets

Runtime product/API keys are separate from model provider credentials. `dari-docs --live-verify --secret-env NAME` reads local environment variables and sends them in the workflow payload for that run only. The runner exposes those values to the agent's environment and instructs agents not to print secret values. Do not use `--agent-env` for secrets.

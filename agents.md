# Bundled Harbor Runner

`dari-docs` ships a Python runner built on [Harbor](https://docs.harborframework.com), plus its Modal deployment and the tester and editor instructions, under `agents/`:

```text
agents/modal_app.py          Modal gateways and detached controller Function
agents/harbor_runner.py      payload validation, Harbor trial setup, artifact validation, local server
agents/runner_api.py         FastAPI app: blocking workflows, async /runs, run backends
agents/requirements.txt      harbor[modal]==0.24.0, fastapi, uvicorn, httpx
agents/Dockerfile            optional controller image for non-Modal hosts
agents/docs-user-tester-agent/{prompts,skills}/
agents/docs-editor-agent/{prompts,skills}/
agents/tests/                Python tests (not embedded)
```

`agents/templates.go` embeds everything except the tests, and `dari-docs init` extracts it to `.dari-docs/agents/`. Re-running init removes only known legacy Flue runtime files and keeps custom prompts and skills. A Flue-era `.dari-docs/config.json` is rejected until the user re-runs init and redeploys.

## Services

- The tester service exposes `POST /workflows/test?wait=result`; the editor exposes `POST /workflows/edit?wait=result`. Both block and return `{"result": ...}`. The CLI uses these.
- Both services also expose `POST /runs`, `GET /runs/{id}`, `GET /runs/{id}/events?after=N` (returns `{events, next, status}`), `GET /runs/{id}/result` (returns `{result}`), `DELETE /runs/{id}` (cooperative cancel), and `GET /healthz`.
- Status goes `queued` → `running` → `completed` | `failed` | `cancelled`. Events are lifecycle markers only (controller state, Harbor trial hooks, `artifact-retry`, `collecting-result`). There is no live log or trajectory endpoint; Harbor's optional `--stream` viewer is separate.
- Limits: 10 MiB request body, 8 MiB collected output and result.

Local runs: `python harbor_runner.py --service tester|editor --port 8000` after installing `requirements.txt` in a venv. The local backend keeps state in memory, retains completed runs for one hour, and loses everything on restart.

Deployment: `uvx modal deploy .dari-docs/agents/modal_app.py`. Each run is a detached Modal Function (the controller) that creates one Harbor task sandbox. Run state is in the `dari-docs-harbor-runs` Modal Dict, which Modal expires after 30 days of inactivity. There is no durable queue and no automatic retry after a controller crash.

## Artifacts

The tester agent writes `/app/output/feedback.md`. The editor writes `/app/output/changelog.md` and complete files under `/app/output/updated-docs/files/`. Harbor collects `/app/output` before tearing down the sandbox, and the controller validates it on the trusted host: no links or special files, portable repo-relative paths, size limits, no runtime secret values. The editor returns only created or changed files. If required output is missing after the agent finishes, the runner asks it once more to finish in the same session.

## Harness, Model, And Sandbox

The defaults are `claude-code`, `anthropic/claude-sonnet-4-6`, and `modal`, overridable on the runner with `DARI_DOCS_DEFAULT_HARNESS`, `DARI_DOCS_DEFAULT_MODEL`, and `DARI_DOCS_DEFAULT_SANDBOX`. The task sandbox image defaults to `python:3.13-slim-bookworm` (`DARI_DOCS_WORKSPACE_IMAGE`).

Per run, the CLI sends an `execution` object built from `--harness`, `--editor-harness`, `--sandbox`, `--agent-kwarg`, `--agent-env`, and `--sandbox-kwarg`. Harness and sandbox accept Harbor names or custom `module:Class` adapters. Not every harness, model, and sandbox combination works; the default combination is the tested path. Short model IDs (`claude-*`, `gpt-*`, `o<N>*`) are normalized to `provider/model`.

The runner appends each workflow's `skills/*/SKILL.md` text to `prompts/system.md` rather than registering native Harbor skills, so any harness can use them.

## Credentials

- Model provider credentials go in the Modal secret (`dari-docs-model-providers`, overridable with `DARI_DOCS_MODAL_SECRET` at deploy time). The runner forwards Harbor's known provider variables, plus names listed in `DARI_DOCS_PROVIDER_ENV_NAMES`, to the agent. It never forwards `DARI_DOCS_RUNNER_TOKEN` or `MODAL_*`.
- `DARI_DOCS_RUNNER_TOKEN` in the same secret enables bearer auth on everything except `/healthz`. The CLI reads `DARI_DOCS_RUNNER_TOKEN` automatically, or another variable named by `--runner-token-env`.
- `--agent-env` is non-secret. The CLI and runner reject names containing `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `CREDENTIAL`, or `AUTH` (case-insensitive).
- `--live-verify --secret-env NAME` sends runtime product/API secrets in the payload for that run only. They can't override provider, `MODAL_*`, `DARI_DOCS_*`, `PATH`, `HOME`, or `PYTHONPATH` variables, and the agents are told not to print them.
- Tokens are never written to config or events. Failure errors are generic so Harbor exception text can't leak credentials.

## Tests

```bash
pip install -r agents/requirements.txt pytest pytest-asyncio
python -m pytest agents/tests
RUN_HARBOR_DOCKER_TESTS=1 python -m pytest agents/tests   # Docker smoke test with a fake harness; no inference key
```

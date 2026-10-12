# Runner Setup

`dari-docs` runs against tester and editor services that you deploy. The bundled services are a Python runner, `harbor_runner.py`, built on [Harbor](https://docs.harborframework.com). For each workflow request, the runner starts a Harbor trial: it creates a fresh sandbox, runs an agent harness inside it, collects the result files, and tears the sandbox down. The CLI only needs the tester base URL, and for `optimize`, the editor base URL.

The defaults are the `claude-code` harness, the `anthropic/claude-sonnet-4-6` model, and the `modal` sandbox.

## Initialize

Run init in the repository that contains your docs:

```bash
dari-docs init
```

This extracts:

```text
.dari-docs/agents/modal_app.py
.dari-docs/agents/harbor_runner.py
.dari-docs/agents/requirements.txt
```

`requirements.txt` pins `harbor[modal]==0.24.0`. `modal_app.py` deploys `harbor_runner.py` as two Modal web endpoints, one for the tester service and one for the editor service.

## Deploy To Modal

Create a Modal secret for model provider keys. Include whichever providers your harness and model choices need:

```bash
uvx modal setup
uvx modal secret create dari-docs-model-providers \
  ANTHROPIC_API_KEY=... \
  OPENAI_API_KEY=...
```

Deploy both services:

```bash
uvx modal deploy .dari-docs/agents/modal_app.py
```

Modal prints two HTTPS URLs: one for the `tester` endpoint and one for the `editor` endpoint. The CLI calls:

```text
POST <tester-url>/workflows/test?wait=result
POST <editor-url>/workflows/edit?wait=result
```

Both calls block until the Harbor trial finishes and its result has been collected. Keep `--timeout-minutes` long enough for sandbox startup plus the agent run.

If you want a different Modal secret name, set it when deploying:

```bash
DARI_DOCS_MODAL_SECRET=my-model-provider-secret \
  uvx modal deploy .dari-docs/agents/modal_app.py
```

## Protect The Endpoints

The endpoints start agents that can run shell commands. To require a bearer token, add `DARI_DOCS_RUNNER_TOKEN` to the runner's environment, for example in the Modal secret, and redeploy:

```bash
uvx modal secret create dari-docs-model-providers --force \
  ANTHROPIC_API_KEY=... \
  DARI_DOCS_RUNNER_TOKEN=...
uvx modal deploy .dari-docs/agents/modal_app.py
```

When the token is set, requests without `Authorization: Bearer <token>` are rejected. Tell the CLI which local environment variable holds the token:

```bash
export DARI_DOCS_RUNNER_TOKEN=...

dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --runner-token-env DARI_DOCS_RUNNER_TOKEN \
  --task "Install the SDK and make a first API call"
```

The CLI reads the token from that variable locally; it is not written to `.dari-docs/`.

## Run Checks

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --task "Install the SDK and make a first API call"
```

The tester agent writes `feedback.md` in its sandbox. The runner collects it and returns it as the tester response. `check` writes reports to `.dari-docs/runs/` and `.dari-docs/aggregate-feedback.md`.

## Run Optimize

```bash
dari-docs optimize . \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run \
  --task "Install the SDK and make a first API call"
```

`optimize` runs the tester workflow first, then sends the aggregate feedback and source docs to the editor workflow. The editor agent writes complete proposed files under `updated-docs/files/` and a summary in `changelog.md`. The runner collects and validates those files on the trusted host before the sandbox is torn down, and the CLI writes them to `.dari-docs/updated/`.

Use `--apply` only after you are comfortable overwriting matching files in your repo.

## Save URLs

To avoid passing URLs every time, save them in `.dari-docs/config.json`:

```bash
dari-docs init \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run
```

## Run The Runner Locally

You can run the runner on your machine instead of Modal. It needs Python 3.12 or newer. With the default `modal` sandbox, the runner still creates sandboxes in your Modal account, so run `uvx modal setup` first, or pass `--sandbox docker` to `dari-docs` to use local Docker.

```bash
cd .dari-docs/agents
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt

ANTHROPIC_API_KEY=... python harbor_runner.py --service tester --port 8000
```

Run the editor service on another port:

```bash
ANTHROPIC_API_KEY=... python harbor_runner.py --service editor --port 8001
```

Then point the CLI at them:

```bash
dari-docs optimize . \
  --tester-url http://127.0.0.1:8000 \
  --editor-url http://127.0.0.1:8001 \
  --task "Follow the quickstart"
```

## Parallel Runs

Use `--parallel` to control how many tester workflow calls the CLI keeps in flight. Each call runs in its own sandbox, and the CLI combines the finished reports into the normal aggregate feedback file. Sandbox provider quotas and model rate limits still apply.

## Progress And Live Trajectories

Harbor emits trial lifecycle events (start, environment start, agent start, agent end, end, cancel) through hooks. The blocking `?wait=result` endpoints return only the final result, so the CLI does not show these events while a run is in progress.

An asynchronous `/runs` API with progress reporting is planned. Until it is available, treat `POST /workflows/<name>?wait=result` as the only supported contract.

Harbor can stream a live agent trajectory only for some harnesses: currently Claude Code and Codex, on supported sandboxes such as Modal and Docker. Other harnesses report their trajectory only after the run finishes.

# Runner Setup

`dari-docs` runs against tester and editor services that you deploy. The bundled services are a Python runner built on [Harbor](https://docs.harborframework.com). For each workflow request, the runner's controller starts a Harbor trial: it creates a fresh task sandbox, runs an agent harness inside it, collects the result files, validates them on the controller, and lets Harbor tear the sandbox down. The CLI only needs the tester base URL, and for `optimize`, the editor base URL.

The defaults are the `claude-code` harness, the `anthropic/claude-sonnet-4-6` model, and the `modal` sandbox.

## Initialize

Run init in the repository that contains your docs:

```bash
dari-docs init
```

This extracts:

```text
.dari-docs/agents/
  modal_app.py          Modal deployment
  harbor_runner.py      Harbor trial controller and local server entrypoint
  runner_api.py         HTTP API and run state
  requirements.txt      harbor[modal]==0.24.0 plus the HTTP server
  Dockerfile            optional controller image for non-Modal hosts
  docs-user-tester-agent/
    prompts/system.md
    skills/docs-user-test/SKILL.md
  docs-editor-agent/
    prompts/system.md
    skills/documentation-editor/SKILL.md
```

The prompt and skill files are editable; see [harness, model, and sandbox selection](agent-customization.md#change-prompts-or-skills).

### Upgrading From The Flue Runtime

A `.dari-docs/` directory created for the earlier Flue runtime does not work with this runner. Run `dari-docs init` again and redeploy. Init removes only the known Flue runtime files (`package.json`, `bun.lock`, `flue.config.ts`, `app.ts`, the agent entrypoint, and the workflow file) and keeps your prompts and skills. Init also rewrites `.dari-docs/config.json`, so pass `--tester-url` and `--editor-url` again if you saved them before.

## Deploy To Modal

Generate a runner token. The Modal deployment requires it, and every request must send it as a bearer token:

```bash
export DARI_DOCS_RUNNER_TOKEN="$(python3 -c "import secrets; print(secrets.token_urlsafe(32))")"
```

Create a Modal secret with the token and the model provider keys your harness and model choices need:

```bash
uvx modal setup
uvx modal secret create dari-docs-model-providers \
  ANTHROPIC_API_KEY=... \
  OPENAI_API_KEY=... \
  DARI_DOCS_RUNNER_TOKEN="$DARI_DOCS_RUNNER_TOKEN"
```

Store the token somewhere safe, such as a password manager or CI secret. You need the same value wherever you run `dari-docs`.

Deploy:

```bash
uvx modal deploy .dari-docs/agents/modal_app.py
```

This creates the `dari-docs-agents` Modal app. Modal prints two HTTPS URLs: one for the `tester` endpoint and one for the `editor` endpoint. Each run executes in a detached Modal Function (the controller), which creates the task sandbox in the same Modal app.

If you want a different Modal secret name, set it when deploying:

```bash
DARI_DOCS_MODAL_SECRET=my-model-provider-secret \
  uvx modal deploy .dari-docs/agents/modal_app.py
```

### Runner Environment

The runner reads these optional variables from its environment (on Modal, from the secret):

| Variable | Purpose |
| --- | --- |
| `DARI_DOCS_RUNNER_TOKEN` | Bearer token required on every endpoint except `/healthz`. Required on Modal and for local runners on a non-loopback host. See [Protect The Endpoints](#protect-the-endpoints). |
| `DARI_DOCS_DEFAULT_HARNESS` | Harness when a request doesn't set one. Default `claude-code`. |
| `DARI_DOCS_DEFAULT_MODEL` | Model when a request doesn't set one. Default `anthropic/claude-sonnet-4-6`. |
| `DARI_DOCS_DEFAULT_SANDBOX` | Sandbox when a request doesn't set one. Default `modal`. |
| `DARI_DOCS_WORKSPACE_IMAGE` | Registry image for the task sandbox. Default `python:3.13-slim-bookworm`. |
| `DARI_DOCS_PROVIDER_ENV_NAMES` | Comma-separated extra variable names to forward to the agent as provider credentials. |
| `DARI_DOCS_ALLOWED_MODEL_ENDPOINTS` | Comma-separated exact URLs that requests may set as a `*_BASE_URL` with `--agent-env`. See [Model Endpoints](#model-endpoints). |
| `DARI_DOCS_CUSTOM_ADAPTERS` | Comma-separated `module:Class` harness and sandbox adapters that requests may select. Built-in Harbor names are always allowed. |
| `DARI_DOCS_ENVIRONMENT_KWARGS` | JSON object of deployment-level Harbor sandbox kwargs, such as Modal secrets or labels. |

The runner forwards known model-provider variables (the names Harbor's provider registry uses, such as `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`, plus any in `DARI_DOCS_PROVIDER_ENV_NAMES`) from its environment to the agent, because the harness needs them to call the model. It never forwards `DARI_DOCS_RUNNER_TOKEN` or `MODAL_*` variables.

Use `DARI_DOCS_WORKSPACE_IMAGE` when the task needs tools that `python:3.13-slim-bookworm` lacks, such as Node.js.

### Model Endpoints

To send model traffic to a proxy or gateway, set the provider's base URL variable, such as `ANTHROPIC_BASE_URL` or `OPENAI_BASE_URL`, in the runner environment next to its key. The runner forwards it to the agent like the key.

A request can also set a `*_BASE_URL` with `--agent-env`, but only to a URL the operator allows: an exact match for an entry in `DARI_DOCS_ALLOWED_MODEL_ENDPOINTS`, or the same value already configured on the runner. Other values are rejected. This prevents accidental or direct endpoint redirection; it is not a security boundary against a malicious task or token holder.

### Sandbox Defaults

`DARI_DOCS_ENVIRONMENT_KWARGS` sets Harbor sandbox kwargs for every run. Valid keys depend on the sandbox; for `modal`, see Harbor's `ModalEnvironment`. For example, to attach a Modal secret named `docs-sandbox-env` to every task sandbox:

```bash
DARI_DOCS_ENVIRONMENT_KWARGS='{"secrets": ["docs-sandbox-env"], "labels": {"team": "docs"}}'
```

Anything attached this way is visible to the agent inside the sandbox.

Requests can't set `secrets`, `volumes`, or `registry_secret` with `--sandbox-kwarg`; only the operator can supply those, through this variable.

## Protect The Endpoints

The endpoints start agents that can run shell commands, so the runner requires `DARI_DOCS_RUNNER_TOKEN` on Modal and whenever a local runner listens on a non-loopback host. A local runner on `127.0.0.1` may run without it. If you deployed without a token, or need to rotate it, recreate the secret and redeploy:

```bash
export DARI_DOCS_RUNNER_TOKEN="$(python3 -c "import secrets; print(secrets.token_urlsafe(32))")"
uvx modal secret create dari-docs-model-providers --force \
  ANTHROPIC_API_KEY=... \
  DARI_DOCS_RUNNER_TOKEN="$DARI_DOCS_RUNNER_TOKEN"
uvx modal deploy .dari-docs/agents/modal_app.py
```

Requests without `Authorization: Bearer <token>` get `401`. The CLI sends the token automatically when `DARI_DOCS_RUNNER_TOKEN` is set in its environment:

```bash
export DARI_DOCS_RUNNER_TOKEN=...

dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --task "Install the SDK and make a first API call"
```

To read the token from a different variable, pass `--runner-token-env NAME`. The CLI fails if that variable is empty. The token is never saved in `.dari-docs/config.json`.

## Trust Model

The runner token is a single operator-level credential. Anyone who holds it can start agents that spend your model provider keys and use the sandbox provider account, and can read the status and results of any run on the deployment. The runner doesn't separate callers from each other.

Share one deployment only among people you would trust with those provider keys. Use spend-limited provider keys: an agent has access to them and can exfiltrate them despite output scanning. Don't use it to run requests from untrusted users, and don't treat sandboxing as isolation between tenants. Give separate teams or customers separate deployments with separate tokens and secrets.

### Result Retention

Run results contain the tester feedback and, for the editor, complete proposed docs files. If your docs are sensitive, so are these results.

- On Modal, run state and results stay in the `dari-docs-harbor-runs` Modal Dict until Modal expires it after 30 days without activity.
- A local runner keeps completed runs in memory for one hour and loses them on restart.

There is no endpoint to delete a run's result yet; one is planned. Until it ships, you can remove all retained runs by deleting the Dict with `uvx modal dict delete dari-docs-harbor-runs`, which also drops in-progress run state.

## Run Checks

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --task "Install the SDK and make a first API call"
```

The tester agent writes `feedback.md` in its sandbox. The runner collects and validates it, and `check` writes reports to `.dari-docs/runs/` and `.dari-docs/aggregate-feedback.md`.

## Run Optimize

```bash
dari-docs optimize . \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run \
  --task "Install the SDK and make a first API call"
```

`optimize` runs the tester workflow first, then sends the aggregate feedback and source docs to the editor workflow. The editor agent writes complete proposed files under `updated-docs/files/` and a summary in `changelog.md`. The runner collects those artifacts before the sandbox is torn down, validates them, and returns only created or changed files. The CLI writes them to `.dari-docs/updated/`.

Use `--apply` only after you are comfortable overwriting matching files in your repo.

## Save URLs

To avoid passing URLs every time, save them in `.dari-docs/config.json`:

```bash
dari-docs init \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run
```

## Run The Runner Locally

You can run the runner on your machine instead of Modal. It needs Python 3.12 or newer.

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

The server listens on `127.0.0.1` unless you pass `--host`. A token is optional on loopback; with any other `--host`, such as `0.0.0.0`, set `DARI_DOCS_RUNNER_TOKEN` in the runner environment. With the default `modal` sandbox, a local runner still creates task sandboxes in your Modal account, so run `uvx modal setup` first. To keep sandboxes local, pass `--sandbox docker` to `dari-docs`; the runner's host then needs a running Docker engine.

```bash
dari-docs optimize . \
  --tester-url http://127.0.0.1:8000 \
  --editor-url http://127.0.0.1:8001 \
  --sandbox docker \
  --task "Follow the quickstart"
```

A local runner keeps run state in memory. Completed runs are kept for one hour, and all runs are lost when the process restarts.

### Controller Image

`Dockerfile` builds an optional controller image for hosts other than Modal. Its default command listens on `0.0.0.0`, so the container needs `DARI_DOCS_RUNNER_TOKEN`. The image contains only the controller; task sandboxes are created separately by the selected sandbox provider. A containerized controller can't use `--sandbox docker` unless you separately give it access to a Docker engine. To use other sandbox providers, add their Harbor extras or SDKs to `requirements.txt` before building.

## Workflow API

### Blocking Workflows

The CLI uses the blocking endpoints:

```text
POST <tester-url>/workflows/test?wait=result
POST <editor-url>/workflows/edit?wait=result
```

Each call submits a run and waits for it to finish. A completed run returns `200` with `{ "result": ... }`. A failed or cancelled run returns `502`. If the client disconnects, the runner requests cancellation of the run. The CLI's `--timeout-minutes` defaults to 60, which covers the runner's own limits below.

### Async Runs

Both services also expose an asynchronous runs API. Use it to submit a run and poll it without holding a connection open.

| Request | Response |
| --- | --- |
| `POST /runs` with `{ "workflow": "test" \| "edit", "payload": { ... } }` | `202` with `runId`, `statusUrl`, `resultUrl`, and `eventsUrl` |
| `GET /runs/{runId}` | Run status: `runId`, `workflow`, `status`, `createdAt`, `finishedAt`, `error` |
| `GET /runs/{runId}/events?after=N` | `{ "events": [...], "next": N, "status": "..." }` |
| `GET /runs/{runId}/result` | `{ "result": ... }` once completed; `409` otherwise |
| `DELETE /runs/{runId}` | `202` with `status: "cancellation-requested"`; `409` if already finished |
| `GET /healthz` | `{ "ok": true, "service": "...", "runtime": "harbor" }`; no token required |

The `workflow` must match the service: `test` on the tester URL, `edit` on the editor URL. The `payload` is the same object the blocking endpoint accepts. `GET` requests for a run created on the other service return `404`.

Run `status` moves from `queued` to `running` and ends as `completed`, `failed`, or `cancelled`. A run that hasn't finished about an hour after creation is reported as `failed`.

Cancellation is cooperative. `DELETE` records a request, and the controller cancels the Harbor trial the next time it checks, within about a second while the run is executing.

### Events

Events are lifecycle markers, not logs. Each event has `sequence`, `timestamp`, `event`, and, for Harbor trial events, `trialId`. Events include:

- `running` when the controller starts.
- Harbor trial lifecycle events, such as `start`, `environment-start`, `agent-start`, `agent-end`, and `end`.
- `artifact-retry` when the agent finished without writing its required output and the runner asked it to finish once more.
- `collecting-result` when the runner reads and validates artifacts.
- A final `completed`, `failed`, or `cancelled` event.

Each response returns up to 100 events after `after`. Pass the returned `next` as `after` on the next request, and stop when `status` is terminal and `events` is empty.

There is no endpoint for live agent logs or trajectories. Harbor's own `--stream` viewer, which supports only Claude Code and Codex on some sandboxes, is separate from this runner and isn't wired into it.

### Durability

On Modal, run state lives in the `dari-docs-harbor-runs` Modal Dict. Modal expires a Dict after 30 days without activity. There is no durable application queue: if a controller crashes or Modal restarts it, the run is not retried, and it is reported as `failed` once the deadline passes. Resubmit failed runs.

### Limits

| Limit | Value |
| --- | --- |
| Request body | 10 MiB (`413` above it) |
| Collected output and serialized result | 8 MiB each |
| Agent run | 30 minutes |
| Controller run, and hard lifetime of the `modal` task sandbox | 55 minutes |

Invalid payloads return `422` with a generic message. The runner doesn't echo payload values, because they may contain runtime secrets.

## Parallel Runs

Use `--parallel` to control how many tester workflow calls the CLI keeps in flight. Each call runs in its own controller and task sandbox, and the CLI combines the finished reports into the normal aggregate feedback file. Modal concurrency settings in `modal_app.py`, sandbox provider quotas, and model rate limits still apply. A local runner runs up to 30 trials at a time and queues the rest.

## Security Notes

- Tokens and credentials are not stored in `.dari-docs/config.json` or in run events.
- `--agent-env` and `--secret-env` values are used literally. Values containing `${...}` templates are rejected, so a request can't make the runner expand its own environment variables.
- Failed runs report a generic error instead of Harbor's exception text, which can echo command arguments or credentials. Inspect the controller's logs privately for details.
- The runner rejects results that contain a runtime secret value passed with `--secret-env`.
- Output artifacts can't contain symlinks, special files, or paths outside the output directory.

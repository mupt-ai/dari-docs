# Flue App Setup

`dari-docs` runs against Flue agents that you deploy when you choose self-managed mode. Flue is the TypeScript agent runtime used by the bundled tester and editor; it packages the agent code and workflow files into HTTP servers. The CLI only needs the tester base URL, and for `optimize`, the editor base URL.

The bundled path is Modal: `dari-docs init` extracts two Flue agent folders plus a Modal deploy file. The deployed Modal endpoints are lightweight gateways; each workflow request runs the actual Flue agent inside a fresh Modal Sandbox.

This is separate from `--managed`. Managed mode uses hosted Dari Docs agents deployed by the project maintainers with `sandbox.provider: modal`; self-managed users deploy their own Modal gateway URLs.

## Initialize

Run init in the repository that contains your docs:

```bash
dari-docs init
```

This extracts:

```text
.dari-docs/agents/modal_app.py
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

Each agent project is a normal, editable Flue folder with visible source files: `package.json`, `bun.lock`, `flue.config.ts`, `app.ts`, an agent entrypoint under `agents/`, one workflow under `workflows/`, prompts, and skills. There is no hidden `.flue/` directory in the initialized output.

## Deploy To Modal

Create a Modal secret for model provider keys. Include whichever providers your model choices need:

```bash
uvx modal setup
uvx modal secret create dari-docs-model-providers \
  ANTHROPIC_API_KEY=... \
  OPENAI_API_KEY=...
```

Deploy both agents:

```bash
uvx modal deploy .dari-docs/agents/modal_app.py
```

Modal prints two HTTPS URLs: one for the `tester` gateway and one for the `editor` gateway. The CLI calls:

```text
POST <tester-url>/workflows/test?wait=result
POST <editor-url>/workflows/edit?wait=result
```

If you want a different Modal secret name, set it when deploying:

```bash
DARI_DOCS_MODAL_SECRET=my-model-provider-secret \
  uvx modal deploy .dari-docs/agents/modal_app.py
```

## Run Checks

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --task "Install the SDK and make a first API call"
```

`check` writes reports to `.dari-docs/runs/` and `.dari-docs/aggregate-feedback.md`.

## Run Optimize

```bash
dari-docs optimize . \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run \
  --task "Install the SDK and make a first API call"
```

`optimize` runs the tester workflow first, then sends the aggregate feedback and source docs to the editor workflow. Proposed files are written to `.dari-docs/updated/`.

Use `--apply` only after you are comfortable overwriting matching files in your repo.

## Save URLs

To avoid passing URLs every time, save them in `.dari-docs/config.json`:

```bash
dari-docs init \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run
```

## Local Development

For local development instead of Modal, run an extracted app directly. The templates use Bun for install/build and Node 22+ for runtime; `bun run start` invokes `node dist/server.mjs` because Flue currently imports Node runtime modules that Bun does not implement.

Tester app:

```bash
cd .dari-docs/agents/docs-user-tester-agent
bun install --frozen-lockfile
bun run build
PORT=8787 ANTHROPIC_API_KEY=... bun run start
```

Editor app:

```bash
cd .dari-docs/agents/docs-editor-agent
bun install --frozen-lockfile
bun run build
PORT=8788 ANTHROPIC_API_KEY=... bun run start
```

The workflow URLs must be reachable by the `dari-docs` CLI. Modal URLs can execute an agent that runs shell commands. Keep them private to your org, or add authentication/private ingress before using them with sensitive docs or secrets.

## Parallel Runs And Model Configuration

The Modal template is configured for sandbox fanout. Each tester workflow request creates its own Modal Sandbox, starts the Flue tester server in that sandbox, forwards `POST /workflows/test?wait=result`, then terminates the sandbox after the result. Use `--parallel` to control how many tester workflow calls the CLI keeps in flight, then it combines the finished reports into the normal aggregate feedback file.

The Flue app has a default model, and each `dari-docs` run can request a model override in the workflow payload:

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --parallel 30 \
  --feedback-llm gpt-5.5 \
  --feedback-llm claude-opus-4-7 \
  --feedback-llm claude-sonnet-4-6 \
  --task "Install the SDK"
```

Short model IDs are normalized for common providers: `claude-*` becomes `anthropic/claude-*`, and `gpt-*` becomes `openai/gpt-*`. The Modal secret or app environment still needs the matching provider key.

Default provider key names:

```text
ANTHROPIC_API_KEY
OPENAI_API_KEY
OPENROUTER_API_KEY
```

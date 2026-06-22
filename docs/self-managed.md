# Flue App Setup

`dari-docs` runs against Flue agents that you deploy. Flue packages the agent code and workflow files into HTTP servers. The CLI only needs the tester base URL, and for `optimize`, the editor base URL.

The bundled path is Modal: `dari-docs init` extracts two Flue projects plus a Modal deploy file that serves both agents.

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

Each agent project includes `package.json`, `bun.lock`, `flue.config.ts`, an agent entrypoint under `agents/`, prompts, skills, a small `.flue/app.ts` HTTP entrypoint, and one workflow under `.flue/workflows/`.

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

Modal prints two HTTPS URLs: one for the `tester` web server and one for the `editor` web server. The CLI calls:

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

The Modal template is configured for horizontal fanout: one request per container and up to 50 containers. Use `--parallel` to control how many tester workflow calls the CLI keeps in flight, then it combines the finished reports into the normal aggregate feedback file.

The Flue app has a default model, and each `dari-docs` run can request a model override in the workflow payload:

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --parallel 30 \
  --feedback-llm gpt-5.5 \
  --feedback-llm claude-opus-4-8 \
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

# Flue App Setup

`dari-docs` runs against Flue apps that you run or deploy. Flue packages agent code and workflow files into an HTTP server. The CLI only needs the base URL of the tester app, and for `optimize`, the base URL of the editor app.

## Initialize

Run init in the repository that contains your docs:

```bash
dari-docs init
```

This extracts two normal Flue projects:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

Each project includes `package.json`, `bun.lock`, `flue.config.ts`, an agent entrypoint under `agents/`, prompts, skills, and one HTTP workflow under `.flue/workflows/`.

## Build And Run

The bundled apps use Bun for install/build commands and Node 22+ for the Flue server runtime. The default model is `anthropic/claude-sonnet-4-6`, so the examples set `ANTHROPIC_API_KEY`.

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

For production, deploy each app as a long-running service with Bun and Node 22+ available. Use the same commands as above and set provider environment variables on the host. `bun run start` runs `node dist/server.mjs` because Flue currently imports Node runtime modules that Bun does not implement. Render, Fly.io, Railway, a VM, ECS, or Kubernetes are fine as long as the service can run a persistent HTTP server. The important output is the base URL of each app, for example `https://docs-tester.example.com`.

The workflow URLs must be reachable by the `dari-docs` CLI. The CLI does not add an authorization header, so do not expose these apps publicly without network-level protection such as a VPN, private ingress, or firewall rules.

## Run Checks

```bash
dari-docs check . \
  --tester-url https://docs-tester.example.com \
  --task "Install the SDK and make a first API call"
```

`check` sends the docs bundle and task to `POST /workflows/test?wait=result` on the tester app. It writes reports to `.dari-docs/runs/` and `.dari-docs/aggregate-feedback.md`.

## Run Optimize

```bash
dari-docs optimize . \
  --tester-url https://docs-tester.example.com \
  --editor-url https://docs-editor.example.com \
  --task "Install the SDK and make a first API call"
```

`optimize` runs the tester workflow first, then sends the aggregate feedback and source docs to `POST /workflows/edit?wait=result` on the editor app. Proposed files are written to `.dari-docs/updated/`.

Use `--apply` only after you are comfortable overwriting matching files in your repo.

## Save URLs

To avoid passing URLs every time, save them in `.dari-docs/config.json`:

```bash
dari-docs init \
  --tester-url https://docs-tester.example.com \
  --editor-url https://docs-editor.example.com
```

## Model Configuration

The Flue app has a default model, and each `dari-docs` run can request a model override in the workflow payload:

```bash
dari-docs check . \
  --tester-url https://docs-tester.example.com \
  --feedback-llm gpt-5.5 \
  --feedback-llm claude-opus-4-8 \
  --feedback-llm claude-sonnet-4-6 \
  --task "Install the SDK"
```

Short model IDs are normalized for common providers: `claude-*` becomes `anthropic/claude-*`, and `gpt-*` becomes `openai/gpt-*`. The app still needs the matching provider key in its environment.

Default provider key names:

```text
ANTHROPIC_API_KEY
OPENAI_API_KEY
OPENROUTER_API_KEY
```

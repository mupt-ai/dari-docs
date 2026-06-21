# Flue App Setup

`dari-docs` runs against Flue apps that you deploy. Flue is the JavaScript agent runtime that builds a Node server from agent code and workflow files. The CLI does not deploy agents for you and does not require a separate service API key. It only needs the base URL of a deployed tester app, and for `optimize`, the base URL of a deployed editor app.

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

Each project includes `package.json`, `flue.config.ts`, an agent entrypoint under `agents/`, prompts, skills, and one HTTP workflow under `.flue/workflows/`.

## Build And Run

The bundled apps target Node because the agents use a local sandbox for shell commands and file writes. The sandbox gives the agent a working directory and command runner; it is not a hardened jail. Run these apps on hosts and networks you are comfortable letting an agent use. Use Node 20 or newer. The default model is `anthropic/claude-sonnet-4-6`, so the examples set `ANTHROPIC_API_KEY`.

Tester app:

```bash
cd .dari-docs/agents/docs-user-tester-agent
npm install
npx flue build --target node
PORT=8787 ANTHROPIC_API_KEY=... node dist/server.mjs
```

Editor app:

```bash
cd .dari-docs/agents/docs-editor-agent
npm install
npx flue build --target node
PORT=8788 ANTHROPIC_API_KEY=... node dist/server.mjs
```

For production, deploy `dist/`, `package.json`, and `package-lock.json` using your normal Node hosting platform. Install dependencies on the host with `npm ci --omit=dev` and start the server with `PORT=$PORT node dist/server.mjs`. Use a host that supports a persistent Node HTTP server, such as Render, Fly.io, Railway, a VM with systemd, ECS, or Kubernetes. Set the same provider environment variables there. The important output is the base URL of each app, for example `https://docs-tester.example.com`.

The workflow URLs must be reachable by the `dari-docs` CLI. The bundled CLI does not add an authorization header. Do not expose these apps publicly without network-level protection that still allows your CLI environment to reach them, such as a VPN, private ingress, or firewall rules.

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

Use `--apply` only after you are comfortable overwriting matching files in your repo:

```bash
dari-docs optimize . \
  --tester-url https://docs-tester.example.com \
  --editor-url https://docs-editor.example.com \
  --apply \
  --task "Install the SDK"
```

## Save URLs

To avoid passing URLs every time, save them in `.dari-docs/config.json`:

```bash
dari-docs init \
  --tester-url https://docs-tester.example.com \
  --editor-url https://docs-editor.example.com
```

You can edit `.dari-docs/config.json` later or override the saved values with command flags.

## Model Configuration

The Flue app has a default model, and each `dari-docs` run can request a model override in the workflow payload. Use `--llm` for one model across tester and editor workflows, `--feedback-llm` for tester model matrices, and `--editor-llm` for the editor workflow.

```bash
dari-docs check . \
  --tester-url https://docs-tester.example.com \
  --feedback-llm gpt-5.5 \
  --feedback-llm claude-opus-4-8 \
  --feedback-llm claude-sonnet-4-6 \
  --task "Install the SDK"
```

Model strings without a provider prefix are normalized for common providers: `claude-*` becomes `anthropic/claude-*`, and `gpt-*` becomes `openai/gpt-*`. To change the app default, edit the agent TypeScript files, then rebuild and redeploy the app.

The default provider key names are:

```text
ANTHROPIC_API_KEY
OPENAI_API_KEY
OPENROUTER_API_KEY
```

Set the key used by your chosen provider in the Flue app deployment environment. Do not put model provider keys in your docs repo or pass them as live verification secrets.

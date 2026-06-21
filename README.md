# dari-docs

> Make your docs so good even the dumbest agent can ship.

`dari-docs` tests whether your documentation is clear enough to use. It gives a deployed Flue tester app a real task, collects what happened, and can ask a deployed Flue editor app to turn that feedback into proposed docs changes.

The only supported runtime path is Flue app deployment. Flue is the JavaScript agent runtime that packages an agent plus workflows into a Node server. A workflow is an HTTP entrypoint in that server. `dari-docs init` extracts two ordinary Flue projects into your repo. You install, build, and run those projects with Flue on a local or hosted Node server. The `dari-docs` CLI then calls their workflow URLs.

## Quickstart

Install the CLI:

```bash
curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | bash
dari-docs --help
```

From the repo that contains your docs, extract the bundled Flue apps:

```bash
dari-docs init
```

This creates:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

Each extracted app has its own `package.json`; `npm install` installs the Flue CLI/runtime used by `npx flue`. Build and deploy the tester app. Use Node 20 or newer. The bundled apps default to `anthropic/claude-sonnet-4-6`, so set `ANTHROPIC_API_KEY` in the deployment environment unless you edit the agent to use another provider.

```bash
cd .dari-docs/agents/docs-user-tester-agent
npm install
npx flue build --target node
PORT=8787 ANTHROPIC_API_KEY=... node dist/server.mjs
```

Deploy the editor app the same way when you want `optimize`:

```bash
cd ../docs-editor-agent
npm install
npx flue build --target node
PORT=8788 ANTHROPIC_API_KEY=... node dist/server.mjs
```

For a first run, keeping the app on your machine is fine: the local URLs above would be `http://127.0.0.1:8787` and `http://127.0.0.1:8788`. In production, deploy each app as a long-running Node service with `dist/`, `package.json`, and `package-lock.json`, run `npm ci --omit=dev`, and start it with `PORT=$PORT node dist/server.mjs`. Render, Fly.io, Railway, a VM with systemd, ECS, or any other Node 20 host is fine as long as it can run a persistent HTTP server.

Run a docs check:

```bash
dari-docs check . \
  --tester-url https://your-tester.example \
  --task "Install the SDK and make a first API call"
```

The check writes individual tester reports under `.dari-docs/runs/` and an aggregate report to `.dari-docs/aggregate-feedback.md`. Feedback is not a pass/fail score; read it to decide what to fix.

Ask the editor app for proposed docs changes:

```bash
dari-docs optimize . \
  --tester-url https://your-tester.example \
  --editor-url https://your-editor.example \
  --task "Install the SDK and make a first API call"
```

`optimize` writes proposed files under `.dari-docs/updated/`. It does not change your repo unless you pass `--apply`. With `--apply`, files from `.dari-docs/updated/` are copied into your repo; matching paths are overwritten, new files are created, and files that are absent from `.dari-docs/updated/` are not deleted.

```bash
dari-docs optimize . \
  --tester-url https://your-tester.example \
  --editor-url https://your-editor.example \
  --apply \
  --task "Install the SDK and make a first API call"
```

## Saving Deployment URLs

You can save deployment URLs during init so you do not have to pass them every time:

```bash
dari-docs init \
  --tester-url https://your-tester.example \
  --editor-url https://your-editor.example
```

The URLs are stored in `.dari-docs/config.json`. You can edit that file or override the URLs with flags on any run.

## What The Flue Apps Do

The tester app exposes a `test` workflow at `/workflows/test`. `dari-docs check` posts the docs, task, and optional public docs URLs to that workflow with `?wait=result`, which tells Flue to hold the HTTP response open until the workflow returns its result. The workflow creates a Flue agent workspace directory for that run, writes the supplied docs under `input-docs/files/`, asks the tester agent to try the task, and returns Markdown feedback. The bundled agents can read and write inside that workspace, run shell commands, and use network access provided by the host. Treat the workspace as a task working directory, not a hard security boundary; run these apps on hosts and networks you are comfortable letting an agent use.

The editor app exposes an `edit` workflow at `/workflows/edit`. `dari-docs optimize` first runs the tester workflow, then posts the aggregate feedback and source docs to the editor workflow. The editor returns proposed files, which the CLI writes to `.dari-docs/updated/`.

The apps are normal Flue projects. Their entrypoints live under `agents/`, prompts under `prompts/`, skills under `skills/`, and workflows under `.flue/workflows/`. Keep the workflow base URLs reachable by the CLI. If you expose them beyond a private network, protect them with network controls such as a VPN, private ingress, or firewall rules. The CLI does not add an authorization header. There is no hosted service or custom deployment step in the supported flow.

## Bundling And Secrets

Before you run against a local checkout, make sure the repo does not contain secrets that match the default bundle rules. By default, the bundle includes common docs and docs-adjacent files such as Markdown, MDX, JSON, YAML, TOML, CSS, JavaScript, and TypeScript. That can include config files, OpenAPI examples, generated JSON, or TypeScript snippets if they live in your docs tree. Use `--bundle-exclude` for files that should not be sent to your Flue app.

Environment variables are not included unless you explicitly pass them with `--live-verify --secret-env NAME`. Use that only for safe test-mode product credentials. Model provider keys such as `ANTHROPIC_API_KEY` belong in the Flue app deployment environment, not in the docs bundle.

## Useful Options

Use more than one task by repeating `--task`:

```bash
dari-docs check . \
  --tester-url https://your-tester.example \
  --task "Install the SDK" \
  --task "Set up authentication"
```

For repeated checks, keep tasks in a file and pass `--tasks-file`.

Use `--bundle-include` and `--bundle-exclude` when your docs need extra files or when generated docs should be skipped.

Use `--docs-url` with `check` to test public documentation without checking out a repo. `llms.txt` and `llms-full.txt` are LLM-oriented docs index files that list pages an agent should read.

```bash
dari-docs check \
  --tester-url https://your-tester.example \
  --docs-url https://docs.example.com/llms.txt \
  --task "Create a browser session"
```

## Documentation

- [Flue app setup](docs/self-managed.md)
- [Agent customization](docs/agent-customization.md)
- [GitHub Actions](docs/github-actions.md)
- [Task files and repeated checks](docs/tasks.md)
- [Bundle selection](docs/bundle-selection.md)
- [Live verification secrets](docs/live-verification.md)
- [Local development](docs/local-development.md)

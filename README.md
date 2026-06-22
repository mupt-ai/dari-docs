# dari-docs

> Make your docs so good even the dumbest agent can ship.

`dari-docs` checks whether your documentation is clear enough to use. It bundles selected docs files, sends them with a real task to a Flue tester app, and collects feedback from an agent that reads the docs, writes files, and runs shell commands in a workspace. It can also ask a Flue editor app to propose documentation changes.

Flue is the JavaScript agent runtime used by the bundled tester/editor apps. There is no hosted or managed Dari Docs service in this flow: you run the Flue apps yourself and point the CLI at their URLs. Because the tester app can run shell commands, keep those URLs private. The templates use Bun for installs/builds and Node 22+ for the Flue server runtime.

## Quickstart

Install the CLI:

```bash
curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | bash
```

From the repo that contains your docs, extract the bundled Flue apps:

```bash
dari-docs init
```

Start the tester app. The bundled apps default to `anthropic/claude-sonnet-4-6`, so set `ANTHROPIC_API_KEY` unless you request a different provider/model.

```bash
cd .dari-docs/agents/docs-user-tester-agent
bun install --frozen-lockfile
bun run build
PORT=8787 ANTHROPIC_API_KEY=... bun run start
```

In another terminal, run a check:

```bash
dari-docs check . \
  --tester-url http://127.0.0.1:8787 \
  --task "Install the SDK and make a first API call"
```

Feedback is written to `.dari-docs/aggregate-feedback.md` and `.dari-docs/runs/`. A completed check is not a pass/fail score: the command exits zero when the workflow completed, even if the tester feedback says the docs were confusing.

## Optimize Docs

Start the editor app when you want proposed edits:

```bash
cd .dari-docs/agents/docs-editor-agent
bun install --frozen-lockfile
bun run build
PORT=8788 ANTHROPIC_API_KEY=... bun run start
```

Then run:

```bash
dari-docs optimize . \
  --tester-url http://127.0.0.1:8787 \
  --editor-url http://127.0.0.1:8788 \
  --task "Install the SDK and make a first API call"
```

`optimize` runs a fresh tester pass first, sends the aggregate feedback to the editor app, and writes proposed files under `.dari-docs/updated/`. It does not change your repo unless you pass `--apply`; with `--apply`, files are copied by repo-relative path, existing files at those paths are overwritten, new files are created, and files missing from `.dari-docs/updated/` are not deleted.

## Save App URLs

Save tester/editor URLs once:

```bash
dari-docs init \
  --tester-url https://your-tester.example \
  --editor-url https://your-editor.example
```

After that, you can omit the URL flags:

```bash
dari-docs check . --task "Install the SDK"
```

## Model Matrix

One tester app can run the same task with multiple models. Runs are sequential and reported separately, then aggregated:

```bash
dari-docs check . \
  --tester-url https://your-tester.example \
  --feedback-llm gpt-5.5 \
  --feedback-llm claude-opus-4-8 \
  --feedback-llm claude-sonnet-4-6 \
  --task "Install the SDK"
```

Short model IDs are normalized for common providers: `claude-*` becomes `anthropic/claude-*`, and `gpt-*` becomes `openai/gpt-*`. The Flue app still needs the matching provider key in its environment, such as `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`.

## Deploy The Flue Apps

Each extracted app is a normal Flue project with `package.json`, `bun.lock`, `flue.config.ts`, agent code under `agents/`, and workflows under `.flue/workflows/`.

For production, deploy each app as a long-running service with Bun and Node 22+ available:

```bash
bun install --frozen-lockfile
bun run build
bun run start
```

`bun run start` runs `node dist/server.mjs`; Flue currently imports Node runtime modules that Bun does not implement. Set `PORT` and model provider keys in the deployment environment. Render, Fly.io, Railway, a VM, ECS, or Kubernetes are fine as long as the service can run a persistent HTTP server.

The CLI does not add an authorization header. Do not expose the Flue apps publicly without network-level protection such as a VPN, private ingress, or firewall rules.

## Bundling And Secrets

Before running against a local checkout, make sure the repo does not contain secrets that match the bundle rules. By default, `dari-docs` includes common docs and docs-adjacent files such as Markdown, MDX, JSON, YAML, TOML, CSS, JavaScript, and TypeScript; see [Bundle selection](docs/bundle-selection.md) for the full behavior. Use `--bundle-exclude` for files that should not be sent to the Flue app.

Environment variables are not included unless you explicitly pass them with `--live-verify --secret-env NAME`. Live verification means the agent may use those named credentials to try safe test-mode API calls while following the docs. Model provider keys belong in the Flue app environment, not in the docs bundle.

## Documentation

- [Flue app setup](docs/self-managed.md)
- [Agent customization](docs/agent-customization.md)
- [GitHub Actions](docs/github-actions.md)
- [Task files and repeated checks](docs/tasks.md)
- [Bundle selection](docs/bundle-selection.md)
- [Live verification secrets](docs/live-verification.md)
- [Local development](docs/local-development.md)

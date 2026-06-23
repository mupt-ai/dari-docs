# dari-docs

> Make your docs so good even the dumbest agent can ship.

`dari-docs` checks whether your documentation is clear enough to use. It bundles selected docs files or passes public docs URLs, sends them with a real task to hosted tester agents, and collects feedback from agents that read docs, write files, and run shell commands in a workspace. It can also ask an editor agent to propose documentation changes.

The fastest path is managed mode: `dari-docs` submits runs to the hosted Dari Docs service. The managed tester and editor agents run on Modal-backed Dari sandboxes, so you do not deploy or operate agent infrastructure. If you want to customize the agents, use the self-managed Flue templates and deploy their Modal gateway yourself.

## Quickstart

Install the CLI:

```bash
curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | bash
```

From the repo that contains your docs, log in and run a managed check:

```bash
dari-docs auth login
dari-docs check . \
  --managed \
  --wait \
  --feedback-llm claude-haiku-4-5 \
  --task "Install the SDK and make a first API call"
```

Feedback is written to `.dari-docs/aggregate-feedback.md` and `.dari-docs/runs/`. A completed check is not a pass/fail score: the command exits zero when the managed run completed, even if the tester feedback says the docs were confusing. In CI, add your own policy step that reads the aggregate feedback if you want confusing docs to block a change.

## Self-Managed Flue Apps

Use self-managed mode when you want to edit the bundled Flue agents or run them under your own Modal account. From the repo that contains your docs, extract the bundled Flue agent folders:

```bash
dari-docs init
```

Deploy the tester and editor Flue apps to Modal. Put your model provider keys in one Modal secret; include whichever providers your model choices need.

```bash
uvx modal setup
uvx modal secret create dari-docs-model-providers \
  ANTHROPIC_API_KEY=... \
  OPENAI_API_KEY=...
uvx modal deploy .dari-docs/agents/modal_app.py
```

Modal prints two HTTPS endpoints, one labeled `tester` and one labeled `editor`. Use the tester URL for checks:

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --task "Install the SDK and make a first API call"
```

## Optimize Docs

Managed optimize runs use the hosted Modal-backed editor agent:

```bash
dari-docs optimize . \
  --managed \
  --wait \
  --task "Install the SDK and make a first API call"
```

Self-managed optimize uses both Modal URLs:

```bash
dari-docs optimize . \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run \
  --task "Install the SDK and make a first API call"
```

`optimize` runs a fresh tester pass first, sends the aggregate feedback to the editor agent, and writes proposed files under `.dari-docs/updated/`. It does not change your repo unless you pass `--apply`; with `--apply`, files are copied by repo-relative path, existing files at those paths are overwritten, new files are created, and files missing from `.dari-docs/updated/` are not deleted.

## Save Agent URLs

Save tester/editor URLs once:

```bash
dari-docs init \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run
```

After that, you can omit the URL flags:

```bash
dari-docs check . --task "Install the SDK"
```

## Model Matrix

Managed mode can run the same task with multiple hosted model options:

```bash
dari-docs check . \
  --managed \
  --wait \
  --feedback-llm claude \
  --task "Install the SDK"
```

For self-managed Flue apps, add `--parallel 30` to fan out up to 30 tester workflow calls at once. The Modal gateway starts a fresh Modal Sandbox for each workflow request, runs Flue inside that sandbox, terminates it after the result, and then the CLI combines the returned reports:

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --parallel 30 \
  --feedback-llm gpt-5.5 \
  --feedback-llm claude-opus-4-7 \
  --feedback-llm claude-sonnet-4-6 \
  --task "Install the SDK"
```

Short model IDs are normalized for common providers in self-managed Flue apps: `claude-*` becomes `anthropic/claude-*`, and `gpt-*` becomes `openai/gpt-*`. The Modal secret still needs the matching provider key, such as `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`. Managed mode uses platform-managed model credentials.

## What Gets Deployed

Managed users do not deploy agents. The hosted Dari Docs tester/editor agents are deployed from this repo with `dari.yml` manifests that set `sandbox.provider: modal` and omit BYOK LLM or Modal credential fields.

`dari-docs init` writes normal self-managed Flue projects under `.dari-docs/agents/`. Each agent folder has visible source files: `app.ts`, `agents/`, `workflows/`, `prompts/`, and `skills/`. There is no hidden `.flue/` layout to learn or edit.

- `docs-user-tester-agent/` exposes `POST /workflows/test?wait=result`.
- `docs-editor-agent/` exposes `POST /workflows/edit?wait=result`.
- `modal_app.py` deploys tester/editor gateway endpoints. Each workflow request creates a Modal Sandbox, starts the matching Flue server inside it, forwards the workflow call, then terminates the sandbox.

The agent templates use Bun for install/build (`bun install --frozen-lockfile`, `bun run build`) and Node 22+ for runtime (`bun run start` runs `node dist/server.mjs`). Flue currently imports Node runtime modules that Bun does not implement, so the runtime is intentionally Node.

For local development instead of Modal, run an extracted app directly:

```bash
cd .dari-docs/agents/docs-user-tester-agent
bun install --frozen-lockfile
bun run build
PORT=8787 ANTHROPIC_API_KEY=... bun run start
```

## Bundling And Secrets

Before running against a local checkout, make sure the repo does not contain secrets that match the bundle rules. By default, `dari-docs` includes common docs and docs-adjacent files such as Markdown, MDX, JSON, YAML, TOML, CSS, JavaScript, and TypeScript; see [Bundle selection](docs/bundle-selection.md) for the full behavior. Use `--bundle-exclude` for files that should not be sent to the Flue app.

Environment variables are not included unless you explicitly pass them with `--live-verify --secret-env NAME`. Live verification means the agent may use those named credentials to try safe test-mode API calls while following the docs. Model provider keys belong in the Modal secret or Flue app environment, not in the docs bundle.

Modal URLs can execute an agent that runs shell commands. Keep them private to your org, or add authentication/private ingress before using them with sensitive docs or secrets.

## Documentation

- [Managed mode and billing](docs/managed.md)
- [Flue app setup](docs/self-managed.md)
- [Agent customization](docs/agent-customization.md)
- [GitHub Actions](docs/github-actions.md)
- [Task files and repeated checks](docs/tasks.md)
- [Bundle selection](docs/bundle-selection.md)
- [Live verification secrets](docs/live-verification.md)
- [Local development](docs/local-development.md)

# dari-docs

> Make your docs so good even the dumbest agent can ship.

`dari-docs` tests whether your documentation is clear enough for agents to use. It deploys two Flue agents to your dari.dev org: a tester agent that tries tasks you provide using your docs, and an editor agent that can turn the tester feedback into proposed documentation changes.

Flue is the agent runtime used by dari.dev. A Flue agent is a deployed program that can read inputs, call tools, and work in a remote session workspace. A dari.dev org is your team workspace/account, and a Dari credential is a secret stored in that org for deployed agents to use.

Use `dari-docs` when you want a repeatable answer to “can someone actually follow these docs?” instead of a vibes-based docs review. The feedback is useful for docs meant for humans, agents, or both: the tester behaves like a developer trying to get a task done from the written instructions.

## Quickstart

Before you start, you need a dari.dev account with an org/workspace, a Dari API key for that org, and a model provider key for the agents. Create or copy the Dari API key from your dari.dev dashboard. If your org uses scoped keys, grant permissions to deploy agents, upload files, create sessions, read session transcripts, and download session workspaces. The examples below use an Anthropic key stored as a Dari credential named `ANTHROPIC_API_KEY`.

Install the CLI:

```bash
curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | bash
dari-docs --help
```

`dari-docs init --deploy` calls `dari deploy`, so install the Dari CLI too if `dari` is not already on your PATH:

```bash
curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-cli/main/install.sh | bash
```

From the repo that contains your docs, deploy the bundled Flue agents to your dari.dev org:

```bash
# One-time setup for your org.
dari auth login
dari credentials add ANTHROPIC_API_KEY # paste your Anthropic key when prompted
export DARI_API_KEY=... # create or copy a dari.dev API key for this org

# Extracts the Flue agent projects and deploys them.
dari-docs init --deploy
```

Before you run `check .`, make sure the repo does not contain secrets that match the default bundle rules. By default, the bundle includes common docs and docs-adjacent files such as Markdown, MDX, JSON, YAML, TOML, CSS, JavaScript, and TypeScript. `dari-docs` uploads that docs bundle to your Dari org for the run. Use `--bundle-exclude` for any local files that should not be sent.

Now run a check:

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call"
```

The check writes feedback to `.dari-docs/aggregate-feedback.md` and individual tester reports under `.dari-docs/runs/`. Completed feedback is not a pass/fail score; read it to decide what to fix.

To ask the editor agent for proposed docs changes, run:

```bash
dari-docs optimize . \
  --task "Install the SDK and make a first API call"
```

`optimize` writes proposed changes under `.dari-docs/updated/`. It does not change your repo unless you pass `--apply`. With `--apply`, regular files from `.dari-docs/updated/` are copied into your repo and existing files at the same paths are overwritten. It does not apply a patch or delete files that are absent from the update.

```bash
dari-docs optimize . \
  --apply \
  --task "Install the SDK and make a first API call"
```

## What gets deployed

`dari-docs init` extracts the bundled agents into your repo under `.dari-docs/agents/`:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

Each folder is a regular Flue agent project with a `dari.yml`, `package.json`, TypeScript entrypoint, prompts, and skills. `dari-docs init --deploy` runs `dari deploy` for both agents and saves their agent IDs in `.dari-docs/config.json` so future `check` and `optimize` commands can call them.

The bundled agents default to `anthropic/claude-sonnet-4-6`. Their deploy manifests expect a Dari credential named `ANTHROPIC_API_KEY`. A Dari credential is a secret stored in your dari.dev org and exposed to deployed Flue agents by name.

If your Anthropic credential uses a different name, pass it during init:

```bash
dari-docs init --deploy --anthropic-api-key-secret TEAM_ANTHROPIC_KEY
```

If you edit the agents to use OpenAI, pass the stored OpenAI credential name too:

```bash
dari-docs init --deploy \
  --anthropic-api-key-secret TEAM_ANTHROPIC_KEY \
  --openai-api-key-secret TEAM_OPENAI_KEY
```

Model selection lives in the Flue agent project, not in `dari-docs check` flags. To change models, edit the agent TypeScript entrypoints and redeploy.

## How it works

When you run `check`, the CLI bundles your docs, uploads the bundle to your dari.dev org, and starts tester sessions against your deployed Flue tester agent. The tester receives a task prompt plus either the uploaded archive or a public docs URL. Tester reports are downloaded as Markdown under `.dari-docs/runs/`, and the aggregate feedback is written to `.dari-docs/aggregate-feedback.md`. The bundle is the set of local files the agent can read for the run; the CLI prints a summary and writes the archive to `.dari-docs/input-docs-bundle.tar.gz`. Environment variables are not included unless you explicitly pass them with `--live-verify --secret-env`, but secrets committed to files can still be bundled, so exclude them with `--bundle-exclude`.

Each tester receives the docs and one task, then may search the docs, create files, install packages, run shell commands, and use internet access when the task or public docs source requires it. The tester works in a remote Flue workspace that is separate from your checkout, so tester commands do not modify your local repo. A completed tester report is feedback, not an automatic quality gate; the CLI exits nonzero when setup, upload, deployment, session execution, or download fails.

When you run `optimize`, `dari-docs` first runs the tester sessions, then sends the collected feedback and source docs to your deployed Flue editor agent. The editor writes proposed files for review, and the CLI downloads those files into `.dari-docs/updated/`.

## Useful options

Use more than one task by repeating `--task`:

```bash
dari-docs check . \
  --task "Install the SDK" \
  --task "Set up authentication"
```

For repeated checks, keep tasks in a file and pass `--tasks-file`.

Use `--bundle-include` and `--bundle-exclude` when your docs need extra files or when generated docs should be skipped.

Use `--docs-url` with `check` to test public documentation without checking out a repo:

```bash
dari-docs check \
  --docs-url https://docs.dari.dev/llms.txt \
  --task "Create a browser session"
```

Use `--live-verify --secret-env NAME` only when you want agents to receive safe test-mode product credentials for a run.

## Documentation

- [Flue agent usage](docs/self-managed.md)
- [Agent customization](docs/agent-customization.md)
- [GitHub Actions](docs/github-actions.md)
- [Task files and repeated checks](docs/tasks.md)
- [Bundle selection](docs/bundle-selection.md)
- [Live verification secrets](docs/live-verification.md)
- [Local development](docs/local-development.md)

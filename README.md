# dari-docs

Run simulated users against your docs, then pull back edited docs.

`dari-docs` is a standalone CLI that includes the Dari agents it needs.

It bundles your docs, deploys Dari agents, then has them test your docs and writes the updated docs back locally.

You can run it two ways:

- **Managed**: use the hosted Dari Docs service. No local Dari API key is required.
- **Self-managed**: use your own Dari org and API key.

## How It Works

```text
local docs
  -> bundled by the CLI
  -> tested by remote simulated users on specific tasks
  -> summarized into feedback
  -> optionally edited by a remote docs editor
  -> downloaded into .dari-docs/updated/
```

The tester agent acts like a developer trying to complete each task and reports where the docs blocked progress. The editor agent turns that feedback into proposed docs edits.

## Install

Install with Go:

```bash
go install github.com/mupt-ai/dari-docs/cmd/dari-docs@latest
dari-docs --help
```

Or build from this repo:

```bash
go build ./cmd/dari-docs
./dari-docs --help
```

## Managed Quickstart

Managed mode uses the hosted Dari Docs service and a separate Dari Docs credit balance. New accounts start with five dollars worth of free credits.

From your docs repo:

```bash
dari-docs auth login
dari-docs init
dari-docs agents deploy --managed
```

Agent deployment can continue after the CLI exits.

- If the command is interrupted while waiting, rerun `dari-docs agents deploy --managed` from the same repo to resume watching the pending deployment.
- If you changed the local agent files and want to start a new deployment in the middle of waiting on an existing deployment, use `--force-new`.

Ask tester Dari agents to attempt specific tasks based on the docs:

```bash
dari-docs check . \
  --managed \
  --task "Install the SDK and make a first API call"
```

Generate doc revisions based on tester feedback:

```bash
dari-docs optimize . \
  --managed \
  --task "Install the SDK and make a first API call"
```

This downloads proposed revisions into `.dari-docs/updated/` without changing your repo.

Apply the edited docs:

```bash
dari-docs optimize . \
  --managed \
  --task "Install the SDK and make a first API call" \
  --apply
```

## Managed Account and Billing

After logging in, check your balance:

```bash
dari-docs billing balance
```

Purchase more credits:

```bash
dari-docs billing checkout --amount 5
```

Log out:

```bash
dari-docs auth logout
```

Before a managed run starts, the CLI prints a bundle summary and credit estimate. Credits are reserved before the run, then reconciled to the actual session cost after completion.

- Managed runs currently support up to three tasks per run.
- Managed runs currently support up to three active runs per account at a time.
- Managed runs execute tasks sequentially; use self-managed mode if you need parallel tester sessions.

## Tasks

Pass one or more `--task` values:

```bash
dari-docs check . \
  --managed \
  --task "Install the SDK" \
  --task "Set up authentication"
```

Or keep tasks in a file, one task per paragraph or bullet:

```bash
dari-docs check . \
  --managed \
  --tasks-file docs-test-tasks.txt
```

By default, local run artifacts are written under `.dari-docs/`, and later runs overwrite the previous local outputs. Use `--out` to keep separate run directories:

```bash
dari-docs optimize . \
  --managed \
  --out .dari-docs/runs/install-sdk \
  --task "Install the SDK and make a first API call"
```

When `--out` is used with `--apply`, the CLI still applies the downloaded revisions back into the target repo.

## Bundle Selection

Before a run starts, `dari-docs` creates `.dari-docs/input-docs-bundle.tar.gz`. By default it includes likely docs and docs-adjacent source files, while skipping common generated, dependency, build, and local output directories.

Use repo-relative globs when your docs need extra inputs or when generated paths should be excluded:

```bash
dari-docs check . \
  --managed \
  --bundle-include "schemas/*.proto" \
  --bundle-exclude "docs/generated/**" \
  --task "Create an API key"
```

- `--bundle-include` adds files in addition to the defaults.
- `--bundle-exclude` wins over both defaults and include patterns.

The CLI prints a bundle summary before starting the run.

## Live Verification Secrets

Static testing is the default. To let agents use test-mode product/API keys for safe checks, pass `--live-verify` and repeat `--secret-env NAME` for local environment variables to send.

Managed example:

```bash
export STRIPE_TEST_SECRET_KEY=sk_test_...

dari-docs optimize . \
  --managed \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --task "Create a checkout session"
```

Secret values are passed to the remote sessions only for that run. In managed mode, runtime secrets are encrypted while the run is active and cleared after use.

## Agent Customization

`dari-docs init` creates local agent projects under:

```text
.dari-docs/agents/
```

These are regular Dari agent projects. You can edit the local prompts and skills before deploying them.

The bundled tester agent enables sandbox internet access by default so it can install packages and try docs that call external services. You can turn this off in `.dari-docs/agents/docs-user-tester-agent/dari.yml` before deploying if you want tests to run without network access.

For customized agents, network access is controlled in each agent's `dari.yml`:

```yaml
sandbox:
  internet_access: true
```

In managed mode, deploy the edited agents with:

```bash
dari-docs agents deploy --managed
```

In self-managed mode, `dari-docs init --deploy` deploys the default local agents. For more control, deploy your own Dari agents and pass their IDs with `--feedback-agent` and `--editor-agent`.

## Self-Managed Usage

Use self-managed mode when you want runs to execute in your own Dari org.

```bash
dari auth login
export DARI_API_KEY=...
dari-docs init --deploy
```

Then run the same commands without `--managed`:

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call"

dari-docs optimize . \
  --task "Install the SDK and make a first API call"
```

By default, the bundled agents expose named LLM options such as `dumb-claude`, `medium-claude`, `smart-claude`, `dumb-gpt`, `medium-gpt`, and `smart-gpt`. In self-managed mode, tester sessions run every task across all six options by default; the editor uses the manifest default (`medium-claude`).

To explicitly choose tester model tiers:

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call" \
  --feedback-llm dumb-claude,medium-claude,smart-claude
```

Use `--llm ID` to collapse the run to one option for all sessions, or `--editor-llm ID` to select the editor model independently.

To use your own stored provider key at agent deploy time:

```bash
dari credentials add MY_OPENROUTER_KEY
dari-docs init --deploy --llm-api-key-secret MY_OPENROUTER_KEY
```

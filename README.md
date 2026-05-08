# dari-docs

Run simulated users against your docs, then pull back edited docs.

`dari-docs` is a standalone CLI. It includes the Dari agents it needs, deploys them into your Dari org, runs them on your docs, and writes the updated docs back locally.

## Install

```bash
go install github.com/mupt-ai/dari-docs/cmd/dari-docs@latest
```

Or from this repo:

```bash
go build ./cmd/dari-docs
```

## Setup

```bash
dari auth login
export DARI_API_KEY=...
```

From your docs directory:

```bash
dari-docs init --deploy
```

This creates:

```text
.dari-docs/
  agents/        # bundled tester/editor agents
  config.json    # deployed agent IDs
```

By default, the agents use Dari's platform-managed LLM. To use your own stored LLM key at agent deploy time:

```bash
dari credentials add MY_OPENROUTER_KEY
dari-docs init --deploy --llm-api-key-secret MY_OPENROUTER_KEY
```

## Get feedback only

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call" \
  --task "Set up authentication"
```

Results:

```text
.dari-docs/aggregate-feedback.md
.dari-docs/runs/feedback-001.md
```

## Generate edited docs

```bash
dari-docs optimize . \
  --task "Install the SDK and make a first API call" \
  --task "Set up authentication"
```

Updated files are downloaded to:

```text
.dari-docs/updated/updated-docs/files/
```

Preview the diff:

```bash
diff -ru --exclude='.dari-docs' . .dari-docs/updated/updated-docs/files
```

Apply changes:

```bash
dari-docs optimize . --task "Set up authentication" --apply
```

Or manually:

```bash
rsync -av .dari-docs/updated/updated-docs/files/ ./
```

## Live verification secrets

Static/local testing is the default. To let tester agents use product API keys for safe checks:

```bash
dari-docs optimize . \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --task "Create a checkout session"
```

Secret values are sent only as session secrets and should not appear in reports.

## How it works

```text
local docs
  -> bundled and uploaded
  -> tester agent sessions try each task
  -> editor agent updates docs in /workspace/updated-docs
  -> CLI downloads updated files
```

The tester agent is intentionally lightweight: it acts like a developer trying the task and reports what blocked it. The editor agent turns that feedback into updated docs files.

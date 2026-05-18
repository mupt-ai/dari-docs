# Managed Mode

Managed mode runs `dari-docs` through the hosted dari.dev Docs service. Use it when you want to test and optimize docs without managing your own dari.dev org or API key.

Under the hood, the tester and editor are ordinary dari.dev agents: folders of prompts, skills, setup scripts, and a `dari.yml` manifest. Managed mode runs sessions through hosted agents so you can test docs without deploying or operating agent infrastructure.

## Set up managed mode

From your docs repo, log in:

```bash
dari-docs auth login
```

## Run a managed check

Ask the tester agents to attempt one or more tasks from your docs:

```bash
dari-docs check . \
  --managed \
  --task "Install the SDK and make a first API call"
```

## Generate proposed edits

Use `optimize` to turn tester feedback into proposed documentation revisions:

```bash
dari-docs optimize . \
  --managed \
  --task "Install the SDK and make a first API call"
```

The edited files are downloaded into `.dari-docs/updated/` without changing your repo. To apply the edited docs directly, add `--apply`:

```bash
dari-docs optimize . \
  --managed \
  --task "Install the SDK and make a first API call" \
  --apply
```

## Account and billing

New accounts start with five dollars worth of free credits. After logging in, check your balance with:

```bash
dari-docs billing balance
```

Purchase more credits with:

```bash
dari-docs billing checkout --amount 5
```

Before a managed run starts, the CLI prints a bundle summary and credit estimate. Credits are reserved before the run, then reconciled to the actual session cost after completion.

Managed runs currently support up to three tasks per run and three active runs per account at a time. Tester sessions are started with the Dari session-batch API; optimize runs start the editor after tester feedback is complete.

## Model selection

Managed mode supports the hosted Claude and GPT LLM options:

- `dumb-claude`
- `medium-claude`
- `smart-claude`
- `dumb-gpt`
- `medium-gpt`
- `smart-gpt`

By default, managed tester sessions run each task across all three Claude options. The editor uses `medium-claude`.

Use one model for every managed session:

```bash
dari-docs check . \
  --managed \
  --llm smart-claude \
  --task "Install the SDK and make a first API call"
```

Or choose the tester and editor models separately:

```bash
dari-docs optimize . \
  --managed \
  --feedback-llm dumb-claude,smart-claude \
  --editor-llm smart-claude \
  --task "Install the SDK and make a first API call"
```

For tester sessions, `--feedback-llm` also accepts groups:

- `claude` expands to `dumb-claude`, `medium-claude`, and `smart-claude`
- `gpt` expands to `dumb-gpt`, `medium-gpt`, and `smart-gpt`
- `all` expands to all six hosted options

You can mix groups and explicit IDs:

```bash
dari-docs check . \
  --managed \
  --feedback-llm claude,medium-gpt \
  --task "Install the SDK and make a first API call"
```

## Log out

Log out with:

```bash
dari-docs auth logout
```

To revoke managed tokens from all devices, run:

```bash
dari-docs auth logout --all
```

You can narrow revocation to browser-login tokens with `--interactive-only` or automation tokens with `--automation-only`.

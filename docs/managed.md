# Managed Mode

Managed mode runs `dari-docs` through the hosted dari.dev Docs service. Use it when you want to test and optimize docs without managing your own dari.dev org or API key.

Under the hood, the tester and editor are ordinary Flue-backed dari.dev agents with prompts, skills, a Flue entrypoint, and a `dari.yml` deploy manifest. Managed mode runs sessions through hosted agents so you can test docs without deploying or operating agent infrastructure.

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

By default, the command submits the run and exits after printing the run ID. Add `--wait` if you want the CLI to block until the run finishes and write feedback files locally.

## Generate proposed edits

Use `optimize` to turn tester feedback into proposed documentation revisions:

```bash
dari-docs optimize . \
  --managed \
  --task "Install the SDK and make a first API call"
```

By default, this submits the optimize run and exits. To wait for completion and download edited files into `.dari-docs/updated/`, add `--wait`:

```bash
dari-docs optimize . \
  --managed \
  --wait \
  --task "Install the SDK and make a first API call"
```

Review `.dari-docs/updated/` and copy changes into your repo when ready. To apply edited docs directly after the run finishes, use `--wait --apply`:

```bash
dari-docs optimize . \
  --managed \
  --wait \
  --task "Install the SDK and make a first API call" \
  --apply
```

## Recover an existing run

After a managed command exits, use the run ID printed by the command:

```bash
dari-docs runs status run_...
dari-docs runs wait run_...
```

`status` prints the current state without waiting. `wait` polls until the run finishes, but does not write files locally. To fetch completed run artifacts from the repo where you want them written:

```bash
dari-docs runs download run_...
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

- `claude-haiku-4-5`
- `claude-sonnet-4-6`
- `claude-opus-4-7`
- `gpt-5-mini`
- `gpt-5.1`
- `gpt-5.5`

By default, managed tester sessions run each task across all three Claude options. The editor uses `claude-sonnet-4-6`.

Use one model for every managed session:

```bash
dari-docs check . \
  --managed \
  --llm claude-opus-4-7 \
  --task "Install the SDK and make a first API call"
```

Or choose the tester and editor models separately:

```bash
dari-docs optimize . \
  --managed \
  --feedback-llm claude-haiku-4-5,claude-opus-4-7 \
  --editor-llm claude-opus-4-7 \
  --task "Install the SDK and make a first API call"
```

For tester sessions, `--feedback-llm` also accepts groups:

- `claude` expands to `claude-haiku-4-5`, `claude-sonnet-4-6`, and `claude-opus-4-7`
- `gpt` expands to `gpt-5-mini`, `gpt-5.1`, and `gpt-5.5`
- `all` expands to all six hosted options

You can mix groups and explicit IDs:

```bash
dari-docs check . \
  --managed \
  --feedback-llm claude,gpt-5.1 \
  --task "Install the SDK and make a first API call"
```

## Log out

Log out with:

```bash
dari-docs auth logout
```

To revoke managed credentials from all devices, run:

```bash
dari-docs auth logout --all
```

You can narrow revocation to browser-login sessions with `--interactive-only` or API keys with `--automation-only`.

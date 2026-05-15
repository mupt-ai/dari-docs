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

Managed runs currently support up to three tasks per run and three active runs per account at a time. Managed runs execute tasks sequentially; use [self-managed mode](self-managed.md) if you need parallel tester sessions.

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

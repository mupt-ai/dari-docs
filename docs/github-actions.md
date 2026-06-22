# GitHub Actions

Run `dari-docs` in CI against the same Modal-deployed Flue agents you use locally. The simplest workflow stores the tester URL as a repository variable and runs `check` on every pull request.

## Check With A Deployed Tester App

```yaml
name: docs-check

on:
  pull_request:
    paths:
      - "**/*.md"
      - "**/*.mdx"
      - "docs/**"

jobs:
  docs-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install dari-docs
        run: |
          curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | bash
          echo "$HOME/.dari-docs/bin" >> "$GITHUB_PATH"

      - name: Run docs check
        run: |
          dari-docs check . \
            --tester-url "$DARI_DOCS_TESTER_URL" \
            --parallel 8 \
            --task "Install the SDK and make a first API call"
        env:
          DARI_DOCS_TESTER_URL: ${{ vars.DARI_DOCS_TESTER_URL }}

      - name: Upload feedback
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: dari-docs-feedback
          path: .dari-docs/
```

A completed tester report is not an automatic docs-quality gate. The job fails when setup, bundling, workflow execution, or download fails. If you want CI to block on content quality, add a follow-up step that inspects `.dari-docs/aggregate-feedback.md` for your own policy.

## Optimize In CI

Use `optimize` when you want CI to produce an artifact with proposed changes:

```yaml
- name: Optimize docs
  run: |
    dari-docs optimize . \
      --tester-url "$DARI_DOCS_TESTER_URL" \
      --editor-url "$DARI_DOCS_EDITOR_URL" \
      --task "Install the SDK and make a first API call"
  env:
    DARI_DOCS_TESTER_URL: ${{ vars.DARI_DOCS_TESTER_URL }}
    DARI_DOCS_EDITOR_URL: ${{ vars.DARI_DOCS_EDITOR_URL }}

- name: Upload proposed docs
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: dari-docs-updated
    path: .dari-docs/updated/
```

Avoid `--apply` in pull-request CI unless your workflow commits changes intentionally.

## Running The Flue App In CI

For quick experiments, you can build and start the tester app inside the job. This is slower than using the Modal deployment, does not fan out across Modal containers, and requires a model provider key in GitHub Secrets.

```yaml
- name: Extract Flue apps
  run: dari-docs init

- uses: oven-sh/setup-bun@v2

- name: Start tester app
  working-directory: .dari-docs/agents/docs-user-tester-agent
  run: |
    bun install --frozen-lockfile
    bun run build
    PORT=8787 ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" bun run start > /tmp/dari-docs-tester.log 2>&1 &
    echo $! > /tmp/dari-docs-tester.pid
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}

- name: Run check against local app
  run: |
    for i in $(seq 1 30); do
      if curl -sS http://127.0.0.1:8787/ >/dev/null 2>&1; then break; fi
      sleep 1
    done
    dari-docs check . \
      --tester-url http://127.0.0.1:8787 \
      --task "Install the SDK"
```

# GitHub Actions

Run `dari-docs` in CI against the same Modal-deployed runner you use locally. The simplest workflow stores the tester URL as a repository variable and the runner token as a repository secret, then runs `check` on every pull request.

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
          DARI_DOCS_RUNNER_TOKEN: ${{ secrets.DARI_DOCS_RUNNER_TOKEN }}

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
    DARI_DOCS_RUNNER_TOKEN: ${{ secrets.DARI_DOCS_RUNNER_TOKEN }}

- name: Upload proposed docs
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: dari-docs-updated
    path: .dari-docs/updated/
```

Avoid `--apply` in pull-request CI unless your workflow commits changes intentionally.

## Running The Runner In CI

For quick experiments, you can start the tester runner inside the job. This example passes `--sandbox docker` so the runner creates task sandboxes with the job's Docker engine instead of Modal. The runner listens on loopback, so it doesn't need a runner token. It needs a model provider key in GitHub Secrets.

```yaml
- name: Extract runner
  run: dari-docs init

- uses: actions/setup-python@v5
  with:
    python-version: "3.12"

- name: Start tester runner
  working-directory: .dari-docs/agents
  run: |
    python -m venv .venv
    . .venv/bin/activate
    pip install -r requirements.txt
    python harbor_runner.py --service tester --port 8000 > /tmp/dari-docs-tester.log 2>&1 &
    echo $! > /tmp/dari-docs-tester.pid
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}

- name: Run check against local runner
  run: |
    for i in $(seq 1 30); do
      if curl -fsS http://127.0.0.1:8000/healthz >/dev/null 2>&1; then break; fi
      sleep 1
    done
    dari-docs check . \
      --tester-url http://127.0.0.1:8000 \
      --sandbox docker \
      --task "Install the SDK"
```

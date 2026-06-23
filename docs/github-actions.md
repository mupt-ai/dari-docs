# GitHub Actions

Managed checks can run in CI with a scoped Dari Docs API key. Add `--wait` so the job waits until the hosted Modal-backed run finishes and writes feedback artifacts.

## Create An API Key

Create the API key locally after logging in:

```bash
dari-docs auth login
dari-docs auth api-key create --name github-actions
```

Add the printed value to your repository or environment secrets as `DARI_DOCS_API_KEY`.

By default, API keys can read managed account/run state and create managed checks. Add scopes explicitly for broader workflows, for example `--scope managed:read --scope managed:optimize` if CI should generate proposed revisions.

## Managed Check

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

      - name: Run managed docs check
        run: |
          dari-docs check . \
            --managed \
            --wait \
            --feedback-llm claude-haiku-4-5 \
            --task "Install the SDK and make a first API call"
        env:
          DARI_DOCS_API_KEY: ${{ secrets.DARI_DOCS_API_KEY }}

      - name: Upload feedback
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: dari-docs-feedback
          path: .dari-docs/
```

A completed tester report is not an automatic docs-quality gate. The job fails when setup, bundling, managed run execution, or download fails. If you want CI to block on content quality, add a follow-up step that inspects `.dari-docs/aggregate-feedback.md` for your own policy.

## Self-Managed Flue Apps

If you operate your own Modal-deployed Flue tester/editor URLs, run the same commands documented in [Flue app setup](self-managed.md) and pass `--tester-url`/`--editor-url` instead of `--managed`.

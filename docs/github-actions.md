# GitHub Actions

Dari Docs can run in CI against the same Flue agents you use locally. The workflow needs a Dari API key and either deployed agent IDs or permission to deploy the bundled agents during the job.

## Simple workflow

This workflow deploys the bundled agents and runs a docs check. The job fails if setup, deploy, upload, session execution, or download fails. A completed tester report is not an automatic docs-quality gate; feedback is written under `.dari-docs/` for humans to review. If you want CI to block on docs quality, add your own follow-up step that inspects `.dari-docs/aggregate-feedback.md` and enforces your threshold.

```yaml
name: Docs Check

on:
  pull_request:

jobs:
  dari-docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install Dari Docs
        run: |
          curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | DARI_DOCS_INSTALL_DIR="$HOME/.local/bin" bash
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"

      - name: Install Dari CLI
        run: |
          curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-cli/main/install.sh | DARI_INSTALL_DIR="$HOME/.local/bin" bash
          echo "$HOME/.local/bin" >> "$GITHUB_PATH"

      - name: Run Docs Check
        env:
          DARI_API_KEY: ${{ secrets.DARI_API_KEY }}
        run: |
          dari-docs init --deploy
          dari-docs check . --task "Install the SDK and make a first API call"
```

Before using this workflow, store the model provider credential that the agents expect in your Dari org. The bundled agents default to a Dari credential named `ANTHROPIC_API_KEY`. The `DARI_API_KEY` secret must be able to deploy agents, upload files, create sessions, read session transcripts, and download session workspaces.

## Reuse already deployed agents

For faster CI, deploy the agents once, save their IDs as repository or environment secrets, and pass the IDs directly:

```yaml
      - name: Run Docs Check
        env:
          DARI_API_KEY: ${{ secrets.DARI_API_KEY }}
          DARI_DOCS_TESTER_AGENT_ID: ${{ secrets.DARI_DOCS_TESTER_AGENT_ID }}
        run: |
          dari-docs check . \
            --feedback-agent "$DARI_DOCS_TESTER_AGENT_ID" \
            --task "Install the SDK and make a first API call"
```

For `optimize`, also pass an editor agent ID:

```bash
dari-docs optimize . \
  --feedback-agent "$DARI_DOCS_TESTER_AGENT_ID" \
  --editor-agent "$DARI_DOCS_EDITOR_AGENT_ID" \
  --task "Install the SDK and make a first API call"
```

`dari-docs` writes feedback and proposed edits under `.dari-docs/`. Upload that directory as an artifact so reviewers can inspect the results from the Actions run.

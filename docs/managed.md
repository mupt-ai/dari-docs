# Flue Agent Setup

This page is kept for older links. Dari Docs now has one supported setup path: deploy the bundled Flue agents to your own dari.dev org and run checks against those agents.

Start here:

```bash
dari auth login
dari credentials add ANTHROPIC_API_KEY # paste your Anthropic key when prompted
export DARI_API_KEY=... # create or copy a dari.dev API key for this org
dari-docs init --deploy

dari-docs check . \
  --task "Install the SDK and make a first API call"
```

For the full flow, see [Flue agent usage](self-managed.md).

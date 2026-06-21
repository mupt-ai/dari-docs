# docs-editor-agent

This is the bundled Flue editor app for `dari-docs`. The HTTP workflow is `.flue/workflows/edit.ts`, exposed as:

```text
POST /workflows/edit?wait=result
```

The workflow receives docs files plus aggregate tester feedback and returns proposed files for the CLI to write under `.dari-docs/updated/`.

## Run Locally

```bash
npm install
npx flue build --target node
PORT=8788 ANTHROPIC_API_KEY=... node dist/server.mjs
```

Use it with a tester app:

```bash
dari-docs optimize . \
  --tester-url http://127.0.0.1:8787 \
  --editor-url http://127.0.0.1:8788 \
  --task "Install the SDK"
```

The default model is `anthropic/claude-sonnet-4-6`. Change it in `agents/docs-editor-agent.ts` before rebuilding, set `DARI_DOCS_DEFAULT_MODEL`, or pass a model in the workflow payload via `dari-docs --llm` / `--editor-llm`.

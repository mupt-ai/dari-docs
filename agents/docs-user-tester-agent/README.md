# docs-user-tester-agent

This is the bundled Flue tester app for `dari-docs`. The HTTP workflow is `.flue/workflows/test.ts`, exposed as:

```text
POST /workflows/test?wait=result
```

The workflow receives a task plus docs files, writes the docs under `input-docs/files/`, and asks the tester agent to try the task in an isolated workspace. It returns structured feedback as Markdown.

## Run Locally

```bash
bun install --frozen-lockfile
bun run build
PORT=8787 ANTHROPIC_API_KEY=... bun run start
```

Then run:

```bash
dari-docs check . \
  --tester-url http://127.0.0.1:8787 \
  --task "Install the SDK"
```

The default model is `anthropic/claude-sonnet-4-6`. Change it in `agents/docs-user-tester-agent.ts` before rebuilding, set `DARI_DOCS_DEFAULT_MODEL`, or pass a model in the workflow payload via `dari-docs --llm` / `--feedback-llm`.

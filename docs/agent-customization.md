# Agent Customization

`dari-docs init` extracts editable Flue projects plus `modal_app.py` into `.dari-docs/agents/`. Customize those projects the same way you would customize any other Flue app, then redeploy the Modal app. The Modal app is a gateway that launches the actual Flue agent inside a fresh Modal Sandbox for each workflow request. In these templates, the agent source, workflow source, prompts, and skills are all visible files in the agent folder; there is no hidden `.flue/` layout to edit.

## Project Layout

```text
.dari-docs/agents/modal_app.py

.dari-docs/agents/docs-user-tester-agent/
  flue.config.ts
  package.json
  bun.lock
  app.ts
  agents/docs-user-tester-agent.ts
  workflows/test.ts
  prompts/system.md
  skills/docs-user-test/SKILL.md

.dari-docs/agents/docs-editor-agent/
  flue.config.ts
  package.json
  bun.lock
  app.ts
  agents/docs-editor-agent.ts
  workflows/edit.ts
  prompts/system.md
  skills/documentation-editor/SKILL.md
```

The tester workflow receives a task and docs files, writes them into the workspace under `input-docs/files/`, and asks the tester agent to try the task. The editor workflow receives aggregate feedback plus docs files and returns proposed files.

The CLI calls the workflows with `POST /workflows/<name>?wait=result`. The tester payload has this shape:

```json
{
  "task": "Install the SDK",
  "model": "gpt-5.5",
  "files": [{ "path": "README.md", "content": "# Docs\n" }],
  "publicDocUrls": ["https://docs.example.com/llms.txt"],
  "liveVerify": false,
  "runtimeSecrets": {}
}
```

The tester returns `{ "feedback": "...markdown..." }`. The editor payload is `{ "files": [...], "feedback": "...markdown...", "model": "claude-sonnet-4-6", "liveVerify": false, "runtimeSecrets": {} }`, and the editor returns `{ "changelog": "...markdown...", "files": [{ "path": "README.md", "content": "...complete file..." }] }`. Paths in editor results should be repo-relative, not workspace paths.

## Change The Model

The bundled entrypoints default to `anthropic/claude-sonnet-4-6`:

```ts
const DEFAULT_MODEL = 'anthropic/claude-sonnet-4-6';
```

Change that constant or add your own environment-based selection. Then redeploy:

```bash
uvx modal deploy .dari-docs/agents/modal_app.py
```

Set the provider key in the Modal secret. The bundled code recognizes the usual provider env vars: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, and `OPENROUTER_API_KEY`. It also accepts per-run model overrides from the workflow payload; short common model IDs are normalized (`claude-*` to `anthropic/claude-*`, `gpt-*` to `openai/gpt-*`).

## Change Prompts Or Skills

Edit `prompts/system.md` to change the persistent agent instructions. Edit the skill files under `skills/` to change task-specific behavior. Keep the workflows focused on input/output plumbing; put judgment and task behavior in prompts and skills so the same app can be redeployed without changing the CLI.

After changes, run a local smoke test before deploying:

```bash
bun run build
PORT=8787 ANTHROPIC_API_KEY=... bun run start
```

Then in another terminal:

```bash
dari-docs check . \
  --tester-url http://127.0.0.1:8787 \
  --task "Follow the quickstart"
```

## Use Different Deployments

`dari-docs` identifies apps by URL, not by agent ID. Pass URLs directly:

```bash
dari-docs check . \
  --tester-url https://staging-tester-url.modal.run \
  --parallel 30 \
  --task "Install the SDK"
```

Or save defaults in `.dari-docs/config.json`:

```bash
dari-docs init \
  --tester-url https://tester-url.modal.run \
  --editor-url https://editor-url.modal.run
```

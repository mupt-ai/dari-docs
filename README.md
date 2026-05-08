# dari-docs

`dari-docs` is a standalone Go CLI that tests and improves documentation with Dari agents.

It includes the agent source templates it needs. On setup, the CLI extracts those agents into your repo and can deploy them into **your Dari org**. LLM configuration happens at agent publish time: use the platform-managed LLM by default, or provide a stored BYOK credential when you run `init --deploy`.

## What it does

```text
local docs directory
  -> bundle docs into .dari-docs/input-docs-bundle.tar.gz
  -> run one lightweight docs-user-tester-agent session per task
  -> aggregate brief user-test feedback
  -> run docs-editor-agent once with original bundle + feedback
  -> download /workspace/updated-docs from the editor session
  -> write updated files locally under .dari-docs/updated/
  -> optionally copy updated files back into the docs directory
```

The tester agent is intentionally simple: it acts like a developer, reads the docs, tries the task in `/workspace/attempt`, and reports what worked or blocked it. It does not produce scorecards.

## Security and credentials

Required for CLI/API access:

- `DARI_API_KEY`: your Dari org API key, used by the local CLI to upload files, create sessions, and download the editor workspace.

LLM configuration is handled by Dari at agent publish time:

- default: omit `llm.api_key_secret` and use platform-managed LLM for the user's org,
- BYOK: create a stored credential and pass `--llm-api-key-secret <NAME>` during `dari-docs init --deploy`.

Runtime product/API keys are opt-in:

```bash
--live-verify --secret-env STRIPE_TEST_SECRET_KEY
```

Those are packed into `DARI_DOCS_RUNTIME_SECRETS_JSON` for the session. Agents are instructed never to print values.

## Build

```bash
go build ./cmd/dari-docs
```

Or install from a public repo once published:

```bash
go install github.com/mupt-ai/dari-docs/cmd/dari-docs@latest
```

## 1. Configure Dari auth

```bash
dari auth login         # needed once so init can create placeholder runtime-secret credential
export DARI_API_KEY=... # Dari org API key for API/session calls
```

## 2. Initialize the repo with platform-managed LLM

Run this from your docs directory:

```bash
dari-docs init --deploy
```

This:

1. extracts bundled agent templates into `.dari-docs/agents/`,
2. creates the placeholder runtime-secret credential required by the agent manifests,
3. deploys the tester and editor agents into your Dari org,
4. writes `.dari-docs/config.json` with the deployed agent IDs.

## 2b. Optional BYOK LLM at publish time

If you want the agents to use your stored LLM credential instead of platform-managed LLM:

```bash
dari credentials add MY_OPENROUTER_KEY
DARI_API_KEY=... dari-docs init --deploy --llm-api-key-secret MY_OPENROUTER_KEY
```

The CLI patches the extracted agent manifests before deploy:

```yaml
llm:
  model: openai/gpt-5.5
  api_key_secret: MY_OPENROUTER_KEY
```

No per-session LLM key is required.

Generated config:

```json
{
  "tester_agent_id": "agt_...",
  "editor_agent_id": "agt_...",
  "agents_dir": ".dari-docs/agents",
  "llm_mode": "platform-managed"
}
```

If you only want to inspect the bundled agents without deploying:

```bash
dari-docs init
```

## 3. Get feedback only

```bash
dari-docs check . \
  --task "Install the CLI and complete the quickstart" \
  --task "Create a session over direct HTTP"
```

Outputs:

```text
.dari-docs/
  input-docs-bundle.tar.gz
  runs/feedback-001.md
  runs/feedback-002.md
  aggregate-feedback.md
```

## 4. Optimize docs and pull updated files

```bash
dari-docs optimize . \
  --task "Install the CLI and complete the quickstart" \
  --task "Create a session over direct HTTP"
```

Outputs:

```text
.dari-docs/
  input-docs-bundle.tar.gz
  runs/feedback-*.md
  aggregate-feedback.md
  editor-output.md
  updated-docs-workspace.zip
  updated/updated-docs/files/
```

The updated docs live at:

```text
.dari-docs/updated/updated-docs/files/
```

Preview the changes:

```bash
diff -ru --exclude='.dari-docs' . .dari-docs/updated/updated-docs/files
```

Apply them to the current docs directory:

```bash
dari-docs optimize . --task "..." --apply
```

or manually:

```bash
rsync -av .dari-docs/updated/updated-docs/files/ ./
```

## Runtime secrets for live verification

Static/local testing is the default. To let tester sessions run safe live checks with product credentials:

```bash
dari-docs optimize . \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --secret-env GITHUB_TOKEN \
  --task "Create a checkout session and verify the response"
```

The CLI sends a JSON object like this as the session-scoped secret `DARI_DOCS_RUNTIME_SECRETS_JSON`:

```json
{
  "STRIPE_TEST_SECRET_KEY": "...",
  "GITHUB_TOKEN": "..."
}
```

Secret values are not printed or written to reports.

## Agent templates included in the CLI package

The Go binary embeds:

```text
internal/agenttemplates/templates/docs-user-tester-agent/
internal/agenttemplates/templates/docs-editor-agent/
```

`dari-docs init` materializes those templates into:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

You can edit those generated agent files and redeploy them manually with `dari deploy .` if needed.

## Notes

- The tester sandbox installs Homebrew/Linuxbrew but intentionally does **not** preinstall Dari. Installing the Dari CLI remains part of the developer experience when the docs ask the user to install it.
- The editor writes updated files into `/workspace/updated-docs/files`; the CLI downloads that directory via `GET /v1/sessions/{session_id}/workspace.zip?path=updated-docs`.
- By default, `optimize` does not overwrite your repo. Use `--apply` or manually copy files after reviewing.

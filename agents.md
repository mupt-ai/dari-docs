# Bundled dari.dev agents

## Code style

Do not add trivial wrapper functions that only forward to another function without adding behavior or clarity. Inline direct calls instead (for example, use `runner.DefaultFeedbackLLMIDs()` directly rather than defining a local `defaultFeedbackLLMIDs()` wrapper).

## UI copy casing

In the web app, user-facing labels, headings, navigation items, button text, empty-state titles, table headings, badges, and short UI actions should start with uppercase letters for each important word (for example, `New Agent`, `Buy Credits`, `API Keys`). Longer explanatory sentences may use normal sentence casing, but must still start with an uppercase letter.

## Local Compose

`compose.yaml` defaults to Docker-assigned localhost ports so multiple worktrees can run in parallel. Discover them with:

```bash
docker compose port backend 8080
docker compose port frontend 5173
```

The agent template folders live at the repo root:

```text
agents/docs-user-tester-agent/dari.yml
agents/docs-editor-agent/dari.yml
```

They are normal Flue-backed dari.dev agent projects: folders with `dari.yml`, `package.json`, `agents/<name>.ts`, prompts, and skills. There is no special `dari-docs` runtime hidden inside them; they are generic Flue projects that can be inspected, edited, versioned, and reused in other contexts. The CLI embeds these folders into the Go binary.

- `docs-user-tester-agent` — lightweight simulated-user testing agent
- `docs-editor-agent` — remote editor agent

`dari-docs init` extracts them into:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

`dari-docs init --deploy` deploys those agents into the user's current dari.dev org and writes their agent IDs to `.dari-docs/config.json`. Once deployed, dari.dev gives each agent an endpoint, which lets `dari-docs` fan out isolated tester and editor sessions without running local agent workers.

## LLM configuration

The Flue templates put model choice in `agents/<name>.ts` and default to `anthropic/claude-sonnet-4-6`. They declare the Dari credential name `ANTHROPIC_API_KEY` under `sandbox.secrets`, so deploys require that credential to exist in the target org before `dari-docs init --deploy`.

For a different stored credential name, pass `--anthropic-api-key-secret NAME` to `dari-docs init --deploy`. The CLI updates the Flue deploy manifest so Dari exposes that stored credential to the Flue process. `--openai-api-key-secret` is available if you edit the Flue agent model to use OpenAI.

Flue agents do not use per-session `--llm`, `--feedback-llm`, or `--editor-llm` selection. Configure the model in the Flue agent project before deploy.

## Runtime product/API secrets

Runtime product/API keys are separate from LLM credentials. `dari-docs --live-verify --secret-env NAME` passes runtime product/API keys directly to sessions as environment variables for that run only.

# Bundled dari.dev agents

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

They are normal dari.dev agent projects: folders with `dari.yml`, prompts, skills, and setup scripts. There is no special `dari-docs` runtime hidden inside them; they are generic agents that can be inspected, edited, versioned, and reused in other contexts. The CLI embeds these folders into the Go binary.

- `docs-user-tester-agent` — lightweight simulated-user testing agent
- `docs-editor-agent` — remote editor agent

`dari-docs init` extracts them into:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

`dari-docs init --deploy` deploys those agents into the user's current dari.dev org and writes their agent IDs to `.dari-docs/config.json`. Once deployed, dari.dev gives each agent a hosted endpoint, which lets `dari-docs` fan out isolated tester and editor sessions without running local agent workers.

## LLM configuration

By default the templates omit `llm.api_key_secret`, so dari.dev uses the platform-managed OpenAI or Anthropic credential for each option. Claude options use `provider: anthropic`, and GPT options use `provider: openai`.

For BYOK at publish time, create provider-specific dari.dev credentials and pass `--anthropic-api-key-secret` and/or `--openai-api-key-secret` to `dari-docs init --deploy`. The CLI sets `api_key_secret` only on matching `llm.options` entries. No per-session LLM key is required by `dari-docs`.

The bundled agents define these LLM option IDs for runtime selection:

- `dumb-claude`
- `medium-claude`
- `smart-claude`
- `dumb-gpt`
- `medium-gpt`
- `smart-gpt`

Self-managed runs use all of these tester LLM options per task by default. Pass one option to all sessions with `--llm`, or override the tester matrix with repeated/comma-separated `--feedback-llm`.

## Runtime product/API secrets

Runtime product/API keys are separate from LLM credentials. Both agents declare:

```yaml
sandbox:
  secrets:
    - DARI_DOCS_RUNTIME_SECRETS_JSON
```

That lets `dari-docs --live-verify --secret-env NAME` pass runtime product/API keys at session creation.

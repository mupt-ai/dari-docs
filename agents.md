# Bundled Dari agents

The agent template folders live at the repo root:

```text
agents/docs-user-tester-agent/dari.yml
agents/docs-editor-agent/dari.yml
```

They are normal Dari agent projects with `dari.yml`, prompts, skills, and setup scripts. The CLI embeds these folders into the Go binary.

- `docs-user-tester-agent` — lightweight simulated-user testing agent
- `docs-editor-agent` — remote editor agent

`dari-docs init` extracts them into:

```text
.dari-docs/agents/docs-user-tester-agent/
.dari-docs/agents/docs-editor-agent/
```

`dari-docs init --deploy` deploys those agents into the user's current Dari org and writes their agent IDs to `.dari-docs/config.json`.

## LLM configuration

By default the templates omit `llm.api_key_secret`, so Dari uses the platform-managed LLM credential for the user's org.

For BYOK at publish time with a single-provider agent configuration, create a Dari credential and pass it during init:

```bash
dari credentials add MY_ANTHROPIC_KEY
DARI_API_KEY=... dari-docs init --deploy --llm-api-key-secret MY_ANTHROPIC_KEY
```

For the bundled mixed Anthropic/OpenAI options, omit this flag to use platform-managed credentials, or deploy custom agents with provider-specific `api_key_secret` values.

The CLI then patches the extracted agents before deploy:

```yaml
llm:
  default: medium-claude
  options:
    medium-claude:
      provider: anthropic
      model: claude-sonnet-4-6
      api_key_secret: MY_ANTHROPIC_KEY
```

No per-session LLM key is required by `dari-docs`.

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

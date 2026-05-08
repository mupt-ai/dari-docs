# Bundled Dari agents

The CLI embeds these agent templates:

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

For BYOK at publish time, create a Dari credential and pass it during init:

```bash
dari credentials add MY_OPENROUTER_KEY
DARI_API_KEY=... dari-docs init --deploy --llm-api-key-secret MY_OPENROUTER_KEY
```

The CLI then patches the extracted agents before deploy:

```yaml
llm:
  model: openai/gpt-5.5
  api_key_secret: MY_OPENROUTER_KEY
```

No per-session LLM key is required by `dari-docs`.

## Runtime product/API secrets

Runtime product/API keys are separate from LLM credentials. Both agents declare:

```yaml
sandbox:
  secrets:
    - DARI_DOCS_RUNTIME_SECRETS_JSON
```

That lets `dari-docs --live-verify --secret-env NAME` pass runtime product/API keys at session creation.

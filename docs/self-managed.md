# Self-Managed Usage

Use self-managed mode when you want runs to execute in your own dari.dev org.

The bundled tester and editor are ordinary Flue-backed dari.dev agents: folders with a `dari.yml`, `package.json`, `agents/<name>.ts`, prompts, and skills. Deploying them to your org gives each agent a hosted endpoint that `dari-docs` can call for tester and editor sessions.

## Set up self-managed mode

Log in with the dari.dev CLI, store the model provider credential the Flue agents use, export your API key, and deploy the bundled agents:

```bash
dari auth login
dari credentials add ANTHROPIC_API_KEY
export DARI_API_KEY=...
dari-docs init --deploy
```

If your org stores the Anthropic key under a different Dari credential name, pass `--anthropic-api-key-secret NAME` to `dari-docs init --deploy`.

Then run the same commands without `--managed`:

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call"

dari-docs optimize . \
  --task "Install the SDK and make a first API call"
```

## Choose model tiers

By default, the bundled Flue agents use `anthropic/claude-sonnet-4-6`. In self-managed Flue mode, model choice lives in the Flue project code and deploy manifest rather than in Dari session `llm_id` options, so `--llm`, `--feedback-llm`, and `--editor-llm` are not supported for self-managed runs yet.

To use a different model, edit `.dari-docs/agents/docs-user-tester-agent/agents/docs-user-tester-agent.ts` and `.dari-docs/agents/docs-editor-agent/agents/docs-editor-agent.ts`, update `sandbox.secrets` if the provider needs a different credential, then redeploy with `dari-docs init --deploy` or `dari deploy` from each agent folder.

Managed mode still uses hosted model-selection flags with the service's configured Claude and GPT options. See [Managed mode and billing](managed.md#model-selection).

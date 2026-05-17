# Self-Managed Usage

Use self-managed mode when you want runs to execute in your own dari.dev org.

The bundled tester and editor are ordinary dari.dev agents: folders of prompts, skills, setup scripts, and a `dari.yml` manifest. Deploying them to your org gives each agent a hosted endpoint that `dari-docs` can call for tester and editor sessions.

## Set up self-managed mode

Log in with the dari.dev CLI, export your API key, and deploy the bundled agents:

```bash
dari auth login
export DARI_API_KEY=...
dari-docs init --deploy
```

Then run the same commands without `--managed`:

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call"

dari-docs optimize . \
  --task "Install the SDK and make a first API call"
```

## Choose model tiers

By default, the bundled agents expose named LLM options such as `dumb-claude`, `medium-claude`, `smart-claude`, `dumb-gpt`, `medium-gpt`, and `smart-gpt`. The Claude options use the `anthropic` provider and the GPT options use the `openai` provider.

In self-managed mode, tester sessions run every task across all six options by default. The CLI creates tester sessions through the Dari session-batch API, in chunks controlled by `--parallel`, and attaches metadata such as `kind`, `task_index`, and `llm_id` so agent webhooks can correlate lifecycle events. The editor uses the manifest default, `medium-claude`.

To explicitly choose tester model tiers:

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call" \
  --feedback-llm dumb-claude,medium-claude,smart-claude
```

Use `--llm ID` to collapse the run to one option for all sessions, or `--editor-llm ID` to select the editor model independently.

If you need BYOK at agent deploy time, add provider-specific dari.dev credentials and pass `--anthropic-api-key-secret` and/or `--openai-api-key-secret` to `dari-docs init --deploy`.

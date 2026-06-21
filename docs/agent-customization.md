# Agent Customization

`dari-docs init` creates local Flue agent projects under:

```text
.dari-docs/agents/
```

Each folder contains a `dari.yml`, `package.json`, a Flue agent entrypoint under `agents/`, prompts, and skills. Flue is the agent runtime used by dari.dev, and a Flue agent is a deployed program that can read inputs, call tools, and work in a remote session workspace. These folders are regular dari.dev Flue projects: you can inspect them, edit them, version them, and deploy them outside `dari-docs` if you want.

## Customize the bundled agents

Run init once to extract the templates:

```bash
dari-docs init
```

Edit the agent folders under `.dari-docs/agents/`, then deploy them:

```bash
export DARI_API_KEY=... # create or copy a dari.dev API key for this org
dari-docs init --deploy
```

The deploy writes the tester and editor agent IDs to `.dari-docs/config.json`. Future `dari-docs check` and `dari-docs optimize` runs use those IDs automatically.

## Change models or provider credentials

The bundled agents configure their model in `agents/<name>.ts` and default to `anthropic/claude-sonnet-4-6`.

The deploy manifest declares model provider credentials under `sandbox.secrets`; by default the credential name is `ANTHROPIC_API_KEY`. Store that credential in your Dari org before deploying:

```bash
dari credentials add ANTHROPIC_API_KEY
```

If your org uses a different stored credential name, pass it during deploy:

```bash
dari-docs init --deploy --anthropic-api-key-secret TEAM_ANTHROPIC_KEY
```

If you edit `.dari-docs/agents/docs-user-tester-agent/agents/docs-user-tester-agent.ts` or `.dari-docs/agents/docs-editor-agent/agents/docs-editor-agent.ts` to use OpenAI, pass the OpenAI credential name too:

```bash
dari-docs init --deploy \
  --anthropic-api-key-secret TEAM_ANTHROPIC_KEY \
  --openai-api-key-secret TEAM_OPENAI_KEY
```

`dari-docs check` and `dari-docs optimize` do not select models with CLI flags. Keep model and credential choices in the Flue project, then redeploy.

## Use your own agents

You can deploy compatible Flue agents yourself and pass their IDs. Compatible agents should follow the same contract as the bundled templates: the tester receives a docs bundle or public docs URL plus one task and returns task-focused feedback; the editor receives source docs plus tester feedback and writes proposed files for download.

Pass custom agent IDs with:

```bash
dari-docs check . \
  --feedback-agent agt_... \
  --task "Install the SDK"

dari-docs optimize . \
  --feedback-agent agt_... \
  --editor-agent agt_... \
  --task "Install the SDK"
```

Use this when you want different prompts, skills, models, or verification behavior than the bundled agents provide.

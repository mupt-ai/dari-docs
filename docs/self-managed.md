# Flue Agent Usage

Dari Docs runs Flue agents in your dari.dev org. Flue is the agent runtime used by dari.dev, and a Flue agent is a deployed program that can read inputs, call tools, and work in a remote session workspace. A dari.dev org is your team workspace/account. The bundled tester and editor agents are regular Flue projects: each folder has a `dari.yml`, `package.json`, TypeScript agent entrypoint, prompts, and skills.

## Set up the agents

From the repo that contains your docs, store the model provider key in your Dari org, export a Dari API key, and deploy the bundled agents. Create or copy the Dari API key from your dari.dev dashboard. If your org uses scoped keys, grant permissions to deploy agents, upload files, create sessions, read session transcripts, and download session workspaces. This setup also requires the `dari` CLI because `dari-docs init --deploy` runs `dari deploy` for the agent projects.

```bash
dari auth login
dari credentials add ANTHROPIC_API_KEY # paste your Anthropic key when prompted
export DARI_API_KEY=... # create or copy a dari.dev API key for this org
dari-docs init --deploy
```

`dari-docs init --deploy` extracts the agent projects into `.dari-docs/agents/`, deploys them with `dari deploy`, and writes their agent IDs to `.dari-docs/config.json`.

If your org stores the Anthropic key under a different Dari credential name, pass it during init:

```bash
dari-docs init --deploy --anthropic-api-key-secret TEAM_ANTHROPIC_KEY
```

## Run checks

Run `check` to have tester agents try one or more tasks from your docs:

```bash
dari-docs check . \
  --task "Install the SDK and make a first API call"
```

Feedback is written to `.dari-docs/aggregate-feedback.md`, with individual reports under `.dari-docs/runs/`. Completed feedback is not a pass/fail score; read it to decide what to fix.

## Generate proposed edits

Run `optimize` to collect tester feedback and ask the editor agent to produce documentation changes:

```bash
dari-docs optimize . \
  --task "Install the SDK and make a first API call"
```

Proposed changes are downloaded into `.dari-docs/updated/`. Review that directory and copy changes into your repo when ready, or let the CLI apply them after download. With `--apply`, regular files from `.dari-docs/updated/` are copied into your repo and existing files at the same paths are overwritten. It does not apply a patch or delete files that are absent from the update.

```bash
dari-docs optimize . \
  --apply \
  --task "Install the SDK and make a first API call"
```

## Use existing Flue agents

If you have already deployed compatible Flue agents, pass their IDs directly:

```bash
dari-docs check . \
  --feedback-agent agt_... \
  --task "Install the SDK"

dari-docs optimize . \
  --feedback-agent agt_... \
  --editor-agent agt_... \
  --task "Install the SDK"
```

Compatible agents should follow the same contract as the bundled templates: the tester receives a docs bundle or public docs URL plus one task and returns task-focused feedback; the editor receives source docs plus tester feedback and writes proposed files for download.

## Choose models and credentials

Model choice lives in the Flue agent project. The bundled agents default to `anthropic/claude-sonnet-4-6` in `agents/<name>.ts` and declare the Dari credential name `ANTHROPIC_API_KEY` in `dari.yml`.

To change models, edit the agent TypeScript entrypoints and update `dari.yml` if the provider needs a different stored credential. Then redeploy with `dari-docs init --deploy` or run `dari deploy` from each agent folder.

`dari-docs check` and `dari-docs optimize` do not choose per-session LLMs for Flue agents. Keep model configuration with the agent code so deployed runs are predictable.

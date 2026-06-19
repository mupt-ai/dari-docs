# Agent Customization

`dari-docs init` creates local agent projects under:

```text
.dari-docs/agents/
```

These are regular Flue-backed dari.dev agent projects. Each folder contains a small `dari.yml`, a `package.json`, a Flue agent entrypoint under `agents/`, prompts, and skills. The same agent folder can be inspected, edited, versioned, and reused outside `dari-docs`; deploying it to your dari.dev org gives it a hosted endpoint that can run many isolated sessions without you managing the runtime infrastructure.

The bundled agents use Flue's Node runtime and configure their model in `agents/<name>.ts`. The deploy manifest declares provider credentials under `sandbox.secrets`; by default that is `ANTHROPIC_API_KEY`. Store that credential in your Dari org before deploying, or pass `--anthropic-api-key-secret NAME` to use a different stored credential name.

Managed mode uses the hosted Dari Docs tester and editor agents automatically and does not deploy customized agents into the managed service account.

Use self-managed mode for customized agents. `dari-docs init --deploy` deploys the default local agents into your org. For more control, deploy your own dari.dev agents and pass their IDs with `--feedback-agent` and `--editor-agent`.

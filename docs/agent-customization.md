# Agent Customization

`dari-docs init` creates local agent projects under:

```text
.dari-docs/agents/
```

These are regular dari.dev agent projects. A dari.dev agent is just a folder with prompts, skills, optional setup scripts, and a `dari.yml` manifest. The same agent folder can be inspected, edited, versioned, and reused outside `dari-docs`; deploying it to dari.dev gives it a hosted endpoint that can run many isolated sessions without you managing the runtime infrastructure.

The bundled tester agent enables sandbox internet access by default so it can install packages and try docs that call external services. You can turn this off in `.dari-docs/agents/docs-user-tester-agent/dari.yml` before deploying if you want tests to run without network access.

For customized agents, network access is controlled in each agent's `dari.yml`:

```yaml
sandbox:
  internet_access: true
```

In managed mode, deploy the edited agents with:

```bash
dari-docs agents deploy --managed
```

In self-managed mode, `dari-docs init --deploy` deploys the default local agents. For more control, deploy your own dari.dev agents and pass their IDs with `--feedback-agent` and `--editor-agent`.

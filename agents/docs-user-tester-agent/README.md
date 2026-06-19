# docs-user-tester-agent

A lightweight dari.dev/Flue agent that simulates a developer using supplied docs to complete one task.

It is intentionally not a formal docs auditor. It reads the supplied docs context, tries the task in an `attempt/` workspace directory, runs the smallest safe verification it can, then returns brief user-style feedback.

Used by the `dari-docs` CLI as the fanout testing agent. The Flue entrypoint is `agents/docs-user-tester-agent.ts`; it defaults to `anthropic/claude-sonnet-4-6` and expects a Dari credential exposed as `ANTHROPIC_API_KEY` unless you edit `dari.yml`/the agent code or run `dari-docs init --deploy --anthropic-api-key-secret NAME`.

## Deploy

```bash
dari credentials add ANTHROPIC_API_KEY
dari deploy .
```

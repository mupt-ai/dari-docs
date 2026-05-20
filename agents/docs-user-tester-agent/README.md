# docs-user-tester-agent

A lightweight dari.dev/Pi agent that simulates a developer using supplied docs to complete one task.

It is intentionally not a formal docs auditor. It reads the attached docs bundle, tries the task in `/workspace/attempt`, runs the smallest safe verification it can, then returns brief user-style feedback.

Used by the `dari-docs` CLI as the fanout testing agent. The manifest exposes named LLM options (`claude-haiku-4-5`, `claude-sonnet-4-6`, `claude-opus-4-7`, `gpt-5-mini`, `gpt-5.1`, `gpt-5.5`) so the CLI runs each task under all bundled model tiers by default, or under an explicit repeated/comma-separated `--feedback-llm` matrix.

## Deploy

```bash
dari deploy .
```

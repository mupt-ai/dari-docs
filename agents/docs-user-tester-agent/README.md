# docs-user-tester-agent

A lightweight Dari/Pi agent that simulates a developer using supplied docs to complete one task.

It is intentionally not a formal docs auditor. It reads the attached docs bundle, tries the task in `/workspace/attempt`, runs the smallest safe verification it can, then returns brief user-style feedback.

Used by the `dari-docs` CLI as the fanout testing agent.

## Deploy

```bash
dari deploy .
```

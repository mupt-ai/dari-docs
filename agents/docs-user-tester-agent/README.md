# docs-user-tester-agent

A lightweight Dari/Pi agent that simulates a developer using supplied docs to complete one task.

It is intentionally not a formal docs auditor. It reads the attached docs bundle, tries the task in `/workspace/attempt`, runs the smallest safe verification it can, then returns brief user-style feedback.

The sandbox setup installs Homebrew for Linux, but intentionally does **not** install Dari. Installing the Dari CLI remains part of the docs/developer experience under test, for example `brew install mupt-ai/tap/dari`.

Used by the `dari-docs` CLI as the fanout testing agent.

## Deploy

```bash
dari deploy .
```

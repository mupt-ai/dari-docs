# dari-docs

> Make your docs so good even the dumbest agent can ship.

`dari-docs` is a CLI for testing whether your documentation is clear enough for agents to use. It sends your docs to simulated developer agents, asks them to complete real tasks, reports where they get stuck, and can generate proposed docs edits from that feedback.

Use it to turn documentation quality from “seems understandable” into “an agent can actually complete the task.”

## Why dari-docs?

Good docs used to mean “a developer can eventually figure this out.” That is no longer enough.

When the reader is an agent, ambiguity becomes measurable. Inconsistent terminology, hidden assumptions, scattered context, and missing setup steps all increase the chance that the agent fails the task or wastes context trying to infer what the docs meant.

`dari-docs` gives you a repeatable feedback loop for agent-readable documentation: define the task, run simulated users, inspect failures, and optionally pull back edited docs.

## What it does

- **Tests docs with simulated developers** — agents attempt concrete tasks using only the docs you provide.
- **Finds task-blocking ambiguity** — reports missing context, unclear setup, inconsistent terms, and places where the agent had to guess.
- **Generates proposed fixes** — `optimize` turns tester feedback into edited documentation you can review locally.
- **Runs managed or self-managed** — use the hosted dari.dev Docs service, or run against agents in your own dari.dev org.
- **Uses normal agent projects** — the tester and editor are just folders of prompts, skills, setup scripts, and a `dari.yml` manifest.

## Install

Install with Homebrew:

```bash
brew install mupt-ai/tap/dari-docs
dari-docs --help
```

Install with Go:

```bash
go install github.com/mupt-ai/dari-docs/cmd/dari-docs@latest
dari-docs --help
```

Or build from this repo:

```bash
go build ./cmd/dari-docs
./dari-docs --help
```

## Quickstart

Managed mode uses the hosted dari.dev Docs service and a separate dari.dev Docs credit balance. New accounts start with five dollars worth of free credits.

From your docs repo:

```bash
dari-docs auth login
dari-docs init
dari-docs agents deploy --managed
```

Run a docs check:

```bash
dari-docs check . \
  --managed \
  --task "Install the SDK and make a first API call"
```

Generate proposed docs edits:

```bash
dari-docs optimize . \
  --managed \
  --task "Install the SDK and make a first API call"
```

The edited files are downloaded into `.dari-docs/updated/` without changing your repo. Add `--apply` if you want `dari-docs` to apply the revisions directly.

## How it works

1. You point `dari-docs` at a docs directory and give it one or more tasks.
2. The CLI bundles your docs and sends them to hosted developer-agent endpoints.
3. Tester agents try to complete the task and report where the docs blocked progress.
4. `dari-docs` summarizes the feedback into local run artifacts.
5. If you run `optimize`, an editor agent proposes documentation changes.
6. Proposed edits are written to `.dari-docs/updated/` for review.

The simulated users are plain dari.dev agents. A dari.dev agent is not a special binary or hidden service; it is a portable folder containing prompts, skills, setup scripts, and `dari.yml`. Deploying that folder to dari.dev gives it a hosted endpoint, so `dari-docs` can fan out isolated tester and editor sessions without you running agent workers yourself.

## Managed vs self-managed

| Mode | Use when | Requires |
| --- | --- | --- |
| Managed | You want the fastest setup and hosted execution. | `dari-docs auth login` |
| Self-managed | You want runs in your own dari.dev org. | A dari.dev API key and deployed agents |

Most users should start with managed mode.

## Documentation

- [Managed mode, billing, and deployment](docs/managed.md)
- [GitHub Actions](docs/github-actions.md)
- [Task files and repeated checks](docs/tasks.md)
- [Bundle selection](docs/bundle-selection.md)
- [Live verification secrets](docs/live-verification.md)
- [Agent customization](docs/agent-customization.md)
- [Self-managed usage](docs/self-managed.md)

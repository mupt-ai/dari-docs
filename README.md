<p align="center">
  <img src="docs/media/dari-logo.svg" alt="Dari Docs logo" width="72">
</p>

<h1 align="center">Dari Docs</h1>

<p align="center">
  <b>Find out whether a docs change helps coding agents get real work done.</b><br>
  A GitHub bot on <a href="https://selfbench.dev">SelfBench</a>: every pull request that changes your docs gets agents running your tasks with the old docs and with the new ones.
</p>

<p align="center">
  <a href="https://github.com/mupt-ai/dari-docs/actions/workflows/pr-checks.yml"><img src="https://github.com/mupt-ai/dari-docs/actions/workflows/pr-checks.yml/badge.svg" alt="PR Checks"></a>
  <a href="https://github.com/mupt-ai/dari-docs/releases"><img src="https://img.shields.io/github/v/release/mupt-ai/dari-docs?color=0f9d76" alt="Latest release"></a>
</p>

A quickstart can read fine and still send an agent the wrong way. Dari Docs measures it. On each pull request that changes docs, SelfBench builds your tasks twice, once with the changed docs as they were and once as the pull request has them, runs the models you pick on both, and the bot comments with what changed.

What a comment looks like (example content):

> ### dari-docs: ⚠️ The docs change made some tasks fail
>
> Agents ran 6 tasks with the docs as they were before this change and as of `3f9c2e1`. Only the changed docs differ between the two runs.
>
> | Model · Harness | Before | After | Change | Cost before → after |
> | --- | --- | --- | --- | --- |
> | Claude Sonnet 5.5 · claude-code | 4/6 | 4/6 | 0 | $1.84 → $1.79 |
>
> **Changed results**
>
> - ❌ now fails `scn-91f/first-api-call` on Claude Sonnet 5.5 · claude-code: the agent followed the new install step, which names a package that doesn't exist.
> - ✅ now passes `scn-77a/verify-webhook` on Claude Sonnet 5.5 · claude-code

The comment also lists the tasks that fail with both versions, with SelfBench's explanation of each failure: what the docs still don't get across.

## Get started

You need a [SelfBench](https://app.selfbench.dev) workspace with your repository connected, and the [`self-bench` CLI](https://docs.selfbench.dev/quickstart) signed in (`self-bench login`).

**1. Install**

```bash
curl -fsSL https://github.com/mupt-ai/dari-docs/releases/latest/download/install.sh | bash
# or: go install github.com/mupt-ai/dari-docs/cmd/dari-docs@latest
```

**2. Set up the repository.** From its root:

```bash
dari-docs init
```

`init` uses your `self-bench` login, checks the repository is connected, picks a model from SelfBench's catalog and the workspace's credentials to run it on, and writes `.dari-docs/config.json` and `.github/workflows/dari-docs.yml`.

**3. Add tasks.** A task is something a reader of your docs should be able to do. SelfBench turns each one into a task with tests:

```bash
dari-docs tasks add "Install the SDK and make a first authenticated API call"
dari-docs tasks add --file docs-tasks.txt   # one task per paragraph or bullet
```

Each new task is added to `tasks` in the config. SelfBench takes a while to write and check them; approve each one in the app when it's ready. `dari-docs tasks list` shows where they are.

**4. Give the workflow a key** (SelfBench → Settings → API Keys), then commit the two files:

```bash
gh secret set SELFBENCH_API_KEY --repo OWNER/NAME
```

The next pull request that changes docs gets a comment.

## How it works

1. **Find the docs change**: the paths matching `docs` in the config that differ between the pull request's merge base and its head. Only the pull request's own change counts, not whatever landed on main since.
2. **Build both versions** as two SelfBench [repo ablations](https://docs.selfbench.dev/cli/environments). One puts the changed files back as they were; the other sets them as the pull request has them. Every other file stays as each task has it, so the docs change is the only difference.
3. **Run** a comparison: each model on each task that built in both versions, under both. The tasks as originally authored aren't run.
4. **Report** in one comment, updated as the check goes: building, trials finished, then the result.

Pushing again reuses builds for docs that didn't change and stops the previous push's comparison. Rerunning a job on the same commit reads the finished comparison instead of running it again. Closing the pull request stops its comparison and deletes its ablations.

A pull request that changes code and docs is checked on its docs alone: tasks keep their own code.

More: [how a check works](docs/how-it-works.md) · [configuration](docs/configuration.md)

## Tasks

Checks run the tasks listed in `tasks` in the config, once they're approved. With no list, they run every approved task in the repository, including the coding tasks SelfBench builds from merged pull requests, which docs rarely affect.

| Command | Does |
| --- | --- |
| `dari-docs tasks add "…"` | Writes one task from a scenario; `--title` names it |
| `dari-docs tasks add --file F` | Writes one per paragraph or bullet; older dari-docs task files work as they are |
| `dari-docs tasks list` | Every task in the repository, its state, and whether checks run it |

## Commands

| Command | Does |
| --- | --- |
| `dari-docs init` | Writes the config and workflow |
| `dari-docs check` | Runs a check. In a pull request job it reads the pull request from the event; elsewhere, `--base` and `--head` (default `origin/main` and `HEAD`). `--dry-run` prints the changed docs and both ablation scripts without calling SelfBench |
| `dari-docs cleanup --pr N` | Stops a pull request's comparison and deletes its ablations |
| `dari-docs tasks list` · `tasks add` | See [Tasks](#tasks) |

## Cost and limits

- **Model and sandbox usage** is billed to your SelfBench workspace: its managed models, or your own credentials (`credentialId`, `sandboxCredentialId`).
- **Each check builds every approved task twice**, not only the ones in `tasks`, because a repo ablation builds a repository's whole approved set. Builds are reused when the docs didn't change.
- **One trial per task per side.** Agents vary, so a single flip can be noise. `failOnRegression` (off by default) fails the job when a task goes from passing to failing.
- **The workflow waits** for builds and trials, up to `timeoutMinutes` (240 by default).
- **A docs change over about 64 KB** compressed is refused; narrow `docs` or split the pull request.
- **Pull requests from forks** get no secrets, so they're skipped.

## Self-hosted SelfBench

Set `apiUrl` in the config, or `selfbench-api-url` on the action, to your deployment.

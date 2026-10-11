<p align="center">
  <img src="web/public/dari-logo.svg" alt="Dari Docs logo" width="72">
</p>

<h1 align="center">Dari Docs</h1>

<p align="center">
  <b>Find out if your docs work.</b><br>
  Have an agent try real developer tasks from your documentation, then see where it gets stuck.
</p>

<p align="center">
  <a href="docs/self-managed.md">Setup</a> ·
  <a href="docs/tasks.md">Tasks</a> ·
  <a href="docs/github-actions.md">GitHub Actions</a> ·
  <a href="docs/agent-customization.md">Customize agents</a>
</p>

<p align="center">
  <a href="https://github.com/mupt-ai/dari-docs/actions/workflows/pr-checks.yml"><img src="https://github.com/mupt-ai/dari-docs/actions/workflows/pr-checks.yml/badge.svg" alt="PR Checks"></a>
</p>

`dari-docs` bundles selected documentation and sends it with your task to a tester agent that tries to follow the instructions in a workspace. The CLI talks to tester and editor services over HTTP; the bundled deployment uses Flue for the agent runtime and Modal for sandboxed execution. You deploy these services yourself.

## How it works

1. **Choose a task.** Describe something a developer should be able to do using your docs, such as installing an SDK and making a first API call.
2. **Let the tester try it.** `dari-docs` sends the task and selected docs to a tester agent, which works in a sandbox and reports what it tried and where it got stuck.
3. **Improve the docs.** Optionally, run an editor agent to propose changes based on the tester’s feedback.

A successful check means the tester workflows completed—it is **not** a pass/fail score for your documentation. If you want CI to block on feedback, add your own policy step to interpret the report.

## Get started

The bundled deployment runs agents on Modal. You’ll need [`uvx`](https://docs.astral.sh/uv/), a Modal account, and model-provider credentials for the model you use. The CLI uses service URLs, so you can also connect it to your own compatible tester and editor services.

1. **Install the CLI** (macOS or Linux, x86_64 or arm64):

   ```bash
   curl -fsSL https://raw.githubusercontent.com/mupt-ai/dari-docs/main/install.sh | bash
   ```

2. **Initialize the agents** from the repository containing your docs:

   ```bash
   dari-docs init
   ```

3. **Configure model credentials and deploy.** Include only the providers your selected models need. This example uses Anthropic and OpenAI credentials; the bundled tester defaults to `anthropic/claude-sonnet-4-6`.

   ```bash
   uvx modal setup
   uvx modal secret create dari-docs-model-providers \
     ANTHROPIC_API_KEY=... \
     OPENAI_API_KEY=...
   uvx modal deploy .dari-docs/agents/modal_app.py
   ```

   Modal prints two HTTPS URLs: one labeled `tester`, the other `editor`. `check` needs the tester URL; `optimize` needs both. See [agent setup](docs/self-managed.md#parallel-runs-and-model-configuration) for model overrides.

4. **Run a check** with the tester URL and a concrete task:

   ```bash
   dari-docs check . \
     --tester-url <tester-url-from-modal> \
     --task "Install the SDK and make a first API call"
   ```

   The aggregate report is written to `.dari-docs/aggregate-feedback.md`; individual task/model reports go in `.dari-docs/runs/`. They describe what the tester tried and where it had trouble. The input bundle is kept at `.dari-docs/input-docs-bundle.tar.gz` for inspection.

> [!IMPORTANT]
> The agent receives the bundled documentation and can run shell commands in its workspace. Review the bundle before sending sensitive repositories, and protect deployed endpoints. See [bundle selection](docs/bundle-selection.md) and the [deployment guide](docs/self-managed.md).

## Check published docs

Use `--docs-url` to test public documentation instead of a local checkout. Public URL checks require internet access and work with `check` only. For example, `llms.txt` is a plain-text or Markdown index that links to documentation pages:

```bash
dari-docs check \
  --tester-url <tester-url-from-modal> \
  --docs-url https://docs.example.com/llms.txt \
  --task "Create a browser session"
```

For local docs, `--bundle-include` adds files to the default selection and `--bundle-exclude` excludes paths. Exclusions take precedence over both defaults and include patterns.

## Propose documentation edits

`optimize` runs a fresh tester pass, then asks the editor agent to propose changes under `.dari-docs/updated/`:

```bash
dari-docs optimize . \
  --tester-url <tester-url-from-modal> \
  --editor-url <editor-url-from-modal> \
  --task "Install the SDK and make a first API call"
```

Review the proposed files before applying them. With `--apply`, files are copied by repository-relative path: matching files are overwritten, new files are created, and files are not deleted.

```bash
dari-docs optimize . \
  --tester-url <tester-url-from-modal> \
  --editor-url <editor-url-from-modal> \
  --task "Install the SDK and make a first API call" \
  --apply
```

Each optimize run replaces the previous contents of `.dari-docs/updated/`. Use `--out` to keep outputs from separate runs in different directories.

## Repeat tasks and compare models

Pass `--task` more than once, or put tasks in a file with `--tasks-file` (one task per paragraph or bullet). Tester concurrency is one workflow at a time by default; use `--parallel` to run more concurrently. This example tries two tasks with two models:

```bash
dari-docs check . \
  --tester-url <tester-url-from-modal> \
  --parallel 4 \
  --feedback-llm claude-sonnet-4-6 \
  --feedback-llm gpt-5.5 \
  --task "Install the SDK" \
  --task "Configure retries"
```

The bundled Modal template starts a temporary Modal Sandbox for each workflow request and terminates it when the result returns. Model-provider credentials belong in the Modal secret, not in the docs bundle. See [task files and repeated checks](docs/tasks.md) for batching, timeouts, and model groups.

## Configuration and safety

- **Save deployment URLs:** `dari-docs init --tester-url <tester-url> --editor-url <editor-url>` stores the URLs in `.dari-docs/config.json`, so later commands can omit the URL flags. Do this during initial setup if possible: running `dari-docs init` again re-extracts the bundled agent files and can overwrite your edits to those templates.
- **Review the bundle:** defaults include docs and docs-adjacent formats such as Markdown, JSON, YAML, TOML, CSS, JavaScript, and TypeScript; common generated directories and files over 5 MB are skipped. Secrets committed to files matching the bundle rules are still included unless excluded.
- **Runtime credentials are opt-in:** environment variables are not bundled. `--live-verify --secret-env NAME` explicitly sends the named runtime product/API credential to the workflow for live verification. The agent can use it in shell commands, SDK calls, or API requests; use test-mode, read-only, or otherwise safe credentials. Do not pass model-provider keys this way.
- **Protect deployed URLs:** the Modal endpoints run agents that can execute shell commands. Keep them private or add appropriate access controls, especially when using sensitive docs or credentials.

## Documentation

**Setup:** [Deploy the bundled agents](docs/self-managed.md) · [Local development](docs/local-development.md) · [Agent customization](docs/agent-customization.md)

**Workflows:** [Task files and repeated checks](docs/tasks.md) · [GitHub Actions](docs/github-actions.md)

**Data and credentials:** [Bundle selection](docs/bundle-selection.md) · [Live verification and runtime secrets](docs/live-verification.md)

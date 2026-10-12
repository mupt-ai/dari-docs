# Harness, Model, And Sandbox Selection

`dari-docs init` extracts the runner into `.dari-docs/agents/`:

```text
.dari-docs/agents/modal_app.py
.dari-docs/agents/harbor_runner.py
.dari-docs/agents/requirements.txt
```

`harbor_runner.py` runs each workflow request as a [Harbor](https://docs.harborframework.com) trial. Most choices are per-run CLI flags, so you can try other harnesses, models, and sandboxes without editing or redeploying the runner.

## Defaults

| Setting | Default |
| --- | --- |
| Harness | `claude-code` |
| Model | `anthropic/claude-sonnet-4-6` |
| Sandbox | `modal` |

## Choose A Harness

A harness is the Harbor agent that runs in the sandbox. Use `--harness` with a pre-integrated Harbor agent name, such as `claude-code` or `codex`. `--harness` applies to the tester and the editor; use `--editor-harness` to choose a different editor harness for `optimize`.

```bash
dari-docs optimize . \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run \
  --harness codex \
  --feedback-llm openai/gpt-5.5 \
  --editor-harness claude-code \
  --editor-llm anthropic/claude-sonnet-4-6 \
  --task "Install the SDK"
```

Choose a model the harness supports with `--llm`, `--feedback-llm`, and `--editor-llm`. Use the `provider/model` form Harbor expects, such as `anthropic/claude-sonnet-4-6`. The runner environment needs the provider key that harness reads, for example `ANTHROPIC_API_KEY` for Claude Code or `OPENAI_API_KEY` for Codex.

See Harbor's [pre-integrated agents](https://docs.harborframework.com/agents/pre-integrated-agents) for available harnesses.

## Pass Harness Options

Use `--agent-kwarg key=value` to pass constructor options to the Harbor agent, and `--agent-env NAME=value` to set environment variables for the agent phase. Both flags are repeatable.

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --agent-kwarg max_turns=60 \
  --agent-env MCP_TIMEOUT=20000 \
  --task "Install the SDK"
```

Valid keys depend on the harness; check its Harbor adapter. `--agent-env` values are sent in plain text in the workflow payload. Use it only for non-secret settings. Model provider keys belong in the runner environment, and product/API credentials belong in [`--secret-env`](live-verification.md).

## Choose A Sandbox

Use `--sandbox` with a Harbor environment type, such as `modal` or `docker`, and `--sandbox-kwarg key=value` (repeatable) for its constructor options:

```bash
dari-docs check . \
  --tester-url http://127.0.0.1:8000 \
  --sandbox docker \
  --task "Install the SDK"
```

The runner process needs the sandbox provider's SDK and credentials. `requirements.txt` installs `harbor[modal]`; add the matching Harbor extra or SDK for other providers. Keep sandbox provider credentials in the runner environment, not in `--agent-env`. A `docker` sandbox needs a Docker daemon available to the runner, so it suits local runs rather than the Modal deployment.

See Harbor's [pre-integrated sandboxes](https://docs.harborframework.com/sandboxes/pre-integrated-sandboxes) for available types.

## Use A Custom Harbor Adapter

`--harness`, `--editor-harness`, and `--sandbox` also accept a Harbor import path in `module:Class` form:

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --harness my_agents.docs_agent:DocsAgent \
  --sandbox my_sandboxes.vm:MySandbox \
  --task "Install the SDK"
```

The module must be importable by the runner process. Add it next to `harbor_runner.py` or install it from `requirements.txt`, then redeploy. Agent adapters extend Harbor's `BaseInstalledAgent` or `BaseAgent`, and sandbox adapters extend `BaseEnvironment`. See Harbor's guides to [custom agents](https://docs.harborframework.com/agents/custom-agents) and [custom sandboxes](https://docs.harborframework.com/sandboxes/custom-sandboxes).

## Compatibility

Harbor supports many harnesses, models, and sandboxes, but `dari-docs` does not promise that every combination works. A harness must be able to follow the tester or editor instructions and write the expected result files, the model must work with that harness, and the sandbox must support what the harness installs and runs. The default Claude Code + `anthropic/claude-sonnet-4-6` + Modal combination is the tested path. Run a small check before using another combination widely.

## Workflow Contract

The CLI calls the runner with `POST /workflows/<name>?wait=result`. The tester payload includes the task, model, docs files, public docs URLs, and live-verify settings:

```json
{
  "task": "Install the SDK",
  "model": "openai/gpt-5.5",
  "files": [{ "path": "README.md", "content": "# Docs\n" }],
  "publicDocUrls": ["https://docs.example.com/llms.txt"],
  "liveVerify": false,
  "runtimeSecrets": {}
}
```

Harness and sandbox selections are sent in the same payload. The tester returns `{ "feedback": "...markdown..." }`, read from the agent's `feedback.md`. The editor payload is `{ "files": [...], "feedback": "...markdown...", "model": "anthropic/claude-sonnet-4-6", "liveVerify": false, "runtimeSecrets": {} }`, and the editor returns `{ "changelog": "...markdown...", "files": [{ "path": "README.md", "content": "...complete file..." }] }`, read from `changelog.md` and `updated-docs/files/`. The runner validates editor output on the trusted host before returning it; file paths are repo-relative.

## Use Different Deployments

`dari-docs` identifies services by URL. Pass URLs directly:

```bash
dari-docs check . \
  --tester-url https://staging-tester-url.modal.run \
  --parallel 30 \
  --task "Install the SDK"
```

Or save defaults in `.dari-docs/config.json`:

```bash
dari-docs init \
  --tester-url https://tester-url.modal.run \
  --editor-url https://editor-url.modal.run
```

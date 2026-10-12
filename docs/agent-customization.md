# Harness, Model, And Sandbox Selection

`dari-docs init` extracts the [Harbor](https://docs.harborframework.com) runner into `.dari-docs/agents/` (see [runner setup](self-managed.md#initialize) for the full layout). The runner runs each workflow request as a Harbor trial. Harness, model, and sandbox are per-run CLI flags, so you can try other combinations without editing or redeploying the runner.

## Defaults

| Setting | Default | Change the runner default with |
| --- | --- | --- |
| Harness | `claude-code` | `DARI_DOCS_DEFAULT_HARNESS` |
| Model | `anthropic/claude-sonnet-4-6` | `DARI_DOCS_DEFAULT_MODEL` |
| Sandbox | `modal` | `DARI_DOCS_DEFAULT_SANDBOX` |

Set the runner defaults in the Modal secret or the local runner's environment.

## Choose A Harness

A harness is the Harbor agent that runs in the sandbox. Use `--harness` with a pre-integrated Harbor agent name, such as `claude-code` or `codex`. `--harness` applies to the tester and the editor; `--editor-harness` overrides it for the editor in `optimize`.

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

Choose a model the harness supports with `--llm`, `--feedback-llm`, and `--editor-llm`. Models use Harbor's `provider/model` form, such as `anthropic/claude-sonnet-4-6`. Short IDs are normalized: `claude-*` becomes `anthropic/claude-*`, and `gpt-*` and `o<N>*` become `openai/...`. The runner environment needs the provider key that harness reads, for example `ANTHROPIC_API_KEY` for Claude Code or `OPENAI_API_KEY` for Codex.

See Harbor's [pre-integrated agents](https://docs.harborframework.com/agents/pre-integrated-agents) for available harnesses.

## Pass Harness Options

Use `--agent-kwarg KEY=VALUE` to pass constructor options to the Harbor agent, and `--agent-env NAME=VALUE` to set non-secret environment variables for the agent. Both flags are repeatable. Kwarg values are parsed as JSON when possible, so numbers, booleans, lists, and objects keep their types; anything else is sent as a string.

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --agent-kwarg max_turns=60 \
  --agent-env MCP_TIMEOUT=20000 \
  --task "Install the SDK"
```

Valid kwargs depend on the harness; check its Harbor adapter.

`--agent-env` is only for non-secret settings, because values are sent in plain text in the workflow payload. The CLI and the runner reject names that contain `KEY`, `TOKEN`, `SECRET`, `PASSWORD`, `CREDENTIAL`, or `AUTH`, in any case. Put model provider keys in the runner environment and product/API credentials in [`--secret-env`](live-verification.md).

## Choose A Sandbox

Use `--sandbox` with a Harbor environment type, such as `modal` or `docker`, and `--sandbox-kwarg KEY=VALUE` (repeatable) for its constructor options:

```bash
dari-docs check . \
  --tester-url http://127.0.0.1:8000 \
  --sandbox docker \
  --task "Install the SDK"
```

The runner sets `app_name` and `sandbox_timeout_secs` for the `modal` sandbox unless you override them with `--sandbox-kwarg`.

The runner process needs the sandbox provider's SDK and credentials. `requirements.txt` installs `harbor[modal]`; add the matching Harbor extra or SDK for other providers and redeploy. Keep sandbox provider credentials in the runner environment, never in `--agent-env`. A `docker` sandbox needs a Docker engine on the runner's host, so it suits a local runner rather than the Modal deployment.

The task sandbox uses `python:3.13-slim-bookworm`. Set `DARI_DOCS_WORKSPACE_IMAGE` on the runner to use another registry image.

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

The module must be importable by the runner. On Modal, the whole `.dari-docs/agents/` directory is copied into the image and is on `PYTHONPATH`, so you can add the module there and redeploy. Add any dependencies to `requirements.txt`. Agent adapters extend Harbor's `BaseInstalledAgent` or `BaseAgent`, and sandbox adapters extend `BaseEnvironment`. See Harbor's guides to [custom agents](https://docs.harborframework.com/agents/custom-agents) and [custom sandboxes](https://docs.harborframework.com/sandboxes/custom-sandboxes).

## Compatibility

Harbor supports many harnesses, models, and sandboxes, but `dari-docs` doesn't promise that every combination works. The harness must follow the instructions and write the expected output files, the model must work with that harness, and the sandbox must support what the harness installs and runs. The default Claude Code, `anthropic/claude-sonnet-4-6`, and Modal combination is the tested path. Run a small check before you rely on another combination.

## Change Prompts Or Skills

Each workflow has editable instructions:

```text
.dari-docs/agents/docs-user-tester-agent/prompts/system.md
.dari-docs/agents/docs-user-tester-agent/skills/docs-user-test/SKILL.md
.dari-docs/agents/docs-editor-agent/prompts/system.md
.dari-docs/agents/docs-editor-agent/skills/documentation-editor/SKILL.md
```

The runner builds each trial's instruction from `prompts/system.md`, then every `skills/*/SKILL.md` in name order, then the task and output requirements. Skills are added as text rather than registered as Harbor native skills, so they work with any harness. Redeploy after editing.

Keep the output requirements in mind: the tester must write `/app/output/feedback.md`, and the editor must write `/app/output/changelog.md` and complete files under `/app/output/updated-docs/files/`. The runner appends these requirements itself.

## Workflow Payload

The CLI calls `POST /workflows/<name>?wait=result`. The tester payload:

```json
{
  "task": "Install the SDK",
  "model": "openai/gpt-5.5",
  "files": [{ "path": "README.md", "content": "# Docs\n" }],
  "publicDocUrls": ["https://docs.example.com/llms.txt"],
  "liveVerify": false,
  "runtimeSecrets": {},
  "execution": {
    "harness": "codex",
    "sandbox": "modal",
    "agentKwargs": { "max_turns": 60 },
    "agentEnv": { "MCP_TIMEOUT": "20000" },
    "environmentKwargs": {}
  }
}
```

The editor payload has `feedback` instead of `task` and no `publicDocUrls`. Every `execution` field is optional; unset fields use the runner defaults. Unknown fields are rejected.

Responses wrap the workflow result:

```json
{ "result": { "feedback": "...markdown..." } }
```

```json
{ "result": { "changelog": "...markdown...", "files": [{ "path": "README.md", "content": "...complete file..." }] } }
```

Editor `files` are repo-relative and contain only created or changed files. See [runner setup](self-managed.md#workflow-api) for the async runs API, events, and limits.

## Save Defaults

`.dari-docs/config.json` stores the service URLs:

```bash
dari-docs init \
  --tester-url https://tester-url.modal.run \
  --editor-url https://editor-url.modal.run
```

`init` doesn't save harness, sandbox, or other execution settings. `check` and `optimize` read an `execution` object from `config.json` if you add one by hand, with the same fields as the payload above, and CLI flags override it. Running `init` again rewrites `config.json` and drops that object.

## Use Different Deployments

`dari-docs` identifies services by URL. Pass URLs directly to use another deployment:

```bash
dari-docs check . \
  --tester-url https://staging-tester-url.modal.run \
  --parallel 30 \
  --task "Install the SDK"
```

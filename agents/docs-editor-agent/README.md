# docs-editor-agent

A Dari/Pi agent that applies documentation feedback to user-supplied docs.

The manifest exposes the same named LLM options as the tester agent; `medium-claude` remains the default, and self-managed runs can select a different editor model with `--editor-llm`.

Pair it with `docs-checker-agent`:

1. Run `docs-checker-agent` with an implementation task and a Mintlify `llms.txt`, `llms-full.txt`, URL list, or uploaded docs.
2. Pass the checker's feedback report plus the docs source files to `docs-editor-agent`.
3. The editor updates markdown/MDX/README/API docs, validates the changes when possible, and reports what was changed or left unresolved.

## Inputs

The agent works best with:

- docs source files or a mounted docs repo,
- feedback from the docs checker or reviewer comments,
- the target implementation task/audience,
- optional source-of-truth files such as OpenAPI specs, SDK types, CLI help output, or config examples.

If only pasted docs are provided, the agent returns rewritten replacement content and a patch-style summary.

## Safety

- Does not invent product behavior.
- Does not ask for raw secrets.
- Uses environment variable names or platform secrets for credential-dependent verification.
- Avoids production-mutating tests unless explicitly requested and documented as safe.

## Validate/deploy

```bash
cd docs-editor-agent
dari deploy --dry-run .
dari deploy .
```

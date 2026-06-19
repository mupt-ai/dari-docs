# docs-editor-agent

A dari.dev/Flue agent that applies documentation feedback to user-supplied docs.

The Flue entrypoint is `agents/docs-editor-agent.ts`; it defaults to `anthropic/claude-sonnet-4-6` and expects a Dari credential exposed as `ANTHROPIC_API_KEY` unless you edit `dari.yml`/the agent code or run `dari-docs init --deploy --anthropic-api-key-secret NAME`.

Pair it with the docs user tester:

1. Run `docs-user-tester-agent` with an implementation task and supplied docs context.
2. Pass the tester feedback plus the docs source files to `docs-editor-agent`.
3. The editor updates markdown/MDX/README/API docs when source files are available, validates the changes when possible, and reports what was changed or left unresolved.

## Safety

- Does not invent product behavior.
- Does not ask for raw secrets.
- Uses environment variable names or platform secrets for credential-dependent verification.
- Avoids production-mutating tests unless explicitly requested and documented as safe.

## Validate/deploy

```bash
cd docs-editor-agent
npm install
npx flue build --target node
dari deploy --dry-run .
dari deploy .
```

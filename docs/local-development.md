# Local Development

For CLI development, run the Go tests from the repository root:

```bash
go test ./...
```

Run the CLI from source while editing:

```bash
go run ./cmd/dari-docs --help
go run ./cmd/dari-docs init --help
go run ./cmd/dari-docs check --help
```

The bundled Flue apps and Modal deploy file live under `agents/` and are embedded into the CLI binary. The embed patterns intentionally include source files, `.flue/app.ts`, `.flue/workflows/`, `modal_app.py`, and Bun lockfiles, but not local `node_modules` or `dist` directories.

After editing an app template, validate it from that app folder:

```bash
cd agents/docs-user-tester-agent
bun install --frozen-lockfile
bun run build
PORT=8787 ANTHROPIC_API_KEY=... bun run start
```

In another terminal, run a local check against that app:

```bash
go run ./cmd/dari-docs check . \
  --tester-url http://127.0.0.1:8787 \
  --task "Follow the quickstart"
```

Repeat the same build check for `agents/docs-editor-agent` when you change the editor template:

```bash
cd agents/docs-editor-agent
bun install --frozen-lockfile
bun run build
```

A full local optimize smoke test needs both apps running on different ports and both URLs passed to `dari-docs optimize`. For Modal sandbox-gateway changes, also smoke-check `uvx modal deploy .dari-docs/agents/modal_app.py` from an extracted template when credentials are available.

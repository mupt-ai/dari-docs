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

Managed CLI flows use the hosted Dari Docs service by default (`https://optimize.dari.dev`, or `DARI_DOCS_BASE_URL` when set). To exercise real managed runs from a local checkout, set `DARI_DOCS_API_KEY` and run `go run ./cmd/dari-docs check --managed --wait ...`.

The bundled Flue agent folders and Modal deploy file live under `agents/` and are embedded into the CLI binary. The embed patterns intentionally include visible source files such as `app.ts`, `agents/`, `workflows/`, `prompts/`, `skills/`, `modal_app.py`, and Bun lockfiles, but not local generated directories such as `.flue/`, `.flue-vite/`, `node_modules/`, or `dist/`.

The source tree also contains `agents/docs-user-tester-agent/dari.yml` and `agents/docs-editor-agent/dari.yml` for the hosted managed agents. Those manifests use `sandbox.provider: modal` and are deployed with `dari deploy`; they are not extracted by `dari-docs init`.

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

For managed-agent manifest changes, validate dry-run packaging before deploying:

```bash
dari deploy agents/docs-user-tester-agent --dry-run
dari deploy agents/docs-editor-agent --dry-run
```

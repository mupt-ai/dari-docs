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

The bundled agents live under `agents/` and are embedded into the CLI binary. After editing an agent template, validate the Flue project from that agent folder. `npm install` installs the local Flue dependency, and `npx flue` runs that installed CLI:

```bash
cd agents/docs-user-tester-agent
npm install
npx flue build --target node
```

Repeat the same check for `agents/docs-editor-agent` when you change the editor template.

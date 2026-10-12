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

The bundled runner (`harbor_runner.py`), its `requirements.txt`, and the Modal deploy file (`modal_app.py`) live under `agents/` and are embedded into the CLI binary. `dari-docs init` extracts them to `.dari-docs/agents/`.

## Test The Runner

The runner needs Python 3.12 or newer. Create a virtual environment, install the pinned requirements, and run the Python tests:

```bash
cd agents
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt pytest
python -m pytest
```

## Smoke-Test Locally

Start the tester and editor services on different ports:

```bash
cd agents
. .venv/bin/activate
ANTHROPIC_API_KEY=... python harbor_runner.py --service tester --port 8000
```

```bash
cd agents
. .venv/bin/activate
ANTHROPIC_API_KEY=... python harbor_runner.py --service editor --port 8001
```

In another terminal, run a local optimize against both services:

```bash
go run ./cmd/dari-docs optimize . \
  --tester-url http://127.0.0.1:8000 \
  --editor-url http://127.0.0.1:8001 \
  --task "Follow the quickstart"
```

The default sandbox is `modal`, so the runner creates sandboxes in your Modal account; run `uvx modal setup` first. To stay fully local, pass `--sandbox docker` to `dari-docs` with a Docker daemon running.

For Modal deploy changes, also smoke-check `uvx modal deploy .dari-docs/agents/modal_app.py` from an extracted template when credentials are available.

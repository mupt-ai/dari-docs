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

The bundled runner (`harbor_runner.py`, `runner_api.py`, `requirements.txt`, `Dockerfile`), the Modal deploy file (`modal_app.py`), and the tester and editor prompts and skills live under `agents/` and are embedded into the CLI binary. `dari-docs init` extracts them to `.dari-docs/agents/`. The Python tests under `agents/tests/` are not embedded.

## Test The Runner

The runner needs Python 3.12 or newer. From the repository root, create a virtual environment, install the pinned requirements plus the test tools, and run the Python tests:

```bash
python3 -m venv .venv
. .venv/bin/activate
pip install -r agents/requirements.txt pytest pytest-asyncio
python -m pytest agents/tests
```

These tests don't need model keys or a sandbox provider. The Docker smoke tests are skipped unless you opt in. They run real Harbor trials in local Docker with a fake harness, so they need a running Docker engine but no inference key:

```bash
RUN_HARBOR_DOCKER_TESTS=1 python -m pytest agents/tests/test_docker.py
```

PR checks run both commands.

## Smoke-Test Locally

Start the tester and editor services on different ports:

```bash
. .venv/bin/activate
cd agents
ANTHROPIC_API_KEY=... python harbor_runner.py --service tester --port 8000
```

```bash
. .venv/bin/activate
cd agents
ANTHROPIC_API_KEY=... python harbor_runner.py --service editor --port 8001
```

In another terminal, run a local optimize against both services:

```bash
go run ./cmd/dari-docs optimize . \
  --tester-url http://127.0.0.1:8000 \
  --editor-url http://127.0.0.1:8001 \
  --task "Follow the quickstart"
```

The default sandbox is `modal`, so the runner creates sandboxes in your Modal account; run `uvx modal setup` first. To stay fully local, pass `--sandbox docker` to `dari-docs` with a Docker engine running on the same host. Local run state is in memory and is lost when the runner restarts.

For Modal deploy changes, also smoke-check `uvx modal deploy .dari-docs/agents/modal_app.py` from an extracted template when credentials are available.

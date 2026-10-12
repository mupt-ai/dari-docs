"""Modal HTTP gateways and detached Harbor controllers (one task sandbox per run)."""

import os
from pathlib import Path
from uuid import uuid4

import modal

ROOT = Path(__file__).parent
SECRET_NAME = os.environ.get("DARI_DOCS_MODAL_SECRET", "dari-docs-model-providers")
app = modal.App("dari-docs-agents")
model_secret = modal.Secret.from_name(SECRET_NAME)
runs = modal.Dict.from_name("dari-docs-harbor-runs", create_if_missing=True)

image = (
    modal.Image.debian_slim(python_version="3.13")
    .apt_install("openssh-client", "ca-certificates")
    .pip_install_from_requirements(str(ROOT / "requirements.txt"))
    .add_local_dir(
        ROOT,
        remote_path="/app",
        copy=True,
        ignore=["__pycache__", ".pytest_cache", ".venv", "tests", "*.go"],
    )
    .env({"PYTHONPATH": "/app"})
    .workdir("/app")
)


class ModalBackend:
    async def submit(self, workflow, payload):
        from runner_api import initial_state

        run_id = str(uuid4())
        await self.put(run_id, initial_state(workflow))
        try:
            await execute.spawn.aio(run_id, workflow, payload.model_dump())
        except Exception:
            state = await self.get(run_id)
            state.update(status="failed", error="Unable to schedule controller")
            await self.put(run_id, state)
            raise
        return run_id

    async def get(self, run_id):
        return await runs.get.aio(run_id, None)

    async def put(self, run_id, state):
        # Only the controller updates this record. Cancellation has a separate key.
        await runs.put.aio(run_id, state)

    async def cancel_requested(self, run_id):
        return await runs.get.aio(f"cancel:{run_id}", False)

    async def cancel(self, run_id):
        await runs.put.aio(f"cancel:{run_id}", True)


@app.function(image=image, secrets=[model_secret], timeout=3500, max_containers=100)
async def execute(run_id: str, workflow: str, payload: dict):
    from harbor_runner import Payload
    from runner_api import execute_run

    await execute_run(ModalBackend(), run_id, workflow, Payload.model_validate(payload))


@app.function(image=image, secrets=[model_secret], timeout=3600, max_containers=50)
@modal.concurrent(max_inputs=100, target_inputs=30)
@modal.asgi_app(label="tester")
def tester():
    from runner_api import create_app

    return create_app("tester", ModalBackend())


@app.function(image=image, secrets=[model_secret], timeout=3600, max_containers=20)
@modal.concurrent(max_inputs=50, target_inputs=10)
@modal.asgi_app(label="editor")
def editor():
    from runner_api import create_app

    return create_app("editor", ModalBackend())

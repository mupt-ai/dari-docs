from __future__ import annotations

import asyncio
import os
import time
from pathlib import Path

import modal

APP_ROOT = Path(__file__).parent
BUN_VERSION = "1.3.13"
PORT = 8000
MODEL_SECRET_NAME = os.environ.get("DARI_DOCS_MODAL_SECRET", "dari-docs-model-providers")

app = modal.App("dari-docs-agents")
model_secret = modal.Secret.from_name(MODEL_SECRET_NAME)


def ignore_template_artifacts(path: Path) -> bool:
    return any(part in {".dari", "node_modules", "dist"} for part in path.parts)


base_agent_image = (
    modal.Image.debian_slim(python_version="3.13")
    .apt_install("ca-certificates", "curl", "gnupg")
    .run_commands(
        "curl -fsSL https://deb.nodesource.com/setup_22.x | bash -",
        "apt-get install -y nodejs",
        f"npm install -g bun@{BUN_VERSION}",
    )
)

gateway_image = modal.Image.debian_slim(python_version="3.13").pip_install(
    "fastapi==0.125.0",
    "httpx==0.28.1",
)


def agent_image(name: str, remote_path: str) -> modal.Image:
    return (
        base_agent_image.add_local_dir(
            APP_ROOT / name,
            remote_path=remote_path,
            copy=True,
            ignore=ignore_template_artifacts,
        )
        .workdir(remote_path)
        .run_commands("bun install --frozen-lockfile", "bun run build")
    )


tester_image = agent_image("docs-user-tester-agent", "/app/tester")
editor_image = agent_image("docs-editor-agent", "/app/editor")


def invoke_workflow_in_sandbox(
    *,
    image: modal.Image,
    workdir: str,
    workflow: str,
    body: bytes,
    content_type: str,
) -> tuple[int, str, bytes]:
    import httpx

    sandbox = modal.Sandbox.create(
        "bun",
        "run",
        "start",
        app=app,
        image=image,
        workdir=workdir,
        env={"PORT": str(PORT)},
        secrets=[model_secret],
        encrypted_ports=[PORT],
        timeout=3600,
        idle_timeout=300,
    )
    try:
        tunnels = sandbox.tunnels(timeout=120)
        tunnel = tunnels[PORT]
        base_url = tunnel.url.rstrip("/")
        with httpx.Client(timeout=None) as client:
            wait_for_health(client, base_url)
            response = client.post(
                f"{base_url}/workflows/{workflow}",
                params={"wait": "result"},
                content=body,
                headers={"content-type": content_type or "application/json"},
            )
            return response.status_code, response.headers.get("content-type", "application/json"), response.content
    finally:
        sandbox.terminate(wait=False)


def wait_for_health(client, base_url: str) -> None:
    deadline = time.monotonic() + 120
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            response = client.get(f"{base_url}/healthz", timeout=5)
            if response.status_code == 200:
                return
            last_error = RuntimeError(f"health check returned HTTP {response.status_code}")
        except Exception as exc:  # noqa: BLE001 - report the last startup failure.
            last_error = exc
        time.sleep(1)
    raise RuntimeError(f"Flue sandbox did not become healthy: {last_error}")


def gateway_app(*, workflow: str, image: modal.Image, workdir: str, service: str):
    from fastapi import FastAPI, Request, Response

    web = FastAPI()

    @web.get("/healthz")
    def healthz():
        return {"ok": True, "service": service, "runtime": "modal-sandbox-gateway"}

    @web.post(f"/workflows/{workflow}")
    async def run_workflow(request: Request):
        body = await request.body()
        status, media_type, content = await asyncio.to_thread(
            invoke_workflow_in_sandbox,
            image=image,
            workdir=workdir,
            workflow=workflow,
            body=body,
            content_type=request.headers.get("content-type", "application/json"),
        )
        return Response(content=content, status_code=status, headers={"content-type": media_type})

    return web


@app.function(
    image=gateway_image,
    timeout=3700,
    max_containers=50,
    scaledown_window=300,
)
@modal.concurrent(max_inputs=100, target_inputs=30)
@modal.asgi_app(label="tester")
def tester():
    return gateway_app(
        workflow="test",
        image=tester_image,
        workdir="/app/tester",
        service="docs-user-tester-agent",
    )


@app.function(
    image=gateway_image,
    timeout=3700,
    max_containers=20,
    scaledown_window=300,
)
@modal.concurrent(max_inputs=50, target_inputs=10)
@modal.asgi_app(label="editor")
def editor():
    return gateway_app(
        workflow="edit",
        image=editor_image,
        workdir="/app/editor",
        service="docs-editor-agent",
    )

from __future__ import annotations

import os
import subprocess
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


base_image = (
    modal.Image.debian_slim(python_version="3.13")
    .apt_install("ca-certificates", "curl", "gnupg")
    .run_commands(
        "curl -fsSL https://deb.nodesource.com/setup_22.x | bash -",
        "apt-get install -y nodejs",
        f"npm install -g bun@{BUN_VERSION}",
    )
)


def agent_image(name: str, remote_path: str) -> modal.Image:
    return (
        base_image.add_local_dir(
            APP_ROOT / name,
            remote_path=remote_path,
            copy=True,
            ignore=ignore_template_artifacts,
        )
        .workdir(remote_path)
        .run_commands("bun install --frozen-lockfile", "bun run build")
    )


def start_agent(remote_path: str) -> None:
    subprocess.Popen(["bun", "run", "start"], cwd=remote_path)


@app.function(
    image=agent_image("docs-user-tester-agent", "/app/tester"),
    secrets=[model_secret],
    env={"PORT": str(PORT)},
    timeout=3600,
    scaledown_window=300,
    max_containers=50,
)
@modal.concurrent(max_inputs=1, target_inputs=1)
@modal.web_server(PORT, startup_timeout=120, label="tester")
def tester() -> None:
    start_agent("/app/tester")


@app.function(
    image=agent_image("docs-editor-agent", "/app/editor"),
    secrets=[model_secret],
    env={"PORT": str(PORT)},
    timeout=3600,
    scaledown_window=300,
    max_containers=50,
)
@modal.concurrent(max_inputs=1, target_inputs=1)
@modal.web_server(PORT, startup_timeout=120, label="editor")
def editor() -> None:
    start_agent("/app/editor")

"""Turn docs workflow requests into isolated Harbor trials and validated artifacts."""

from __future__ import annotations

import json
import logging
import os
import re
import tempfile
from collections.abc import Awaitable, Callable
from pathlib import Path, PurePosixPath
from typing import Any, Literal
from uuid import uuid4

from pydantic import BaseModel, ConfigDict, Field, model_validator

ROOT = Path(__file__).parent
MAX_OUTPUT_BYTES = 8 * 1024 * 1024
ENV_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
SENSITIVE_NAME = re.compile(r"KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL|AUTH", re.I)
WORKFLOWS = {"test": "docs-user-tester-agent", "edit": "docs-editor-agent"}
EventSink = Callable[[dict[str, Any]], Awaitable[None]]


class InputError(ValueError):
    pass


class Execution(BaseModel):
    model_config = ConfigDict(extra="forbid")
    harness: str | None = None
    sandbox: str | None = None
    agentKwargs: dict[str, Any] = Field(default_factory=dict)
    agentEnv: dict[str, str] = Field(default_factory=dict)
    environmentKwargs: dict[str, Any] = Field(default_factory=dict)


class DocFile(BaseModel):
    model_config = ConfigDict(extra="forbid")
    path: str
    content: str


class Payload(BaseModel):
    model_config = ConfigDict(extra="forbid")
    task: str | None = None
    feedback: str | None = None
    model: str | None = None
    files: list[DocFile] = Field(default_factory=list)
    publicDocUrls: list[str] = Field(default_factory=list)
    liveVerify: bool = False
    runtimeSecrets: dict[str, str] = Field(default_factory=dict)
    execution: Execution = Field(default_factory=Execution)

    @model_validator(mode="after")
    def validate_inputs(self):
        seen = set()
        for file in self.files:
            rel = safe_relative_path(file.path)
            if rel in seen:
                raise InputError("duplicate documentation path")
            seen.add(rel)
        if self.runtimeSecrets and not self.liveVerify:
            raise InputError("runtimeSecrets requires liveVerify")
        for name, value in self.runtimeSecrets.items():
            if not ENV_NAME.fullmatch(name) or not value:
                raise InputError(
                    "runtimeSecrets requires valid names and nonempty values"
                )
            if (
                name in provider_env_names()
                or name.startswith(("MODAL_", "DARI_DOCS_"))
                or name in {"PATH", "HOME", "PYTHONPATH"}
            ):
                raise InputError(
                    "runtimeSecrets cannot override runner or provider configuration"
                )
        for name, value in self.execution.agentEnv.items():
            if not ENV_NAME.fullmatch(name) or SENSITIVE_NAME.search(name):
                raise InputError(
                    "agentEnv is for non-secret settings; configure credentials on the deployment"
                )
            if name.endswith(("_BASE_URL", "_API_BASE")) or name == "DATABRICKS_HOST":
                allowed = set(
                    filter(
                        None,
                        os.environ.get("DARI_DOCS_ALLOWED_MODEL_ENDPOINTS", "").split(
                            ","
                        ),
                    )
                )
                if value != os.environ.get(name) and value not in allowed:
                    raise InputError("model endpoint must be allowed by the deployment")
            elif name in provider_env_names() or name.startswith(
                ("MODAL_", "DARI_DOCS_", "AWS_", "GOOGLE_", "CLAUDE_CODE_USE_")
            ):
                raise InputError("provider routing settings belong on the deployment")
        for name in self.execution.environmentKwargs:
            if SENSITIVE_NAME.search(name) or name in {
                "volumes",
                "env",
                "persistent_env",
            }:
                raise InputError(
                    "sandbox credentials and mounts belong on the deployment"
                )
        allowed_adapters = set(
            filter(None, os.environ.get("DARI_DOCS_CUSTOM_ADAPTERS", "").split(","))
        )
        for adapter in [self.execution.harness, self.execution.sandbox]:
            if adapter and ":" in adapter and adapter not in allowed_adapters:
                raise InputError("custom adapter must be allowed by the deployment")
        for value in [*self.runtimeSecrets.values(), *self.execution.agentEnv.values()]:
            if "${" in value:
                raise InputError(
                    "request environment values must be literal, not controller environment templates"
                )
        return self


def safe_relative_path(value: str) -> str:
    if not value or "\\" in value or "\0" in value:
        raise InputError("unsafe documentation path")
    path = PurePosixPath(value)
    if (
        path.is_absolute()
        or any(p in {"", ".", ".."} for p in value.split("/"))
        or ":" in value
    ):
        raise InputError("documentation paths must be portable repo-relative paths")
    return path.as_posix()


def provider_env_names() -> set[str]:
    from harbor.agents.model_connection import PROVIDERS

    names = {
        "ANTHROPIC_AUTH_TOKEN",
        "CLAUDE_CODE_OAUTH_TOKEN",
        "ANTHROPIC_OAUTH_TOKEN",
        "AWS_ACCESS_KEY_ID",
        "AWS_SECRET_ACCESS_KEY",
        "AWS_SESSION_TOKEN",
        "AWS_BEARER_TOKEN_BEDROCK",
        "AWS_REGION",
        "AWS_DEFAULT_REGION",
        "CLAUDE_CODE_USE_BEDROCK",
        "CLAUDE_CODE_USE_VERTEX",
        "GOOGLE_APPLICATION_CREDENTIALS",
    }
    for provider in PROVIDERS.values():
        names.update(provider.api_key_envs)
        names.update(provider.base_url_envs)
        names.update(provider.extra_envs)
    names.update(
        filter(None, os.environ.get("DARI_DOCS_PROVIDER_ENV_NAMES", "").split(","))
    )
    # Never expose control-plane credentials to an agent, even if misconfigured.
    return {
        n for n in names if n != "DARI_DOCS_RUNNER_TOKEN" and not n.startswith("MODAL_")
    }


def private_error_message(message: str, payload: Payload) -> str:
    values = list(payload.runtimeSecrets.values()) + [
        value
        for name, value in os.environ.items()
        if SENSITIVE_NAME.search(name) and value
    ]
    for value in sorted(values, key=len, reverse=True):
        message = message.replace(value, "[REDACTED]")
    return (
        message
        if len(message) <= 2000
        else message[:500] + "\n[truncated]\n" + message[-1500:]
    )


def normalize_model(value: str) -> str:
    value = value.strip()
    if "/" in value:
        return value
    if value.startswith("claude-"):
        return "anthropic/" + value
    if value.startswith("gpt-") or re.match(r"^o\d", value):
        return "openai/" + value
    return value


def workflow_instruction(workflow: str, payload: Payload) -> str:
    field = payload.task if workflow == "test" else payload.feedback
    if not field or not field.strip():
        raise InputError(
            f"payload.{'task' if workflow == 'test' else 'feedback'} must be nonempty"
        )
    folder = ROOT / WORKFLOWS[workflow]
    system = (folder / "prompts/system.md").read_text()
    for skill in sorted((folder / "skills").glob("*/SKILL.md")):
        system += "\n\n" + skill.read_text()
    secrets = ", ".join(sorted(payload.runtimeSecrets)) or "none"
    live = (
        f"Live verification is enabled. Runtime secrets are environment variables: {secrets}. Never print secret values."
        if payload.liveVerify
        else "Live verification is disabled. Only run safe no-credential smoke tests."
    )
    if workflow == "test":
        urls = "\n".join(payload.publicDocUrls)
        return f"{system}\n\nTask:\n{field}\n\nLocal docs: /app/input-docs/files/. Public docs starting points:\n{urls or 'none'}\n\nActually try the task in /app/attempt/. {live}\n\nWrite concise Markdown feedback to /app/output/feedback.md. Describe attempts, outcomes, blockers, and smallest useful docs changes. Do not score the docs. Writing this file is required; a chat response alone is not a result."
    return f"{system}\n\nOriginal docs: /app/input-docs/files/. Tester feedback:\n{field}\n\n{live}\n\nWrite proposed complete files ONLY under /app/output/updated-docs/files/, preserving repo-relative paths (e.g. README.md). Include only created or changed files; do not copy unchanged files. Write a concise changelog and unresolved items to /app/output/changelog.md. Do not include secrets. Do not invent product facts; use explicit TODO(owner) where source truth is missing. When original files exist, produce at least one proposed file rather than a changelog alone."


def materialize_task(root: Path, workflow: str, payload: Payload) -> Path:
    task = root / "task"
    workspace = task / "environment"
    for name in ("input-docs/files", "attempt", "output/updated-docs/files"):
        (workspace / name).mkdir(parents=True, exist_ok=True)
    for file in payload.files:
        target = workspace / "input-docs/files" / safe_relative_path(file.path)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(file.content, encoding="utf-8")
    (task / "instruction.md").write_text(
        workflow_instruction(workflow, payload), encoding="utf-8"
    )
    # A prebuilt image plus loose workspace files avoids rebuilding for every docs bundle.
    image = os.environ.get("DARI_DOCS_WORKSPACE_IMAGE", "python:3.13-slim-bookworm")
    (task / "task.toml").write_text(
        'version = "1.0"\nartifacts = [{source = "/app/output", destination = "output"}]\n'
        "[agent]\ntimeout_sec = 1800\n[environment]\n"
        f'docker_image = {json.dumps(image)}\nworkdir = "/app"\ncpus = 2\nmemory_mb = 4096\n',
        encoding="utf-8",
    )
    return task


def trial_config(task: Path, root: Path, payload: Payload):
    from harbor.models.environment_type import EnvironmentType
    from harbor.models.trial.config import (
        AgentConfig,
        EnvironmentConfig,
        TaskConfig,
        TrialConfig,
        VerifierConfig,
    )

    execution = payload.execution
    harness = execution.harness or os.environ.get(
        "DARI_DOCS_DEFAULT_HARNESS", "claude-code"
    )
    sandbox = execution.sandbox or os.environ.get("DARI_DOCS_DEFAULT_SANDBOX", "modal")
    kwargs = json.loads(os.environ.get("DARI_DOCS_ENVIRONMENT_KWARGS", "{}"))
    kwargs.update(execution.environmentKwargs)
    if sandbox == "modal":
        kwargs.setdefault("app_name", "dari-docs-agents")
        kwargs["sandbox_timeout_secs"] = min(
            int(kwargs.get("sandbox_timeout_secs", 3300)), 3300
        )
    environment = (
        EnvironmentConfig(import_path=sandbox, kwargs=kwargs)
        if ":" in sandbox
        else EnvironmentConfig(type=EnvironmentType(sandbox), kwargs=kwargs)
    )
    env = {
        name: os.environ[name] for name in provider_env_names() if os.environ.get(name)
    }
    env.update(execution.agentEnv)
    env.update(payload.runtimeSecrets)
    model = normalize_model(
        payload.model
        or os.environ.get("DARI_DOCS_DEFAULT_MODEL", "anthropic/claude-sonnet-4-6")
    )
    return TrialConfig(
        task=TaskConfig(path=task),
        trial_name="docs-" + uuid4().hex[:12],
        trials_dir=root / "trials",
        agent=AgentConfig(
            name=harness,
            model_name=model,
            kwargs=execution.agentKwargs,
            env=env,
            override_setup_timeout_sec=600,
        ),
        environment=environment,
        verifier=VerifierConfig(disable=True),
    )


async def install_artifact_retry(
    trial, workflow: str, payload: Payload, emit: EventSink
) -> None:
    """Keep retries in the same Harbor session/workspace and agent timeout budget."""
    original_run = trial.agent.run
    original_resume = trial.agent.resume
    require_files = workflow == "edit" and bool(payload.files)
    check = (
        "test -s /app/output/feedback.md"
        if workflow == "test"
        else "test -s /app/output/changelog.md"
        + (
            " && find /app/output/updated-docs/files -type f -exec sh -c "
            "'for f do rel=${f#/app/output/updated-docs/files/}; "
            'if ! cmp -s "$f" "/app/input-docs/files/$rel"; then printf changed; fi; done\' sh {} + | grep -q changed'
            if require_files
            else ""
        )
    )

    async def run(instruction, environment, context):
        await original_run(
            instruction=instruction, environment=environment, context=context
        )
        status = await environment.exec(check)
        if status.return_code == 0:
            return
        await emit({"event": "artifact-retry"})
        retry = "Required output files are missing or empty. Finish writing the files specified in the task; a chat reply alone is not sufficient."
        # resume() delegates to self.run(); temporarily restore the native method
        # so a retry cannot recursively initiate another retry.
        trial.agent.run = original_run
        try:
            if trial.agent.capabilities.resume:
                await original_resume(
                    instruction=retry, environment=environment, context=context
                )
            else:
                await original_run(
                    instruction=instruction + "\n\n" + retry,
                    environment=environment,
                    context=context,
                )
        finally:
            trial.agent.run = run

    trial.agent.run = run


def read_output(output: Path, workflow: str, payload: Payload) -> dict[str, Any]:
    total = 0
    paths: dict[str, str] = {}
    if not output.is_dir() or output.is_symlink():
        raise InputError("agent did not produce output artifacts")
    for directory, dirs, files in os.walk(output, followlinks=False):
        for name in dirs + files:
            path = Path(directory) / name
            if path.is_symlink() or not (path.is_file() or path.is_dir()):
                raise InputError(
                    "output artifacts cannot contain links or special files"
                )
        for name in files:
            path = Path(directory) / name
            total += path.stat().st_size
            if total > MAX_OUTPUT_BYTES:
                raise InputError("output artifacts exceed the size limit")
            rel = safe_relative_path(path.relative_to(output).as_posix())
            relevant = (
                rel == "feedback.md"
                if workflow == "test"
                else rel == "changelog.md" or rel.startswith("updated-docs/files/")
            )
            if relevant:
                try:
                    paths[rel] = path.read_text(encoding="utf-8")
                except UnicodeError:
                    raise InputError(
                        "documentation output must be UTF-8 text"
                    ) from None
    if workflow == "test":
        feedback = paths.get("feedback.md", "").strip()
        if not feedback:
            raise InputError("agent did not write nonempty feedback.md")
        result = {"feedback": feedback}
    else:
        changelog = paths.get("changelog.md", "").strip()
        if not changelog:
            raise InputError("agent did not write nonempty changelog.md")
        originals = {file.path: file.content for file in payload.files}
        prefix = "updated-docs/files/"
        files = [
            {"path": safe_relative_path(rel[len(prefix) :]), "content": content}
            for rel, content in sorted(paths.items())
            if rel.startswith(prefix) and originals.get(rel[len(prefix) :]) != content
        ]
        if payload.files and not files:
            raise InputError(
                "editor produced no created or changed documentation files"
            )
        result = {"changelog": changelog, "files": files}
    secrets = list(payload.runtimeSecrets.values()) + [
        os.environ[name]
        for name in provider_env_names()
        if os.environ.get(name) and SENSITIVE_NAME.search(name)
    ]
    for secret in secrets:
        if any(secret in content or secret in rel for rel, content in paths.items()):
            raise InputError("result contains a credential")
    if len(json.dumps(result, ensure_ascii=False).encode()) > MAX_OUTPUT_BYTES:
        raise InputError("serialized result exceeds the size limit")
    return result


async def run_workflow(
    workflow: Literal["test", "edit"], payload: Payload, emit: EventSink
) -> dict[str, Any]:
    from harbor.trial.hooks import TrialEvent
    from harbor.trial.trial import Trial

    if workflow not in WORKFLOWS:
        raise InputError("unknown workflow")
    with tempfile.TemporaryDirectory(prefix="dari-docs-") as temporary:
        root = Path(temporary)
        task = materialize_task(root, workflow, payload)
        config = trial_config(task, root, payload)
        trial = await Trial.create(config)

        async def lifecycle(event):
            # Deliberately omit raw config, exceptions and logs: they may contain credentials.
            await emit({"event": event.event.value, "trialId": str(event.trial_id)})

        for event in TrialEvent:
            trial.add_hook(event, lifecycle)
        await install_artifact_retry(trial, workflow, payload, emit)
        result = await trial.run()
        if result.exception_info:
            logging.getLogger(__name__).error(
                "Harbor trial %s failed: %s: %s",
                trial.config.trial_name,
                result.exception_info.exception_type,
                private_error_message(result.exception_info.exception_message, payload),
            )
            raise RuntimeError("Harbor trial failed; inspect private controller logs")
        await emit({"event": "collecting-result"})
        # Harbor collects artifacts before deleting the sandbox.
        return read_output(trial.paths.artifacts_dir / "output", workflow, payload)


if __name__ == "__main__":
    import argparse
    import uvicorn
    from runner_api import LocalBackend, create_app

    parser = argparse.ArgumentParser()
    parser.add_argument("--service", choices=["tester", "editor"], default="tester")
    parser.add_argument("--port", type=int, default=8000)
    parser.add_argument("--host", default="127.0.0.1")
    args = parser.parse_args()
    if args.host not in {"127.0.0.1", "::1", "localhost"} and not os.environ.get(
        "DARI_DOCS_RUNNER_TOKEN"
    ):
        parser.error("non-loopback service requires DARI_DOCS_RUNNER_TOKEN")
    uvicorn.run(
        create_app(args.service, LocalBackend()), host=args.host, port=args.port
    )

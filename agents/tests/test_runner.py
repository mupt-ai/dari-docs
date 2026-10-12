import os
from types import SimpleNamespace

import pytest

from harbor_runner import (
    InputError,
    Payload,
    install_artifact_retry,
    materialize_task,
    normalize_model,
    read_output,
    trial_config,
)


@pytest.mark.parametrize(
    "path",
    [
        "/etc/passwd",
        "../README.md",
        "a/../b",
        "a\\b",
        "a//b",
        "a/./b",
        "C:/file",
        "",
        "a\0b",
    ],
)
def test_reject_unsafe_input_paths(path):
    with pytest.raises(ValueError):
        Payload(task="test", files=[{"path": path, "content": "x"}])


def test_duplicate_and_secret_inputs():
    with pytest.raises(ValueError):
        Payload(files=[{"path": "a", "content": "a"}, {"path": "a", "content": "b"}])
    for kwargs in [
        {"runtimeSecrets": {"KEY": "secret"}},
        {"liveVerify": True, "runtimeSecrets": {"MODAL_TOKEN_SECRET": "secret"}},
        {"liveVerify": True, "runtimeSecrets": {"OPENAI_API_KEY": "secret"}},
        {"execution": {"agentEnv": {"MY_API_KEY": "secret"}}},
        {"execution": {"agentEnv": {"BAD NAME": "value"}}},
        {"execution": {"agentEnv": {"PASSWD": "secret"}}},
        {"execution": {"agentEnv": {"ANTHROPIC_BASE_URL": "https://attacker.example"}}},
        {"execution": {"agentEnv": {"LEAK": "${DARI_DOCS_RUNNER_TOKEN}"}}},
        {"liveVerify": True, "runtimeSecrets": {"LEAK": "${MODAL_TOKEN_SECRET}"}},
        {"execution": {"environmentKwargs": {"secrets": ["model-secret"]}}},
        {"execution": {"environmentKwargs": {"volumes": {"/data": "private"}}}},
        {"execution": {"harness": "unapproved.module:Agent"}},
    ]:
        with pytest.raises(ValueError):
            Payload(**kwargs)


def test_materialize_and_real_harbor_config(tmp_path, monkeypatch):
    monkeypatch.setenv("DARI_DOCS_RUNNER_TOKEN", "CONTROL-SECRET")
    monkeypatch.setenv("MODAL_TOKEN_SECRET", "CONTROL-SECRET")
    monkeypatch.setenv("OPENAI_API_KEY", "provider-key")
    monkeypatch.setenv("DARI_DOCS_CUSTOM_ADAPTERS", "custom.env:Sandbox")
    payload = Payload(
        task="Install SDK",
        liveVerify=True,
        runtimeSecrets={"PRODUCT_KEY": "product-key"},
        files=[{"path": "docs/start.md", "content": "hello"}],
        execution={
            "harness": "codex",
            "sandbox": "custom.env:Sandbox",
            "agentKwargs": {"reasoning_effort": "high"},
        },
    )
    task = materialize_task(tmp_path, "test", payload)
    assert (task / "environment/input-docs/files/docs/start.md").read_text() == "hello"
    instruction = (task / "instruction.md").read_text()
    assert "feedback.md" in instruction and "PRODUCT_KEY" in instruction
    assert "product-key" not in instruction
    cfg = trial_config(task, tmp_path, payload)
    assert cfg.agent.model_name == "anthropic/claude-sonnet-4-6"
    assert cfg.agent.env["OPENAI_API_KEY"] == "provider-key"
    assert cfg.agent.env["PRODUCT_KEY"] == "product-key"
    assert "DARI_DOCS_RUNNER_TOKEN" not in cfg.agent.env
    assert "MODAL_TOKEN_SECRET" not in cfg.agent.env
    assert cfg.environment.import_path == "custom.env:Sandbox"
    assert cfg.environment.delete and cfg.verifier.disable
    assert cfg.trial_name != trial_config(task, tmp_path, payload).trial_name
    from harbor.models.task.task import Task

    parsed = Task(task_dir=task, disable_verification=True)
    assert parsed.config.environment.docker_image == "python:3.13-slim-bookworm"
    assert parsed.config.artifacts[0].destination == "output"


@pytest.mark.asyncio
async def test_real_pinned_trial_creation(tmp_path):
    from harbor.trial.trial import Trial

    payload = Payload(task="test", execution={"harness": "nop", "sandbox": "docker"})
    task = materialize_task(tmp_path, "test", payload)
    trial = await Trial.create(trial_config(task, tmp_path, payload))
    assert trial.agent.name() == "nop"
    assert trial.paths.artifacts_dir.parent.name.startswith("docs-")


def output(root, files):
    root.mkdir()
    for rel, content in files.items():
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
    return root


def test_tester_output(tmp_path):
    folder = output(tmp_path / "output", {"feedback.md": "It worked\n"})
    assert read_output(folder, "test", Payload(task="t")) == {"feedback": "It worked"}
    (folder / "feedback.md").write_text("")
    with pytest.raises(InputError, match="feedback"):
        read_output(folder, "test", Payload())


def test_editor_changed_files_only(tmp_path):
    folder = output(
        tmp_path / "output",
        {
            "changelog.md": "Fixed setup",
            "updated-docs/files/README.md": "fixed",
            "updated-docs/files/unchanged.md": "original",
        },
    )
    payload = Payload(
        feedback="blocked",
        files=[
            {"path": "README.md", "content": "old"},
            {"path": "unchanged.md", "content": "original"},
        ],
    )
    result = read_output(folder, "edit", payload)
    assert result == {
        "changelog": "Fixed setup",
        "files": [{"path": "README.md", "content": "fixed"}],
    }
    (folder / "updated-docs/files/README.md").write_text("old")
    with pytest.raises(InputError, match="no created or changed"):
        read_output(folder, "edit", payload)


def test_output_symlink_special_and_size(tmp_path, monkeypatch):
    folder = output(tmp_path / "output", {"feedback.md": "hello"})
    (folder / "leak").symlink_to("/etc/passwd")
    with pytest.raises(InputError, match="links"):
        read_output(folder, "test", Payload())
    (folder / "leak").unlink()
    os.mkfifo(folder / "pipe")
    with pytest.raises(InputError, match="special"):
        read_output(folder, "test", Payload())
    (folder / "pipe").unlink()
    monkeypatch.setattr("harbor_runner.MAX_OUTPUT_BYTES", 1)
    with pytest.raises(InputError, match="size limit"):
        read_output(folder, "test", Payload())


def test_secret_result_rejected(tmp_path):
    folder = output(tmp_path / "output", {"feedback.md": "accidental secret-value"})
    payload = Payload(liveVerify=True, runtimeSecrets={"PRODUCT_KEY": "secret-value"})
    with pytest.raises(InputError, match="credential"):
        read_output(folder, "test", payload)


@pytest.mark.asyncio
@pytest.mark.parametrize("resume", [True, False])
async def test_retry_once_same_session(resume):
    calls = []
    agent = SimpleNamespace(capabilities=SimpleNamespace(resume=resume))

    async def run(**kwargs):
        calls.append("run")

    async def resumed(**kwargs):
        calls.append("resume")
        await agent.run(**kwargs)

    agent.run = run
    agent.resume = resumed
    trial = SimpleNamespace(agent=agent)

    async def execute(command):
        return SimpleNamespace(return_code=1)

    env = SimpleNamespace(exec=execute)
    events = []

    async def emit(event):
        events.append(event)

    await install_artifact_retry(
        trial, "edit", Payload(files=[{"path": "README.md", "content": "old"}]), emit
    )
    await trial.agent.run("task", env, SimpleNamespace())
    assert calls == (["run", "resume", "run"] if resume else ["run", "run"])
    assert events == [{"event": "artifact-retry"}]


@pytest.mark.parametrize(
    "model, expected",
    [
        ("claude-sonnet-4-6", "anthropic/claude-sonnet-4-6"),
        ("gpt-5.5", "openai/gpt-5.5"),
        ("openrouter/provider/model", "openrouter/provider/model"),
    ],
)
def test_model_normalization(model, expected):
    assert normalize_model(model) == expected

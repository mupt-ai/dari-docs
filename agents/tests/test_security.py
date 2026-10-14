import pytest

from harbor_runner import (
    InputError,
    Payload,
    materialize_task,
    private_error_message,
    read_output,
    trial_config,
)


@pytest.mark.parametrize("secret", ['key"with\\quotes', "key\nwith\nlines"])
def test_secret_scan_raw_text(tmp_path, secret):
    output = tmp_path / "output"
    output.mkdir()
    (output / "feedback.md").write_text("Leaked " + secret)
    payload = Payload(liveVerify=True, runtimeSecrets={"PRODUCT_KEY": secret})
    with pytest.raises(InputError, match="credential"):
        read_output(output, "test", payload)


def test_provider_secret_and_private_error_redaction(tmp_path, monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-provider-secret")
    output = tmp_path / "output"
    output.mkdir()
    (output / "feedback.md").write_text("fake-provider-secret")
    with pytest.raises(InputError, match="credential"):
        read_output(output, "test", Payload())
    assert (
        private_error_message(
            "error fake-provider-secret product-secret",
            Payload(liveVerify=True, runtimeSecrets={"PRODUCT_KEY": "product-secret"}),
        )
        == "error [REDACTED] [REDACTED]"
    )


def test_endpoint_allowlist_and_custom_adapter(tmp_path, monkeypatch):
    monkeypatch.setenv(
        "DARI_DOCS_ALLOWED_MODEL_ENDPOINTS", "https://approved.example/v1"
    )
    monkeypatch.setenv(
        "DARI_DOCS_CUSTOM_ADAPTERS", "my_agents:Agent,my_env:Environment"
    )
    payload = Payload(
        task="test",
        execution={
            "harness": "my_agents:Agent",
            "sandbox": "my_env:Environment",
            "agentEnv": {"ANTHROPIC_BASE_URL": "https://approved.example/v1"},
        },
    )
    task = materialize_task(tmp_path, "test", payload)
    config = trial_config(task, tmp_path, payload)
    assert config.agent.env["ANTHROPIC_BASE_URL"] == "https://approved.example/v1"
    assert config.environment.import_path == "my_env:Environment"


def test_ignore_unrelated_binary_but_reject_binary_docs(tmp_path):
    output = tmp_path / "output"
    output.mkdir()
    (output / "feedback.md").write_text("ok")
    (output / "screenshot.png").write_bytes(b"\xff\xfe")
    assert read_output(output, "test", Payload()) == {"feedback": "ok"}
    (output / "feedback.md").write_bytes(b"\xff\xfe")
    with pytest.raises(InputError, match="UTF-8"):
        read_output(output, "test", Payload())


def test_modal_lifetime_cap_and_deployment_kwargs(tmp_path, monkeypatch):
    monkeypatch.setenv(
        "DARI_DOCS_ENVIRONMENT_KWARGS", '{"region":"us-east","app_name":"operator-app"}'
    )
    payload = Payload(
        task="test", execution={"environmentKwargs": {"sandbox_timeout_secs": 9000}}
    )
    task = materialize_task(tmp_path, "test", payload)
    config = trial_config(task, tmp_path, payload)
    assert config.environment.kwargs["sandbox_timeout_secs"] == 3300
    assert config.environment.kwargs["region"] == "us-east"

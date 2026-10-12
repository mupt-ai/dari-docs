"""Opt-in real Harbor/Docker smoke tests, without inference credentials."""

import asyncio
import os

import pytest
from harbor.agents.base import BaseAgent

from harbor_runner import Payload, run_workflow


class ArtifactAgent(BaseAgent):
    @staticmethod
    def name():
        return "artifact-smoke"

    def version(self):
        return "1"

    async def setup(self, environment):
        pass

    async def run(self, instruction, environment, context):
        result = await environment.exec(
            'test -f /app/input-docs/files/README.md && test -z "$DARI_DOCS_RUNNER_TOKEN" && test -z "$MODAL_TOKEN_SECRET"'
        )
        if result.return_code:
            raise RuntimeError("workspace or credential isolation failed")
        if "Write concise Markdown feedback" in instruction:
            await environment.exec("printf 'Smoke feedback' > /app/output/feedback.md")
        else:
            await environment.exec(
                "printf 'Fixed docs' > /app/output/updated-docs/files/README.md; printf 'Fixed README' > /app/output/changelog.md"
            )


@pytest.mark.skipif(
    os.environ.get("RUN_HARBOR_DOCKER_TESTS") != "1", reason="requires local Docker"
)
@pytest.mark.asyncio
@pytest.mark.parametrize("workflow", ["test", "edit"])
async def test_real_trial_artifact_collection_and_teardown(workflow, monkeypatch):
    monkeypatch.setenv("DARI_DOCS_CUSTOM_ADAPTERS", "test_docker:ArtifactAgent")
    monkeypatch.setenv("DARI_DOCS_RUNNER_TOKEN", "controller-secret")
    monkeypatch.setenv("MODAL_TOKEN_SECRET", "controller-secret")
    payload = Payload(
        task="test",
        feedback="fix it",
        files=[{"path": "README.md", "content": "Original docs"}],
        execution={"harness": "test_docker:ArtifactAgent", "sandbox": "docker"},
    )
    events = []

    async def emit(event):
        events.append(event)

    result = await run_workflow(workflow, payload, emit)
    if workflow == "test":
        assert result == {"feedback": "Smoke feedback"}
    else:
        assert result == {
            "changelog": "Fixed README",
            "files": [{"path": "README.md", "content": "Fixed docs"}],
        }
    assert {e["event"] for e in events} >= {
        "start",
        "agent-start",
        "agent-end",
        "end",
        "collecting-result",
    }


@pytest.mark.skipif(
    os.environ.get("RUN_HARBOR_DOCKER_TESTS") != "1", reason="requires local Docker"
)
@pytest.mark.asyncio
async def test_concurrent_trials_do_not_collide(monkeypatch):
    monkeypatch.setenv("DARI_DOCS_CUSTOM_ADAPTERS", "test_docker:ArtifactAgent")

    async def emit(event):
        pass

    payload = Payload(
        task="test",
        files=[{"path": "README.md", "content": "docs"}],
        execution={"harness": "test_docker:ArtifactAgent", "sandbox": "docker"},
    )
    results = await asyncio.gather(
        run_workflow("test", payload, emit), run_workflow("test", payload, emit)
    )
    assert results == [{"feedback": "Smoke feedback"}, {"feedback": "Smoke feedback"}]

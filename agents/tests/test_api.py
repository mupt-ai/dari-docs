import asyncio

import httpx
import pytest
import pytest_asyncio

from runner_api import LocalBackend, create_app


@pytest_asyncio.fixture
async def client():
    async def runner(workflow, payload, emit):
        await emit({"event": "agent-start"})
        if payload.task == "wait":
            await asyncio.sleep(60)
        if payload.task == "fail":
            raise RuntimeError("command contained secret-value")
        return {"feedback": "ok"}

    backend = LocalBackend(runner=runner)
    app = create_app("tester", backend, token="private-token")
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app),
        base_url="http://runner",
        headers={"Authorization": "Bearer private-token"},
    ) as client:
        yield client
    await backend.close()


async def settle(client, run_id):
    for _ in range(100):
        result = await client.get(f"/runs/{run_id}")
        if result.json()["status"] in {"completed", "failed", "cancelled"}:
            return result.json()
        await asyncio.sleep(0.02)
    raise AssertionError("run never settled")


@pytest.mark.asyncio
async def test_auth_and_validation(client):
    assert (
        await client.get("/healthz", headers={"Authorization": ""})
    ).status_code == 200
    assert (
        await client.post("/runs", json={}, headers={"Authorization": ""})
    ).status_code == 401
    for data in [
        {"workflow": "edit", "payload": {"feedback": "hello"}},
        {"workflow": "test", "payload": {"task": ""}},
        {
            "workflow": "test",
            "payload": {"task": "t", "runtimeSecrets": {"KEY": "secret-value"}},
        },
        {
            "workflow": "test",
            "payload": {
                "task": "t",
                "execution": {"agentEnv": {"OPENAI_API_KEY": "secret-value"}},
            },
        },
    ]:
        response = await client.post("/runs", json=data)
        assert response.status_code == 422
        assert "secret-value" not in response.text
    assert (await client.get("/runs/missing")).status_code == 404


@pytest.mark.asyncio
async def test_async_results_events_and_secrets(client):
    response = await client.post(
        "/runs",
        json={
            "workflow": "test",
            "payload": {
                "task": "go",
                "liveVerify": True,
                "runtimeSecrets": {"PRODUCT_KEY": "secret-value"},
            },
        },
    )
    assert response.status_code == 202
    run_id = response.json()["runId"]
    state = await settle(client, run_id)
    assert state["status"] == "completed"
    assert "secret-value" not in str(state)
    result = await client.get(f"/runs/{run_id}/result")
    assert result.json() == {"result": {"feedback": "ok"}}
    events = (await client.get(f"/runs/{run_id}/events")).json()
    assert [e["event"] for e in events["events"]] == [
        "running",
        "agent-start",
        "completed",
    ]
    assert (await client.get(f"/runs/{run_id}/events?after={events['next']}")).json()[
        "events"
    ] == []
    assert (await client.get(f"/runs/{run_id}/events?after=-1")).status_code == 422
    assert (await client.delete(f"/runs/{run_id}")).status_code == 409


@pytest.mark.asyncio
async def test_cancellation(client):
    response = await client.post(
        "/runs", json={"workflow": "test", "payload": {"task": "wait"}}
    )
    run_id = response.json()["runId"]
    assert (await client.get(f"/runs/{run_id}/result")).status_code == 409
    await asyncio.sleep(0.03)
    assert (await client.delete(f"/runs/{run_id}")).status_code == 202
    assert (await settle(client, run_id))["status"] == "cancelled"


@pytest.mark.asyncio
async def test_blocking_compatibility_and_safe_errors(client):
    response = await client.post("/workflows/test?wait=result", json={"task": "go"})
    assert response.status_code == 200
    assert response.json() == {"result": {"feedback": "ok"}}
    response = await client.post("/workflows/test?wait=result", json={"task": "fail"})
    assert response.status_code == 502
    assert "secret-value" not in response.text


@pytest.mark.asyncio
async def test_body_limit(client, monkeypatch):
    monkeypatch.setattr("runner_api.MAX_REQUEST_BYTES", 2)
    assert (await client.post("/runs", content=b"123")).status_code == 413

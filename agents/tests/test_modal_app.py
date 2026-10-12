"""Exercise Modal scheduling/state contracts without provisioning resources."""

from types import SimpleNamespace

import pytest

import modal_app
from harbor_runner import Payload


class MemoryDict:
    def __init__(self):
        self.values = {}
        self.get = SimpleNamespace(aio=self._get)
        self.put = SimpleNamespace(aio=self._put)

    async def _get(self, key, default=None):
        return self.values.get(key, default)

    async def _put(self, key, value):
        self.values[key] = value


@pytest.mark.asyncio
async def test_modal_submit_detaches_and_cancel_preserves_state(monkeypatch):
    store = MemoryDict()
    scheduled = []

    async def spawn(run_id, workflow, payload):
        scheduled.append((run_id, workflow, payload))

    monkeypatch.setattr(modal_app, "runs", store)
    monkeypatch.setattr(
        modal_app, "execute", SimpleNamespace(spawn=SimpleNamespace(aio=spawn))
    )
    backend = modal_app.ModalBackend()
    run_id = await backend.submit("test", Payload(task="test"))
    state = await backend.get(run_id)
    assert state["status"] == "queued"
    assert "payload" not in state
    assert scheduled[0][0] == run_id
    await backend.cancel(run_id)
    assert await backend.cancel_requested(run_id)
    assert (await backend.get(run_id))["status"] == "queued"

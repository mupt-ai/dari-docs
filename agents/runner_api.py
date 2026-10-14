"""HTTP compatibility and asynchronous run observation for the Harbor runner."""

from __future__ import annotations

import asyncio
import hmac
import json
import logging
import os
import time
from collections.abc import Callable
from contextlib import asynccontextmanager
from typing import Any, Literal
from uuid import uuid4

from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel, ConfigDict, ValidationError

from harbor_runner import InputError, Payload, run_workflow, workflow_instruction

MAX_REQUEST_BYTES = 10 * 1024 * 1024
TERMINAL = {"completed", "failed", "cancelled"}


class RunRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    workflow: Literal["test", "edit"]
    payload: Payload


def initial_state(workflow: str) -> dict[str, Any]:
    return {
        "workflow": workflow,
        "status": "queued",
        "createdAt": time.time(),
        "events": [],
        "result": None,
        "error": None,
    }


async def execute_run(
    backend, run_id: str, workflow: str, payload: Payload, runner=run_workflow
):
    state = await backend.get(run_id)
    state["status"] = "running"
    state["startedAt"] = time.time()
    await backend.put(run_id, state)

    async def emit(event):
        state["events"].append(
            {"sequence": len(state["events"]) + 1, "timestamp": time.time(), **event}
        )
        await backend.put(run_id, state)

    await emit({"event": "running"})
    task = asyncio.create_task(runner(workflow, payload, emit))

    async def check_cancel():
        while not task.done():
            if await backend.cancel_requested(run_id):
                task.cancel()
                return
            await asyncio.sleep(0.5)

    watcher = asyncio.create_task(check_cancel())
    try:
        state["result"] = await asyncio.wait_for(task, timeout=3300)
        state["status"] = "completed"
    except asyncio.CancelledError:
        state["status"] = "cancelled"
        if asyncio.current_task().cancelling():
            raise
    except Exception as exc:
        state["status"] = "failed"
        logging.getLogger(__name__).error(
            "Run %s failed: %s", run_id, type(exc).__name__
        )
        # Harbor exception messages/tracebacks may echo command arguments or credentials.
        state["error"] = (
            str(exc)
            if isinstance(exc, InputError)
            else "Execution failed; inspect private controller logs"
        )
    finally:
        watcher.cancel()
        await asyncio.gather(watcher, return_exceptions=True)
        state["finishedAt"] = time.time()
        await emit({"event": state["status"]})


class LocalBackend:
    """Single-process development backend; runs do not survive process restarts."""

    def __init__(self, runner=run_workflow, concurrency: int = 30):
        self.states: dict[str, dict[str, Any]] = {}
        self.cancelled: set[str] = set()
        self.tasks: dict[str, asyncio.Task] = {}
        self.runner = runner
        self.slots = asyncio.Semaphore(concurrency)

    async def submit(self, workflow: str, payload: Payload) -> str:
        # Bound local memory; completed runs are retained for one hour.
        for key, state in list(self.states.items()):
            if state.get("finishedAt", time.time()) < time.time() - 3600:
                self.states.pop(key)
                self.cancelled.discard(key)
        if len(self.states) >= 1000:
            raise HTTPException(429, "Local run capacity reached")
        run_id = str(uuid4())
        self.states[run_id] = initial_state(workflow)

        async def worker():
            async with self.slots:
                if run_id in self.cancelled:
                    state = self.states[run_id]
                    state.update(status="cancelled", finishedAt=time.time())
                    return
                await execute_run(self, run_id, workflow, payload, self.runner)

        task = asyncio.create_task(worker())
        self.tasks[run_id] = task
        task.add_done_callback(lambda _: self.tasks.pop(run_id, None))
        return run_id

    async def get(self, run_id):
        return self.states.get(run_id)

    async def put(self, run_id, state):
        self.states[run_id] = state

    async def cancel_requested(self, run_id):
        return run_id in self.cancelled

    async def cancel(self, run_id):
        self.cancelled.add(run_id)

    async def close(self):
        for task in list(self.tasks.values()):
            task.cancel()
        await asyncio.gather(*list(self.tasks.values()), return_exceptions=True)


def create_app(service: str, backend, token: str | None = None) -> FastAPI:
    workflow = {"tester": "test", "editor": "edit"}[service]
    token = token if token is not None else os.environ.get("DARI_DOCS_RUNNER_TOKEN", "")

    @asynccontextmanager
    async def lifespan(app):
        yield
        if hasattr(backend, "close"):
            await backend.close()

    app = FastAPI(lifespan=lifespan)

    @app.middleware("http")
    async def authenticate(request: Request, call_next: Callable):
        if request.url.path != "/healthz" and token:
            from fastapi.responses import JSONResponse

            if not hmac.compare_digest(
                request.headers.get("authorization", ""), f"Bearer {token}"
            ):
                return JSONResponse({"detail": "Unauthorized"}, status_code=401)
        return await call_next(request)

    async def body(request: Request) -> dict:
        chunks = []
        size = 0
        async for chunk in request.stream():
            size += len(chunk)
            if size > MAX_REQUEST_BYTES:
                raise HTTPException(413, "Request exceeds size limit")
            chunks.append(chunk)
        try:
            parsed = json.loads(b"".join(chunks))
            if not isinstance(parsed, dict):
                raise ValueError
            return parsed
        except (ValueError, UnicodeError):
            raise HTTPException(422, "Request must be a JSON object") from None

    def validate(raw, asynchronous=False):
        try:
            submitted = (
                RunRequest.model_validate(raw)
                if asynchronous
                else RunRequest(workflow=workflow, payload=raw)
            )
            if submitted.workflow != workflow:
                raise InputError("wrong workflow for this service")
            workflow_instruction(workflow, submitted.payload)
            return submitted
        except (ValueError, ValidationError):
            # Pydantic errors include input values; do not echo secret payloads.
            raise HTTPException(
                422,
                "Invalid workflow payload; check required fields, paths and execution settings",
            ) from None

    async def state_for(run_id):
        state = await backend.get(run_id)
        if state is None or state["workflow"] != workflow:
            raise HTTPException(404, "Run not found")
        if (
            state["status"] == "running"
            and time.time() - state.get("startedAt", state["createdAt"]) > 3700
        ):
            state = {
                **state,
                "status": "failed",
                "error": "Controller exceeded its execution deadline",
            }
        return state

    @app.get("/healthz")
    def health():
        return {"ok": True, "service": service, "runtime": "harbor"}

    @app.post("/runs", status_code=202)
    async def submit(request: Request):
        submitted = validate(await body(request), asynchronous=True)
        run_id = await backend.submit(workflow, submitted.payload)
        return {
            "runId": run_id,
            "statusUrl": f"/runs/{run_id}",
            "resultUrl": f"/runs/{run_id}/result",
            "eventsUrl": f"/runs/{run_id}/events",
        }

    @app.get("/runs/{run_id}")
    async def status(run_id: str):
        state = await state_for(run_id)
        return {
            "runId": run_id,
            **{
                key: value
                for key, value in state.items()
                if key not in {"events", "result"}
            },
        }

    @app.get("/runs/{run_id}/events")
    async def events(run_id: str, after: int = 0):
        if after < 0:
            raise HTTPException(422, "after must be nonnegative")
        state = await state_for(run_id)
        page = [event for event in state["events"] if event["sequence"] > after][:100]
        return {
            "events": page,
            "next": page[-1]["sequence"] if page else after,
            "status": state["status"],
        }

    @app.get("/runs/{run_id}/result")
    async def result(run_id: str):
        state = await state_for(run_id)
        if state["status"] != "completed":
            raise HTTPException(409, state.get("error") or f"Run is {state['status']}")
        return {"result": state["result"]}

    @app.delete("/runs/{run_id}", status_code=202)
    async def cancel(run_id: str):
        state = await state_for(run_id)
        if state["status"] in TERMINAL:
            raise HTTPException(409, "Run is already finished")
        await backend.cancel(run_id)
        return {"runId": run_id, "status": "cancellation-requested"}

    @app.post(f"/workflows/{workflow}")
    async def blocking(request: Request):
        submitted = validate(await body(request))
        run_id = await backend.submit(workflow, submitted.payload)
        try:
            while True:
                state = await state_for(run_id)
                if state["status"] == "completed":
                    return {"result": state["result"]}
                if state["status"] in TERMINAL:
                    raise HTTPException(502, state.get("error") or "Run cancelled")
                if await request.is_disconnected():
                    await backend.cancel(run_id)
                    raise HTTPException(499, "Client disconnected")
                await asyncio.sleep(0.25)
        except asyncio.CancelledError:
            await backend.cancel(run_id)
            raise

    return app

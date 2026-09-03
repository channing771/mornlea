from __future__ import annotations

import asyncio
import json
from collections.abc import Awaitable
from copy import deepcopy
from pathlib import Path

from langgraph.graph import StateGraph

from mornlea_companion_agent.domain.dialogue import DialogueMessage
from mornlea_companion_agent.domain.http_v1 import DialogueTerminalRequest
from mornlea_companion_agent.domain.memory import (
    LeaseIdentity,
    MemoryReconcile,
    MemoryStateNonzero,
)
from mornlea_companion_agent.harness.dialogue import DialogueHarness
from mornlea_companion_agent.storage.sqlite_memory import SQLiteMemoryStore

REPOSITORY_ROOT = Path(__file__).resolve().parents[4]
HTTP_GOLDEN = REPOSITORY_ROOT / "contracts/companion-agent/http-v1/golden/valid.json"


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def terminal_request() -> DialogueTerminalRequest:
    document = json.loads(HTTP_GOLDEN.read_text(encoding="utf-8"))
    value = deepcopy(
        next(
            case["value"]
            for case in document["cases"]
            if case["name"] == "terminal dialogue request carries completed fact"
        )
    )
    value["persona"] = "UNIQUE_PERSONA_NOT_PERSISTED"
    value["fact_node"] = {"kind": "terminal", "state": "completed", "reason": "none"}
    value["environment"] = {
        "exposed_blocks": [{"position": {"x": 17, "y": 63, "z": -9}, "block_id": 2}],
        "heights": [{"x": 17, "z": -9, "height": 63}],
    }
    return DialogueTerminalRequest.model_validate(value)


class StaticModel:
    async def complete(self, messages: tuple[DialogueMessage, ...]) -> object:
        del messages
        return '{"line":"UNIQUE_LINE_NOT_PERSISTED","summary":"UNIQUE_PROPOSAL_NOT_COMMITTED"}'


def test_dialogue_compiles_a_fresh_graph_without_checkpoint(
    monkeypatch,
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        request = terminal_request()
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3")
        identity = LeaseIdentity(
            namespace_id=request.namespace_id,
            client_instance_id=request.client_instance_id,
            lease_id=request.lease_id,
        )
        await store.acquire_namespace(
            request.namespace_id,
            request.client_instance_id,
            request.lease_id,
        )
        await store.reconcile(
            identity,
            MemoryReconcile(
                namespace_id=request.namespace_id,
                companion_id=request.companion_id,
                memory_epoch=request.memory_epoch,
                active=True,
                tombstone_operation_id=None,
                mirror=MemoryStateNonzero(
                    revision=3,
                    operation_id="88888888-8888-4888-8888-888888888888",
                    summary="ALLOWED_COMPACT_RUNTIME_SUMMARY",
                ),
            ),
        )
        compile_calls: list[object] = []
        original = StateGraph.compile

        def spy_compile(self, *args, **kwargs):
            compile_calls.append(kwargs.get("checkpointer", "missing"))
            return original(self, *args, **kwargs)

        monkeypatch.setattr(StateGraph, "compile", spy_compile)
        operations = iter(
            (
                "99999999-9999-4999-8999-999999999991",
                "99999999-9999-4999-8999-999999999992",
            )
        )
        harness = DialogueHarness(
            StaticModel(),
            store,
            wall_clock=lambda: 1_700_000_000.0,
            operation_id_factory=lambda: next(operations),
        )
        first = await harness.run(request)
        second = await harness.run(request)
        assert first.memory_proposal.operation_id != second.memory_proposal.operation_id
        assert compile_calls == [None, None]
        await store.close()

        persisted = b""
        for suffix in ("", "-wal", "-shm"):
            path = tmp_path / f"memory.sqlite3{suffix}"
            if path.exists():
                persisted += path.read_bytes()
        for forbidden in (
            b"UNIQUE_PERSONA_NOT_PERSISTED",
            b"UNIQUE_LINE_NOT_PERSISTED",
            b"UNIQUE_PROPOSAL_NOT_COMMITTED",
            b"99999999-9999-4999-8999-999999999991",
            b"99999999-9999-4999-8999-999999999992",
            b'"fact_node"',
            b'"environment"',
            b'"messages"',
            b'"checkpoint"',
        ):
            assert forbidden not in persisted

    run(scenario())

from __future__ import annotations

import asyncio
import json
from collections.abc import Awaitable
from copy import deepcopy
from pathlib import Path

from harness.agents.companion.dialogue_factory import DialogueHarness
from harness.domain.dialogue import DialogueMessage
from harness.domain.http_v1 import DialogueTerminalRequest
from harness.domain.memory import (
    LeaseIdentity,
    MemoryLookup,
    MemoryStateNonzero,
)
from langgraph.graph import StateGraph

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
HTTP_GOLDEN = REPOSITORY_ROOT / "packages/contracts/companion-agent/http-v1/golden/valid.json"


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


class StaticMemoryReader:
    """返回固定 nonzero 记忆的只读 double（只记录 load 流量，不做任何持久化）。"""

    def __init__(self) -> None:
        self.loads: list[tuple[LeaseIdentity, MemoryLookup]] = []

    async def load(
        self,
        identity: LeaseIdentity,
        lookup: MemoryLookup,
    ) -> MemoryStateNonzero:
        self.loads.append((identity, lookup))
        return MemoryStateNonzero(
            revision=3,
            operation_id="88888888-8888-4888-8888-888888888888",
            summary="ALLOWED_COMPACT_RUNTIME_SUMMARY",
        )


def test_dialogue_compiles_a_fresh_graph_without_checkpoint(
    monkeypatch,
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        request = terminal_request()
        reader = StaticMemoryReader()
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
            reader,
            wall_clock=lambda: 1_700_000_000.0,
            operation_id_factory=lambda: next(operations),
        )
        first = await harness.run(request)
        second = await harness.run(request)
        assert first.memory_proposal.operation_id != second.memory_proposal.operation_id
        assert compile_calls == [None, None]
        assert len(reader.loads) == 2

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

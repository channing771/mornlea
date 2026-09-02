from __future__ import annotations

import ast
import asyncio
import json
from collections.abc import AsyncIterator, Awaitable
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Any

from langgraph.graph import StateGraph

from mornlea_companion_agent.domain.http_v1 import PlanRequest
from mornlea_companion_agent.domain.mcp_v1 import (
    GetPlanningContextResult,
    Plan,
    ValidatePlanSuccessResult,
)
from mornlea_companion_agent.domain.planner import (
    ModelOutput,
    PlannerMessage,
    PlannerModel,
    PlanningToolSession,
    PlanningToolSessionFactory,
)
from mornlea_companion_agent.harness.planner import PlannerHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
SERVICE_ROOT = Path(__file__).resolve().parents[1]


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def golden_value(contract: str, name: str) -> dict[str, Any]:
    path = REPOSITORY_ROOT / f"contracts/companion-agent/{contract}/golden/valid.json"
    document = json.loads(path.read_text(encoding="utf-8"))
    return next(case["value"] for case in document["cases"] if case["name"] == name)


def request() -> PlanRequest:
    return PlanRequest.model_validate(
        golden_value("http-v1", "planner run carries snapshot identity")
    )


def context() -> GetPlanningContextResult:
    current = request()
    value = golden_value("mcp-v1", "planning context is a bounded projection")
    value["instruction"] = current.instruction
    return GetPlanningContextResult.model_validate(value)


def candidate() -> Plan:
    value = golden_value("mcp-v1", "validator accepted result echoes digest and canonical plan")
    return Plan.model_validate(value["plan"])


class OneShotModel(PlannerModel):
    def __init__(self) -> None:
        self.calls = 0

    async def complete(
        self,
        messages: tuple[PlannerMessage, ...],
        *,
        allow_tools: bool,
    ) -> ModelOutput:
        del messages, allow_tools
        self.calls += 1
        return ModelOutput(
            content=json.dumps(candidate().model_dump(mode="json"), ensure_ascii=False),
            tool_calls=(),
        )


class OneShotTools(PlanningToolSession):
    async def get_planning_context(self) -> GetPlanningContextResult:
        return context()

    async def call_model_tool(self, name: str, arguments: dict[str, object]) -> object:
        raise AssertionError((name, arguments))

    async def validate_plan(self, plan: Plan) -> ValidatePlanSuccessResult:
        return ValidatePlanSuccessResult(
            accepted=True,
            snapshot_digest=context().snapshot_digest,
            plan=plan,
        )


class Factory(PlanningToolSessionFactory):
    @asynccontextmanager
    async def open(
        self,
        request: PlanRequest,
        *,
        timeout_seconds: float,
    ) -> AsyncIterator[PlanningToolSession]:
        del request, timeout_seconds
        yield OneShotTools()


def test_each_invoke_compiles_without_checkpointer_and_has_fresh_state(monkeypatch: Any) -> None:
    async def scenario() -> None:
        observed: list[object] = []
        original = StateGraph.compile

        def recording_compile(self: StateGraph[Any], checkpointer: object = None, **kwargs: Any):
            observed.append(checkpointer)
            return original(self, checkpointer=checkpointer, **kwargs)

        monkeypatch.setattr(StateGraph, "compile", recording_compile)
        model = OneShotModel()
        harness = PlannerHarness(model, Factory(), wall_clock=lambda: 1_700_000_000.0)
        first = await harness.run(request())
        second = await harness.run(request())
        assert first.plan == second.plan == candidate()
        assert model.calls == 2
        assert observed == [None, None]

    run(scenario())


def test_planner_sources_do_not_import_checkpoint_or_persistence_modules() -> None:
    for relative in ("domain/planner.py", "harness/planner.py"):
        path = SERVICE_ROOT / "src/mornlea_companion_agent" / relative
        tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        imports = {
            alias.name
            for node in ast.walk(tree)
            if isinstance(node, ast.Import)
            for alias in node.names
        } | {node.module or "" for node in ast.walk(tree) if isinstance(node, ast.ImportFrom)}
        assert all(
            forbidden not in imported
            for imported in imports
            for forbidden in ("sqlite", "checkpoint", ".storage", "aiosqlite")
        )

"""伙伴规划路由：单次有界计划生成。"""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any, cast

from fastapi import Request, Response
from harness.domain.http_v1 import PlanRequest

from app.gateway.http_gate import _request_value, _response
from app.gateway.runtime import _AgentRuntime


def build_plan_routes(
    runtime: _AgentRuntime,
) -> list[tuple[str, Callable[..., Coroutine[Any, Any, Response]], list[str]]]:
    """组装伙伴规划路由，保持预算与回执语义不变。"""

    async def plan(request: Request) -> Response:
        value = cast(PlanRequest, _request_value(request, PlanRequest))
        return _response(await runtime.run_planner(value))

    return [
        ("/v1/plan", plan, ["POST"]),
    ]

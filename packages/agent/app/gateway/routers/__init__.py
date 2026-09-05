"""网关路由聚合：按健康、租约、计划、台词、记忆、运行顺序装配 11 条路由。"""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any

from fastapi import Response

from app.gateway.routers.companion_dialogue import build_dialogue_routes
from app.gateway.routers.companion_memory import build_memory_routes
from app.gateway.routers.companion_plan import build_plan_routes
from app.gateway.routers.health import build_health_routes
from app.gateway.routers.namespaces import build_namespace_routes
from app.gateway.routers.runs import build_run_routes
from app.gateway.runtime import _AgentRuntime


def build_routers(
    runtime: _AgentRuntime,
) -> list[tuple[str, Callable[..., Coroutine[Any, Any, Response]], list[str]]]:
    """聚合全部路由，分组顺序与原应用注册顺序一致。"""

    return [
        *build_health_routes(runtime),
        *build_namespace_routes(runtime),
        *build_plan_routes(runtime),
        *build_dialogue_routes(runtime),
        *build_memory_routes(runtime),
        *build_run_routes(runtime),
    ]


__all__ = ["build_routers"]

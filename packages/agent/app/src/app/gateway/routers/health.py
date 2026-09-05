"""健康检查路由：存活与就绪探针。"""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any, cast

from fastapi import Response
from harness.domain.http_v1 import LiveResponse, ReadyResponse

from app.gateway.http_gate import _response
from app.gateway.runtime import _AgentRuntime


def build_health_routes(
    runtime: _AgentRuntime,
) -> list[tuple[str, Callable[..., Coroutine[Any, Any, Response]], list[str]]]:
    """组装健康检查路由，保持探针语义不变。"""

    async def live() -> Response:
        return _response(LiveResponse(status="live"))

    async def ready() -> Response:
        status = "ready" if runtime.is_ready else "not_ready"
        return _response(
            ReadyResponse(status=cast(Any, status)),
            status_code=200 if status == "ready" else 503,
        )

    return [
        ("/livez", live, ["GET"]),
        ("/readyz", ready, ["GET"]),
    ]

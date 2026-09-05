"""运行取消路由：取消进行中的计划或台词运行。"""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any, cast

from fastapi import Request, Response
from harness.domain.http_v1 import CancelRequest, CancelResponse

from app.gateway.http_gate import _HTTPFailure, _request_value, _response
from app.gateway.runtime import _AgentRuntime, _lease_identity


def build_run_routes(
    runtime: _AgentRuntime,
) -> list[tuple[str, Callable[..., Coroutine[Any, Any, Response]], list[str]]]:
    """组装运行取消路由，保持取消语义不变。"""

    async def cancel(request: Request) -> Response:
        value = cast(CancelRequest, _request_value(request, CancelRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            cancelled = await leases.cancel_run(_lease_identity(value), value.run_id)
        return _response(CancelResponse(**value.model_dump(), cancelled=cancelled))

    return [
        ("/v1/runs/cancel", cancel, ["POST"]),
    ]

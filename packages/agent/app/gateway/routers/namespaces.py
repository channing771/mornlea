"""命名空间租约路由：获取、心跳与释放。"""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any, cast

from fastapi import Request, Response
from harness.domain.http_v1 import (
    AcquireRequest,
    AcquireResponse,
    HeartbeatResponse,
    LeaseRequest,
    ReleaseResponse,
)

from app.gateway.http_gate import _HTTPFailure, _request_value, _response
from app.gateway.runtime import _AgentRuntime, _lease_identity


def build_namespace_routes(
    runtime: _AgentRuntime,
) -> list[tuple[str, Callable[..., Coroutine[Any, Any, Response]], list[str]]]:
    """组装命名空间租约路由，保持租约语义不变。"""

    async def acquire(request: Request) -> Response:
        value = cast(AcquireRequest, _request_value(request, AcquireRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            grant = await leases.acquire(value.namespace_id, value.client_instance_id)
        return _response(
            AcquireResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                **grant.model_dump(),
            )
        )

    async def heartbeat(request: Request) -> Response:
        value = cast(LeaseRequest, _request_value(request, LeaseRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            grant = await leases.heartbeat(_lease_identity(value))
        return _response(
            HeartbeatResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                **grant.model_dump(),
            )
        )

    async def release(request: Request) -> Response:
        value = cast(LeaseRequest, _request_value(request, LeaseRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            await leases.release(_lease_identity(value))
        return _response(ReleaseResponse(**value.model_dump(), released=True))

    return [
        ("/v1/namespaces/acquire", acquire, ["POST"]),
        ("/v1/namespaces/heartbeat", heartbeat, ["POST"]),
        ("/v1/namespaces/release", release, ["POST"]),
    ]

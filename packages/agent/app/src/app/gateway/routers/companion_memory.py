"""伙伴记忆路由：对账、提交与删除。"""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any, cast

from fastapi import Request, Response
from harness.domain.http_v1 import (
    MemoryCommitRequest,
    MemoryCommitResponse,
    MemoryDeleteRequest,
    MemoryDeleteResponse,
    MemoryReconcileActiveRequest,
    MemoryReconcileActiveResponse,
    MemoryReconcileInactiveRequest,
    MemoryReconcileInactiveResponse,
)
from harness.domain.memory import MemoryCommit, MemoryDelete, MemoryReconcile

from app.gateway.http_gate import _HTTPFailure, _request_value, _response
from app.gateway.runtime import _AgentRuntime, _lease_identity


def build_memory_routes(
    runtime: _AgentRuntime,
) -> list[tuple[str, Callable[..., Coroutine[Any, Any, Response]], list[str]]]:
    """组装伙伴记忆路由，保持记忆语义不变。"""

    async def reconcile(request: Request) -> Response:
        raw = getattr(request.state, "contract_request", None)
        if not isinstance(
            raw,
            (MemoryReconcileActiveRequest, MemoryReconcileInactiveRequest),
        ):
            raise _HTTPFailure("internal_error")
        value = raw
        async with runtime.business() as components:
            record = await components.store.reconcile(
                _lease_identity(value),
                MemoryReconcile(
                    namespace_id=value.namespace_id,
                    companion_id=value.companion_id,
                    memory_epoch=value.memory_epoch,
                    active=value.active,
                    tombstone_operation_id=value.tombstone_operation_id,
                    mirror=value.mirror,
                ),
            )
        if record.active:
            if record.memory is None:
                raise _HTTPFailure("internal_error")
            return _response(
                MemoryReconcileActiveResponse(
                    contract_version=value.contract_version,
                    request_id=value.request_id,
                    client_instance_id=value.client_instance_id,
                    namespace_id=value.namespace_id,
                    lease_id=value.lease_id,
                    companion_id=value.companion_id,
                    memory_epoch=record.memory_epoch,
                    active=True,
                    tombstone_operation_id=None,
                    memory=record.memory,
                )
            )
        if record.tombstone_operation_id is None:
            raise _HTTPFailure("internal_error")
        return _response(
            MemoryReconcileInactiveResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                client_instance_id=value.client_instance_id,
                namespace_id=value.namespace_id,
                lease_id=value.lease_id,
                companion_id=value.companion_id,
                memory_epoch=record.memory_epoch,
                active=False,
                tombstone_operation_id=record.tombstone_operation_id,
                memory=None,
            )
        )

    async def commit(request: Request) -> Response:
        value = cast(
            MemoryCommitRequest,
            _request_value(request, MemoryCommitRequest),
        )
        async with runtime.business() as components:
            result = await components.store.commit(
                _lease_identity(value),
                MemoryCommit(
                    namespace_id=value.namespace_id,
                    companion_id=value.companion_id,
                    memory_epoch=value.memory_epoch,
                    base_revision=value.base_revision,
                    operation_id=value.operation_id,
                    summary=value.summary,
                ),
            )
        return _response(
            MemoryCommitResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                client_instance_id=value.client_instance_id,
                lease_id=value.lease_id,
                namespace_id=result.namespace_id,
                companion_id=result.companion_id,
                memory_epoch=result.memory_epoch,
                operation_id=result.operation_id,
                committed_revision=result.committed_revision,
            )
        )

    async def delete(request: Request) -> Response:
        value = cast(
            MemoryDeleteRequest,
            _request_value(request, MemoryDeleteRequest),
        )
        async with runtime.business() as components:
            record = await components.store.delete(
                _lease_identity(value),
                MemoryDelete(
                    namespace_id=value.namespace_id,
                    companion_id=value.companion_id,
                    old_memory_epoch=value.old_memory_epoch,
                    new_memory_epoch=value.new_memory_epoch,
                    tombstone_operation_id=value.tombstone_operation_id,
                ),
            )
        if record.tombstone_operation_id is None:
            raise _HTTPFailure("internal_error")
        return _response(
            MemoryDeleteResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                client_instance_id=value.client_instance_id,
                namespace_id=value.namespace_id,
                lease_id=value.lease_id,
                companion_id=value.companion_id,
                memory_epoch=record.memory_epoch,
                tombstone_operation_id=record.tombstone_operation_id,
            )
        )

    return [
        ("/v1/memory/reconcile", reconcile, ["POST"]),
        ("/v1/memory/commit", commit, ["POST"]),
        ("/v1/memory/delete", delete, ["POST"]),
    ]

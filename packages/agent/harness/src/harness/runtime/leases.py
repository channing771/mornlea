"""namespace fencing lease 与 Planner/Dialogue 共用的无队列运行槽。"""

from __future__ import annotations

import asyncio
from collections.abc import Awaitable, Callable
from enum import StrEnum
from typing import TYPE_CHECKING, Protocol
from uuid import uuid4

from harness.domain.memory import (
    HEARTBEAT_INTERVAL_MS,
    LEASE_TTL_MS,
    ExpiredLease,
    LeaseGrant,
    LeaseIdentity,
    LeaseIDReuse,
    LeaseTransition,
)

if TYPE_CHECKING:
    from harness.runtime.run_gate import RunGate, RunHandle


GLOBAL_RUN_LIMIT = 4
PER_COMPANION_RUN_LIMIT = 1


async def _drain_mandatory[T](
    awaitable: Awaitable[T],
) -> tuple[T, asyncio.CancelledError | None]:
    """延迟外层取消，直到已经提交的生命周期动作完成必要清理。"""

    owned = asyncio.ensure_future(awaitable)
    cancellation: asyncio.CancelledError | None = None
    while not owned.done():
        try:
            await asyncio.shield(owned)
        except asyncio.CancelledError as error:
            if cancellation is None:
                cancellation = error
    try:
        result = owned.result()
    except BaseException:
        if cancellation is not None:
            raise cancellation from None
        raise
    return result, cancellation


def _merge_cancellation(
    current: asyncio.CancelledError | None,
    candidate: asyncio.CancelledError | None,
) -> asyncio.CancelledError | None:
    return current if current is not None else candidate


class LeaseRepository(Protocol):
    async def acquire_namespace(
        self,
        namespace_id: str,
        client_instance_id: str,
        new_lease_id: str,
    ) -> LeaseTransition: ...

    async def heartbeat_namespace(self, identity: LeaseIdentity) -> LeaseGrant: ...

    async def release_namespace(self, identity: LeaseIdentity) -> None: ...

    async def assert_current_lease(self, identity: LeaseIdentity) -> None: ...

    async def expire_namespaces(self) -> tuple[ExpiredLease, ...]: ...


class RunKind(StrEnum):
    PLANNER = "planner"
    DIALOGUE = "dialogue"


class NamespaceLeaseManager:
    """把 lease fence 变更与运行槽注册按固定锁序线性化。"""

    def __init__(
        self,
        repository: LeaseRepository,
        *,
        run_gate: RunGate | None = None,
        lease_id_factory: Callable[[], str] | None = None,
    ) -> None:
        from harness.runtime.run_gate import RunGate

        self._repository = repository
        self.run_gate = run_gate or RunGate()
        self._lease_id_factory = lease_id_factory or (lambda: str(uuid4()))
        self._fence_lock = asyncio.Lock()

    async def acquire(self, namespace_id: str, client_instance_id: str) -> LeaseGrant:
        async with self._fence_lock:
            for _ in range(64):
                candidate = self._lease_id_factory()
                try:
                    transition, cancellation = await _drain_mandatory(
                        self._repository.acquire_namespace(
                            namespace_id,
                            client_instance_id,
                            candidate,
                        )
                    )
                except LeaseIDReuse:
                    continue
                if transition.replaced_lease_id is not None:
                    _, cleanup_cancellation = await _drain_mandatory(
                        self.run_gate.cancel_namespace_lease(
                            namespace_id,
                            transition.replaced_lease_id,
                        )
                    )
                    cancellation = _merge_cancellation(cancellation, cleanup_cancellation)
                if cancellation is not None:
                    raise cancellation
                return transition.grant
            raise LeaseIDReuse

    async def heartbeat(self, identity: LeaseIdentity) -> LeaseGrant:
        async with self._fence_lock:
            grant, cancellation = await _drain_mandatory(
                self._repository.heartbeat_namespace(identity)
            )
            if cancellation is not None:
                raise cancellation
            return grant

    async def release(self, identity: LeaseIdentity) -> None:
        async with self._fence_lock:
            _, cancellation = await _drain_mandatory(self._repository.release_namespace(identity))
            _, cleanup_cancellation = await _drain_mandatory(self.run_gate.cancel_lease(identity))
            cancellation = _merge_cancellation(cancellation, cleanup_cancellation)
            if cancellation is not None:
                raise cancellation

    async def assert_current(self, identity: LeaseIdentity) -> None:
        async with self._fence_lock:
            await self._repository.assert_current_lease(identity)

    async def reserve_run(
        self,
        identity: LeaseIdentity,
        *,
        companion_id: str,
        run_id: str,
        kind: RunKind,
    ) -> RunHandle:
        async with self._fence_lock:
            await self._repository.assert_current_lease(identity)
            handle, cancellation = await _drain_mandatory(
                self.run_gate.try_acquire(
                    identity,
                    companion_id=companion_id,
                    run_id=run_id,
                    kind=kind,
                )
            )
            if cancellation is not None:
                _, cleanup_cancellation = await _drain_mandatory(handle.finish())
                cancellation = _merge_cancellation(cancellation, cleanup_cancellation)
                assert cancellation is not None
                raise cancellation from None
            return handle

    async def cancel_run(self, identity: LeaseIdentity, run_id: str) -> bool:
        async with self._fence_lock:
            await self._repository.assert_current_lease(identity)
            cancelled, cancellation = await _drain_mandatory(
                self.run_gate.cancel_run(identity, run_id)
            )
            if cancellation is not None:
                raise cancellation
            return cancelled

    async def expire_stale(self) -> int:
        async with self._fence_lock:
            expired, cancellation = await _drain_mandatory(self._repository.expire_namespaces())
            for identity in expired:
                _, cleanup_cancellation = await _drain_mandatory(
                    self.run_gate.cancel_lease(identity)
                )
                cancellation = _merge_cancellation(cancellation, cleanup_cancellation)
            if cancellation is not None:
                raise cancellation
            return len(expired)


__all__ = [
    "GLOBAL_RUN_LIMIT",
    "HEARTBEAT_INTERVAL_MS",
    "LEASE_TTL_MS",
    "NamespaceLeaseManager",
    "PER_COMPANION_RUN_LIMIT",
    "RunKind",
]

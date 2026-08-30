"""namespace fencing lease 与 Planner/Dialogue 共用的无队列运行槽。"""

from __future__ import annotations

import asyncio
from collections.abc import Awaitable, Callable
from enum import StrEnum
from typing import Protocol
from uuid import uuid4

from mornlea_companion_agent.domain.memory import (
    HEARTBEAT_INTERVAL_MS,
    LEASE_TTL_MS,
    ExpiredLease,
    LeaseGrant,
    LeaseIdentity,
    LeaseIDReuse,
    LeaseTransition,
    RunOverloaded,
)

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


class RunHandle:
    """持有一个运行槽；cancel 只发信号，`finish` 才安全释放槽。"""

    __slots__ = (
        "_cancelled",
        "_finished",
        "_gate",
        "_task",
        "_token",
        "companion_id",
        "kind",
        "lease",
        "run_id",
    )

    def __init__(
        self,
        gate: RunGate,
        *,
        token: object,
        lease: LeaseIdentity,
        companion_id: str,
        run_id: str,
        kind: RunKind,
    ) -> None:
        self._gate = gate
        self._token = token
        self._cancelled = asyncio.Event()
        self._finished = False
        self._task: asyncio.Task[object] | None = None
        self.lease = lease
        self.companion_id = companion_id
        self.run_id = run_id
        self.kind = kind

    @property
    def cancelled(self) -> bool:
        return self._cancelled.is_set()

    async def wait_cancelled(self) -> None:
        await self._cancelled.wait()

    def ensure_running(self) -> None:
        if self.cancelled:
            raise asyncio.CancelledError

    async def bind_task(self, task: asyncio.Task[object]) -> None:
        await self._gate._bind_task(self, task)

    async def finish(self) -> None:
        _, cancellation = await _drain_mandatory(self._gate._finish(self))
        if cancellation is not None:
            raise cancellation from None

    async def __aenter__(self) -> RunHandle:
        return self

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc_value: BaseException | None,
        traceback: object | None,
    ) -> None:
        await self.finish()

    def _cancel(self) -> None:
        self._cancelled.set()
        if self._task is not None and not self._task.done():
            self._task.cancel()


class RunGate:
    """用即时检查实现共享槽，避免 `Semaphore` 隐式形成等待队列。"""

    def __init__(
        self,
        *,
        global_limit: int = GLOBAL_RUN_LIMIT,
        per_companion_limit: int = PER_COMPANION_RUN_LIMIT,
    ) -> None:
        if type(global_limit) is not int or global_limit != GLOBAL_RUN_LIMIT:
            raise ValueError("global_limit must equal four")
        if type(per_companion_limit) is not int or per_companion_limit != 1:
            raise ValueError("per_companion_limit must equal one")
        self._global_limit = global_limit
        self._per_companion_limit = per_companion_limit
        self._lock = asyncio.Lock()
        self._by_run: dict[str, RunHandle] = {}
        self._by_companion: dict[tuple[str, str], RunHandle] = {}

    @property
    def active_count(self) -> int:
        return len(self._by_run)

    async def try_acquire(
        self,
        lease: LeaseIdentity,
        *,
        companion_id: str,
        run_id: str,
        kind: RunKind,
    ) -> RunHandle:
        from pydantic import TypeAdapter

        from mornlea_companion_agent.domain.common import UUIDv4

        canonical_companion = TypeAdapter(UUIDv4).validate_python(companion_id)
        canonical_run = TypeAdapter(UUIDv4).validate_python(run_id)
        canonical_kind = RunKind(kind)
        key = (lease.namespace_id, canonical_companion)
        async with self._lock:
            if (
                len(self._by_run) >= self._global_limit
                or key in self._by_companion
                or canonical_run in self._by_run
            ):
                raise RunOverloaded
            handle = RunHandle(
                self,
                token=object(),
                lease=lease,
                companion_id=canonical_companion,
                run_id=canonical_run,
                kind=canonical_kind,
            )
            self._by_run[canonical_run] = handle
            self._by_companion[key] = handle
            return handle

    async def cancel_run(self, lease: LeaseIdentity, run_id: str) -> bool:
        async with self._lock:
            handle = self._by_run.get(run_id)
            if handle is None or handle.lease != lease:
                return False
            handle._cancel()
            return True

    async def cancel_lease(self, lease: LeaseIdentity) -> tuple[str, ...]:
        return await self.cancel_namespace_lease(lease.namespace_id, lease.lease_id)

    async def cancel_namespace_lease(
        self,
        namespace_id: str,
        lease_id: str,
    ) -> tuple[str, ...]:
        async with self._lock:
            matches = tuple(
                handle
                for handle in self._by_run.values()
                if handle.lease.namespace_id == namespace_id and handle.lease.lease_id == lease_id
            )
            for handle in matches:
                handle._cancel()
            return tuple(handle.run_id for handle in matches)

    async def cancel_all(self) -> tuple[str, ...]:
        """关服时取消全部 run，但仍由各 runner 的 `finish` 释放槽。"""

        async with self._lock:
            matches = tuple(self._by_run.values())
            for handle in matches:
                handle._cancel()
            return tuple(handle.run_id for handle in matches)

    async def _bind_task(self, handle: RunHandle, task: asyncio.Task[object]) -> None:
        async with self._lock:
            current = self._by_run.get(handle.run_id)
            if current is not handle or current._token is not handle._token or handle._finished:
                task.cancel()
                return
            if handle._task is not None and handle._task is not task:
                raise RuntimeError("run task is already bound")
            handle._task = task
            if handle.cancelled and not task.done():
                task.cancel()

    async def _finish(self, handle: RunHandle) -> None:
        async with self._lock:
            if handle._finished:
                return
            handle._finished = True
            current = self._by_run.get(handle.run_id)
            if current is not handle or current._token is not handle._token:
                return
            self._by_run.pop(handle.run_id, None)
            key = (handle.lease.namespace_id, handle.companion_id)
            if self._by_companion.get(key) is handle:
                self._by_companion.pop(key, None)


class NamespaceLeaseManager:
    """把 lease fence 变更与运行槽注册按固定锁序线性化。"""

    def __init__(
        self,
        repository: LeaseRepository,
        *,
        run_gate: RunGate | None = None,
        lease_id_factory: Callable[[], str] | None = None,
    ) -> None:
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
    "RunGate",
    "RunHandle",
    "RunKind",
]

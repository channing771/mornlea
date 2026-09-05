"""Planner 与 Dialogue 共用的无队列运行槽。"""

from __future__ import annotations

import asyncio

from harness.domain.memory import LeaseIdentity, RunOverloaded
from harness.runtime.leases import (
    GLOBAL_RUN_LIMIT,
    PER_COMPANION_RUN_LIMIT,
    RunKind,
    _drain_mandatory,
)


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
        if self._cancelled.is_set():
            return
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

        from harness.domain.common import UUIDv4

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


__all__ = [
    "RunGate",
    "RunHandle",
]

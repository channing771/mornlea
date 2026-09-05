from __future__ import annotations

import asyncio
import sqlite3
from collections.abc import Awaitable, Callable, Iterator
from pathlib import Path

import pytest
from harness.domain.memory import (
    LeaseIdentity,
    LeaseNotFound,
    NamespaceConflict,
    RunOverloaded,
)
from harness.runtime.leases import (
    HEARTBEAT_INTERVAL_MS,
    LEASE_TTL_MS,
    NamespaceLeaseManager,
    RunKind,
)
from harness.runtime.run_gate import RunGate
from harness.store.sqlite_memory import SQLiteMemoryStore

NAMESPACE_A = "11111111-1111-4111-8111-111111111111"
NAMESPACE_B = "22222222-2222-4222-8222-222222222222"
NAMESPACE_C = "33333333-3333-4333-8333-333333333333"
NAMESPACE_D = "44444444-4444-4444-8444-444444444444"
NAMESPACE_E = "55555555-5555-4555-8555-555555555555"
CLIENT_A = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
CLIENT_B = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
COMPANION_A = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
COMPANION_B = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
LEASE_A = "10000000-0000-4000-8000-000000000001"
LEASE_B = "10000000-0000-4000-8000-000000000002"
LEASE_C = "10000000-0000-4000-8000-000000000003"
LEASE_D = "10000000-0000-4000-8000-000000000004"
LEASE_E = "10000000-0000-4000-8000-000000000005"
LEASE_F = "10000000-0000-4000-8000-000000000006"
RUN_A = "20000000-0000-4000-8000-000000000001"
RUN_B = "20000000-0000-4000-8000-000000000002"
RUN_C = "20000000-0000-4000-8000-000000000003"
RUN_D = "20000000-0000-4000-8000-000000000004"
RUN_E = "20000000-0000-4000-8000-000000000005"


class ManualClock:
    def __init__(self, now_ms: int = 1_000_000) -> None:
        self.now_ms = now_ms

    def __call__(self) -> int:
        return self.now_ms

    def advance(self, milliseconds: int) -> None:
        self.now_ms += milliseconds


class BlockingCancellationGate(RunGate):
    def __init__(self) -> None:
        super().__init__()
        self.entered = asyncio.Event()
        self.allow = asyncio.Event()
        self.armed = False

    async def cancel_namespace_lease(
        self,
        namespace_id: str,
        lease_id: str,
    ) -> tuple[str, ...]:
        if self.armed:
            self.entered.set()
            await self.allow.wait()
        return await super().cancel_namespace_lease(namespace_id, lease_id)


class BlockingReturnGate(RunGate):
    def __init__(self) -> None:
        super().__init__()
        self.entered = asyncio.Event()
        self.allow = asyncio.Event()

    async def try_acquire(
        self,
        lease: LeaseIdentity,
        *,
        companion_id: str,
        run_id: str,
        kind: RunKind,
    ):
        handle = await super().try_acquire(
            lease,
            companion_id=companion_id,
            run_id=run_id,
            kind=kind,
        )
        self.entered.set()
        await self.allow.wait()
        return handle


def uuid_sequence(*values: str) -> Callable[[], str]:
    iterator: Iterator[str] = iter(values)
    return lambda: next(iterator)


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def test_lease_constants_match_the_checked_in_http_contract() -> None:
    assert LEASE_TTL_MS == 15_000
    assert HEARTBEAT_INTERVAL_MS == 5_000


def test_acquire_conflict_and_same_owner_reacquire_create_a_new_fence(tmp_path: Path) -> None:
    async def scenario() -> None:
        clock = ManualClock()
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3", clock_ms=clock)
        manager = NamespaceLeaseManager(
            store,
            lease_id_factory=uuid_sequence(LEASE_A, LEASE_A, LEASE_B),
        )
        try:
            first = await manager.acquire(NAMESPACE_A, CLIENT_A)
            assert first.lease_id == LEASE_A
            assert first.lease_expires_in_ms == LEASE_TTL_MS

            with pytest.raises(NamespaceConflict) as captured:
                await manager.acquire(NAMESPACE_A, CLIENT_B)
            assert CLIENT_A not in str(captured.value)
            assert LEASE_A not in str(captured.value)

            old_run = await manager.reserve_run(
                LeaseIdentity(
                    namespace_id=NAMESPACE_A,
                    client_instance_id=CLIENT_A,
                    lease_id=LEASE_A,
                ),
                companion_id=COMPANION_A,
                run_id=RUN_A,
                kind=RunKind.PLANNER,
            )
            second = await manager.acquire(NAMESPACE_A, CLIENT_A)
            assert second.lease_id == LEASE_B
            assert second.lease_id != first.lease_id
            assert old_run.cancelled
            assert manager.run_gate.active_count == 1

            with pytest.raises(RunOverloaded):
                await manager.reserve_run(
                    LeaseIdentity(
                        namespace_id=NAMESPACE_A,
                        client_instance_id=CLIENT_A,
                        lease_id=LEASE_B,
                    ),
                    companion_id=COMPANION_A,
                    run_id=RUN_B,
                    kind=RunKind.DIALOGUE,
                )
            await old_run.finish()
            replacement = await manager.reserve_run(
                LeaseIdentity(
                    namespace_id=NAMESPACE_A,
                    client_instance_id=CLIENT_A,
                    lease_id=LEASE_B,
                ),
                companion_id=COMPANION_A,
                run_id=RUN_B,
                kind=RunKind.DIALOGUE,
            )
            await replacement.finish()
        finally:
            await store.close()

    run(scenario())


def test_heartbeat_release_and_old_lease_operations_are_not_found(tmp_path: Path) -> None:
    async def scenario() -> None:
        clock = ManualClock()
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3", clock_ms=clock)
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE_A))
        identity = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        try:
            await manager.acquire(NAMESPACE_A, CLIENT_A)
            clock.advance(4_000)
            renewed = await manager.heartbeat(identity)
            assert renewed.lease_expires_in_ms == LEASE_TTL_MS

            handle = await manager.reserve_run(
                identity,
                companion_id=COMPANION_A,
                run_id=RUN_A,
                kind=RunKind.DIALOGUE,
            )

            clock.advance(14_999)
            await manager.assert_current(identity)
            await manager.release(identity)
            assert handle.cancelled
            assert manager.run_gate.active_count == 1
            await handle.finish()

            for operation in (
                lambda: manager.heartbeat(identity),
                lambda: manager.release(identity),
                lambda: manager.cancel_run(identity, RUN_A),
            ):
                with pytest.raises(LeaseNotFound) as captured:
                    await operation()
                assert LEASE_A not in str(captured.value)
        finally:
            await store.close()

    run(scenario())


def test_expired_takeover_directly_cancels_the_replaced_fence(tmp_path: Path) -> None:
    async def scenario() -> None:
        clock = ManualClock()
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3", clock_ms=clock)
        manager = NamespaceLeaseManager(
            store,
            lease_id_factory=uuid_sequence(LEASE_A, LEASE_B),
        )
        identity = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        try:
            await manager.acquire(NAMESPACE_A, CLIENT_A)
            handle = await manager.reserve_run(
                identity,
                companion_id=COMPANION_A,
                run_id=RUN_A,
                kind=RunKind.PLANNER,
            )
            clock.advance(LEASE_TTL_MS)
            takeover = await manager.acquire(NAMESPACE_A, CLIENT_B)
            assert takeover.lease_id == LEASE_B
            assert handle.cancelled
            with pytest.raises(LeaseNotFound):
                await manager.heartbeat(identity)
            await handle.finish()
        finally:
            await store.close()

    run(scenario())


def test_lease_time_is_sampled_after_waiting_for_the_database_writer(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        clock = ManualClock()
        store = await SQLiteMemoryStore.open(path, clock_ms=clock)
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE_A))
        identity = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        blocker = sqlite3.connect(path)
        try:
            await manager.acquire(NAMESPACE_A, CLIENT_A)
            blocker.execute("BEGIN IMMEDIATE")
            heartbeat = asyncio.create_task(manager.heartbeat(identity))
            await asyncio.sleep(0.02)
            assert not heartbeat.done()
            clock.advance(LEASE_TTL_MS)
            blocker.rollback()
            with pytest.raises(LeaseNotFound):
                await heartbeat
        finally:
            if blocker.in_transaction:
                blocker.rollback()
            blocker.close()
            await store.close()

    run(scenario())


def test_expiry_cancels_old_runs_and_takeover_hides_the_new_owner(tmp_path: Path) -> None:
    async def scenario() -> None:
        clock = ManualClock()
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3", clock_ms=clock)
        manager = NamespaceLeaseManager(
            store,
            lease_id_factory=uuid_sequence(LEASE_A, LEASE_B),
        )
        old_identity = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        try:
            await manager.acquire(NAMESPACE_A, CLIENT_A)
            handle = await manager.reserve_run(
                old_identity,
                companion_id=COMPANION_A,
                run_id=RUN_A,
                kind=RunKind.PLANNER,
            )
            clock.advance(LEASE_TTL_MS)
            expired = await manager.expire_stale()
            assert expired == 1
            assert handle.cancelled

            takeover = await manager.acquire(NAMESPACE_A, CLIENT_B)
            assert takeover.lease_id == LEASE_B
            with pytest.raises(LeaseNotFound) as captured:
                await manager.heartbeat(old_identity)
            assert CLIENT_B not in str(captured.value)
            assert LEASE_B not in str(captured.value)
            await handle.finish()
        finally:
            await store.close()

    run(scenario())


@pytest.mark.parametrize("transition", ["acquire", "release", "expire"])
def test_persistent_lease_transition_drains_run_cancellation_before_propagating_cancel(
    tmp_path: Path,
    transition: str,
) -> None:
    async def scenario() -> None:
        clock = ManualClock()
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3", clock_ms=clock)
        gate = BlockingCancellationGate()
        manager = NamespaceLeaseManager(
            store,
            run_gate=gate,
            lease_id_factory=uuid_sequence(LEASE_A, LEASE_B),
        )
        old_identity = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        try:
            await manager.acquire(NAMESPACE_A, CLIENT_A)
            handle = await manager.reserve_run(
                old_identity,
                companion_id=COMPANION_A,
                run_id=RUN_A,
                kind=RunKind.PLANNER,
            )
            gate.armed = True
            if transition == "acquire":
                operation = asyncio.create_task(manager.acquire(NAMESPACE_A, CLIENT_A))
            elif transition == "release":
                operation = asyncio.create_task(manager.release(old_identity))
            else:
                clock.advance(LEASE_TTL_MS)
                operation = asyncio.create_task(manager.expire_stale())

            await gate.entered.wait()
            operation.cancel()
            operation.cancel()
            await asyncio.sleep(0)
            assert not operation.done()
            gate.allow.set()
            with pytest.raises(asyncio.CancelledError):
                await operation

            assert handle.cancelled
            assert manager.run_gate.active_count == 1
            if transition == "acquire":
                await manager.assert_current(
                    LeaseIdentity(
                        namespace_id=NAMESPACE_A,
                        client_instance_id=CLIENT_A,
                        lease_id=LEASE_B,
                    )
                )
            else:
                with pytest.raises(LeaseNotFound):
                    await manager.assert_current(old_identity)
            await handle.finish()
        finally:
            gate.allow.set()
            await store.close()

    run(scenario())


def test_acquire_cancellation_after_repository_commit_still_cancels_old_runs(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3")
        manager = NamespaceLeaseManager(
            store,
            lease_id_factory=uuid_sequence(LEASE_A, LEASE_B),
        )
        old_identity = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        original_acquire = store.acquire_namespace
        committed = asyncio.Event()
        allow_return = asyncio.Event()

        async def blocking_acquire(
            namespace_id: str,
            client_instance_id: str,
            new_lease_id: str,
        ):
            transition = await original_acquire(
                namespace_id,
                client_instance_id,
                new_lease_id,
            )
            if new_lease_id == LEASE_B:
                committed.set()
                await allow_return.wait()
            return transition

        store.acquire_namespace = blocking_acquire
        try:
            await manager.acquire(NAMESPACE_A, CLIENT_A)
            handle = await manager.reserve_run(
                old_identity,
                companion_id=COMPANION_A,
                run_id=RUN_A,
                kind=RunKind.PLANNER,
            )
            reacquire = asyncio.create_task(manager.acquire(NAMESPACE_A, CLIENT_A))
            await committed.wait()
            reacquire.cancel()
            reacquire.cancel()
            await asyncio.sleep(0)
            assert not reacquire.done()
            allow_return.set()
            with pytest.raises(asyncio.CancelledError):
                await reacquire

            assert handle.cancelled
            assert manager.run_gate.active_count == 1
            await manager.assert_current(
                LeaseIdentity(
                    namespace_id=NAMESPACE_A,
                    client_instance_id=CLIENT_A,
                    lease_id=LEASE_B,
                )
            )
            await handle.finish()
        finally:
            allow_return.set()
            await store.close()

    run(scenario())


def test_cancelled_reserve_after_slot_creation_does_not_orphan_a_handle(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        store = await SQLiteMemoryStore.open(tmp_path / "memory.sqlite3")
        gate = BlockingReturnGate()
        manager = NamespaceLeaseManager(
            store,
            run_gate=gate,
            lease_id_factory=uuid_sequence(LEASE_A),
        )
        identity = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        try:
            await manager.acquire(NAMESPACE_A, CLIENT_A)
            pending = asyncio.create_task(
                manager.reserve_run(
                    identity,
                    companion_id=COMPANION_A,
                    run_id=RUN_A,
                    kind=RunKind.DIALOGUE,
                )
            )
            await gate.entered.wait()
            assert gate.active_count == 1
            pending.cancel()
            pending.cancel()
            await asyncio.sleep(0)
            assert not pending.done()
            gate.allow.set()
            with pytest.raises(asyncio.CancelledError):
                await pending
            assert gate.active_count == 0
        finally:
            gate.allow.set()
            await store.close()

    run(scenario())


def test_run_gate_is_shared_immediate_and_cancel_does_not_release_slots() -> None:
    async def scenario() -> None:
        gate = RunGate(global_limit=4, per_companion_limit=1)
        leases = [
            LeaseIdentity(
                namespace_id=NAMESPACE_A,
                client_instance_id=CLIENT_A,
                lease_id=LEASE_A,
            ),
            LeaseIdentity(
                namespace_id=NAMESPACE_B,
                client_instance_id=CLIENT_A,
                lease_id=LEASE_B,
            ),
            LeaseIdentity(
                namespace_id=NAMESPACE_C,
                client_instance_id=CLIENT_A,
                lease_id=LEASE_C,
            ),
            LeaseIdentity(
                namespace_id=NAMESPACE_D,
                client_instance_id=CLIENT_A,
                lease_id=LEASE_D,
            ),
        ]
        handles = []
        for index, (lease, run_id) in enumerate(
            zip(leases, [RUN_A, RUN_B, RUN_C, RUN_D], strict=True)
        ):
            handles.append(
                await gate.try_acquire(
                    lease,
                    companion_id=COMPANION_A,
                    run_id=run_id,
                    kind=RunKind.PLANNER if index % 2 == 0 else RunKind.DIALOGUE,
                )
            )
        assert gate.active_count == 4

        with pytest.raises(RunOverloaded):
            await gate.try_acquire(
                LeaseIdentity(
                    namespace_id=NAMESPACE_E,
                    client_instance_id=CLIENT_A,
                    lease_id=LEASE_E,
                ),
                companion_id=COMPANION_A,
                run_id=RUN_E,
                kind=RunKind.PLANNER,
            )
        with pytest.raises(RunOverloaded):
            await gate.try_acquire(
                leases[0],
                companion_id=COMPANION_A,
                run_id=RUN_E,
                kind=RunKind.DIALOGUE,
            )

        assert await gate.cancel_run(leases[0], RUN_A)
        assert handles[0].cancelled
        assert gate.active_count == 4
        with pytest.raises(RunOverloaded):
            await gate.try_acquire(
                LeaseIdentity(
                    namespace_id=NAMESPACE_E,
                    client_instance_id=CLIENT_A,
                    lease_id=LEASE_E,
                ),
                companion_id=COMPANION_A,
                run_id=RUN_E,
                kind=RunKind.DIALOGUE,
            )

        await handles[0].finish()
        successor = await gate.try_acquire(
            LeaseIdentity(
                namespace_id=NAMESPACE_E,
                client_instance_id=CLIENT_A,
                lease_id=LEASE_E,
            ),
            companion_id=COMPANION_A,
            run_id=RUN_E,
            kind=RunKind.DIALOGUE,
        )
        await handles[0].finish()
        assert gate.active_count == 4
        await asyncio.gather(*(handle.finish() for handle in handles[1:]), successor.finish())
        assert gate.active_count == 0

    run(scenario())


def test_run_cancellation_reaches_a_bound_task_and_finish_is_exception_safe() -> None:
    async def scenario() -> None:
        gate = RunGate()
        lease = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        handle = await gate.try_acquire(
            lease,
            companion_id=COMPANION_A,
            run_id=RUN_A,
            kind=RunKind.PLANNER,
        )
        started = asyncio.Event()

        async def worker() -> None:
            started.set()
            await asyncio.Event().wait()

        task = asyncio.create_task(worker())
        await started.wait()
        await handle.bind_task(task)
        assert await gate.cancel_run(lease, RUN_A)
        with pytest.raises(asyncio.CancelledError):
            await task
        assert gate.active_count == 1
        await handle.finish()
        assert gate.active_count == 0

        cancel_before_bind = await gate.try_acquire(
            lease,
            companion_id=COMPANION_B,
            run_id=RUN_B,
            kind=RunKind.DIALOGUE,
        )
        assert await gate.cancel_all() == (RUN_B,)
        late_task = asyncio.create_task(asyncio.sleep(60))
        await cancel_before_bind.bind_task(late_task)
        with pytest.raises(asyncio.CancelledError):
            await late_task
        await cancel_before_bind.finish()

    run(scenario())


def test_repeated_run_cancellation_does_not_interrupt_async_worker_cleanup() -> None:
    async def scenario() -> None:
        gate = RunGate()
        lease = LeaseIdentity(
            namespace_id=NAMESPACE_A,
            client_instance_id=CLIENT_A,
            lease_id=LEASE_A,
        )
        handle = await gate.try_acquire(
            lease,
            companion_id=COMPANION_A,
            run_id=RUN_A,
            kind=RunKind.DIALOGUE,
        )
        cleanup_started = asyncio.Event()
        allow_cleanup = asyncio.Event()
        cleanup_finished = asyncio.Event()
        worker_started = asyncio.Event()

        async def worker() -> None:
            worker_started.set()
            try:
                await asyncio.Event().wait()
            except asyncio.CancelledError:
                cleanup_started.set()
                await allow_cleanup.wait()
                cleanup_finished.set()
                raise

        task = asyncio.create_task(worker())
        await worker_started.wait()
        await handle.bind_task(task)
        assert await gate.cancel_run(lease, RUN_A)
        await cleanup_started.wait()

        assert await gate.cancel_run(lease, RUN_A)
        assert await gate.cancel_lease(lease) == (RUN_A,)
        assert await gate.cancel_all() == (RUN_A,)
        await asyncio.sleep(0)
        assert not task.done()
        assert gate.active_count == 1

        allow_cleanup.set()
        with pytest.raises(asyncio.CancelledError):
            await task
        assert cleanup_finished.is_set()
        assert gate.active_count == 1
        with pytest.raises(RunOverloaded):
            await gate.try_acquire(
                lease,
                companion_id=COMPANION_A,
                run_id=RUN_B,
                kind=RunKind.PLANNER,
            )
        await handle.finish()
        successor = await gate.try_acquire(
            lease,
            companion_id=COMPANION_A,
            run_id=RUN_B,
            kind=RunKind.PLANNER,
        )
        await successor.finish()
        assert gate.active_count == 0

    run(scenario())


def test_concurrent_acquire_is_atomic_across_store_instances(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        clock = ManualClock()
        store_a = await SQLiteMemoryStore.open(path, clock_ms=clock)
        store_b = await SQLiteMemoryStore.open(path, clock_ms=clock)
        manager_a = NamespaceLeaseManager(store_a, lease_id_factory=uuid_sequence(LEASE_A))
        manager_b = NamespaceLeaseManager(store_b, lease_id_factory=uuid_sequence(LEASE_B))
        try:
            results = await asyncio.gather(
                manager_a.acquire(NAMESPACE_A, CLIENT_A),
                manager_b.acquire(NAMESPACE_A, CLIENT_B),
                return_exceptions=True,
            )
            assert sum(not isinstance(result, BaseException) for result in results) == 1
            assert sum(isinstance(result, NamespaceConflict) for result in results) == 1
        finally:
            await store_a.close()
            await store_b.close()

    run(scenario())

from __future__ import annotations

import asyncio
import sqlite3
import threading
from collections.abc import Awaitable, Callable, Iterator
from pathlib import Path

import pytest
from pydantic import ValidationError

from mornlea_companion_agent.domain.common import UINT64_MAX
from mornlea_companion_agent.domain.memory import (
    LeaseIdentity,
    LeaseNotFound,
    MemoryCommit,
    MemoryConflict,
    MemoryDelete,
    MemoryLookup,
    MemoryReconcile,
    MemoryStateNonzero,
    MemoryStateZero,
    MemoryStorageFailure,
    RevisionOverflow,
)
from mornlea_companion_agent.harness.leases import NamespaceLeaseManager
from mornlea_companion_agent.storage.sqlite_memory import SQLiteMemoryStore

NAMESPACE = "11111111-1111-4111-8111-111111111111"
CLIENT = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
COMPANION = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
LEASE_1 = "10000000-0000-4000-8000-000000000001"
LEASE_2 = "10000000-0000-4000-8000-000000000002"
OP_1 = "20000000-0000-4000-8000-000000000001"
OP_2 = "20000000-0000-4000-8000-000000000002"
OP_3 = "20000000-0000-4000-8000-000000000003"
OP_4 = "20000000-0000-4000-8000-000000000004"


class ManualClock:
    def __init__(self, now_ms: int = 1_000_000) -> None:
        self.now_ms = now_ms

    def __call__(self) -> int:
        return self.now_ms


def uuid_sequence(*values: str) -> Callable[[], str]:
    iterator: Iterator[str] = iter(values)
    return lambda: next(iterator)


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def active(
    *, epoch: int, revision: int = 0, operation_id: str | None = None, summary: str = ""
) -> MemoryReconcile:
    memory = (
        MemoryStateZero(revision=0, operation_id=None, summary="")
        if revision == 0
        else MemoryStateNonzero(
            revision=revision,
            operation_id=operation_id,
            summary=summary,
        )
    )
    return MemoryReconcile(
        namespace_id=NAMESPACE,
        companion_id=COMPANION,
        memory_epoch=epoch,
        active=True,
        tombstone_operation_id=None,
        mirror=memory,
    )


def inactive(*, epoch: int, tombstone: str) -> MemoryReconcile:
    return MemoryReconcile(
        namespace_id=NAMESPACE,
        companion_id=COMPANION,
        memory_epoch=epoch,
        active=False,
        tombstone_operation_id=tombstone,
        mirror=None,
    )


async def opened_store(
    path: Path,
    *,
    clock: ManualClock | None = None,
    lease_ids: tuple[str, ...] = (LEASE_1,),
) -> tuple[SQLiteMemoryStore, NamespaceLeaseManager, LeaseIdentity]:
    store = await SQLiteMemoryStore.open(path, clock_ms=clock or ManualClock())
    manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(*lease_ids))
    grant = await manager.acquire(NAMESPACE, CLIENT)
    return (
        store,
        manager,
        LeaseIdentity(
            namespace_id=NAMESPACE,
            client_instance_id=CLIENT,
            lease_id=grant.lease_id,
        ),
    )


def test_memory_domain_is_strict_bounded_and_canonical() -> None:
    MemoryStateZero(revision=0, operation_id=None, summary="")
    MemoryStateNonzero(revision=1, operation_id=OP_1, summary="")
    MemoryStateNonzero(revision=UINT64_MAX, operation_id=OP_1, summary="界" * 682)

    invalid_values = [
        lambda: MemoryStateZero(revision=False, operation_id=None, summary=""),
        lambda: MemoryStateZero(revision=0, operation_id=OP_1, summary=""),
        lambda: MemoryStateNonzero(revision=0, operation_id=OP_1, summary=""),
        lambda: MemoryStateNonzero(revision=1, operation_id=None, summary=""),
        lambda: MemoryStateNonzero(revision=1, operation_id=OP_1, summary="x\x00"),
        lambda: MemoryStateNonzero(revision=1, operation_id=OP_1, summary="界" * 683),
        lambda: active(epoch=0),
        lambda: active(epoch=UINT64_MAX + 1),
        lambda: MemoryReconcile(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            active=False,
            tombstone_operation_id=OP_1,
            mirror=MemoryStateZero(revision=0, operation_id=None, summary=""),
        ),
        lambda: MemoryReconcile(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            active=True,
            tombstone_operation_id=OP_1,
            mirror=MemoryStateZero(revision=0, operation_id=None, summary=""),
        ),
    ]
    for build in invalid_values:
        with pytest.raises(ValidationError):
            build()


def test_reconcile_missing_and_revision_order_use_python_as_runtime_authority(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        try:
            restored = await store.reconcile(
                lease,
                active(epoch=7, revision=7, operation_id=OP_1, summary="mirror seven"),
            )
            assert restored.memory_epoch == 7
            assert restored.active
            assert restored.memory is not None
            assert restored.memory.revision == 7

            python_wins = await store.reconcile(
                lease,
                active(epoch=7, revision=3, operation_id=OP_2, summary="older mirror"),
            )
            assert python_wins.memory == restored.memory

            go_wins = await store.reconcile(
                lease,
                active(epoch=7, revision=8, operation_id=OP_2, summary="mirror eight"),
            )
            assert go_wins.memory is not None
            assert go_wins.memory.revision == 8
            assert go_wins.memory.summary == "mirror eight"

            same = await store.reconcile(
                lease,
                active(epoch=7, revision=8, operation_id=OP_2, summary="mirror eight"),
            )
            assert same == go_wins
            exact_older_receipt = await store.reconcile(
                lease,
                active(epoch=7, revision=7, operation_id=OP_1, summary="mirror seven"),
            )
            assert exact_older_receipt == go_wins
            with pytest.raises(MemoryConflict):
                await store.reconcile(
                    lease,
                    active(epoch=7, revision=3, operation_id=OP_2, summary="older mirror"),
                )
            loaded = await store.load(
                lease,
                MemoryLookup(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    memory_epoch=7,
                ),
            )
            assert loaded == go_wins.memory
            with pytest.raises(MemoryConflict):
                await store.load(
                    lease,
                    MemoryLookup(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=8,
                    ),
                )
            with pytest.raises(MemoryConflict):
                await store.reconcile(
                    lease,
                    active(epoch=7, revision=8, operation_id=OP_3, summary="mirror eight"),
                )
            with pytest.raises(MemoryConflict):
                await store.reconcile(
                    lease,
                    active(epoch=7, revision=8, operation_id=OP_2, summary="fork"),
                )
        finally:
            await store.close()

    run(scenario())


def test_reconcile_orders_epoch_then_state_then_revision(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        try:
            await store.reconcile(
                lease,
                active(
                    epoch=2,
                    revision=UINT64_MAX,
                    operation_id=OP_1,
                    summary="old epoch high revision",
                ),
            )

            replacement = await store.reconcile(lease, inactive(epoch=3, tombstone=OP_2))
            assert replacement.memory_epoch == 3
            assert not replacement.active
            assert replacement.memory is None
            assert replacement.tombstone_operation_id == OP_2

            same = await store.reconcile(lease, inactive(epoch=3, tombstone=OP_2))
            assert same == replacement
            with pytest.raises(MemoryConflict):
                await store.reconcile(lease, inactive(epoch=3, tombstone=OP_3))
            with pytest.raises(MemoryConflict):
                await store.reconcile(lease, active(epoch=3))
            with pytest.raises(MemoryConflict):
                await store.reconcile(lease, inactive(epoch=2, tombstone=OP_1))

            reactivated = await store.reconcile(lease, active(epoch=4))
            assert reactivated.active
            assert reactivated.memory == MemoryStateZero(
                revision=0,
                operation_id=None,
                summary="",
            )
        finally:
            await store.close()

    run(scenario())


def test_commit_cas_is_idempotent_exact_and_fenced_by_current_lease(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, manager, lease = await opened_store(
            tmp_path / "memory.sqlite3",
            lease_ids=(LEASE_1, LEASE_2),
        )
        command = MemoryCommit(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            base_revision=0,
            operation_id=OP_1,
            summary="committed summary",
        )
        try:
            await store.reconcile(lease, active(epoch=1))
            committed = await store.commit(lease, command)
            assert committed.committed_revision == 1
            assert (await store.commit(lease, command)) == committed

            for changed in (
                command.model_copy(update={"base_revision": 1}),
                command.model_copy(update={"summary": "different"}),
                command.model_copy(update={"memory_epoch": 2}),
            ):
                with pytest.raises(MemoryConflict):
                    await store.commit(lease, changed)

            new_grant = await manager.acquire(NAMESPACE, CLIENT)
            assert new_grant.lease_id == LEASE_2
            with pytest.raises(LeaseNotFound):
                await store.commit(lease, command)

            new_lease = LeaseIdentity(
                namespace_id=NAMESPACE,
                client_instance_id=CLIENT,
                lease_id=LEASE_2,
            )
            with pytest.raises(MemoryConflict):
                await store.commit(new_lease, command)
            current = await store.reconcile(
                new_lease,
                active(epoch=1, revision=1, operation_id=OP_1, summary="committed summary"),
            )
            assert current.memory is not None
            assert current.memory.revision == 1
        finally:
            await store.close()

    run(scenario())


def test_commit_revision_mismatch_and_overflow_preserve_memory(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        try:
            await store.reconcile(
                lease,
                active(
                    epoch=9,
                    revision=UINT64_MAX,
                    operation_id=OP_1,
                    summary="at maximum",
                ),
            )
            with pytest.raises(RevisionOverflow):
                await store.commit(
                    lease,
                    MemoryCommit(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=9,
                        base_revision=UINT64_MAX,
                        operation_id=OP_2,
                        summary="must not replace",
                    ),
                )
            unchanged = await store.reconcile(
                lease,
                active(
                    epoch=9,
                    revision=UINT64_MAX,
                    operation_id=OP_1,
                    summary="at maximum",
                ),
            )
            assert unchanged.memory is not None
            assert unchanged.memory.summary == "at maximum"

            with pytest.raises(MemoryConflict):
                await store.commit(
                    lease,
                    MemoryCommit(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=9,
                        base_revision=UINT64_MAX - 1,
                        operation_id=OP_3,
                        summary="wrong base",
                    ),
                )
        finally:
            await store.close()

    run(scenario())


def test_immutable_receipts_replay_any_exact_commit_and_fence_operation_reuse(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        first = MemoryCommit(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            base_revision=0,
            operation_id=OP_1,
            summary="first",
        )
        second = MemoryCommit(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            base_revision=1,
            operation_id=OP_2,
            summary="second",
        )
        try:
            await store.reconcile(lease, active(epoch=1))
            assert (await store.commit(lease, first)).committed_revision == 1
            assert (await store.commit(lease, second)).committed_revision == 2
            assert (await store.commit(lease, second)).committed_revision == 2
            assert (await store.commit(lease, first)).committed_revision == 1

            with pytest.raises(MemoryConflict):
                await store.commit(
                    lease,
                    first.model_copy(update={"base_revision": 2}),
                )
            with pytest.raises(MemoryConflict):
                await store.commit(
                    lease,
                    first.model_copy(update={"summary": "changed first"}),
                )
            current = await store.reconcile(
                lease,
                active(epoch=1, revision=2, operation_id=OP_2, summary="second"),
            )
            assert current.memory is not None
            assert current.memory.revision == 2
            assert current.memory.operation_id == OP_2
        finally:
            await store.close()

    run(scenario())


def test_historical_exact_commit_replay_survives_a_valid_reopen(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        store, _, lease = await opened_store(path)
        first = MemoryCommit(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            base_revision=0,
            operation_id=OP_1,
            summary="first",
        )
        second = MemoryCommit(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            base_revision=1,
            operation_id=OP_2,
            summary="second",
        )
        await store.reconcile(lease, active(epoch=1))
        await store.commit(lease, first)
        await store.commit(lease, second)
        await store.close()

        reopened = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        try:
            replay = await reopened.commit(lease, first)
            assert replay.committed_revision == 1
            current = await reopened.reconcile(
                lease,
                active(epoch=1, revision=2, operation_id=OP_2, summary="second"),
            )
            assert current.memory is not None
            assert current.memory.revision == 2
        finally:
            await reopened.close()

    run(scenario())


def test_tombstone_operation_cannot_be_reused_across_epochs_or_states(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        try:
            await store.reconcile(lease, active(epoch=1))
            first_delete = MemoryDelete(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                old_memory_epoch=1,
                new_memory_epoch=2,
                tombstone_operation_id=OP_3,
            )
            tombstone = await store.delete(lease, first_delete)
            assert (await store.delete(lease, first_delete)) == tombstone

            await store.reconcile(lease, active(epoch=3))
            with pytest.raises(MemoryConflict):
                await store.delete(
                    lease,
                    first_delete.model_copy(update={"old_memory_epoch": 3, "new_memory_epoch": 4}),
                )
            with pytest.raises(MemoryConflict):
                await store.reconcile(lease, inactive(epoch=4, tombstone=OP_3))

            current = await store.reconcile(lease, active(epoch=3))
            assert current.active
            assert current.memory_epoch == 3
        finally:
            await store.close()

    run(scenario())


def test_stale_lease_is_fenced_inside_cross_store_memory_transaction(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        clock = ManualClock()
        store_a = await SQLiteMemoryStore.open(path, clock_ms=clock)
        manager_a = NamespaceLeaseManager(store_a, lease_id_factory=uuid_sequence(LEASE_1))
        first_grant = await manager_a.acquire(NAMESPACE, CLIENT)
        stale = LeaseIdentity(
            namespace_id=NAMESPACE,
            client_instance_id=CLIENT,
            lease_id=first_grant.lease_id,
        )
        await store_a.reconcile(stale, active(epoch=1))

        store_b = await SQLiteMemoryStore.open(path, clock_ms=clock)
        manager_b = NamespaceLeaseManager(store_b, lease_id_factory=uuid_sequence(LEASE_2))
        try:
            clock.now_ms += 15_000
            await manager_b.acquire(NAMESPACE, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
            with pytest.raises(LeaseNotFound):
                await store_a.commit(
                    stale,
                    MemoryCommit(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=1,
                        base_revision=0,
                        operation_id=OP_1,
                        summary="must not commit",
                    ),
                )

            current = LeaseIdentity(
                namespace_id=NAMESPACE,
                client_instance_id="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                lease_id=LEASE_2,
            )
            memory = await store_b.reconcile(current, active(epoch=1))
            assert memory.memory == MemoryStateZero(
                revision=0,
                operation_id=None,
                summary="",
            )
        finally:
            await store_a.close()
            await store_b.close()

    run(scenario())


def test_every_memory_operation_rejects_a_replaced_lease_as_not_found(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, manager, stale = await opened_store(
            tmp_path / "memory.sqlite3",
            lease_ids=(LEASE_1, LEASE_2),
        )
        try:
            await store.reconcile(stale, active(epoch=1))
            await manager.acquire(NAMESPACE, CLIENT)
            operations: tuple[Callable[[], Awaitable[object]], ...] = (
                lambda: store.reconcile(stale, active(epoch=1)),
                lambda: store.load(
                    stale,
                    MemoryLookup(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=1,
                    ),
                ),
                lambda: store.commit(
                    stale,
                    MemoryCommit(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=1,
                        base_revision=0,
                        operation_id=OP_1,
                        summary="stale",
                    ),
                ),
                lambda: store.delete(
                    stale,
                    MemoryDelete(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        old_memory_epoch=1,
                        new_memory_epoch=2,
                        tombstone_operation_id=OP_2,
                    ),
                ),
            )
            for operation in operations:
                with pytest.raises(LeaseNotFound):
                    await operation()
        finally:
            await store.close()

    run(scenario())


def test_concurrent_commits_are_atomic_across_store_instances(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        clock = ManualClock()
        store_a, _, lease = await opened_store(path, clock=clock)
        store_b = await SQLiteMemoryStore.open(path, clock_ms=clock)
        try:
            await store_a.reconcile(lease, active(epoch=1))
            commands = [
                MemoryCommit(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    memory_epoch=1,
                    base_revision=0,
                    operation_id=operation,
                    summary=summary,
                )
                for operation, summary in ((OP_1, "one"), (OP_2, "two"))
            ]
            results = await asyncio.gather(
                store_a.commit(lease, commands[0]),
                store_b.commit(lease, commands[1]),
                return_exceptions=True,
            )
            assert sum(not isinstance(result, BaseException) for result in results) == 1
            assert sum(isinstance(result, MemoryConflict) for result in results) == 1
            winner = await store_a.reconcile(
                lease,
                active(epoch=1),
            )
            assert winner.memory is not None
            assert winner.memory.revision == 1
            assert winner.memory.summary in {"one", "two"}
        finally:
            await store_a.close()
            await store_b.close()

    run(scenario())


def test_delete_is_monotonic_idempotent_and_prevents_old_commit_revival(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        try:
            missing = MemoryDelete(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                old_memory_epoch=5,
                new_memory_epoch=6,
                tombstone_operation_id=OP_2,
            )
            tombstone = await store.delete(lease, missing)
            assert tombstone.memory_epoch == 6
            assert not tombstone.active
            assert (await store.delete(lease, missing)) == tombstone

            with pytest.raises(MemoryConflict):
                await store.delete(
                    lease,
                    missing.model_copy(update={"tombstone_operation_id": OP_3}),
                )
            with pytest.raises(MemoryConflict):
                await store.commit(
                    lease,
                    MemoryCommit(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=5,
                        base_revision=0,
                        operation_id=OP_1,
                        summary="late old summary",
                    ),
                )

            reactivated = await store.reconcile(lease, active(epoch=7))
            assert reactivated.memory == MemoryStateZero(
                revision=0,
                operation_id=None,
                summary="",
            )
            assert reactivated.tombstone_operation_id is None
        finally:
            await store.close()

    run(scenario())


def test_delete_allows_go_to_fence_a_behind_epoch_and_rejects_illegal_advance(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        try:
            await store.reconcile(
                lease,
                active(epoch=2, revision=1, operation_id=OP_1, summary="behind"),
            )
            tombstone = await store.delete(
                lease,
                MemoryDelete(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    old_memory_epoch=5,
                    new_memory_epoch=6,
                    tombstone_operation_id=OP_2,
                ),
            )
            assert tombstone.memory_epoch == 6

            invalid_commands = [
                MemoryDelete(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    old_memory_epoch=6,
                    new_memory_epoch=8,
                    tombstone_operation_id=OP_3,
                ),
                MemoryDelete(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    old_memory_epoch=UINT64_MAX,
                    new_memory_epoch=UINT64_MAX,
                    tombstone_operation_id=OP_4,
                ),
            ]
            for command in invalid_commands:
                with pytest.raises((MemoryConflict, RevisionOverflow)):
                    await store.delete(lease, command)
            still_tombstone = await store.reconcile(lease, inactive(epoch=6, tombstone=OP_2))
            assert still_tombstone == tombstone
        finally:
            await store.close()

    run(scenario())


def test_uint64_values_are_stored_as_exact_big_endian_blobs(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        store, _, lease = await opened_store(path)
        try:
            await store.reconcile(
                lease,
                active(
                    epoch=UINT64_MAX,
                    revision=1 << 63,
                    operation_id=OP_1,
                    summary="unsigned",
                ),
            )
        finally:
            await store.close()

        connection = sqlite3.connect(path)
        try:
            row = connection.execute(
                "SELECT memory_epoch, revision FROM companion_memory"
            ).fetchone()
            assert row == (UINT64_MAX.to_bytes(8, "big"), (1 << 63).to_bytes(8, "big"))
            assert connection.execute(
                "SELECT typeof(memory_epoch), typeof(revision) FROM companion_memory"
            ).fetchone() == ("blob", "blob")
        finally:
            connection.close()

    run(scenario())


@pytest.mark.parametrize("corrupt_epoch", [b"\x01" * 7, b"\x01" * 9])
def test_corrupt_uint64_blob_is_rejected_without_rewriting_state(
    tmp_path: Path,
    corrupt_epoch: bytes,
) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        store, _, lease = await opened_store(path)
        try:
            await store.reconcile(lease, active(epoch=1))
            connection = sqlite3.connect(path)
            try:
                connection.execute("PRAGMA ignore_check_constraints = ON")
                connection.execute(
                    "UPDATE companion_memory SET memory_epoch = ?",
                    (corrupt_epoch,),
                )
                connection.commit()
            finally:
                connection.close()

            with pytest.raises(MemoryStorageFailure):
                await store.reconcile(lease, active(epoch=1))
        finally:
            await store.close()

        connection = sqlite3.connect(path)
        try:
            assert connection.execute("SELECT memory_epoch FROM companion_memory").fetchone() == (
                corrupt_epoch,
            )
        finally:
            connection.close()

    run(scenario())


def test_sqlite_failure_rolls_back_and_close_is_explicit(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        store, _, lease = await opened_store(path)
        try:
            await store.reconcile(lease, active(epoch=1))
            connection = sqlite3.connect(path)
            try:
                connection.execute(
                    """
                    CREATE TRIGGER reject_memory_update
                    BEFORE UPDATE ON companion_memory
                    BEGIN
                        SELECT RAISE(ABORT, 'injected failure');
                    END
                    """
                )
                connection.commit()
            finally:
                connection.close()

            with pytest.raises(MemoryStorageFailure) as captured:
                await store.commit(
                    lease,
                    MemoryCommit(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=1,
                        base_revision=0,
                        operation_id=OP_1,
                        summary="must roll back",
                    ),
                )
            assert "must roll back" not in str(captured.value)
        finally:
            await store.close()

        connection = sqlite3.connect(path)
        try:
            connection.execute("DROP TRIGGER reject_memory_update")
            connection.commit()
            row = connection.execute(
                "SELECT revision, operation_id, summary FROM companion_memory"
            ).fetchone()
            assert row == ((0).to_bytes(8, "big"), None, b"")
        finally:
            connection.close()

        with pytest.raises(MemoryStorageFailure):
            await store.reconcile(lease, active(epoch=1))

    run(scenario())


def test_cancelled_blocked_begin_is_drained_and_connection_remains_usable(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        store, _, lease = await opened_store(path)
        blocker = sqlite3.connect(path)
        try:
            await store.reconcile(lease, active(epoch=1))
            blocker.execute("BEGIN IMMEDIATE")
            command = MemoryCommit(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                base_revision=0,
                operation_id=OP_1,
                summary="cancelled",
            )
            pending = asyncio.create_task(store.commit(lease, command))
            await asyncio.sleep(0.02)
            pending.cancel()
            pending.cancel()
            await asyncio.sleep(0)
            assert not pending.done()

            blocker.rollback()
            with pytest.raises(asyncio.CancelledError):
                await pending

            committed = await store.commit(
                lease,
                command.model_copy(update={"operation_id": OP_2, "summary": "healthy"}),
            )
            assert committed.committed_revision == 1
        finally:
            if blocker.in_transaction:
                blocker.rollback()
            blocker.close()
            await store.close()

    run(scenario())


def test_cancelled_statement_and_double_cancel_drain_rollback_before_reuse(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        path = tmp_path / "memory.sqlite3"
        store, _, lease = await opened_store(path)
        statement_entered = threading.Event()
        allow_statement = threading.Event()
        rollback_entered = asyncio.Event()
        allow_rollback = asyncio.Event()
        original_rollback = store._connection.rollback

        def block_write() -> int:
            statement_entered.set()
            if not allow_statement.wait(timeout=5):
                raise RuntimeError
            return 0

        async def blocking_rollback() -> None:
            rollback_entered.set()
            await allow_rollback.wait()
            await original_rollback()

        try:
            await store.reconcile(lease, active(epoch=1))
            await store._connection.create_function("block_write", 0, block_write)
            await store._execute_statement(
                """
                CREATE TRIGGER block_memory_update
                BEFORE UPDATE ON companion_memory
                BEGIN
                    SELECT block_write();
                END
                """
            )
            store._connection.rollback = blocking_rollback
            pending = asyncio.create_task(
                store.commit(
                    lease,
                    MemoryCommit(
                        namespace_id=NAMESPACE,
                        companion_id=COMPANION,
                        memory_epoch=1,
                        base_revision=0,
                        operation_id=OP_1,
                        summary="must roll back",
                    ),
                )
            )
            assert await asyncio.to_thread(statement_entered.wait, 1)
            pending.cancel()
            await asyncio.sleep(0)
            assert not pending.done()
            allow_statement.set()
            await rollback_entered.wait()
            pending.cancel()
            await asyncio.sleep(0)
            assert not pending.done()
            allow_rollback.set()
            with pytest.raises(asyncio.CancelledError):
                await pending

            assert not store._connection.in_transaction
            await store._execute_statement("DROP TRIGGER block_memory_update")
            committed = await store.commit(
                lease,
                MemoryCommit(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    memory_epoch=1,
                    base_revision=0,
                    operation_id=OP_2,
                    summary="healthy",
                ),
            )
            assert committed.committed_revision == 1
        finally:
            allow_statement.set()
            allow_rollback.set()
            await store.close()

    run(scenario())


def test_cancel_during_commit_reports_unknown_outcome_but_exact_replay_is_safe(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        original_commit = store._connection.commit
        commit_entered = asyncio.Event()
        allow_commit = asyncio.Event()

        async def blocking_commit() -> None:
            commit_entered.set()
            await allow_commit.wait()
            await original_commit()

        command = MemoryCommit(
            namespace_id=NAMESPACE,
            companion_id=COMPANION,
            memory_epoch=1,
            base_revision=0,
            operation_id=OP_1,
            summary="possibly committed",
        )
        try:
            await store.reconcile(lease, active(epoch=1))
            store._connection.commit = blocking_commit
            pending = asyncio.create_task(store.commit(lease, command))
            await commit_entered.wait()
            pending.cancel()
            pending.cancel()
            await asyncio.sleep(0)
            assert not pending.done()
            allow_commit.set()
            with pytest.raises(asyncio.CancelledError):
                await pending

            replay = await store.commit(lease, command)
            assert replay.committed_revision == 1
            next_result = await store.commit(
                lease,
                command.model_copy(
                    update={
                        "base_revision": 1,
                        "operation_id": OP_2,
                        "summary": "next",
                    }
                ),
            )
            assert next_result.committed_revision == 2
        finally:
            allow_commit.set()
            await store.close()

    run(scenario())


def test_close_failure_keeps_store_retryable_until_real_close(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        original_close = store._connection.close
        attempts = 0

        async def fail_once() -> None:
            nonlocal attempts
            attempts += 1
            if attempts == 1:
                raise sqlite3.OperationalError("injected close failure")
            await original_close()

        store._connection.close = fail_once
        with pytest.raises(MemoryStorageFailure):
            await store.close()
        await store.assert_current_lease(lease)
        await store.close()
        with pytest.raises(MemoryStorageFailure):
            await store.assert_current_lease(lease)

    run(scenario())


def test_close_cancellation_drains_actual_close_before_marking_closed(tmp_path: Path) -> None:
    async def scenario() -> None:
        store, _, lease = await opened_store(tmp_path / "memory.sqlite3")
        original_close = store._connection.close
        entered = asyncio.Event()
        allow = asyncio.Event()

        async def blocking_close() -> None:
            entered.set()
            await allow.wait()
            await original_close()

        store._connection.close = blocking_close
        closing = asyncio.create_task(store.close())
        await entered.wait()
        closing.cancel()
        closing.cancel()
        await asyncio.sleep(0)
        assert not closing.done()
        allow.set()
        with pytest.raises(asyncio.CancelledError):
            await closing
        with pytest.raises(MemoryStorageFailure):
            await store.assert_current_lease(lease)
        await store.close()

    run(scenario())

from __future__ import annotations

import asyncio
import sqlite3
from collections.abc import Awaitable, Callable, Iterator
from pathlib import Path

import pytest
from harness.domain.memory import (
    LeaseIdentity,
    MemoryCommit,
    MemoryReconcile,
    MemoryStateNonzero,
    MemoryStateZero,
    MemoryStorageFailure,
    StorageCorruption,
)
from harness.runtime.leases import NamespaceLeaseManager
from harness.store.sqlite_memory import SQLiteMemoryStore

NAMESPACE = "11111111-1111-4111-8111-111111111111"
CLIENT = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
COMPANION = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
LEASE = "10000000-0000-4000-8000-000000000001"
LEASE_2 = "10000000-0000-4000-8000-000000000002"
OPERATION = "20000000-0000-4000-8000-000000000001"

FORBIDDEN_STORAGE_WORDS = {
    "plan",
    "task",
    "fifo",
    "snapshot",
    "prompt",
    "messages",
    "persona",
    "line",
    "proposal",
    "checkpoint",
    "checkpoints",
    "threads",
}


class ManualClock:
    def __call__(self) -> int:
        return 1_000_000


def uuid_sequence(*values: str) -> Callable[[], str]:
    iterator: Iterator[str] = iter(values)
    return lambda: next(iterator)


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def test_sqlite_schema_and_rows_only_hold_lease_and_compact_memory(tmp_path: Path) -> None:
    async def create_database() -> Path:
        path = tmp_path / "memory.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE))
        try:
            await manager.acquire(NAMESPACE, CLIENT)
            identity = LeaseIdentity(
                namespace_id=NAMESPACE,
                client_instance_id=CLIENT,
                lease_id=LEASE,
            )
            await store.reconcile(
                identity,
                MemoryReconcile(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    memory_epoch=1,
                    active=True,
                    tombstone_operation_id=None,
                    mirror=MemoryStateZero(revision=0, operation_id=None, summary=""),
                ),
            )
            await store.commit(
                identity,
                MemoryCommit(
                    namespace_id=NAMESPACE,
                    companion_id=COMPANION,
                    memory_epoch=1,
                    base_revision=0,
                    operation_id=OPERATION,
                    summary="compact memory only",
                ),
            )
        finally:
            await store.close()
        return path

    path = run(create_database())
    assert isinstance(path, Path)
    connection = sqlite3.connect(path)
    try:
        tables = {
            row[0]
            for row in connection.execute(
                "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
            )
        }
        assert tables == {
            "agent_schema",
            "namespace_lease_history",
            "namespace_leases",
            "companion_memory",
            "memory_operations",
        }
        assert connection.execute("PRAGMA user_version").fetchone() == (1,)
        assert connection.execute(
            "SELECT singleton, schema_version FROM agent_schema"
        ).fetchall() == [(1, 1)]

        expected_columns = {
            "agent_schema": {"singleton", "schema_version"},
            "namespace_lease_history": {"lease_id"},
            "namespace_leases": {
                "namespace_id",
                "client_instance_id",
                "lease_id",
                "expires_at_unix_ms",
            },
            "companion_memory": {
                "namespace_id",
                "companion_id",
                "memory_epoch",
                "active",
                "revision",
                "operation_id",
                "summary",
                "tombstone_operation_id",
                "tombstone_old_epoch",
            },
            "memory_operations": {
                "namespace_id",
                "companion_id",
                "operation_id",
                "operation_kind",
                "commit_lease_id",
                "payload_fingerprint",
                "state_fingerprint",
                "result_epoch",
                "result_revision",
            },
        }
        for table, columns in expected_columns.items():
            assert {row[1] for row in connection.execute(f"PRAGMA table_info({table})")} == columns

        schema_text = "\n".join(
            str(row[0])
            for row in connection.execute(
                "SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY name"
            )
        ).lower()
        for forbidden in FORBIDDEN_STORAGE_WORDS:
            assert forbidden not in schema_text

        memory = connection.execute(
            """
            SELECT namespace_id, companion_id, memory_epoch, active, revision,
                   operation_id, summary, tombstone_operation_id
            FROM companion_memory
            """
        ).fetchone()
        assert memory == (
            NAMESPACE,
            COMPANION,
            (1).to_bytes(8, "big"),
            1,
            (1).to_bytes(8, "big"),
            OPERATION,
            b"compact memory only",
            None,
        )
        receipt = connection.execute(
            """
            SELECT operation_id, operation_kind, commit_lease_id,
                   length(payload_fingerprint),
                   length(state_fingerprint), result_epoch, result_revision
            FROM memory_operations
            """
        ).fetchone()
        assert receipt == (
            OPERATION,
            "commit",
            LEASE,
            32,
            32,
            (1).to_bytes(8, "big"),
            (1).to_bytes(8, "big"),
        )
    finally:
        connection.close()

    raw = path.read_bytes().lower()
    for forbidden in FORBIDDEN_STORAGE_WORDS:
        assert forbidden.encode() not in raw


def test_memory_models_have_no_transient_graph_fields() -> None:
    model_fields = set(MemoryReconcile.model_fields) | set(MemoryCommit.model_fields)
    assert model_fields.isdisjoint(FORBIDDEN_STORAGE_WORDS)


def test_project_does_not_add_the_sqlite_langgraph_checkpointer() -> None:
    service_root = Path(__file__).resolve().parents[1]
    manifest = (service_root / "pyproject.toml").read_text(encoding="utf-8").lower()
    lock = (service_root / "uv.lock").read_text(encoding="utf-8").lower()
    assert "langgraph-checkpoint-sqlite" not in manifest
    assert 'name = "langgraph-checkpoint-sqlite"' not in lock


def test_existing_empty_database_is_rejected_without_initializing_it(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "existing.sqlite3"
        path.write_bytes(b"")
        before = path.read_bytes()
        with pytest.raises(MemoryStorageFailure):
            await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        assert path.read_bytes() == before

    run(scenario())


@pytest.mark.parametrize(
    "damage",
    [
        "missing_table",
        "ddl_drift",
        "weak_ddl",
        "extra_object",
        "schema_version",
        "user_version",
    ],
)
def test_existing_schema_damage_fails_closed_without_repair(
    tmp_path: Path,
    damage: str,
) -> None:
    async def scenario() -> None:
        path = tmp_path / f"{damage}.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        await store.close()

        connection = sqlite3.connect(path)
        try:
            if damage == "missing_table":
                connection.execute("DROP TABLE memory_operations")
            elif damage == "ddl_drift":
                connection.execute("ALTER TABLE companion_memory ADD COLUMN drift TEXT")
            elif damage == "weak_ddl":
                connection.execute("DROP TABLE memory_operations")
                connection.execute(
                    """
                    CREATE TABLE memory_operations (
                        namespace_id TEXT,
                        companion_id TEXT,
                        operation_id TEXT,
                        operation_kind TEXT,
                        commit_lease_id TEXT,
                        payload_fingerprint BLOB,
                        state_fingerprint BLOB,
                        result_epoch BLOB,
                        result_revision BLOB
                    )
                    """
                )
            elif damage == "extra_object":
                connection.execute(
                    """
                    CREATE TRIGGER unexpected_trigger
                    AFTER INSERT ON companion_memory
                    BEGIN
                        SELECT 1;
                    END
                    """
                )
            elif damage == "schema_version":
                connection.execute("PRAGMA ignore_check_constraints = ON")
                connection.execute("UPDATE agent_schema SET schema_version = 2")
            else:
                connection.execute("PRAGMA user_version = 2")
            connection.commit()
        finally:
            connection.close()

        before = path.read_bytes()
        with pytest.raises(MemoryStorageFailure):
            await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        assert path.read_bytes() == before

    run(scenario())


def test_existing_unversioned_partial_database_is_not_bootstrapped(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "partial.sqlite3"
        connection = sqlite3.connect(path)
        try:
            connection.execute("CREATE TABLE namespace_leases (namespace_id TEXT)")
            connection.commit()
        finally:
            connection.close()

        before = path.read_bytes()
        with pytest.raises(MemoryStorageFailure):
            await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        assert path.read_bytes() == before

    run(scenario())


def test_existing_foreign_key_violation_fails_readiness_without_repair(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "foreign-key.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE))
        await manager.acquire(NAMESPACE, CLIENT)
        await store.close()

        connection = sqlite3.connect(path)
        try:
            connection.execute("PRAGMA foreign_keys = OFF")
            connection.execute("DELETE FROM namespace_lease_history WHERE lease_id = ?", (LEASE,))
            connection.commit()
        finally:
            connection.close()

        before = path.read_bytes()
        with pytest.raises(MemoryStorageFailure):
            await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        assert path.read_bytes() == before

    run(scenario())


def test_existing_commit_receipt_requires_its_historical_lease(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "missing-commit-lease-history.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE, LEASE_2))
        first = await manager.acquire(NAMESPACE, CLIENT)
        identity = LeaseIdentity(
            namespace_id=NAMESPACE,
            client_instance_id=CLIENT,
            lease_id=first.lease_id,
        )
        await store.reconcile(
            identity,
            MemoryReconcile(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                active=True,
                tombstone_operation_id=None,
                mirror=MemoryStateZero(revision=0, operation_id=None, summary=""),
            ),
        )
        await store.commit(
            identity,
            MemoryCommit(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                base_revision=0,
                operation_id=OPERATION,
                summary="current",
            ),
        )
        await manager.acquire(NAMESPACE, CLIENT)
        await store.close()

        connection = sqlite3.connect(path)
        try:
            connection.execute("PRAGMA foreign_keys = OFF")
            connection.execute(
                "DELETE FROM namespace_lease_history WHERE lease_id = ?",
                (LEASE,),
            )
            connection.commit()
        finally:
            connection.close()

        before = path.read_bytes()
        unexpected: SQLiteMemoryStore | None = None
        try:
            with pytest.raises(StorageCorruption):
                unexpected = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        finally:
            if unexpected is not None:
                await unexpected.close()
        assert path.read_bytes() == before

    run(scenario())


@pytest.mark.parametrize(
    "damage",
    [
        "missing",
        "kind",
        "epoch",
        "revision",
        "payload_fingerprint",
        "state_fingerprint",
        "commit_lease",
        "invalid_commit_lease",
    ],
)
def test_existing_current_active_receipt_damage_fails_closed(
    tmp_path: Path,
    damage: str,
) -> None:
    async def scenario() -> None:
        path = tmp_path / f"active-receipt-{damage}.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE))
        grant = await manager.acquire(NAMESPACE, CLIENT)
        identity = LeaseIdentity(
            namespace_id=NAMESPACE,
            client_instance_id=CLIENT,
            lease_id=grant.lease_id,
        )
        await store.reconcile(
            identity,
            MemoryReconcile(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                active=True,
                tombstone_operation_id=None,
                mirror=MemoryStateZero(revision=0, operation_id=None, summary=""),
            ),
        )
        await store.commit(
            identity,
            MemoryCommit(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                base_revision=0,
                operation_id=OPERATION,
                summary="current",
            ),
        )
        await store.close()

        connection = sqlite3.connect(path)
        try:
            if damage == "missing":
                connection.execute("DELETE FROM memory_operations")
            elif damage == "kind":
                connection.execute(
                    """
                    UPDATE memory_operations
                    SET operation_kind = 'active_mirror', commit_lease_id = NULL
                    """
                )
            elif damage == "epoch":
                connection.execute(
                    "UPDATE memory_operations SET result_epoch = ?",
                    ((2).to_bytes(8, "big"),),
                )
            elif damage == "revision":
                connection.execute(
                    "UPDATE memory_operations SET result_revision = ?",
                    ((2).to_bytes(8, "big"),),
                )
            elif damage == "payload_fingerprint":
                connection.execute(
                    "UPDATE memory_operations SET payload_fingerprint = ?",
                    (b"p" * 32,),
                )
            elif damage == "state_fingerprint":
                connection.execute(
                    "UPDATE memory_operations SET state_fingerprint = ?",
                    (b"x" * 32,),
                )
            elif damage == "commit_lease":
                connection.execute(
                    "INSERT INTO namespace_lease_history (lease_id) VALUES (?)",
                    (LEASE_2,),
                )
                connection.execute(
                    "UPDATE memory_operations SET commit_lease_id = ?",
                    (LEASE_2,),
                )
            else:
                invalid_lease = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
                connection.execute(
                    "INSERT INTO namespace_lease_history (lease_id) VALUES (?)",
                    (invalid_lease,),
                )
                connection.execute(
                    "UPDATE memory_operations SET commit_lease_id = ?",
                    (invalid_lease,),
                )
            connection.commit()
        finally:
            connection.close()

        before = path.read_bytes()
        unexpected: SQLiteMemoryStore | None = None
        try:
            with pytest.raises(StorageCorruption):
                unexpected = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        finally:
            if unexpected is not None:
                await unexpected.close()
        assert path.read_bytes() == before

    run(scenario())


def test_existing_current_active_mirror_requires_matching_receipt_payload(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        path = tmp_path / "active-mirror-receipt.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE))
        grant = await manager.acquire(NAMESPACE, CLIENT)
        await store.reconcile(
            LeaseIdentity(
                namespace_id=NAMESPACE,
                client_instance_id=CLIENT,
                lease_id=grant.lease_id,
            ),
            MemoryReconcile(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                active=True,
                tombstone_operation_id=None,
                mirror=MemoryStateNonzero(
                    revision=1,
                    operation_id=OPERATION,
                    summary="mirror",
                ),
            ),
        )
        await store.close()

        connection = sqlite3.connect(path)
        try:
            connection.execute(
                "UPDATE memory_operations SET payload_fingerprint = ?",
                (b"x" * 32,),
            )
            connection.commit()
        finally:
            connection.close()

        before = path.read_bytes()
        unexpected: SQLiteMemoryStore | None = None
        try:
            with pytest.raises(StorageCorruption):
                unexpected = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        finally:
            if unexpected is not None:
                await unexpected.close()
        assert path.read_bytes() == before

    run(scenario())


def test_existing_current_tombstone_requires_its_receipt(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "tombstone-receipt.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE))
        grant = await manager.acquire(NAMESPACE, CLIENT)
        await store.reconcile(
            LeaseIdentity(
                namespace_id=NAMESPACE,
                client_instance_id=CLIENT,
                lease_id=grant.lease_id,
            ),
            MemoryReconcile(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                active=False,
                tombstone_operation_id=OPERATION,
                mirror=None,
            ),
        )
        await store.close()

        connection = sqlite3.connect(path)
        try:
            connection.execute("DELETE FROM memory_operations")
            connection.commit()
        finally:
            connection.close()

        before = path.read_bytes()
        unexpected: SQLiteMemoryStore | None = None
        try:
            with pytest.raises(StorageCorruption):
                unexpected = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        finally:
            if unexpected is not None:
                await unexpected.close()
        assert path.read_bytes() == before

    run(scenario())


def test_existing_zero_memory_does_not_require_a_receipt(tmp_path: Path) -> None:
    async def scenario() -> None:
        path = tmp_path / "zero-memory.sqlite3"
        store = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        manager = NamespaceLeaseManager(store, lease_id_factory=uuid_sequence(LEASE))
        grant = await manager.acquire(NAMESPACE, CLIENT)
        await store.reconcile(
            LeaseIdentity(
                namespace_id=NAMESPACE,
                client_instance_id=CLIENT,
                lease_id=grant.lease_id,
            ),
            MemoryReconcile(
                namespace_id=NAMESPACE,
                companion_id=COMPANION,
                memory_epoch=1,
                active=True,
                tombstone_operation_id=None,
                mirror=MemoryStateZero(revision=0, operation_id=None, summary=""),
            ),
        )
        await store.close()

        reopened = await SQLiteMemoryStore.open(path, clock_ms=ManualClock())
        await reopened.close()

    run(scenario())

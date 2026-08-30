from __future__ import annotations

import asyncio
import sqlite3
from collections.abc import Awaitable, Callable, Iterator
from pathlib import Path

from mornlea_companion_agent.domain.memory import (
    LeaseIdentity,
    MemoryCommit,
    MemoryReconcile,
    MemoryStateZero,
)
from mornlea_companion_agent.harness.leases import NamespaceLeaseManager
from mornlea_companion_agent.storage.sqlite_memory import SQLiteMemoryStore

NAMESPACE = "11111111-1111-4111-8111-111111111111"
CLIENT = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
COMPANION = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
LEASE = "10000000-0000-4000-8000-000000000001"
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
        assert tables == {"namespace_lease_history", "namespace_leases", "companion_memory"}

        expected_columns = {
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
                "last_commit_lease_id",
                "last_commit_epoch",
                "last_commit_base_revision",
                "last_commit_operation_id",
                "last_commit_summary",
                "last_commit_revision",
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
                   operation_id, summary, tombstone_operation_id,
                   last_commit_operation_id, last_commit_summary
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
            OPERATION,
            b"compact memory only",
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

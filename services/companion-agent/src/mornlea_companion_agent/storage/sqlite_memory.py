"""只保存 namespace lease 与 compact MemoryState 的 SQLite adapter。"""

from __future__ import annotations

import asyncio
import sqlite3
import time
from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Self

import aiosqlite
from pydantic import ValidationError

from mornlea_companion_agent.domain.common import UINT64_MAX
from mornlea_companion_agent.domain.memory import (
    LEASE_TTL_MS,
    AgentDomainFailure,
    ExpiredLease,
    LeaseGrant,
    LeaseIdentity,
    LeaseIDReuse,
    LeaseNotFound,
    LeaseTransition,
    MemoryCommit,
    MemoryCommitResult,
    MemoryConflict,
    MemoryDelete,
    MemoryLookup,
    MemoryReconcile,
    MemoryRecord,
    MemoryStateNonzero,
    MemoryStateZero,
    MemoryStorageFailure,
    NamespaceConflict,
    RevisionOverflow,
    StorageCorruption,
)

_EXPECTED_COLUMNS = {
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

_SCHEMA = """
CREATE TABLE IF NOT EXISTS namespace_lease_history (
    lease_id TEXT PRIMARY KEY NOT NULL
) STRICT, WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS namespace_leases (
    namespace_id TEXT PRIMARY KEY NOT NULL,
    client_instance_id TEXT NOT NULL,
    lease_id TEXT NOT NULL UNIQUE,
    expires_at_unix_ms INTEGER NOT NULL CHECK (expires_at_unix_ms >= 0),
    FOREIGN KEY (lease_id) REFERENCES namespace_lease_history (lease_id)
) STRICT, WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS companion_memory (
    namespace_id TEXT NOT NULL,
    companion_id TEXT NOT NULL,
    memory_epoch BLOB NOT NULL
        CHECK (typeof(memory_epoch) = 'blob' AND length(memory_epoch) = 8),
    active INTEGER NOT NULL CHECK (active IN (0, 1)),
    revision BLOB
        CHECK (revision IS NULL OR (typeof(revision) = 'blob' AND length(revision) = 8)),
    operation_id TEXT,
    summary BLOB
        CHECK (summary IS NULL OR (typeof(summary) = 'blob' AND length(summary) <= 2048)),
    tombstone_operation_id TEXT,
    tombstone_old_epoch BLOB
        CHECK (
            tombstone_old_epoch IS NULL
            OR (typeof(tombstone_old_epoch) = 'blob' AND length(tombstone_old_epoch) = 8)
        ),
    last_commit_lease_id TEXT,
    last_commit_epoch BLOB
        CHECK (
            last_commit_epoch IS NULL
            OR (typeof(last_commit_epoch) = 'blob' AND length(last_commit_epoch) = 8)
        ),
    last_commit_base_revision BLOB
        CHECK (
            last_commit_base_revision IS NULL
            OR (
                typeof(last_commit_base_revision) = 'blob'
                AND length(last_commit_base_revision) = 8
            )
        ),
    last_commit_operation_id TEXT,
    last_commit_summary BLOB
        CHECK (
            last_commit_summary IS NULL
            OR (typeof(last_commit_summary) = 'blob' AND length(last_commit_summary) <= 2048)
        ),
    last_commit_revision BLOB
        CHECK (
            last_commit_revision IS NULL
            OR (typeof(last_commit_revision) = 'blob' AND length(last_commit_revision) = 8)
        ),
    PRIMARY KEY (namespace_id, companion_id),
    CHECK (
        (
            active = 1
            AND revision IS NOT NULL
            AND summary IS NOT NULL
            AND tombstone_operation_id IS NULL
            AND tombstone_old_epoch IS NULL
            AND (
                (revision = X'0000000000000000' AND operation_id IS NULL AND length(summary) = 0)
                OR (revision != X'0000000000000000' AND operation_id IS NOT NULL)
            )
        )
        OR (
            active = 0
            AND revision IS NULL
            AND operation_id IS NULL
            AND summary IS NULL
            AND tombstone_operation_id IS NOT NULL
        )
    ),
    CHECK (
        (
            last_commit_lease_id IS NULL
            AND last_commit_epoch IS NULL
            AND last_commit_base_revision IS NULL
            AND last_commit_operation_id IS NULL
            AND last_commit_summary IS NULL
            AND last_commit_revision IS NULL
        )
        OR (
            active = 1
            AND last_commit_lease_id IS NOT NULL
            AND last_commit_epoch IS NOT NULL
            AND last_commit_base_revision IS NOT NULL
            AND last_commit_operation_id IS NOT NULL
            AND last_commit_summary IS NOT NULL
            AND last_commit_revision IS NOT NULL
        )
    )
) STRICT, WITHOUT ROWID;
"""


def _default_clock_ms() -> int:
    return time.time_ns() // 1_000_000


def _encode_uint64(value: int) -> bytes:
    if type(value) is not int or not 0 <= value <= UINT64_MAX:
        raise MemoryConflict
    return value.to_bytes(8, byteorder="big", signed=False)


def _decode_uint64(value: object) -> int:
    if not isinstance(value, bytes) or len(value) != 8:
        raise StorageCorruption
    return int.from_bytes(value, byteorder="big", signed=False)


def _encode_summary(value: str) -> bytes:
    try:
        encoded = value.encode("utf-8", errors="strict")
    except UnicodeEncodeError as error:
        raise MemoryConflict from error
    if len(encoded) > 2048 or b"\x00" in encoded:
        raise MemoryConflict
    return encoded


def _decode_summary(value: object) -> str:
    if not isinstance(value, bytes) or len(value) > 2048 or b"\x00" in value:
        raise StorageCorruption
    try:
        return value.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise StorageCorruption from error


class SQLiteMemoryStore:
    """单连接 adapter；所有 writer 使用一个显式 `BEGIN IMMEDIATE` 临界区。"""

    def __init__(
        self,
        connection: aiosqlite.Connection,
        *,
        clock_ms: Callable[[], int],
    ) -> None:
        self._connection = connection
        self._clock_ms = clock_ms
        self._writer_lock = asyncio.Lock()
        self._closed = False

    @classmethod
    async def open(
        cls,
        path: Path,
        *,
        clock_ms: Callable[[], int] | None = None,
    ) -> Self:
        connection: aiosqlite.Connection | None = None
        try:
            connection = await aiosqlite.connect(
                path,
                isolation_level=None,
                timeout=5.0,
            )
            connection.row_factory = sqlite3.Row
            for statement in (
                "PRAGMA foreign_keys = ON",
                "PRAGMA busy_timeout = 5000",
                "PRAGMA journal_mode = WAL",
                "PRAGMA synchronous = FULL",
            ):
                cursor = await connection.execute(statement)
                await cursor.close()
            cursor = await connection.executescript(_SCHEMA)
            await cursor.close()
            store = cls(connection, clock_ms=clock_ms or _default_clock_ms)
            await store._validate_schema()
            return store
        except BaseException as error:
            if connection is not None:
                await connection.close()
            if isinstance(error, asyncio.CancelledError):
                raise
            if isinstance(error, AgentDomainFailure):
                raise
            raise MemoryStorageFailure from None

    async def close(self) -> None:
        async with self._writer_lock:
            if self._closed:
                return
            try:
                if self._connection.in_transaction:
                    await self._connection.rollback()
                await self._connection.close()
            except asyncio.CancelledError:
                raise
            except BaseException:
                raise MemoryStorageFailure from None
            finally:
                self._closed = True

    async def __aenter__(self) -> Self:
        self._ensure_open()
        return self

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc_value: BaseException | None,
        traceback: object | None,
    ) -> None:
        await self.close()

    async def acquire_namespace(
        self,
        namespace_id: str,
        client_instance_id: str,
        new_lease_id: str,
    ) -> LeaseTransition:
        proposed = LeaseIdentity(
            namespace_id=namespace_id,
            client_instance_id=client_instance_id,
            lease_id=new_lease_id,
        )
        async with self._write_transaction():
            now = self._now_ms()
            expires = self._expiry(now)
            row = await self._fetchone(
                """
                SELECT namespace_id, client_instance_id, lease_id, expires_at_unix_ms
                FROM namespace_leases WHERE namespace_id = ?
                """,
                (proposed.namespace_id,),
            )
            if (
                row is not None
                and self._decode_expiry(row["expires_at_unix_ms"]) > now
                and row["client_instance_id"] != proposed.client_instance_id
            ):
                raise NamespaceConflict
            previous_id = None if row is None else self._decode_text(row["lease_id"])
            try:
                await self._execute_statement(
                    "INSERT INTO namespace_lease_history (lease_id) VALUES (?)",
                    (proposed.lease_id,),
                )
            except aiosqlite.IntegrityError as error:
                raise LeaseIDReuse from error
            await self._execute_statement(
                """
                INSERT INTO namespace_leases (
                    namespace_id, client_instance_id, lease_id, expires_at_unix_ms
                ) VALUES (?, ?, ?, ?)
                ON CONFLICT(namespace_id) DO UPDATE SET
                    client_instance_id = excluded.client_instance_id,
                    lease_id = excluded.lease_id,
                    expires_at_unix_ms = excluded.expires_at_unix_ms
                """,
                (
                    proposed.namespace_id,
                    proposed.client_instance_id,
                    proposed.lease_id,
                    expires,
                ),
            )
            return LeaseTransition(
                grant=LeaseGrant(
                    **proposed.model_dump(),
                    lease_expires_in_ms=LEASE_TTL_MS,
                ),
                replaced_lease_id=previous_id,
            )

    async def heartbeat_namespace(self, identity: LeaseIdentity) -> LeaseGrant:
        async with self._write_transaction():
            now = self._now_ms()
            expires = self._expiry(now)
            await self._assert_current_in_transaction(identity, now)
            await self._execute_statement(
                """
                UPDATE namespace_leases SET expires_at_unix_ms = ?
                WHERE namespace_id = ? AND client_instance_id = ? AND lease_id = ?
                """,
                (
                    expires,
                    identity.namespace_id,
                    identity.client_instance_id,
                    identity.lease_id,
                ),
            )
        return LeaseGrant(**identity.model_dump(), lease_expires_in_ms=LEASE_TTL_MS)

    async def release_namespace(self, identity: LeaseIdentity) -> None:
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            await self._execute_statement(
                "DELETE FROM namespace_leases WHERE namespace_id = ?",
                (identity.namespace_id,),
            )

    async def assert_current_lease(self, identity: LeaseIdentity) -> None:
        async with self._writer_lock:
            self._ensure_open()
            try:
                now = self._now_ms()
                await self._assert_current_in_transaction(identity, now)
            except AgentDomainFailure:
                raise
            except asyncio.CancelledError:
                raise
            except BaseException:
                raise MemoryStorageFailure from None

    async def expire_namespaces(self) -> tuple[ExpiredLease, ...]:
        async with self._write_transaction():
            now = self._now_ms()
            cursor = await self._connection.execute(
                """
                SELECT namespace_id, client_instance_id, lease_id
                FROM namespace_leases WHERE expires_at_unix_ms <= ?
                ORDER BY namespace_id
                """,
                (now,),
            )
            rows = await cursor.fetchall()
            await cursor.close()
            expired = tuple(
                ExpiredLease(
                    namespace_id=self._decode_text(row["namespace_id"]),
                    client_instance_id=self._decode_text(row["client_instance_id"]),
                    lease_id=self._decode_text(row["lease_id"]),
                )
                for row in rows
            )
            if expired:
                await self._execute_statement(
                    "DELETE FROM namespace_leases WHERE expires_at_unix_ms <= ?",
                    (now,),
                )
            return expired

    async def reconcile(
        self,
        identity: LeaseIdentity,
        desired: MemoryReconcile,
    ) -> MemoryRecord:
        self._require_namespace(identity, desired.namespace_id)
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            row = await self._memory_row(desired.namespace_id, desired.companion_id)
            if row is None:
                record = self._record_from_reconcile(desired)
                await self._replace_memory(record, tombstone_old_epoch=None)
                return record

            current = self._decode_record(row)
            if desired.memory_epoch > current.memory_epoch:
                record = self._record_from_reconcile(desired)
                await self._replace_memory(record, tombstone_old_epoch=None)
                return record
            if desired.memory_epoch < current.memory_epoch:
                raise MemoryConflict
            if desired.active != current.active:
                raise MemoryConflict
            if not desired.active:
                if desired.tombstone_operation_id != current.tombstone_operation_id:
                    raise MemoryConflict
                return current

            if desired.mirror is None or current.memory is None:
                raise StorageCorruption
            if desired.mirror.revision > current.memory.revision:
                record = self._record_from_reconcile(desired)
                await self._replace_memory(record, tombstone_old_epoch=None)
                return record
            if desired.mirror.revision < current.memory.revision:
                return current
            if desired.mirror.operation_id != current.memory.operation_id or _encode_summary(
                desired.mirror.summary
            ) != _encode_summary(current.memory.summary):
                raise MemoryConflict
            return current

    async def load(
        self,
        identity: LeaseIdentity,
        lookup: MemoryLookup,
    ) -> MemoryStateZero | MemoryStateNonzero:
        """读取 Python 运行期权威摘要，不接受 Go mirror 作为普通提示来源。"""

        self._require_namespace(identity, lookup.namespace_id)
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            row = await self._memory_row(lookup.namespace_id, lookup.companion_id)
            if row is None:
                raise MemoryConflict
            current = self._decode_record(row)
            if (
                not current.active
                or current.memory_epoch != lookup.memory_epoch
                or current.memory is None
            ):
                raise MemoryConflict
            return current.memory

    async def commit(
        self,
        identity: LeaseIdentity,
        command: MemoryCommit,
    ) -> MemoryCommitResult:
        self._require_namespace(identity, command.namespace_id)
        summary = _encode_summary(command.summary)
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            row = await self._memory_row(command.namespace_id, command.companion_id)
            if row is None:
                raise MemoryConflict
            current = self._decode_record(row)
            if not current.active or current.memory_epoch != command.memory_epoch:
                raise MemoryConflict
            if current.memory is None:
                raise StorageCorruption

            last_operation = row["last_commit_operation_id"]
            if last_operation == command.operation_id:
                if not self._receipt_matches(row, identity, command, summary):
                    raise MemoryConflict
                return MemoryCommitResult(
                    namespace_id=command.namespace_id,
                    companion_id=command.companion_id,
                    memory_epoch=command.memory_epoch,
                    operation_id=command.operation_id,
                    committed_revision=_decode_uint64(row["last_commit_revision"]),
                )
            if current.memory.operation_id == command.operation_id:
                raise MemoryConflict
            if command.base_revision != current.memory.revision:
                raise MemoryConflict
            if current.memory.revision == UINT64_MAX:
                raise RevisionOverflow

            committed_revision = current.memory.revision + 1
            encoded_epoch = _encode_uint64(command.memory_epoch)
            encoded_base = _encode_uint64(command.base_revision)
            encoded_committed = _encode_uint64(committed_revision)
            await self._execute_statement(
                """
                UPDATE companion_memory SET
                    revision = ?, operation_id = ?, summary = ?,
                    last_commit_lease_id = ?, last_commit_epoch = ?,
                    last_commit_base_revision = ?, last_commit_operation_id = ?,
                    last_commit_summary = ?, last_commit_revision = ?
                WHERE namespace_id = ? AND companion_id = ?
                """,
                (
                    encoded_committed,
                    command.operation_id,
                    summary,
                    identity.lease_id,
                    encoded_epoch,
                    encoded_base,
                    command.operation_id,
                    summary,
                    encoded_committed,
                    command.namespace_id,
                    command.companion_id,
                ),
            )
            return MemoryCommitResult(
                namespace_id=command.namespace_id,
                companion_id=command.companion_id,
                memory_epoch=command.memory_epoch,
                operation_id=command.operation_id,
                committed_revision=committed_revision,
            )

    async def delete(
        self,
        identity: LeaseIdentity,
        command: MemoryDelete,
    ) -> MemoryRecord:
        self._require_namespace(identity, command.namespace_id)
        if command.old_memory_epoch == UINT64_MAX:
            raise RevisionOverflow
        if command.new_memory_epoch != command.old_memory_epoch + 1:
            raise MemoryConflict
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            row = await self._memory_row(command.namespace_id, command.companion_id)
            desired = MemoryRecord(
                namespace_id=command.namespace_id,
                companion_id=command.companion_id,
                memory_epoch=command.new_memory_epoch,
                active=False,
                tombstone_operation_id=command.tombstone_operation_id,
                memory=None,
            )
            if row is None:
                await self._replace_memory(
                    desired,
                    tombstone_old_epoch=command.old_memory_epoch,
                )
                return desired
            current = self._decode_record(row)
            if current.memory_epoch > command.new_memory_epoch:
                raise MemoryConflict
            if current.memory_epoch == command.new_memory_epoch:
                stored_old = row["tombstone_old_epoch"]
                if (
                    current.active
                    or current.tombstone_operation_id != command.tombstone_operation_id
                    or (
                        stored_old is not None
                        and _decode_uint64(stored_old) != command.old_memory_epoch
                    )
                ):
                    raise MemoryConflict
                return current
            await self._replace_memory(
                desired,
                tombstone_old_epoch=command.old_memory_epoch,
            )
            return desired

    async def _validate_schema(self) -> None:
        self._ensure_open()
        cursor = await self._connection.execute(
            """
            SELECT name FROM sqlite_master
            WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
            """
        )
        rows = await cursor.fetchall()
        await cursor.close()
        if {self._decode_text(row["name"]) for row in rows} != set(_EXPECTED_COLUMNS):
            raise StorageCorruption
        for table, expected in _EXPECTED_COLUMNS.items():
            cursor = await self._connection.execute(f"PRAGMA table_info({table})")
            columns = await cursor.fetchall()
            await cursor.close()
            if {self._decode_text(column["name"]) for column in columns} != expected:
                raise StorageCorruption
        cursor = await self._connection.execute("PRAGMA quick_check")
        check = await cursor.fetchone()
        await cursor.close()
        if check is None or check[0] != "ok":
            raise StorageCorruption

    @asynccontextmanager
    async def _write_transaction(self) -> AsyncIterator[None]:
        async with self._writer_lock:
            self._ensure_open()
            begun = False
            try:
                await self._execute_statement("BEGIN IMMEDIATE")
                begun = True
                yield
                await self._connection.commit()
            except BaseException as error:
                if begun and self._connection.in_transaction:
                    try:
                        await asyncio.shield(self._connection.rollback())
                    except BaseException:
                        if isinstance(error, asyncio.CancelledError):
                            raise error from None
                        raise MemoryStorageFailure from None
                if isinstance(error, asyncio.CancelledError):
                    raise
                if isinstance(error, AgentDomainFailure):
                    raise
                raise MemoryStorageFailure from None

    async def _assert_current_in_transaction(
        self,
        identity: LeaseIdentity,
        now: int,
    ) -> None:
        row = await self._fetchone(
            """
            SELECT client_instance_id, lease_id, expires_at_unix_ms
            FROM namespace_leases WHERE namespace_id = ?
            """,
            (identity.namespace_id,),
        )
        if (
            row is None
            or row["client_instance_id"] != identity.client_instance_id
            or row["lease_id"] != identity.lease_id
            or self._decode_expiry(row["expires_at_unix_ms"]) <= now
        ):
            raise LeaseNotFound

    async def _memory_row(self, namespace_id: str, companion_id: str) -> sqlite3.Row | None:
        return await self._fetchone(
            "SELECT * FROM companion_memory WHERE namespace_id = ? AND companion_id = ?",
            (namespace_id, companion_id),
        )

    async def _fetchone(
        self,
        query: str,
        parameters: tuple[object, ...],
    ) -> sqlite3.Row | None:
        cursor = await self._connection.execute(query, parameters)
        row = await cursor.fetchone()
        await cursor.close()
        return row

    async def _execute_statement(
        self,
        query: str,
        parameters: tuple[object, ...] = (),
    ) -> None:
        cursor = await self._connection.execute(query, parameters)
        await cursor.close()

    async def _replace_memory(
        self,
        record: MemoryRecord,
        *,
        tombstone_old_epoch: int | None,
    ) -> None:
        if record.active:
            if record.memory is None:
                raise StorageCorruption
            revision: bytes | None = _encode_uint64(record.memory.revision)
            operation_id = record.memory.operation_id
            summary: bytes | None = _encode_summary(record.memory.summary)
            tombstone = None
            old_epoch = None
        else:
            revision = None
            operation_id = None
            summary = None
            tombstone = record.tombstone_operation_id
            old_epoch = None if tombstone_old_epoch is None else _encode_uint64(tombstone_old_epoch)
        await self._execute_statement(
            """
            INSERT INTO companion_memory (
                namespace_id, companion_id, memory_epoch, active,
                revision, operation_id, summary,
                tombstone_operation_id, tombstone_old_epoch,
                last_commit_lease_id, last_commit_epoch,
                last_commit_base_revision, last_commit_operation_id,
                last_commit_summary, last_commit_revision
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL)
            ON CONFLICT(namespace_id, companion_id) DO UPDATE SET
                memory_epoch = excluded.memory_epoch,
                active = excluded.active,
                revision = excluded.revision,
                operation_id = excluded.operation_id,
                summary = excluded.summary,
                tombstone_operation_id = excluded.tombstone_operation_id,
                tombstone_old_epoch = excluded.tombstone_old_epoch,
                last_commit_lease_id = NULL,
                last_commit_epoch = NULL,
                last_commit_base_revision = NULL,
                last_commit_operation_id = NULL,
                last_commit_summary = NULL,
                last_commit_revision = NULL
            """,
            (
                record.namespace_id,
                record.companion_id,
                _encode_uint64(record.memory_epoch),
                1 if record.active else 0,
                revision,
                operation_id,
                summary,
                tombstone,
                old_epoch,
            ),
        )

    def _decode_record(self, row: sqlite3.Row) -> MemoryRecord:
        try:
            namespace_id = self._decode_text(row["namespace_id"])
            companion_id = self._decode_text(row["companion_id"])
            epoch = _decode_uint64(row["memory_epoch"])
            active_raw = row["active"]
            if type(active_raw) is not int or active_raw not in (0, 1):
                raise StorageCorruption
            if active_raw == 1:
                revision = _decode_uint64(row["revision"])
                summary = _decode_summary(row["summary"])
                operation_id = row["operation_id"]
                memory = (
                    MemoryStateZero(revision=0, operation_id=None, summary="")
                    if revision == 0
                    else MemoryStateNonzero(
                        revision=revision,
                        operation_id=operation_id,
                        summary=summary,
                    )
                )
                return MemoryRecord(
                    namespace_id=namespace_id,
                    companion_id=companion_id,
                    memory_epoch=epoch,
                    active=True,
                    tombstone_operation_id=None,
                    memory=memory,
                )
            return MemoryRecord(
                namespace_id=namespace_id,
                companion_id=companion_id,
                memory_epoch=epoch,
                active=False,
                tombstone_operation_id=row["tombstone_operation_id"],
                memory=None,
            )
        except AgentDomainFailure:
            raise
        except (KeyError, TypeError, ValidationError, ValueError):
            raise StorageCorruption from None

    @staticmethod
    def _record_from_reconcile(desired: MemoryReconcile) -> MemoryRecord:
        return MemoryRecord(
            namespace_id=desired.namespace_id,
            companion_id=desired.companion_id,
            memory_epoch=desired.memory_epoch,
            active=desired.active,
            tombstone_operation_id=desired.tombstone_operation_id,
            memory=desired.mirror,
        )

    @staticmethod
    def _receipt_matches(
        row: sqlite3.Row,
        identity: LeaseIdentity,
        command: MemoryCommit,
        summary: bytes,
    ) -> bool:
        try:
            return (
                row["last_commit_lease_id"] == identity.lease_id
                and _decode_uint64(row["last_commit_epoch"]) == command.memory_epoch
                and _decode_uint64(row["last_commit_base_revision"]) == command.base_revision
                and row["last_commit_operation_id"] == command.operation_id
                and row["last_commit_summary"] == summary
                and _decode_uint64(row["last_commit_revision"]) == command.base_revision + 1
            )
        except AgentDomainFailure:
            raise
        except (KeyError, TypeError):
            raise StorageCorruption from None

    @staticmethod
    def _require_namespace(identity: LeaseIdentity, namespace_id: str) -> None:
        if identity.namespace_id != namespace_id:
            raise MemoryConflict

    def _now_ms(self) -> int:
        try:
            value = self._clock_ms()
        except BaseException:
            raise MemoryStorageFailure from None
        if type(value) is not int or not 0 <= value <= (1 << 63) - 1:
            raise MemoryStorageFailure
        return value

    @staticmethod
    def _expiry(now: int) -> int:
        expires = now + LEASE_TTL_MS
        if expires > (1 << 63) - 1:
            raise MemoryStorageFailure
        return expires

    @staticmethod
    def _decode_expiry(value: object) -> int:
        if type(value) is not int or not 0 <= value <= (1 << 63) - 1:
            raise StorageCorruption
        return value

    @staticmethod
    def _decode_text(value: object) -> str:
        if not isinstance(value, str):
            raise StorageCorruption
        return value

    def _ensure_open(self) -> None:
        if self._closed:
            raise MemoryStorageFailure


__all__ = ["SQLiteMemoryStore"]

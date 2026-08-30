"""只保存 namespace lease 与 compact MemoryState 的 SQLite adapter。"""

from __future__ import annotations

import asyncio
import hashlib
import re
import sqlite3
import time
from collections.abc import AsyncIterator, Awaitable, Callable
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

_SCHEMA_VERSION = 1
_EXPECTED_COLUMNS = {
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
        "payload_fingerprint",
        "state_fingerprint",
        "result_epoch",
        "result_revision",
    },
}

_SCHEMA_DDL = {
    "agent_schema": """
CREATE TABLE agent_schema (
    singleton INTEGER PRIMARY KEY NOT NULL CHECK (singleton = 1),
    schema_version INTEGER NOT NULL CHECK (schema_version = 1)
) STRICT, WITHOUT ROWID
""".strip(),
    "namespace_lease_history": """
CREATE TABLE namespace_lease_history (
    lease_id TEXT PRIMARY KEY NOT NULL
) STRICT, WITHOUT ROWID
""".strip(),
    "namespace_leases": """
CREATE TABLE namespace_leases (
    namespace_id TEXT PRIMARY KEY NOT NULL,
    client_instance_id TEXT NOT NULL,
    lease_id TEXT NOT NULL UNIQUE,
    expires_at_unix_ms INTEGER NOT NULL CHECK (expires_at_unix_ms >= 0),
    FOREIGN KEY (lease_id) REFERENCES namespace_lease_history (lease_id)
) STRICT, WITHOUT ROWID
""".strip(),
    "companion_memory": """
CREATE TABLE companion_memory (
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
    )
) STRICT, WITHOUT ROWID
""".strip(),
    "memory_operations": """
CREATE TABLE memory_operations (
    namespace_id TEXT NOT NULL,
    companion_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    operation_kind TEXT NOT NULL
        CHECK (operation_kind IN ('commit', 'active_mirror', 'tombstone')),
    payload_fingerprint BLOB NOT NULL
        CHECK (typeof(payload_fingerprint) = 'blob' AND length(payload_fingerprint) = 32),
    state_fingerprint BLOB NOT NULL
        CHECK (typeof(state_fingerprint) = 'blob' AND length(state_fingerprint) = 32),
    result_epoch BLOB NOT NULL
        CHECK (typeof(result_epoch) = 'blob' AND length(result_epoch) = 8),
    result_revision BLOB
        CHECK (
            result_revision IS NULL
            OR (typeof(result_revision) = 'blob' AND length(result_revision) = 8)
        ),
    PRIMARY KEY (namespace_id, companion_id, operation_id),
    CHECK (
        (
            operation_kind = 'tombstone'
            AND result_revision IS NULL
        )
        OR (
            operation_kind IN ('commit', 'active_mirror')
            AND result_revision IS NOT NULL
        )
    )
) STRICT, WITHOUT ROWID
""".strip(),
}


async def _drain_owned[T](
    awaitable: Awaitable[T],
) -> tuple[T, asyncio.CancelledError | None]:
    """完成已经交给 SQLite worker 的动作，再把外层取消交还调用方。"""

    owned = asyncio.ensure_future(awaitable)
    cancellation: asyncio.CancelledError | None = None
    while not owned.done():
        try:
            await asyncio.shield(owned)
        except asyncio.CancelledError as error:
            if owned.done() and (task := asyncio.current_task()) is not None:
                if task.cancelling() == 0:
                    break
            if cancellation is None:
                cancellation = error
        except BaseException:
            break
    return owned.result(), cancellation


async def _await_owned[T](awaitable: Awaitable[T]) -> T:
    result, cancellation = await _drain_owned(awaitable)
    if cancellation is not None:
        raise cancellation from None
    return result


def _normalize_sql(value: str) -> str:
    return re.sub(r"\s+", " ", value.strip())


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


def _fingerprint(domain: bytes, *fields: bytes) -> bytes:
    digest = hashlib.sha256()
    for field in (b"mornlea-memory-operation-v1", domain, *fields):
        digest.update(len(field).to_bytes(8, byteorder="big", signed=False))
        digest.update(field)
    return digest.digest()


def _text_bytes(value: str) -> bytes:
    try:
        return value.encode("ascii", errors="strict")
    except UnicodeEncodeError as error:
        raise MemoryConflict from error


def _active_state_fingerprint(
    namespace_id: str,
    companion_id: str,
    memory_epoch: int,
    revision: int,
    operation_id: str,
    summary: bytes,
) -> bytes:
    return _fingerprint(
        b"active-state",
        _text_bytes(namespace_id),
        _text_bytes(companion_id),
        _encode_uint64(memory_epoch),
        _encode_uint64(revision),
        _text_bytes(operation_id),
        summary,
    )


def _tombstone_state_fingerprint(
    namespace_id: str,
    companion_id: str,
    memory_epoch: int,
    operation_id: str,
) -> bytes:
    return _fingerprint(
        b"tombstone-state",
        _text_bytes(namespace_id),
        _text_bytes(companion_id),
        _encode_uint64(memory_epoch),
        _text_bytes(operation_id),
    )


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
        self._broken = False
        self._closed = False

    @classmethod
    async def open(
        cls,
        path: Path,
        *,
        clock_ms: Callable[[], int] | None = None,
    ) -> Self:
        existed = path.exists()
        connection: aiosqlite.Connection | None = None
        try:
            opened_connection, connection_cancellation = await _drain_owned(
                aiosqlite.connect(
                    path,
                    isolation_level=None,
                    timeout=5.0,
                )
            )
            connection = opened_connection
            if connection_cancellation is not None:
                raise connection_cancellation from None
            connection.row_factory = sqlite3.Row
            for statement in (
                "PRAGMA foreign_keys = ON",
                "PRAGMA busy_timeout = 5000",
            ):
                await cls._execute_on_connection(connection, statement)
            store = cls(connection, clock_ms=clock_ms or _default_clock_ms)
            if existed:
                await store._validate_schema()
            else:
                await store._bootstrap_new_schema()
            for statement in (
                "PRAGMA journal_mode = WAL",
                "PRAGMA synchronous = FULL",
                "PRAGMA secure_delete = ON",
            ):
                await cls._execute_on_connection(connection, statement)
            return store
        except BaseException as error:
            if connection is not None:
                try:
                    await _await_owned(connection.close())
                except BaseException:
                    if not isinstance(error, asyncio.CancelledError):
                        raise MemoryStorageFailure from None
            if isinstance(error, asyncio.CancelledError):
                raise
            if isinstance(error, AgentDomainFailure):
                raise
            raise MemoryStorageFailure from None

    async def close(self) -> None:
        async with self._writer_lock:
            if self._closed:
                return
            cancellation: asyncio.CancelledError | None = None
            rollback_failed = False
            if self._connection.in_transaction:
                try:
                    _, cancellation = await _drain_owned(self._connection.rollback())
                except BaseException:
                    rollback_failed = True
            try:
                _, close_cancellation = await _drain_owned(self._connection.close())
                if cancellation is None:
                    cancellation = close_cancellation
            except BaseException:
                raise MemoryStorageFailure from None
            self._closed = True
            if rollback_failed:
                raise MemoryStorageFailure from None
            if cancellation is not None:
                raise cancellation from None

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
            rows = await self._fetchall(
                """
                SELECT namespace_id, client_instance_id, lease_id
                FROM namespace_leases WHERE expires_at_unix_ms <= ?
                ORDER BY namespace_id
                """,
                (now,),
            )
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
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            self._require_namespace(identity, desired.namespace_id)
            row = await self._memory_row(desired.namespace_id, desired.companion_id)
            if row is None:
                record = self._record_from_reconcile(desired)
                await self._claim_reconcile_operation(record, require_existing=False)
                await self._replace_memory(record, tombstone_old_epoch=None)
                return record

            current = self._decode_record(row)
            if desired.memory_epoch > current.memory_epoch:
                record = self._record_from_reconcile(desired)
                await self._claim_reconcile_operation(record, require_existing=False)
                await self._replace_memory(record, tombstone_old_epoch=None)
                return record
            if desired.memory_epoch < current.memory_epoch:
                raise MemoryConflict
            if desired.active != current.active:
                raise MemoryConflict
            if not desired.active:
                if desired.tombstone_operation_id != current.tombstone_operation_id:
                    raise MemoryConflict
                await self._claim_reconcile_operation(current, require_existing=True)
                return current

            if desired.mirror is None or current.memory is None:
                raise StorageCorruption
            if desired.mirror.revision > current.memory.revision:
                record = self._record_from_reconcile(desired)
                await self._claim_reconcile_operation(record, require_existing=False)
                await self._replace_memory(record, tombstone_old_epoch=None)
                return record
            if desired.mirror.revision < current.memory.revision:
                await self._claim_reconcile_operation(current, require_existing=True)
                await self._validate_seen_reconcile_operation(self._record_from_reconcile(desired))
                return current
            if desired.mirror.operation_id != current.memory.operation_id or _encode_summary(
                desired.mirror.summary
            ) != _encode_summary(current.memory.summary):
                raise MemoryConflict
            await self._claim_reconcile_operation(current, require_existing=True)
            return current

    async def load(
        self,
        identity: LeaseIdentity,
        lookup: MemoryLookup,
    ) -> MemoryStateZero | MemoryStateNonzero:
        """读取 Python 运行期权威摘要，不接受 Go mirror 作为普通提示来源。"""

        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            self._require_namespace(identity, lookup.namespace_id)
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
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            self._require_namespace(identity, command.namespace_id)
            summary = _encode_summary(command.summary)
            row = await self._memory_row(command.namespace_id, command.companion_id)
            if row is None:
                raise MemoryConflict
            current = self._decode_record(row)
            if not current.active or current.memory_epoch != command.memory_epoch:
                raise MemoryConflict
            if current.memory is None:
                raise StorageCorruption

            existing_revision = await self._existing_commit_result(identity, command, summary)
            if existing_revision is not None:
                return MemoryCommitResult(
                    namespace_id=command.namespace_id,
                    companion_id=command.companion_id,
                    memory_epoch=command.memory_epoch,
                    operation_id=command.operation_id,
                    committed_revision=existing_revision,
                )
            if current.memory.operation_id == command.operation_id:
                raise StorageCorruption
            if command.base_revision != current.memory.revision:
                raise MemoryConflict
            if current.memory.revision == UINT64_MAX:
                raise RevisionOverflow

            committed_revision = current.memory.revision + 1
            encoded_committed = _encode_uint64(committed_revision)
            await self._execute_statement(
                """
                UPDATE companion_memory SET
                    revision = ?, operation_id = ?, summary = ?
                WHERE namespace_id = ? AND companion_id = ?
                """,
                (
                    encoded_committed,
                    command.operation_id,
                    summary,
                    command.namespace_id,
                    command.companion_id,
                ),
            )
            await self._insert_commit_operation(
                identity,
                command,
                summary,
                committed_revision,
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
        async with self._write_transaction():
            now = self._now_ms()
            await self._assert_current_in_transaction(identity, now)
            self._require_namespace(identity, command.namespace_id)
            if command.old_memory_epoch == UINT64_MAX:
                raise RevisionOverflow
            if command.new_memory_epoch != command.old_memory_epoch + 1:
                raise MemoryConflict
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
                await self._claim_tombstone_operation(desired, require_existing=False)
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
                await self._claim_tombstone_operation(current, require_existing=True)
                return current
            await self._claim_tombstone_operation(desired, require_existing=False)
            await self._replace_memory(
                desired,
                tombstone_old_epoch=command.old_memory_epoch,
            )
            return desired

    async def _bootstrap_new_schema(self) -> None:
        """只在调用前确认路径原本不存在时创建规范 schema。"""

        async with self._write_transaction():
            objects = await self._fetchall(
                """
                SELECT type, name, tbl_name, sql FROM sqlite_master
                WHERE name NOT LIKE 'sqlite_%'
                """
            )
            user_version = await self._pragma_uint("PRAGMA user_version")
            if not objects and user_version == 0:
                for statement in _SCHEMA_DDL.values():
                    await self._execute_statement(statement)
                await self._execute_statement(
                    "INSERT INTO agent_schema (singleton, schema_version) VALUES (1, ?)",
                    (_SCHEMA_VERSION,),
                )
                await self._execute_statement(f"PRAGMA user_version = {_SCHEMA_VERSION}")
            await self._validate_schema()

    async def _validate_schema(self) -> None:
        self._ensure_open()
        if await self._pragma_uint("PRAGMA foreign_keys") != 1:
            raise StorageCorruption
        if await self._pragma_uint("PRAGMA user_version") != _SCHEMA_VERSION:
            raise StorageCorruption

        objects = await self._fetchall(
            """
            SELECT type, name, tbl_name, sql FROM sqlite_master
            WHERE name NOT LIKE 'sqlite_%'
            ORDER BY type, name
            """
        )
        if len(objects) != len(_SCHEMA_DDL):
            raise StorageCorruption
        for row in objects:
            name = self._decode_text(row["name"])
            if (
                row["type"] != "table"
                or row["tbl_name"] != name
                or name not in _SCHEMA_DDL
                or not isinstance(row["sql"], str)
                or _normalize_sql(row["sql"]) != _normalize_sql(_SCHEMA_DDL[name])
            ):
                raise StorageCorruption

        for table, expected in _EXPECTED_COLUMNS.items():
            columns = await self._fetchall(f"PRAGMA table_xinfo({table})")
            if {self._decode_text(column["name"]) for column in columns} != expected or any(
                column["hidden"] != 0 for column in columns
            ):
                raise StorageCorruption

        table_rows = await self._fetchall("PRAGMA table_list")
        table_shapes = {
            self._decode_text(row["name"]): (row["type"], row["wr"], row["strict"])
            for row in table_rows
            if row["name"] in _EXPECTED_COLUMNS
        }
        if table_shapes != {table: ("table", 1, 1) for table in _EXPECTED_COLUMNS}:
            raise StorageCorruption

        version_rows = await self._fetchall("SELECT singleton, schema_version FROM agent_schema")
        if [tuple(row) for row in version_rows] != [(1, _SCHEMA_VERSION)]:
            raise StorageCorruption

        expected_foreign_keys: dict[str, list[tuple[object, ...]]] = {
            "agent_schema": [],
            "namespace_lease_history": [],
            "namespace_leases": [
                (
                    0,
                    0,
                    "namespace_lease_history",
                    "lease_id",
                    "lease_id",
                    "NO ACTION",
                    "NO ACTION",
                    "NONE",
                )
            ],
            "companion_memory": [],
            "memory_operations": [],
        }
        for table, foreign_key_rows_expected in expected_foreign_keys.items():
            rows = await self._fetchall(f"PRAGMA foreign_key_list({table})")
            if [tuple(row) for row in rows] != foreign_key_rows_expected:
                raise StorageCorruption
        if await self._fetchall("PRAGMA foreign_key_check"):
            raise StorageCorruption
        check = await self._fetchall("PRAGMA quick_check")
        if len(check) != 1 or tuple(check[0]) != ("ok",):
            raise StorageCorruption

    @asynccontextmanager
    async def _write_transaction(self) -> AsyncIterator[None]:
        async with self._writer_lock:
            self._ensure_open()
            try:
                await self._execute_statement("BEGIN IMMEDIATE")
                yield
                await _await_owned(self._connection.commit())
            except BaseException as error:
                if self._connection.in_transaction:
                    try:
                        await _drain_owned(self._connection.rollback())
                    except BaseException:
                        self._broken = True
                        raise MemoryStorageFailure from None
                if isinstance(error, asyncio.CancelledError):
                    raise error from None
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
        parameters: tuple[object, ...] = (),
    ) -> sqlite3.Row | None:
        async def operation() -> sqlite3.Row | None:
            cursor = await self._connection.execute(query, parameters)
            try:
                return await cursor.fetchone()
            finally:
                await cursor.close()

        return await _await_owned(operation())

    async def _fetchall(
        self,
        query: str,
        parameters: tuple[object, ...] = (),
    ) -> list[sqlite3.Row]:
        async def operation() -> list[sqlite3.Row]:
            cursor = await self._connection.execute(query, parameters)
            try:
                return list(await cursor.fetchall())
            finally:
                await cursor.close()

        return await _await_owned(operation())

    async def _execute_statement(
        self,
        query: str,
        parameters: tuple[object, ...] = (),
    ) -> None:
        await self._execute_on_connection(self._connection, query, parameters)

    async def _pragma_uint(self, query: str) -> int:
        row = await self._fetchone(query)
        if row is None or len(row) != 1 or type(row[0]) is not int:
            raise StorageCorruption
        return row[0]

    @staticmethod
    async def _execute_on_connection(
        connection: aiosqlite.Connection,
        query: str,
        parameters: tuple[object, ...] = (),
    ) -> None:
        async def operation() -> None:
            cursor = await connection.execute(query, parameters)
            await cursor.close()

        await _await_owned(operation())

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
                tombstone_operation_id, tombstone_old_epoch
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(namespace_id, companion_id) DO UPDATE SET
                memory_epoch = excluded.memory_epoch,
                active = excluded.active,
                revision = excluded.revision,
                operation_id = excluded.operation_id,
                summary = excluded.summary,
                tombstone_operation_id = excluded.tombstone_operation_id,
                tombstone_old_epoch = excluded.tombstone_old_epoch
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

    async def _operation_row(
        self,
        namespace_id: str,
        companion_id: str,
        operation_id: str,
    ) -> sqlite3.Row | None:
        return await self._fetchone(
            """
            SELECT operation_kind, payload_fingerprint, state_fingerprint,
                   result_epoch, result_revision
            FROM memory_operations
            WHERE namespace_id = ? AND companion_id = ? AND operation_id = ?
            """,
            (namespace_id, companion_id, operation_id),
        )

    async def _insert_operation(
        self,
        namespace_id: str,
        companion_id: str,
        operation_id: str,
        *,
        kind: str,
        payload_fingerprint: bytes,
        state_fingerprint: bytes,
        result_epoch: int,
        result_revision: int | None,
    ) -> None:
        await self._execute_statement(
            """
            INSERT INTO memory_operations (
                namespace_id, companion_id, operation_id, operation_kind,
                payload_fingerprint, state_fingerprint, result_epoch, result_revision
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                namespace_id,
                companion_id,
                operation_id,
                kind,
                payload_fingerprint,
                state_fingerprint,
                _encode_uint64(result_epoch),
                None if result_revision is None else _encode_uint64(result_revision),
            ),
        )

    @staticmethod
    def _receipt_values(
        row: sqlite3.Row,
    ) -> tuple[str, bytes, bytes, int, int | None]:
        try:
            kind = row["operation_kind"]
            payload = row["payload_fingerprint"]
            state = row["state_fingerprint"]
            epoch = _decode_uint64(row["result_epoch"])
            revision_raw = row["result_revision"]
            revision = None if revision_raw is None else _decode_uint64(revision_raw)
            if (
                not isinstance(kind, str)
                or not isinstance(payload, bytes)
                or len(payload) != 32
                or not isinstance(state, bytes)
                or len(state) != 32
            ):
                raise StorageCorruption
            return kind, payload, state, epoch, revision
        except AgentDomainFailure:
            raise
        except (IndexError, KeyError, TypeError):
            raise StorageCorruption from None

    async def _claim_active_operation(
        self,
        record: MemoryRecord,
        *,
        require_existing: bool,
    ) -> None:
        if not record.active or not isinstance(record.memory, MemoryStateNonzero):
            raise StorageCorruption
        summary = _encode_summary(record.memory.summary)
        payload = _fingerprint(
            b"active-mirror",
            _text_bytes(record.namespace_id),
            _text_bytes(record.companion_id),
            _encode_uint64(record.memory_epoch),
            _encode_uint64(record.memory.revision),
            _text_bytes(record.memory.operation_id),
            summary,
        )
        state = _active_state_fingerprint(
            record.namespace_id,
            record.companion_id,
            record.memory_epoch,
            record.memory.revision,
            record.memory.operation_id,
            summary,
        )
        row = await self._operation_row(
            record.namespace_id,
            record.companion_id,
            record.memory.operation_id,
        )
        if row is None:
            if require_existing:
                raise StorageCorruption
            await self._insert_operation(
                record.namespace_id,
                record.companion_id,
                record.memory.operation_id,
                kind="active_mirror",
                payload_fingerprint=payload,
                state_fingerprint=state,
                result_epoch=record.memory_epoch,
                result_revision=record.memory.revision,
            )
            return
        kind, stored_payload, stored_state, epoch, revision = self._receipt_values(row)
        if (
            kind not in {"commit", "active_mirror"}
            or (kind == "active_mirror" and stored_payload != payload)
            or stored_state != state
            or epoch != record.memory_epoch
            or revision != record.memory.revision
        ):
            raise MemoryConflict

    async def _claim_reconcile_operation(
        self,
        record: MemoryRecord,
        *,
        require_existing: bool,
    ) -> None:
        if record.active:
            if isinstance(record.memory, MemoryStateZero):
                return
            await self._claim_active_operation(record, require_existing=require_existing)
            return
        await self._claim_tombstone_operation(record, require_existing=require_existing)

    async def _validate_seen_reconcile_operation(self, record: MemoryRecord) -> None:
        if record.active:
            if not isinstance(record.memory, MemoryStateNonzero):
                return
            operation_id = record.memory.operation_id
        else:
            if record.tombstone_operation_id is None:
                raise StorageCorruption
            operation_id = record.tombstone_operation_id
        if (
            await self._operation_row(
                record.namespace_id,
                record.companion_id,
                operation_id,
            )
            is not None
        ):
            await self._claim_reconcile_operation(record, require_existing=True)

    async def _claim_tombstone_operation(
        self,
        record: MemoryRecord,
        *,
        require_existing: bool,
    ) -> None:
        if record.active or record.tombstone_operation_id is None:
            raise StorageCorruption
        payload = _fingerprint(
            b"tombstone",
            _text_bytes(record.namespace_id),
            _text_bytes(record.companion_id),
            _encode_uint64(record.memory_epoch),
            _text_bytes(record.tombstone_operation_id),
        )
        state = _tombstone_state_fingerprint(
            record.namespace_id,
            record.companion_id,
            record.memory_epoch,
            record.tombstone_operation_id,
        )
        row = await self._operation_row(
            record.namespace_id,
            record.companion_id,
            record.tombstone_operation_id,
        )
        if row is None:
            if require_existing:
                raise StorageCorruption
            await self._insert_operation(
                record.namespace_id,
                record.companion_id,
                record.tombstone_operation_id,
                kind="tombstone",
                payload_fingerprint=payload,
                state_fingerprint=state,
                result_epoch=record.memory_epoch,
                result_revision=None,
            )
            return
        kind, stored_payload, stored_state, epoch, revision = self._receipt_values(row)
        if (
            kind != "tombstone"
            or stored_payload != payload
            or stored_state != state
            or epoch != record.memory_epoch
            or revision is not None
        ):
            raise MemoryConflict

    async def _existing_commit_result(
        self,
        identity: LeaseIdentity,
        command: MemoryCommit,
        summary: bytes,
    ) -> int | None:
        row = await self._operation_row(
            command.namespace_id,
            command.companion_id,
            command.operation_id,
        )
        if row is None:
            return None
        if command.base_revision == UINT64_MAX:
            raise MemoryConflict
        committed_revision = command.base_revision + 1
        payload = _fingerprint(
            b"commit",
            _text_bytes(command.namespace_id),
            _text_bytes(command.companion_id),
            _text_bytes(command.operation_id),
            _text_bytes(identity.lease_id),
            _encode_uint64(command.memory_epoch),
            _encode_uint64(command.base_revision),
            summary,
        )
        state = _active_state_fingerprint(
            command.namespace_id,
            command.companion_id,
            command.memory_epoch,
            committed_revision,
            command.operation_id,
            summary,
        )
        kind, stored_payload, stored_state, epoch, revision = self._receipt_values(row)
        if (
            kind != "commit"
            or stored_payload != payload
            or stored_state != state
            or epoch != command.memory_epoch
            or revision != committed_revision
        ):
            raise MemoryConflict
        return committed_revision

    async def _insert_commit_operation(
        self,
        identity: LeaseIdentity,
        command: MemoryCommit,
        summary: bytes,
        committed_revision: int,
    ) -> None:
        await self._insert_operation(
            command.namespace_id,
            command.companion_id,
            command.operation_id,
            kind="commit",
            payload_fingerprint=_fingerprint(
                b"commit",
                _text_bytes(command.namespace_id),
                _text_bytes(command.companion_id),
                _text_bytes(command.operation_id),
                _text_bytes(identity.lease_id),
                _encode_uint64(command.memory_epoch),
                _encode_uint64(command.base_revision),
                summary,
            ),
            state_fingerprint=_active_state_fingerprint(
                command.namespace_id,
                command.companion_id,
                command.memory_epoch,
                committed_revision,
                command.operation_id,
                summary,
            ),
            result_epoch=command.memory_epoch,
            result_revision=committed_revision,
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
            old_epoch_raw = row["tombstone_old_epoch"]
            if old_epoch_raw is not None:
                old_epoch = _decode_uint64(old_epoch_raw)
                if old_epoch == UINT64_MAX or old_epoch + 1 != epoch:
                    raise StorageCorruption
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
        if self._closed or self._broken:
            raise MemoryStorageFailure


__all__ = ["SQLiteMemoryStore"]

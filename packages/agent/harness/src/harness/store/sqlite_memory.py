"""只保存 namespace lease 与 compact MemoryState 的 SQLite adapter。"""

from __future__ import annotations

import asyncio
import sqlite3
import time
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Self

import aiosqlite

from harness.domain.memory import (
    LEASE_TTL_MS,
    AgentDomainFailure,
    ExpiredLease,
    LeaseGrant,
    LeaseIdentity,
    LeaseIDReuse,
    LeaseNotFound,
    LeaseTransition,
    MemoryStorageFailure,
    NamespaceConflict,
    StorageCorruption,
)
from harness.persistence.sqlite_schema import (
    _EXPECTED_COLUMNS,
    _SCHEMA_DDL,
    _SCHEMA_VERSION,
    _normalize_sql,
)
from harness.store import memory_ops as _memory_ops


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


def _default_clock_ms() -> int:
    return time.time_ns() // 1_000_000


class SQLiteMemoryStore:
    """单连接 adapter；所有 writer 使用一个显式 `BEGIN IMMEDIATE` 临界区。"""

    # 记忆操作实现见 `memory_ops`，此处挂载以保持单一对外接口。
    reconcile = _memory_ops.reconcile
    load = _memory_ops.load
    commit = _memory_ops.commit
    delete = _memory_ops.delete

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
            "memory_operations": [
                (
                    0,
                    0,
                    "namespace_lease_history",
                    "commit_lease_id",
                    "lease_id",
                    "NO ACTION",
                    "NO ACTION",
                    "NONE",
                )
            ],
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
        await _memory_ops._validate_current_memory_receipts(self)

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

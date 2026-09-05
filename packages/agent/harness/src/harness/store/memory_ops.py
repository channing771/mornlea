"""compact MemoryState 的四种记忆操作与 fingerprint 幂等回执。"""

from __future__ import annotations

import hashlib
import sqlite3
from typing import TYPE_CHECKING

from pydantic import TypeAdapter, ValidationError

from harness.domain.common import UINT64_MAX, UUIDv4
from harness.domain.memory import (
    AgentDomainFailure,
    LeaseIdentity,
    MemoryCommit,
    MemoryCommitResult,
    MemoryConflict,
    MemoryDelete,
    MemoryLookup,
    MemoryReconcile,
    MemoryRecord,
    MemoryStateNonzero,
    MemoryStateZero,
    RevisionOverflow,
    StorageCorruption,
)

if TYPE_CHECKING:
    from harness.store.sqlite_memory import SQLiteMemoryStore

_UUID_ADAPTER = TypeAdapter(UUIDv4)


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


def _decode_uuid(value: object) -> str:
    try:
        return _UUID_ADAPTER.validate_python(value)
    except ValidationError as error:
        raise StorageCorruption from error


def _commit_payload_fingerprint(
    namespace_id: str,
    companion_id: str,
    operation_id: str,
    lease_id: str,
    memory_epoch: int,
    base_revision: int,
    summary: bytes,
) -> bytes:
    return _fingerprint(
        b"commit",
        _text_bytes(namespace_id),
        _text_bytes(companion_id),
        _text_bytes(operation_id),
        _text_bytes(lease_id),
        _encode_uint64(memory_epoch),
        _encode_uint64(base_revision),
        summary,
    )


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


async def reconcile(
    self: SQLiteMemoryStore,
    identity: LeaseIdentity,
    desired: MemoryReconcile,
) -> MemoryRecord:
    async with self._write_transaction():
        now = self._now_ms()
        await self._assert_current_in_transaction(identity, now)
        _require_namespace(identity, desired.namespace_id)
        row = await self._memory_row(desired.namespace_id, desired.companion_id)
        if row is None:
            record = _record_from_reconcile(desired)
            await _claim_reconcile_operation(self, record, require_existing=False)
            await _replace_memory(self, record, tombstone_old_epoch=None)
            return record

        current = _decode_record(self, row)
        if desired.memory_epoch > current.memory_epoch:
            record = _record_from_reconcile(desired)
            await _claim_reconcile_operation(self, record, require_existing=False)
            await _replace_memory(self, record, tombstone_old_epoch=None)
            return record
        if desired.memory_epoch < current.memory_epoch:
            raise MemoryConflict
        if desired.active != current.active:
            raise MemoryConflict
        if not desired.active:
            if desired.tombstone_operation_id != current.tombstone_operation_id:
                raise MemoryConflict
            await _claim_reconcile_operation(self, current, require_existing=True)
            return current

        if desired.mirror is None or current.memory is None:
            raise StorageCorruption
        if desired.mirror.revision > current.memory.revision:
            record = _record_from_reconcile(desired)
            await _claim_reconcile_operation(self, record, require_existing=False)
            await _replace_memory(self, record, tombstone_old_epoch=None)
            return record
        if desired.mirror.revision < current.memory.revision:
            await _claim_reconcile_operation(self, current, require_existing=True)
            await _validate_seen_reconcile_operation(self, _record_from_reconcile(desired))
            return current
        if desired.mirror.operation_id != current.memory.operation_id or _encode_summary(
            desired.mirror.summary
        ) != _encode_summary(current.memory.summary):
            raise MemoryConflict
        await _claim_reconcile_operation(self, current, require_existing=True)
        return current


async def load(
    self: SQLiteMemoryStore,
    identity: LeaseIdentity,
    lookup: MemoryLookup,
) -> MemoryStateZero | MemoryStateNonzero:
    """读取 Python 运行期权威摘要，不接受 Go mirror 作为普通提示来源。"""

    async with self._write_transaction():
        now = self._now_ms()
        await self._assert_current_in_transaction(identity, now)
        _require_namespace(identity, lookup.namespace_id)
        row = await self._memory_row(lookup.namespace_id, lookup.companion_id)
        if row is None:
            raise MemoryConflict
        current = _decode_record(self, row)
        if (
            not current.active
            or current.memory_epoch != lookup.memory_epoch
            or current.memory is None
        ):
            raise MemoryConflict
        return current.memory


async def commit(
    self: SQLiteMemoryStore,
    identity: LeaseIdentity,
    command: MemoryCommit,
) -> MemoryCommitResult:
    async with self._write_transaction():
        now = self._now_ms()
        await self._assert_current_in_transaction(identity, now)
        _require_namespace(identity, command.namespace_id)
        summary = _encode_summary(command.summary)
        row = await self._memory_row(command.namespace_id, command.companion_id)
        if row is None:
            raise MemoryConflict
        current = _decode_record(self, row)
        if not current.active or current.memory_epoch != command.memory_epoch:
            raise MemoryConflict
        if current.memory is None:
            raise StorageCorruption

        existing_revision = await _existing_commit_result(self, identity, command, summary)
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
        await _insert_commit_operation(
            self,
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
    self: SQLiteMemoryStore,
    identity: LeaseIdentity,
    command: MemoryDelete,
) -> MemoryRecord:
    async with self._write_transaction():
        now = self._now_ms()
        await self._assert_current_in_transaction(identity, now)
        _require_namespace(identity, command.namespace_id)
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
            await _claim_tombstone_operation(self, desired, require_existing=False)
            await _replace_memory(
                self,
                desired,
                tombstone_old_epoch=command.old_memory_epoch,
            )
            return desired
        current = _decode_record(self, row)
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
            await _claim_tombstone_operation(self, current, require_existing=True)
            return current
        await _claim_tombstone_operation(self, desired, require_existing=False)
        await _replace_memory(
            self,
            desired,
            tombstone_old_epoch=command.old_memory_epoch,
        )
        return desired


async def _validate_current_memory_receipts(self: SQLiteMemoryStore) -> None:
    rows = await self._fetchall(
        "SELECT * FROM companion_memory ORDER BY namespace_id, companion_id"
    )
    for row in rows:
        try:
            record = _decode_record(self, row)
            await _claim_reconcile_operation(self, record, require_existing=True)
        except StorageCorruption:
            raise
        except MemoryConflict:
            raise StorageCorruption from None


async def _replace_memory(
    self: SQLiteMemoryStore,
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
    self: SQLiteMemoryStore,
    namespace_id: str,
    companion_id: str,
    operation_id: str,
) -> sqlite3.Row | None:
    return await self._fetchone(
        """
        SELECT operation_kind, commit_lease_id, payload_fingerprint,
               state_fingerprint, result_epoch, result_revision
        FROM memory_operations
        WHERE namespace_id = ? AND companion_id = ? AND operation_id = ?
        """,
        (namespace_id, companion_id, operation_id),
    )


async def _insert_operation(
    self: SQLiteMemoryStore,
    namespace_id: str,
    companion_id: str,
    operation_id: str,
    *,
    kind: str,
    commit_lease_id: str | None,
    payload_fingerprint: bytes,
    state_fingerprint: bytes,
    result_epoch: int,
    result_revision: int | None,
) -> None:
    await self._execute_statement(
        """
        INSERT INTO memory_operations (
            namespace_id, companion_id, operation_id, operation_kind,
            commit_lease_id, payload_fingerprint, state_fingerprint,
            result_epoch, result_revision
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        (
            namespace_id,
            companion_id,
            operation_id,
            kind,
            commit_lease_id,
            payload_fingerprint,
            state_fingerprint,
            _encode_uint64(result_epoch),
            None if result_revision is None else _encode_uint64(result_revision),
        ),
    )


def _receipt_values(
    row: sqlite3.Row,
) -> tuple[str, str | None, bytes, bytes, int, int | None]:
    try:
        kind = row["operation_kind"]
        commit_lease_raw = row["commit_lease_id"]
        payload = row["payload_fingerprint"]
        state = row["state_fingerprint"]
        epoch = _decode_uint64(row["result_epoch"])
        revision_raw = row["result_revision"]
        revision = None if revision_raw is None else _decode_uint64(revision_raw)
        if (
            kind not in {"commit", "active_mirror", "tombstone"}
            or not isinstance(payload, bytes)
            or len(payload) != 32
            or not isinstance(state, bytes)
            or len(state) != 32
        ):
            raise StorageCorruption
        if kind == "commit":
            commit_lease_id = _decode_uuid(commit_lease_raw)
            if revision is None:
                raise StorageCorruption
        else:
            if commit_lease_raw is not None:
                raise StorageCorruption
            commit_lease_id = None
            if (kind == "tombstone") != (revision is None):
                raise StorageCorruption
        return kind, commit_lease_id, payload, state, epoch, revision
    except AgentDomainFailure:
        raise
    except (IndexError, KeyError, TypeError):
        raise StorageCorruption from None


async def _claim_active_operation(
    self: SQLiteMemoryStore,
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
    row = await _operation_row(
        self,
        record.namespace_id,
        record.companion_id,
        record.memory.operation_id,
    )
    if row is None:
        if require_existing:
            raise StorageCorruption
        await _insert_operation(
            self,
            record.namespace_id,
            record.companion_id,
            record.memory.operation_id,
            kind="active_mirror",
            commit_lease_id=None,
            payload_fingerprint=payload,
            state_fingerprint=state,
            result_epoch=record.memory_epoch,
            result_revision=record.memory.revision,
        )
        return
    (
        kind,
        commit_lease_id,
        stored_payload,
        stored_state,
        epoch,
        revision,
    ) = _receipt_values(row)
    expected_payload = payload
    if kind == "commit":
        if commit_lease_id is None or record.memory.revision == 0:
            raise StorageCorruption
        expected_payload = _commit_payload_fingerprint(
            record.namespace_id,
            record.companion_id,
            record.memory.operation_id,
            commit_lease_id,
            record.memory_epoch,
            record.memory.revision - 1,
            summary,
        )
    if (
        kind not in {"commit", "active_mirror"}
        or stored_payload != expected_payload
        or stored_state != state
        or epoch != record.memory_epoch
        or revision != record.memory.revision
    ):
        raise MemoryConflict


async def _claim_reconcile_operation(
    self: SQLiteMemoryStore,
    record: MemoryRecord,
    *,
    require_existing: bool,
) -> None:
    if record.active:
        if isinstance(record.memory, MemoryStateZero):
            return
        await _claim_active_operation(self, record, require_existing=require_existing)
        return
    await _claim_tombstone_operation(self, record, require_existing=require_existing)


async def _validate_seen_reconcile_operation(self: SQLiteMemoryStore, record: MemoryRecord) -> None:
    if record.active:
        if not isinstance(record.memory, MemoryStateNonzero):
            return
        operation_id = record.memory.operation_id
    else:
        if record.tombstone_operation_id is None:
            raise StorageCorruption
        operation_id = record.tombstone_operation_id
    if (
        await _operation_row(
            self,
            record.namespace_id,
            record.companion_id,
            operation_id,
        )
        is not None
    ):
        await _claim_reconcile_operation(self, record, require_existing=True)


async def _claim_tombstone_operation(
    self: SQLiteMemoryStore,
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
    row = await _operation_row(
        self,
        record.namespace_id,
        record.companion_id,
        record.tombstone_operation_id,
    )
    if row is None:
        if require_existing:
            raise StorageCorruption
        await _insert_operation(
            self,
            record.namespace_id,
            record.companion_id,
            record.tombstone_operation_id,
            kind="tombstone",
            commit_lease_id=None,
            payload_fingerprint=payload,
            state_fingerprint=state,
            result_epoch=record.memory_epoch,
            result_revision=None,
        )
        return
    (
        kind,
        _commit_lease_id,
        stored_payload,
        stored_state,
        epoch,
        revision,
    ) = _receipt_values(row)
    if (
        kind != "tombstone"
        or stored_payload != payload
        or stored_state != state
        or epoch != record.memory_epoch
        or revision is not None
    ):
        raise MemoryConflict


async def _existing_commit_result(
    self: SQLiteMemoryStore,
    identity: LeaseIdentity,
    command: MemoryCommit,
    summary: bytes,
) -> int | None:
    row = await _operation_row(
        self,
        command.namespace_id,
        command.companion_id,
        command.operation_id,
    )
    if row is None:
        return None
    if command.base_revision == UINT64_MAX:
        raise MemoryConflict
    committed_revision = command.base_revision + 1
    payload = _commit_payload_fingerprint(
        command.namespace_id,
        command.companion_id,
        command.operation_id,
        identity.lease_id,
        command.memory_epoch,
        command.base_revision,
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
    (
        kind,
        commit_lease_id,
        stored_payload,
        stored_state,
        epoch,
        revision,
    ) = _receipt_values(row)
    if (
        kind != "commit"
        or commit_lease_id != identity.lease_id
        or stored_payload != payload
        or stored_state != state
        or epoch != command.memory_epoch
        or revision != committed_revision
    ):
        raise MemoryConflict
    return committed_revision


async def _insert_commit_operation(
    self: SQLiteMemoryStore,
    identity: LeaseIdentity,
    command: MemoryCommit,
    summary: bytes,
    committed_revision: int,
) -> None:
    await _insert_operation(
        self,
        command.namespace_id,
        command.companion_id,
        command.operation_id,
        kind="commit",
        commit_lease_id=identity.lease_id,
        payload_fingerprint=_commit_payload_fingerprint(
            command.namespace_id,
            command.companion_id,
            command.operation_id,
            identity.lease_id,
            command.memory_epoch,
            command.base_revision,
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


def _decode_record(self: SQLiteMemoryStore, row: sqlite3.Row) -> MemoryRecord:
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


def _record_from_reconcile(desired: MemoryReconcile) -> MemoryRecord:
    return MemoryRecord(
        namespace_id=desired.namespace_id,
        companion_id=desired.companion_id,
        memory_epoch=desired.memory_epoch,
        active=desired.active,
        tombstone_operation_id=desired.tombstone_operation_id,
        memory=desired.mirror,
    )


def _require_namespace(identity: LeaseIdentity, namespace_id: str) -> None:
    if identity.namespace_id != namespace_id:
        raise MemoryConflict

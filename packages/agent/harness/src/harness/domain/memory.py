"""伙伴 Agent 租约与 compact memory 的领域值。"""

from __future__ import annotations

from collections.abc import Mapping
from typing import ClassVar, Literal

from pydantic import StrictBool, model_validator

from harness.domain.common import (
    MemorySummary,
    PositiveUInt64,
    StrictModel,
    UInt64,
    UUIDv4,
)

LEASE_TTL_MS: Literal[15000] = 15_000
HEARTBEAT_INTERVAL_MS: Literal[5000] = 5_000


class AgentDomainFailure(Exception):
    """提供给 HTTP 层映射的稳定领域失败，不携带不可信正文。"""

    code: ClassVar[str] = "internal_error"
    public_message: ClassVar[str] = "agent operation failed"

    def __init__(self) -> None:
        super().__init__(self.public_message)


class NamespaceConflict(AgentDomainFailure):
    code = "namespace_conflict"
    public_message = "namespace is already leased"


class LeaseNotFound(AgentDomainFailure):
    code = "not_found"
    public_message = "lease was not found"


class LeaseIDReuse(AgentDomainFailure):
    """租约 ID 已签发过；manager 应生成新 ID，而非复用 fence。"""

    public_message = "lease identity is unavailable"


class RunOverloaded(AgentDomainFailure):
    code = "overloaded"
    public_message = "run capacity is unavailable"


class MemoryConflict(AgentDomainFailure):
    code = "memory_conflict"
    public_message = "memory state conflicts"


class RevisionOverflow(MemoryConflict):
    public_message = "memory counter cannot advance"


class MemoryStorageFailure(AgentDomainFailure):
    public_message = "memory storage operation failed"


class StorageCorruption(MemoryStorageFailure):
    public_message = "memory storage is invalid"


class LeaseIdentity(StrictModel):
    namespace_id: UUIDv4
    client_instance_id: UUIDv4
    lease_id: UUIDv4


class LeaseGrant(LeaseIdentity):
    exact_constants: ClassVar[Mapping[str, object]] = {"lease_expires_in_ms": LEASE_TTL_MS}
    lease_expires_in_ms: Literal[15000]


class LeaseTransition(StrictModel):
    grant: LeaseGrant
    replaced_lease_id: UUIDv4 | None


class ExpiredLease(LeaseIdentity):
    pass


class MemoryStateZero(StrictModel):
    exact_constants: ClassVar[Mapping[str, object]] = {"revision": 0}
    revision: Literal[0]
    operation_id: None
    summary: Literal[""]


class MemoryStateNonzero(StrictModel):
    revision: PositiveUInt64
    operation_id: UUIDv4
    summary: MemorySummary


MemoryState = MemoryStateZero | MemoryStateNonzero


class MemoryReconcile(StrictModel):
    namespace_id: UUIDv4
    companion_id: UUIDv4
    memory_epoch: PositiveUInt64
    active: StrictBool
    tombstone_operation_id: UUIDv4 | None
    mirror: MemoryState | None

    @model_validator(mode="after")
    def validate_state_shape(self) -> MemoryReconcile:
        if self.active:
            if self.tombstone_operation_id is not None or self.mirror is None:
                raise ValueError("active memory requires only a mirror")
        elif self.tombstone_operation_id is None or self.mirror is not None:
            raise ValueError("inactive memory requires only a tombstone")
        return self


class MemoryRecord(StrictModel):
    namespace_id: UUIDv4
    companion_id: UUIDv4
    memory_epoch: PositiveUInt64
    active: StrictBool
    tombstone_operation_id: UUIDv4 | None
    memory: MemoryState | None

    @model_validator(mode="after")
    def validate_state_shape(self) -> MemoryRecord:
        if self.active:
            if self.tombstone_operation_id is not None or self.memory is None:
                raise ValueError("active record requires only memory")
        elif self.tombstone_operation_id is None or self.memory is not None:
            raise ValueError("inactive record requires only a tombstone")
        return self


class MemoryLookup(StrictModel):
    namespace_id: UUIDv4
    companion_id: UUIDv4
    memory_epoch: PositiveUInt64


class MemoryCommit(StrictModel):
    namespace_id: UUIDv4
    companion_id: UUIDv4
    memory_epoch: PositiveUInt64
    base_revision: UInt64
    operation_id: UUIDv4
    summary: MemorySummary


class MemoryCommitResult(StrictModel):
    namespace_id: UUIDv4
    companion_id: UUIDv4
    memory_epoch: PositiveUInt64
    operation_id: UUIDv4
    committed_revision: PositiveUInt64


class MemoryDelete(StrictModel):
    namespace_id: UUIDv4
    companion_id: UUIDv4
    old_memory_epoch: PositiveUInt64
    new_memory_epoch: PositiveUInt64
    tombstone_operation_id: UUIDv4


__all__ = [
    "AgentDomainFailure",
    "ExpiredLease",
    "HEARTBEAT_INTERVAL_MS",
    "LEASE_TTL_MS",
    "LeaseGrant",
    "LeaseIDReuse",
    "LeaseIdentity",
    "LeaseNotFound",
    "LeaseTransition",
    "MemoryCommit",
    "MemoryCommitResult",
    "MemoryConflict",
    "MemoryDelete",
    "MemoryLookup",
    "MemoryRecord",
    "MemoryReconcile",
    "MemoryState",
    "MemoryStateNonzero",
    "MemoryStateZero",
    "MemoryStorageFailure",
    "NamespaceConflict",
    "RevisionOverflow",
    "RunOverloaded",
    "StorageCorruption",
]

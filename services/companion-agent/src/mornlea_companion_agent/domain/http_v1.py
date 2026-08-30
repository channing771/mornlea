"""Agent HTTP application contract v1 的严格领域模型。"""

from __future__ import annotations

from typing import Annotated, Any, Literal

from pydantic import BeforeValidator, Field, StrictInt

from mornlea_companion_agent.domain.common import (
    SHA256,
    BlockID,
    BlockPosition,
    DialogueLine,
    InstructionText,
    Int32,
    MCPCapability,
    MCPEndpoint,
    MemorySummary,
    PersonaText,
    Position,
    PositiveUInt64,
    StrictModel,
    UInt64,
    UUIDv4,
    json_tuple,
)
from mornlea_companion_agent.domain.mcp_v1 import Plan

ContractVersion = Literal["v1"]
DeadlineUnixMilliseconds = Annotated[StrictInt, Field(ge=1, le=(1 << 63) - 1)]


class LiveResponse(StrictModel):
    status: Literal["live"]


class ReadyResponse(StrictModel):
    status: Literal["ready", "not_ready"]


class AcquireRequest(StrictModel):
    contract_version: ContractVersion
    request_id: UUIDv4
    client_instance_id: UUIDv4
    namespace_id: UUIDv4


class AcquireResponse(AcquireRequest):
    lease_id: UUIDv4
    lease_expires_in_ms: Literal[15000]


class LeaseRequest(AcquireRequest):
    lease_id: UUIDv4


class HeartbeatResponse(LeaseRequest):
    lease_expires_in_ms: Literal[15000]


class ReleaseResponse(LeaseRequest):
    released: Literal[True]


class PlanRequest(LeaseRequest):
    run_id: UUIDv4
    companion_id: UUIDv4
    generation: PositiveUInt64
    snapshot_id: UUIDv4
    snapshot_digest: SHA256
    deadline_unix_ms: DeadlineUnixMilliseconds
    mcp_endpoint: MCPEndpoint
    mcp_capability: MCPCapability
    instruction: InstructionText


class PlanResponse(LeaseRequest):
    run_id: UUIDv4
    companion_id: UUIDv4
    generation: PositiveUInt64
    snapshot_id: UUIDv4
    snapshot_digest: SHA256
    plan: Plan


class StartFactNode(StrictModel):
    kind: Literal["start"]


class ProgressFactNode(StrictModel):
    kind: Literal["progress"]
    step_kind: Literal["go_to", "mine", "place"]


class FirstArrivalFactNode(StrictModel):
    kind: Literal["first_arrival"]


class IdleFactNode(StrictModel):
    kind: Literal["idle"]


DialogueNonterminalFactNode = Annotated[
    StartFactNode | ProgressFactNode | FirstArrivalFactNode | IdleFactNode,
    Field(discriminator="kind"),
]


class DialogueTerminalNonfailedFactNode(StrictModel):
    kind: Literal["terminal"]
    state: Literal["completed", "timed_out", "stopped"]
    reason: Literal["none"]


class DialogueTerminalFailedFactNode(StrictModel):
    kind: Literal["terminal"]
    state: Literal["failed"]
    reason: Literal[
        "planner_unavailable",
        "invalid_plan",
        "path_unreachable",
        "world_changed",
        "inventory_full",
    ]


DialogueTerminalFactNode = Annotated[
    DialogueTerminalNonfailedFactNode | DialogueTerminalFailedFactNode,
    Field(discriminator="state"),
]
DialogueFactNode = (
    StartFactNode
    | ProgressFactNode
    | FirstArrivalFactNode
    | IdleFactNode
    | DialogueTerminalNonfailedFactNode
    | DialogueTerminalFailedFactNode
)


class ExposedBlock(StrictModel):
    position: BlockPosition
    block_id: BlockID


class HeightSample(StrictModel):
    x: Int32
    z: Int32
    height: Annotated[StrictInt, Field(ge=-65, le=319)]


ExposedBlocks = Annotated[
    tuple[ExposedBlock, ...],
    BeforeValidator(json_tuple),
    Field(max_length=256),
]
HeightSamples = Annotated[
    tuple[HeightSample, ...],
    BeforeValidator(json_tuple),
    Field(max_length=1089),
]


class DialogueEnvironment(StrictModel):
    exposed_blocks: ExposedBlocks
    heights: HeightSamples


class DialogueRunIdentity(LeaseRequest):
    run_id: UUIDv4
    companion_id: UUIDv4
    generation: PositiveUInt64
    memory_epoch: PositiveUInt64


class DialogueNonterminalRequest(DialogueRunIdentity):
    deadline_unix_ms: DeadlineUnixMilliseconds
    persona: PersonaText
    fact_node: DialogueNonterminalFactNode
    environment: DialogueEnvironment
    terminal: Literal[False]


class DialogueTerminalRequest(DialogueRunIdentity):
    deadline_unix_ms: DeadlineUnixMilliseconds
    persona: PersonaText
    fact_node: DialogueTerminalFactNode
    environment: DialogueEnvironment
    terminal: Literal[True]


DialogueRequest = Annotated[
    DialogueNonterminalRequest | DialogueTerminalRequest,
    Field(discriminator="terminal"),
]


class MemoryProposal(StrictModel):
    operation_id: UUIDv4
    base_revision: UInt64
    summary: MemorySummary


class DialogueNonterminalResponse(DialogueRunIdentity):
    line: DialogueLine


class DialogueTerminalResponse(DialogueRunIdentity):
    line: DialogueLine
    memory_proposal: MemoryProposal


DialogueResponse = DialogueNonterminalResponse | DialogueTerminalResponse


class MemoryStateZero(StrictModel):
    revision: Literal[0]
    operation_id: None
    summary: Literal[""]


class MemoryStateNonzero(StrictModel):
    revision: PositiveUInt64
    operation_id: UUIDv4
    summary: MemorySummary


MemoryState = MemoryStateZero | MemoryStateNonzero


class MemoryIdentity(LeaseRequest):
    companion_id: UUIDv4
    memory_epoch: PositiveUInt64


class MemoryReconcileActiveRequest(MemoryIdentity):
    active: Literal[True]
    tombstone_operation_id: None
    mirror: MemoryState


class MemoryReconcileInactiveRequest(MemoryIdentity):
    active: Literal[False]
    tombstone_operation_id: UUIDv4
    mirror: None


MemoryReconcileRequest = Annotated[
    MemoryReconcileActiveRequest | MemoryReconcileInactiveRequest,
    Field(discriminator="active"),
]


class MemoryReconcileActiveResponse(MemoryIdentity):
    active: Literal[True]
    tombstone_operation_id: None
    memory: MemoryState


class MemoryReconcileInactiveResponse(MemoryIdentity):
    active: Literal[False]
    tombstone_operation_id: UUIDv4
    memory: None


MemoryReconcileResponse = Annotated[
    MemoryReconcileActiveResponse | MemoryReconcileInactiveResponse,
    Field(discriminator="active"),
]


class MemoryCommitRequest(MemoryIdentity):
    base_revision: UInt64
    operation_id: UUIDv4
    summary: MemorySummary


class MemoryCommitResponse(MemoryIdentity):
    operation_id: UUIDv4
    committed_revision: PositiveUInt64


class MemoryDeleteRequest(LeaseRequest):
    companion_id: UUIDv4
    old_memory_epoch: PositiveUInt64
    new_memory_epoch: PositiveUInt64
    tombstone_operation_id: UUIDv4


class MemoryDeleteResponse(MemoryIdentity):
    tombstone_operation_id: UUIDv4


class CancelRequest(LeaseRequest):
    run_id: UUIDv4


class CancelResponse(CancelRequest):
    cancelled: bool


ErrorCode = Literal[
    "invalid_request",
    "unauthorized",
    "unsupported_version",
    "namespace_conflict",
    "overloaded",
    "deadline_exceeded",
    "agent_unavailable",
    "invalid_model_output",
    "memory_conflict",
    "not_found",
    "internal_error",
]


class ErrorDetail(StrictModel):
    code: ErrorCode


class ErrorResponse(StrictModel):
    contract_version: ContractVersion
    request_id: UUIDv4 | None
    error: ErrorDetail


HTTP_V1_TYPES: dict[str, Any] = {
    "uuid_v4": UUIDv4,
    "sha256": SHA256,
    "uint64": UInt64,
    "positive_uint64": PositiveUInt64,
    "instruction_text": InstructionText,
    "persona_text": PersonaText,
    "dialogue_line": DialogueLine,
    "memory_summary": MemorySummary,
    "mcp_capability": MCPCapability,
    "mcp_endpoint": MCPEndpoint,
    "position": Position,
    "block_position": BlockPosition,
    "live_response": LiveResponse,
    "ready_response": ReadyResponse,
    "acquire_request": AcquireRequest,
    "acquire_response": AcquireResponse,
    "lease_request": LeaseRequest,
    "heartbeat_response": HeartbeatResponse,
    "release_response": ReleaseResponse,
    "plan_request": PlanRequest,
    "plan_response": PlanResponse,
    "dialogue_nonterminal_fact_node": DialogueNonterminalFactNode,
    "dialogue_terminal_nonfailed_fact_node": DialogueTerminalNonfailedFactNode,
    "dialogue_terminal_failed_fact_node": DialogueTerminalFailedFactNode,
    "dialogue_terminal_fact_node": DialogueTerminalFactNode,
    "dialogue_fact_node": DialogueFactNode,
    "dialogue_environment": DialogueEnvironment,
    "dialogue_nonterminal_request": DialogueNonterminalRequest,
    "dialogue_terminal_request": DialogueTerminalRequest,
    "dialogue_request": DialogueRequest,
    "memory_proposal": MemoryProposal,
    "dialogue_nonterminal_response": DialogueNonterminalResponse,
    "dialogue_terminal_response": DialogueTerminalResponse,
    "dialogue_response": DialogueResponse,
    "memory_state_zero": MemoryStateZero,
    "memory_state_nonzero": MemoryStateNonzero,
    "memory_state": MemoryState,
    "memory_reconcile_active_request": MemoryReconcileActiveRequest,
    "memory_reconcile_inactive_request": MemoryReconcileInactiveRequest,
    "memory_reconcile_request": MemoryReconcileRequest,
    "memory_reconcile_active_response": MemoryReconcileActiveResponse,
    "memory_reconcile_inactive_response": MemoryReconcileInactiveResponse,
    "memory_reconcile_response": MemoryReconcileResponse,
    "memory_commit_request": MemoryCommitRequest,
    "memory_commit_response": MemoryCommitResponse,
    "memory_delete_request": MemoryDeleteRequest,
    "memory_delete_response": MemoryDeleteResponse,
    "cancel_request": CancelRequest,
    "cancel_response": CancelResponse,
    "error_response": ErrorResponse,
}


__all__ = [
    "AcquireRequest",
    "AcquireResponse",
    "CancelRequest",
    "CancelResponse",
    "DialogueEnvironment",
    "DialogueNonterminalRequest",
    "DialogueNonterminalResponse",
    "DialogueRequest",
    "DialogueResponse",
    "DialogueTerminalRequest",
    "DialogueTerminalResponse",
    "ErrorResponse",
    "HTTP_V1_TYPES",
    "HeartbeatResponse",
    "LeaseRequest",
    "LiveResponse",
    "MemoryCommitRequest",
    "MemoryCommitResponse",
    "MemoryDeleteRequest",
    "MemoryDeleteResponse",
    "MemoryProposal",
    "MemoryReconcileRequest",
    "MemoryReconcileResponse",
    "MemoryState",
    "PlanRequest",
    "PlanResponse",
    "ReadyResponse",
    "ReleaseResponse",
]

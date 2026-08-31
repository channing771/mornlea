"""MCP application tool contract v1 的严格领域模型。"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Annotated, Any, ClassVar, Literal, Self

from pydantic import (
    AfterValidator,
    BeforeValidator,
    Field,
    StrictInt,
    StrictStr,
    TypeAdapter,
    ValidationInfo,
    field_validator,
    model_validator,
)

from mornlea_companion_agent.domain.common import (
    SHA256,
    BlockID,
    BlockPosition,
    BoundedName,
    FiniteNumber,
    InstructionText,
    Int32,
    PlanSummary,
    Position,
    StrictModel,
    TaskStatusText,
    TerrainHeight,
    UInt64,
    UUIDv4,
    WorldY,
    json_tuple,
    require_canonical_size,
    text_validator,
)

ACCEPTED_MINE_SEMANTICS = ("single_drop", "container_batch")
REJECTED_MINE_SEMANTICS = (
    "forbidden_farming",
    "forbidden_torch",
    "no_drop",
    "undelivered_multi_drop",
)
MineSemantics = Literal[
    "single_drop",
    "container_batch",
    "forbidden_farming",
    "forbidden_torch",
    "no_drop",
    "undelivered_multi_drop",
]
PlaceBlock = Literal[
    "brick",
    "chest",
    "clay",
    "cobblestone",
    "dirt",
    "furnace",
    "glass",
    "grass",
    "gravel",
    "iron_block",
    "leaves",
    "light_block",
    "mossy_cobblestone",
    "oak_log",
    "oak_planks",
    "roof_tile",
    "sand",
    "smooth_stone",
    "snow_block",
    "stone",
    "stone_brick",
    "white_wool",
    "workbench",
]
ValidatorCode = Literal[
    "invalid_schema",
    "out_of_bounds",
    "unknown_player",
    "unmineable_target",
    "unknown_block",
    "missing_item",
    "snapshot_mismatch",
]
ValidatorHint = Annotated[
    StrictStr,
    AfterValidator(
        text_validator(
            minimum_bytes=1,
            maximum_bytes=256,
            no_nul=True,
            no_control=True,
            no_edge_whitespace=True,
        )
    ),
]


class GoToStep(StrictModel):
    kind: Literal["go_to"]
    x: Int32
    y: WorldY
    z: Int32


class MineStep(StrictModel):
    kind: Literal["mine"]
    x: Int32
    y: WorldY
    z: Int32


class PlaceStep(StrictModel):
    kind: Literal["place"]
    x: Int32
    y: WorldY
    z: Int32
    block: PlaceBlock


class FollowStep(StrictModel):
    kind: Literal["follow"]
    player_id: UUIDv4


PlanStep = Annotated[
    GoToStep | MineStep | PlaceStep | FollowStep,
    Field(discriminator="kind"),
]
PlanSteps = Annotated[
    tuple[PlanStep, ...],
    BeforeValidator(json_tuple),
    Field(min_length=1, max_length=5000),
]


class Plan(StrictModel):
    summary: PlanSummary
    steps: PlanSteps

    @field_validator("steps")
    @classmethod
    def validate_follow_is_last(cls, value: tuple[PlanStep, ...]) -> tuple[PlanStep, ...]:
        for index, step in enumerate(value[:-1]):
            if isinstance(step, FollowStep):
                raise ValueError(f"follow step at index {index} must be final")
        return value

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 65536, "plan")
        return self


class EmptyInput(StrictModel):
    pass


class ChunkRevision(StrictModel):
    x: Int32
    z: Int32
    revision: UInt64


class IssuerContext(StrictModel):
    player_id: UUIDv4
    position: Position
    yaw: FiniteNumber
    pitch: FiniteNumber
    look_hit: BlockPosition | None


class CompanionContext(StrictModel):
    companion_id: UUIDv4
    position: Position
    yaw: FiniteNumber
    pitch: FiniteNumber
    task_status: TaskStatusText


ChunkRevisions = Annotated[
    tuple[ChunkRevision, ...],
    BeforeValidator(json_tuple),
    Field(max_length=9),
]


class GetPlanningContextResult(StrictModel):
    snapshot_digest: SHA256
    instruction: InstructionText
    issuer: IssuerContext
    companion: CompanionContext
    world_time_ticks: UInt64
    chunk_revisions: ChunkRevisions

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 24576, "get_planning_context result")
        return self


class OnlinePlayer(StrictModel):
    player_id: UUIDv4
    position: Position


class VisibleBlock(StrictModel):
    position: BlockPosition
    block_id: BlockID
    block_name: BoundedName
    drop_item: BoundedName | None
    mine_semantics: MineSemantics


StepKinds = Annotated[
    tuple[
        Literal["go_to"],
        Literal["follow"],
        Literal["mine"],
        Literal["place"],
    ],
    BeforeValidator(json_tuple),
]
OnlinePlayers = Annotated[
    tuple[OnlinePlayer, ...],
    BeforeValidator(json_tuple),
    Field(max_length=8),
]
VisibleBlocks = Annotated[
    tuple[VisibleBlock, ...],
    BeforeValidator(json_tuple),
    Field(max_length=256),
]


class ListAffordancesResult(StrictModel):
    step_kinds: StepKinds
    online_players: OnlinePlayers
    visible_blocks: VisibleBlocks

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 24576, "list_affordances result")
        return self


class InspectInventoryInput(StrictModel):
    offset: StrictInt = Field(ge=0, le=35)
    limit: StrictInt = Field(ge=1, le=36)


class InventorySlot(StrictModel):
    slot: StrictInt = Field(ge=0, le=35)
    item: BoundedName
    count: StrictInt = Field(ge=1, le=64)


InventorySlots = Annotated[
    tuple[InventorySlot, ...],
    BeforeValidator(json_tuple),
    Field(max_length=36),
]


class InspectInventoryResult(StrictModel):
    slots: InventorySlots

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 8192, "inspect_inventory result")
        return self


BlockNames = Annotated[
    tuple[BoundedName, ...],
    BeforeValidator(json_tuple),
    Field(min_length=1, max_length=16),
]


class FindVisibleBlocksInput(StrictModel):
    block_names: BlockNames
    limit: StrictInt = Field(ge=1, le=64)

    @field_validator("block_names")
    @classmethod
    def validate_unique_names(cls, value: tuple[str, ...]) -> tuple[str, ...]:
        if len(set(value)) != len(value):
            raise ValueError("block_names must be unique")
        return value


class VisibleBlockMatch(StrictModel):
    position: BlockPosition
    block_name: BoundedName
    drop_item: BoundedName | None


VisibleBlockMatches = Annotated[
    tuple[VisibleBlockMatch, ...],
    BeforeValidator(json_tuple),
    Field(max_length=64),
]


class FindVisibleBlocksSuccessResult(StrictModel):
    matches: VisibleBlockMatches

    @field_validator("matches")
    @classmethod
    def validate_coordinate_sort(
        cls, value: tuple[VisibleBlockMatch, ...]
    ) -> tuple[VisibleBlockMatch, ...]:
        coordinates = [(item.position.x, item.position.y, item.position.z) for item in value]
        if any(left >= right for left, right in zip(coordinates, coordinates[1:], strict=False)):
            raise ValueError("matches must use strictly increasing x/y/z coordinate order")
        return value

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 16384, "find_visible_blocks result")
        return self


class FindVisibleBlocksFailureResult(StrictModel):
    exact_constants: ClassVar[Mapping[str, object]] = {"code": "unknown_block"}
    code: Literal["unknown_block"]
    hint: ValidatorHint


FindVisibleBlocksResult = FindVisibleBlocksSuccessResult | FindVisibleBlocksFailureResult


TerrainPositions = Annotated[
    tuple[BlockPosition, ...],
    BeforeValidator(json_tuple),
    Field(min_length=1, max_length=64),
]


class QueryTerrainInput(StrictModel):
    positions: TerrainPositions


class TerrainItem(StrictModel):
    position: BlockPosition
    height: TerrainHeight
    block_name: BoundedName


TerrainItems = Annotated[
    tuple[TerrainItem, ...],
    BeforeValidator(json_tuple),
    Field(max_length=64),
]
_TERRAIN_CONTEXT_ADAPTER = TypeAdapter(TerrainPositions)


class QueryTerrainSuccessResult(StrictModel):
    terrain: TerrainItems

    @field_validator("terrain")
    @classmethod
    def validate_input_positions(
        cls, value: tuple[TerrainItem, ...], info: ValidationInfo
    ) -> tuple[TerrainItem, ...]:
        if not isinstance(info.context, dict) or "positions" not in info.context:
            raise ValueError("query_terrain result requires input positions in validation context")
        expected = _TERRAIN_CONTEXT_ADAPTER.validate_python(info.context["positions"])
        if tuple(item.position for item in value) != expected:
            raise ValueError("terrain positions must match input count and order")
        return value

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 16384, "query_terrain result")
        return self


class QueryTerrainFailureResult(StrictModel):
    exact_constants: ClassVar[Mapping[str, object]] = {"code": "out_of_bounds"}
    code: Literal["out_of_bounds"]
    hint: ValidatorHint


QueryTerrainResult = QueryTerrainSuccessResult | QueryTerrainFailureResult


class ValidatePlanInput(StrictModel):
    plan: Plan

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 65536, "validate_plan input")
        return self


class ValidatePlanSuccessResult(StrictModel):
    exact_constants: ClassVar[Mapping[str, object]] = {"accepted": True}
    accepted: Literal[True]
    snapshot_digest: SHA256
    plan: Plan

    @model_validator(mode="after")
    def validate_payload_size(self) -> Self:
        require_canonical_size(self, 73728, "validate_plan success result")
        return self


class ValidatePlanFailureResult(StrictModel):
    exact_constants: ClassVar[Mapping[str, object]] = {"accepted": False}
    accepted: Literal[False]
    code: ValidatorCode
    hint: ValidatorHint


ValidatePlanResult = Annotated[
    ValidatePlanSuccessResult | ValidatePlanFailureResult,
    Field(discriminator="accepted"),
]


MCP_V1_TYPES: dict[str, Any] = {
    "uuid_v4": UUIDv4,
    "sha256": SHA256,
    "uint64": UInt64,
    "instruction_text": InstructionText,
    "plan_summary": PlanSummary,
    "task_status_text": TaskStatusText,
    "bounded_name": BoundedName,
    "validator_hint": ValidatorHint,
    "position": Position,
    "block_position": BlockPosition,
    "go_to_step": GoToStep,
    "mine_step": MineStep,
    "place_step": PlaceStep,
    "follow_step": FollowStep,
    "plan": Plan,
    "empty_input": EmptyInput,
    "get_planning_context_input": EmptyInput,
    "get_planning_context_result": GetPlanningContextResult,
    "list_affordances_input": EmptyInput,
    "list_affordances_result": ListAffordancesResult,
    "inspect_inventory_input": InspectInventoryInput,
    "inspect_inventory_result": InspectInventoryResult,
    "find_visible_blocks_input": FindVisibleBlocksInput,
    "find_visible_blocks_success_result": FindVisibleBlocksSuccessResult,
    "find_visible_blocks_failure_result": FindVisibleBlocksFailureResult,
    "find_visible_blocks_result": FindVisibleBlocksResult,
    "query_terrain_input": QueryTerrainInput,
    "query_terrain_success_result": QueryTerrainSuccessResult,
    "query_terrain_failure_result": QueryTerrainFailureResult,
    "query_terrain_result": QueryTerrainResult,
    "validate_plan_input": ValidatePlanInput,
    "validate_plan_success_result": ValidatePlanSuccessResult,
    "validate_plan_failure_result": ValidatePlanFailureResult,
    "validate_plan_result": ValidatePlanResult,
}


__all__ = [
    "ACCEPTED_MINE_SEMANTICS",
    "FindVisibleBlocksFailureResult",
    "FindVisibleBlocksInput",
    "FindVisibleBlocksResult",
    "FindVisibleBlocksSuccessResult",
    "FollowStep",
    "GetPlanningContextResult",
    "GoToStep",
    "InspectInventoryInput",
    "InspectInventoryResult",
    "ListAffordancesResult",
    "MCP_V1_TYPES",
    "MineStep",
    "Plan",
    "PlaceStep",
    "QueryTerrainFailureResult",
    "QueryTerrainInput",
    "QueryTerrainResult",
    "QueryTerrainSuccessResult",
    "REJECTED_MINE_SEMANTICS",
    "ValidatePlanFailureResult",
    "ValidatePlanInput",
    "ValidatePlanResult",
    "ValidatePlanSuccessResult",
]

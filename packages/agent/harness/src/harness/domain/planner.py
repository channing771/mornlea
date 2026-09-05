"""Planner graph 的稳定领域端口、预算与失败。"""

from __future__ import annotations

import json
from collections.abc import Mapping
from contextlib import AbstractAsyncContextManager
from dataclasses import dataclass
from typing import Any, ClassVar, Protocol

from pydantic import BaseModel, Field, StrictFloat, StrictInt, TypeAdapter, ValidationError

from harness.domain.common import StrictModel, canonical_json_bytes
from harness.domain.http_v1 import PlanRequest
from harness.domain.mcp_v1 import (
    EmptyInput,
    FindVisibleBlocksInput,
    FindVisibleBlocksResult,
    GetPlanningContextResult,
    InspectInventoryInput,
    InspectInventoryResult,
    ListAffordancesResult,
    Plan,
    QueryTerrainInput,
    QueryTerrainResult,
    ValidatePlanFailureResult,
    ValidatePlanSuccessResult,
)

MODEL_VISIBLE_TOOLS = (
    "list_affordances",
    "inspect_inventory",
    "find_visible_blocks",
    "query_terrain",
)
MAX_PLAN_BYTES = 65_536

_TOOL_INPUT_ADAPTERS: Mapping[str, TypeAdapter[Any]] = {
    "list_affordances": TypeAdapter(EmptyInput),
    "inspect_inventory": TypeAdapter(InspectInventoryInput),
    "find_visible_blocks": TypeAdapter(FindVisibleBlocksInput),
    "query_terrain": TypeAdapter(QueryTerrainInput),
}
_TOOL_OUTPUT_ADAPTERS: Mapping[str, TypeAdapter[Any]] = {
    "list_affordances": TypeAdapter(ListAffordancesResult),
    "inspect_inventory": TypeAdapter(InspectInventoryResult),
    "find_visible_blocks": TypeAdapter(FindVisibleBlocksResult),
    "query_terrain": TypeAdapter(QueryTerrainResult),
}


class PlannerFailure(Exception):
    """不携带上游正文的稳定 Planner 失败。"""

    code: ClassVar[str] = "agent_unavailable"
    public_message: ClassVar[str] = "planner operation failed"

    def __init__(self) -> None:
        super().__init__(self.public_message)


class PlannerUnavailable(PlannerFailure):
    code = "agent_unavailable"
    public_message = "planner dependency is unavailable"


class PlannerDeadlineExceeded(PlannerFailure):
    code = "deadline_exceeded"
    public_message = "planner deadline exceeded"


class InvalidModelOutput(PlannerFailure):
    code = "invalid_model_output"
    public_message = "model output is invalid"


class PlannerLimits(StrictModel):
    """单次 run 的预算；硬上限由领域模型直接钉死。"""

    model_calls: StrictInt = Field(default=3, ge=1, le=5)
    tool_calls: StrictInt = Field(default=4, ge=1, le=8)
    timeout_seconds: StrictInt | StrictFloat = Field(default=30, gt=0, le=60)


@dataclass(frozen=True, slots=True)
class ModelToolCall:
    call_id: str
    name: str
    arguments: object


@dataclass(frozen=True, slots=True)
class PlannerMessage:
    role: str
    content: str
    tool_calls: tuple[ModelToolCall, ...] = ()
    tool_call_id: str | None = None
    tool_name: str | None = None


@dataclass(frozen=True, slots=True)
class ModelOutput:
    content: object | None
    tool_calls: tuple[ModelToolCall, ...]
    invalid_tool_calls: bool = False


class PlannerModel(Protocol):
    async def complete(
        self,
        messages: tuple[PlannerMessage, ...],
        *,
        allow_tools: bool,
    ) -> ModelOutput: ...


class PlanningToolSession(Protocol):
    async def get_planning_context(self) -> GetPlanningContextResult: ...

    async def call_model_tool(self, name: str, arguments: dict[str, object]) -> object: ...

    async def validate_plan(
        self, plan: Plan
    ) -> ValidatePlanSuccessResult | ValidatePlanFailureResult: ...


class PlanningToolSessionFactory(Protocol):
    def open(
        self,
        request: PlanRequest,
        *,
        timeout_seconds: float,
    ) -> AbstractAsyncContextManager[PlanningToolSession]: ...


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON object key")
        result[key] = value
    return result


def _reject_constant(value: str) -> object:
    del value
    raise ValueError("non-finite JSON number")


def strict_json_object(value: str) -> dict[str, object]:
    """只解码一个无重复键、无非有限数值的 JSON object。"""

    try:
        decoded = json.loads(
            value,
            object_pairs_hook=_unique_object,
            parse_constant=_reject_constant,
        )
    except (json.JSONDecodeError, UnicodeError, ValueError, TypeError):
        raise InvalidModelOutput from None
    if type(decoded) is not dict:
        raise InvalidModelOutput
    return decoded


def parse_plan_output(content: object) -> Plan:
    if type(content) is not str:
        raise InvalidModelOutput
    try:
        raw_size = len(content.encode("utf-8", errors="strict"))
    except UnicodeError:
        raise InvalidModelOutput from None
    if raw_size > MAX_PLAN_BYTES:
        raise InvalidModelOutput
    decoded = strict_json_object(content)
    try:
        plan = Plan.model_validate(decoded)
    except ValidationError:
        raise InvalidModelOutput from None
    if len(canonical_json_bytes(plan)) > MAX_PLAN_BYTES:
        raise InvalidModelOutput
    return plan


def validate_tool_input(name: str, arguments: object) -> BaseModel:
    adapter = _TOOL_INPUT_ADAPTERS.get(name)
    if adapter is None or type(arguments) is not dict:
        raise InvalidModelOutput
    try:
        value = adapter.validate_python(arguments)
    except ValidationError:
        raise InvalidModelOutput from None
    if not isinstance(value, BaseModel):
        raise InvalidModelOutput
    return value


def validate_tool_output(name: str, value: object, arguments: BaseModel) -> BaseModel:
    adapter = _TOOL_OUTPUT_ADAPTERS.get(name)
    if adapter is None:
        raise PlannerUnavailable
    context: dict[str, object] | None = None
    if name == "query_terrain":
        if not isinstance(arguments, QueryTerrainInput):
            raise PlannerUnavailable
        context = {"positions": arguments.model_dump(mode="json")["positions"]}
    payload = value.model_dump(mode="json") if isinstance(value, BaseModel) else value
    try:
        output = adapter.validate_python(payload, context=context)
    except ValidationError:
        raise PlannerUnavailable from None
    if not isinstance(output, BaseModel):
        raise PlannerUnavailable
    return output


__all__ = [
    "InvalidModelOutput",
    "MAX_PLAN_BYTES",
    "MODEL_VISIBLE_TOOLS",
    "ModelOutput",
    "ModelToolCall",
    "PlannerDeadlineExceeded",
    "PlannerFailure",
    "PlannerLimits",
    "PlannerMessage",
    "PlannerModel",
    "PlannerUnavailable",
    "PlanningToolSession",
    "PlanningToolSessionFactory",
    "parse_plan_output",
    "strict_json_object",
    "validate_tool_input",
    "validate_tool_output",
]

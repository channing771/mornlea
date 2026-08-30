"""HTTP 与 MCP v1 共用的严格领域标量。"""

from __future__ import annotations

import json
import math
import re
import unicodedata
from typing import Annotated, Any
from urllib.parse import urlsplit

from pydantic import (
    AfterValidator,
    BaseModel,
    BeforeValidator,
    ConfigDict,
    Field,
    StrictFloat,
    StrictInt,
    StrictStr,
)

UINT64_MAX = (1 << 64) - 1
INT32_MIN = -(1 << 31)
INT32_MAX = (1 << 31) - 1
WORLD_MIN_Y = -64
WORLD_MAX_Y = 319

_UUID_V4 = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
_SHA256 = re.compile(r"^[0-9a-f]{64}$")


class StrictModel(BaseModel):
    """所有 application contract object 的共同 strict/frozen 规则。"""

    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)


def _utf8(value: str) -> bytes:
    try:
        return value.encode("utf-8", errors="strict")
    except UnicodeEncodeError as error:
        raise ValueError("text must be valid UTF-8") from error


def _has_control(value: str) -> bool:
    return any(unicodedata.category(character) == "Cc" for character in value)


def text_validator(
    *,
    minimum_bytes: int = 0,
    maximum_bytes: int,
    no_nul: bool = False,
    no_control: bool = False,
    no_edge_whitespace: bool = False,
    non_blank: bool = False,
) -> Any:
    def validate(value: str) -> str:
        encoded = _utf8(value)
        if len(encoded) < minimum_bytes or len(encoded) > maximum_bytes:
            raise ValueError(f"text must contain {minimum_bytes}..{maximum_bytes} UTF-8 bytes")
        if no_nul and "\x00" in value:
            raise ValueError("text must not contain NUL")
        if no_control and _has_control(value):
            raise ValueError("text must not contain Unicode control characters")
        if no_edge_whitespace and value.strip() != value:
            raise ValueError("text must not contain edge Unicode whitespace")
        if non_blank and value.strip() == "":
            raise ValueError("text must not be blank")
        return value

    return validate


def _canonical_uuid_v4(value: str) -> str:
    if _UUID_V4.fullmatch(value) is None:
        raise ValueError("identity must be a canonical lowercase UUIDv4")
    return value


def _canonical_sha256(value: str) -> str:
    if _SHA256.fullmatch(value) is None:
        raise ValueError("digest must be canonical lowercase SHA-256 hex")
    return value


UUIDv4 = Annotated[StrictStr, AfterValidator(_canonical_uuid_v4)]
SHA256 = Annotated[StrictStr, AfterValidator(_canonical_sha256)]
UInt64 = Annotated[StrictInt, Field(ge=0, le=UINT64_MAX)]
PositiveUInt64 = Annotated[StrictInt, Field(ge=1, le=UINT64_MAX)]
Int32 = Annotated[StrictInt, Field(ge=INT32_MIN, le=INT32_MAX)]
WorldY = Annotated[StrictInt, Field(ge=WORLD_MIN_Y, le=WORLD_MAX_Y)]
TerrainHeight = Annotated[StrictInt, Field(ge=WORLD_MIN_Y - 1, le=WORLD_MAX_Y)]
BlockID = Annotated[StrictInt, Field(ge=1, le=65534)]

InstructionText = Annotated[
    StrictStr,
    AfterValidator(
        text_validator(
            minimum_bytes=1,
            maximum_bytes=1024,
            no_control=True,
            non_blank=True,
        )
    ),
]
PersonaText = Annotated[
    StrictStr,
    AfterValidator(text_validator(maximum_bytes=4096, no_nul=True)),
]
DialogueLine = Annotated[
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
MemorySummary = Annotated[
    StrictStr,
    AfterValidator(text_validator(maximum_bytes=2048, no_nul=True)),
]
MCPCapability = Annotated[
    StrictStr,
    AfterValidator(text_validator(minimum_bytes=1, maximum_bytes=512, no_control=True)),
]
PlanSummary = Annotated[
    StrictStr,
    AfterValidator(
        text_validator(
            minimum_bytes=1,
            maximum_bytes=512,
            no_control=True,
            non_blank=True,
        )
    ),
]
TaskStatusText = Annotated[
    StrictStr,
    AfterValidator(text_validator(maximum_bytes=96, no_control=True)),
]
BoundedName = Annotated[
    StrictStr,
    AfterValidator(
        text_validator(
            minimum_bytes=1,
            maximum_bytes=64,
            no_control=True,
            non_blank=True,
        )
    ),
]
ValidatorHint = Annotated[
    StrictStr,
    AfterValidator(text_validator(maximum_bytes=256)),
]


def _validate_finite_number(value: object) -> int | float:
    if type(value) is int:
        return value
    if type(value) is not float:
        raise ValueError("position component must be a JSON number")
    if not math.isfinite(value):
        raise ValueError("position component must be finite")
    return value


FiniteNumber = Annotated[
    StrictInt | StrictFloat,
    BeforeValidator(_validate_finite_number),
]


def json_tuple(value: object) -> tuple[Any, ...]:
    if isinstance(value, tuple):
        return value
    if isinstance(value, list):
        return tuple(value)
    raise ValueError("value must be a JSON array")


Position = Annotated[
    tuple[FiniteNumber, FiniteNumber, FiniteNumber],
    BeforeValidator(json_tuple),
]


class BlockPosition(StrictModel):
    x: Int32
    y: WorldY
    z: Int32


def _validate_mcp_endpoint(value: str) -> str:
    text_validator(minimum_bytes=1, maximum_bytes=256, no_control=True)(value)
    try:
        parsed = urlsplit(value)
        port = parsed.port
    except ValueError as error:
        raise ValueError("MCP endpoint must be a valid URL") from error
    if parsed.scheme != "http" or parsed.username is not None or parsed.password is not None:
        raise ValueError("MCP endpoint must be an unauthenticated http URL")
    if parsed.query or parsed.fragment or "?" in value or "#" in value or parsed.path != "/mcp":
        raise ValueError("MCP endpoint must use the exact /mcp path without query or fragment")
    if parsed.hostname is None or port is None or not 1 <= port <= 65535:
        raise ValueError("MCP endpoint must include a valid loopback listener port")
    from ipaddress import ip_address

    try:
        address = ip_address(parsed.hostname)
    except ValueError as error:
        raise ValueError("MCP endpoint host must be a loopback IP literal") from error
    if not address.is_loopback:
        raise ValueError("MCP endpoint host must be a loopback IP literal")
    return value


MCPEndpoint = Annotated[StrictStr, AfterValidator(_validate_mcp_endpoint)]


def canonical_json_bytes(value: BaseModel | object) -> bytes:
    payload = value.model_dump(mode="json") if isinstance(value, BaseModel) else value
    return json.dumps(
        payload,
        ensure_ascii=False,
        allow_nan=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")


def require_canonical_size(value: BaseModel, maximum: int, label: str) -> None:
    if len(canonical_json_bytes(value)) > maximum:
        raise ValueError(f"{label} exceeds {maximum} canonical UTF-8 bytes")


__all__ = [
    "BlockID",
    "BlockPosition",
    "BoundedName",
    "DialogueLine",
    "FiniteNumber",
    "InstructionText",
    "Int32",
    "MCPCapability",
    "MCPEndpoint",
    "MemorySummary",
    "PersonaText",
    "PlanSummary",
    "Position",
    "PositiveUInt64",
    "SHA256",
    "StrictModel",
    "TaskStatusText",
    "TerrainHeight",
    "UUIDv4",
    "UInt64",
    "ValidatorHint",
    "WorldY",
    "canonical_json_bytes",
    "json_tuple",
    "require_canonical_size",
]

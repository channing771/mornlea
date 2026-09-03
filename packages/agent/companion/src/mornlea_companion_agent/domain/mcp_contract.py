"""读取随 wheel 分发的 MCP v1 manifest 与精确 JSON Schema。"""

from __future__ import annotations

import json
from copy import deepcopy
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path
from typing import Any

_RESOURCE_ROOT = Path(__file__).resolve().parents[1] / "_contracts" / "mcp-v1"
_SOURCE_ROOT = Path(__file__).resolve().parents[6] / "contracts" / "companion-agent" / "mcp-v1"


@dataclass(frozen=True, slots=True)
class MCPToolContract:
    """单个 MCP 工具在 wire 上必须广告的不可漂移定义。"""

    name: str
    model_visible: bool
    input_schema: dict[str, object]
    output_schema: dict[str, object]


def _read_document(name: str) -> dict[str, Any]:
    resource = _RESOURCE_ROOT / name
    if resource.is_file():
        raw = resource.read_text(encoding="utf-8")
    else:
        source = _SOURCE_ROOT / name
        if not source.is_file():
            raise RuntimeError("bundled MCP contract is missing")
        raw = source.read_text(encoding="utf-8")
    try:
        value = json.loads(raw)
    except (UnicodeError, json.JSONDecodeError):
        raise RuntimeError("bundled MCP contract is invalid") from None
    if type(value) is not dict:
        raise RuntimeError("bundled MCP contract is invalid")
    return value


def _resolve_schema(
    value: object,
    definitions: dict[str, object],
    stack: tuple[str, ...],
) -> object:
    if type(value) is dict:
        reference = value.get("$ref")
        if set(value) == {"$ref"} and type(reference) is str:
            prefix = "#/$defs/"
            if not reference.startswith(prefix):
                raise RuntimeError("bundled MCP contract contains an external reference")
            target = reference.removeprefix(prefix)
            if target in stack or target not in definitions:
                raise RuntimeError("bundled MCP contract contains an invalid reference")
            return _resolve_schema(definitions[target], definitions, (*stack, target))
        return {
            key: _resolve_schema(item, definitions, stack)
            for key, item in value.items()
            if type(key) is str
        }
    if type(value) is list:
        return [_resolve_schema(item, definitions, stack) for item in value]
    if value is None or type(value) in (str, int, float, bool):
        return value
    raise RuntimeError("bundled MCP contract contains an invalid JSON value")


@lru_cache(maxsize=1)
def _load_contracts() -> tuple[MCPToolContract, ...]:
    manifest = _read_document("manifest.json")
    schema = _read_document("schema.json")
    definitions = schema.get("$defs")
    tools = manifest.get("tools")
    limits = manifest.get("limits")
    if (
        manifest.get("application_contract_version") != "v1"
        or manifest.get("mcp_protocol_version") != "2025-11-25"
        or type(definitions) is not dict
        or type(tools) is not list
        or type(limits) is not dict
        or limits.get("wire_response_bytes") != 163_840
        or limits.get("plan_input_bytes") != 65_536
    ):
        raise RuntimeError("bundled MCP contract metadata is invalid")

    result: list[MCPToolContract] = []
    for item in tools:
        if type(item) is not dict:
            raise RuntimeError("bundled MCP tool manifest is invalid")
        name = item.get("name")
        model_visible = item.get("model_visible")
        input_name = item.get("input_schema")
        output_name = item.get("result_schema")
        if (
            type(name) is not str
            or type(model_visible) is not bool
            or type(input_name) is not str
            or type(output_name) is not str
            or input_name not in definitions
            or output_name not in definitions
        ):
            raise RuntimeError("bundled MCP tool manifest is invalid")
        input_schema = _resolve_schema(definitions[input_name], definitions, (input_name,))
        output_schema = _resolve_schema(definitions[output_name], definitions, (output_name,))
        if type(input_schema) is not dict or type(output_schema) is not dict:
            raise RuntimeError("bundled MCP tool schema is invalid")
        result.append(
            MCPToolContract(
                name=name,
                model_visible=model_visible,
                input_schema=input_schema,
                output_schema=output_schema,
            )
        )
    if len(result) != 6 or len({tool.name for tool in result}) != len(result):
        raise RuntimeError("bundled MCP tool manifest is invalid")
    return tuple(result)


def mcp_tool_contracts() -> tuple[MCPToolContract, ...]:
    """返回独立副本，避免 SDK 或 provider 修改进程级契约。"""

    return tuple(
        MCPToolContract(
            name=tool.name,
            model_visible=tool.model_visible,
            input_schema=deepcopy(tool.input_schema),
            output_schema=deepcopy(tool.output_schema),
        )
        for tool in _load_contracts()
    )


__all__ = ["MCPToolContract", "mcp_tool_contracts"]

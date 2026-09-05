"""共享 application contract 的 Pydantic TypeAdapter registry。"""

from __future__ import annotations

from typing import Any

from pydantic import TypeAdapter

from harness.domain.http_v1 import HTTP_V1_TYPES
from harness.domain.mcp_v1 import MCP_V1_TYPES


def _build_registry(types: dict[str, Any]) -> dict[str, TypeAdapter[Any]]:
    return {name: TypeAdapter(value) for name, value in types.items()}


CONTRACT_ADAPTERS: dict[str, dict[str, TypeAdapter[Any]]] = {
    "http-v1": _build_registry(HTTP_V1_TYPES),
    "mcp-v1": _build_registry(MCP_V1_TYPES),
}


def adapter_for(contract: str, schema: str) -> TypeAdapter[Any]:
    """返回 checked-in contract/schema 对应的唯一运行时 adapter。"""

    try:
        return CONTRACT_ADAPTERS[contract][schema]
    except KeyError as error:
        raise KeyError(f"unknown contract schema {contract}/{schema}") from error


__all__ = ["CONTRACT_ADAPTERS", "adapter_for"]

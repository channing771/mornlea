"""真实 Go/Python 合同测试使用的受限子进程入口。"""

from __future__ import annotations

import asyncio
import json
import sys
from typing import Any

from pydantic import BaseModel, ConfigDict

from mornlea_companion_agent.adapters.mcp import MCP_PROTOCOL_VERSION, MCPToolSessionFactory
from mornlea_companion_agent.domain.common import canonical_json_bytes
from mornlea_companion_agent.domain.http_v1 import PlanRequest
from mornlea_companion_agent.domain.mcp_contract import mcp_tool_contracts
from mornlea_companion_agent.domain.mcp_v1 import Plan, QueryTerrainSuccessResult


class _ProbeInput(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    request: PlanRequest


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    value: dict[str, object] = {}
    for key, item in pairs:
        if key in value:
            raise ValueError("duplicate JSON key")
        value[key] = item
    return value


def _read_input() -> _ProbeInput:
    raw = sys.stdin.buffer.read(256 * 1024 + 1)
    if not raw or len(raw) > 256 * 1024:
        raise ValueError("invalid probe input")
    decoded = json.loads(raw.decode("utf-8", errors="strict"), object_pairs_hook=_unique_object)
    return _ProbeInput.model_validate(decoded)


async def _mcp_probe(value: _ProbeInput) -> dict[str, Any]:
    calls: list[str] = []
    factory = MCPToolSessionFactory()
    phase = "open"
    try:
        async with factory.open(value.request, timeout_seconds=10.0) as tools:
            phase = "get_planning_context"
            context = await tools.get_planning_context()
            calls.append("get_planning_context")
            if (
                context.snapshot_digest != value.request.snapshot_digest
                or context.instruction != value.request.instruction
                or context.companion.companion_id != value.request.companion_id
            ):
                raise ValueError("planning context correlation mismatch")

            phase = "query_terrain"
            terrain = await tools.call_model_tool(
                "query_terrain",
                {"positions": [{"x": 8, "y": 63, "z": -2}]},
            )
            calls.append("query_terrain")
            if not isinstance(terrain, QueryTerrainSuccessResult):
                raise ValueError("query terrain did not return success")

            plan = Plan.model_validate(
                {
                    "summary": "采集石头",
                    "steps": [{"kind": "mine", "x": 8, "y": 63, "z": -2}],
                }
            )
            phase = "validate_plan"
            validated = await tools.validate_plan(plan)
            calls.append("validate_plan")
            if not getattr(validated, "accepted", False):
                raise ValueError("plan validator rejected deterministic plan")
    except BaseException as error:
        raise RuntimeError(phase) from error

    return {
        "protocol_version": MCP_PROTOCOL_VERSION,
        "implementation_version": "v1",
        "tools": [tool.name for tool in mcp_tool_contracts()],
        "calls": calls,
    }


def main() -> int:
    if sys.argv != [sys.argv[0], "mcp-probe"]:
        return 2
    try:
        result = asyncio.run(_mcp_probe(_read_input()))
        sys.stdout.buffer.write(canonical_json_bytes(result) + b"\n")
        return 0
    except BaseException as error:
        sys.stdout.buffer.write(
            canonical_json_bytes({"error_phase": str(error), "error_type": type(error).__name__})
            + b"\n"
        )
        sys.stderr.write("cross-language MCP probe failed\n")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

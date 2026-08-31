"""真实 Go/Python 合同测试使用的受限子进程入口。"""

from __future__ import annotations

import asyncio
import json
import socket
import sys
from pathlib import Path
from typing import Any

import uvicorn
from pydantic import BaseModel, ConfigDict, SecretStr

from mornlea_companion_agent.adapters.mcp import MCP_PROTOCOL_VERSION, MCPToolSessionFactory
from mornlea_companion_agent.app import AppComponents, create_app
from mornlea_companion_agent.config import AgentConfig, ResolvedSecrets
from mornlea_companion_agent.domain.common import canonical_json_bytes
from mornlea_companion_agent.domain.dialogue import DialogueMessage
from mornlea_companion_agent.domain.http_v1 import PlanRequest
from mornlea_companion_agent.domain.mcp_contract import mcp_tool_contracts
from mornlea_companion_agent.domain.mcp_v1 import Plan, QueryTerrainSuccessResult
from mornlea_companion_agent.domain.planner import ModelOutput, PlannerMessage, PlannerUnavailable
from mornlea_companion_agent.harness.dialogue import DialogueHarness
from mornlea_companion_agent.harness.planner import PlannerHarness
from mornlea_companion_agent.storage.sqlite_memory import SQLiteMemoryStore


class _ProbeInput(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    request: PlanRequest


class _HTTPServerInput(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    block_ready_path: str
    http_bearer_token: str
    port: int
    sqlite_path: str


class _DeterministicPlannerModel:
    def __init__(self, block_ready_path: Path) -> None:
        self._block_ready_path = block_ready_path

    async def complete(
        self,
        messages: tuple[PlannerMessage, ...],
        *,
        allow_tools: bool,
    ) -> ModelOutput:
        del allow_tools
        if any("BLOCK_UNTIL_CANCEL" in message.content for message in messages):
            self._block_ready_path.write_text("ready", encoding="ascii")
            await asyncio.Future()
        return ModelOutput(
            content='{"summary":"采集石头","steps":[{"kind":"mine","x":8,"y":63,"z":-2}]}',
            tool_calls=(),
        )


class _DeterministicDialogueModel:
    async def complete(self, messages: tuple[DialogueMessage, ...]) -> object:
        prompt = json.loads(messages[-1].content)
        if prompt["fact_node"]["kind"] == "terminal":
            return '{"line":"石料已经送到。","summary":"为玩家采集并交付了石料。"}'
        return '{"line":"我已经挖完这一块。"}'


class _FakeModelOwner:
    async def aclose(self) -> None:
        return None


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    value: dict[str, object] = {}
    for key, item in pairs:
        if key in value:
            raise ValueError("duplicate JSON key")
        value[key] = item
    return value


def _read_input(model: type[BaseModel]) -> BaseModel:
    raw = sys.stdin.buffer.read(256 * 1024 + 1)
    if not raw or len(raw) > 256 * 1024:
        raise ValueError("invalid probe input")
    decoded = json.loads(raw.decode("utf-8", errors="strict"), object_pairs_hook=_unique_object)
    return model.model_validate(decoded)


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


async def _mcp_cancel_probe(value: _ProbeInput) -> dict[str, str]:
    try:
        async with MCPToolSessionFactory().open(value.request, timeout_seconds=0.25) as tools:
            await tools.get_planning_context()
    except PlannerUnavailable:
        return {"status": "cancelled"}
    raise ValueError("cancel probe unexpectedly succeeded")


async def _http_server(value: _HTTPServerInput) -> None:
    sqlite_path = Path(value.sqlite_path)
    config = AgentConfig.model_validate(
        {
            "config_version": "v1",
            "http": {
                "bind": "127.0.0.1",
                "port": value.port,
                "workers": 1,
                "bearer_token_env": "INTEGRATION_HTTP_TOKEN",
            },
            "storage": {"sqlite_path": str(sqlite_path)},
            "provider": {
                "base_url": "http://127.0.0.1:1/v1",
                "model": "deterministic-integration-model",
                "api_key_env": "INTEGRATION_PROVIDER_KEY",
            },
            "limits": {"model_calls": 3, "tool_calls": 4, "timeout_seconds": 30},
        },
        context={"config_dir": sqlite_path.parent},
    )
    secrets = ResolvedSecrets(
        http_bearer_token=SecretStr(value.http_bearer_token),
        provider_api_key=SecretStr("unused-integration-provider-key"),
    )

    async def component_factory(
        current: AgentConfig,
        provider_api_key: SecretStr,
    ) -> AppComponents:
        del provider_api_key
        store = await SQLiteMemoryStore.open(current.storage.sqlite_path)
        return AppComponents(
            store=store,
            planner=PlannerHarness(
                _DeterministicPlannerModel(Path(value.block_ready_path)),
                MCPToolSessionFactory(),
            ),
            dialogue=DialogueHarness(_DeterministicDialogueModel(), store),
            model_owner=_FakeModelOwner(),
        )

    app = create_app(config, secrets, component_factory=component_factory)
    inherited = socket.socket(fileno=3)
    server = uvicorn.Server(
        uvicorn.Config(
            app,
            host="127.0.0.1",
            port=value.port,
            workers=1,
            http="h11",
            access_log=False,
            proxy_headers=False,
            server_header=False,
            log_level="critical",
        )
    )
    task = asyncio.create_task(server.serve(sockets=[inherited]))
    while not server.started and not task.done():
        await asyncio.sleep(0.005)
    if task.done():
        await task
        raise RuntimeError("HTTP server exited before readiness")
    sys.stdout.buffer.write(b'{"status":"ready"}\n')
    sys.stdout.buffer.flush()
    await task


def main() -> int:
    if len(sys.argv) != 2:
        return 2
    try:
        if sys.argv[1] == "mcp-probe":
            value = _read_input(_ProbeInput)
            if not isinstance(value, _ProbeInput):
                raise TypeError("invalid probe input")
            result = asyncio.run(_mcp_probe(value))
            sys.stdout.buffer.write(canonical_json_bytes(result) + b"\n")
        elif sys.argv[1] == "mcp-cancel-probe":
            value = _read_input(_ProbeInput)
            if not isinstance(value, _ProbeInput):
                raise TypeError("invalid probe input")
            result = asyncio.run(_mcp_cancel_probe(value))
            sys.stdout.buffer.write(canonical_json_bytes(result) + b"\n")
        elif sys.argv[1] == "http-server":
            value = _read_input(_HTTPServerInput)
            if not isinstance(value, _HTTPServerInput):
                raise TypeError("invalid HTTP server input")
            asyncio.run(_http_server(value))
        else:
            return 2
        return 0
    except BaseException as error:
        safe_phase = "process"
        if isinstance(error, RuntimeError) and str(error) in {
            "open",
            "get_planning_context",
            "query_terrain",
            "validate_plan",
        }:
            safe_phase = str(error)
        sys.stdout.buffer.write(
            canonical_json_bytes({"error_phase": safe_phase, "error_type": type(error).__name__})
            + b"\n"
        )
        sys.stderr.write("cross-language process failed\n")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())

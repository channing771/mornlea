from __future__ import annotations

import asyncio
import json
from collections.abc import Awaitable
from copy import deepcopy
from pathlib import Path
from typing import Any

import httpx
import pytest

from mornlea_companion_agent.adapters import mcp as mcp_adapter
from mornlea_companion_agent.adapters.mcp import MCPToolSessionFactory
from mornlea_companion_agent.domain.http_v1 import PlanRequest
from mornlea_companion_agent.domain.mcp_v1 import (
    ListAffordancesResult,
    Plan,
    QueryTerrainResult,
)
from mornlea_companion_agent.domain.planner import PlannerUnavailable

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
CONTRACT_ROOT = REPOSITORY_ROOT / "contracts/companion-agent"
PROTOCOL_VERSION = "2025-11-25"
TOOL_NAMES = (
    "get_planning_context",
    "list_affordances",
    "inspect_inventory",
    "find_visible_blocks",
    "query_terrain",
    "validate_plan",
)


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def golden(contract: str, kind: str, name: str) -> dict[str, Any]:
    document = json.loads(
        (CONTRACT_ROOT / contract / "golden" / f"{kind}.json").read_text(encoding="utf-8")
    )
    return next(deepcopy(case) for case in document["cases"] if case["name"] == name)


def request() -> PlanRequest:
    return PlanRequest.model_validate(
        golden("http-v1", "valid", "planner run carries snapshot identity")["value"]
    )


def result_for_tool(name: str, arguments: dict[str, object]) -> dict[str, object]:
    if name == "get_planning_context":
        value = golden("mcp-v1", "valid", "planning context is a bounded projection")["value"]
        current = request()
        value["instruction"] = current.instruction
        value["companion"]["companion_id"] = current.companion_id
        return value
    if name == "list_affordances":
        return golden(
            "mcp-v1", "valid", "affordances include fixed steps and mine classifications"
        )["value"]
    if name == "inspect_inventory":
        return golden("mcp-v1", "valid", "inventory result contains bounded occupied slots")[
            "value"
        ]
    if name == "find_visible_blocks":
        return golden("mcp-v1", "valid", "visible block result is coordinate ordered")["value"]
    if name == "query_terrain":
        value = golden("mcp-v1", "valid", "terrain result preserves input order")["value"]
        assert arguments["positions"] == [item["position"] for item in value["terrain"]]
        return value
    if name == "validate_plan":
        value = golden(
            "mcp-v1",
            "valid",
            "validator accepted result echoes digest and canonical plan",
        )["value"]
        value["plan"] = arguments["plan"]
        return value
    raise AssertionError(name)


class MCPMock:
    def __init__(self, *, text_only: set[str] | None = None) -> None:
        self.text_only = text_only or set()
        self.requests: list[tuple[str, dict[str, str], dict[str, object]]] = []

    async def __call__(self, raw: httpx.Request) -> httpx.Response:
        body = json.loads(raw.content)
        headers = {key.lower(): value for key, value in raw.headers.items()}
        self.requests.append((raw.method, headers, body))
        method = body["method"]
        if method == "initialize":
            result: dict[str, object] = {
                "protocolVersion": PROTOCOL_VERSION,
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "mornlea-mcp", "version": "v1"},
            }
        elif method == "notifications/initialized":
            return httpx.Response(202, request=raw)
        elif method == "tools/list":
            result = {
                "tools": [
                    {
                        "name": name,
                        "description": f"Mornlea {name}",
                        "inputSchema": {"type": "object", "additionalProperties": False},
                        "outputSchema": {"type": "object"},
                    }
                    for name in TOOL_NAMES
                ]
            }
        elif method == "tools/call":
            params = body["params"]
            name = params["name"]
            result_value = result_for_tool(name, params.get("arguments", {}))
            result = {
                "content": [
                    {
                        "type": "text",
                        "text": json.dumps(result_value, ensure_ascii=False, separators=(",", ":")),
                    }
                ],
                "isError": False,
            }
            if name not in self.text_only:
                result["structuredContent"] = result_value
        else:
            raise AssertionError(f"unexpected MCP method {method}")
        return httpx.Response(
            200,
            headers={"content-type": "application/json"},
            json={"jsonrpc": "2.0", "id": body["id"], "result": result},
            request=raw,
        )


def test_real_sdk_session_uses_exact_wire_sequence_headers_and_one_session() -> None:
    async def scenario() -> None:
        mock = MCPMock(text_only={"query_terrain"})
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        current = request()
        query = golden("mcp-v1", "valid", "terrain query preserves input order")["value"]
        candidate = Plan.model_validate(
            golden(
                "mcp-v1",
                "valid",
                "validator accepted result echoes digest and canonical plan",
            )["value"]["plan"]
        )

        async with factory.open(current, timeout_seconds=5) as session:
            context = await session.get_planning_context()
            affordances = await session.call_model_tool("list_affordances", {})
            terrain = await session.call_model_tool("query_terrain", query)
            validated = await session.validate_plan(candidate)

        assert context.snapshot_digest == current.snapshot_digest
        assert isinstance(affordances, ListAffordancesResult)
        assert affordances.step_kinds == ("go_to", "follow", "mine", "place")
        assert isinstance(terrain, QueryTerrainResult)
        assert validated.accepted is True

        methods = [body["method"] for _, _, body in mock.requests]
        assert methods == [
            "initialize",
            "notifications/initialized",
            "tools/list",
            "tools/call",
            "tools/call",
            "tools/call",
            "tools/call",
        ]
        assert all(method == "POST" for method, _, _ in mock.requests)
        assert not ({"ping", "subscriptions/listen"} & set(methods))
        for index, (_, headers, _) in enumerate(mock.requests):
            assert headers["authorization"] == f"Bearer {current.mcp_capability}"
            assert headers["content-type"] == "application/json"
            assert "text/event-stream" in headers["accept"]
            if index == 0:
                assert "mcp-protocol-version" not in headers
            else:
                assert headers["mcp-protocol-version"] == PROTOCOL_VERSION

    run(scenario())


@pytest.mark.parametrize(
    "capabilities",
    [
        {"tools": {"listChanged": True}},
        {"tools": {"listChanged": False}, "logging": {}},
        {},
    ],
)
def test_server_must_advertise_only_stable_tools_capability(
    capabilities: dict[str, object],
) -> None:
    class CapabilityMock(MCPMock):
        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] == "initialize":
                payload = json.loads(response.content)
                payload["result"]["capabilities"] = capabilities
                return httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
            return response

    async def scenario() -> None:
        mock = CapabilityMock()
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        with pytest.raises(PlannerUnavailable):
            async with factory.open(request(), timeout_seconds=5):
                pass

    run(scenario())


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("protocolVersion", "2025-06-18"),
        ("serverInfo.version", "v2"),
    ],
)
def test_initialize_pins_wire_and_application_versions(field: str, value: str) -> None:
    class InitializeMock(MCPMock):
        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] == "initialize":
                payload = json.loads(response.content)
                if field == "protocolVersion":
                    payload["result"]["protocolVersion"] = value
                else:
                    payload["result"]["serverInfo"]["version"] = value
                return httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
            return response

    async def scenario() -> None:
        mock = InitializeMock()
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=5
            ):
                pass

    run(scenario())


@pytest.mark.parametrize("mutation", ["duplicate", "missing", "pagination", "reordered"])
def test_tool_discovery_requires_exact_unique_six_without_pagination(mutation: str) -> None:
    class ToolListMock(MCPMock):
        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] == "tools/list":
                payload = json.loads(response.content)
                tools = payload["result"]["tools"]
                if mutation == "duplicate":
                    tools[-1]["name"] = tools[0]["name"]
                elif mutation == "missing":
                    tools.pop()
                elif mutation == "pagination":
                    payload["result"]["nextCursor"] = "more"
                else:
                    tools.reverse()
                return httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
            return response

    async def scenario() -> None:
        mock = ToolListMock()
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=5
            ):
                pass

    run(scenario())


def test_sdk_latest_protocol_drift_fails_before_network(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    mock = MCPMock()

    async def scenario() -> None:
        monkeypatch.setattr(mcp_adapter.types, "LATEST_PROTOCOL_VERSION", "2026-07-28")
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=5
            ):
                pass
        assert mock.requests == []

    run(scenario())


def test_session_header_and_sse_response_are_rejected() -> None:
    class ForbiddenResponseMock(MCPMock):
        def __init__(self, kind: str) -> None:
            super().__init__()
            self.kind = kind

        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] != "initialize":
                return response
            if self.kind == "session":
                return httpx.Response(
                    200,
                    headers={
                        "content-type": "application/json",
                        "mcp-session-id": "forbidden-session",
                    },
                    content=response.content,
                    request=raw,
                )
            return httpx.Response(
                200,
                headers={"content-type": "text/event-stream"},
                content=b"event: message\ndata: {}\n\n",
                request=raw,
            )

    async def scenario(kind: str) -> None:
        mock = ForbiddenResponseMock(kind)
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=1
            ):
                pass

    for kind in ("session", "sse"):
        run(scenario(kind))


def test_redirect_is_not_followed() -> None:
    requests: list[httpx.Request] = []

    async def redirect(raw: httpx.Request) -> httpx.Response:
        requests.append(raw)
        return httpx.Response(
            307,
            headers={"location": "http://127.0.0.1:9/steal"},
            request=raw,
        )

    async def scenario() -> None:
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(redirect)).open(
                request(), timeout_seconds=1
            ):
                pass
        assert len(requests) == 1
        assert requests[0].url == request().mcp_endpoint

    run(scenario())


def test_http_client_disables_environment_and_redirects(monkeypatch: pytest.MonkeyPatch) -> None:
    captured: dict[str, object] = {}
    mock = MCPMock()
    original = httpx.AsyncClient

    def recording_client(**kwargs: object) -> httpx.AsyncClient:
        captured.update(kwargs)
        kwargs["transport"] = httpx.MockTransport(mock)
        return original(**kwargs)

    async def scenario() -> None:
        monkeypatch.setattr(mcp_adapter.httpx, "AsyncClient", recording_client)
        async with MCPToolSessionFactory().open(request(), timeout_seconds=5):
            pass
        assert captured["trust_env"] is False
        assert captured["follow_redirects"] is False

    run(scenario())


def test_text_fallback_is_strict_json_and_never_retries_tool_call() -> None:
    class InvalidTextMock(MCPMock):
        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] == "tools/call":
                payload = json.loads(response.content)
                payload["result"].pop("structuredContent", None)
                payload["result"]["content"] = [
                    {"type": "text", "text": '{"slots":[],"slots":[]}'},
                ]
                return httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
            return response

    async def scenario() -> None:
        mock = InvalidTextMock(text_only={"inspect_inventory"})
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        async with factory.open(request(), timeout_seconds=5) as session:
            with pytest.raises(PlannerUnavailable):
                await session.call_model_tool(
                    "inspect_inventory",
                    golden("mcp-v1", "valid", "inventory query is paged")["value"],
                )
        assert [body["method"] for _, _, body in mock.requests].count("tools/call") == 1

    run(scenario())


@pytest.mark.parametrize("mutation", ["mismatch", "multiple", "is_error"])
def test_tool_result_shape_fails_closed_without_retry(mutation: str) -> None:
    class ResultMutationMock(MCPMock):
        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] != "tools/call":
                return response
            payload = json.loads(response.content)
            result = payload["result"]
            if mutation == "mismatch":
                result["structuredContent"] = {"slots": []}
            elif mutation == "multiple":
                result["content"].append({"type": "text", "text": "{}"})
            else:
                result["isError"] = True
            return httpx.Response(
                200,
                headers={"content-type": "application/json"},
                json=payload,
                request=raw,
            )

    async def scenario() -> None:
        mock = ResultMutationMock()
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        async with factory.open(request(), timeout_seconds=5) as session:
            with pytest.raises(PlannerUnavailable):
                await session.call_model_tool(
                    "inspect_inventory",
                    golden("mcp-v1", "valid", "inventory query is paged")["value"],
                )
        assert [body["method"] for _, _, body in mock.requests].count("tools/call") == 1

    run(scenario())


def test_invalid_tool_golden_is_rejected_locally_before_wire_call() -> None:
    async def scenario() -> None:
        mock = MCPMock()
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        invalid = golden("mcp-v1", "invalid", "inventory limit is bounded")["value"]
        async with factory.open(request(), timeout_seconds=5) as session:
            with pytest.raises(PlannerUnavailable):
                await session.call_model_tool("inspect_inventory", invalid)
        assert [body["method"] for _, _, body in mock.requests].count("tools/call") == 0

    run(scenario())

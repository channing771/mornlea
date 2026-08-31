from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncIterator, Awaitable
from copy import deepcopy
from pathlib import Path
from typing import Any

import httpx
import pytest

from mornlea_companion_agent.adapters import mcp as mcp_adapter
from mornlea_companion_agent.adapters.mcp import (
    MCP_RESPONSE_BODY_LIMIT,
    MCPToolSessionFactory,
)
from mornlea_companion_agent.domain.http_v1 import PlanRequest
from mornlea_companion_agent.domain.mcp_v1 import (
    FindVisibleBlocksFailureResult,
    ListAffordancesResult,
    Plan,
    QueryTerrainFailureResult,
    QueryTerrainResult,
)
from mornlea_companion_agent.domain.planner import PlannerUnavailable

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
CONTRACT_ROOT = REPOSITORY_ROOT / "contracts/companion-agent"
PROTOCOL_VERSION = "2025-11-25"
MCP_WIRE_LIMIT = MCP_RESPONSE_BODY_LIMIT
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


def contract_document(name: str) -> dict[str, Any]:
    return json.loads((CONTRACT_ROOT / "mcp-v1" / name).read_text(encoding="utf-8"))


def resolve_contract_schema(name: str) -> dict[str, object]:
    definitions = contract_document("schema.json")["$defs"]

    def resolve(value: object, stack: tuple[str, ...] = ()) -> object:
        if isinstance(value, dict):
            reference = value.get("$ref")
            if set(value) == {"$ref"} and isinstance(reference, str):
                prefix = "#/$defs/"
                assert reference.startswith(prefix)
                target = reference.removeprefix(prefix)
                assert target not in stack
                return resolve(definitions[target], (*stack, target))
            return {key: resolve(item, stack) for key, item in value.items()}
        if isinstance(value, list):
            return [resolve(item, stack) for item in value]
        return value

    resolved = resolve(definitions[name], (name,))
    assert isinstance(resolved, dict)
    return resolved


def contract_tools() -> list[dict[str, object]]:
    manifest = contract_document("manifest.json")
    return [
        {
            "name": tool["name"],
            "description": f"Mornlea {tool['name']}",
            "inputSchema": resolve_contract_schema(tool["input_schema"]),
            "outputSchema": resolve_contract_schema(tool["result_schema"]),
        }
        for tool in manifest["tools"]
    ]


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
            result = {"tools": contract_tools()}
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


class DomainFailureMock(MCPMock):
    def __init__(self, *, tool_name: str, case_name: str, is_error: bool = False) -> None:
        super().__init__()
        self.tool_name = tool_name
        self.case_name = case_name
        self.is_error = is_error
        self.mutated_responses = 0

    async def __call__(self, raw: httpx.Request) -> httpx.Response:
        response = await super().__call__(raw)
        body = json.loads(raw.content)
        if body["method"] != "tools/call" or body["params"]["name"] != self.tool_name:
            return response
        payload = json.loads(response.content)
        value = golden("mcp-v1", "valid", self.case_name)["value"]
        result = payload["result"]
        result["content"] = [
            {
                "type": "text",
                "text": json.dumps(
                    value, ensure_ascii=False, sort_keys=True, separators=(",", ":")
                ),
            }
        ]
        result["structuredContent"] = value
        result["isError"] = self.is_error
        mutated_response = httpx.Response(
            200,
            headers={"content-type": "application/json"},
            json=payload,
            request=raw,
        )
        self.mutated_responses += 1
        return mutated_response


class TrackingStream(httpx.AsyncByteStream):
    def __init__(self, chunks: tuple[bytes, ...]) -> None:
        self.chunks = chunks
        self.bytes_yielded = 0
        self.closed = False

    async def __aiter__(self) -> AsyncIterator[bytes]:
        for chunk in self.chunks:
            self.bytes_yielded += len(chunk)
            yield chunk

    async def aclose(self) -> None:
        self.closed = True


class OversizedPhaseMock(MCPMock):
    def __init__(self, *, phase: str, mode: str) -> None:
        super().__init__()
        self.phase = phase
        self.mode = mode
        self.stream: TrackingStream | None = None
        self.mutated_responses = 0

    async def __call__(self, raw: httpx.Request) -> httpx.Response:
        response = await super().__call__(raw)
        body = json.loads(raw.content)
        if body["method"] != self.phase:
            return response
        if self.mode == "content_length":
            self.stream = TrackingStream((response.content,))
            mutated_response = httpx.Response(
                response.status_code,
                headers={
                    "content-type": response.headers.get("content-type", "application/json"),
                    "content-length": str(MCP_WIRE_LIMIT + 1),
                },
                stream=self.stream,
                request=raw,
            )
            self.mutated_responses += 1
            return mutated_response
        padding = b" " * (MCP_WIRE_LIMIT + 1 - len(response.content))
        self.stream = TrackingStream((response.content, padding))
        mutated_response = httpx.Response(
            response.status_code,
            headers={"content-type": response.headers.get("content-type", "application/json")},
            stream=self.stream,
            request=raw,
        )
        self.mutated_responses += 1
        return mutated_response


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
            assert headers["accept-encoding"] == "identity"
            assert headers["content-type"] == "application/json"
            assert "text/event-stream" in headers["accept"]
            if index == 0:
                assert "mcp-protocol-version" not in headers
            else:
                assert headers["mcp-protocol-version"] == PROTOCOL_VERSION

    run(scenario())


@pytest.mark.parametrize(
    ("tool_name", "case_name", "arguments", "result_type"),
    [
        (
            "find_visible_blocks",
            "unknown visible block is a normal domain failure",
            {"block_names": ["unknown_name"], "limit": 1},
            FindVisibleBlocksFailureResult,
        ),
        (
            "query_terrain",
            "terrain outside projection is a normal domain failure",
            {
                "positions": [
                    {"x": 4, "y": 64, "z": -1},
                    {"x": 8, "y": 64, "z": -2},
                ]
            },
            QueryTerrainFailureResult,
        ),
    ],
)
def test_is_error_false_domain_failure_is_returned_without_retry(
    tool_name: str,
    case_name: str,
    arguments: dict[str, object],
    result_type: type[object],
) -> None:
    async def scenario() -> None:
        mock = DomainFailureMock(tool_name=tool_name, case_name=case_name)
        async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
            request(), timeout_seconds=5
        ) as session:
            result = await session.call_model_tool(tool_name, arguments)
        assert isinstance(result, result_type)
        assert mock.mutated_responses == 1
        assert [body["method"] for _, _, body in mock.requests].count("tools/call") == 1

    run(scenario())


def test_is_error_true_domain_failure_remains_unavailable_without_retry() -> None:
    async def scenario() -> None:
        mock = DomainFailureMock(
            tool_name="find_visible_blocks",
            case_name="unknown visible block is a normal domain failure",
            is_error=True,
        )
        async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
            request(), timeout_seconds=5
        ) as session:
            with pytest.raises(PlannerUnavailable):
                await session.call_model_tool(
                    "find_visible_blocks",
                    {"block_names": ["unknown_name"], "limit": 1},
                )
        assert mock.mutated_responses == 1
        assert [body["method"] for _, _, body in mock.requests].count("tools/call") == 1

    run(scenario())


@pytest.mark.parametrize("phase", ["initialize", "tools/list", "tools/call"])
@pytest.mark.parametrize("mode", ["content_length", "chunked"])
def test_mcp_transport_rejects_oversized_response_before_sdk_buffers(
    phase: str,
    mode: str,
) -> None:
    async def scenario() -> None:
        mock = OversizedPhaseMock(phase=phase, mode=mode)
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        with pytest.raises(PlannerUnavailable):
            async with factory.open(request(), timeout_seconds=5) as session:
                if phase == "tools/call":
                    await session.call_model_tool(
                        "inspect_inventory",
                        golden("mcp-v1", "valid", "inventory query is paged")["value"],
                    )
        assert mock.mutated_responses == 1
        assert mock.stream is not None
        assert mock.stream.closed
        if mode == "content_length":
            assert mock.stream.bytes_yielded == 0
        else:
            assert mock.stream.bytes_yielded <= MCP_WIRE_LIMIT + 1

    run(scenario())


def test_mcp_transport_accepts_exact_wire_limit() -> None:
    class ExactLimitMock(MCPMock):
        def __init__(self) -> None:
            super().__init__()
            self.stream: TrackingStream | None = None

        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] != "initialize":
                return response
            padding = b" " * (MCP_WIRE_LIMIT - len(response.content))
            self.stream = TrackingStream((response.content, padding))
            return httpx.Response(
                200,
                headers={
                    "content-length": str(MCP_WIRE_LIMIT),
                    "content-type": "application/json",
                },
                stream=self.stream,
                request=raw,
            )

    async def scenario() -> None:
        mock = ExactLimitMock()
        async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
            request(), timeout_seconds=5
        ):
            pass
        assert mock.stream is not None
        assert mock.stream.bytes_yielded == MCP_WIRE_LIMIT
        assert mock.stream.closed

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
        def __init__(self) -> None:
            super().__init__()
            self.mutated_responses = 0

        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] == "initialize":
                payload = json.loads(response.content)
                payload["result"]["capabilities"] = capabilities
                mutated_response = httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
                self.mutated_responses += 1
                return mutated_response
            return response

    async def scenario() -> None:
        mock = CapabilityMock()
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        with pytest.raises(PlannerUnavailable):
            async with factory.open(request(), timeout_seconds=5):
                pass
        assert mock.mutated_responses == 1

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
        def __init__(self) -> None:
            super().__init__()
            self.mutated_responses = 0

        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] == "initialize":
                payload = json.loads(response.content)
                if field == "protocolVersion":
                    payload["result"]["protocolVersion"] = value
                else:
                    payload["result"]["serverInfo"]["version"] = value
                mutated_response = httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
                self.mutated_responses += 1
                return mutated_response
            return response

    async def scenario() -> None:
        mock = InitializeMock()
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=5
            ):
                pass
        assert mock.mutated_responses == 1

    run(scenario())


@pytest.mark.parametrize(
    "mutation",
    [
        "duplicate",
        "missing",
        "pagination",
        "reordered",
        "input_schema",
        "output_schema",
        "input_schema_bool_int",
        "input_schema_int_float",
    ],
)
def test_tool_discovery_requires_exact_unique_six_without_pagination(mutation: str) -> None:
    delivered_payloads: list[dict[str, Any]] = []

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
                elif mutation == "reordered":
                    tools.reverse()
                elif mutation == "input_schema":
                    tools[2]["inputSchema"]["properties"]["limit"]["maximum"] = 37
                elif mutation == "output_schema":
                    tools[4]["outputSchema"]["oneOf"][0]["properties"]["terrain"]["maxItems"] = 65
                elif mutation == "input_schema_bool_int":
                    tools[2]["inputSchema"]["properties"]["limit"]["minimum"] = True
                else:
                    tools[2]["inputSchema"]["properties"]["limit"]["minimum"] = 1.0
                mutated_response = httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
                delivered_payloads.append(payload)
                return mutated_response
            return response

    async def scenario() -> None:
        mock = ToolListMock()
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=5
            ):
                pass
        assert len(delivered_payloads) == 1
        assert [body["method"] for _, _, body in mock.requests].count("tools/list") == 1
        if mutation == "output_schema":
            assert (
                delivered_payloads[0]["result"]["tools"][4]["outputSchema"]["oneOf"][0][
                    "properties"
                ]["terrain"]["maxItems"]
                == 65
            )

    run(scenario())


def test_tool_schema_serialization_failure_fails_closed(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    serialization_attempts = 0

    def fail_serialization(_: object) -> bytes:
        nonlocal serialization_attempts
        serialization_attempts += 1
        raise TypeError("unserializable schema")

    async def scenario() -> None:
        mock = MCPMock()
        monkeypatch.setattr(mcp_adapter, "canonical_json_bytes", fail_serialization)
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=5
            ):
                pass
        assert serialization_attempts == 1

    run(scenario())


def near_wrapper_limit_plan() -> Plan:
    return Plan(
        summary="x" * 512,
        steps=tuple({"kind": "go_to", "x": 0, "y": 0, "z": 0} for _ in range(1857)),
    )


def test_validate_plan_rejects_oversized_wrapper_before_wire_call() -> None:
    async def scenario() -> None:
        mock = MCPMock()
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        async with factory.open(request(), timeout_seconds=5) as session:
            with pytest.raises(PlannerUnavailable):
                await session.validate_plan(near_wrapper_limit_plan())
        assert [body["method"] for _, _, body in mock.requests].count("tools/call") == 0

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
            self.mutated_responses = 0

        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] != "initialize":
                return response
            if self.kind == "session":
                mutated_response = httpx.Response(
                    200,
                    headers={
                        "content-type": "application/json",
                        "mcp-session-id": "forbidden-session",
                    },
                    content=response.content,
                    request=raw,
                )
            else:
                mutated_response = httpx.Response(
                    200,
                    headers={"content-type": "text/event-stream"},
                    content=b"event: message\ndata: {}\n\n",
                    request=raw,
                )
            self.mutated_responses += 1
            return mutated_response

    async def scenario(kind: str) -> None:
        mock = ForbiddenResponseMock(kind)
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
                request(), timeout_seconds=1
            ):
                pass
        assert mock.mutated_responses == 1

    for kind in ("session", "sse"):
        run(scenario(kind))


def test_redirect_is_not_followed() -> None:
    requests: list[httpx.Request] = []
    responses: list[httpx.Response] = []

    async def redirect(raw: httpx.Request) -> httpx.Response:
        requests.append(raw)
        response = httpx.Response(
            307,
            headers={"location": "http://127.0.0.1:9/steal"},
            request=raw,
        )
        responses.append(response)
        return response

    async def scenario() -> None:
        with pytest.raises(PlannerUnavailable):
            async with MCPToolSessionFactory(transport=httpx.MockTransport(redirect)).open(
                request(), timeout_seconds=1
            ):
                pass
        assert len(requests) == 1
        assert len(responses) == 1
        assert requests[0].url == request().mcp_endpoint

    run(scenario())


def test_http_client_disables_environment_and_redirects(monkeypatch: pytest.MonkeyPatch) -> None:
    captured: dict[str, object] = {}
    mock = MCPMock()
    original = httpx.AsyncClient

    def recording_client(**kwargs: object) -> httpx.AsyncClient:
        captured.update(kwargs)
        return original(**kwargs)

    async def scenario() -> None:
        monkeypatch.setattr(mcp_adapter.httpx, "AsyncClient", recording_client)
        async with MCPToolSessionFactory(transport=httpx.MockTransport(mock)).open(
            request(), timeout_seconds=5
        ):
            pass
        assert captured["trust_env"] is False
        assert captured["follow_redirects"] is False

    run(scenario())


def test_text_fallback_is_strict_json_and_never_retries_tool_call() -> None:
    class InvalidTextMock(MCPMock):
        def __init__(self) -> None:
            super().__init__(text_only={"inspect_inventory"})
            self.mutated_responses = 0

        async def __call__(self, raw: httpx.Request) -> httpx.Response:
            response = await super().__call__(raw)
            body = json.loads(raw.content)
            if body["method"] == "tools/call":
                payload = json.loads(response.content)
                payload["result"].pop("structuredContent", None)
                payload["result"]["content"] = [
                    {"type": "text", "text": '{"slots":[],"slots":[]}'},
                ]
                mutated_response = httpx.Response(
                    200,
                    headers={"content-type": "application/json"},
                    json=payload,
                    request=raw,
                )
                self.mutated_responses += 1
                return mutated_response
            return response

    async def scenario() -> None:
        mock = InvalidTextMock()
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        async with factory.open(request(), timeout_seconds=5) as session:
            with pytest.raises(PlannerUnavailable):
                await session.call_model_tool(
                    "inspect_inventory",
                    golden("mcp-v1", "valid", "inventory query is paged")["value"],
                )
        assert mock.mutated_responses == 1
        assert [body["method"] for _, _, body in mock.requests].count("tools/call") == 1

    run(scenario())


class DirectResultSession:
    def __init__(self, result: object) -> None:
        self.result = result
        self.calls = 0

    async def send_request(self, *args: object, **kwargs: object) -> object:
        del args, kwargs
        self.calls += 1
        return self.result


def test_oversized_text_content_is_rejected_before_json_decode(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    decoded = 0

    def forbidden_decode(value: str) -> dict[str, object]:
        nonlocal decoded
        del value
        decoded += 1
        raise AssertionError("oversized text reached JSON decoder")

    result = mcp_adapter.types.CallToolResult(
        content=[mcp_adapter.types.TextContent(type="text", text=" " * (MCP_WIRE_LIMIT + 1))],
        isError=False,
    )
    direct = DirectResultSession(result)

    async def scenario() -> None:
        monkeypatch.setattr(mcp_adapter, "strict_json_object", forbidden_decode)
        session = mcp_adapter._MCPPlanningToolSession(direct, timeout_seconds=5)
        with pytest.raises(PlannerUnavailable):
            await session._raw_call("inspect_inventory", {"offset": 0, "limit": 1})
        assert decoded == 0
        assert direct.calls == 1

    run(scenario())


def test_oversized_structured_content_stops_before_text_comparison(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    result = mcp_adapter.types.CallToolResult(
        content=[mcp_adapter.types.TextContent(type="text", text="{}")],
        structuredContent={"padding": "x" * MCP_WIRE_LIMIT},
        isError=False,
    )
    direct = DirectResultSession(result)
    original = mcp_adapter.canonical_json_bytes
    canonical_sizes: list[int] = []

    def recording_canonical(value: object) -> bytes:
        encoded = original(value)
        canonical_sizes.append(len(encoded))
        return encoded

    async def scenario() -> None:
        monkeypatch.setattr(mcp_adapter, "canonical_json_bytes", recording_canonical)
        session = mcp_adapter._MCPPlanningToolSession(direct, timeout_seconds=5)
        with pytest.raises(PlannerUnavailable):
            await session._raw_call("inspect_inventory", {"offset": 0, "limit": 1})
        assert canonical_sizes == [MCP_WIRE_LIMIT + len('{"padding":""}')]
        assert direct.calls == 1

    run(scenario())


@pytest.mark.parametrize("mutation", ["mismatch", "multiple", "is_error"])
def test_tool_result_shape_fails_closed_without_retry(mutation: str) -> None:
    class ResultMutationMock(MCPMock):
        def __init__(self) -> None:
            super().__init__()
            self.mutated_responses = 0

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
            mutated_response = httpx.Response(
                200,
                headers={"content-type": "application/json"},
                json=payload,
                request=raw,
            )
            self.mutated_responses += 1
            return mutated_response

    async def scenario() -> None:
        mock = ResultMutationMock()
        factory = MCPToolSessionFactory(transport=httpx.MockTransport(mock))
        async with factory.open(request(), timeout_seconds=5) as session:
            with pytest.raises(PlannerUnavailable):
                await session.call_model_tool(
                    "inspect_inventory",
                    golden("mcp-v1", "valid", "inventory query is paged")["value"],
                )
        assert mock.mutated_responses == 1
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

"""每个 Planner run 一个 stateless Streamable HTTP MCP 会话。"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from datetime import timedelta

import httpx
from mcp import ClientSession, types
from mcp.client.streamable_http import streamable_http_client
from pydantic import ValidationError

from mornlea_companion_agent.adapters.response_limit import BoundedAsyncTransport
from mornlea_companion_agent.domain import adapter_for
from mornlea_companion_agent.domain.common import canonical_json_bytes
from mornlea_companion_agent.domain.http_v1 import PlanRequest
from mornlea_companion_agent.domain.mcp_contract import mcp_tool_contracts
from mornlea_companion_agent.domain.mcp_v1 import (
    GetPlanningContextResult,
    Plan,
    QueryTerrainInput,
    ValidatePlanFailureResult,
    ValidatePlanInput,
    ValidatePlanSuccessResult,
)
from mornlea_companion_agent.domain.planner import (
    MODEL_VISIBLE_TOOLS,
    InvalidModelOutput,
    PlannerFailure,
    PlannerUnavailable,
    PlanningToolSession,
    strict_json_object,
    validate_tool_input,
    validate_tool_output,
)

MCP_PROTOCOL_VERSION = "2025-11-25"
MCP_RESPONSE_BODY_LIMIT = 160 * 1024
_TOOL_CONTRACTS = mcp_tool_contracts()
if tuple(tool.name for tool in _TOOL_CONTRACTS if tool.model_visible) != MODEL_VISIBLE_TOOLS:
    raise RuntimeError("MCP manifest model-visible tools do not match planner contract")


class _MCPPlanningToolSession:
    def __init__(self, session: ClientSession, *, timeout_seconds: float) -> None:
        self._session = session
        self._read_timeout = timedelta(seconds=timeout_seconds)

    async def get_planning_context(self) -> GetPlanningContextResult:
        value = await self._call("get_planning_context", {}, "get_planning_context_result")
        if not isinstance(value, GetPlanningContextResult):
            raise PlannerUnavailable
        return value

    async def call_model_tool(self, name: str, arguments: dict[str, object]) -> object:
        if name not in MODEL_VISIBLE_TOOLS:
            raise PlannerUnavailable
        try:
            parsed_input = validate_tool_input(name, arguments)
        except InvalidModelOutput:
            raise PlannerUnavailable from None
        raw = await self._raw_call(name, parsed_input.model_dump(mode="json"))
        return validate_tool_output(name, raw, parsed_input)

    async def validate_plan(
        self, plan: Plan
    ) -> ValidatePlanSuccessResult | ValidatePlanFailureResult:
        try:
            parsed = ValidatePlanInput.model_validate({"plan": plan})
        except ValidationError:
            raise PlannerUnavailable from None
        value = await self._call(
            "validate_plan",
            parsed.model_dump(mode="json"),
            "validate_plan_result",
        )
        if not isinstance(value, (ValidatePlanSuccessResult, ValidatePlanFailureResult)):
            raise PlannerUnavailable
        return value

    async def _call(self, name: str, arguments: dict[str, object], schema: str) -> object:
        payload = await self._raw_call(name, arguments)
        context: dict[str, object] | None = None
        if name == "query_terrain":
            try:
                parsed = QueryTerrainInput.model_validate(arguments)
            except ValidationError:
                raise PlannerUnavailable from None
            context = {"positions": parsed.model_dump(mode="json")["positions"]}
        try:
            return adapter_for("mcp-v1", schema).validate_python(payload, context=context)
        except ValidationError:
            raise PlannerUnavailable from None

    async def _raw_call(self, name: str, arguments: dict[str, object]) -> dict[str, object]:
        request = types.ClientRequest(
            types.CallToolRequest(
                params=types.CallToolRequestParams(name=name, arguments=arguments)
            )
        )
        try:
            result = await self._session.send_request(
                request,
                types.CallToolResult,
                request_read_timeout_seconds=self._read_timeout,
            )
        except asyncio.CancelledError:
            raise
        except Exception:
            raise PlannerUnavailable from None
        if result.isError or len(result.content) != 1:
            raise PlannerUnavailable
        content = result.content[0]
        if not isinstance(content, types.TextContent):
            raise PlannerUnavailable
        try:
            encoded_text = content.text.encode("utf-8", errors="strict")
        except UnicodeEncodeError:
            raise PlannerUnavailable from None
        if len(encoded_text) > MCP_RESPONSE_BODY_LIMIT:
            raise PlannerUnavailable
        try:
            text_value = strict_json_object(content.text)
        except PlannerFailure:
            raise PlannerUnavailable from None
        structured = result.structuredContent
        if structured is None:
            return text_value
        if type(structured) is not dict:
            raise PlannerUnavailable
        try:
            structured_bytes = canonical_json_bytes(structured)
        except (TypeError, ValueError, UnicodeEncodeError):
            raise PlannerUnavailable from None
        if len(structured_bytes) > MCP_RESPONSE_BODY_LIMIT:
            raise PlannerUnavailable
        if structured_bytes != canonical_json_bytes(text_value):
            raise PlannerUnavailable
        return structured


class MCPToolSessionFactory:
    """创建真实 SDK 会话，同时拒绝 redirect、SSE 与 session state。"""

    def __init__(self, *, transport: httpx.AsyncBaseTransport | None = None) -> None:
        self._transport = transport

    @asynccontextmanager
    async def open(
        self,
        request: PlanRequest,
        *,
        timeout_seconds: float,
    ) -> AsyncIterator[PlanningToolSession]:
        if types.LATEST_PROTOCOL_VERSION != MCP_PROTOCOL_VERSION:
            raise PlannerUnavailable

        async def reject_stateful_response(response: httpx.Response) -> None:
            content_type = response.headers.get("content-type", "").lower()
            if (
                response.is_redirect
                or "mcp-session-id" in response.headers
                or content_type.startswith("text/event-stream")
            ):
                await response.aclose()
                raise PlannerUnavailable

        client = httpx.AsyncClient(
            headers={
                "Accept-Encoding": "identity",
                "Authorization": f"Bearer {request.mcp_capability}",
            },
            follow_redirects=False,
            trust_env=False,
            timeout=httpx.Timeout(timeout_seconds),
            transport=BoundedAsyncTransport(
                self._transport or httpx.AsyncHTTPTransport(retries=0),
                maximum_bytes=MCP_RESPONSE_BODY_LIMIT,
            ),
            event_hooks={"response": [reject_stateful_response]},
        )
        try:
            async with client:
                async with streamable_http_client(
                    request.mcp_endpoint,
                    http_client=client,
                    terminate_on_close=False,
                ) as (read_stream, write_stream, get_session_id):
                    async with ClientSession(
                        read_stream,
                        write_stream,
                        read_timeout_seconds=timedelta(seconds=timeout_seconds),
                        client_info=types.Implementation(
                            name="mornlea-companion-agent",
                            version="v1",
                        ),
                    ) as session:
                        initialized = await session.initialize()
                        self._validate_initialize(initialized, get_session_id())
                        listed = await session.list_tools()
                        self._validate_tools(listed)
                        yield _MCPPlanningToolSession(
                            session,
                            timeout_seconds=timeout_seconds,
                        )
        except asyncio.CancelledError:
            raise
        except PlannerFailure:
            raise
        except Exception:
            raise PlannerUnavailable from None

    @staticmethod
    def _validate_initialize(result: types.InitializeResult, session_id: str | None) -> None:
        capabilities = result.capabilities.model_dump(
            mode="json",
            by_alias=True,
            exclude_none=True,
        )
        if (
            type(result.protocolVersion) is not str
            or result.protocolVersion != MCP_PROTOCOL_VERSION
            or result.serverInfo.version != "v1"
            or capabilities != {"tools": {"listChanged": False}}
            or session_id is not None
        ):
            raise PlannerUnavailable

    @staticmethod
    def _validate_tools(result: types.ListToolsResult) -> None:
        if result.nextCursor is not None or len(result.tools) != len(_TOOL_CONTRACTS):
            raise PlannerUnavailable
        for advertised, expected in zip(result.tools, _TOOL_CONTRACTS, strict=True):
            try:
                input_schema_matches = canonical_json_bytes(
                    advertised.inputSchema
                ) == canonical_json_bytes(expected.input_schema)
                output_schema_matches = canonical_json_bytes(
                    advertised.outputSchema
                ) == canonical_json_bytes(expected.output_schema)
            except (TypeError, ValueError, UnicodeError, RecursionError, OverflowError):
                raise PlannerUnavailable from None
            if (
                advertised.name != expected.name
                or not input_schema_matches
                or not output_schema_matches
            ):
                raise PlannerUnavailable


__all__ = ["MCP_PROTOCOL_VERSION", "MCP_RESPONSE_BODY_LIMIT", "MCPToolSessionFactory"]

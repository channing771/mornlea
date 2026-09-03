"""无 checkpoint 的有界 Planner LangGraph harness。"""

from __future__ import annotations

import asyncio
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph

from mornlea_companion_agent.domain.common import canonical_json_bytes
from mornlea_companion_agent.domain.http_v1 import PlanRequest, PlanResponse
from mornlea_companion_agent.domain.mcp_v1 import (
    GetPlanningContextResult,
    Plan,
    ValidatePlanFailureResult,
    ValidatePlanSuccessResult,
)
from mornlea_companion_agent.domain.planner import (
    InvalidModelOutput,
    ModelOutput,
    ModelToolCall,
    PlannerDeadlineExceeded,
    PlannerFailure,
    PlannerLimits,
    PlannerMessage,
    PlannerModel,
    PlannerUnavailable,
    PlanningToolSession,
    PlanningToolSessionFactory,
    parse_plan_output,
    validate_tool_input,
    validate_tool_output,
)

_SYSTEM_PROMPT = (
    "你是 Mornlea 伙伴规划器。只依据本次冻结事实和只读工具，返回一个严格 JSON Plan；"
    "不得返回代码、链接、解释或未交付步骤。"
)
_HTTP_RESPONSE_BODY_LIMIT = 65_536


class _GraphState(TypedDict, total=False):
    request: PlanRequest
    response: PlanResponse


@dataclass(slots=True)
class _Budget:
    limits: PlannerLimits
    model_calls: int = 0
    tool_calls: int = 0
    validator_calls: int = 0
    tool_keys: set[bytes] = field(default_factory=set)
    call_ids: set[str] = field(default_factory=set)

    def begin_model(self) -> None:
        if self.model_calls >= self.limits.model_calls:
            raise InvalidModelOutput
        self.model_calls += 1

    def preflight_tools(self, count: int) -> None:
        if self.tool_calls + count > self.limits.tool_calls:
            raise InvalidModelOutput

    def begin_tool(self) -> None:
        self.tool_calls += 1

    def begin_validator(self) -> None:
        if self.validator_calls >= 2:
            raise InvalidModelOutput
        self.validator_calls += 1


class _PlannerRun:
    def __init__(
        self,
        model: PlannerModel,
        tools: PlanningToolSession,
        limits: PlannerLimits,
    ) -> None:
        self._model = model
        self._tools = tools
        self._budget = _Budget(limits)

    async def execute(self, request: PlanRequest) -> PlanResponse:
        context = await self._get_context(request)
        messages = self._initial_messages(context)
        candidate = await self._candidate(messages, allow_tools=True)
        result = await self._validate(candidate)
        if isinstance(result, ValidatePlanFailureResult):
            messages.append(
                PlannerMessage(
                    role="assistant",
                    content=canonical_json_bytes(candidate).decode("utf-8"),
                )
            )
            messages.append(
                PlannerMessage(
                    role="user",
                    content=canonical_json_bytes(
                        {
                            "previous_plan": candidate.model_dump(mode="json"),
                            "repair": {
                                "code": result.code,
                                "hint": result.hint,
                            },
                        }
                    ).decode("utf-8"),
                )
            )
            candidate = await self._candidate(messages, allow_tools=False)
            result = await self._validate(candidate)
            if isinstance(result, ValidatePlanFailureResult):
                raise InvalidModelOutput
        self._assert_accepted(request, candidate, result)
        response = PlanResponse(
            contract_version=request.contract_version,
            request_id=request.request_id,
            client_instance_id=request.client_instance_id,
            namespace_id=request.namespace_id,
            lease_id=request.lease_id,
            run_id=request.run_id,
            companion_id=request.companion_id,
            generation=request.generation,
            snapshot_id=request.snapshot_id,
            snapshot_digest=request.snapshot_digest,
            plan=candidate,
        )
        if len(canonical_json_bytes(response)) > _HTTP_RESPONSE_BODY_LIMIT:
            raise InvalidModelOutput
        return response

    async def _get_context(self, request: PlanRequest) -> GetPlanningContextResult:
        try:
            context = await self._tools.get_planning_context()
        except asyncio.CancelledError:
            raise
        except PlannerFailure:
            raise
        except Exception:
            raise PlannerUnavailable from None
        if (
            context.snapshot_digest != request.snapshot_digest
            or context.instruction != request.instruction
            or context.companion.companion_id != request.companion_id
        ):
            raise PlannerUnavailable
        return context

    @staticmethod
    def _initial_messages(context: GetPlanningContextResult) -> list[PlannerMessage]:
        return [
            PlannerMessage(role="system", content=_SYSTEM_PROMPT),
            PlannerMessage(
                role="user",
                content=canonical_json_bytes(context).decode("utf-8"),
            ),
        ]

    async def _candidate(
        self,
        messages: list[PlannerMessage],
        *,
        allow_tools: bool,
    ) -> Plan:
        while True:
            output = await self._complete(messages, allow_tools=allow_tools)
            if output.invalid_tool_calls:
                raise InvalidModelOutput
            has_content = output.content not in (None, "")
            if output.tool_calls:
                if has_content or not allow_tools:
                    raise InvalidModelOutput
                await self._execute_tool_batch(messages, output.tool_calls)
                continue
            return parse_plan_output(output.content)

    async def _complete(
        self,
        messages: list[PlannerMessage],
        *,
        allow_tools: bool,
    ) -> ModelOutput:
        self._budget.begin_model()
        try:
            return await self._model.complete(tuple(messages), allow_tools=allow_tools)
        except asyncio.CancelledError:
            raise
        except PlannerFailure:
            raise
        except Exception:
            raise PlannerUnavailable from None

    async def _execute_tool_batch(
        self,
        messages: list[PlannerMessage],
        calls: tuple[ModelToolCall, ...],
    ) -> None:
        self._budget.preflight_tools(len(calls))
        prepared: list[tuple[ModelToolCall, Any, bytes]] = []
        batch_keys: set[bytes] = set()
        batch_ids: set[str] = set()
        for call in calls:
            if (
                type(call.call_id) is not str
                or not call.call_id
                or call.call_id in self._budget.call_ids
                or call.call_id in batch_ids
            ):
                raise InvalidModelOutput
            arguments = validate_tool_input(call.name, call.arguments)
            key = call.name.encode("utf-8") + b"\x00" + canonical_json_bytes(arguments)
            if key in self._budget.tool_keys or key in batch_keys:
                raise InvalidModelOutput
            batch_ids.add(call.call_id)
            batch_keys.add(key)
            prepared.append((call, arguments, key))

        messages.append(PlannerMessage(role="assistant", content="", tool_calls=calls))
        for call, arguments, key in prepared:
            self._budget.begin_tool()
            try:
                raw = await self._tools.call_model_tool(
                    call.name,
                    arguments.model_dump(mode="json"),
                )
                output = validate_tool_output(call.name, raw, arguments)
            except asyncio.CancelledError:
                raise
            except PlannerFailure:
                raise
            except Exception:
                raise PlannerUnavailable from None
            self._budget.call_ids.add(call.call_id)
            self._budget.tool_keys.add(key)
            messages.append(
                PlannerMessage(
                    role="tool",
                    content=canonical_json_bytes(output).decode("utf-8"),
                    tool_call_id=call.call_id,
                    tool_name=call.name,
                )
            )

    async def _validate(
        self, candidate: Plan
    ) -> ValidatePlanSuccessResult | ValidatePlanFailureResult:
        self._budget.begin_validator()
        try:
            return await self._tools.validate_plan(candidate)
        except asyncio.CancelledError:
            raise
        except PlannerFailure:
            raise
        except Exception:
            raise PlannerUnavailable from None

    @staticmethod
    def _assert_accepted(
        request: PlanRequest,
        candidate: Plan,
        result: ValidatePlanSuccessResult,
    ) -> None:
        if result.snapshot_digest != request.snapshot_digest or canonical_json_bytes(
            result.plan
        ) != canonical_json_bytes(candidate):
            raise PlannerUnavailable


class PlannerHarness:
    """为每个请求建立独立无持久化 graph state。"""

    def __init__(
        self,
        model: PlannerModel,
        tool_sessions: PlanningToolSessionFactory,
        *,
        limits: PlannerLimits | None = None,
        wall_clock: Callable[[], float] | None = None,
    ) -> None:
        self._model = model
        self._tool_sessions = tool_sessions
        self._limits = limits or PlannerLimits()
        self._wall_clock = wall_clock or time.time

    async def run(self, request: PlanRequest) -> PlanResponse:
        remaining = request.deadline_unix_ms / 1000 - self._wall_clock()
        timeout_seconds = min(float(self._limits.timeout_seconds), remaining)
        if timeout_seconds <= 0:
            raise PlannerDeadlineExceeded
        try:
            async with asyncio.timeout(timeout_seconds):
                async with self._tool_sessions.open(
                    request,
                    timeout_seconds=timeout_seconds,
                ) as tools:
                    graph = self._compile_graph(_PlannerRun(self._model, tools, self._limits))
                    state = await graph.ainvoke({"request": request})
                    response = state.get("response")
                    if not isinstance(response, PlanResponse):
                        raise PlannerUnavailable
                    return response
        except asyncio.CancelledError:
            raise
        except TimeoutError:
            raise PlannerDeadlineExceeded from None
        except PlannerFailure:
            raise
        except Exception:
            raise PlannerUnavailable from None

    @staticmethod
    def _compile_graph(run: _PlannerRun) -> Any:
        async def execute(state: _GraphState) -> dict[str, PlanResponse]:
            return {"response": await run.execute(state["request"])}

        graph = StateGraph(_GraphState)
        graph.add_node("planner", execute)
        graph.add_edge(START, "planner")
        graph.add_edge("planner", END)
        return graph.compile(checkpointer=None)


__all__ = ["PlannerHarness"]

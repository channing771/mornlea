from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncIterator, Awaitable
from contextlib import asynccontextmanager
from copy import deepcopy
from pathlib import Path
from typing import Any, cast

import httpx
import pytest
from langchain_core.messages import AIMessage
from pydantic import ValidationError
from pydantic.types import SecretStr

from mornlea_companion_agent.adapters import model as model_adapter
from mornlea_companion_agent.adapters.model import (
    PROVIDER_RESPONSE_BODY_LIMIT,
    ChatOpenAIPlannerModel,
)
from mornlea_companion_agent.domain.common import canonical_json_bytes
from mornlea_companion_agent.domain.http_v1 import PlanRequest
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
    PlannerLimits,
    PlannerMessage,
    PlannerModel,
    PlannerUnavailable,
    PlanningToolSession,
    PlanningToolSessionFactory,
)
from mornlea_companion_agent.harness.planner import PlannerHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[4]
CONTRACT_ROOT = REPOSITORY_ROOT / "contracts/companion-agent"
PROVIDER_RESPONSE_LIMIT = PROVIDER_RESPONSE_BODY_LIMIT


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def golden(contract: str, kind: str, name: str) -> dict[str, Any]:
    document = json.loads(
        (CONTRACT_ROOT / contract / "golden" / f"{kind}.json").read_text(encoding="utf-8")
    )
    return next(deepcopy(case) for case in document["cases"] if case["name"] == name)


def resolved_contract_schema(name: str) -> dict[str, object]:
    document = json.loads((CONTRACT_ROOT / "mcp-v1/schema.json").read_text(encoding="utf-8"))
    definitions = document["$defs"]

    def resolve(value: object, stack: tuple[str, ...] = ()) -> object:
        if isinstance(value, dict):
            reference = value.get("$ref")
            if set(value) == {"$ref"} and isinstance(reference, str):
                target = reference.removeprefix("#/$defs/")
                assert target not in stack
                return resolve(definitions[target], (*stack, target))
            return {key: resolve(item, stack) for key, item in value.items()}
        if isinstance(value, list):
            return [resolve(item, stack) for item in value]
        return value

    resolved = resolve(definitions[name], (name,))
    assert isinstance(resolved, dict)
    return resolved


def model_visible_contract_tools() -> list[dict[str, object]]:
    manifest = json.loads((CONTRACT_ROOT / "mcp-v1/manifest.json").read_text(encoding="utf-8"))
    return [tool for tool in manifest["tools"] if tool["model_visible"]]


def plan_request() -> PlanRequest:
    case = golden("http-v1", "valid", "planner run carries snapshot identity")
    return PlanRequest.model_validate(case["value"])


def planning_context(request: PlanRequest | None = None) -> GetPlanningContextResult:
    request = request or plan_request()
    case = golden("mcp-v1", "valid", "planning context is a bounded projection")
    value = case["value"]
    value["instruction"] = request.instruction
    value["companion"]["companion_id"] = request.companion_id
    value["snapshot_digest"] = request.snapshot_digest
    return GetPlanningContextResult.model_validate(value)


def accepted_plan() -> Plan:
    case = golden(
        "mcp-v1",
        "valid",
        "validator accepted result echoes digest and canonical plan",
    )
    return Plan.model_validate(case["value"]["plan"])


def plan_json(plan: Plan | None = None) -> str:
    value = plan or accepted_plan()
    return json.dumps(value.model_dump(mode="json"), ensure_ascii=False, separators=(",", ":"))


def response_envelope_overflow_plan() -> Plan:
    plan = Plan(
        summary="x" * 512,
        steps=tuple({"kind": "go_to", "x": 0, "y": 0, "z": 0} for _ in range(1843)),
    )
    assert len(canonical_json_bytes(plan)) <= 65_536
    return plan


class ScriptedModel(PlannerModel):
    def __init__(self, *outputs: ModelOutput | Awaitable[ModelOutput]) -> None:
        self.outputs = list(outputs)
        self.calls: list[tuple[tuple[PlannerMessage, ...], bool]] = []

    async def complete(
        self,
        messages: tuple[PlannerMessage, ...],
        *,
        allow_tools: bool,
    ) -> ModelOutput:
        self.calls.append((messages, allow_tools))
        if not self.outputs:
            raise AssertionError("unexpected model call")
        output = self.outputs.pop(0)
        if isinstance(output, Awaitable):
            return await output
        return output


class HangingModel(PlannerModel):
    def __init__(self) -> None:
        self.started = asyncio.Event()

    async def complete(
        self,
        messages: tuple[PlannerMessage, ...],
        *,
        allow_tools: bool,
    ) -> ModelOutput:
        del messages, allow_tools
        self.started.set()
        await asyncio.Event().wait()
        raise AssertionError


class FakeTools(PlanningToolSession):
    def __init__(
        self,
        *,
        context: GetPlanningContextResult | None = None,
        validator_results: (
            list[ValidatePlanSuccessResult | ValidatePlanFailureResult] | None
        ) = None,
        tool_results: dict[str, object] | None = None,
        hang_tool: bool = False,
    ) -> None:
        self.context = context or planning_context()
        self.validator_results = validator_results or [
            ValidatePlanSuccessResult(
                accepted=True,
                snapshot_digest=self.context.snapshot_digest,
                plan=accepted_plan(),
            )
        ]
        self.tool_results = tool_results or {}
        self.hang_tool = hang_tool
        self.context_calls = 0
        self.validator_calls: list[Plan] = []
        self.tool_calls: list[tuple[str, dict[str, object]]] = []
        self.tool_started = asyncio.Event()

    async def get_planning_context(self) -> GetPlanningContextResult:
        self.context_calls += 1
        return self.context

    async def call_model_tool(self, name: str, arguments: dict[str, object]) -> object:
        self.tool_started.set()
        if self.hang_tool:
            await asyncio.Event().wait()
        self.tool_calls.append((name, arguments))
        return self.tool_results[name]

    async def validate_plan(
        self, plan: Plan
    ) -> ValidatePlanSuccessResult | ValidatePlanFailureResult:
        self.validator_calls.append(plan)
        if not self.validator_results:
            raise AssertionError("unexpected validator call")
        return self.validator_results.pop(0)


class FakeToolFactory(PlanningToolSessionFactory):
    def __init__(self, session: FakeTools) -> None:
        self.session = session
        self.opened = 0
        self.closed = 0
        self.timeout_seconds: list[float] = []

    @asynccontextmanager
    async def open(
        self,
        request: PlanRequest,
        *,
        timeout_seconds: float,
    ) -> AsyncIterator[PlanningToolSession]:
        del request
        self.opened += 1
        self.timeout_seconds.append(timeout_seconds)
        try:
            yield self.session
        finally:
            self.closed += 1


def accepted_result(
    context: GetPlanningContextResult,
    plan: Plan | None = None,
) -> ValidatePlanSuccessResult:
    return ValidatePlanSuccessResult(
        accepted=True,
        snapshot_digest=context.snapshot_digest,
        plan=plan or accepted_plan(),
    )


def test_planner_calls_context_once_and_returns_strict_correlated_plan() -> None:
    async def scenario() -> None:
        request = plan_request()
        context = planning_context(request)
        affordance_case = golden(
            "mcp-v1", "valid", "affordances include fixed steps and mine classifications"
        )
        model = ScriptedModel(
            ModelOutput(
                content=None,
                tool_calls=(
                    ModelToolCall(call_id="call-1", name="list_affordances", arguments={}),
                ),
            ),
            ModelOutput(content=plan_json(), tool_calls=()),
        )
        tools = FakeTools(
            context=context,
            validator_results=[accepted_result(context)],
            tool_results={"list_affordances": affordance_case["value"]},
        )
        factory = FakeToolFactory(tools)
        harness = PlannerHarness(model, factory, wall_clock=lambda: 1_700_000_000.0)

        response = await harness.run(request)

        assert response.plan == accepted_plan()
        assert response.snapshot_digest == request.snapshot_digest
        assert response.run_id == request.run_id
        assert tools.context_calls == 1
        assert tools.tool_calls == [("list_affordances", {})]
        assert tools.validator_calls == [accepted_plan()]
        assert [allow_tools for _, allow_tools in model.calls] == [True, True]
        assert factory.opened == factory.closed == 1

        visible_prompt = "\n".join(
            message.content
            for messages, _ in model.calls
            for message in messages
            if message.content is not None
        )
        for forbidden in (
            request.mcp_endpoint,
            request.mcp_capability,
            request.snapshot_id,
            "persona",
            "memory",
            "secret",
        ):
            assert forbidden not in visible_prompt

    run(scenario())


@pytest.mark.parametrize(
    ("tool_name", "arguments", "case_name"),
    [
        (
            "find_visible_blocks",
            {"block_names": ["unknown_name"], "limit": 1},
            "unknown visible block is a normal domain failure",
        ),
        (
            "query_terrain",
            {"positions": [{"x": 4, "y": 64, "z": -1}]},
            "terrain outside projection is a normal domain failure",
        ),
    ],
)
def test_domain_failure_is_canonical_tool_message_without_automatic_retry(
    tool_name: str,
    arguments: dict[str, object],
    case_name: str,
) -> None:
    async def scenario() -> None:
        failure = golden("mcp-v1", "valid", case_name)["value"]
        call = ModelToolCall(call_id="domain-failure", name=tool_name, arguments=arguments)
        model = ScriptedModel(
            ModelOutput(content=None, tool_calls=(call,)),
            ModelOutput(content=plan_json(), tool_calls=()),
        )
        tools = FakeTools(tool_results={tool_name: failure})
        response = await PlannerHarness(
            model,
            FakeToolFactory(tools),
            limits=PlannerLimits(model_calls=3, tool_calls=1, timeout_seconds=30),
            wall_clock=lambda: 1_700_000_000.0,
        ).run(plan_request())

        assert response.plan == accepted_plan()
        assert tools.tool_calls == [(tool_name, arguments)]
        assert len(model.calls) == 2
        tool_message = model.calls[1][0][-1]
        assert tool_message == PlannerMessage(
            role="tool",
            content=canonical_json_bytes(failure).decode("utf-8"),
            tool_call_id="domain-failure",
            tool_name=tool_name,
        )

    run(scenario())


def test_domain_failure_call_is_deduplicated_and_not_replayed() -> None:
    async def scenario() -> None:
        arguments = {"block_names": ["unknown_name"], "limit": 1}
        call = ModelToolCall(
            call_id="domain-failure-1",
            name="find_visible_blocks",
            arguments=arguments,
        )
        repeated = ModelToolCall(
            call_id="domain-failure-2",
            name="find_visible_blocks",
            arguments={"limit": 1, "block_names": ["unknown_name"]},
        )
        failure = golden("mcp-v1", "valid", "unknown visible block is a normal domain failure")[
            "value"
        ]
        tools = FakeTools(tool_results={"find_visible_blocks": failure})
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                ScriptedModel(
                    ModelOutput(content=None, tool_calls=(call,)),
                    ModelOutput(content=None, tool_calls=(repeated,)),
                ),
                FakeToolFactory(tools),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(plan_request())
        assert tools.tool_calls == [("find_visible_blocks", arguments)]

    run(scenario())


def test_full_plan_response_envelope_must_fit_http_v1_limit() -> None:
    async def scenario() -> None:
        request = plan_request()
        context = planning_context(request)
        plan = response_envelope_overflow_plan()
        tools = FakeTools(
            context=context,
            validator_results=[accepted_result(context, plan)],
        )
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                ScriptedModel(ModelOutput(content=plan_json(plan), tool_calls=())),
                FakeToolFactory(tools),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(request)
        assert tools.validator_calls == [plan]

    run(scenario())


@pytest.mark.parametrize(
    "calls",
    [
        (
            ModelToolCall(
                call_id="call-1",
                name="inspect_inventory",
                arguments={"offset": 0, "limit": 36},
            ),
            ModelToolCall(
                call_id="call-2",
                name="inspect_inventory",
                arguments={"limit": 36, "offset": 0},
            ),
        ),
        (
            ModelToolCall(call_id="call-1", name="list_affordances", arguments={}),
            ModelToolCall(call_id="call-2", name="unknown_tool", arguments={}),
        ),
    ],
)
def test_tool_batch_is_fully_prevalidated_before_any_execution(
    calls: tuple[ModelToolCall, ...],
) -> None:
    async def scenario() -> None:
        tools = FakeTools()
        model = ScriptedModel(ModelOutput(content=None, tool_calls=calls))
        harness = PlannerHarness(model, FakeToolFactory(tools), wall_clock=lambda: 1_700_000_000.0)
        with pytest.raises(InvalidModelOutput):
            await harness.run(plan_request())
        assert tools.context_calls == 1
        assert tools.tool_calls == []
        assert tools.validator_calls == []

    run(scenario())


def test_tool_batch_budget_overflow_has_no_partial_execution() -> None:
    async def scenario() -> None:
        calls = tuple(
            ModelToolCall(call_id=f"call-{index}", name="list_affordances", arguments={})
            for index in range(2)
        )
        tools = FakeTools()
        harness = PlannerHarness(
            ScriptedModel(ModelOutput(content=None, tool_calls=calls)),
            FakeToolFactory(tools),
            limits=PlannerLimits(model_calls=3, tool_calls=1, timeout_seconds=30),
            wall_clock=lambda: 1_700_000_000.0,
        )
        with pytest.raises(InvalidModelOutput):
            await harness.run(plan_request())
        assert tools.tool_calls == []

    run(scenario())


def test_hard_budget_configuration_and_exact_tool_boundary() -> None:
    with pytest.raises(ValidationError):
        PlannerLimits(model_calls=6, tool_calls=8, timeout_seconds=60)
    with pytest.raises(ValidationError):
        PlannerLimits(model_calls=5, tool_calls=9, timeout_seconds=60)

    async def scenario() -> None:
        inventory = golden("mcp-v1", "valid", "inventory result contains bounded occupied slots")[
            "value"
        ]
        calls = tuple(
            ModelToolCall(
                call_id=f"call-{index}",
                name="inspect_inventory",
                arguments={"offset": index, "limit": 1},
            )
            for index in range(8)
        )
        tools = FakeTools(
            tool_results={"inspect_inventory": inventory},
            validator_results=[accepted_result(planning_context())],
        )
        model = ScriptedModel(
            ModelOutput(content=None, tool_calls=calls),
            ModelOutput(content=plan_json(), tool_calls=()),
        )
        response = await PlannerHarness(
            model,
            FakeToolFactory(tools),
            limits=PlannerLimits(model_calls=5, tool_calls=8, timeout_seconds=60),
            wall_clock=lambda: 1_700_000_000.0,
        ).run(plan_request())
        assert response.plan == accepted_plan()
        assert len(tools.tool_calls) == 8

    run(scenario())


def test_model_budget_is_counted_before_each_new_round() -> None:
    async def scenario() -> None:
        affordances = golden(
            "mcp-v1", "valid", "affordances include fixed steps and mine classifications"
        )["value"]
        inventory = golden("mcp-v1", "valid", "inventory result contains bounded occupied slots")[
            "value"
        ]
        model = ScriptedModel(
            ModelOutput(
                content=None,
                tool_calls=(ModelToolCall("call-1", "list_affordances", {}),),
            ),
            ModelOutput(
                content=None,
                tool_calls=(
                    ModelToolCall(
                        "call-2",
                        "inspect_inventory",
                        {"offset": 0, "limit": 1},
                    ),
                ),
            ),
            ModelOutput(content=plan_json(), tool_calls=()),
        )
        tools = FakeTools(
            tool_results={
                "list_affordances": affordances,
                "inspect_inventory": inventory,
            }
        )
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                model,
                FakeToolFactory(tools),
                limits=PlannerLimits(model_calls=2, tool_calls=4, timeout_seconds=30),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(plan_request())
        assert len(model.calls) == 2
        assert len(tools.tool_calls) == 2
        assert tools.validator_calls == []

    run(scenario())


def test_one_validator_rejection_allows_one_tool_free_repair() -> None:
    async def scenario() -> None:
        request = plan_request()
        context = planning_context(request)
        first = Plan(summary="先走错一步", steps=({"kind": "go_to", "x": 1, "y": 64, "z": 0},))
        repaired = accepted_plan()
        tools = FakeTools(
            context=context,
            validator_results=[
                ValidatePlanFailureResult(
                    accepted=False,
                    code="snapshot_mismatch",
                    hint="候选与冻结快照不一致",
                ),
                accepted_result(context, repaired),
            ],
        )
        model = ScriptedModel(
            ModelOutput(content=plan_json(first), tool_calls=()),
            ModelOutput(content=plan_json(repaired), tool_calls=()),
        )
        response = await PlannerHarness(
            model, FakeToolFactory(tools), wall_clock=lambda: 1_700_000_000.0
        ).run(request)
        assert response.plan == repaired
        assert [allow_tools for _, allow_tools in model.calls] == [True, False]
        assert tools.validator_calls == [first, repaired]
        repair_messages = model.calls[1][0]
        assert repair_messages[-2] == PlannerMessage(
            role="assistant",
            content=json.dumps(
                first.model_dump(mode="json"),
                ensure_ascii=False,
                sort_keys=True,
                separators=(",", ":"),
            ),
        )
        repair_payload = json.loads(repair_messages[-1].content)
        assert repair_payload["previous_plan"] == first.model_dump(mode="json")
        assert repair_payload["repair"] == {
            "code": "snapshot_mismatch",
            "hint": "候选与冻结快照不一致",
        }

    run(scenario())


def test_second_validator_rejection_is_terminal() -> None:
    async def scenario() -> None:
        context = planning_context()
        rejection = ValidatePlanFailureResult(
            accepted=False,
            code="unmineable_target",
            hint="目标不可采掘",
        )
        tools = FakeTools(context=context, validator_results=[rejection, rejection])
        model = ScriptedModel(
            ModelOutput(content=plan_json(), tool_calls=()),
            ModelOutput(content=plan_json(), tool_calls=()),
        )
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                model, FakeToolFactory(tools), wall_clock=lambda: 1_700_000_000.0
            ).run(plan_request())
        assert len(tools.validator_calls) == 2
        assert len(model.calls) == 2

    run(scenario())


@pytest.mark.parametrize(
    "output",
    [
        ModelOutput(
            content=plan_json(),
            tool_calls=(ModelToolCall("call-1", "list_affordances", {}),),
        ),
        ModelOutput(content=plan_json(), tool_calls=(), invalid_tool_calls=True),
    ],
)
def test_mixed_content_and_invalid_tool_calls_are_rejected(output: ModelOutput) -> None:
    async def scenario() -> None:
        tools = FakeTools()
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                ScriptedModel(output),
                FakeToolFactory(tools),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(plan_request())
        assert tools.tool_calls == []
        assert tools.validator_calls == []

    run(scenario())


def test_repair_phase_forbids_tools_without_execution() -> None:
    async def scenario() -> None:
        rejection = ValidatePlanFailureResult(
            accepted=False,
            code="snapshot_mismatch",
            hint="请修复",
        )
        tools = FakeTools(validator_results=[rejection])
        model = ScriptedModel(
            ModelOutput(content=plan_json(), tool_calls=()),
            ModelOutput(
                content=None,
                tool_calls=(ModelToolCall("repair-tool", "list_affordances", {}),),
            ),
        )
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                model,
                FakeToolFactory(tools),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(plan_request())
        assert tools.tool_calls == []
        assert len(tools.validator_calls) == 1

    run(scenario())


@pytest.mark.parametrize(
    "raw",
    [
        '{"summary":"x","summary":"y","steps":[{"kind":"go_to","x":1,"y":64,"z":0}]}',
        '{"summary":"x","steps":[{"kind":"go_to","x":1,"y":64,"z":0}]} trailing',
        '{"summary":"x","steps":[{"kind":"go_to","x":NaN,"y":64,"z":0}]}',
        '[{"summary":"x","steps":[]}]',
    ],
)
def test_malformed_final_json_never_reaches_validator_or_repair(raw: str) -> None:
    async def scenario() -> None:
        tools = FakeTools()
        model = ScriptedModel(ModelOutput(content=raw, tool_calls=()))
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                model, FakeToolFactory(tools), wall_clock=lambda: 1_700_000_000.0
            ).run(plan_request())
        assert tools.validator_calls == []
        assert len(model.calls) == 1

    run(scenario())


def test_invalid_contract_golden_plan_is_rejected_before_validator() -> None:
    async def scenario() -> None:
        case = golden("mcp-v1", "invalid", "plan rejects unknown top-level field")
        raw = json.dumps(case["value"]["plan"], ensure_ascii=False)
        tools = FakeTools()
        with pytest.raises(InvalidModelOutput):
            await PlannerHarness(
                ScriptedModel(ModelOutput(content=raw, tool_calls=())),
                FakeToolFactory(tools),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(plan_request())
        assert tools.validator_calls == []

    run(scenario())


@pytest.mark.parametrize("field", ["snapshot_digest", "instruction", "companion_id"])
def test_context_identity_mismatch_fails_before_model(field: str) -> None:
    async def scenario() -> None:
        request = plan_request()
        context_value = planning_context(request).model_dump(mode="json")
        if field == "snapshot_digest":
            context_value[field] = "f" * 64
        elif field == "instruction":
            context_value[field] = "另一条指令"
        else:
            context_value["companion"][field] = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
        tools = FakeTools(context=GetPlanningContextResult.model_validate(context_value))
        model = ScriptedModel(ModelOutput(content=plan_json(), tool_calls=()))
        with pytest.raises(PlannerUnavailable):
            await PlannerHarness(
                model, FakeToolFactory(tools), wall_clock=lambda: 1_700_000_000.0
            ).run(request)
        assert model.calls == []
        assert tools.validator_calls == []

    run(scenario())


@pytest.mark.parametrize("mismatch", ["digest", "plan"])
def test_validator_acceptance_must_echo_digest_and_canonical_candidate(mismatch: str) -> None:
    async def scenario() -> None:
        context = planning_context()
        result = accepted_result(context)
        if mismatch == "digest":
            result = result.model_copy(update={"snapshot_digest": "f" * 64})
        else:
            different = Plan(
                summary="不同计划",
                steps=({"kind": "go_to", "x": 1, "y": 64, "z": 0},),
            )
            result = result.model_copy(update={"plan": different})
        tools = FakeTools(context=context, validator_results=[result])
        with pytest.raises(PlannerUnavailable):
            await PlannerHarness(
                ScriptedModel(ModelOutput(content=plan_json(), tool_calls=())),
                FakeToolFactory(tools),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(plan_request())

    run(scenario())


def test_expired_deadline_fails_before_opening_mcp() -> None:
    async def scenario() -> None:
        request = plan_request().model_copy(update={"deadline_unix_ms": 1_699_999_999_999})
        factory = FakeToolFactory(FakeTools())
        with pytest.raises(PlannerDeadlineExceeded):
            await PlannerHarness(ScriptedModel(), factory, wall_clock=lambda: 1_700_000_000.0).run(
                request
            )
        assert factory.opened == 0

    run(scenario())


def test_model_hang_obeys_total_timeout_and_closes_session() -> None:
    async def scenario() -> None:
        factory = FakeToolFactory(FakeTools())
        harness = PlannerHarness(
            HangingModel(),
            factory,
            limits=PlannerLimits(model_calls=3, tool_calls=4, timeout_seconds=0.02),
            wall_clock=lambda: 1_700_000_000.0,
        )
        with pytest.raises(PlannerDeadlineExceeded):
            await harness.run(plan_request())
        assert factory.opened == factory.closed == 1

    run(scenario())


def test_mcp_tool_hang_obeys_same_total_timeout_and_closes_session() -> None:
    async def scenario() -> None:
        tools = FakeTools(hang_tool=True)
        factory = FakeToolFactory(tools)
        harness = PlannerHarness(
            ScriptedModel(
                ModelOutput(
                    content=None,
                    tool_calls=(ModelToolCall("call-1", "list_affordances", {}),),
                )
            ),
            factory,
            limits=PlannerLimits(model_calls=3, tool_calls=4, timeout_seconds=0.02),
            wall_clock=lambda: 1_700_000_000.0,
        )
        with pytest.raises(PlannerDeadlineExceeded):
            await harness.run(plan_request())
        assert tools.tool_started.is_set()
        assert factory.opened == factory.closed == 1

    run(scenario())


def test_dependency_failure_does_not_echo_instruction_or_capability() -> None:
    class FailingModel(PlannerModel):
        async def complete(
            self,
            messages: tuple[PlannerMessage, ...],
            *,
            allow_tools: bool,
        ) -> ModelOutput:
            del messages, allow_tools
            current = plan_request()
            raise RuntimeError(f"{current.instruction} {current.mcp_capability}")

    async def scenario() -> None:
        current = plan_request()
        with pytest.raises(PlannerUnavailable) as captured:
            await PlannerHarness(
                FailingModel(),
                FakeToolFactory(FakeTools()),
                wall_clock=lambda: 1_700_000_000.0,
            ).run(current)
        assert current.instruction not in str(captured.value)
        assert current.mcp_capability not in str(captured.value)

    run(scenario())


class FakeRunnable:
    def __init__(self, *outputs: object) -> None:
        self.outputs = list(outputs)
        self.messages: list[object] = []

    async def ainvoke(self, messages: object) -> object:
        self.messages.append(messages)
        if not self.outputs:
            raise AssertionError("unexpected runnable call")
        output = self.outputs.pop(0)
        if isinstance(output, BaseException):
            raise output
        return output


class FakeChatModel(FakeRunnable):
    def __init__(self, *outputs: object, bound_outputs: tuple[object, ...] = ()) -> None:
        super().__init__(*outputs)
        self.bound = FakeRunnable(*bound_outputs)
        self.bound_tools: list[dict[str, object]] | None = None

    def bind_tools(self, tools: list[dict[str, object]]) -> FakeRunnable:
        self.bound_tools = tools
        return self.bound


class HTTPXTrackingStream(httpx.AsyncByteStream):
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


class ProviderHTTPChatModel(FakeChatModel):
    def __init__(self, client: httpx.AsyncClient) -> None:
        super().__init__()
        self.client = client

    async def ainvoke(self, messages: object) -> object:
        del messages
        await self.client.get("https://provider.example/v1/chat/completions")
        return AIMessage(content=plan_json())


def test_model_adapter_exposes_only_four_local_tools_and_converts_ai_message() -> None:
    async def scenario() -> None:
        tool_message = AIMessage(
            content="",
            tool_calls=[
                {
                    "id": "call-1",
                    "name": "inspect_inventory",
                    "args": {"offset": 0, "limit": 1},
                    "type": "tool_call",
                }
            ],
        )
        final_message = AIMessage(content=plan_json())
        fake = FakeChatModel(final_message, bound_outputs=(tool_message,))
        adapter = ChatOpenAIPlannerModel(cast(Any, fake))
        prompt = (PlannerMessage(role="user", content="facts"),)

        tool_output = await adapter.complete(prompt, allow_tools=True)
        final_output = await adapter.complete(prompt, allow_tools=False)

        assert tool_output == ModelOutput(
            content="",
            tool_calls=(
                ModelToolCall(
                    call_id="call-1",
                    name="inspect_inventory",
                    arguments={"offset": 0, "limit": 1},
                ),
            ),
        )
        assert final_output.content == plan_json()
        assert fake.bound_tools is not None
        functions = [definition["function"] for definition in fake.bound_tools]
        contract_tools = model_visible_contract_tools()
        assert [function["name"] for function in functions] == [
            tool["name"] for tool in contract_tools
        ]
        assert [canonical_json_bytes(function["parameters"]) for function in functions] == [
            canonical_json_bytes(resolved_contract_schema(tool["input_schema"]))
            for tool in contract_tools
        ]
        find_schema = functions[2]["parameters"]
        assert find_schema["properties"]["block_names"]["maxItems"] == 16
        assert (
            find_schema["properties"]["block_names"]["items"]["x-mornlea-rules"][0][
                "max_utf8_bytes"
            ]
            == 64
        )
        assert len(fake.bound.messages) == 1
        assert len(fake.messages) == 1

    run(scenario())


def test_model_tool_binding_cannot_pollute_later_adapter_instances() -> None:
    first = FakeChatModel()
    ChatOpenAIPlannerModel(cast(Any, first))
    assert first.bound_tools is not None
    first_function = first.bound_tools[0]["function"]
    assert isinstance(first_function, dict)
    first_parameters = first_function["parameters"]
    assert isinstance(first_parameters, dict)
    first_parameters["provider_mutation"] = True

    second = FakeChatModel()
    ChatOpenAIPlannerModel(cast(Any, second))
    assert second.bound_tools is not None
    second_function = second.bound_tools[0]["function"]
    assert isinstance(second_function, dict)
    second_parameters = second_function["parameters"]
    assert isinstance(second_parameters, dict)
    assert "provider_mutation" not in second_parameters


def test_model_adapter_preserves_invalid_tool_signal_and_non_string_content() -> None:
    async def scenario() -> None:
        invalid = AIMessage(
            content=[{"type": "text", "text": "not a single string"}],
            invalid_tool_calls=[
                {
                    "id": "bad-call",
                    "name": "inspect_inventory",
                    "args": "{",
                    "error": "invalid JSON",
                    "type": "invalid_tool_call",
                }
            ],
        )
        fake = FakeChatModel(bound_outputs=(invalid,))
        output = await ChatOpenAIPlannerModel(cast(Any, fake)).complete(
            (PlannerMessage(role="user", content="facts"),),
            allow_tools=True,
        )
        assert output.invalid_tool_calls is True
        assert type(output.content) is list

    run(scenario())


def test_model_adapter_sanitizes_provider_failure_and_propagates_cancel() -> None:
    async def scenario() -> None:
        prompt = (PlannerMessage(role="user", content="facts"),)
        failing = ChatOpenAIPlannerModel(cast(Any, FakeChatModel(RuntimeError("provider-secret"))))
        with pytest.raises(PlannerUnavailable) as captured:
            await failing.complete(prompt, allow_tools=False)
        assert "provider-secret" not in str(captured.value)

        cancelled = ChatOpenAIPlannerModel(cast(Any, FakeChatModel(asyncio.CancelledError())))
        with pytest.raises(asyncio.CancelledError):
            await cancelled.complete(prompt, allow_tools=False)

    run(scenario())


def test_model_create_disables_retry_streaming_and_multiple_candidates(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured: dict[str, object] = {}
    fake = FakeChatModel()

    def fake_chat_openai(**kwargs: object) -> object:
        captured.update(kwargs)
        return fake

    monkeypatch.setattr(model_adapter, "ChatOpenAI", fake_chat_openai)
    secret = SecretStr("provider-secret")
    adapter = ChatOpenAIPlannerModel.create(
        base_url="https://provider.example/v1",
        model="planner-model",
        api_key=secret,
    )
    assert captured["max_retries"] == 0
    assert captured["streaming"] is False
    assert captured["n"] == 1
    assert "response_format" not in captured
    assert "provider-secret" not in repr(captured)
    assert "provider-secret" not in repr(adapter)
    run(adapter.aclose())


def test_real_model_create_owns_only_bounded_async_client() -> None:
    async def response(_: httpx.Request) -> httpx.Response:
        raise AssertionError("model construction must not contact provider")

    async def scenario() -> None:
        adapter = ChatOpenAIPlannerModel.create(
            base_url="https://provider.example/v1",
            model="planner-model",
            api_key=SecretStr("provider-secret"),
            transport=httpx.MockTransport(response),
        )
        chat_model = cast(Any, adapter)._chat_model
        client = cast(Any, adapter)._http_client
        assert chat_model.root_client is None
        assert chat_model.client is None
        assert isinstance(client, httpx.AsyncClient)
        assert not client.is_closed
        await adapter.aclose()
        await adapter.aclose()
        assert client.is_closed

    run(scenario())


@pytest.mark.parametrize("mode", ["content_length", "chunked"])
def test_provider_transport_caps_response_before_chat_sdk_buffers(
    mode: str,
) -> None:
    requests: list[httpx.Request] = []
    stream: HTTPXTrackingStream
    if mode == "content_length":
        stream = HTTPXTrackingStream((b"{}",))
        headers = {"content-length": str(PROVIDER_RESPONSE_LIMIT + 1)}
    else:
        stream = HTTPXTrackingStream((b"{}", b" " * (PROVIDER_RESPONSE_LIMIT - 1)))
        headers = {}

    async def response(raw: httpx.Request) -> httpx.Response:
        requests.append(raw)
        return httpx.Response(200, headers=headers, stream=stream)

    async def scenario() -> None:
        adapter = ChatOpenAIPlannerModel.create(
            base_url="https://provider.example/v1",
            model="planner-model",
            api_key=SecretStr("provider-secret"),
            transport=httpx.MockTransport(response),
        )
        try:
            with pytest.raises(PlannerUnavailable):
                await adapter.complete(
                    (PlannerMessage(role="user", content="facts"),),
                    allow_tools=False,
                )
        finally:
            await adapter.aclose()
        assert stream.closed
        if mode == "content_length":
            assert stream.bytes_yielded == 0
        else:
            assert stream.bytes_yielded <= PROVIDER_RESPONSE_LIMIT + 1
        assert len(requests) == 1
        assert requests[0].headers["accept-encoding"] == "identity"

    run(scenario())


def test_provider_transport_accepts_exact_body_limit(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    stream = HTTPXTrackingStream((b" " * PROVIDER_RESPONSE_LIMIT,))

    async def response(_: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            headers={"content-length": str(PROVIDER_RESPONSE_LIMIT)},
            stream=stream,
        )

    def fake_chat_openai(**kwargs: object) -> object:
        client = kwargs["http_async_client"]
        assert isinstance(client, httpx.AsyncClient)
        return ProviderHTTPChatModel(client)

    async def scenario() -> None:
        adapter = ChatOpenAIPlannerModel.create(
            base_url="https://provider.example/v1",
            model="planner-model",
            api_key=SecretStr("provider-secret"),
            transport=httpx.MockTransport(response),
        )
        try:
            output = await adapter.complete(
                (PlannerMessage(role="user", content="facts"),),
                allow_tools=False,
            )
        finally:
            await adapter.aclose()
        assert output.content == plan_json()
        assert stream.bytes_yielded == PROVIDER_RESPONSE_LIMIT
        assert stream.closed

    monkeypatch.setattr(model_adapter, "ChatOpenAI", fake_chat_openai)
    run(scenario())


def test_explicit_cancellation_is_not_swallowed_and_closes_session() -> None:
    async def scenario() -> None:
        factory = FakeToolFactory(FakeTools())
        model = HangingModel()
        harness = PlannerHarness(model, factory, wall_clock=lambda: 1_700_000_000.0)
        task = asyncio.create_task(harness.run(plan_request()))
        await model.started.wait()
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
        assert factory.closed == 1

    run(scenario())

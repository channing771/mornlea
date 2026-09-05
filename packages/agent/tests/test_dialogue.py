from __future__ import annotations

import asyncio
import json
from collections.abc import Awaitable, Callable
from copy import deepcopy
from pathlib import Path

import httpx
import pytest
from harness.agents.companion.dialogue_factory import DialogueHarness
from harness.domain.dialogue import (
    DialogueDeadlineExceeded,
    DialogueLimits,
    DialogueMessage,
    DialogueUnavailable,
    InvalidDialogueOutput,
)
from harness.domain.http_v1 import (
    DialogueNonterminalRequest,
    DialogueNonterminalResponse,
    DialogueTerminalRequest,
    DialogueTerminalResponse,
)
from harness.domain.memory import (
    LeaseIdentity,
    MemoryLookup,
    MemoryStateNonzero,
)
from harness.models import chat_openai as model_adapter
from harness.models.chat_openai import (
    ChatOpenAIDialogueModel,
    ChatOpenAIModelAdapters,
)
from langchain_core.language_models import BaseChatModel
from langchain_core.messages import AIMessage, BaseMessage
from langchain_core.outputs import ChatGeneration, ChatResult
from pydantic import SecretStr

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
HTTP_GOLDEN = REPOSITORY_ROOT / "packages/contracts/companion-agent/http-v1/golden/valid.json"


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def golden(name: str) -> dict[str, object]:
    document = json.loads(HTTP_GOLDEN.read_text(encoding="utf-8"))
    return deepcopy(next(case["value"] for case in document["cases"] if case["name"] == name))


class FakeMemoryReader:
    def __init__(self, state: MemoryStateNonzero) -> None:
        self.state = state
        self.calls: list[tuple[LeaseIdentity, MemoryLookup]] = []

    async def load(
        self,
        identity: LeaseIdentity,
        lookup: MemoryLookup,
    ) -> MemoryStateNonzero:
        self.calls.append((identity, lookup))
        return self.state


class FakeDialogueModel:
    def __init__(self, output: object) -> None:
        self.output = output
        self.calls: list[tuple[DialogueMessage, ...]] = []

    async def complete(self, messages: tuple[DialogueMessage, ...]) -> object:
        self.calls.append(messages)
        return self.output


class BlockingDialogueModel(FakeDialogueModel):
    def __init__(self) -> None:
        super().__init__(None)
        self.started = asyncio.Event()
        self.cancelled = asyncio.Event()

    async def complete(self, messages: tuple[DialogueMessage, ...]) -> object:
        self.calls.append(messages)
        self.started.set()
        try:
            await asyncio.Future()
        except asyncio.CancelledError:
            self.cancelled.set()
            raise


def memory() -> MemoryStateNonzero:
    return MemoryStateNonzero(
        revision=3,
        operation_id="88888888-8888-4888-8888-888888888888",
        summary="Python runtime summary only",
    )


def nonterminal_request() -> DialogueNonterminalRequest:
    return DialogueNonterminalRequest.model_validate(golden("nonterminal dialogue run"))


def terminal_request() -> DialogueTerminalRequest:
    return DialogueTerminalRequest.model_validate(
        golden("terminal dialogue request carries completed fact")
    )


def test_nonterminal_prompt_contains_exactly_four_runtime_categories() -> None:
    async def scenario() -> None:
        model = FakeDialogueModel('{"line":"继续前进。"}')
        reader = FakeMemoryReader(memory())
        harness = DialogueHarness(model, reader, wall_clock=lambda: 1_700_000_000.0)

        response = await harness.run(nonterminal_request())

        assert isinstance(response, DialogueNonterminalResponse)
        assert response.line == "继续前进。"
        assert len(model.calls) == 1
        assert [message.role for message in model.calls[0]] == ["system", "user"]
        user = json.loads(model.calls[0][1].content)
        assert set(user) == {"environment", "fact_node", "persona", "summary"}
        assert user["summary"] == memory().summary
        serialized = model.calls[0][1].content
        for forbidden in (
            response.request_id,
            response.client_instance_id,
            response.namespace_id,
            response.lease_id,
            response.run_id,
            response.companion_id,
            str(response.generation),
            str(response.memory_epoch),
            "deadline_unix_ms",
            "operation_id",
            "revision",
            "mcp",
            "planner",
        ):
            assert forbidden.lower() not in serialized.lower()
        assert reader.calls == [
            (
                LeaseIdentity(
                    namespace_id=response.namespace_id,
                    client_instance_id=response.client_instance_id,
                    lease_id=response.lease_id,
                ),
                MemoryLookup(
                    namespace_id=response.namespace_id,
                    companion_id=response.companion_id,
                    memory_epoch=response.memory_epoch,
                ),
            )
        ]

    run(scenario())


def test_terminal_response_uses_runtime_operation_and_does_not_commit() -> None:
    async def scenario() -> None:
        model = FakeDialogueModel('{"line":"完成了。","summary":"新的终态摘要"}')
        reader = FakeMemoryReader(memory())
        operation = "99999999-9999-4999-8999-999999999999"
        harness = DialogueHarness(
            model,
            reader,
            wall_clock=lambda: 1_700_000_000.0,
            operation_id_factory=lambda: operation,
        )

        response = await harness.run(terminal_request())

        assert isinstance(response, DialogueTerminalResponse)
        assert response.line == "完成了。"
        assert response.memory_proposal.operation_id == operation
        assert response.memory_proposal.base_revision == memory().revision
        assert response.memory_proposal.summary == "新的终态摘要"
        assert len(reader.calls) == 1
        assert not hasattr(reader, "commit")

    run(scenario())


@pytest.mark.parametrize(
    ("request_factory", "output"),
    [
        (nonterminal_request, '{"line":"ok","summary":"forbidden"}'),
        (terminal_request, '{"line":"ok"}'),
        (terminal_request, '{"line":"ok","summary":"x","extra":1}'),
        (nonterminal_request, '{"line":" ok"}'),
        (nonterminal_request, '{"line":"ok","line":"duplicate"}'),
        (nonterminal_request, '{"line":"ok"} trailing'),
        (nonterminal_request, '{"line":NaN}'),
        (nonterminal_request, "[" * 2_000 + "]" * 2_000),
        (nonterminal_request, ["not", "text"]),
        (nonterminal_request, " " * 65_537),
    ],
)
def test_dialogue_output_is_strict_and_bounded(
    request_factory: Callable[[], DialogueNonterminalRequest | DialogueTerminalRequest],
    output: object,
) -> None:
    async def scenario() -> None:
        model = FakeDialogueModel(output)
        harness = DialogueHarness(model, FakeMemoryReader(memory()), wall_clock=lambda: 1.0)
        with pytest.raises(InvalidDialogueOutput):
            await harness.run(request_factory())
        assert len(model.calls) == 1

    run(scenario())


def test_deadline_and_cancellation_do_not_retry_model() -> None:
    async def scenario() -> None:
        expired = DialogueHarness(
            FakeDialogueModel('{"line":"unused"}'),
            FakeMemoryReader(memory()),
            wall_clock=lambda: 2_000_000_000.0,
        )
        with pytest.raises(DialogueDeadlineExceeded):
            await expired.run(nonterminal_request())

        blocking = BlockingDialogueModel()
        harness = DialogueHarness(
            blocking,
            FakeMemoryReader(memory()),
            limits=DialogueLimits(timeout_seconds=60),
            wall_clock=lambda: 1_700_000_000.0,
        )
        task = asyncio.create_task(harness.run(nonterminal_request()))
        await blocking.started.wait()
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
        assert blocking.cancelled.is_set()
        assert len(blocking.calls) == 1

    run(scenario())


def test_provider_failure_is_sanitized_without_retry() -> None:
    class FailingModel(FakeDialogueModel):
        async def complete(self, messages: tuple[DialogueMessage, ...]) -> object:
            self.calls.append(messages)
            raise RuntimeError("provider body SECRET_RESPONSE")

    async def scenario() -> None:
        model = FailingModel(None)
        harness = DialogueHarness(model, FakeMemoryReader(memory()), wall_clock=lambda: 1.0)
        with pytest.raises(DialogueUnavailable) as captured:
            await harness.run(nonterminal_request())
        assert "SECRET_RESPONSE" not in str(captured.value)
        assert len(model.calls) == 1

    run(scenario())


class StubChatModel(BaseChatModel):
    response: AIMessage
    calls: int = 0

    @property
    def _llm_type(self) -> str:
        return "stub-dialogue"

    def _generate(
        self,
        messages: list[BaseMessage],
        stop: list[str] | None = None,
        run_manager: object | None = None,
        **kwargs: object,
    ) -> ChatResult:
        del messages, stop, run_manager, kwargs
        self.calls += 1
        return ChatResult(generations=[ChatGeneration(message=self.response)])


@pytest.mark.parametrize(
    "response",
    [
        AIMessage(content=[{"type": "text", "text": "list content"}]),
        AIMessage(
            content='{"line":"x"}',
            tool_calls=[{"id": "call-1", "name": "list_affordances", "args": {}}],
        ),
        AIMessage(content='{"line":"x"}', additional_kwargs={"function_call": {"name": "x"}}),
    ],
)
def test_dialogue_adapter_rejects_tools_legacy_calls_and_list_content(
    response: AIMessage,
) -> None:
    async def scenario() -> None:
        chat = StubChatModel(response=response)
        adapter = ChatOpenAIDialogueModel(chat)
        with pytest.raises(InvalidDialogueOutput):
            await adapter.complete((DialogueMessage(role="user", content="{}"),))
        assert chat.calls == 1

    run(scenario())


def test_dialogue_adapter_uses_raw_model_without_binding_tools() -> None:
    async def scenario() -> None:
        chat = StubChatModel(response=AIMessage(content='{"line":"ok"}'))
        adapter = ChatOpenAIDialogueModel(chat)
        output = await adapter.complete(
            (
                DialogueMessage(role="system", content="fixed"),
                DialogueMessage(role="user", content="{}"),
            )
        )
        assert output == '{"line":"ok"}'
        assert chat.calls == 1

    run(scenario())


def test_shared_model_factory_closes_client_when_model_construction_fails(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    clients: list[object] = []

    class TrackingTransport(httpx.AsyncBaseTransport):
        def __init__(self) -> None:
            self.closed = False

        async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
            del request
            raise AssertionError("constructor must not issue requests")

        async def aclose(self) -> None:
            self.closed = True

    class TrackingClient:
        def __init__(self, **kwargs: object) -> None:
            self.transport = kwargs["transport"]
            self.closed = False
            clients.append(self)

        async def aclose(self) -> None:
            self.closed = True
            await self.transport.aclose()

    def fail_model(**kwargs: object) -> object:
        del kwargs
        raise RuntimeError("provider constructor secret")

    class FailingBindModel:
        def bind_tools(self, tools: object) -> object:
            del tools
            raise RuntimeError("tool binding secret")

    async def scenario() -> None:
        transport = TrackingTransport()
        monkeypatch.setattr(model_adapter.httpx, "AsyncClient", TrackingClient)
        monkeypatch.setattr(model_adapter, "ChatOpenAI", fail_model)
        with pytest.raises(RuntimeError):
            await ChatOpenAIModelAdapters.create(
                base_url="https://provider.example/v1",
                model="dialogue-model",
                api_key=SecretStr("provider-secret"),
                transport=transport,
            )
        assert len(clients) == 1
        assert clients[0].closed is True
        assert transport.closed is True

        second_transport = TrackingTransport()
        monkeypatch.setattr(
            model_adapter,
            "ChatOpenAI",
            lambda **kwargs: FailingBindModel(),
        )
        with pytest.raises(RuntimeError):
            await ChatOpenAIModelAdapters.create(
                base_url="https://provider.example/v1",
                model="dialogue-model",
                api_key=SecretStr("provider-secret"),
                transport=second_transport,
            )
        assert len(clients) == 2
        assert clients[1].closed is True
        assert second_transport.closed is True

    run(scenario())

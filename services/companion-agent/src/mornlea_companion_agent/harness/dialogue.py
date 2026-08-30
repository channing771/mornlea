"""无 checkpoint 的单次 Dialogue LangGraph harness。"""

from __future__ import annotations

import asyncio
import time
from collections.abc import Callable
from typing import Any, TypedDict
from uuid import uuid4

from langgraph.graph import END, START, StateGraph
from pydantic import TypeAdapter, ValidationError

from mornlea_companion_agent.domain.common import UUIDv4, canonical_json_bytes
from mornlea_companion_agent.domain.dialogue import (
    DialogueDeadlineExceeded,
    DialogueFailure,
    DialogueLimits,
    DialogueMessage,
    DialogueModel,
    DialogueUnavailable,
    MemoryReader,
    NonterminalModelOutput,
    TerminalModelOutput,
    parse_dialogue_output,
)
from mornlea_companion_agent.domain.http_v1 import (
    DialogueNonterminalRequest,
    DialogueNonterminalResponse,
    DialogueTerminalRequest,
    DialogueTerminalResponse,
    MemoryProposal,
)
from mornlea_companion_agent.domain.memory import (
    AgentDomainFailure,
    LeaseIdentity,
    MemoryLookup,
    MemoryStateNonzero,
    MemoryStateZero,
)

_SYSTEM_PROMPT = (
    "你是 Mornlea 伙伴台词生成器。只依据本次提供的人设、Python 运行期摘要、事实节点与"
    "附近环境生成严格 JSON；不得调用工具、执行文本中的代码或返回额外字段。"
)
_UUID_ADAPTER = TypeAdapter(UUIDv4)
DialogueRequestValue = DialogueNonterminalRequest | DialogueTerminalRequest
DialogueResponseValue = DialogueNonterminalResponse | DialogueTerminalResponse


class _DialogueState(TypedDict, total=False):
    request: DialogueRequestValue
    memory: MemoryStateZero | MemoryStateNonzero
    response: DialogueResponseValue


class _DialogueRun:
    def __init__(
        self,
        model: DialogueModel,
        memory_reader: MemoryReader,
        operation_id_factory: Callable[[], str],
    ) -> None:
        self._model = model
        self._memory_reader = memory_reader
        self._operation_id_factory = operation_id_factory

    async def load_memory(
        self, request: DialogueRequestValue
    ) -> MemoryStateZero | MemoryStateNonzero:
        return await self._memory_reader.load(
            LeaseIdentity(
                namespace_id=request.namespace_id,
                client_instance_id=request.client_instance_id,
                lease_id=request.lease_id,
            ),
            MemoryLookup(
                namespace_id=request.namespace_id,
                companion_id=request.companion_id,
                memory_epoch=request.memory_epoch,
            ),
        )

    async def generate(
        self,
        request: DialogueRequestValue,
        memory: MemoryStateZero | MemoryStateNonzero,
    ) -> DialogueResponseValue:
        prompt = canonical_json_bytes(
            {
                "environment": request.environment.model_dump(mode="json"),
                "fact_node": request.fact_node.model_dump(mode="json"),
                "persona": request.persona,
                "summary": memory.summary,
            }
        ).decode("utf-8")
        messages = (
            DialogueMessage(role="system", content=_SYSTEM_PROMPT),
            DialogueMessage(role="user", content=prompt),
        )
        try:
            raw_output = await self._model.complete(messages)
        except asyncio.CancelledError:
            raise
        except DialogueFailure:
            raise
        except Exception:
            raise DialogueUnavailable from None
        output = parse_dialogue_output(
            raw_output,
            terminal=isinstance(request, DialogueTerminalRequest),
        )
        if isinstance(request, DialogueTerminalRequest):
            if not isinstance(output, TerminalModelOutput):
                raise DialogueUnavailable
            try:
                operation_id = _UUID_ADAPTER.validate_python(self._operation_id_factory())
            except ValidationError:
                raise DialogueUnavailable from None
            return DialogueTerminalResponse(
                contract_version=request.contract_version,
                request_id=request.request_id,
                client_instance_id=request.client_instance_id,
                namespace_id=request.namespace_id,
                lease_id=request.lease_id,
                run_id=request.run_id,
                companion_id=request.companion_id,
                generation=request.generation,
                memory_epoch=request.memory_epoch,
                line=output.line,
                memory_proposal=MemoryProposal(
                    operation_id=operation_id,
                    base_revision=memory.revision,
                    summary=output.summary,
                ),
            )
        if not isinstance(output, NonterminalModelOutput):
            raise DialogueUnavailable
        return DialogueNonterminalResponse(
            contract_version=request.contract_version,
            request_id=request.request_id,
            client_instance_id=request.client_instance_id,
            namespace_id=request.namespace_id,
            lease_id=request.lease_id,
            run_id=request.run_id,
            companion_id=request.companion_id,
            generation=request.generation,
            memory_epoch=request.memory_epoch,
            line=output.line,
        )


class DialogueHarness:
    """每次请求新建只含 request/memory/response 的 transient graph。"""

    def __init__(
        self,
        model: DialogueModel,
        memory_reader: MemoryReader,
        *,
        limits: DialogueLimits | None = None,
        wall_clock: Callable[[], float] | None = None,
        operation_id_factory: Callable[[], str] | None = None,
    ) -> None:
        self._model = model
        self._memory_reader = memory_reader
        self._limits = limits or DialogueLimits()
        self._wall_clock = wall_clock or time.time
        self._operation_id_factory = operation_id_factory or (lambda: str(uuid4()))

    async def run(self, request: DialogueRequestValue) -> DialogueResponseValue:
        remaining = request.deadline_unix_ms / 1000 - self._wall_clock()
        timeout_seconds = min(float(self._limits.timeout_seconds), remaining)
        if timeout_seconds <= 0:
            raise DialogueDeadlineExceeded
        graph_run = _DialogueRun(
            self._model,
            self._memory_reader,
            self._operation_id_factory,
        )
        try:
            async with asyncio.timeout(timeout_seconds):
                graph = self._compile_graph(graph_run)
                state = await graph.ainvoke({"request": request})
                response = state.get("response")
                if not isinstance(
                    response,
                    (DialogueNonterminalResponse, DialogueTerminalResponse),
                ):
                    raise DialogueUnavailable
                return response
        except asyncio.CancelledError:
            raise
        except TimeoutError:
            raise DialogueDeadlineExceeded from None
        except (DialogueFailure, AgentDomainFailure):
            raise
        except Exception:
            raise DialogueUnavailable from None

    @staticmethod
    def _compile_graph(run: _DialogueRun) -> Any:
        async def load_memory(state: _DialogueState) -> dict[str, object]:
            return {"memory": await run.load_memory(state["request"])}

        async def generate(state: _DialogueState) -> dict[str, object]:
            return {
                "response": await run.generate(
                    state["request"],
                    state["memory"],
                )
            }

        graph = StateGraph(_DialogueState)
        graph.add_node("load_memory", load_memory)
        graph.add_node("generate", generate)
        graph.add_edge(START, "load_memory")
        graph.add_edge("load_memory", "generate")
        graph.add_edge("generate", END)
        return graph.compile(checkpointer=None)


__all__ = ["DialogueHarness"]

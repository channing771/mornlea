"""网关运行期：组件装配、租约门禁下的计划与台词执行、优雅停机。"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator, Awaitable, Callable, Coroutine
from contextlib import asynccontextmanager
from dataclasses import dataclass
from typing import Any, Protocol, cast

from harness.agents.companion import DialogueHarness, PlannerHarness
from harness.config import AgentConfig
from harness.domain.dialogue import DialogueLimits
from harness.domain.http_v1 import (
    DialogueNonterminalRequest,
    DialogueNonterminalResponse,
    DialogueTerminalRequest,
    DialogueTerminalResponse,
    LeaseRequest,
    PlanRequest,
    PlanResponse,
)
from harness.domain.memory import LeaseIdentity
from harness.domain.planner import PlannerLimits
from harness.models.chat_openai import ChatOpenAIModelAdapters
from harness.runtime.leases import HEARTBEAT_INTERVAL_MS, NamespaceLeaseManager, RunKind
from harness.store.sqlite_memory import SQLiteMemoryStore
from harness.tools.mcp_session import MCPToolSessionFactory
from pydantic import BaseModel, SecretStr

from app.gateway.http_gate import _drain_cleanup, _HTTPFailure


class PlannerRunner(Protocol):
    async def run(self, request: PlanRequest) -> PlanResponse: ...


class DialogueRunner(Protocol):
    async def run(
        self,
        request: DialogueNonterminalRequest | DialogueTerminalRequest,
    ) -> DialogueNonterminalResponse | DialogueTerminalResponse: ...


class AsyncCloser(Protocol):
    async def aclose(self) -> None: ...


@dataclass(frozen=True, slots=True)
class AppComponents:
    store: SQLiteMemoryStore
    planner: PlannerRunner
    dialogue: DialogueRunner
    model_owner: AsyncCloser


ComponentFactory = Callable[
    [AgentConfig, SecretStr],
    Awaitable[AppComponents],
]


async def _close_unowned_components(
    components: AppComponents,
) -> asyncio.CancelledError | None:
    cancellation: asyncio.CancelledError | None = None
    for close in (components.model_owner.aclose, components.store.close):
        cleanup_error = await _drain_cleanup(close())
        if isinstance(cleanup_error, asyncio.CancelledError) and cancellation is None:
            cancellation = cleanup_error
    return cancellation


class _AgentRuntime:
    def __init__(self) -> None:
        self.components: AppComponents | None = None
        self.leases: NamespaceLeaseManager | None = None
        self.accepting = False
        self.ready = False
        self.bootstrap_failed = False
        self._state_lock = asyncio.Lock()
        self._business_tasks: set[asyncio.Task[object]] = set()
        self._expiry_task: asyncio.Task[None] | None = None

    async def start(self, components: AppComponents) -> None:
        leases = NamespaceLeaseManager(components.store)
        async with self._state_lock:
            expiry = asyncio.create_task(
                self._expire_loop(),
                name="companion-agent-lease-expiry",
            )
            self.components = components
            self.leases = leases
            self.accepting = True
            self.ready = True
            self.bootstrap_failed = False
            self._expiry_task = expiry

    @property
    def is_ready(self) -> bool:
        return self.ready and self.accepting and self.components is not None

    @asynccontextmanager
    async def business(self) -> AsyncIterator[AppComponents]:
        current = asyncio.current_task()
        if current is None:
            raise _HTTPFailure("internal_error")
        task = cast(asyncio.Task[object], current)
        async with self._state_lock:
            components = self.components
            if not self.accepting or components is None:
                code = "internal_error" if self.bootstrap_failed else "agent_unavailable"
                raise _HTTPFailure(code)
            self._business_tasks.add(task)
        try:
            yield components
        finally:
            async with self._state_lock:
                self._business_tasks.discard(task)

    async def _expire_loop(self) -> None:
        try:
            while True:
                await asyncio.sleep(HEARTBEAT_INTERVAL_MS / 1000)
                leases = self.leases
                if leases is None:
                    return
                await leases.expire_stale()
        except asyncio.CancelledError:
            raise
        except Exception:
            async with self._state_lock:
                self.ready = False
                self.accepting = False
            leases = self.leases
            if leases is not None:
                await leases.run_gate.cancel_all()

    async def run_planner(self, request: PlanRequest) -> PlanResponse:
        async with self.business() as components:
            result = await self._run(
                request,
                kind=RunKind.PLANNER,
                operation=lambda: components.planner.run(request),
            )
            if not isinstance(result, PlanResponse):
                raise _HTTPFailure("internal_error")
            _assert_response_identity(request, result)
            return result

    async def run_dialogue(
        self,
        request: DialogueNonterminalRequest | DialogueTerminalRequest,
    ) -> DialogueNonterminalResponse | DialogueTerminalResponse:
        async with self.business() as components:
            result = await self._run(
                request,
                kind=RunKind.DIALOGUE,
                operation=lambda: components.dialogue.run(request),
            )
            if not isinstance(
                result,
                (DialogueNonterminalResponse, DialogueTerminalResponse),
            ):
                raise _HTTPFailure("internal_error")
            _assert_response_identity(request, result)
            return result

    async def _run(
        self,
        request: PlanRequest | DialogueNonterminalRequest | DialogueTerminalRequest,
        *,
        kind: RunKind,
        operation: Callable[[], Coroutine[Any, Any, object]],
    ) -> object:
        leases = self.leases
        if leases is None:
            raise _HTTPFailure("internal_error")
        identity = _lease_identity(request)
        handle = await leases.reserve_run(
            identity,
            companion_id=request.companion_id,
            run_id=request.run_id,
            kind=kind,
        )
        child: asyncio.Task[object] | None = None
        try:
            async with self._state_lock:
                accepting = self.accepting
            if not accepting:
                raise _HTTPFailure("agent_unavailable")
            child = asyncio.create_task(operation())
            await handle.bind_task(child)
            try:
                result = await child
            except asyncio.CancelledError:
                if handle.cancelled:
                    await leases.assert_current(identity)
                    raise _HTTPFailure("agent_unavailable") from None
                raise
            handle.ensure_running()
            await leases.assert_current(identity)
            handle.ensure_running()
            return result
        finally:

            async def cleanup() -> None:
                if child is not None:
                    if not child.done():
                        child.cancel()
                    await asyncio.gather(child, return_exceptions=True)
                await handle.finish()

            cleanup_error = await _drain_cleanup(cleanup())
            if cleanup_error is not None:
                raise cleanup_error

    async def shutdown(self) -> None:
        caller = asyncio.current_task()
        cleanup_error = await _drain_cleanup(self._shutdown_pipeline(caller))
        if cleanup_error is not None:
            raise cleanup_error

    async def _shutdown_pipeline(self, caller: asyncio.Task[Any] | None) -> None:
        cancellation: asyncio.CancelledError | None = None

        async def drain(awaitable: Awaitable[object]) -> None:
            nonlocal cancellation
            cleanup_error = await _drain_cleanup(awaitable)
            if isinstance(cleanup_error, asyncio.CancelledError) and cancellation is None:
                cancellation = cleanup_error

        try:
            async with self._state_lock:
                self.accepting = False
                self.ready = False
                expiry = self._expiry_task
                self._expiry_task = None
            if expiry is not None:
                expiry.cancel()
                await drain(asyncio.gather(expiry, return_exceptions=True))
            leases = self.leases
            if leases is not None:
                await drain(leases.run_gate.cancel_all())
            while True:
                async with self._state_lock:
                    pending = tuple(
                        task
                        for task in self._business_tasks
                        if task is not caller and not task.done()
                    )
                if not pending:
                    break
                await drain(asyncio.gather(*pending, return_exceptions=True))
            components = self.components
            if components is not None:
                await drain(components.model_owner.aclose())
                await drain(components.store.close())
        finally:
            self.components = None
            self.leases = None
        if cancellation is not None:
            raise cancellation


def _lease_identity(request: LeaseRequest) -> LeaseIdentity:
    return LeaseIdentity(
        namespace_id=request.namespace_id,
        client_instance_id=request.client_instance_id,
        lease_id=request.lease_id,
    )


async def _default_component_factory(
    config: AgentConfig,
    provider_api_key: SecretStr,
) -> AppComponents:
    store: SQLiteMemoryStore | None = None
    models: ChatOpenAIModelAdapters | None = None
    try:
        store = await SQLiteMemoryStore.open(config.storage.sqlite_path)
        models = await ChatOpenAIModelAdapters.create(
            base_url=config.provider.base_url,
            model=config.provider.model,
            api_key=provider_api_key,
        )
        planner = PlannerHarness(
            models.planner,
            MCPToolSessionFactory(),
            limits=PlannerLimits(
                model_calls=config.limits.model_calls,
                tool_calls=config.limits.tool_calls,
                timeout_seconds=config.limits.timeout_seconds,
            ),
        )
        dialogue = DialogueHarness(
            models.dialogue,
            store,
            limits=DialogueLimits(timeout_seconds=config.limits.timeout_seconds),
        )
        return AppComponents(
            store=store,
            planner=planner,
            dialogue=dialogue,
            model_owner=models,
        )
    except BaseException:
        if models is not None:
            await _drain_cleanup(models.aclose())
        if store is not None:
            await _drain_cleanup(store.close())
        raise


def _assert_response_identity(request: BaseModel, response: BaseModel) -> None:
    fields: tuple[str, ...] = (
        "contract_version",
        "request_id",
        "client_instance_id",
        "namespace_id",
        "lease_id",
        "run_id",
        "companion_id",
        "generation",
    )
    if isinstance(request, PlanRequest):
        fields += ("snapshot_id", "snapshot_digest")
    else:
        fields += ("memory_epoch",)
        if isinstance(request, DialogueTerminalRequest) != isinstance(
            response, DialogueTerminalResponse
        ):
            raise _HTTPFailure("internal_error")
    if any(getattr(request, field) != getattr(response, field) for field in fields):
        raise _HTTPFailure("internal_error")


__all__ = [
    "AppComponents",
    "AsyncCloser",
    "ComponentFactory",
    "DialogueRunner",
    "PlannerRunner",
]

"""伙伴 Agent 的固定 HTTP v1 application、生命周期与 Uvicorn 入口。"""

from __future__ import annotations

import asyncio
import hashlib
import hmac
import json
import math
from collections.abc import AsyncIterator, Awaitable, Callable, Coroutine
from contextlib import asynccontextmanager
from dataclasses import dataclass
from ipaddress import IPv4Address, IPv6Address, ip_address
from typing import Any, Protocol, cast

import uvicorn
from fastapi import FastAPI, Request, Response
from pydantic import BaseModel, SecretStr, TypeAdapter, ValidationError
from starlette.types import ASGIApp, Message, Receive, Scope, Send

from mornlea_companion_agent.adapters.mcp import MCPToolSessionFactory
from mornlea_companion_agent.adapters.model import ChatOpenAIModelAdapters
from mornlea_companion_agent.config import AgentConfig, ResolvedSecrets
from mornlea_companion_agent.domain import adapter_for
from mornlea_companion_agent.domain.common import UUIDv4, canonical_json_bytes
from mornlea_companion_agent.domain.dialogue import DialogueFailure, DialogueLimits
from mornlea_companion_agent.domain.http_v1 import (
    AcquireRequest,
    AcquireResponse,
    CancelRequest,
    CancelResponse,
    DialogueNonterminalRequest,
    DialogueNonterminalResponse,
    DialogueTerminalRequest,
    DialogueTerminalResponse,
    ErrorDetail,
    ErrorResponse,
    HeartbeatResponse,
    LeaseRequest,
    LiveResponse,
    MemoryCommitRequest,
    MemoryCommitResponse,
    MemoryDeleteRequest,
    MemoryDeleteResponse,
    MemoryReconcileActiveRequest,
    MemoryReconcileActiveResponse,
    MemoryReconcileInactiveRequest,
    MemoryReconcileInactiveResponse,
    PlanRequest,
    PlanResponse,
    ReadyResponse,
    ReleaseResponse,
)
from mornlea_companion_agent.domain.memory import (
    AgentDomainFailure,
    LeaseIdentity,
    MemoryCommit,
    MemoryDelete,
    MemoryReconcile,
)
from mornlea_companion_agent.domain.planner import PlannerFailure, PlannerLimits
from mornlea_companion_agent.harness.dialogue import DialogueHarness
from mornlea_companion_agent.harness.leases import (
    HEARTBEAT_INTERVAL_MS,
    NamespaceLeaseManager,
    RunKind,
)
from mornlea_companion_agent.harness.planner import PlannerHarness
from mornlea_companion_agent.storage.sqlite_memory import SQLiteMemoryStore

HEADER_BYTES_LIMIT = 16_384
REQUEST_BODY_BYTES_LIMIT = 262_144
RESPONSE_BODY_BYTES_LIMIT = 65_536

_UUID_ADAPTER = TypeAdapter(UUIDv4)
_ERROR_STATUSES = {
    "invalid_request": 400,
    "unauthorized": 401,
    "unsupported_version": 426,
    "namespace_conflict": 409,
    "overloaded": 429,
    "deadline_exceeded": 504,
    "agent_unavailable": 503,
    "invalid_model_output": 422,
    "memory_conflict": 409,
    "not_found": 404,
    "internal_error": 500,
}


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


class _HTTPFailure(Exception):
    def __init__(self, code: str) -> None:
        self.code = code
        super().__init__(code)


@dataclass(frozen=True, slots=True)
class _RouteContract:
    method: str
    path: str
    authenticated: bool
    request_schema: str | None


_ROUTES = (
    _RouteContract("GET", "/livez", False, None),
    _RouteContract("GET", "/readyz", True, None),
    _RouteContract("POST", "/v1/namespaces/acquire", True, "acquire_request"),
    _RouteContract("POST", "/v1/namespaces/heartbeat", True, "lease_request"),
    _RouteContract("POST", "/v1/namespaces/release", True, "lease_request"),
    _RouteContract("POST", "/v1/plan", True, "plan_request"),
    _RouteContract("POST", "/v1/dialogue", True, "dialogue_request"),
    _RouteContract("POST", "/v1/memory/reconcile", True, "memory_reconcile_request"),
    _RouteContract("POST", "/v1/memory/commit", True, "memory_commit_request"),
    _RouteContract("POST", "/v1/memory/delete", True, "memory_delete_request"),
    _RouteContract("POST", "/v1/runs/cancel", True, "cancel_request"),
)
_ROUTE_BY_KEY = {(route.method, route.path): route for route in _ROUTES}
_RUN_REQUEST_SCHEMAS = frozenset({"plan_request", "dialogue_request"})
_MAX_REQUEST_LINE_BYTES = max(
    len(f"{route.method} {route.path} HTTP/1.1\r\n".encode("ascii")) for route in _ROUTES
)
H11_INCOMPLETE_EVENT_BYTES_LIMIT = HEADER_BYTES_LIMIT + _MAX_REQUEST_LINE_BYTES + len(b"\r\n")


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def _reject_constant(value: str) -> object:
    del value
    raise ValueError("non-finite JSON number")


def _strict_json_object(body: bytes) -> dict[str, object]:
    try:
        text = body.decode("utf-8", errors="strict")
        value = json.loads(
            text,
            object_pairs_hook=_unique_object,
            parse_constant=_reject_constant,
        )
        if type(value) is not dict:
            raise ValueError("JSON root must be an object")
        pending: list[object] = [value]
        while pending:
            current = pending.pop()
            if type(current) is str:
                current.encode("utf-8", errors="strict")
            elif type(current) is dict:
                for key, item in current.items():
                    key.encode("utf-8", errors="strict")
                    pending.append(item)
            elif type(current) is list:
                pending.extend(current)
            elif type(current) is float and not math.isfinite(current):
                raise ValueError("non-finite JSON number")
        return value
    except Exception:
        raise _HTTPFailure("invalid_request") from None


def _header_values(headers: list[tuple[bytes, bytes]], name: bytes) -> list[bytes]:
    return [value for key, value in headers if key.lower() == name]


def _parse_host(value: bytes) -> tuple[IPv4Address | IPv6Address, int]:
    try:
        text = value.decode("ascii", errors="strict")
        if text.startswith("["):
            closing = text.find("]")
            if closing <= 1 or text[closing + 1 : closing + 2] != ":":
                raise ValueError
            host_text = text[1:closing]
            port_text = text[closing + 2 :]
        else:
            host_text, separator, port_text = text.rpartition(":")
            if not separator or ":" in host_text:
                raise ValueError
        if not port_text or not port_text.isascii() or not port_text.isdecimal():
            raise ValueError
        port = int(port_text)
        if str(port) != port_text or not 1 <= port <= 65_535 or "%" in host_text:
            raise ValueError
        address = ip_address(host_text)
        if not address.is_loopback:
            raise ValueError
        return address, port
    except (UnicodeError, ValueError):
        raise _HTTPFailure("unauthorized") from None


def _valid_loopback_scope(scope: Scope, config: AgentConfig, host: bytes) -> bool:
    client = scope.get("client")
    server = scope.get("server")
    if (
        not isinstance(client, tuple)
        or len(client) != 2
        or not isinstance(client[0], str)
        or not isinstance(server, tuple)
        or len(server) != 2
        or not isinstance(server[0], str)
        or type(server[1]) is not int
    ):
        return False
    try:
        client_address = ip_address(client[0])
        server_address = ip_address(server[0])
        host_address, host_port = _parse_host(host)
    except (ValueError, _HTTPFailure):
        return False
    return (
        client_address.is_loopback
        and server_address.is_loopback
        and server_address == config.http.bind
        and server[1] == config.http.port
        and host_address == server_address
        and host_port == server[1]
    )


async def _read_body(
    receive: Receive,
    *,
    maximum: int,
    expected_length: int | None,
) -> bytes:
    body = bytearray()
    while True:
        message = await receive()
        if message.get("type") != "http.request":
            raise _HTTPFailure("invalid_request")
        chunk = message.get("body", b"")
        more_body = message.get("more_body", False)
        if type(chunk) is not bytes or type(more_body) is not bool:
            raise _HTTPFailure("invalid_request")
        if len(chunk) > maximum - len(body):
            raise _HTTPFailure("invalid_request")
        body.extend(chunk)
        if not more_body:
            break
    if expected_length is not None and expected_length != len(body):
        raise _HTTPFailure("invalid_request")
    return bytes(body)


def _error_payload(code: str, request_id: str | None) -> ErrorResponse:
    return ErrorResponse(
        contract_version="v1",
        request_id=request_id,
        error=ErrorDetail(code=cast(Any, code)),
    )


async def _send_json(send: Send, status: int, value: BaseModel | object) -> None:
    try:
        body = canonical_json_bytes(value)
    except Exception:
        body = canonical_json_bytes(_error_payload("internal_error", None))
        status = 500
    if len(body) > RESPONSE_BODY_BYTES_LIMIT:
        body = canonical_json_bytes(_error_payload("internal_error", None))
        status = 500
    await send(
        {
            "type": "http.response.start",
            "status": status,
            "headers": [
                (b"content-length", str(len(body)).encode("ascii")),
                (b"content-type", b"application/json"),
                (b"cache-control", b"no-store"),
            ],
        }
    )
    await send({"type": "http.response.body", "body": body})


class _StrictHTTPGate:
    """在 FastAPI 分派前完成 wire、认证、JSON 与 Pydantic 校验。"""

    def __init__(
        self,
        app: ASGIApp,
        *,
        config: AgentConfig,
        authorization_digest: bytes,
    ) -> None:
        self._app = app
        self._config = config
        self._authorization_digest = authorization_digest

    async def _dispatch_run(
        self,
        scope: Scope,
        receive: Receive,
        send: Send,
    ) -> None:
        async def run_application() -> None:
            await self._app(scope, receive, send)

        async def wait_for_disconnect() -> Message:
            return await receive()

        application: asyncio.Task[None] = asyncio.create_task(
            run_application(),
            name="companion-agent-http-run",
        )
        disconnect: asyncio.Task[Message] = asyncio.create_task(
            wait_for_disconnect(),
            name="companion-agent-http-disconnect",
        )
        try:
            done, _ = await asyncio.wait(
                (application, disconnect),
                return_when=asyncio.FIRST_COMPLETED,
            )
            if application in done:
                cleanup_cancellation = await _cancel_tasks(disconnect)
                if cleanup_cancellation is not None:
                    raise cleanup_cancellation
                application.result()
                return

            try:
                message = disconnect.result()
            except Exception:
                cleanup_cancellation = await _cancel_tasks(application)
                if cleanup_cancellation is not None:
                    raise cleanup_cancellation from None
                raise
            if not isinstance(message, dict) or message.get("type") != "http.disconnect":
                cleanup_cancellation = await _cancel_tasks(application)
                if cleanup_cancellation is not None:
                    raise cleanup_cancellation
                raise _HTTPFailure("invalid_request")
            cleanup_cancellation = await _cancel_tasks(application)
            if cleanup_cancellation is not None:
                raise cleanup_cancellation
        except asyncio.CancelledError:
            await _cancel_tasks(application, disconnect)
            raise

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self._app(scope, receive, send)
            return
        request_id: str | None = None
        dispatched = False
        response_started = False

        async def tracked_send(message: Message) -> None:
            nonlocal response_started
            if message.get("type") == "http.response.start":
                response_started = True
            await send(message)

        try:
            raw_headers = scope.get("headers")
            if not isinstance(raw_headers, list) or any(
                not isinstance(item, tuple)
                or len(item) != 2
                or type(item[0]) is not bytes
                or type(item[1]) is not bytes
                for item in raw_headers
            ):
                raise _HTTPFailure("invalid_request")
            headers = cast(list[tuple[bytes, bytes]], raw_headers)
            if sum(len(name) + 2 + len(value) + 2 for name, value in headers) > HEADER_BYTES_LIMIT:
                raise _HTTPFailure("invalid_request")
            if any(
                b"\r" in name
                or b"\n" in name
                or b"\x00" in name
                or b"\r" in value
                or b"\n" in value
                or b"\x00" in value
                for name, value in headers
            ):
                raise _HTTPFailure("invalid_request")

            host_values = _header_values(headers, b"host")
            if len(host_values) != 1 or not _valid_loopback_scope(
                scope, self._config, host_values[0]
            ):
                raise _HTTPFailure("unauthorized")
            if (
                scope.get("scheme") != "http"
                or scope.get("root_path", "") != ""
                or scope.get("query_string", b"") != b""
            ):
                raise _HTTPFailure("invalid_request")
            method = scope.get("method")
            path = scope.get("path")
            raw_path = scope.get("raw_path")
            if type(method) is not str or type(path) is not str:
                raise _HTTPFailure("invalid_request")
            try:
                expected_raw_path = path.encode("ascii", errors="strict")
            except UnicodeError:
                raise _HTTPFailure("invalid_request") from None
            if raw_path is not None and raw_path != expected_raw_path:
                raise _HTTPFailure("invalid_request")
            route = _ROUTE_BY_KEY.get((method, path))
            if route is None:
                raise _HTTPFailure("invalid_request")

            if route.authenticated:
                authorization = _header_values(headers, b"authorization")
                supplied = authorization[0] if len(authorization) == 1 else b""
                if len(authorization) != 1 or not hmac.compare_digest(
                    hashlib.sha256(supplied).digest(),
                    self._authorization_digest,
                ):
                    raise _HTTPFailure("unauthorized")

            content_lengths = _header_values(headers, b"content-length")
            if len(content_lengths) > 1:
                raise _HTTPFailure("invalid_request")
            expected_length: int | None = None
            if content_lengths:
                encoded_length = content_lengths[0]
                if (
                    not encoded_length
                    or len(encoded_length) > len(str(REQUEST_BODY_BYTES_LIMIT))
                    or not encoded_length.isascii()
                    or not encoded_length.isdigit()
                ):
                    raise _HTTPFailure("invalid_request")
                expected_length = int(encoded_length)
                if str(expected_length).encode("ascii") != encoded_length:
                    raise _HTTPFailure("invalid_request")
                maximum = REQUEST_BODY_BYTES_LIMIT if route.request_schema else 0
                if expected_length > maximum:
                    raise _HTTPFailure("invalid_request")

            if route.request_schema is None:
                body = await _read_body(receive, maximum=0, expected_length=expected_length)
                if body:
                    raise _HTTPFailure("invalid_request")
            else:
                content_types = _header_values(headers, b"content-type")
                if content_types != [b"application/json"]:
                    raise _HTTPFailure("invalid_request")
                body = await _read_body(
                    receive,
                    maximum=REQUEST_BODY_BYTES_LIMIT,
                    expected_length=expected_length,
                )
                document = _strict_json_object(body)
                candidate_id = document.get("request_id")
                try:
                    request_id = _UUID_ADAPTER.validate_python(candidate_id)
                except ValidationError:
                    request_id = None
                version = document.get("contract_version")
                if type(version) is str and version != "v1":
                    raise _HTTPFailure("unsupported_version")
                if version != "v1":
                    raise _HTTPFailure("invalid_request")
                try:
                    parsed = adapter_for("http-v1", route.request_schema).validate_python(document)
                except ValidationError:
                    raise _HTTPFailure("invalid_request") from None
                state = scope.setdefault("state", {})
                if not isinstance(state, dict):
                    raise _HTTPFailure("internal_error")
                state["contract_request"] = parsed
                state["request_id"] = request_id
            dispatched = True
            if route.request_schema in _RUN_REQUEST_SCHEMAS:
                await self._dispatch_run(scope, receive, tracked_send)
            else:
                await self._app(scope, receive, tracked_send)
        except _HTTPFailure as error:
            if response_started:
                return
            status = _ERROR_STATUSES.get(error.code, 500)
            await _send_json(send, status, _error_payload(error.code, request_id))
        except Exception as error:
            if response_started:
                return
            code = _map_exception(error) if dispatched else "internal_error"
            await _send_json(send, _ERROR_STATUSES[code], _error_payload(code, request_id))


async def _drain_cleanup(awaitable: Awaitable[object]) -> BaseException | None:
    task = asyncio.ensure_future(awaitable)
    cancellation: asyncio.CancelledError | None = None
    while not task.done():
        try:
            await asyncio.shield(task)
        except asyncio.CancelledError as error:
            if cancellation is None:
                cancellation = error
        except BaseException:
            break
    try:
        task.result()
    except BaseException as error:
        return cancellation or error
    return cancellation


async def _cancel_tasks(
    *tasks: asyncio.Task[Any],
) -> asyncio.CancelledError | None:
    for task in tasks:
        if not task.done():
            task.cancel()
    cleanup_error = await _drain_cleanup(asyncio.gather(*tasks, return_exceptions=True))
    if isinstance(cleanup_error, asyncio.CancelledError):
        return cleanup_error
    return None


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


def _response(value: BaseModel | object, *, status_code: int = 200) -> Response:
    try:
        body = canonical_json_bytes(value)
    except Exception:
        raise _HTTPFailure("internal_error") from None
    if len(body) > RESPONSE_BODY_BYTES_LIMIT:
        raise _HTTPFailure("internal_error")
    return Response(
        content=body,
        status_code=status_code,
        media_type="application/json",
        headers={"Cache-Control": "no-store"},
    )


def _request_value(request: Request, expected: type[BaseModel]) -> BaseModel:
    value = getattr(request.state, "contract_request", None)
    if not isinstance(value, expected):
        raise _HTTPFailure("internal_error")
    return value


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


def _map_exception(error: Exception) -> str:
    if isinstance(error, _HTTPFailure):
        return error.code if error.code in _ERROR_STATUSES else "internal_error"
    if isinstance(error, (PlannerFailure, DialogueFailure, AgentDomainFailure)):
        return error.code if error.code in _ERROR_STATUSES else "internal_error"
    return "internal_error"


def create_app(
    config: AgentConfig,
    secrets: ResolvedSecrets,
    *,
    component_factory: ComponentFactory | None = None,
) -> FastAPI:
    """创建不带 docs、OpenAPI、redirect 或隐式 route 的固定应用。"""

    runtime = _AgentRuntime()
    factory = component_factory or _default_component_factory
    authorization_digest = hashlib.sha256(
        b"Bearer " + secrets.http_bearer_token.get_secret_value().encode("ascii")
    ).digest()
    bootstrap_provider_api_key: SecretStr | None = secrets.provider_api_key
    del secrets

    async def bootstrap_components() -> AppComponents:
        nonlocal bootstrap_provider_api_key
        provider_api_key = bootstrap_provider_api_key
        bootstrap_provider_api_key = None
        if provider_api_key is None:
            raise RuntimeError("component bootstrap already attempted")
        try:
            return await factory(config, provider_api_key)
        finally:
            del provider_api_key

    async def bootstrap_runtime() -> None:
        components = await bootstrap_components()
        try:
            await runtime.start(components)
        except BaseException:
            await _close_unowned_components(components)
            raise

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        try:
            try:
                startup_error = await _drain_cleanup(bootstrap_runtime())
                if startup_error is not None:
                    raise startup_error
            except asyncio.CancelledError:
                raise
            except Exception:
                runtime.bootstrap_failed = True
            yield
        finally:
            cleanup_error = await _drain_cleanup(runtime.shutdown())
            if isinstance(cleanup_error, asyncio.CancelledError):
                raise cleanup_error from None

    app = FastAPI(
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
        redirect_slashes=False,
        lifespan=lifespan,
    )
    app.state.agent_runtime = runtime
    app.add_middleware(
        _StrictHTTPGate,
        config=config,
        authorization_digest=authorization_digest,
    )

    async def live() -> Response:
        return _response(LiveResponse(status="live"))

    async def ready() -> Response:
        status = "ready" if runtime.is_ready else "not_ready"
        return _response(
            ReadyResponse(status=cast(Any, status)),
            status_code=200 if status == "ready" else 503,
        )

    async def acquire(request: Request) -> Response:
        value = cast(AcquireRequest, _request_value(request, AcquireRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            grant = await leases.acquire(value.namespace_id, value.client_instance_id)
        return _response(
            AcquireResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                **grant.model_dump(),
            )
        )

    async def heartbeat(request: Request) -> Response:
        value = cast(LeaseRequest, _request_value(request, LeaseRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            grant = await leases.heartbeat(_lease_identity(value))
        return _response(
            HeartbeatResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                **grant.model_dump(),
            )
        )

    async def release(request: Request) -> Response:
        value = cast(LeaseRequest, _request_value(request, LeaseRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            await leases.release(_lease_identity(value))
        return _response(ReleaseResponse(**value.model_dump(), released=True))

    async def plan(request: Request) -> Response:
        value = cast(PlanRequest, _request_value(request, PlanRequest))
        return _response(await runtime.run_planner(value))

    async def dialogue(request: Request) -> Response:
        raw = getattr(request.state, "contract_request", None)
        if not isinstance(raw, (DialogueNonterminalRequest, DialogueTerminalRequest)):
            raise _HTTPFailure("internal_error")
        return _response(await runtime.run_dialogue(raw))

    async def reconcile(request: Request) -> Response:
        raw = getattr(request.state, "contract_request", None)
        if not isinstance(
            raw,
            (MemoryReconcileActiveRequest, MemoryReconcileInactiveRequest),
        ):
            raise _HTTPFailure("internal_error")
        value = raw
        async with runtime.business() as components:
            record = await components.store.reconcile(
                _lease_identity(value),
                MemoryReconcile(
                    namespace_id=value.namespace_id,
                    companion_id=value.companion_id,
                    memory_epoch=value.memory_epoch,
                    active=value.active,
                    tombstone_operation_id=value.tombstone_operation_id,
                    mirror=value.mirror,
                ),
            )
        if record.active:
            if record.memory is None:
                raise _HTTPFailure("internal_error")
            return _response(
                MemoryReconcileActiveResponse(
                    contract_version=value.contract_version,
                    request_id=value.request_id,
                    client_instance_id=value.client_instance_id,
                    namespace_id=value.namespace_id,
                    lease_id=value.lease_id,
                    companion_id=value.companion_id,
                    memory_epoch=record.memory_epoch,
                    active=True,
                    tombstone_operation_id=None,
                    memory=record.memory,
                )
            )
        if record.tombstone_operation_id is None:
            raise _HTTPFailure("internal_error")
        return _response(
            MemoryReconcileInactiveResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                client_instance_id=value.client_instance_id,
                namespace_id=value.namespace_id,
                lease_id=value.lease_id,
                companion_id=value.companion_id,
                memory_epoch=record.memory_epoch,
                active=False,
                tombstone_operation_id=record.tombstone_operation_id,
                memory=None,
            )
        )

    async def commit(request: Request) -> Response:
        value = cast(
            MemoryCommitRequest,
            _request_value(request, MemoryCommitRequest),
        )
        async with runtime.business() as components:
            result = await components.store.commit(
                _lease_identity(value),
                MemoryCommit(
                    namespace_id=value.namespace_id,
                    companion_id=value.companion_id,
                    memory_epoch=value.memory_epoch,
                    base_revision=value.base_revision,
                    operation_id=value.operation_id,
                    summary=value.summary,
                ),
            )
        return _response(
            MemoryCommitResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                client_instance_id=value.client_instance_id,
                lease_id=value.lease_id,
                namespace_id=result.namespace_id,
                companion_id=result.companion_id,
                memory_epoch=result.memory_epoch,
                operation_id=result.operation_id,
                committed_revision=result.committed_revision,
            )
        )

    async def delete(request: Request) -> Response:
        value = cast(
            MemoryDeleteRequest,
            _request_value(request, MemoryDeleteRequest),
        )
        async with runtime.business() as components:
            record = await components.store.delete(
                _lease_identity(value),
                MemoryDelete(
                    namespace_id=value.namespace_id,
                    companion_id=value.companion_id,
                    old_memory_epoch=value.old_memory_epoch,
                    new_memory_epoch=value.new_memory_epoch,
                    tombstone_operation_id=value.tombstone_operation_id,
                ),
            )
        if record.tombstone_operation_id is None:
            raise _HTTPFailure("internal_error")
        return _response(
            MemoryDeleteResponse(
                contract_version=value.contract_version,
                request_id=value.request_id,
                client_instance_id=value.client_instance_id,
                namespace_id=value.namespace_id,
                lease_id=value.lease_id,
                companion_id=value.companion_id,
                memory_epoch=record.memory_epoch,
                tombstone_operation_id=record.tombstone_operation_id,
            )
        )

    async def cancel(request: Request) -> Response:
        value = cast(CancelRequest, _request_value(request, CancelRequest))
        async with runtime.business():
            leases = runtime.leases
            if leases is None:
                raise _HTTPFailure("internal_error")
            cancelled = await leases.cancel_run(_lease_identity(value), value.run_id)
        return _response(CancelResponse(**value.model_dump(), cancelled=cancelled))

    app.add_api_route("/livez", live, methods=["GET"], include_in_schema=False)
    app.add_api_route("/readyz", ready, methods=["GET"], include_in_schema=False)
    app.add_api_route("/v1/namespaces/acquire", acquire, methods=["POST"], include_in_schema=False)
    app.add_api_route(
        "/v1/namespaces/heartbeat", heartbeat, methods=["POST"], include_in_schema=False
    )
    app.add_api_route("/v1/namespaces/release", release, methods=["POST"], include_in_schema=False)
    app.add_api_route("/v1/plan", plan, methods=["POST"], include_in_schema=False)
    app.add_api_route("/v1/dialogue", dialogue, methods=["POST"], include_in_schema=False)
    app.add_api_route("/v1/memory/reconcile", reconcile, methods=["POST"], include_in_schema=False)
    app.add_api_route("/v1/memory/commit", commit, methods=["POST"], include_in_schema=False)
    app.add_api_route("/v1/memory/delete", delete, methods=["POST"], include_in_schema=False)
    app.add_api_route("/v1/runs/cancel", cancel, methods=["POST"], include_in_schema=False)
    return app


def serve(config: AgentConfig, secrets: ResolvedSecrets) -> int:
    """以前台单 worker、h11 有界 header 配置运行 application。"""

    uvicorn.run(
        create_app(config, secrets),
        host=str(config.http.bind),
        port=config.http.port,
        workers=config.http.workers,
        http="h11",
        h11_max_incomplete_event_size=H11_INCOMPLETE_EVENT_BYTES_LIMIT,
        access_log=False,
        proxy_headers=False,
        server_header=False,
    )
    return 0


__all__ = [
    "AppComponents",
    "H11_INCOMPLETE_EVENT_BYTES_LIMIT",
    "HEADER_BYTES_LIMIT",
    "REQUEST_BODY_BYTES_LIMIT",
    "RESPONSE_BODY_BYTES_LIMIT",
    "create_app",
    "serve",
]

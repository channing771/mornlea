"""网关 wire 层：严格 HTTP 校验、认证、任务取消与错误映射。"""

from __future__ import annotations

import asyncio
import hashlib
import hmac
import json
import math
from collections.abc import Awaitable
from dataclasses import dataclass
from ipaddress import IPv4Address, IPv6Address, ip_address
from typing import Any, cast

from fastapi import Request, Response
from harness.config import AgentConfig
from harness.domain import adapter_for
from harness.domain.common import UUIDv4, canonical_json_bytes
from harness.domain.dialogue import DialogueFailure
from harness.domain.http_v1 import ErrorDetail, ErrorResponse
from harness.domain.memory import AgentDomainFailure
from harness.domain.planner import PlannerFailure
from pydantic import BaseModel, TypeAdapter, ValidationError
from starlette.types import ASGIApp, Message, Receive, Scope, Send

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


async def _finish_awaitable(
    awaitable: Awaitable[object],
    *,
    cancel_on_caller_cancellation: bool,
) -> BaseException | None:
    task = asyncio.ensure_future(awaitable)
    cancellation: asyncio.CancelledError | None = None
    while not task.done():
        try:
            await asyncio.shield(task)
        except asyncio.CancelledError as error:
            if cancellation is None:
                cancellation = error
                if cancel_on_caller_cancellation and not task.done():
                    task.cancel()
        except BaseException:
            break
    try:
        task.result()
    except BaseException as error:
        return cancellation or error
    return cancellation


async def _drain_cleanup(awaitable: Awaitable[object]) -> BaseException | None:
    return await _finish_awaitable(
        awaitable,
        cancel_on_caller_cancellation=False,
    )


async def _cancel_and_drain(awaitable: Awaitable[object]) -> BaseException | None:
    return await _finish_awaitable(
        awaitable,
        cancel_on_caller_cancellation=True,
    )


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


def _map_exception(error: Exception) -> str:
    if isinstance(error, _HTTPFailure):
        return error.code if error.code in _ERROR_STATUSES else "internal_error"
    if isinstance(error, (PlannerFailure, DialogueFailure, AgentDomainFailure)):
        return error.code if error.code in _ERROR_STATUSES else "internal_error"
    return "internal_error"


__all__ = [
    "H11_INCOMPLETE_EVENT_BYTES_LIMIT",
    "HEADER_BYTES_LIMIT",
    "REQUEST_BODY_BYTES_LIMIT",
    "RESPONSE_BODY_BYTES_LIMIT",
]

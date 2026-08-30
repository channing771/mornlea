"""在上层 SDK 缓冲前限制异步 HTTP 响应正文。"""

from __future__ import annotations

import asyncio
import re
from collections.abc import AsyncIterator

import httpx

_DECIMAL = re.compile(r"^[0-9]+$")


class ResponseBodyLimitError(httpx.TransportError):
    """响应 framing 或正文突破本地硬边界。"""


class _BoundedAsyncByteStream(httpx.AsyncByteStream):
    def __init__(self, inner: httpx.AsyncByteStream, *, maximum_bytes: int) -> None:
        self._inner = inner
        self._maximum_bytes = maximum_bytes
        self._seen = 0
        self._close_task: asyncio.Task[None] | None = None

    async def __aiter__(self) -> AsyncIterator[bytes]:
        try:
            async for chunk in self._inner:
                if len(chunk) > self._maximum_bytes - self._seen:
                    await self._close_for_error()
                    raise ResponseBodyLimitError("HTTP response body exceeds configured limit")
                self._seen += len(chunk)
                yield chunk
        except BaseException:
            await self._close_for_error()
            raise

    async def _close_for_error(self) -> None:
        try:
            await asyncio.shield(self._ensure_close_task())
        except BaseException:
            pass

    async def aclose(self) -> None:
        await asyncio.shield(self._ensure_close_task())

    def _ensure_close_task(self) -> asyncio.Task[None]:
        if self._close_task is None:
            self._close_task = asyncio.create_task(self._inner.aclose())
        return self._close_task


def _declared_content_length(headers: httpx.Headers) -> int | None:
    values = [
        part.strip() for value in headers.get_list("content-length") for part in value.split(",")
    ]
    if not values:
        return None
    if any(not value or _DECIMAL.fullmatch(value) is None for value in values):
        raise ResponseBodyLimitError("HTTP response has invalid content length")
    lengths = {int(value) for value in values}
    if len(lengths) != 1:
        raise ResponseBodyLimitError("HTTP response has conflicting content lengths")
    return lengths.pop()


def _requires_decoding(headers: httpx.Headers) -> bool:
    values = [
        part.strip().lower()
        for value in headers.get_list("content-encoding")
        for part in value.split(",")
        if part.strip()
    ]
    return any(value != "identity" for value in values)


class BoundedAsyncTransport(httpx.AsyncBaseTransport):
    """限制 raw body，并拒绝可在限制后膨胀的 content encoding。"""

    def __init__(
        self,
        inner: httpx.AsyncBaseTransport,
        *,
        maximum_bytes: int,
    ) -> None:
        if maximum_bytes <= 0:
            raise ValueError("maximum_bytes must be positive")
        self._inner = inner
        self._maximum_bytes = maximum_bytes
        self._close_task: asyncio.Task[None] | None = None

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        response = await self._inner.handle_async_request(request)
        try:
            declared = _declared_content_length(response.headers)
            if declared is not None and declared > self._maximum_bytes:
                raise ResponseBodyLimitError("HTTP response body exceeds configured limit")
            if _requires_decoding(response.headers):
                raise ResponseBodyLimitError("HTTP response content encoding is not allowed")
            if not isinstance(response.stream, httpx.AsyncByteStream):
                raise ResponseBodyLimitError("HTTP response stream is not asynchronous")
        except BaseException:
            try:
                await response.aclose()
            except BaseException:
                pass
            raise
        response.stream = _BoundedAsyncByteStream(
            response.stream,
            maximum_bytes=self._maximum_bytes,
        )
        return response

    async def aclose(self) -> None:
        await asyncio.shield(self._ensure_close_task())

    def _ensure_close_task(self) -> asyncio.Task[None]:
        if self._close_task is None:
            self._close_task = asyncio.create_task(self._inner.aclose())
        return self._close_task


__all__ = ["BoundedAsyncTransport", "ResponseBodyLimitError"]

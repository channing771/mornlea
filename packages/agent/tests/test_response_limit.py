from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator, Awaitable

import httpx
import pytest
from harness.tools.response_limit import (
    BoundedAsyncTransport,
    ResponseBodyLimitError,
)


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


class TrackingStream(httpx.AsyncByteStream):
    def __init__(self, chunks: tuple[bytes, ...]) -> None:
        self.chunks = chunks
        self.yielded = 0
        self.close_calls = 0

    async def __aiter__(self) -> AsyncIterator[bytes]:
        for chunk in self.chunks:
            self.yielded += len(chunk)
            yield chunk

    async def aclose(self) -> None:
        self.close_calls += 1


class SingleResponseTransport(httpx.AsyncBaseTransport):
    def __init__(self, response: httpx.Response) -> None:
        self.response = response
        self.close_calls = 0

    async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
        del request
        return self.response

    async def aclose(self) -> None:
        self.close_calls += 1


@pytest.mark.parametrize(
    "headers",
    [
        [(b"content-length", b"1"), (b"content-length", b"2")],
        [(b"content-length", b"invalid")],
        [(b"content-encoding", b"gzip")],
    ],
)
def test_invalid_framing_is_rejected_before_body_iteration(
    headers: list[tuple[bytes, bytes]],
) -> None:
    stream = TrackingStream((b"x",))

    async def scenario() -> None:
        transport = BoundedAsyncTransport(
            SingleResponseTransport(httpx.Response(200, headers=headers, stream=stream)),
            maximum_bytes=8,
        )
        with pytest.raises(ResponseBodyLimitError):
            await transport.handle_async_request(httpx.Request("GET", "https://example.test"))
        assert stream.yielded == 0
        assert stream.close_calls == 1

    run(scenario())


def test_declared_length_does_not_bypass_stream_count() -> None:
    stream = TrackingStream((b"ab",))

    async def scenario() -> None:
        transport = BoundedAsyncTransport(
            SingleResponseTransport(
                httpx.Response(200, headers={"content-length": "1"}, stream=stream)
            ),
            maximum_bytes=1,
        )
        response = await transport.handle_async_request(
            httpx.Request("GET", "https://example.test")
        )
        with pytest.raises(ResponseBodyLimitError):
            await response.aread()
        assert stream.yielded == 2
        assert stream.close_calls == 1

    run(scenario())


class BlockingStream(httpx.AsyncByteStream):
    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.close_calls = 0

    async def __aiter__(self) -> AsyncIterator[bytes]:
        self.started.set()
        await asyncio.Event().wait()
        yield b"unreachable"

    async def aclose(self) -> None:
        self.close_calls += 1


def test_cancel_closes_stream_and_all_close_paths_are_idempotent() -> None:
    async def scenario() -> None:
        stream = BlockingStream()
        inner = SingleResponseTransport(httpx.Response(200, stream=stream))
        transport = BoundedAsyncTransport(inner, maximum_bytes=8)
        response = await transport.handle_async_request(
            httpx.Request("GET", "https://example.test")
        )
        reader = asyncio.create_task(response.aread())
        await stream.started.wait()
        reader.cancel()
        with pytest.raises(asyncio.CancelledError):
            await reader
        await response.aclose()
        await response.aclose()
        await transport.aclose()
        await transport.aclose()
        assert stream.close_calls == 1
        assert inner.close_calls == 1

    run(scenario())

"""伙伴 Agent 的固定 HTTP v1 application、生命周期与 Uvicorn 入口。"""

from __future__ import annotations

import asyncio
import hashlib
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI
from harness.config import AgentConfig, ResolvedSecrets
from pydantic import SecretStr

from app.gateway.http_gate import (
    H11_INCOMPLETE_EVENT_BYTES_LIMIT,
    HEADER_BYTES_LIMIT,
    REQUEST_BODY_BYTES_LIMIT,
    RESPONSE_BODY_BYTES_LIMIT,
    _cancel_and_drain,
    _drain_cleanup,
    _StrictHTTPGate,
)
from app.gateway.routers import build_routers
from app.gateway.runtime import (
    AppComponents,
    ComponentFactory,
    _AgentRuntime,
    _close_unowned_components,
    _default_component_factory,
)


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
                startup_error = await _cancel_and_drain(bootstrap_runtime())
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

    for path, endpoint, methods in build_routers(runtime):
        app.add_api_route(path, endpoint, methods=methods, include_in_schema=False)
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

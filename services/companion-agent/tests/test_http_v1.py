from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncIterator, Awaitable
from contextlib import asynccontextmanager
from copy import deepcopy
from pathlib import Path
from types import FunctionType, MethodType, ModuleType
from typing import Any, cast

import httpx
import pytest
from pydantic import SecretStr

from mornlea_companion_agent.app import AppComponents, create_app, serve
from mornlea_companion_agent.config import (
    AgentConfig,
    HTTPConfig,
    LimitConfig,
    ProviderConfig,
    ResolvedSecrets,
    StorageConfig,
)
from mornlea_companion_agent.domain import adapter_for
from mornlea_companion_agent.domain.dialogue import DialogueMessage
from mornlea_companion_agent.domain.http_v1 import (
    DialogueNonterminalRequest,
    DialogueNonterminalResponse,
    DialogueTerminalRequest,
    DialogueTerminalResponse,
    MemoryProposal,
    PlanRequest,
    PlanResponse,
)
from mornlea_companion_agent.domain.mcp_v1 import Plan
from mornlea_companion_agent.domain.memory import LeaseIdentity, MemoryLookup
from mornlea_companion_agent.harness.dialogue import DialogueHarness
from mornlea_companion_agent.storage.sqlite_memory import SQLiteMemoryStore

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
CONTRACT_ROOT = REPOSITORY_ROOT / "contracts/companion-agent/http-v1"
TOKEN = "HTTP_TEST_BEARER_TOKEN_4fbe93"


def run(coroutine: Awaitable[object]) -> object:
    return asyncio.run(coroutine)


def contract_document(name: str) -> dict[str, Any]:
    return json.loads((CONTRACT_ROOT / name).read_text(encoding="utf-8"))


def golden(name: str) -> dict[str, Any]:
    document = contract_document("golden/valid.json")
    return deepcopy(next(case["value"] for case in document["cases"] if case["name"] == name))


def uuid4_text(index: int) -> str:
    return f"{index:08x}-0000-4000-8000-{index:012x}"


def config(tmp_path: Path) -> AgentConfig:
    return AgentConfig(
        config_version="v1",
        http=HTTPConfig(
            bind="127.0.0.1",
            port=8765,
            workers=1,
            bearer_token_env="HTTP_TOKEN",
        ),
        storage=StorageConfig.model_validate(
            {"sqlite_path": str(tmp_path / "agent.sqlite3")},
            context={"config_dir": tmp_path},
        ),
        provider=ProviderConfig(
            base_url="http://127.0.0.1:9999/v1",
            model="fake-model",
            api_key_env="PROVIDER_KEY",
        ),
        limits=LimitConfig(),
    )


def secrets() -> ResolvedSecrets:
    return ResolvedSecrets(
        http_bearer_token=SecretStr(TOKEN),
        provider_api_key=SecretStr("PROVIDER_TEST_KEY"),
    )


class FakeCloser:
    def __init__(self, order: list[str] | None = None) -> None:
        self.closed = False
        self.order = order

    async def aclose(self) -> None:
        self.closed = True
        if self.order is not None:
            self.order.append("model")


class FailingCloser(FakeCloser):
    async def aclose(self) -> None:
        await super().aclose()
        raise RuntimeError("SECRET_MODEL_CLOSE_FAILURE")


class EchoPlanner:
    async def run(self, request: PlanRequest) -> PlanResponse:
        return PlanResponse(
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
            plan=Plan.model_validate(
                {"summary": "走近箱子", "steps": [{"kind": "go_to", "x": 8, "y": 64, "z": -2}]}
            ),
        )


class EchoDialogue:
    async def run(
        self,
        request: DialogueNonterminalRequest | DialogueTerminalRequest,
    ) -> DialogueNonterminalResponse | DialogueTerminalResponse:
        identity = {
            "contract_version": request.contract_version,
            "request_id": request.request_id,
            "client_instance_id": request.client_instance_id,
            "namespace_id": request.namespace_id,
            "lease_id": request.lease_id,
            "run_id": request.run_id,
            "companion_id": request.companion_id,
            "generation": request.generation,
            "memory_epoch": request.memory_epoch,
        }
        if isinstance(request, DialogueTerminalRequest):
            return DialogueTerminalResponse(
                **identity,
                line="任务完成。",
                memory_proposal=MemoryProposal(
                    operation_id="99999999-9999-4999-8999-999999999999",
                    base_revision=3,
                    summary="完成任务。",
                ),
            )
        return DialogueNonterminalResponse(**identity, line="继续执行。")


class BlockingPlanner(EchoPlanner):
    def __init__(self, expected_starts: int = 1) -> None:
        self.started = 0
        self.expected_starts = expected_starts
        self.started_event = asyncio.Event()
        self.cancelled = 0

    async def run(self, request: PlanRequest) -> PlanResponse:
        del request
        self.started += 1
        if self.started >= self.expected_starts:
            self.started_event.set()
        try:
            await asyncio.Future()
        except asyncio.CancelledError:
            self.cancelled += 1
            raise


class BlockingDialogue(EchoDialogue):
    def __init__(self) -> None:
        self.started_event = asyncio.Event()
        self.cancelled = 0

    async def run(
        self,
        request: DialogueNonterminalRequest | DialogueTerminalRequest,
    ) -> DialogueNonterminalResponse | DialogueTerminalResponse:
        del request
        self.started_event.set()
        try:
            await asyncio.Future()
        except asyncio.CancelledError:
            self.cancelled += 1
            raise


class SlowCancellationPlanner(EchoPlanner):
    def __init__(self) -> None:
        self.started = asyncio.Event()
        self.cancelling = asyncio.Event()
        self.finish_cancellation = asyncio.Event()

    async def run(self, request: PlanRequest) -> PlanResponse:
        del request
        self.started.set()
        try:
            await asyncio.Future()
        except asyncio.CancelledError:
            self.cancelling.set()
            await self.finish_cancellation.wait()
            raise


class InvocationTrackingPlanner(BlockingPlanner):
    def __init__(self, expected_starts: int) -> None:
        super().__init__(expected_starts)
        self.invocations = 0

    def run(self, request: PlanRequest) -> Awaitable[PlanResponse]:
        self.invocations += 1
        return super().run(request)


class CountingDialogue(EchoDialogue):
    def __init__(self) -> None:
        self.calls = 0

    async def run(
        self,
        request: DialogueNonterminalRequest | DialogueTerminalRequest,
    ) -> DialogueNonterminalResponse | DialogueTerminalResponse:
        self.calls += 1
        return await super().run(request)


class StaticTerminalModel:
    async def complete(self, messages: tuple[DialogueMessage, ...]) -> object:
        del messages
        return '{"line":"完成。","summary":"模型只提出新摘要"}'


class OversizedPlanner(EchoPlanner):
    async def run(self, request: PlanRequest) -> PlanResponse:
        return PlanResponse.model_construct(
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
            plan=Plan.model_construct(summary="x" * 70_000, steps=()),
        )


@asynccontextmanager
async def client_for(app: Any) -> AsyncIterator[httpx.AsyncClient]:
    async with app.router.lifespan_context(app):
        transport = httpx.ASGITransport(
            app=app,
            client=("127.0.0.1", 54321),
            raise_app_exceptions=False,
        )
        async with httpx.AsyncClient(
            transport=transport,
            base_url="http://127.0.0.1:8765",
        ) as client:
            yield client


def app_with_components(
    tmp_path: Path,
    *,
    planner: object | None = None,
    dialogue: object | None = None,
    closer: FakeCloser | None = None,
    order: list[str] | None = None,
    factory_completed: asyncio.Event | None = None,
) -> tuple[Any, dict[str, SQLiteMemoryStore], FakeCloser]:
    stores: dict[str, SQLiteMemoryStore] = {}
    owned_closer = closer or FakeCloser(order)

    async def factory(
        agent_config: AgentConfig,
        provider_api_key: SecretStr,
    ) -> AppComponents:
        del provider_api_key
        store = await SQLiteMemoryStore.open(agent_config.storage.sqlite_path)
        if order is not None:
            original_close = store.close

            async def tracked_close() -> None:
                order.append("sqlite")
                await original_close()

            store.close = tracked_close  # type: ignore[method-assign]
        stores["store"] = store
        components = AppComponents(
            store=store,
            planner=planner or EchoPlanner(),
            dialogue=dialogue or EchoDialogue(),
            model_owner=owned_closer,
        )
        if factory_completed is not None:
            factory_completed.set()
        return components

    return create_app(config(tmp_path), secrets(), component_factory=factory), stores, owned_closer


async def acquire(client: httpx.AsyncClient, *, namespace: str, owner: str) -> dict[str, Any]:
    payload = golden("namespace acquire omits lease")
    payload.update(
        request_id=uuid4_text(1000 + int(namespace[:8], 16)),
        client_instance_id=owner,
        namespace_id=namespace,
    )
    response = await client.post(
        "/v1/namespaces/acquire",
        json=payload,
        headers={"Authorization": f"Bearer {TOKEN}"},
    )
    assert response.status_code == 200, response.text
    return response.json()


def plan_payload(
    *,
    namespace: str,
    owner: str,
    lease: str,
    companion: str,
    run_id: str,
) -> dict[str, Any]:
    payload = golden("planner run carries snapshot identity")
    payload.update(
        request_id=uuid4_text(int(run_id[:8], 16) + 100),
        client_instance_id=owner,
        namespace_id=namespace,
        lease_id=lease,
        run_id=run_id,
        companion_id=companion,
        snapshot_id=uuid4_text(int(run_id[:8], 16) + 200),
        deadline_unix_ms=4_000_000_000_000,
    )
    return payload


def recursively_retained_values(root: object) -> tuple[object, ...]:
    retained: list[object] = []
    seen: set[int] = set()

    def visit(value: object, depth: int) -> None:
        if depth > 20 or id(value) in seen:
            return
        seen.add(id(value))
        retained.append(value)
        if isinstance(value, (str, bytes, int, float, bool, type(None), ModuleType, type)):
            return
        if isinstance(value, FunctionType):
            for cell in value.__closure__ or ():
                try:
                    visit(cell.cell_contents, depth + 1)
                except ValueError:
                    continue
            return
        if isinstance(value, MethodType):
            visit(value.__self__, depth + 1)
            visit(value.__func__, depth + 1)
            return
        if isinstance(value, dict):
            for key, item in value.items():
                visit(key, depth + 1)
                visit(item, depth + 1)
            return
        if isinstance(value, (list, tuple, set, frozenset)):
            for item in value:
                visit(item, depth + 1)
            return
        try:
            attributes = vars(value)
        except TypeError:
            return
        visit(attributes, depth + 1)

    visit(root, 0)
    return tuple(retained)


def test_app_exposes_only_manifest_routes_and_health_auth(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        manifest = contract_document("manifest.json")
        expected = {(route["method"], route["path"]) for route in manifest["routes"]}
        actual = {
            (method, route.path)
            for route in app.routes
            for method in cast(set[str], route.methods or set())
        }
        assert actual == expected
        assert TOKEN not in repr(vars(app))
        assert TOKEN not in repr(app.user_middleware)

        async with client_for(app) as client:
            live = await client.get("/livez")
            assert live.status_code == 200
            adapter_for("http-v1", "live_response").validate_python(live.json())
            assert TOKEN not in repr(app.middleware_stack)
            assert TOKEN not in repr(vars(app.middleware_stack))

            unauthorized = await client.get("/readyz")
            assert unauthorized.status_code == 401
            assert unauthorized.json() == {
                "contract_version": "v1",
                "request_id": None,
                "error": {"code": "unauthorized"},
            }
            ready = await client.get("/readyz", headers={"Authorization": f"Bearer {TOKEN}"})
            assert ready.status_code == 200
            assert ready.json() == {"status": "ready"}

            for method, path in (
                ("HEAD", "/livez"),
                ("OPTIONS", "/v1/plan"),
                ("GET", "/v1/plan"),
                ("GET", "/docs"),
                ("GET", "/openapi.json"),
                ("GET", "/livez/"),
            ):
                response = await client.request(method, path, follow_redirects=False)
                assert not response.is_redirect
                assert response.status_code in {400, 404}

    run(scenario())


def test_app_lifespan_does_not_retain_resolved_secrets_or_raw_tokens(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        retained_before = recursively_retained_values(
            (app.router.lifespan_context, vars(app), app.user_middleware)
        )
        assert not any(isinstance(value, ResolvedSecrets) for value in retained_before)
        assert TOKEN not in {value for value in retained_before if type(value) is str}

        async with app.router.lifespan_context(app):
            retained_after = recursively_retained_values(
                (app.router.lifespan_context, vars(app), app.user_middleware)
            )
            retained_text = {value for value in retained_after if type(value) is str}
            assert TOKEN not in retained_text
            assert "PROVIDER_TEST_KEY" not in retained_text

    run(scenario())


def test_valid_route_goldens_round_trip_strict_identity(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        async with client_for(app) as client:
            namespace = "33333333-3333-4333-8333-333333333333"
            owner = "22222222-2222-4222-8222-222222222222"
            grant = await acquire(client, namespace=namespace, owner=owner)
            lease = grant["lease_id"]
            headers = {"Authorization": f"Bearer {TOKEN}"}

            heartbeat_request = golden("heartbeat carries lease only")
            heartbeat_request.update(
                client_instance_id=owner, namespace_id=namespace, lease_id=lease
            )
            heartbeat = await client.post(
                "/v1/namespaces/heartbeat", json=heartbeat_request, headers=headers
            )
            assert heartbeat.status_code == 200
            adapter_for("http-v1", "heartbeat_response").validate_python(heartbeat.json())

            plan_request = plan_payload(
                namespace=namespace,
                owner=owner,
                lease=lease,
                companion="66666666-6666-4666-8666-666666666666",
                run_id="55555555-5555-4555-8555-555555555555",
            )
            planned = await client.post("/v1/plan", json=plan_request, headers=headers)
            assert planned.status_code == 200, planned.text
            adapter_for("http-v1", "plan_response").validate_python(planned.json())
            for field in (
                "request_id",
                "client_instance_id",
                "namespace_id",
                "lease_id",
                "run_id",
                "companion_id",
                "generation",
                "snapshot_id",
                "snapshot_digest",
            ):
                assert planned.json()[field] == plan_request[field]

            dialogue_request = golden("nonterminal dialogue run")
            dialogue_request.update(
                client_instance_id=owner, namespace_id=namespace, lease_id=lease
            )
            dialogue = await client.post("/v1/dialogue", json=dialogue_request, headers=headers)
            assert dialogue.status_code == 200, dialogue.text
            adapter_for("http-v1", "dialogue_nonterminal_response").validate_python(dialogue.json())

            reconcile_request = golden("active memory reconcile carries mirror")
            reconcile_request.update(
                client_instance_id=owner, namespace_id=namespace, lease_id=lease
            )
            reconciled = await client.post(
                "/v1/memory/reconcile", json=reconcile_request, headers=headers
            )
            assert reconciled.status_code == 200, reconciled.text
            adapter_for("http-v1", "memory_reconcile_response").validate_python(reconciled.json())

            commit_request = golden("memory commit carries CAS identity")
            commit_request.update(
                client_instance_id=owner,
                namespace_id=namespace,
                lease_id=lease,
                operation_id="99999999-9999-4999-8999-999999999999",
            )
            committed = await client.post("/v1/memory/commit", json=commit_request, headers=headers)
            assert committed.status_code == 200, committed.text
            adapter_for("http-v1", "memory_commit_response").validate_python(committed.json())

            delete_request = golden("memory delete advances epoch")
            delete_request.update(client_instance_id=owner, namespace_id=namespace, lease_id=lease)
            deleted = await client.post("/v1/memory/delete", json=delete_request, headers=headers)
            assert deleted.status_code == 200, deleted.text
            adapter_for("http-v1", "memory_delete_response").validate_python(deleted.json())

            release_request = golden("heartbeat carries lease only")
            release_request.update(client_instance_id=owner, namespace_id=namespace, lease_id=lease)
            released = await client.post(
                "/v1/namespaces/release", json=release_request, headers=headers
            )
            assert released.status_code == 200
            adapter_for("http-v1", "release_response").validate_python(released.json())

    run(scenario())


def test_terminal_dialogue_returns_only_proposal_without_committing_memory(
    tmp_path: Path,
) -> None:
    stores: dict[str, SQLiteMemoryStore] = {}

    async def factory(
        agent_config: AgentConfig,
        provider_api_key: SecretStr,
    ) -> AppComponents:
        del provider_api_key
        store = await SQLiteMemoryStore.open(agent_config.storage.sqlite_path)
        stores["store"] = store
        return AppComponents(
            store=store,
            planner=EchoPlanner(),
            dialogue=DialogueHarness(
                StaticTerminalModel(),
                store,
                wall_clock=lambda: 1_700_000_000.0,
                operation_id_factory=lambda: "99999999-9999-4999-8999-999999999999",
            ),
            model_owner=FakeCloser(),
        )

    async def scenario() -> None:
        app = create_app(config(tmp_path), secrets(), component_factory=factory)
        async with client_for(app) as client:
            namespace = "33333333-3333-4333-8333-333333333333"
            owner = "22222222-2222-4222-8222-222222222222"
            grant = await acquire(client, namespace=namespace, owner=owner)
            lease = grant["lease_id"]
            headers = {"Authorization": f"Bearer {TOKEN}"}
            reconcile = golden("active memory reconcile carries mirror")
            reconcile.update(client_instance_id=owner, namespace_id=namespace, lease_id=lease)
            assert (
                await client.post("/v1/memory/reconcile", json=reconcile, headers=headers)
            ).status_code == 200

            terminal = golden("terminal dialogue request carries completed fact")
            terminal.update(
                client_instance_id=owner,
                namespace_id=namespace,
                lease_id=lease,
                deadline_unix_ms=4_000_000_000_000,
            )
            response = await client.post("/v1/dialogue", json=terminal, headers=headers)
            assert response.status_code == 200, response.text
            proposal = response.json()["memory_proposal"]
            assert proposal == {
                "operation_id": "99999999-9999-4999-8999-999999999999",
                "base_revision": 3,
                "summary": "模型只提出新摘要",
            }
            persisted = await stores["store"].load(
                LeaseIdentity(
                    namespace_id=namespace,
                    client_instance_id=owner,
                    lease_id=lease,
                ),
                MemoryLookup(
                    namespace_id=namespace,
                    companion_id=terminal["companion_id"],
                    memory_epoch=terminal["memory_epoch"],
                ),
            )
            assert persisted.revision == 3
            assert persisted.summary == "为玩家采集并交付了石料。"
            assert persisted.operation_id == "88888888-8888-4888-8888-888888888888"

    run(scenario())


def test_oversized_success_is_replaced_before_any_body_is_exposed(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path, planner=OversizedPlanner())
        async with client_for(app) as client:
            namespace = uuid4_text(21)
            owner = uuid4_text(121)
            grant = await acquire(client, namespace=namespace, owner=owner)
            payload = plan_payload(
                namespace=namespace,
                owner=owner,
                lease=grant["lease_id"],
                companion=uuid4_text(221),
                run_id=uuid4_text(321),
            )
            response = await client.post(
                "/v1/plan",
                json=payload,
                headers={"Authorization": f"Bearer {TOKEN}"},
            )
            assert response.status_code == 500
            assert response.json() == {
                "contract_version": "v1",
                "request_id": payload["request_id"],
                "error": {"code": "internal_error"},
            }
            assert b"x" * 128 not in response.content
            assert len(response.content) <= 65_536

    run(scenario())


@pytest.mark.parametrize(
    ("raw", "content_type", "status", "code", "echo_request"),
    [
        (b'{"contract_version":"v1",', "application/json", 400, "invalid_request", False),
        (
            b'[{"request_id":"11111111-1111-4111-8111-111111111111"}]',
            "application/json",
            400,
            "invalid_request",
            False,
        ),
        (b'{"request_id":NaN}', "application/json", 400, "invalid_request", False),
        (
            b'{"contract_version":"v1","request_id":"11111111-1111-4111-8111-111111111111","overflow":1e999}',
            "application/json",
            400,
            "invalid_request",
            False,
        ),
        (b'{"request_id":"x"}\xff', "application/json", 400, "invalid_request", False),
        (b'{"contract_version":"v1"} trailing', "application/json", 400, "invalid_request", False),
        (
            b'{"contract_version":"v1","request_id":"11111111-1111-4111-8111-111111111111","request_id":"11111111-1111-4111-8111-111111111111"}',
            "application/json",
            400,
            "invalid_request",
            False,
        ),
        (
            json.dumps(golden("namespace acquire omits lease")).encode(),
            "text/plain",
            400,
            "invalid_request",
            False,
        ),
    ],
)
def test_http_gate_rejects_non_strict_envelopes_without_leaking_body(
    tmp_path: Path,
    raw: bytes,
    content_type: str,
    status: int,
    code: str,
    echo_request: bool,
) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        async with client_for(app) as client:
            response = await client.post(
                "/v1/namespaces/acquire",
                content=raw,
                headers={
                    "Authorization": f"Bearer {TOKEN}",
                    "Content-Type": content_type,
                },
            )
            assert response.status_code == status
            value = response.json()
            assert value["error"]["code"] == code
            assert value["request_id"] is not None if echo_request else value["request_id"] is None
            body = response.text
            assert TOKEN not in body
            assert "PROVIDER_TEST_KEY" not in body

    run(scenario())


def test_request_id_and_version_are_classified_after_strict_json(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        async with client_for(app) as client:
            headers = {
                "Authorization": f"Bearer {TOKEN}",
                "Content-Type": "application/json",
            }
            unsupported = golden("namespace acquire omits lease")
            unsupported["contract_version"] = "v2"
            response = await client.post(
                "/v1/namespaces/acquire", content=json.dumps(unsupported), headers=headers
            )
            assert response.status_code == 426
            assert response.json() == {
                "contract_version": "v1",
                "request_id": unsupported["request_id"],
                "error": {"code": "unsupported_version"},
            }

            unknown = golden("namespace acquire omits lease")
            unknown["unknown"] = "DO_NOT_ECHO_THIS_VALUE"
            response = await client.post(
                "/v1/namespaces/acquire", content=json.dumps(unknown), headers=headers
            )
            assert response.status_code == 400
            assert response.json()["request_id"] == unknown["request_id"]
            assert "DO_NOT_ECHO_THIS_VALUE" not in response.text

            invalid_id = golden("namespace acquire omits lease")
            invalid_id["request_id"] = "not-canonical"
            response = await client.post(
                "/v1/namespaces/acquire", content=json.dumps(invalid_id), headers=headers
            )
            assert response.status_code == 400
            assert response.json()["request_id"] is None

    run(scenario())


async def raw_asgi_request(
    app: Any,
    *,
    method: str,
    path: str,
    headers: list[tuple[bytes, bytes]],
    messages: list[dict[str, object]],
) -> tuple[int, bytes, int]:
    sent: list[dict[str, object]] = []
    receive_calls = 0

    async def receive() -> dict[str, object]:
        nonlocal receive_calls
        receive_calls += 1
        if messages:
            return messages.pop(0)
        return {"type": "http.disconnect"}

    async def send(message: dict[str, object]) -> None:
        sent.append(message)

    await app(
        {
            "type": "http",
            "asgi": {"version": "3.0", "spec_version": "2.4"},
            "http_version": "1.1",
            "method": method,
            "scheme": "http",
            "path": path,
            "raw_path": path.encode("ascii"),
            "query_string": b"",
            "root_path": "",
            "headers": headers,
            "client": ("127.0.0.1", 50000),
            "server": ("127.0.0.1", 8765),
            "state": {},
        },
        receive,
        send,
    )
    start = next(item for item in sent if item["type"] == "http.response.start")
    body = b"".join(item.get("body", b"") for item in sent if item["type"] == "http.response.body")
    return int(start["status"]), body, receive_calls


def test_send_failure_after_response_start_does_not_start_a_second_response(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        sent: list[dict[str, object]] = []
        body_failed = False

        async def receive() -> dict[str, object]:
            return {"type": "http.request", "body": b"", "more_body": False}

        async def send(message: dict[str, object]) -> None:
            nonlocal body_failed
            sent.append(message)
            if message["type"] == "http.response.body" and not body_failed:
                body_failed = True
                raise RuntimeError("simulated disconnected send")

        await app(
            {
                "type": "http",
                "asgi": {"version": "3.0", "spec_version": "2.4"},
                "http_version": "1.1",
                "method": "GET",
                "scheme": "http",
                "path": "/livez",
                "raw_path": b"/livez",
                "query_string": b"",
                "root_path": "",
                "headers": [(b"host", b"127.0.0.1:8765")],
                "client": ("127.0.0.1", 50000),
                "server": ("127.0.0.1", 8765),
                "state": {},
            },
            receive,
            send,
        )
        assert sum(message["type"] == "http.response.start" for message in sent) == 1

    run(scenario())


@pytest.mark.parametrize("run_kind", ["plan", "dialogue"])
def test_http_disconnect_cancels_inflight_run_and_releases_capacity(
    tmp_path: Path,
    run_kind: str,
) -> None:
    async def scenario() -> None:
        planner = BlockingPlanner()
        dialogue = BlockingDialogue()
        app, _, _ = app_with_components(
            tmp_path,
            planner=planner,
            dialogue=dialogue,
        )
        async with app.router.lifespan_context(app):
            transport = httpx.ASGITransport(
                app=app,
                client=("127.0.0.1", 50000),
                raise_app_exceptions=False,
            )
            async with httpx.AsyncClient(
                transport=transport,
                base_url="http://127.0.0.1:8765",
            ) as client:
                namespace = uuid4_text(51 if run_kind == "plan" else 52)
                owner = uuid4_text(151 if run_kind == "plan" else 152)
                grant = await acquire(client, namespace=namespace, owner=owner)

            if run_kind == "plan":
                path = "/v1/plan"
                started = planner.started_event
                runner = planner
                payload = plan_payload(
                    namespace=namespace,
                    owner=owner,
                    lease=grant["lease_id"],
                    companion=uuid4_text(251),
                    run_id=uuid4_text(351),
                )
            else:
                path = "/v1/dialogue"
                started = dialogue.started_event
                runner = dialogue
                payload = golden("nonterminal dialogue run")
                payload.update(
                    client_instance_id=owner,
                    namespace_id=namespace,
                    lease_id=grant["lease_id"],
                    companion_id=uuid4_text(252),
                    run_id=uuid4_text(352),
                    deadline_unix_ms=4_000_000_000_000,
                )

            body = json.dumps(payload, separators=(",", ":")).encode()
            messages = [
                {"type": "http.request", "body": body, "more_body": False},
                {"type": "http.disconnect"},
                {"type": "http.disconnect"},
            ]
            disconnect_observed = asyncio.Event()
            receive_calls = 0
            sent: list[dict[str, object]] = []

            async def receive() -> dict[str, object]:
                nonlocal receive_calls
                receive_calls += 1
                message = messages.pop(0)
                if message["type"] == "http.disconnect":
                    await started.wait()
                    disconnect_observed.set()
                return message

            async def send(message: dict[str, object]) -> None:
                sent.append(message)

            request = asyncio.create_task(
                app(
                    {
                        "type": "http",
                        "asgi": {"version": "3.0", "spec_version": "2.4"},
                        "http_version": "1.1",
                        "method": "POST",
                        "scheme": "http",
                        "path": path,
                        "raw_path": path.encode("ascii"),
                        "query_string": b"",
                        "root_path": "",
                        "headers": [
                            (b"host", b"127.0.0.1:8765"),
                            (b"authorization", f"Bearer {TOKEN}".encode()),
                            (b"content-type", b"application/json"),
                            (b"content-length", str(len(body)).encode()),
                        ],
                        "client": ("127.0.0.1", 50000),
                        "server": ("127.0.0.1", 8765),
                        "state": {},
                    },
                    receive,
                    send,
                )
            )
            try:
                await asyncio.wait_for(started.wait(), timeout=1)
                await asyncio.sleep(0.05)
                assert disconnect_observed.is_set()
                assert request.done()
                await request
                assert receive_calls == 2
                assert runner.cancelled == 1
                assert sent == []
                assert app.state.agent_runtime.leases.run_gate.active_count == 0
            finally:
                if not request.done():
                    request.cancel()
                await asyncio.gather(request, return_exceptions=True)

    run(scenario())


def test_asgi_context_cancellation_drains_pending_disconnect_watcher(tmp_path: Path) -> None:
    async def scenario() -> None:
        planner = BlockingPlanner()
        app, _, _ = app_with_components(tmp_path, planner=planner)
        async with client_for(app) as client:
            namespace = uuid4_text(53)
            owner = uuid4_text(153)
            grant = await acquire(client, namespace=namespace, owner=owner)
            payload = plan_payload(
                namespace=namespace,
                owner=owner,
                lease=grant["lease_id"],
                companion=uuid4_text(253),
                run_id=uuid4_text(353),
            )
            request = asyncio.create_task(
                client.post(
                    "/v1/plan",
                    json=payload,
                    headers={"Authorization": f"Bearer {TOKEN}"},
                )
            )
            await asyncio.wait_for(planner.started_event.wait(), timeout=1)
            request.cancel()
            request.cancel()
            with pytest.raises(asyncio.CancelledError):
                await request
            await asyncio.sleep(0)

            assert planner.cancelled == 1
            assert app.state.agent_runtime.leases.run_gate.active_count == 0
            assert not any(
                task.get_name() in {"companion-agent-http-run", "companion-agent-http-disconnect"}
                for task in asyncio.all_tasks()
                if not task.done()
            )

    run(scenario())


def test_double_cancellation_during_run_bind_still_releases_capacity(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    async def scenario() -> None:
        planner = SlowCancellationPlanner()
        app, _, _ = app_with_components(tmp_path, planner=planner)
        async with app.router.lifespan_context(app):
            runtime = app.state.agent_runtime
            leases = runtime.leases
            assert leases is not None
            namespace = uuid4_text(54)
            owner = uuid4_text(154)
            grant = await leases.acquire(namespace, owner)
            payload = plan_payload(
                namespace=namespace,
                owner=owner,
                lease=grant.lease_id,
                companion=uuid4_text(254),
                run_id=uuid4_text(354),
            )
            bind_entered = asyncio.Event()

            async def blocking_bind(handle: object, task: asyncio.Task[object]) -> None:
                del handle, task
                bind_entered.set()
                await asyncio.Future()

            monkeypatch.setattr(leases.run_gate, "_bind_task", blocking_bind)
            operation = asyncio.create_task(
                runtime.run_planner(PlanRequest.model_validate(payload))
            )
            await asyncio.wait_for(bind_entered.wait(), timeout=1)
            await asyncio.wait_for(planner.started.wait(), timeout=1)

            operation.cancel()
            await asyncio.wait_for(planner.cancelling.wait(), timeout=1)
            operation.cancel()
            planner.finish_cancellation.set()
            await asyncio.gather(operation, return_exceptions=True)

            assert leases.run_gate.active_count == 0

    run(scenario())


def test_auth_header_and_content_length_fail_before_body_receive(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        valid_body = json.dumps(golden("namespace acquire omits lease")).encode()
        common = [
            (b"host", b"127.0.0.1:8765"),
            (b"content-type", b"application/json"),
            (b"content-length", str(len(valid_body)).encode()),
        ]
        status, body, calls = await raw_asgi_request(
            app,
            method="POST",
            path="/v1/namespaces/acquire",
            headers=[*common, (b"authorization", b"Bearer wrong")],
            messages=[{"type": "http.request", "body": valid_body, "more_body": False}],
        )
        assert status == 401
        assert calls == 0
        assert json.loads(body)["request_id"] is None

        oversized = [
            (b"host", b"127.0.0.1:8765"),
            (b"authorization", f"Bearer {TOKEN}".encode()),
            (b"content-type", b"application/json"),
            (b"content-length", b"262145"),
        ]
        status, body, calls = await raw_asgi_request(
            app,
            method="POST",
            path="/v1/namespaces/acquire",
            headers=oversized,
            messages=[{"type": "http.request", "body": b"never-read", "more_body": False}],
        )
        assert status == 400
        assert calls == 0
        assert json.loads(body)["request_id"] is None

    run(scenario())


def test_exact_body_limit_and_duplicate_contract_headers(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        payload = json.dumps(
            golden("namespace acquire omits lease"),
            separators=(",", ":"),
        ).encode()
        exact_body = payload + b" " * (262_144 - len(payload))
        base = [
            (b"host", b"127.0.0.1:8765"),
            (b"authorization", f"Bearer {TOKEN}".encode()),
            (b"content-type", b"application/json"),
        ]
        async with app.router.lifespan_context(app):
            status, _, calls = await raw_asgi_request(
                app,
                method="POST",
                path="/v1/namespaces/acquire",
                headers=[*base, (b"content-length", b"262144")],
                messages=[{"type": "http.request", "body": exact_body, "more_body": False}],
            )
            assert status == 200
            assert calls == 1

            duplicates = (
                [*base, (b"authorization", f"Bearer {TOKEN}".encode())],
                [*base, (b"content-type", b"application/json")],
                [*base, (b"content-length", b"1"), (b"content-length", b"1")],
            )
            for headers in duplicates:
                status, body, calls = await raw_asgi_request(
                    app,
                    method="POST",
                    path="/v1/namespaces/acquire",
                    headers=headers,
                    messages=[{"type": "http.request", "body": payload, "more_body": False}],
                )
                assert status in {400, 401}
                assert calls == 0
                assert json.loads(body)["request_id"] is None

    run(scenario())


def test_streamed_body_and_header_limits_are_checked_before_copy_or_parse(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        headers = [
            (b"host", b"127.0.0.1:8765"),
            (b"authorization", f"Bearer {TOKEN}".encode()),
            (b"content-type", b"application/json"),
        ]
        status, body, calls = await raw_asgi_request(
            app,
            method="POST",
            path="/v1/namespaces/acquire",
            headers=headers,
            messages=[
                {"type": "http.request", "body": b"x" * 262_144, "more_body": True},
                {"type": "http.request", "body": b"y", "more_body": False},
            ],
        )
        assert status == 400
        assert calls == 2
        assert json.loads(body)["error"]["code"] == "invalid_request"

        ready_headers = [
            (b"host", b"127.0.0.1:8765"),
            (b"authorization", f"Bearer {TOKEN}".encode()),
        ]
        current = sum(len(name) + 2 + len(value) + 2 for name, value in ready_headers)
        filler_name = b"x-fill"
        filler = b"a" * (16_384 - current - len(filler_name) - 4)
        exact = [*ready_headers, (filler_name, filler)]
        assert sum(len(name) + 2 + len(value) + 2 for name, value in exact) == 16_384
        status, _, _ = await raw_asgi_request(
            app,
            method="GET",
            path="/readyz",
            headers=exact,
            messages=[{"type": "http.request", "body": b"", "more_body": False}],
        )
        assert status == 503
        over = [*ready_headers, (filler_name, filler + b"a")]
        status, body, calls = await raw_asgi_request(
            app,
            method="GET",
            path="/readyz",
            headers=over,
            messages=[{"type": "http.request", "body": b"", "more_body": False}],
        )
        assert status == 400
        assert calls == 0
        assert json.loads(body)["request_id"] is None

    run(scenario())


def test_non_loopback_client_and_host_are_rejected(tmp_path: Path) -> None:
    async def scenario() -> None:
        app, _, _ = app_with_components(tmp_path)
        transport = httpx.ASGITransport(
            app=app,
            client=("203.0.113.9", 50000),
            raise_app_exceptions=False,
        )
        async with app.router.lifespan_context(app):
            async with httpx.AsyncClient(
                transport=transport,
                base_url="http://127.0.0.1:8765",
            ) as client:
                response = await client.get("/livez")
                assert response.status_code == 401
                assert response.json()["request_id"] is None

            transport = httpx.ASGITransport(
                app=app,
                client=("127.0.0.1", 50000),
                raise_app_exceptions=False,
            )
            async with httpx.AsyncClient(
                transport=transport,
                base_url="http://example.com:8765",
            ) as client:
                response = await client.get("/livez")
                assert response.status_code == 401

    run(scenario())


def test_stale_lease_is_not_found_for_every_post_acquire_operation(tmp_path: Path) -> None:
    async def scenario() -> None:
        planner = InvocationTrackingPlanner(expected_starts=1)
        app, _, _ = app_with_components(tmp_path, planner=planner)
        async with client_for(app) as client:
            namespace = "33333333-3333-4333-8333-333333333333"
            owner = "22222222-2222-4222-8222-222222222222"
            first = await acquire(client, namespace=namespace, owner=owner)
            old_lease = first["lease_id"]
            second = await acquire(client, namespace=namespace, owner=owner)
            assert second["lease_id"] != old_lease
            headers = {"Authorization": f"Bearer {TOKEN}"}

            lease_request = golden("heartbeat carries lease only")
            lease_request.update(
                client_instance_id=owner, namespace_id=namespace, lease_id=old_lease
            )
            payloads: list[tuple[str, dict[str, Any]]] = [
                ("/v1/namespaces/heartbeat", deepcopy(lease_request)),
                ("/v1/namespaces/release", deepcopy(lease_request)),
            ]
            plan = plan_payload(
                namespace=namespace,
                owner=owner,
                lease=old_lease,
                companion="66666666-6666-4666-8666-666666666666",
                run_id="55555555-5555-4555-8555-555555555555",
            )
            payloads.append(("/v1/plan", plan))
            dialogue = golden("nonterminal dialogue run")
            dialogue.update(client_instance_id=owner, namespace_id=namespace, lease_id=old_lease)
            payloads.append(("/v1/dialogue", dialogue))
            for path, case_name in (
                ("/v1/memory/reconcile", "active memory reconcile carries mirror"),
                ("/v1/memory/commit", "memory commit carries CAS identity"),
                ("/v1/memory/delete", "memory delete advances epoch"),
                ("/v1/runs/cancel", "cancel carries run identity but no companion identity"),
            ):
                value = golden(case_name)
                value.update(client_instance_id=owner, namespace_id=namespace, lease_id=old_lease)
                payloads.append((path, value))
            for path, value in payloads:
                response = await client.post(path, json=value, headers=headers)
                assert response.status_code == 404, (path, response.text)
                assert response.json() == {
                    "contract_version": "v1",
                    "request_id": value["request_id"],
                    "error": {"code": "not_found"},
                }
            assert planner.invocations == 0

    run(scenario())


def test_planner_and_dialogue_share_per_companion_capacity(tmp_path: Path) -> None:
    async def scenario() -> None:
        planner = BlockingPlanner()
        dialogue_runner = CountingDialogue()
        app, _, _ = app_with_components(
            tmp_path,
            planner=planner,
            dialogue=dialogue_runner,
        )
        async with client_for(app) as client:
            namespace = uuid4_text(31)
            owner = uuid4_text(131)
            companion = uuid4_text(231)
            run_id = uuid4_text(331)
            grant = await acquire(client, namespace=namespace, owner=owner)
            headers = {"Authorization": f"Bearer {TOKEN}"}
            planned = plan_payload(
                namespace=namespace,
                owner=owner,
                lease=grant["lease_id"],
                companion=companion,
                run_id=run_id,
            )
            active = asyncio.create_task(client.post("/v1/plan", json=planned, headers=headers))
            await asyncio.wait_for(planner.started_event.wait(), timeout=1)

            dialogue = golden("nonterminal dialogue run")
            dialogue.update(
                client_instance_id=owner,
                namespace_id=namespace,
                lease_id=grant["lease_id"],
                companion_id=companion,
                run_id=uuid4_text(332),
                deadline_unix_ms=4_000_000_000_000,
            )
            overloaded = await client.post("/v1/dialogue", json=dialogue, headers=headers)
            assert overloaded.status_code == 429
            assert overloaded.json()["error"]["code"] == "overloaded"
            assert dialogue_runner.calls == 0

            cancel = golden("cancel carries run identity but no companion identity")
            cancel.update(
                client_instance_id=owner,
                namespace_id=namespace,
                lease_id=grant["lease_id"],
                run_id=run_id,
            )
            assert (
                await client.post("/v1/runs/cancel", json=cancel, headers=headers)
            ).status_code == 200
            assert (await active).status_code == 503

    run(scenario())


def test_reacquire_cancels_inflight_run_and_fences_its_late_result(tmp_path: Path) -> None:
    async def scenario() -> None:
        planner = BlockingPlanner()
        app, _, _ = app_with_components(tmp_path, planner=planner)
        async with client_for(app) as client:
            namespace = uuid4_text(41)
            owner = uuid4_text(141)
            grant = await acquire(client, namespace=namespace, owner=owner)
            payload = plan_payload(
                namespace=namespace,
                owner=owner,
                lease=grant["lease_id"],
                companion=uuid4_text(241),
                run_id=uuid4_text(341),
            )
            active = asyncio.create_task(
                client.post(
                    "/v1/plan",
                    json=payload,
                    headers={"Authorization": f"Bearer {TOKEN}"},
                )
            )
            await asyncio.wait_for(planner.started_event.wait(), timeout=1)
            replacement = await acquire(client, namespace=namespace, owner=owner)
            assert replacement["lease_id"] != grant["lease_id"]
            response = await active
            assert response.status_code == 404
            assert response.json()["error"]["code"] == "not_found"
            assert planner.cancelled == 1

    run(scenario())


def test_serve_uses_single_worker_h11_without_proxy_or_access_log(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    captured: dict[str, object] = {}

    def fake_run(app: object, **kwargs: object) -> None:
        captured["app"] = app
        captured.update(kwargs)

    monkeypatch.setattr("mornlea_companion_agent.app.uvicorn.run", fake_run)
    assert serve(config(tmp_path), secrets()) == 0
    assert captured["host"] == "127.0.0.1"
    assert captured["port"] == 8765
    assert captured["workers"] == 1
    assert captured["http"] == "h11"
    manifest = contract_document("manifest.json")
    maximum_request_line = max(
        len(f"{route['method']} {route['path']} HTTP/1.1\r\n".encode("ascii"))
        for route in manifest["routes"]
    )
    assert captured["h11_max_incomplete_event_size"] == 16_384 + maximum_request_line + 2
    assert captured["access_log"] is False
    assert captured["proxy_headers"] is False


def test_global_run_overload_is_immediate_and_cancel_releases_slots(tmp_path: Path) -> None:
    async def scenario() -> None:
        planner = InvocationTrackingPlanner(expected_starts=4)
        app, _, _ = app_with_components(tmp_path, planner=planner)
        async with client_for(app) as client:
            headers = {"Authorization": f"Bearer {TOKEN}"}
            running: list[tuple[asyncio.Task[httpx.Response], dict[str, Any]]] = []
            for index in range(1, 5):
                namespace = uuid4_text(index)
                owner = uuid4_text(100 + index)
                companion = uuid4_text(200 + index)
                run_id = uuid4_text(300 + index)
                grant = await acquire(client, namespace=namespace, owner=owner)
                payload = plan_payload(
                    namespace=namespace,
                    owner=owner,
                    lease=grant["lease_id"],
                    companion=companion,
                    run_id=run_id,
                )
                running.append(
                    (
                        asyncio.create_task(client.post("/v1/plan", json=payload, headers=headers)),
                        payload,
                    )
                )
            await asyncio.wait_for(planner.started_event.wait(), timeout=1)

            fifth_namespace = uuid4_text(5)
            fifth_owner = uuid4_text(105)
            fifth_grant = await acquire(client, namespace=fifth_namespace, owner=fifth_owner)
            fifth = plan_payload(
                namespace=fifth_namespace,
                owner=fifth_owner,
                lease=fifth_grant["lease_id"],
                companion=uuid4_text(205),
                run_id=uuid4_text(305),
            )
            overloaded = await client.post("/v1/plan", json=fifth, headers=headers)
            assert overloaded.status_code == 429
            assert overloaded.json()["error"]["code"] == "overloaded"
            assert planner.started == 4
            assert planner.invocations == 4

            for _, payload in running:
                cancel = golden("cancel carries run identity but no companion identity")
                cancel.update(
                    client_instance_id=payload["client_instance_id"],
                    namespace_id=payload["namespace_id"],
                    lease_id=payload["lease_id"],
                    run_id=payload["run_id"],
                )
                response = await client.post("/v1/runs/cancel", json=cancel, headers=headers)
                assert response.status_code == 200
                assert response.json()["cancelled"] is True
            results = await asyncio.gather(*(task for task, _ in running))
            assert all(response.status_code == 503 for response in results)
            assert planner.cancelled == 4
            assert app.state.agent_runtime.leases.run_gate.active_count == 0

    run(scenario())


def test_bootstrap_failure_keeps_live_but_not_ready(tmp_path: Path) -> None:
    async def factory(config: AgentConfig, provider_api_key: SecretStr) -> AppComponents:
        del config, provider_api_key
        raise RuntimeError("SECRET_BOOTSTRAP_BODY")

    async def scenario() -> None:
        app = create_app(config(tmp_path), secrets(), component_factory=factory)
        async with client_for(app) as client:
            assert (await client.get("/livez")).status_code == 200
            ready = await client.get("/readyz", headers={"Authorization": f"Bearer {TOKEN}"})
            assert ready.status_code == 503
            assert ready.json() == {"status": "not_ready"}
            payload = golden("namespace acquire omits lease")
            response = await client.post(
                "/v1/namespaces/acquire",
                json=payload,
                headers={"Authorization": f"Bearer {TOKEN}"},
            )
            assert response.status_code == 500
            assert "SECRET_BOOTSTRAP_BODY" not in response.text

    run(scenario())


def test_startup_cancellation_after_factory_return_closes_transferred_components(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        order: list[str] = []
        factory_completed = asyncio.Event()
        closer = FailingCloser(order)
        app, stores, _ = app_with_components(
            tmp_path,
            closer=closer,
            order=order,
            factory_completed=factory_completed,
        )
        runtime = app.state.agent_runtime
        lifespan = app.router.lifespan_context(app)
        startup: asyncio.Task[object] | None = None
        lock_held = False
        try:
            await runtime._state_lock.acquire()
            lock_held = True
            startup = asyncio.create_task(lifespan.__aenter__())
            await asyncio.wait_for(factory_completed.wait(), timeout=1)
            await asyncio.sleep(0)

            startup.cancel()
            await asyncio.sleep(0)
            startup.cancel()
            await asyncio.sleep(0)
            draining_after_cancellation = not startup.done()
            runtime._state_lock.release()
            lock_held = False

            with pytest.raises(asyncio.CancelledError):
                await startup

            assert draining_after_cancellation
            assert closer.closed
            assert stores["store"]._closed
            assert order == ["model", "sqlite"]
            assert runtime.components is None
            assert runtime.leases is None
            assert runtime._expiry_task is None
            assert not any(
                task.get_name() == "companion-agent-lease-expiry"
                for task in asyncio.all_tasks()
                if not task.done()
            )
        finally:
            if lock_held:
                runtime._state_lock.release()
            if startup is not None and not startup.done():
                startup.cancel()
                await asyncio.gather(startup, return_exceptions=True)
            store = stores.get("store")
            if store is not None and not store._closed:
                await store.close()

    run(scenario())


def test_start_failure_closes_untransferred_components_after_model_close_error(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class StartupAbort(BaseException):
        pass

    async def scenario() -> None:
        order: list[str] = []
        closer = FailingCloser(order)
        app, stores, _ = app_with_components(
            tmp_path,
            closer=closer,
            order=order,
        )
        runtime = app.state.agent_runtime

        async def fail_start(components: AppComponents) -> None:
            del components
            raise StartupAbort

        monkeypatch.setattr(runtime, "start", fail_start)
        lifespan = app.router.lifespan_context(app)
        with pytest.raises(StartupAbort):
            await lifespan.__aenter__()
        model_closed = closer.closed
        store_closed = stores["store"]._closed
        close_order = tuple(order)
        components = runtime.components
        leases = runtime.leases
        if not store_closed:
            await stores["store"].close()

        assert model_closed
        assert store_closed
        assert close_order == ("model", "sqlite")
        assert components is None
        assert leases is None

    run(scenario())


def test_shutdown_closes_sqlite_and_clears_state_when_model_close_fails(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        order: list[str] = []
        closer = FailingCloser(order)
        app, stores, _ = app_with_components(
            tmp_path,
            closer=closer,
            order=order,
        )
        lifespan = app.router.lifespan_context(app)
        await lifespan.__aenter__()
        runtime = app.state.agent_runtime
        shutdown_error: BaseException | None = None
        try:
            await lifespan.__aexit__(None, None, None)
        except BaseException as error:
            shutdown_error = error
        closed_during_shutdown = stores["store"]._closed
        order_during_shutdown = tuple(order)
        components_after_shutdown = runtime.components
        leases_after_shutdown = runtime.leases
        if not closed_during_shutdown:
            await stores["store"].close()

        assert shutdown_error is None
        assert closed_during_shutdown
        assert order_during_shutdown == ("model", "sqlite")
        assert components_after_shutdown is None
        assert leases_after_shutdown is None

    run(scenario())


def test_shutdown_cancellation_while_waiting_for_state_lock_drains_all_resources(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        order: list[str] = []
        closer = FakeCloser(order)
        app, stores, _ = app_with_components(
            tmp_path,
            closer=closer,
            order=order,
        )
        lifespan = app.router.lifespan_context(app)
        await lifespan.__aenter__()
        runtime = app.state.agent_runtime
        expiry = runtime._expiry_task
        shutdown: asyncio.Task[None] | None = None
        lock_held = False
        try:
            await runtime._state_lock.acquire()
            lock_held = True
            shutdown = asyncio.create_task(runtime.shutdown())
            await asyncio.sleep(0)
            shutdown.cancel()
            await asyncio.sleep(0)
            draining_after_cancellation = not shutdown.done()
            runtime._state_lock.release()
            lock_held = False

            with pytest.raises(asyncio.CancelledError):
                await shutdown

            assert draining_after_cancellation
            assert closer.closed
            assert stores["store"]._closed
            assert order == ["model", "sqlite"]
            assert expiry is not None and expiry.done()
            assert runtime._expiry_task is None
            assert runtime.components is None
            assert runtime.leases is None
        finally:
            if lock_held:
                runtime._state_lock.release()
            if shutdown is not None and not shutdown.done():
                shutdown.cancel()
                await asyncio.gather(shutdown, return_exceptions=True)
            await lifespan.__aexit__(None, None, None)
            if not stores["store"]._closed:
                await stores["store"].close()

    run(scenario())


def test_shutdown_stops_accepting_cancels_runs_then_closes_model_and_sqlite(
    tmp_path: Path,
) -> None:
    async def scenario() -> None:
        order: list[str] = []
        planner = BlockingPlanner()
        closer = FakeCloser(order)
        app, stores, _ = app_with_components(
            tmp_path,
            planner=planner,
            closer=closer,
            order=order,
        )
        lifespan = app.router.lifespan_context(app)
        await lifespan.__aenter__()
        transport = httpx.ASGITransport(
            app=app,
            client=("127.0.0.1", 50000),
            raise_app_exceptions=False,
        )
        client = httpx.AsyncClient(transport=transport, base_url="http://127.0.0.1:8765")
        namespace = uuid4_text(1)
        owner = uuid4_text(101)
        grant = await acquire(client, namespace=namespace, owner=owner)
        payload = plan_payload(
            namespace=namespace,
            owner=owner,
            lease=grant["lease_id"],
            companion=uuid4_text(201),
            run_id=uuid4_text(301),
        )
        request = asyncio.create_task(
            client.post(
                "/v1/plan",
                json=payload,
                headers={"Authorization": f"Bearer {TOKEN}"},
            )
        )
        await asyncio.wait_for(planner.started_event.wait(), timeout=1)
        await lifespan.__aexit__(None, None, None)
        response = await request
        assert response.status_code == 503
        assert planner.cancelled == 1
        assert closer.closed
        assert order == ["model", "sqlite"]
        assert stores["store"]._closed is True
        await client.aclose()

    run(scenario())

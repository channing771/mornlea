"""把 OpenAI-compatible LangChain chat model 收窄为 Planner 领域端口。"""

from __future__ import annotations

import asyncio
from copy import deepcopy
from typing import Any

import httpx
from langchain_core.language_models import BaseChatModel
from langchain_core.messages import (
    AIMessage,
    BaseMessage,
    HumanMessage,
    SystemMessage,
    ToolMessage,
)
from langchain_core.runnables import Runnable
from langchain_openai import ChatOpenAI
from pydantic import SecretStr

from harness.domain.dialogue import (
    DialogueMessage,
    DialogueUnavailable,
    InvalidDialogueOutput,
)
from harness.domain.mcp_contract import mcp_tool_contracts
from harness.domain.planner import (
    InvalidModelOutput,
    ModelOutput,
    ModelToolCall,
    PlannerMessage,
    PlannerUnavailable,
)
from harness.tools.response_limit import BoundedAsyncTransport

# provider envelope 会再次转义最大 64 KiB 的 canonical `Plan`；1 MiB 保留有界放大余量。
PROVIDER_RESPONSE_BODY_LIMIT = 1024 * 1024
_MODEL_TOOL_DEFINITIONS = tuple(
    {
        "type": "function",
        "function": {
            "name": tool.name,
            "description": f"Mornlea read-only planner tool: {tool.name}",
            "parameters": tool.input_schema,
        },
    }
    for tool in mcp_tool_contracts()
    if tool.model_visible
)


async def _close_factory_resource(
    resource: httpx.AsyncClient | httpx.AsyncBaseTransport,
) -> None:
    """即使 factory 调用方同时取消，也先完成已创建资源的关闭。"""

    close_task = asyncio.create_task(resource.aclose())
    cancellation: asyncio.CancelledError | None = None
    while not close_task.done():
        try:
            await asyncio.shield(close_task)
        except asyncio.CancelledError as error:
            if cancellation is None:
                cancellation = error
    close_task.result()
    if cancellation is not None:
        raise cancellation from None


class ChatOpenAIPlannerModel:
    """禁用 provider retry、streaming 与多候选的模型适配器。"""

    def __init__(
        self,
        chat_model: BaseChatModel,
        *,
        http_client: httpx.AsyncClient | None = None,
    ) -> None:
        self._chat_model = chat_model
        self._tool_model: Runnable[Any, Any] = chat_model.bind_tools(
            deepcopy(list(_MODEL_TOOL_DEFINITIONS))
        )
        self._http_client = http_client

    @classmethod
    def create(
        cls,
        *,
        base_url: str,
        model: str,
        api_key: SecretStr,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> ChatOpenAIPlannerModel:
        async def provider_api_key() -> str:
            return api_key.get_secret_value()

        inner_transport = transport or httpx.AsyncHTTPTransport(retries=0)
        http_client = httpx.AsyncClient(
            headers={"Accept-Encoding": "identity"},
            follow_redirects=False,
            trust_env=False,
            timeout=httpx.Timeout(60.0),
            transport=BoundedAsyncTransport(
                inner_transport,
                maximum_bytes=PROVIDER_RESPONSE_BODY_LIMIT,
            ),
        )
        chat_model = ChatOpenAI(
            base_url=base_url,
            model=model,
            api_key=provider_api_key,
            http_async_client=http_client,
            max_retries=0,
            streaming=False,
            n=1,
        )
        return cls(chat_model, http_client=http_client)

    async def aclose(self) -> None:
        """关闭该适配器拥有的 provider client；直接注入模型时为空操作。"""

        if self._http_client is None:
            return
        client = self._http_client
        self._http_client = None
        await client.aclose()

    async def complete(
        self,
        messages: tuple[PlannerMessage, ...],
        *,
        allow_tools: bool,
    ) -> ModelOutput:
        converted = [self._convert_message(message) for message in messages]
        runnable: Runnable[Any, Any] = self._tool_model if allow_tools else self._chat_model
        try:
            response = await runnable.ainvoke(converted)
        except asyncio.CancelledError:
            raise
        except Exception:
            raise PlannerUnavailable from None
        if not isinstance(response, AIMessage):
            raise InvalidModelOutput
        calls: list[ModelToolCall] = []
        for call in response.tool_calls:
            call_id = call.get("id")
            name = call.get("name")
            arguments = call.get("args")
            if type(call_id) is not str or type(name) is not str:
                raise InvalidModelOutput
            calls.append(ModelToolCall(call_id=call_id, name=name, arguments=arguments))
        return ModelOutput(
            content=response.content,
            tool_calls=tuple(calls),
            invalid_tool_calls=bool(response.invalid_tool_calls),
        )

    @staticmethod
    def _convert_message(message: PlannerMessage) -> object:
        if message.role == "system":
            return SystemMessage(content=message.content)
        if message.role == "user":
            return HumanMessage(content=message.content)
        if message.role == "assistant":
            return AIMessage(
                content=message.content,
                tool_calls=[
                    {
                        "id": call.call_id,
                        "name": call.name,
                        "args": call.arguments,
                        "type": "tool_call",
                    }
                    for call in message.tool_calls
                ],
            )
        if message.role == "tool" and message.tool_call_id is not None:
            return ToolMessage(
                content=message.content,
                tool_call_id=message.tool_call_id,
                name=message.tool_name,
            )
        raise InvalidModelOutput


class ChatOpenAIDialogueModel:
    """把未绑定工具的原始 chat model 收窄为 Dialogue 端口。"""

    def __init__(self, chat_model: BaseChatModel) -> None:
        self._chat_model = chat_model

    async def complete(self, messages: tuple[DialogueMessage, ...]) -> object:
        converted: list[BaseMessage] = []
        for message in messages:
            if message.role == "system":
                converted.append(SystemMessage(content=message.content))
            elif message.role == "user":
                converted.append(HumanMessage(content=message.content))
            else:
                raise InvalidDialogueOutput
        try:
            response = await self._chat_model.ainvoke(converted)
        except asyncio.CancelledError:
            raise
        except Exception:
            raise DialogueUnavailable from None
        if not isinstance(response, AIMessage):
            raise InvalidDialogueOutput
        if (
            response.tool_calls
            or response.invalid_tool_calls
            or "function_call" in response.additional_kwargs
            or "tool_calls" in response.additional_kwargs
            or type(response.content) is not str
        ):
            raise InvalidDialogueOutput
        return response.content


class ChatOpenAIModelAdapters:
    """让 Planner 与 Dialogue 共享同一个有界 provider client。"""

    def __init__(
        self,
        chat_model: BaseChatModel,
        http_client: httpx.AsyncClient,
    ) -> None:
        self.planner = ChatOpenAIPlannerModel(chat_model)
        self.dialogue = ChatOpenAIDialogueModel(chat_model)
        self._http_client: httpx.AsyncClient | None = http_client

    @classmethod
    async def create(
        cls,
        *,
        base_url: str,
        model: str,
        api_key: SecretStr,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> ChatOpenAIModelAdapters:
        async def provider_api_key() -> str:
            return api_key.get_secret_value()

        inner_transport = transport or httpx.AsyncHTTPTransport(retries=0)
        bounded_transport = BoundedAsyncTransport(
            inner_transport,
            maximum_bytes=PROVIDER_RESPONSE_BODY_LIMIT,
        )
        try:
            http_client = httpx.AsyncClient(
                headers={"Accept-Encoding": "identity"},
                follow_redirects=False,
                trust_env=False,
                timeout=httpx.Timeout(60.0),
                transport=bounded_transport,
            )
        except BaseException:
            await _close_factory_resource(bounded_transport)
            raise
        try:
            chat_model = ChatOpenAI(
                base_url=base_url,
                model=model,
                api_key=provider_api_key,
                http_async_client=http_client,
                max_retries=0,
                streaming=False,
                n=1,
            )
            return cls(chat_model, http_client)
        except BaseException:
            await _close_factory_resource(http_client)
            raise

    async def aclose(self) -> None:
        if self._http_client is None:
            return
        client = self._http_client
        self._http_client = None
        await client.aclose()


__all__ = [
    "PROVIDER_RESPONSE_BODY_LIMIT",
    "ChatOpenAIDialogueModel",
    "ChatOpenAIModelAdapters",
    "ChatOpenAIPlannerModel",
]

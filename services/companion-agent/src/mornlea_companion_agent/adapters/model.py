"""把 OpenAI-compatible LangChain chat model 收窄为 Planner 领域端口。"""

from __future__ import annotations

import asyncio
from typing import Any

from langchain_core.language_models import BaseChatModel
from langchain_core.messages import AIMessage, HumanMessage, SystemMessage, ToolMessage
from langchain_core.runnables import Runnable
from langchain_openai import ChatOpenAI
from pydantic import BaseModel, SecretStr

from mornlea_companion_agent.domain.mcp_v1 import (
    EmptyInput,
    FindVisibleBlocksInput,
    InspectInventoryInput,
    QueryTerrainInput,
)
from mornlea_companion_agent.domain.planner import (
    InvalidModelOutput,
    ModelOutput,
    ModelToolCall,
    PlannerMessage,
    PlannerUnavailable,
)

_MODEL_TOOL_TYPES: dict[str, type[BaseModel]] = {
    "list_affordances": EmptyInput,
    "inspect_inventory": InspectInventoryInput,
    "find_visible_blocks": FindVisibleBlocksInput,
    "query_terrain": QueryTerrainInput,
}
_MODEL_TOOL_DEFINITIONS = tuple(
    {
        "type": "function",
        "function": {
            "name": name,
            "description": f"Mornlea read-only planner tool: {name}",
            "parameters": model.model_json_schema(),
        },
    }
    for name, model in _MODEL_TOOL_TYPES.items()
)


class ChatOpenAIPlannerModel:
    """禁用 provider retry、streaming 与多候选的模型适配器。"""

    def __init__(self, chat_model: BaseChatModel) -> None:
        self._chat_model = chat_model
        self._tool_model: Runnable[Any, Any] = chat_model.bind_tools(list(_MODEL_TOOL_DEFINITIONS))

    @classmethod
    def create(
        cls,
        *,
        base_url: str,
        model: str,
        api_key: SecretStr,
    ) -> ChatOpenAIPlannerModel:
        return cls(
            ChatOpenAI(
                base_url=base_url,
                model=model,
                api_key=api_key,
                max_retries=0,
                streaming=False,
                n=1,
            )
        )

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


__all__ = ["ChatOpenAIPlannerModel"]

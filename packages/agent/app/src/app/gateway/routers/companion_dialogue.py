"""伙伴台词路由：终结与非终结台词生成。"""

from __future__ import annotations

from collections.abc import Callable, Coroutine
from typing import Any

from fastapi import Request, Response
from harness.domain.http_v1 import DialogueNonterminalRequest, DialogueTerminalRequest

from app.gateway.http_gate import _HTTPFailure, _response
from app.gateway.runtime import _AgentRuntime


def build_dialogue_routes(
    runtime: _AgentRuntime,
) -> list[tuple[str, Callable[..., Coroutine[Any, Any, Response]], list[str]]]:
    """组装伙伴台词路由，保持终结性校验语义不变。"""

    async def dialogue(request: Request) -> Response:
        raw = getattr(request.state, "contract_request", None)
        if not isinstance(raw, (DialogueNonterminalRequest, DialogueTerminalRequest)):
            raise _HTTPFailure("internal_error")
        return _response(await runtime.run_dialogue(raw))

    return [
        ("/v1/dialogue", dialogue, ["POST"]),
    ]

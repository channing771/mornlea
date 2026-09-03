"""Dialogue graph 的稳定端口、预算与严格模型输出。"""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import ClassVar, Protocol

from pydantic import StrictFloat, StrictInt

from mornlea_companion_agent.domain.common import (
    DialogueLine,
    MemorySummary,
    StrictModel,
    canonical_json_bytes,
)
from mornlea_companion_agent.domain.memory import (
    LeaseIdentity,
    MemoryLookup,
    MemoryStateNonzero,
    MemoryStateZero,
)

MAX_DIALOGUE_OUTPUT_BYTES = 65_536


class DialogueFailure(Exception):
    """不携带 provider 或模型正文的稳定 Dialogue 失败。"""

    code: ClassVar[str] = "agent_unavailable"
    public_message: ClassVar[str] = "dialogue operation failed"

    def __init__(self) -> None:
        super().__init__(self.public_message)


class DialogueUnavailable(DialogueFailure):
    code = "agent_unavailable"
    public_message = "dialogue dependency is unavailable"


class DialogueDeadlineExceeded(DialogueFailure):
    code = "deadline_exceeded"
    public_message = "dialogue deadline exceeded"


class InvalidDialogueOutput(DialogueFailure):
    code = "invalid_model_output"
    public_message = "dialogue model output is invalid"


class DialogueLimits(StrictModel):
    timeout_seconds: StrictInt | StrictFloat = 30

    def model_post_init(self, context: object, /) -> None:
        del context
        if not 0 < float(self.timeout_seconds) <= 60:
            raise ValueError("timeout_seconds must be within the hard Dialogue limit")


@dataclass(frozen=True, slots=True)
class DialogueMessage:
    role: str
    content: str


class DialogueModel(Protocol):
    async def complete(self, messages: tuple[DialogueMessage, ...]) -> object: ...


class MemoryReader(Protocol):
    async def load(
        self,
        identity: LeaseIdentity,
        lookup: MemoryLookup,
    ) -> MemoryStateZero | MemoryStateNonzero: ...


class NonterminalModelOutput(StrictModel):
    line: DialogueLine


class TerminalModelOutput(StrictModel):
    line: DialogueLine
    summary: MemorySummary


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON object key")
        result[key] = value
    return result


def _reject_constant(value: str) -> object:
    del value
    raise ValueError("non-finite JSON number")


def parse_dialogue_output(
    content: object,
    *,
    terminal: bool,
) -> NonterminalModelOutput | TerminalModelOutput:
    """在解码前限制原始正文，并严格拒绝非单一 JSON object。"""

    if type(content) is not str:
        raise InvalidDialogueOutput
    try:
        encoded = content.encode("utf-8", errors="strict")
    except UnicodeError:
        raise InvalidDialogueOutput from None
    if len(encoded) > MAX_DIALOGUE_OUTPUT_BYTES:
        raise InvalidDialogueOutput
    try:
        decoded = json.loads(
            content,
            object_pairs_hook=_unique_object,
            parse_constant=_reject_constant,
        )
    except (
        json.JSONDecodeError,
        UnicodeError,
        ValueError,
        TypeError,
        RecursionError,
        OverflowError,
    ):
        raise InvalidDialogueOutput from None
    if type(decoded) is not dict:
        raise InvalidDialogueOutput
    output_type = TerminalModelOutput if terminal else NonterminalModelOutput
    try:
        output = output_type.model_validate(decoded)
        if len(canonical_json_bytes(output)) > MAX_DIALOGUE_OUTPUT_BYTES:
            raise ValueError("canonical Dialogue output is too large")
    except (ValueError, TypeError, UnicodeError, RecursionError, OverflowError):
        raise InvalidDialogueOutput from None
    return output


__all__ = [
    "DialogueDeadlineExceeded",
    "DialogueFailure",
    "DialogueLimits",
    "DialogueMessage",
    "DialogueModel",
    "DialogueUnavailable",
    "InvalidDialogueOutput",
    "MAX_DIALOGUE_OUTPUT_BYTES",
    "MemoryReader",
    "NonterminalModelOutput",
    "TerminalModelOutput",
    "parse_dialogue_output",
]

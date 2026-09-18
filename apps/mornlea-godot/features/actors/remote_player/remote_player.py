"""Map typed remote-player snapshots onto a fixed Godot instance pool."""

from __future__ import annotations

import math
from typing import Any, Protocol, runtime_checkable

try:
    from py4godot.classes import gdclass
    from py4godot.classes.core import Vector3
    from py4godot.classes.Node import Node
except ImportError:  # pragma: no cover - source-level checks run without Godot

    def gdclass(value: Any) -> Any:  # type: ignore[misc]
        return value

    class Node:  # type: ignore[no-redef]
        pass

    class Vector3:  # type: ignore[no-redef]
        @staticmethod
        def new3(x: float, y: float, z: float) -> tuple[float, float, float]:
            return (x, y, z)


MAX_REMOTE_PLAYERS = 7


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


@runtime_checkable
class _ArrayView(Protocol):
    def size(self) -> int: ...

    def get(self, index: int) -> object: ...


class _EntityValue:
    """Validated immutable-in-practice projection of one typed bridge value."""

    def __init__(
        self,
        player_id: str,
        dimension: int,
        position: tuple[float, float, float],
        yaw: float,
        pitch: float,
    ) -> None:
        self.player_id = player_id
        self.dimension = dimension
        self.position = position
        self.yaw = yaw
        self.pitch = pitch


def _word(value: object) -> int:
    return value if isinstance(value, int) and not isinstance(value, bool) else -1


def _number(value: object) -> float | None:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        number = float(value)
        return number if math.isfinite(number) else None
    return None


def _array_items(value: object) -> list[object] | None:
    if isinstance(value, (list, tuple)):
        return list(value)
    if isinstance(value, _ArrayView):
        size = value.size()
        if size < 0 or size > MAX_REMOTE_PLAYERS:
            return None
        return [value.get(index) for index in range(size)]
    return None


def _text_value(value: object) -> str | None:
    """Normalize typed-view text before applying the strict UUID validator."""
    if isinstance(value, str):
        return value
    try:
        return str(value)
    except Exception:
        return None


def _canonical_player_id(value: object) -> str | None:
    value_text = _text_value(value)
    if value_text is None or len(value_text) != 36:
        return None
    if any(value_text[index] != "-" for index in (8, 13, 18, 23)):
        return None
    compact = value_text.replace("-", "")
    if len(compact) != 32 or compact.lower() != compact:
        return None
    if any(character not in "0123456789abcdef" for character in compact):
        return None
    if value_text[14] != "4" or value_text[19] not in "89ab":
        return None
    return value_text


def _entity_value(value: object) -> _EntityValue | None:
    if not isinstance(value, _DictionaryView) or _word(value["kind"]) != 1:
        return None
    player_id = _canonical_player_id(value["player_id"])
    dimension = _word(value["dimension"])
    numbers = [
        _number(value[key]) for key in ("position_x", "position_y", "position_z", "yaw", "pitch")
    ]
    if player_id is None or dimension not in (0, 1) or any(number is None for number in numbers):
        return None
    position_x, position_y, position_z, yaw, pitch = numbers
    assert position_x is not None and position_y is not None and position_z is not None
    assert yaw is not None and pitch is not None
    return _EntityValue(
        player_id,
        dimension,
        (position_x, position_y, position_z),
        yaw,
        pitch,
    )


@gdclass
class remote_player(Node):
    """Own a seven-slot pool while Go remains the entity and interpolation owner."""

    _slots: list[Node]
    _player_slots: dict[str, int]
    _last_revision: int
    _last_server_tick: int
    _epoch: int
    _last_error: str
    _last_callback_units: int

    def _ready(self) -> None:
        self._slots = []
        for index in range(MAX_REMOTE_PLAYERS):
            slot = self.get_node_or_null(f"Player{index}")
            if slot is not None:
                self._slots.append(slot)
        self._player_slots = {}
        self._last_revision = 0
        self._last_server_tick = 0
        self._epoch = 0
        self._last_error = ""
        self._last_callback_units = 0
        self._clear_slots()

    def validate_pool(self) -> str:
        return (
            "" if len(self._slots) == MAX_REMOTE_PLAYERS else "the remote-player pool is incomplete"
        )

    def reset_feature(self, epoch: int) -> None:
        self._clear_slots()
        self._player_slots = {}
        self._last_revision = 0
        self._last_server_tick = 0
        self._epoch = epoch
        self._last_error = ""
        self._last_callback_units = 0

    def deactivate_feature(self) -> None:
        self.reset_feature(0)

    def apply_typed_frame(self, typed: _DictionaryView) -> str:
        """Validate the whole entity snapshot before changing any pooled instance."""
        if len(self._slots) != MAX_REMOTE_PLAYERS:
            return self._fail("the remote-player pool is incomplete")
        if _word(typed["status"]) != 0:
            self._clear_presentation()
            return self._fail("the typed entity frame is unavailable")
        if _word(typed["phase"]) != 4:
            self._clear_presentation()
            return ""
        revision = _word(typed["revision"])
        epoch = _word(typed["epoch"])
        server_tick = _word(typed["entity_server_tick"])
        raw_entities = _array_items(typed["entities"])
        if revision <= 0 or epoch <= 0 or server_tick < 0 or raw_entities is None:
            return self._fail("the typed entity frame identity is invalid")
        if len(raw_entities) > MAX_REMOTE_PLAYERS:
            return self._fail("the typed entity frame exceeds the pool capacity")
        entities: list[_EntityValue] = []
        seen: set[str] = set()
        for raw_entity in raw_entities:
            entity = _entity_value(raw_entity)
            if entity is None or entity.player_id in seen:
                return self._fail("the typed entity frame contains an invalid or duplicate player")
            seen.add(entity.player_id)
            entities.append(entity)
        if self._epoch != 0 and epoch < self._epoch:
            return self._fail("the typed entity frame epoch moved backward")
        if epoch == self._epoch and revision < self._last_revision:
            return self._fail("the typed entity frame revision moved backward")
        if epoch == self._epoch and server_tick < self._last_server_tick:
            return self._fail("the typed entity server tick moved backward")
        if epoch == self._epoch and revision == self._last_revision:
            return ""
        if self._epoch != 0 and epoch > self._epoch:
            self._clear_slots()
            self._player_slots = {}

        next_slots: dict[str, int] = {
            player_id: slot for player_id, slot in self._player_slots.items() if player_id in seen
        }
        free_slots = [
            index for index in range(MAX_REMOTE_PLAYERS) if index not in next_slots.values()
        ]
        for entity in entities:
            if entity.player_id not in next_slots:
                next_slots[entity.player_id] = free_slots.pop(0)
        by_slot = {next_slots[entity.player_id]: entity for entity in entities}
        previous_slots = set(self._player_slots.values())
        callback_units = 0
        for slot_index, slot in enumerate(self._slots):
            entity = by_slot.get(slot_index)
            if entity is None:
                if slot_index in previous_slots:
                    slot.call("set_visible", False)
                    slot.set_meta("mornlea_player_id", "")
                    callback_units += 1
                continue
            slot.call("set_position", Vector3.new3(*entity.position))
            slot.call("set_rotation", Vector3.new3(-entity.pitch, entity.yaw, 0.0))
            slot.call("set_visible", True)
            slot.set_meta("mornlea_player_id", entity.player_id)
            slot.set_meta("mornlea_dimension", entity.dimension)
            callback_units += 1
        self._player_slots = next_slots
        self._last_revision = revision
        self._last_server_tick = server_tick
        self._epoch = epoch
        self._last_callback_units = callback_units
        self._last_error = ""
        return ""

    def active_count(self) -> int:
        return len(self._player_slots)

    def last_callback_units(self) -> int:
        return self._last_callback_units

    def last_error(self) -> str:
        return self._last_error

    def _clear_presentation(self) -> None:
        self._clear_slots()
        self._player_slots = {}
        self._last_revision = 0
        self._last_server_tick = 0
        self._last_callback_units = 0

    def _clear_slots(self) -> None:
        for slot in self._slots:
            slot.call("set_visible", False)
            slot.set_meta("mornlea_player_id", "")

    def _fail(self, message: str) -> str:
        self._last_error = message
        return message

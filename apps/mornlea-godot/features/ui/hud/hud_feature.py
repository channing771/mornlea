"""Render confirmed minimum-loop player values in a bounded Control HUD."""

from __future__ import annotations

from typing import Any, Protocol, runtime_checkable

try:
    from py4godot.classes import gdclass
    from py4godot.classes.Control import Control
    from py4godot.classes.Node import Node
except ImportError:  # pragma: no cover - source-level checks run without Godot

    def gdclass(value: Any) -> Any:  # type: ignore[misc]
        return value

    class Control:  # type: ignore[no-redef]
        pass

    class Node:  # type: ignore[no-redef]
        pass


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


STATUS_OK = 0
PHASE_LABELS = {
    0: "Not ready",
    1: "Connecting",
    2: "Logging in",
    3: "Loading",
    4: "Play",
    5: "Disconnected",
}
MAX_HEALTH = 20
MAX_HUNGER = 20
MAX_OXYGEN = 300


def _word(value: object) -> int:
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class hud_feature(Control):
    """Own display labels only; it never submits gameplay actions."""

    _health_label: Node | None
    _hunger_label: Node | None
    _oxygen_label: Node | None
    _connection_label: Node | None
    _pilot_label: Node | None
    _active: bool
    _last_error: str

    def _ready(self) -> None:
        self._health_label = self.get_node_or_null("HealthLabel")
        self._hunger_label = self.get_node_or_null("HungerLabel")
        self._oxygen_label = self.get_node_or_null("OxygenLabel")
        self._connection_label = self.get_node_or_null("ConnectionLabel")
        self._pilot_label = self.get_node_or_null("PilotLabel")
        self._active = False
        self._last_error = ""
        self._set_text(self._pilot_label, "Limited Godot pilot")
        self._set_visible(self._pilot_label, True)
        self._hide_values()
        self._set_text(self._connection_label, "Connection: Not ready")

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "ui.hud" else "unexpected HUD feature ID"

    def bind_host(self, _services_path: str) -> str:
        if any(
            label is None
            for label in (
                self._health_label,
                self._hunger_label,
                self._oxygen_label,
                self._connection_label,
                self._pilot_label,
            )
        ):
            return "the minimum HUD labels are incomplete"
        return ""

    def activate_feature(self, _epoch: int) -> str:
        self._active = True
        self._set_visible(self._pilot_label, True)
        return ""

    def reset_feature(self, _epoch: int) -> None:
        self._hide_values()
        self._set_text(self._connection_label, "Connection: Not ready")

    def deactivate_feature(self) -> None:
        self._hide_values()
        self._active = False

    def apply_typed_frame(self, frame: _DictionaryView) -> str:
        """Display only values confirmed by one complete typed frame."""
        if not self._active:
            return ""
        status = _word(frame["status"])
        phase = _word(frame["phase"])
        self._set_text(
            self._connection_label, f"Connection: {PHASE_LABELS.get(phase, 'Unavailable')}"
        )
        if status != STATUS_OK:
            self._hide_values()
            return ""
        if phase != 4 or not bool(frame["hud_ready"]):
            self._hide_values()
            return ""
        health = _word(frame["health"])
        hunger = _word(frame["hunger"])
        oxygen = _word(frame["oxygen"])
        if (
            not 0 <= health <= MAX_HEALTH
            or not 0 <= hunger <= MAX_HUNGER
            or not 0 <= oxygen <= MAX_OXYGEN
        ):
            self._hide_values()
            return "the confirmed HUD values are outside the supported domain"
        self._set_text(self._health_label, f"Health: {health}/{MAX_HEALTH}")
        self._set_text(self._hunger_label, f"Hunger: {hunger}/{MAX_HUNGER}")
        self._set_text(self._oxygen_label, f"Oxygen: {oxygen}/{MAX_OXYGEN}")
        for label in (self._health_label, self._hunger_label, self._oxygen_label):
            self._set_visible(label, True)
        self._last_error = ""
        return ""

    def last_error(self) -> str:
        return self._last_error

    def _hide_values(self) -> None:
        for label in (self._health_label, self._hunger_label, self._oxygen_label):
            self._set_visible(label, False)

    @staticmethod
    def _set_text(label: Node | None, text: str) -> None:
        if label is not None:
            label.call("set_text", text)

    @staticmethod
    def _set_visible(label: Node | None, visible: bool) -> None:
        if label is not None:
            label.call("set_visible", visible)

"""Collect bounded desktop intent and pass only known client-core semantics."""

from __future__ import annotations

import math
from typing import Any

try:
    from py4godot.classes import gdclass
    from py4godot.classes.Node import Node
except ImportError:  # pragma: no cover - source-level input gate fallback

    def gdclass(value: Any) -> Any:  # type: ignore[misc]
        return value

    class Node:  # type: ignore[no-redef]
        pass


SUPPORTED_ACTIONS = frozenset(
    {
        "move_forward",
        "move_back",
        "move_left",
        "move_right",
        "jump",
        "sprint",
        "sneak",
        "primary",
        "secondary",
    }
)
LOCAL_ACTIONS = frozenset({"f5", "escape"})
UNSUPPORTED_ACTIONS = frozenset({"hotbar", "toggle_camera", "ui_cancel"})


def _finite(value: float) -> float:
    return value if math.isfinite(value) else 0.0


@gdclass
class desktop_input_feature(Node):
    """Own cursor/focus state and map Godot device state to semantic events."""

    _services: Node | None
    _epoch: int
    _captured: bool
    _focused: bool
    _yaw: float
    _pitch: float
    _mouse_baseline: tuple[float, float] | None
    _f5_down: bool
    _escape_down: bool

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0
        self._captured = False
        self._focused = True
        self._yaw = 0.0
        self._pitch = 0.0
        self._mouse_baseline = None
        self._f5_down = False
        self._escape_down = False

    def validate_feature(self, feature_id: str) -> str:
        return (
            "" if feature_id == "platform.desktop.input" else "unexpected desktop-input feature ID"
        )

    def bind_host(self, services: Node) -> str:
        if not services.has_method("host_protocol_version"):
            return "host protocol identity is missing"
        self._services = services
        return ""

    def activate_feature(self, epoch: int) -> str:
        self._epoch = epoch
        self._focused = True
        self.refresh_mouse_baseline()
        return ""

    def reset_feature(self, epoch: int) -> None:
        self._epoch = epoch
        self.neutral_input()
        self.refresh_mouse_baseline()

    def deactivate_feature(self) -> None:
        self.neutral_input()
        self.release_cursor()
        self._services = None
        self._epoch = 0
        self._f5_down = False
        self._escape_down = False

    def capture_cursor(self) -> None:
        self._captured = True
        self.refresh_mouse_baseline()

    def release_cursor(self) -> None:
        self._captured = False
        self._mouse_baseline = None

    def f5_down(self) -> bool:
        """Return the host-local camera-cycle level for edge handling."""
        return self._f5_down

    def escape_down(self) -> bool:
        """Return the host-local escape level without sending a command."""
        return self._escape_down

    def on_focus_changed(self, focused: bool) -> None:
        self._focused = focused
        if not focused:
            self.release_cursor()
            self.neutral_input()
        elif self._captured:
            self.refresh_mouse_baseline()

    def refresh_mouse_baseline(self) -> None:
        self._mouse_baseline = None

    def neutral_input(self) -> int:
        return self._submit(0, 0, False, False, False, False, False, 0.0, 0.0)

    def collect_input(self, source: Any) -> int:
        if not self._focused:
            return self.neutral_input()
        self._f5_down = bool(source.is_action_pressed("f5"))
        self._escape_down = bool(source.is_action_pressed("escape"))
        if self._escape_down:
            self.release_cursor()
            return self.neutral_input()
        move_x = int(source.is_action_pressed("move_right")) - int(
            source.is_action_pressed("move_left")
        )
        move_z = int(source.is_action_pressed("move_forward")) - int(
            source.is_action_pressed("move_back")
        )
        dx, dy = source.mouse_delta()
        if self._mouse_baseline is None:
            self._mouse_baseline = (float(dx), float(dy))
            dx = dy = 0.0
        self._yaw = _finite(self._yaw + float(dx))
        self._pitch = max(-1.5707964, min(1.5707964, _finite(self._pitch + float(dy))))
        return self._submit(
            move_x,
            move_z,
            bool(source.is_action_pressed("jump")),
            bool(source.is_action_pressed("primary")),
            bool(source.is_action_pressed("secondary")),
            bool(source.is_action_pressed("sprint")),
            bool(source.is_action_pressed("sneak")),
            self._yaw,
            self._pitch,
        )

    def _submit(
        self,
        move_x: int,
        move_z: int,
        jump: bool,
        mining: bool,
        eating: bool,
        sprinting: bool,
        sneaking: bool,
        yaw: float,
        pitch: float,
    ) -> int:
        if self._services is None or not self._services.has_method("session_submit_semantic"):
            return 8
        result = self._services.call(
            "session_submit_semantic",
            move_x,
            move_z,
            jump,
            mining,
            eating,
            sprinting,
            sneaking,
            yaw,
            pitch,
        )
        return int(result) if isinstance(result, int) and not isinstance(result, bool) else 8

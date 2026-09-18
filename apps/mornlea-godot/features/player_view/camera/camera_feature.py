"""Map the typed runtime camera snapshot onto one Godot Camera3D."""

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


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


def _number(value: object) -> float | None:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        result = float(value)
        return result if math.isfinite(result) else None
    return None


def _word(value: object) -> int:
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class camera_feature(Node):
    """Own only the Godot-side camera mapping and its bounded presentation state."""

    _bridge: Node | None
    _camera: Node | None
    _aspect_override: float | None
    _ui_blocked: bool
    _ready_state: bool
    _last_error: str
    _apply_count: int

    def _ready(self) -> None:
        self._bridge = None
        self._camera = self.get_node_or_null("Camera3D")
        self._aspect_override = None
        self._ui_blocked = False
        self._ready_state = False
        self._last_error = ""
        self._apply_count = 0
        self._set_visible(False)

    def bind_host(self, bridge_path: str) -> str:
        bridge = self.get_node_or_null(bridge_path)
        if bridge is None or not bridge.has_method("session_frame_typed"):
            return "the typed frame bridge is missing"
        if self._camera is None:
            return "the Camera3D presentation node is missing"
        self._bridge = bridge
        return ""

    def activate_feature(self, _epoch: int) -> str:
        if self._bridge is None:
            return "camera activation before bind"
        self._hide_for_boundary()
        return ""

    def reset_feature(self, _epoch: int) -> None:
        self._hide_for_boundary()

    def deactivate_feature(self) -> None:
        self._hide_for_boundary()
        self._bridge = None

    def set_ui_blocked(self, blocked: bool) -> None:
        self._ui_blocked = blocked
        if blocked:
            self._hide_for_boundary()

    def resize_aspect(self, width: float, height: float) -> str:
        if not math.isfinite(width) or not math.isfinite(height) or width <= 0 or height <= 0:
            return "viewport dimensions must be finite and positive"
        self._aspect_override = width / height
        return ""

    def apply_frame(self) -> str:
        """Apply one immutable typed frame with one bounded bridge call."""
        bridge = self._bridge
        if bridge is None:
            return self._fail("the typed frame bridge is not bound")
        typed = bridge.call("session_frame_typed")
        if not isinstance(typed, _DictionaryView):
            return self._fail("the typed frame answer is not a dictionary")
        return self.apply_typed_frame(typed)

    def apply_typed_frame(self, typed: _DictionaryView) -> str:
        """Apply a frame already sampled by the player-view coordinator."""
        if _word(typed["status"]) != 0:
            self._hide_for_boundary()
            return self._fail("the typed frame is unavailable")
        phase = _word(typed["phase"])
        if phase != 4 or self._ui_blocked or not bool(typed["camera_ready"]):
            self._hide_for_boundary()
            return ""
        values = [
            _number(typed[key])
            for key in (
                "position_x",
                "position_y",
                "position_z",
                "yaw",
                "pitch",
                "fov_y",
                "aspect",
                "near",
                "far",
            )
        ]
        if any(value is None for value in values):
            return self._fail("the typed camera contains a non-finite value")
        position_x, position_y, position_z, yaw, pitch, fov_y, aspect, near, far = values
        assert position_x is not None and position_y is not None and position_z is not None
        assert yaw is not None and pitch is not None and fov_y is not None
        assert aspect is not None and near is not None and far is not None
        if abs(pitch) > math.pi / 2 - 0.01 or fov_y <= 0 or fov_y >= math.pi:
            return self._fail("the typed camera is outside the supported domain")
        if near <= 0 or far <= near:
            return self._fail("the typed camera clipping planes are invalid")
        camera = self._camera
        if camera is None:
            return self._fail("the Camera3D presentation node is missing")
        camera.call("set_position", Vector3.new3(position_x, position_y, position_z))
        camera.call("set_rotation", Vector3.new3(-pitch, yaw, 0.0))
        camera.call("set_fov", math.degrees(fov_y))
        camera.call("set_near", near)
        camera.call("set_far", far)
        # The viewport override is host-owned resize state. It is applied only
        # after validation and never fed back as authoritative gameplay input.
        effective_aspect = self._aspect_override if self._aspect_override is not None else aspect
        camera.set_meta("mornlea_aspect", effective_aspect)
        camera.call("set_current", True)
        self._ready_state = True
        self._last_error = ""
        self._apply_count += 1
        return ""

    def camera_ready(self) -> bool:
        return self._ready_state

    def last_error(self) -> str:
        return self._last_error

    def apply_count(self) -> int:
        return self._apply_count

    def _hide_for_boundary(self) -> None:
        self._ready_state = False
        self._set_visible(False)

    def _set_visible(self, visible: bool) -> None:
        if self._camera is not None:
            self._camera.call("set_current", visible)

    def _fail(self, message: str) -> str:
        self._hide_for_boundary()
        self._last_error = message
        return message

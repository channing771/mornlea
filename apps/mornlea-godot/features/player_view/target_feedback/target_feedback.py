"""Map typed block-target feedback onto a depth-tested outline and label."""

from __future__ import annotations

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


def _word(value: object) -> int:
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class target_feedback(Node):
    """Own reversible target presentation; it never emits gameplay commands."""

    _bridge: Node | None
    _outline: Node | None
    _label: Node | None
    _ui_blocked: bool
    _visible: bool
    _last_name: str
    _last_error: str

    def _ready(self) -> None:
        self._bridge = None
        self._outline = self.get_node_or_null("Outline")
        self._label = self.get_node_or_null("TargetLabel")
        self._ui_blocked = False
        self._visible = False
        self._last_name = ""
        self._last_error = ""
        self._hide()

    def bind_host(self, bridge_path: str) -> str:
        bridge = self.get_node_or_null(bridge_path)
        if bridge is None or not bridge.has_method("session_frame_typed"):
            return "the typed target bridge is missing"
        if self._outline is None:
            return "the target outline node is missing"
        self._bridge = bridge
        return ""

    def activate_feature(self, _epoch: int) -> str:
        if self._bridge is None:
            return "target activation before bind"
        self._hide()
        return ""

    def reset_feature(self, _epoch: int) -> None:
        self._hide()

    def deactivate_feature(self) -> None:
        self._hide()
        self._bridge = None

    def set_ui_blocked(self, blocked: bool) -> None:
        self._ui_blocked = blocked
        if blocked:
            self._hide()

    def apply_frame(self) -> str:
        bridge = self._bridge
        if bridge is None:
            return self._fail("the typed target bridge is not bound")
        typed = bridge.call("session_frame_typed")
        if not isinstance(typed, _DictionaryView):
            return self._fail("the typed target answer is not a dictionary")
        return self.apply_typed_frame(typed)

    def apply_typed_frame(self, typed: _DictionaryView) -> str:
        """Apply a frame already sampled by the player-view coordinator."""
        if _word(typed["status"]) != 0:
            return self._fail("the typed target is unavailable")
        if _word(typed["phase"]) != 4 or self._ui_blocked or not bool(typed["camera_ready"]):
            self._hide()
            return ""
        if not bool(typed["target_visible"]):
            self._hide()
            return ""
        name = typed["target_name"]
        if not isinstance(name, str) or not name:
            return self._fail("the visible target has no localized name")
        coordinates = [typed[key] for key in ("target_x", "target_y", "target_z")]
        if any(not isinstance(value, int) or isinstance(value, bool) for value in coordinates):
            return self._fail("the visible target coordinates are invalid")
        target_x, target_y, target_z = coordinates
        assert isinstance(target_x, int) and not isinstance(target_x, bool)
        assert isinstance(target_y, int) and not isinstance(target_y, bool)
        assert isinstance(target_z, int) and not isinstance(target_z, bool)
        outline = self._outline
        if outline is None:
            return self._fail("the target outline node is missing")
        outline.call(
            "set_position",
            Vector3.new3(
                float(target_x) + 0.5,
                float(target_y) + 0.5,
                float(target_z) + 0.5,
            ),
        )
        outline.call("set_visible", True)
        if self._label is not None:
            self._label.call("set_text", name)
            self._label.call("set_visible", True)
        self._visible = True
        self._last_name = name
        self._last_error = ""
        return ""

    def visible(self) -> bool:
        return self._visible

    def target_name(self) -> str:
        return self._last_name

    def last_error(self) -> str:
        return self._last_error

    def _hide(self) -> None:
        self._visible = False
        self._last_name = ""
        if self._outline is not None:
            self._outline.call("set_visible", False)
        if self._label is not None:
            self._label.call("set_visible", False)

    def _fail(self, message: str) -> str:
        self._hide()
        self._last_error = message
        return message

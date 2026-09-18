"""Own camera and target presentation lifecycle without prediction semantics."""

from __future__ import annotations

from typing import Protocol, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@runtime_checkable
class _DictionaryView(Protocol):
    def __getitem__(self, key: str) -> object: ...


@gdclass
class player_view_feature(Node):
    """Reserve the independently replaceable player-view presentation boundary."""

    _services: Node | None
    _camera: Node | None
    _target: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._camera = self.get_node_or_null("Camera")
        self._target = self.get_node_or_null("TargetFeedback")
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "player_view" else "unexpected player-view feature ID"

    def bind_host(self, services_path: str) -> str:
        # Child components receive the bridge path and acquire the typed node
        # themselves; project-class objects do not cross Python module calls.
        if self._camera is None or self._target is None:
            return "player-view components are missing"
        failure = self._camera.call("bind_host", services_path)
        if not isinstance(failure, str) or failure:
            return failure if isinstance(failure, str) and failure else "camera bind failed"
        failure = self._target.call("bind_host", services_path)
        if not isinstance(failure, str) or failure:
            return failure if isinstance(failure, str) and failure else "target bind failed"
        self._services = self.get_node_or_null(services_path)
        if self._services is None:
            return "typed bridge service node is missing"
        return ""

    def activate_feature(self, epoch: int) -> str:
        if self._camera is None or self._target is None:
            return "activation before player-view bind"
        self._epoch = epoch
        for component in (self._camera, self._target):
            failure = component.call("activate_feature", epoch)
            if isinstance(failure, str) and failure:
                return failure
        return ""

    def reset_feature(self, epoch: int) -> None:
        self._epoch = epoch
        for component in (self._camera, self._target):
            if component is not None:
                component.call("reset_feature", epoch)

    def deactivate_feature(self) -> None:
        for component in (self._target, self._camera):
            if component is not None:
                component.call("deactivate_feature")
        self._services = None
        self._epoch = 0

    def set_ui_blocked(self, blocked: bool) -> None:
        for component in (self._camera, self._target):
            if component is not None:
                component.call("set_ui_blocked", blocked)

    def apply_frame(self) -> str:
        """Apply camera and target from one bounded typed frame observation."""
        if self._camera is None or self._target is None or self._services is None:
            return "player-view components are missing"
        typed = self._services.call("session_frame_typed")
        if not isinstance(typed, _DictionaryView):
            return "the typed frame answer is not a dictionary"
        camera_failure = self._camera.call("apply_typed_frame", typed)
        target_failure = self._target.call("apply_typed_frame", typed)
        for failure in (camera_failure, target_failure):
            if isinstance(failure, str) and failure:
                return failure
        return ""

    def apply_typed_frame(self, typed: _DictionaryView) -> str:
        """Fan one host-sampled frame to both view components."""
        if self._camera is None or self._target is None:
            return "player-view components are missing"
        camera_failure = self._camera.call("apply_typed_frame", typed)
        target_failure = self._target.call("apply_typed_frame", typed)
        for failure in (camera_failure, target_failure):
            if isinstance(failure, str) and failure:
                return failure
        return ""

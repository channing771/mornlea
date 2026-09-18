"""Own the pilot UI presentation lifecycle without gameplay commands."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class ui_feature(Node):
    """Reserve the replaceable UI boundary for confirmed typed view models."""

    _services: Node | None
    _hud: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._hud = self.get_node_or_null("HUD")
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "ui" else "unexpected UI feature ID"

    def bind_host(self, services_path: str) -> str:
        # UI nodes display confirmed values and acquire the bridge by scene path.
        services = self.get_node_or_null(services_path)
        if services is None:
            return "typed bridge service node is missing"
        # The native bridge's negotiated family table is the production
        # identity; UI must not depend on the retired Python facade surface.
        if not services.has_method("feature_families_json"):
            return "typed bridge family identity is missing"
        if self._hud is None:
            return "the minimum HUD is missing"
        failure = self._hud.call("bind_host", services_path)
        if isinstance(failure, str) and failure:
            return failure
        self._services = services
        return ""

    def activate_feature(self, epoch: int) -> str:
        self._epoch = epoch
        if self._hud is not None:
            failure = self._hud.call("activate_feature", epoch)
            if isinstance(failure, str) and failure:
                return failure
        return ""

    def reset_feature(self, epoch: int) -> None:
        self._epoch = epoch
        if self._hud is not None:
            self._hud.call("reset_feature", epoch)

    def deactivate_feature(self) -> None:
        self._services = None
        if self._hud is not None:
            self._hud.call("deactivate_feature")
        self._epoch = 0

    def apply_typed_frame(self, frame: object) -> str:
        if self._hud is None:
            return "the minimum HUD is missing"
        result = self._hud.call("apply_typed_frame", frame)
        return result if isinstance(result, str) else "the HUD apply result is invalid"

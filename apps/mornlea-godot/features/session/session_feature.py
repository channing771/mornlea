"""Own the session presentation lifecycle without opening a network connection."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class session_feature(Node):
    """Reserve the coarse session boundary for the later typed connection feature."""

    _services: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "session" else "unexpected session feature ID"

    def bind_host(self, services: Node) -> str:
        # Skeletons bind only to typed host identity; they never reach TCP or client state.
        if not services.has_method("host_protocol_version"):
            return "host protocol identity is missing"
        self._services = services
        return ""

    def activate_feature(self, epoch: int) -> str:
        self._epoch = epoch
        return ""

    def reset_feature(self, epoch: int) -> None:
        self._epoch = epoch

    def deactivate_feature(self) -> None:
        self._services = None
        self._epoch = 0

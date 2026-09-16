"""Own the pilot UI presentation lifecycle without gameplay commands."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class ui_feature(Node):
    """Reserve the replaceable UI boundary for confirmed typed view models."""

    _services: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "ui" else "unexpected UI feature ID"

    def bind_host(self, services: Node) -> str:
        # UI nodes will submit semantic actions through the host, never guessed packets.
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

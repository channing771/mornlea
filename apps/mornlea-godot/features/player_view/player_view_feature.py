"""Own camera and target presentation lifecycle without prediction semantics."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class player_view_feature(Node):
    """Reserve the independently replaceable player-view presentation boundary."""

    _services: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "player_view" else "unexpected player-view feature ID"

    def bind_host(self, services: Node) -> str:
        # The future implementation consumes typed snapshots through host services only.
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

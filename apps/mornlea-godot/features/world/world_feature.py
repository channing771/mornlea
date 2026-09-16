"""Own the world presentation lifecycle while bulk terrain work remains native."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class world_feature(Node):
    """Reserve the coarse world boundary without implementing numerical gameplay."""

    _services: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "world" else "unexpected world feature ID"

    def bind_host(self, services: Node) -> str:
        # Python will orchestrate bounded resources, not expand meshes or parse packets.
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

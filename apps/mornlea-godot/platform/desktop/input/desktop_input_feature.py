"""Own desktop device adaptation without defining gameplay-input rules."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class desktop_input_feature(Node):
    """Reserve keyboard and mouse adaptation for macOS, Windows, and Linux."""

    _services: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        expected = "platform.desktop.input"
        return "" if feature_id == expected else "unexpected desktop-input feature ID"

    def bind_host(self, services: Node) -> str:
        # No touch, sensor, virtual-joystick, or mobile lifecycle surface is reserved.
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

"""Reserve desktop audio ownership while the pilot remains silent."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class desktop_audio_feature(Node):
    """Fail closed if the disabled pilot audio skeleton is activated manually."""

    _services: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        expected = "platform.desktop.audio"
        return "" if feature_id == expected else "unexpected desktop-audio feature ID"

    def bind_host(self, services: Node) -> str:
        # Cue generation stays in Go and no Godot audio device opens in the pilot.
        if not services.has_method("host_protocol_version"):
            return "host protocol identity is missing"
        self._services = services
        return ""

    def activate_feature(self, epoch: int) -> str:
        self._epoch = epoch
        return "desktop audio is disabled in the pilot"

    def reset_feature(self, epoch: int) -> None:
        self._epoch = epoch

    def deactivate_feature(self) -> None:
        self._services = None
        self._epoch = 0

"""Own cross-desktop window lifecycle behind one feature boundary."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class desktop_lifecycle_feature(Node):
    """Reserve focus, capture, resize, and exit adaptation for desktop hosts."""

    _services: Node | None
    _epoch: int

    def _ready(self) -> None:
        self._services = None
        self._epoch = 0

    def validate_feature(self, feature_id: str) -> str:
        expected = "platform.desktop.lifecycle"
        return "" if feature_id == expected else "unexpected desktop-lifecycle feature ID"

    def bind_host(self, services: Node) -> str:
        # Godot provides the OS APIs; this adapter will emit Mornlea desktop semantics.
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

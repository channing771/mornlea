"""Typed bridge fixture used to validate host protocol and family negotiation."""

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class stub_bridge(Node):
    def host_protocol_version(self) -> str:
        return "1.0"

    def feature_family_version(self, family_id: str) -> str:
        if family_id == "available":
            return "1.0"
        return ""

    def bridge_identity(self) -> str:
        return "typed-test-bridge"

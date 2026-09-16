"""Feature fixture that fails only after validation and typed bridge binding."""

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class stub_fail_activate(Node):
    def validate_feature(self, feature_id: str) -> str:
        if not feature_id:
            return "feature ID is empty"
        return ""

    def bind_host(self, services: Node) -> str:
        if not services.has_method("bridge_identity"):
            return "typed bridge identity is missing"
        return ""

    def activate_feature(self, _epoch: int) -> str:
        return "synthetic activation failure"

    def reset_feature(self, _epoch: int) -> None:
        pass

    def deactivate_feature(self) -> None:
        pass

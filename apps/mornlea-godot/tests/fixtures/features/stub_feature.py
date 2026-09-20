"""Successful feature fixture implementing the complete host lifecycle."""

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class stub_feature(Node):
    def validate_feature(self, feature_id: str) -> str:
        if not feature_id:
            return "feature ID is empty"
        return ""

    def bind_host(self, services_path: str) -> str:
        # Features resolve the typed service object from the scene path the
        # host hands over; project-class objects cannot cross module calls.
        services = self.get_node(services_path)
        if services is None:
            return "typed bridge service is missing"
        if not services.has_method("bridge_identity"):
            return "typed bridge identity is missing"
        if services.call("bridge_identity") != "typed-test-bridge":
            return "typed bridge identity differs"
        return ""

    def activate_feature(self, _epoch: int) -> str:
        return ""

    def reset_feature(self, _epoch: int) -> None:
        pass

    def deactivate_feature(self) -> None:
        pass

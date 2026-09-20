"""Feature fixture that binds the real bridge, then fails during activation."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class real_bridge_fail_activate(Node):
    def validate_feature(self, feature_id: str) -> str:
        if not feature_id:
            return "feature ID is empty"
        return ""

    def bind_host(self, services_path: str) -> str:
        # Binding mirrors the success fixture against the real native bridge
        # so only activation fails: the disabled feature must be isolated
        # while its required sibling keeps a valid bridge binding.
        services = self.get_node(services_path)
        if services is None:
            return "native bridge service node is missing"
        if not services.has_method("feature_families_json"):
            return "native bridge family table is missing"
        return ""

    def activate_feature(self, _epoch: int) -> str:
        return "synthetic activation failure"

    def reset_feature(self, _epoch: int) -> None:
        pass

    def deactivate_feature(self) -> None:
        pass

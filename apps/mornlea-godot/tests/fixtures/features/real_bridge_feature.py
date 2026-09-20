"""Successful feature fixture bound to the real native bridge surface."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class real_bridge_feature(Node):
    def validate_feature(self, feature_id: str) -> str:
        if not feature_id:
            return "feature ID is empty"
        return ""

    def bind_host(self, services_path: str) -> str:
        # Unlike the typed-test-bridge stubs, this fixture binds the real
        # native bridge: the family table identity is the only surface the
        # host negotiates against, so binding fails closed when the node at
        # the scene path is not the native bridge.
        services = self.get_node(services_path)
        if services is None:
            return "native bridge service node is missing"
        if not services.has_method("feature_families_json"):
            return "native bridge family table is missing"
        return ""

    def activate_feature(self, _epoch: int) -> str:
        # Activation deliberately performs no bridge calls: the catalog
        # integration checks must observe a bridge that no feature touched.
        return ""

    def reset_feature(self, _epoch: int) -> None:
        pass

    def deactivate_feature(self) -> None:
        pass

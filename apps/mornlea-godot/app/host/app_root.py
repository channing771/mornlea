"""Hand the prepared Godot scene tree to the Python feature host."""

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class app_root(Node):
    """Provide the temporary typed host identity until the native bridge is injected."""

    def _ready(self) -> None:
        # Catalog contents, not this stable root scene, decide which features exist.
        host = self.get_node("FeatureHost")
        result = host.call("activate_catalog", "res://config/feature_catalog.tres", self, 1)
        print(f"[mornlea-host] catalog={result}")

    def _exit_tree(self) -> None:
        # Explicit teardown keeps feature order deterministic during project reloads.
        host = self.get_node_or_null("FeatureHost")
        if host is not None:
            host.call("deactivate_features")

    def host_protocol_version(self) -> str:
        return "1.0"

    def feature_family_version(self, _family_id: str) -> str:
        return ""

"""Hand the prepared Godot scene tree to the Python feature host."""

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class app_root(Node):
    """Initialize the native facade before assembling the explicit feature catalog."""

    def _ready(self) -> None:
        # Catalog contents, not this stable root scene, decide which features exist.
        bridge = self.get_node("BridgeHost")
        bridge_error = bridge.call("initialize_bridge")
        if isinstance(bridge_error, str) and bridge_error:
            print(f"[mornlea-host] bridge=failed error={bridge_error}")
            return
        host = self.get_node("FeatureHost")
        result = host.call("activate_catalog", "res://config/feature_catalog.tres", bridge, 1)
        print(f"[mornlea-host] catalog={result}")

    def _exit_tree(self) -> None:
        # Explicit teardown keeps feature order deterministic during project reloads.
        host = self.get_node_or_null("FeatureHost")
        if host is not None:
            host.call("deactivate_features")
        bridge = self.get_node_or_null("BridgeHost")
        if bridge is not None:
            bridge.call("release_bridge")

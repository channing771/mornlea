"""Hand the prepared Godot scene tree to the Python feature host."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# The app root acquires the native bridge node through `get_node` plus this
# identity cast (the established acquisition pattern); the pinned runtime
# cannot marshal project-class objects across script-module method calls, so
# only the scene path crosses into the feature host.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

_EXPECTED_CLIENT_CORE_ABI = "1.0"
_EXPECTED_GODOT_API = "4.7"
_EXPECTED_GODOT_RUST_VERSION = "0.5.5"


@gdclass
class app_root(Node):
    """Bind the native bridge node before assembling the feature catalog."""

    def _ready(self) -> None:
        # Catalog contents, not this stable root scene, decide which features exist.
        # The scene-held `MornleaClientBridge` node is the typed service bound by
        # the host; this root only checks identity and never touches its session
        # methods, which belong to features alone.
        bridge = self.get_node("ClientBridge")
        bridge_error = _bridge_identity_error(bridge)
        if bridge_error:
            print(f"[mornlea-host] bridge=failed error={bridge_error}")
            return
        host = self.get_node("FeatureHost")
        result = host.call(
            "activate_catalog",
            "res://config/feature_catalog.tres",
            "../ClientBridge",
            1,
        )
        print(f"[mornlea-host] catalog={result}")
        print("[mornlea-lifecycle] python-init=host")

    def _exit_tree(self) -> None:
        # Explicit teardown keeps feature order deterministic during project reloads.
        host = self.get_node_or_null("FeatureHost")
        if host is not None:
            host.call("deactivate_features")
            print("[mornlea-lifecycle] python-deinit=features")
        bridge = self.get_node_or_null("ClientBridge")
        if bridge is not None:
            # The native node releases any open session through its scene-teardown
            # drop path before extension deinitialization, so the Python side only
            # records the lifecycle marker here.
            print("[mornlea-lifecycle] python-deinit=bridge")


def _bridge_identity_error(bridge: Node) -> str:
    # Identity travels through typed Godot-visible values only; there is no
    # Python facade between the app root and the native bridge any more.
    if not _call_bool(bridge, "supports_godot_api", 4, 7):
        return "MornleaClientBridge rejected Godot API 4.7"
    abi = _pair_version(bridge, "client_core_abi_major", "client_core_abi_minor")
    if abi != _EXPECTED_CLIENT_CORE_ABI:
        return "MornleaClientBridge client-core ABI is not 1.0"
    if _pair_version(bridge, "godot_api_major", "godot_api_minor") != _EXPECTED_GODOT_API:
        return "MornleaClientBridge Godot API is not 4.7"
    if _call_text(bridge, "godot_rust_version") != _EXPECTED_GODOT_RUST_VERSION:
        return "MornleaClientBridge godot-rust version is not 0.5.5"
    if _call_text(bridge, "lifecycle_stage") != "main-loop":
        return "MornleaClientBridge is outside the Godot main-loop stage"
    return ""


def _pair_version(bridge: Node, major_method: str, minor_method: str) -> str:
    major = _call_int(bridge, major_method)
    minor = _call_int(bridge, minor_method)
    if major < 0 or minor < 0:
        return ""
    return f"{major}.{minor}"


def _call_int(target: Node, method: str) -> int:
    value = target.call(method)
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


def _call_text(target: Node, method: str) -> str:
    value = target.call(method)
    return value if isinstance(value, str) else ""


def _call_bool(target: Node, method: str, *arguments: object) -> bool:
    value = target.call(method, *arguments)
    return value if isinstance(value, bool) else False

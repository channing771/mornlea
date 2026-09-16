"""Exercise the Python facade without introducing a client data plane."""

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


@gdclass
class bridge_host_check(Node):
    """Verify native identity through the same facade injected into features."""

    def _ready(self) -> None:
        bridge = self.get_node("BridgeHost")
        failures: list[str] = []
        result = bridge.call("initialize_bridge")
        _expect(result == "", f"bridge initialization failed: {result}", failures)
        _expect(bridge.call("host_protocol_version") == "1.0", "host protocol mismatch", failures)
        _expect(
            bridge.call("client_core_abi_version") == "1.0", "client-core ABI mismatch", failures
        )
        _expect(bridge.call("godot_api_version") == "4.7", "Godot API mismatch", failures)
        _expect(bridge.call("godot_rust_version") == "0.5.5", "godot-rust mismatch", failures)
        _expect(bridge.call("supports_godot_api", 4, 7) is True, "Godot API rejected", failures)
        _expect(
            bridge.call("supports_godot_api", 4, 6) is False, "wrong Godot API accepted", failures
        )
        _expect(bridge.call("lifecycle_stage") == "main-loop", "lifecycle stage mismatch", failures)
        _expect(
            bridge.call("bridge_identity") == "client-core=1.0;godot=4.7;godot-rust=0.5.5",
            "composed bridge identity mismatch",
            failures,
        )
        _expect(
            bridge.call("feature_family_version", "connection") == "",
            "identity-only bridge exposed a data-plane family",
            failures,
        )
        bridge.call("release_bridge")
        if failures:
            for failure in failures:
                print(f"Python bridge host check failed: {failure}")
            self.get_tree().quit(1)
            return
        print("Python bridge host check passed.")
        self.get_tree().quit(0)


def _expect(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)

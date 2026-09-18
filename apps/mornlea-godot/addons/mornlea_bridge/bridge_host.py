"""Expose the project-owned Rust bridge to Python through Godot reflection only.

Retirement ruling: the feature host binds the scene-held `MornleaClientBridge`
node directly, so this static facade is no longer injected into production
assembly. It remains as the subject of the coexistence qualification probe,
which proves Py4Godot and the native bridge load together through reflection.
"""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.ClassDB import ClassDB
from py4godot.classes.Node import Node

_NATIVE_CLASS = "MornleaClientBridge"
_HOST_PROTOCOL_VERSION = "1.0"
_EXPECTED_CLIENT_CORE_ABI = "1.1"
_EXPECTED_GODOT_API = "4.7"
_EXPECTED_GODOT_RUST_VERSION = "0.5.5"


@gdclass
class bridge_host(Node):
    """Own one identity-checked native facade without exposing its C symbols."""

    _initialized: bool
    _last_error: str

    def _ready(self) -> None:
        self._initialized = False
        self._last_error = ""

    def initialize_bridge(self) -> str:
        """Bind the registered identity surface and reject mixed distribution units."""
        self.release_bridge()
        database = ClassDB.instance()
        if not database.class_exists(_NATIVE_CLASS):
            return self._fail("MornleaClientBridge is not registered")
        self._initialized = True
        mismatch = self._identity_mismatch()
        if mismatch:
            self.release_bridge()
            return self._fail(mismatch)
        self._last_error = ""
        return ""

    def release_bridge(self) -> None:
        # The identity-only pilot owns no native instance; reset capability state only.
        self._initialized = False

    def _exit_tree(self) -> None:
        self.release_bridge()

    def host_protocol_version(self) -> str:
        return _HOST_PROTOCOL_VERSION

    def feature_family_version(self, _family_id: str) -> str:
        # Data-plane families stay unavailable until the Go client-core ABI lands.
        return ""

    def client_core_abi_version(self) -> str:
        return self._pair_version("client_core_abi_major", "client_core_abi_minor")

    def godot_api_version(self) -> str:
        return self._pair_version("godot_api_major", "godot_api_minor")

    def godot_rust_version(self) -> str:
        if not self._initialized:
            return ""
        return _call_text(ClassDB.instance(), "godot_rust_version")

    def lifecycle_stage(self) -> str:
        if not self._initialized:
            return "inactive"
        return _call_text(ClassDB.instance(), "lifecycle_stage")

    def supports_godot_api(self, major: int, minor: int) -> bool:
        if not self._initialized:
            return False
        return _call_bool(ClassDB.instance(), "supports_godot_api", major, minor)

    def bridge_identity(self) -> str:
        if not self._initialized:
            return ""
        return (
            f"client-core={self.client_core_abi_version()};"
            f"godot={self.godot_api_version()};"
            f"godot-rust={self.godot_rust_version()}"
        )

    def bridge_error(self) -> str:
        return self._last_error

    def _identity_mismatch(self) -> str:
        if not self.supports_godot_api(4, 7):
            return "MornleaClientBridge rejected Godot API 4.7"
        if self.client_core_abi_version() != _EXPECTED_CLIENT_CORE_ABI:
            return "MornleaClientBridge client-core ABI is not 1.1"
        if self.godot_api_version() != _EXPECTED_GODOT_API:
            return "MornleaClientBridge Godot API is not 4.7"
        if self.godot_rust_version() != _EXPECTED_GODOT_RUST_VERSION:
            return "MornleaClientBridge godot-rust version is not 0.5.5"
        if self.lifecycle_stage() != "main-loop":
            return "MornleaClientBridge is outside the Godot main-loop stage"
        return ""

    def _pair_version(self, major_method: str, minor_method: str) -> str:
        if not self._initialized:
            return ""
        database = ClassDB.instance()
        major = _call_int(database, major_method)
        minor = _call_int(database, minor_method)
        if major is None or minor is None:
            return ""
        return f"{major}.{minor}"

    def _fail(self, message: str) -> str:
        self._last_error = message
        return message


def _call_int(database: ClassDB, method: str) -> int | None:
    value = database.class_call_static(_NATIVE_CLASS, method)
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return None


def _call_text(database: ClassDB, method: str) -> str:
    value = database.class_call_static(_NATIVE_CLASS, method)
    return value if isinstance(value, str) else ""


def _call_bool(database: ClassDB, method: str, *arguments: object) -> bool:
    value = database.class_call_static(_NATIVE_CLASS, method, *arguments)
    return value if isinstance(value, bool) else False

"""Feature fixture that opens a real bridge session and never closes it."""

from __future__ import annotations

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# Features acquire the native bridge through `get_node` plus the identity
# cast, because the pinned runtime cannot marshal project-class objects
# across script-module method boundaries.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

STATUS_OK = 0
# The dial target is deliberately refused in the qualification sandbox, so the
# connection stays honestly offline after the asynchronous begin succeeds.
DENIED_ADDRESS = "127.0.0.1:9"


@gdclass
class stub_open_session(Node):
    """Hold one live producer session so scene teardown must release it."""

    _services: Node | None
    _create_status: int

    def _ready(self) -> None:
        self._services = None
        self._create_status = -1

    def validate_feature(self, feature_id: str) -> str:
        return "" if feature_id == "open_session" else "unexpected open-session feature ID"

    def bind_host(self, services_path: str) -> str:
        # The typed service identity is the native family table; the fixture
        # reaches no C symbol and retains no native buffer.
        services = self.get_node(services_path)
        if services is None:
            return "bridge service node is missing"
        if not services.has_method("feature_families_json"):
            return "typed bridge family table is missing"
        self._services = services
        return ""

    def activate_feature(self, _epoch: int) -> str:
        services = self._services
        if services is None:
            return "activation before bind"
        self._create_status = _call_int(services, "session_create")
        if self._create_status != STATUS_OK:
            return f"session create failed with status {self._create_status}"
        connect_status = _call_int(services, "session_connect", DENIED_ADDRESS)
        if connect_status != STATUS_OK:
            return f"connect begin failed with status {connect_status}"
        return ""

    def reset_feature(self, _epoch: int) -> None:
        pass

    def deactivate_feature(self) -> None:
        # Deliberately no close: the bridge node's teardown drop owns the open
        # session release that this fixture exists to exercise.
        self._services = None

    def create_status(self) -> int:
        return self._create_status


def _call_int(target: Node, method: str, *arguments: object) -> int:
    value = target.call(method, *arguments)
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1

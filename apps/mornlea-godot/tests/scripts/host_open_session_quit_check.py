"""Prove the host tears a deliberately open bridge session cleanly on quit."""

from __future__ import annotations

import json
from typing import Any, cast

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# The generated Py4Godot cast table only knows engine classes; the identity
# cast lets this driver hold the native bridge node and reach its typed
# methods through Godot's dynamic `call`, without any binding patch.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

STATUS_OK = 0


@gdclass
class host_open_session_quit_check(Node):
    """Activate one real session through the host, then quit without close."""

    def _ready(self) -> None:
        failures: list[str] = []
        host = self.get_node("FeatureHost")
        bridge = self.get_node("ClientBridge")
        raw = cast(
            str,
            host.call(
                "activate_catalog",
                "res://tests/fixtures/catalogs/open_session.tres",
                "../ClientBridge",
                3,
            ),
        )
        result = cast(dict[str, Any], json.loads(raw))
        _expect(
            result["ok"],
            f"open-session catalog activation failed: {raw}",
            failures,
        )
        _expect(
            cast(int, host.call("active_count")) == 1,
            "the open-session feature did not stay active",
            failures,
        )
        # The session must be observably live before the quit: the status
        # record is readable only while the producer session exists.
        status = cast(Any, bridge.call("pull_status"))
        _expect(
            status["status"] == STATUS_OK,
            "the open session did not expose a status record",
            failures,
        )
        if failures:
            for failure in failures:
                print(f"Python host open-session quit check failed: {failure}")
            self.get_tree().quit(1)
            return
        # Deliberately no `session_close` and no feature deactivation: the
        # quit must leave session teardown to the bridge node's drop path.
        print("Python host open-session quit check passed with the session left open.")
        self.get_tree().quit(0)


def _expect(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)

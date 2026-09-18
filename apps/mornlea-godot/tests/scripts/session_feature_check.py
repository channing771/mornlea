"""Drive the real production session feature through its offline phases."""

from __future__ import annotations

import gc
import json
from typing import Any, TypedDict, cast

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# The generated Py4Godot cast table only knows engine classes; the identity
# cast lets this driver hold the native bridge node while only the feature
# itself drives the session methods, mirroring the production host pattern.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

# The dial target is deliberately refused in the offline qualification
# sandbox, so the terminal path must classify it as the stable dial error.
DENIED_ADDRESS = "127.0.0.1:9"
DIAL_ERROR_TEXT = "Could not reach the server."
NOT_READY_TEXT = "Not ready"
CONNECTING_TEXT = "Connecting"
DISCONNECTED_TEXT = "Disconnected"
FRAME_BUDGET = 300


class PlanResult(TypedDict):
    ok: bool
    errors: list[str]
    disabled: list[str]
    order: list[str]


def _word(value: object) -> int:
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class session_feature_check(Node):
    """Frame-driven driver: activate the catalog, then watch the display."""

    _failures: list[str]
    _feature: Node | None
    _host: Node | None
    _frames: int
    _saw_connecting: bool

    def _ready(self) -> None:
        self._failures = []
        self._feature = None
        self._host = self.get_node("FeatureHost")
        self._frames = 0
        self._saw_connecting = False
        result = self._activate()
        if not result["ok"]:
            self._failures.append(f"production catalog activation failed: {result}")
            self._finish(1)
            return
        # The production catalog activates the complete minimum remote loop;
        # this probe still drives only the session feature's offline dial.
        expected_order = [
            "session",
            "actors",
            "platform.desktop.input",
            "player_view",
            "ui",
            "world",
        ]
        if result["order"] != expected_order:
            self._failures.append(f"activation order differs: {result['order']}")
        if result["disabled"]:
            self._failures.append(f"a minimum-loop feature was disabled by the plan: {result}")
        host = self._host
        if host is None:
            self._failures.append("the feature host node is missing")
            self._finish(1)
            return
        if cast(int, host.call("active_count")) != len(expected_order):
            self._failures.append("the minimum-loop features did not stay active")
        feature = host.get_node_or_null("SessionFeature")
        if feature is None or not feature.has_method("request_connect"):
            self._failures.append("the active session feature lacks request_connect")
            self._finish(1)
            return
        self._feature = feature
        self._expect_display(NOT_READY_TEXT, "", "the initial not-ready display")
        rejected = self._call_text(feature, "request_connect", "")
        if not rejected:
            self._failures.append("an empty address was accepted for connecting")
        begin = self._call_text(feature, "request_connect", DENIED_ADDRESS)
        if begin:
            self._failures.append(f"connection begin failed: {begin}")
        repeated = self._call_text(feature, "request_connect", DENIED_ADDRESS)
        if not repeated:
            self._failures.append("a second connection request was accepted")
        if self._failures:
            self._finish(1)

    def _process(self, _delta: float) -> None:
        feature = self._feature
        if feature is None or self._failures:
            return
        self._frames += 1
        feature.call("refresh")
        phase = self._call_text(feature, "phase_text")
        if phase == CONNECTING_TEXT:
            self._saw_connecting = True
            if self._call_text(feature, "error_text"):
                self._failures.append("an error line appeared while connecting")
        elif phase == DISCONNECTED_TEXT:
            self._check_terminal_display(feature)
            return
        else:
            self._failures.append(f"unexpected live phase display: {phase}")
        if self._frames > FRAME_BUDGET:
            self._failures.append("the terminal display never arrived")
        if self._failures or self._frames > FRAME_BUDGET:
            self._finish(1)

    def _check_terminal_display(self, feature: Node) -> None:
        self._expect_display(DISCONNECTED_TEXT, DIAL_ERROR_TEXT, "the terminal display")
        # The error mapping must ride the typed path: the bridge's typed
        # status view reports the dial terminal cause that the display maps.
        bridge = self.get_node_or_null("ClientBridge")
        if bridge is None:
            self._failures.append("the typed bridge node is missing from the check scene")
        else:
            typed = cast(Any, bridge.call("session_status_typed"))
            words = (
                _word(typed["status"]),
                _word(typed["phase"]),
                _word(typed["terminal_cause"]),
            )
            if words != (0, 5, 1):
                self._failures.append(f"typed status view differs: {words}")
        # The terminal display is latched: repeated refreshes must not flap.
        feature.call("refresh")
        feature.call("refresh")
        self._expect_display(DISCONNECTED_TEXT, DIAL_ERROR_TEXT, "the latched display")
        # A host-driven reset refreshes observation without fabricating state.
        host = self._host
        if host is not None:
            host.call("reset_features", 43)
        feature.call("refresh")
        self._expect_display(DISCONNECTED_TEXT, DIAL_ERROR_TEXT, "the reset display")
        # The feature-owned clean close clears the error line and stays stable.
        feature.call("close_connection")
        self._expect_display(DISCONNECTED_TEXT, "", "the clean-close display")
        feature.call("refresh")
        self._expect_display(DISCONNECTED_TEXT, "", "the post-close display")
        if host is not None:
            host.call("deactivate_features")
            if cast(int, host.call("active_count")) != 0:
                self._failures.append("deactivation left the session feature active")
        if self._failures:
            self._finish(1)
            return
        print("Python session feature check passed.")
        self._finish(0)

    def _expect_display(self, phase: str, error: str, label: str) -> None:
        feature = self._feature
        if feature is None:
            self._failures.append(f"{label} was checked without a feature instance")
            return
        actual_phase = self._call_text(feature, "phase_text")
        actual_error = self._call_text(feature, "error_text")
        if actual_phase != phase or actual_error != error:
            self._failures.append(
                f"{label} differs: phase={actual_phase!r} error={actual_error!r}"
                f" (want phase={phase!r} error={error!r})"
            )
        phase_node = feature.get_node_or_null("PhaseLabel")
        error_node = feature.get_node_or_null("ErrorLabel")
        if phase_node is None or error_node is None:
            self._failures.append(f"{label} lacks display labels")
            return
        shown_phase = self._call_text(phase_node, "get_text")
        shown_error = self._call_text(error_node, "get_text")
        if shown_phase != phase or shown_error != error:
            self._failures.append(
                f"{label} label text differs: phase={shown_phase!r} error={shown_error!r}"
            )

    def _activate(self) -> PlanResult:
        host = self._host
        if host is None:
            return {"ok": False, "errors": ["host missing"], "disabled": [], "order": []}
        raw = cast(
            str,
            host.call(
                "activate_catalog",
                "res://config/feature_catalog.tres",
                "../ClientBridge",
                41,
            ),
        )
        return cast(PlanResult, json.loads(raw))

    def _finish(self, exit_code: int) -> None:
        if self._failures:
            for failure in self._failures:
                print(f"Python session feature check failed: {failure}")
            print(
                "Python session feature check state:"
                f" frames={self._frames} saw_connecting={self._saw_connecting}"
            )
        self.call_deferred("_quit", exit_code)

    def _quit(self, exit_code: int) -> None:
        gc.collect()
        self.get_tree().quit(exit_code)

    def _call_text(self, target: Node, method: str, *arguments: object) -> str:
        value = target.call(method, *arguments)
        return value if isinstance(value, str) else ""

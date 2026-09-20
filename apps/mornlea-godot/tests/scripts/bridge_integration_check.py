"""Exercise catalog failure and isolation against the real native bridge node."""

from __future__ import annotations

import gc
import json
from typing import Protocol, TypedDict, cast, runtime_checkable

from py4godot.classes import gdclass
from py4godot.classes.Node import Node
from py4godot.utils.smart_cast import (  # type: ignore[import-not-found]
    register_cast_function,
)

# The generated Py4Godot cast table only knows engine classes; the identity
# cast lets this driver hold the native bridge node while only typed Godot
# values cross script-module boundaries, mirroring the production host pattern.
register_cast_function("MornleaClientBridge", lambda bridge: bridge)

# The no-session status word from the frozen client-core contract
# (`MORNLEA_CLIENT_STATUS_INVALID_STATE`): with no session ever created, the
# typed status view must keep reporting this word with every other field
# zeroed, so the word staying 6 across failed and partial activations is the
# observable proof that the authoritative mirror was never touched.
STATUS_INVALID_STATE = 6

# Session epochs are arbitrary distinct words per activation; a fixture that
# secretly depends on one specific epoch value fails at least one scenario.
MISSING_FAMILY_EPOCH = 21
OPTIONAL_FAILURE_EPOCH = 22


class PlanResult(TypedDict):
    ok: bool
    errors: list[str]
    disabled: list[str]
    order: list[str]


@runtime_checkable
class _DictionaryView(Protocol):
    """Minimal typed view of a bridge typed-status result dictionary."""

    def __getitem__(self, key: str) -> object: ...


def _word_of(value: object) -> int:
    # Booleans are rejected so a marshaling drift cannot masquerade as a
    # status word the driver branches on.
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    return -1


@gdclass
class bridge_integration_check(Node):
    def _ready(self) -> None:
        failures: list[str] = []
        host = self.get_node("FeatureHost")
        bridge = self.get_node("ClientBridge")
        # The untouched baseline is captured once: every scenario must leave
        # the bridge's family table and typed status view byte-identical.
        families_before = _call_text(bridge, "feature_families_json")
        status_before = _bridge_status_words(bridge)
        _expect(
            status_before == (STATUS_INVALID_STATE, 0, 0, 0, 0),
            f"the bridge did not start in the no-session state: {status_before}",
            failures,
        )
        table = json.loads(families_before)
        family_ids = (
            {str(entry.get("family")) for entry in table} if isinstance(table, list) else set()
        )
        _expect(
            "1" in family_ids,
            f"the real bridge lacks family 1, so the negotiation baseline is wrong: {family_ids}",
            failures,
        )
        _check_missing_required_family(host, bridge, families_before, status_before, failures)
        _check_optional_failure_isolated(host, bridge, families_before, status_before, failures)
        host.call("deactivate_features")
        _expect(
            cast(int, host.call("active_count")) == 0,
            "deactivation left an active feature behind",
            failures,
        )
        _check_bridge_untouched(
            bridge,
            families_before,
            status_before,
            "after deactivation",
            failures,
        )
        if failures:
            for failure in failures:
                print(f"Python bridge integration check failed: {failure}")
            self.call_deferred("_finish", 1)
            return
        print("Python bridge integration checks passed.")
        self.call_deferred("_finish", 0)

    def _finish(self, exit_code: int) -> None:
        gc.collect()
        self.get_tree().quit(exit_code)


def _check_missing_required_family(
    host: Node,
    bridge: Node,
    families_before: str,
    status_before: tuple[int, ...],
    failures: list[str],
) -> None:
    # Family 99 is outside the real bridge's negotiated table, so the plan
    # must fail before any scene instantiation, with no partially enabled
    # features and no fallback implementation.
    result = _activate(host, "bridge_required_family", MISSING_FAMILY_EPOCH)
    _expect(not result["ok"], "a missing required bridge family was accepted", failures)
    _expect(
        _contains(result["errors"], "bridge family"),
        "the failed plan reported no bridge family diagnostic",
        failures,
    )
    trace = _trace(host)
    instantiated = [entry for entry in trace if entry.startswith("instantiate:")]
    _expect(
        not instantiated,
        f"a rejected plan still instantiated features: {instantiated}",
        failures,
    )
    _expect(
        cast(int, host.call("active_count")) == 0,
        "a rejected plan left active features behind",
        failures,
    )
    _check_bridge_untouched(
        bridge,
        families_before,
        status_before,
        "after the rejected required-family plan",
        failures,
    )


def _check_optional_failure_isolated(
    host: Node,
    bridge: Node,
    families_before: str,
    status_before: tuple[int, ...],
    failures: list[str],
) -> None:
    # The optional feature fails during activation while its required sibling
    # stays live: the failure must disable exactly the failing feature and
    # leave both the sibling and the bridge untouched.
    result = _activate(host, "bridge_optional_failure", OPTIONAL_FAILURE_EPOCH)
    _expect(
        result["ok"],
        f"an optional activation failure stopped the whole catalog: {result['errors']}",
        failures,
    )
    _expect(
        result["disabled"] == ["bridge_optional_failure"],
        f"the disabled set differs from the failing feature: {result['disabled']}",
        failures,
    )
    trace = _trace(host)
    _expect(
        "instantiate:bridge_required_success" in trace,
        f"the required sibling was never instantiated: {trace}",
        failures,
    )
    _expect(
        cast(int, host.call("active_count")) == 1,
        "the required sibling did not stay active",
        failures,
    )
    _check_bridge_untouched(
        bridge,
        families_before,
        status_before,
        "after the isolated optional failure",
        failures,
    )


def _check_bridge_untouched(
    bridge: Node,
    families_before: str,
    status_before: tuple[int, ...],
    label: str,
    failures: list[str],
) -> None:
    # The family table and the typed status view are the bridge's observable
    # identity and mirror state; both comparisons are byte-identical, so any
    # drift from a failed or partial activation fails the check.
    _expect(
        _call_text(bridge, "feature_families_json") == families_before,
        f"{label}: the bridge family table changed",
        failures,
    )
    status_after = _bridge_status_words(bridge)
    _expect(
        status_after == status_before,
        f"{label}: the typed status view changed: {status_after}",
        failures,
    )


def _bridge_status_words(bridge: Node) -> tuple[int, ...]:
    typed = bridge.call("session_status_typed")
    if not isinstance(typed, _DictionaryView):
        return (-1, -1, -1, -1, -1)
    return tuple(
        _word_of(typed[key])
        for key in ("status", "phase", "terminal_cause", "steps_completed", "messages_processed")
    )


def _activate(host: Node, fixture: str, epoch: int) -> PlanResult:
    raw = cast(
        str,
        host.call(
            "activate_catalog",
            f"res://tests/fixtures/catalogs/{fixture}.tres",
            "../ClientBridge",
            epoch,
        ),
    )
    return cast(PlanResult, json.loads(raw))


def _trace(host: Node) -> list[str]:
    return cast(list[str], json.loads(cast(str, host.call("trace_json"))))


def _call_text(target: Node, method: str, *arguments: object) -> str:
    value = target.call(method, *arguments)
    return value if isinstance(value, str) else ""


def _contains(values: list[str], fragment: str) -> bool:
    return any(fragment in value for value in values)


def _expect(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)

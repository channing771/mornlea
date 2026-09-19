"""Exercise the serialized Python feature contract inside the real Godot host."""

from __future__ import annotations

import gc
import json
import sys
from typing import TypedDict, cast

from py4godot.classes import gdclass
from py4godot.classes.Node import Node


class PlanResult(TypedDict):
    ok: bool
    errors: list[str]
    disabled: list[str]
    order: list[str]


@gdclass
class feature_contract_check(Node):
    def _ready(self) -> None:
        failures: list[str] = []
        host = self.get_node("FeatureHost")
        bridge_path = "../StubBridge"
        if "--extensibility-probe" in sys.argv:
            _check_extensibility(host, bridge_path, failures)
        else:
            _check_planning(host, bridge_path, failures)
            _check_lifecycle(host, bridge_path, failures)
        if failures:
            for failure in failures:
                print(f"Python feature contract check failed: {failure}")
            self.call_deferred("_finish", 1)
            return
        print("Python feature contract checks passed.")
        self.call_deferred("_finish", 0)

    def _finish(self, exit_code: int) -> None:
        gc.collect()
        self.get_tree().quit(exit_code)


def _check_planning(host: Node, bridge_path: str, failures: list[str]) -> None:
    # These fixtures verify catalog semantics without relying on implementation imports.
    valid = _plan(host, bridge_path, "valid")
    _expect(valid["ok"], "serialized valid catalog was rejected", failures)
    _expect(
        valid["order"] == ["alpha", "beta"],
        f"deterministic order differs: {valid['order']}",
        failures,
    )
    duplicate = _plan(host, bridge_path, "duplicate")
    _expect(
        not duplicate["ok"] and _contains(duplicate["errors"], "duplicate"),
        "duplicate feature ID was accepted",
        failures,
    )
    cycle = _plan(host, bridge_path, "cycle")
    _expect(
        not cycle["ok"] and _contains(cycle["errors"], "cycle"),
        "dependency cycle was accepted",
        failures,
    )
    required_version = _plan(host, bridge_path, "required_version")
    _expect(
        not required_version["ok"] and _contains(required_version["errors"], "incompatible"),
        "required incompatible feature did not fail",
        failures,
    )
    optional_version = _plan(host, bridge_path, "optional_version")
    _expect(
        optional_version["ok"] and optional_version["disabled"] == ["optional_new"],
        "optional incompatible feature was not isolated",
        failures,
    )
    required_bridge = _plan(host, bridge_path, "required_bridge")
    _expect(
        not required_bridge["ok"] and _contains(required_bridge["errors"], "bridge family"),
        "missing required bridge family did not fail",
        failures,
    )
    unknown_budget = _plan(host, bridge_path, "unknown_budget")
    _expect(
        not unknown_budget["ok"] and _contains(unknown_budget["errors"], "unknown budget"),
        "unknown budget class was accepted",
        failures,
    )
    required_dependent = _plan(host, bridge_path, "required_dependent")
    _expect(
        not required_dependent["ok"]
        and _contains(required_dependent["errors"], "required_dependent")
        and required_dependent["order"] == [],
        "required dependent was partially enabled after a missing family",
        failures,
    )
    optional_bridge = _plan(host, bridge_path, "optional_bridge")
    _expect(
        optional_bridge["ok"] and optional_bridge["disabled"] == ["optional_bridge"],
        "missing optional bridge family was not isolated",
        failures,
    )


def _check_lifecycle(host: Node, bridge_path: str, failures: list[str]) -> None:
    # Required failure must roll back, while optional failure must remain isolated.
    optional_failure = _activate(host, bridge_path, "optional_failure", 4)
    _expect(
        optional_failure["ok"] and optional_failure["disabled"] == ["optional_failure"],
        "optional activation failure stopped the host",
        failures,
    )
    required_failure = _activate(host, bridge_path, "required_failure", 4)
    _expect(
        not required_failure["ok"] and _contains(required_failure["errors"], "activate"),
        "required activation failure did not stop the host",
        failures,
    )
    _expect(
        cast(int, host.call("active_count")) == 0,
        "required activation failure retained active features",
        failures,
    )
    complete = _activate(host, bridge_path, "valid", 7)
    _expect(complete["ok"], "valid lifecycle activation failed", failures)
    host.call("reset_features", 8)
    host.call("deactivate_features")
    trace = cast(list[str], json.loads(cast(str, host.call("trace_json"))))
    expected = [
        "instantiate:alpha",
        "validate:alpha",
        "bind:alpha",
        "activate:alpha:7",
        "instantiate:beta",
        "validate:beta",
        "bind:beta",
        "activate:beta:7",
        "reset:alpha:8",
        "reset:beta:8",
        "deactivate:beta",
        "deactivate:alpha",
    ]
    _expect(trace == expected, f"lifecycle order differs: {trace}", failures)


def _check_extensibility(host: Node, bridge_path: str, failures: list[str]) -> None:
    # The synthetic resource is absent from Bootstrap and app_root.tscn by design.
    result = _activate(host, bridge_path, "extensibility", 11)
    _expect(result["ok"], "synthetic optional feature was rejected", failures)
    _expect(
        cast(int, host.call("active_count")) == 1,
        "synthetic optional feature did not activate independently",
        failures,
    )
    trace = cast(list[str], json.loads(cast(str, host.call("trace_json"))))
    _expect(
        "activate:synthetic_optional:11" in trace,
        "synthetic optional feature did not traverse the lifecycle",
        failures,
    )
    host.call("deactivate_features")


def _plan(host: Node, bridge_path: str, fixture: str) -> PlanResult:
    raw = cast(
        str,
        host.call(
            "plan_catalog",
            f"res://tests/fixtures/catalogs/{fixture}.tres",
            bridge_path,
        ),
    )
    return cast(PlanResult, json.loads(raw))


def _activate(host: Node, bridge_path: str, fixture: str, epoch: int) -> PlanResult:
    raw = cast(
        str,
        host.call(
            "activate_catalog",
            f"res://tests/fixtures/catalogs/{fixture}.tres",
            bridge_path,
            epoch,
        ),
    )
    return cast(PlanResult, json.loads(raw))


def _contains(values: list[str], fragment: str) -> bool:
    return any(fragment in value for value in values)


def _expect(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)

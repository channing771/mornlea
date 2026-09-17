"""Typed bridge fixture used to validate host protocol and family negotiation."""

from __future__ import annotations

import json

from py4godot.classes import gdclass
from py4godot.classes.Node import Node

# The stub mirrors the native bridge's typed identity surface: one JSON family
# table with numeric family identifiers, so negotiation probes exercise the
# same contract the real bridge reports.
_STUB_FAMILIES = [{"family": 1, "record_bytes": 0, "record_limit": 1, "version": 1}]
_STUB_FAMILIES_JSON = json.dumps(_STUB_FAMILIES, separators=(",", ":"))


@gdclass
class stub_bridge(Node):
    def feature_families_json(self) -> str:
        return _STUB_FAMILIES_JSON

    def bridge_identity(self) -> str:
        return "typed-test-bridge"

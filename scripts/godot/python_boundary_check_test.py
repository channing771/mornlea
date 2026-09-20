from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path
from typing import Protocol, cast


class Finding(Protocol):
    rule: str


class Checker(Protocol):
    def scan_project(self, root: Path) -> list[Finding]: ...


def load_checker() -> Checker:
    module_path = Path(__file__).with_name("python_boundary_check.py")
    spec = importlib.util.spec_from_file_location(
        "mornlea_python_boundary_check", module_path
    )
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load boundary checker from {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return cast(Checker, module)


class PythonBoundaryCheckTests(unittest.TestCase):
    checker: Checker

    def setUp(self) -> None:
        self.checker = load_checker()

    def scan(self, source: str, relative: str = "feature.py") -> set[str]:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(source, encoding="utf-8")
            return {finding.rule for finding in self.checker.scan_project(root)}

    def test_accepts_typed_py4godot_import_and_localized_string(self) -> None:
        rules = self.scan(
            'from py4godot.classes.Node import Node\nTEXT = "中文文本"\n# English comment.\n'
        )
        self.assertEqual(rules, set())

    def test_rejects_forbidden_runtime_boundaries(self) -> None:
        cases = {
            "from packages.agent import companion\n": "forbidden-agent-import",
            "import ctypes\n": "forbidden-native-abi",
            "import socket\n": "forbidden-network",
            "from py4godot.classes.HTTPClient import HTTPClient\n": "forbidden-network",
            "import pip\n": "forbidden-runtime-installer",
            "import subprocess\n": "forbidden-process-execution",
            "import importlib\nimportlib.import_module(name)\n": "unrestricted-dynamic-import",
            "__import__(name)\n": "unrestricted-dynamic-import",
            "# 中文注释\n": "non-english-comment",
            "import sqlite3\n": "forbidden-persistence",
            "import numpy\n": "forbidden-numerical-fallback",
            "from packages.shared.network import codec\n": "forbidden-protocol-ownership",
            "value = Predictor()\n": "forbidden-prediction",
        }
        for source, expected in cases.items():
            with self.subTest(expected=expected):
                self.assertIn(expected, self.scan(source))

    def test_rejects_protocol_wire_markers_in_production_scripts(self) -> None:
        cases = {
            'MAGIC = "MCI1"\n': "forbidden-protocol-magic",
            "MAGIC = 0x3149434D\n": "forbidden-protocol-magic",
            'import struct\nRECORD = struct.unpack("<I", data)\n': (
                "forbidden-wire-record-codec"
            ),
            'from struct import pack\nRECORD = pack("<I", 1)\n': (
                "forbidden-wire-record-codec"
            ),
            'import struct\nCODEC = struct.Struct("<I")\n': (
                "forbidden-wire-record-codec"
            ),
        }
        for source, expected in cases.items():
            with self.subTest(expected=expected):
                self.assertIn(expected, self.scan(source))

    def test_protocol_rules_exempt_test_and_typing_trees(self) -> None:
        source = 'import struct\nRECORD = struct.unpack("<I", data)\nMAGIC = "MCI1"\n'
        for relative in ("tests/scripts/probe.py", "typing/stubs.py"):
            with self.subTest(relative=relative):
                self.assertNotIn(
                    "forbidden-wire-record-codec", self.scan(source, relative)
                )
                self.assertNotIn(
                    "forbidden-protocol-magic", self.scan(source, relative)
                )

    def test_rejects_gameplay_bridge_calls_and_feature_references_in_host_scripts(
        self,
    ) -> None:
        cases = {
            'METHOD = "session_step"\n': "host-gameplay-bridge-call",
            'PATH = "res://features/session/session_feature.tscn"\n': (
                "host-feature-reference"
            ),
            'FEATURE = "world"\n': "host-feature-reference",
        }
        for source, expected in cases.items():
            with self.subTest(expected=expected):
                self.assertIn(expected, self.scan(source, "app/host/feature_host.py"))

    def test_host_rules_do_not_fire_outside_host_scripts(self) -> None:
        source = 'METHOD = "session_step"\n'
        self.assertNotIn(
            "host-gameplay-bridge-call",
            self.scan(source, "features/session/session_feature.py"),
        )

    def test_rejects_cross_feature_private_paths_and_unbounded_callbacks(self) -> None:
        self.assertIn(
            "cross-feature-private-path",
            self.scan(
                'SCENE = "res://features/session/feature_root.tscn"\n',
                "features/world/world_feature.py",
            ),
        )
        self.assertNotIn(
            "cross-feature-private-path",
            self.scan(
                'SCENE = "res://features/world/terrain_near/terrain_near_opaque.tres"\n',
                "features/world/world_feature.py",
            ),
        )
        self.assertIn(
            "unbounded-callback",
            self.scan(
                "class Feature:\n    def _process(self, delta: float) -> None:\n        while True:\n            pass\n",
                "features/ui/hud.py",
            ),
        )

    def test_rejects_invalid_python(self) -> None:
        self.assertIn("syntax-error", self.scan("def broken(:\n"))


if __name__ == "__main__":
    unittest.main()

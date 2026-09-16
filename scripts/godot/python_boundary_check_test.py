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

    def scan(self, source: str) -> set[str]:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "feature.py").write_text(source, encoding="utf-8")
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
        }
        for source, expected in cases.items():
            with self.subTest(expected=expected):
                self.assertIn(expected, self.scan(source))

    def test_rejects_invalid_python(self) -> None:
        self.assertIn("syntax-error", self.scan("def broken(:\n"))


if __name__ == "__main__":
    unittest.main()

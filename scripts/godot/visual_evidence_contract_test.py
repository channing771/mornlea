from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from typing import Any, Protocol, cast


class Contract(Protocol):
    def validate_identity(self, payload: dict[str, Any]) -> None: ...

    def validate_identity_file(self, path: Path, run_dir: Path) -> dict[str, Any]: ...

    def validate_run_dir(self, run_dir: Path) -> None: ...


def load_contract() -> Contract:
    module_path = Path(__file__).with_name("visual_evidence_contract.py")
    spec = importlib.util.spec_from_file_location("mornlea_visual_evidence_contract", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load visual evidence contract from {module_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return cast(Contract, module)


def complete_identity() -> dict[str, object]:
    return {
        "schema_version": 1,
        "run_id": "run-1",
        "scenario": "world/terrain-settled",
        "git_commit": "84f3e0e75dff6987e47cf2aa50f50d888e85eafb",
        "worktree_state": "dirty",
        "godot_version": "4.7.2-stable",
        "py4godot_version": "4.7-alpha21",
        "py4godot_source_revision": "d8e17428deeb0428587349b663f6da26cd71ef3a",
        "cpython_version": "3.14.4",
        "platform": {"os": "darwin", "arch": "arm64", "version": "macOS 26.6.2"},
        "gpu": {"name": "Apple M2", "api": "Metal 4"},
        "catalog": "res://config/feature_catalog.tres",
        "feature_families": [{"family": 1, "version": 1, "record_limit": 1, "record_bytes": 16}],
        "images": ["world/terrain-settled.png"],
    }


class VisualEvidenceContractTests(unittest.TestCase):
    contract: Contract

    def setUp(self) -> None:
        self.contract = load_contract()

    def test_rejects_tracked_golden_run_dir(self) -> None:
        with self.assertRaises(ValueError):
            self.contract.validate_run_dir(Path("/tmp/testdata/visual-golden/godot"))

    def test_rejects_incomplete_identity(self) -> None:
        payload = complete_identity()
        del payload["cpython_version"]
        with self.assertRaises(ValueError):
            self.contract.validate_identity(payload)

    def test_accepts_complete_run(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            run_dir = Path(directory) / "build" / "visual" / "godot-pilot" / "run-1"
            image = run_dir / "world" / "terrain-settled.png"
            image.parent.mkdir(parents=True)
            image.write_bytes(b"png")
            identity_path = run_dir / "identity.json"
            identity_path.write_text(json.dumps(complete_identity()), encoding="utf-8")
            payload = self.contract.validate_identity_file(identity_path, run_dir)
            self.assertEqual(payload["scenario"], "world/terrain-settled")


if __name__ == "__main__":
    unittest.main()

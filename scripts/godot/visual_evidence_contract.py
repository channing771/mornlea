"""Validate Godot pilot visual-evidence identity and output routing."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

REQUIRED_IDENTITY_KEYS = (
    "schema_version",
    "run_id",
    "scenario",
    "git_commit",
    "worktree_state",
    "godot_version",
    "py4godot_version",
    "py4godot_source_revision",
    "cpython_version",
    "platform",
    "gpu",
    "catalog",
    "feature_families",
    "images",
)

TRACKED_GOLDEN = "testdata/visual-golden"
PILOT_EVIDENCE = "build/visual/godot-pilot"


def validate_run_dir(run_dir: Path) -> None:
    resolved = run_dir.resolve()
    text = resolved.as_posix()
    if TRACKED_GOLDEN in text:
        raise ValueError("pilot evidence must stay outside tracked visual baselines")
    if "visual-golden/godot" in text:
        raise ValueError("renderer-specific tracked visual classes are forbidden")
    if PILOT_EVIDENCE not in text:
        raise ValueError("Godot pilot evidence must use the dedicated build output")


def validate_identity(payload: dict[str, Any]) -> None:
    missing = [key for key in REQUIRED_IDENTITY_KEYS if key not in payload]
    if missing:
        raise ValueError(f"visual identity is missing {missing}")
    if payload.get("schema_version") != 1:
        raise ValueError("visual identity schema_version must be 1")
    git_commit = payload.get("git_commit")
    if not isinstance(git_commit, str) or len(git_commit) != 40:
        raise ValueError("visual identity git_commit must be a 40-character commit")
    if payload.get("worktree_state") not in {"clean", "dirty"}:
        raise ValueError("visual identity worktree_state must be clean or dirty")
    for key in (
        "run_id",
        "scenario",
        "godot_version",
        "py4godot_version",
        "py4godot_source_revision",
        "cpython_version",
        "catalog",
    ):
        value = payload.get(key)
        if not isinstance(value, str) or not value.strip():
            raise ValueError(f"visual identity {key} is incomplete")
    platform = payload.get("platform")
    if not isinstance(platform, dict) or not platform.get("os") or not platform.get("arch"):
        raise ValueError("visual identity platform is incomplete")
    gpu = payload.get("gpu")
    if not isinstance(gpu, dict) or not gpu.get("name") or not gpu.get("api"):
        raise ValueError("visual identity GPU is incomplete")
    families = payload.get("feature_families")
    if not isinstance(families, list) or not families:
        raise ValueError("visual identity feature_families must not be empty")
    images = payload.get("images")
    if not isinstance(images, list) or not images:
        raise ValueError("visual identity images must not be empty")
    for image in images:
        if not isinstance(image, str) or image.startswith("/") or ".." in Path(image).parts:
            raise ValueError(f"visual identity image path {image!r} must stay run-relative")
        if image.startswith(TRACKED_GOLDEN) or "visual-golden/godot" in image:
            raise ValueError("visual identity must not name a tracked golden")


def validate_identity_file(path: Path, run_dir: Path) -> dict[str, Any]:
    validate_run_dir(run_dir)
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError("visual identity must be a JSON object")
    validate_identity(payload)
    for image in payload["images"]:
        image_path = run_dir / image
        if not image_path.is_file():
            raise ValueError(f"captured image is missing: {image}")
        validate_run_dir(image_path)
    return payload


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--identity", required=True, type=Path)
    parser.add_argument("--run-dir", required=True, type=Path)
    args = parser.parse_args(argv)
    try:
        validate_identity_file(args.identity, args.run_dir)
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"Godot visual evidence contract failed: {error}", file=sys.stderr)
        return 1
    print("Godot visual evidence identity is complete.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

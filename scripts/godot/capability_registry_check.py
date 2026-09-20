#!/usr/bin/env python3
"""Validate the disabled coarse-capability registry without loading Godot."""

from __future__ import annotations

import re
import sys
from pathlib import Path


EXPECTED = {
    "menus": ("3@1.0", "6@1.0", "7@1.0"),
    "containers": ("3@1.0", "5@1.0", "6@1.0", "7@1.0"),
    "chat": ("2@1.0", "3@1.0", "6@1.0", "7@1.0"),
    "audio": ("6@1.0", "8@1.0"),
    "lod": ("5@1.0",),
    "viewmodel": ("3@1.0", "6@1.0"),
    "companions": ("6@1.0",),
    "hostile_mobs": ("6@1.0",),
    "passive_cows": ("6@1.0",),
    "projectiles": ("6@1.0",),
    "drops": ("6@1.0",),
    "name_tags": ("6@1.0",),
    "particles": ("6@1.0", "8@1.0"),
    "full_weather": ("8@1.0",),
}
EXPECTED_BUDGETS = {
    "audio": "light",
    "name_tags": "light",
    "lod": "heavy",
    "full_weather": "heavy",
}
ALLOWED_FAMILIES = frozenset(range(1, 9))
METADATA = re.compile(r"^metadata/([a-z_]+)\s*=\s*(.*)$", re.MULTILINE)
QUOTED = re.compile(r'"([^"\\]*(?:\\.[^"\\]*)*)"')


def metadata(text: str) -> dict[str, str]:
    return {name: value.strip() for name, value in METADATA.findall(text)}


def strings(value: str) -> tuple[str, ...]:
    if not value.startswith("PackedStringArray(") or not value.endswith(")"):
        raise ValueError(f"expected PackedStringArray, got {value}")
    return tuple(match.group(1) for match in QUOTED.finditer(value))


def scalar(fields: dict[str, str], name: str, expected: str) -> None:
    if fields.get(name) != expected:
        raise ValueError(f"{name} = {fields.get(name)!r}, want {expected!r}")


def validate_manifest(root: Path, name: str, path: Path, expected_families: tuple[str, ...]) -> None:
    if not path.is_file():
        raise ValueError(f"{name}: missing manifest {path}")
    feature_dir = path.parent
    files = sorted(item.name for item in feature_dir.iterdir() if item.is_file())
    if files != ["feature.tres"]:
        raise ValueError(f"{name}: capability directory contains implementation files {files}")
    fields = metadata(path.read_text(encoding="utf-8"))
    scalar(fields, "feature_id", f'"{name}"')
    scalar(fields, "host_protocol_major", "1")
    scalar(fields, "host_protocol_minor", "0")
    scalar(fields, "contract_version", '"1.0"')
    scalar(fields, "enabled", "false")
    scalar(fields, "implementation_status", '"reserved"')
    scalar(fields, "entry_scene_path", '""')
    scalar(fields, "required", "false")
    scalar(fields, "budget_class", f'"{EXPECTED_BUDGETS.get(name, "standard")}"')
    scalar(fields, "reset_policy", '"reset"')
    if strings(fields.get("dependencies", "")):
        raise ValueError(f"{name}: reserved capability has feature dependencies")
    actual_families = strings(fields.get("required_bridge_families", ""))
    if actual_families != expected_families:
        raise ValueError(f"{name}: family dependencies {actual_families}, want {expected_families}")
    for family in actual_families:
        match = re.fullmatch(r"([0-9]+)@1\.0", family)
        if match is None or int(match.group(1)) not in ALLOWED_FAMILIES:
            raise ValueError(f"{name}: family dependency is not a known numeric v1 family: {family}")


def validate(root: Path) -> None:
    registry_path = root / "catalog/capability_registry.tres"
    fields = metadata(registry_path.read_text(encoding="utf-8"))
    scalar(fields, "registry_version", "1")
    manifest_paths = strings(fields.get("manifest_paths", ""))
    expected_paths = tuple(f"res://features/{name}/feature.tres" for name in EXPECTED)
    if manifest_paths != expected_paths:
        raise ValueError(f"registry manifest paths {manifest_paths}, want {expected_paths}")
    if len(set(manifest_paths)) != len(manifest_paths):
        raise ValueError("registry contains duplicate manifest paths")
    for name, resource_path in zip(EXPECTED, manifest_paths):
        if not resource_path.startswith("res://features/"):
            raise ValueError(f"{name}: manifest is outside the feature root")
        relative = resource_path.removeprefix("res://")
        validate_manifest(root, name, root / relative, EXPECTED[name])


def main() -> int:
    root = (
        Path(__file__).resolve().parents[2] / "apps/mornlea-godot"
        if len(sys.argv) == 1
        else Path(sys.argv[1]).resolve()
    )
    try:
        validate(root)
    except (OSError, ValueError) as error:
        print(f"Godot capability registry check failed: {error}", file=sys.stderr)
        return 1
    print(f"Godot capability registry check passed ({len(EXPECTED)} disabled coarse capabilities).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

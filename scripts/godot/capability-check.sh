#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
exec python3 "$repo_root/scripts/godot/capability_registry_check.py" "$repo_root/apps/mornlea-godot"

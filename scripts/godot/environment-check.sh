#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
python3 "${script_dir}/environment_check.py"
printf 'Godot environment checks passed.\n'

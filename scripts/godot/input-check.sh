#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
python3 "${script_dir}/input_check.py"
printf 'Godot desktop input checks passed.\n'

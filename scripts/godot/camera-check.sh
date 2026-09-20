#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
python3 "${script_dir}/camera_check.py"
printf 'Godot camera mapping checks passed.\n'

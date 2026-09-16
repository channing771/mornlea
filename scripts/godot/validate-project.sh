#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"
project_root="${MORNLEA_GODOT_PROJECT_ROOT:-${repository_root}/apps/mornlea-godot}"

usage() {
  printf 'usage: %s --script-ownership\n' "${0##*/}" >&2
}

validate_script_ownership() {
  local failures=0
  local source_path relative_path
  local allowed_bootstrap="app/bootstrap/bootstrap.gd"
  local allowed_setup="app/bootstrap/setup_required.gd"

  # Tests may use GDScript to probe the project before Python is available. The
  # production allowlist remains limited to the native bootstrap diagnostics.
  while IFS= read -r -d '' source_path; do
    relative_path="${source_path#${project_root}/}"
    case "${relative_path}" in
      tests/*|addons/py4godot/*|.godot/*|.venv/*)
        continue
        ;;
      "${allowed_bootstrap}"|"${allowed_setup}")
        continue
        ;;
    esac
    printf 'production GDScript is outside the bootstrap allowlist: %s\n' "${relative_path}" >&2
    failures=1
  done < <(find "${project_root}" -type f -name '*.gd' -print0)

  return "${failures}"
}

if (($# != 1)) || [[ "$1" != "--script-ownership" ]]; then
  usage
  exit 2
fi

[[ -d "${project_root}" ]] || {
  printf 'Godot project root is missing: %s\n' "${project_root}" >&2
  exit 1
}

validate_script_ownership
printf 'Godot script ownership validation passed.\n'

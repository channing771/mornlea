#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"
client_root="${repository_root}/packages/client"
generated_root="${MORNLEA_GODOT_GENERATED_ROOT:-${repository_root}/apps/mornlea-godot/assets/generated}"
mode="generate"

if (($# > 1)); then
  printf 'usage: scripts/godot/sync-assets.sh [--check]\n' >&2
  exit 2
fi
if (($# == 1)); then
  [[ "$1" == "--check" ]] || {
    printf 'unsupported sync-assets argument: %s\n' "$1" >&2
    exit 2
  }
  mode="check"
fi

command -v go >/dev/null 2>&1 || {
  printf 'Go is required to generate the authoritative client atlas.\n' >&2
  exit 1
}

arguments=(
  --repository-root "${repository_root}"
  --output "${generated_root}"
)
if [[ "${mode}" == "check" ]]; then
  arguments+=(--check)
fi

# The generator calls the production registry directly so the Godot copy cannot
# drift into a second procedural texture implementation. Dependency resolution
# is offline to keep asset synchronization reproducible and side-effect free.
(
  cd -- "${client_root}"
  GOTOOLCHAIN=local GOPROXY=off go run -mod=readonly ./cmd/mornlea-godot-assets "${arguments[@]}"
)

if [[ "${mode}" == "check" ]]; then
  printf 'Godot generated assets match their authoritative inputs.\n'
else
  printf 'Godot generated assets synchronized at %s\n' "${generated_root}"
fi

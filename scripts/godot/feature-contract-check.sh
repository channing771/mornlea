#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"

usage() {
  printf 'usage: scripts/godot/feature-contract-check.sh [--extensibility-probe|--bridge-integration]\n' >&2
}

mode="contract"
if (($# == 1)); then
  case "$1" in
    --extensibility-probe) mode="probe" ;;
    --bridge-integration) mode="bridge-integration" ;;
    *)
      usage
      exit 2
      ;;
  esac
elif (($# != 0)); then
  usage
  exit 2
fi

if [[ "${mode}" == "bridge-integration" ]]; then
  # The integration scene holds the real native bridge node, so the client
  # core and the GDExtension must be materialized before the headless run.
  "${script_dir}/build-core.sh" --verify >/dev/null
  "${script_dir}/build-extension.sh" --verify >/dev/null
  if ! integration_output="$("${script_dir}/godot.sh" \
    --headless \
    --path "${repository_root}/apps/mornlea-godot" \
    --quit-after 300 \
    res://tests/scenes/bridge_integration_check.tscn 2>&1)"; then
    printf '%s\n' "${integration_output}" >&2
    exit 1
  fi
  printf '%s\n' "${integration_output}"
  if [[ "${integration_output}" == *"SCRIPT ERROR:"* || "${integration_output}" == *"ERROR:"* ]]; then
    printf 'Godot reported a bridge-integration script error.\n' >&2
    exit 1
  fi
  if [[ "${integration_output}" != *"Python bridge integration checks passed."* ]]; then
    printf 'Python bridge-integration success marker is missing.\n' >&2
    exit 1
  fi
  exit 0
fi

run_contract() {
  if [[ "${mode}" == "probe" ]]; then
    "${script_dir}/godot.sh" \
      --headless \
      --path "${repository_root}/apps/mornlea-godot" \
      --quit-after 300 \
      res://tests/scenes/feature_contract_check.tscn \
      -- --extensibility-probe
    return
  fi
  "${script_dir}/godot.sh" \
    --headless \
    --path "${repository_root}/apps/mornlea-godot" \
    --quit-after 300 \
    res://tests/scenes/feature_contract_check.tscn
}

if ! import_output="$("${script_dir}/godot.sh" \
  --headless \
  --path "${repository_root}/apps/mornlea-godot" \
  --editor \
  --quit 2>&1)"; then
  printf '%s\n' "${import_output}" >&2
  exit 1
fi
if [[ "${import_output}" == *"SCRIPT ERROR:"* || "${import_output}" == *"ERROR:"* ]]; then
  printf '%s\nGodot reported a feature-contract import error.\n' "${import_output}" >&2
  exit 1
fi

if ! output="$(run_contract 2>&1)"; then
  printf '%s\n' "${output}" >&2
  exit 1
fi

printf '%s\n' "${output}"
if [[ "${output}" == *"SCRIPT ERROR:"* || "${output}" == *"ERROR:"* ]]; then
  printf 'Godot reported a feature-contract script error.\n' >&2
  exit 1
fi
if [[ "${output}" != *"Python feature contract checks passed."* ]]; then
  printf 'Python feature-contract success marker is missing.\n' >&2
  exit 1
fi

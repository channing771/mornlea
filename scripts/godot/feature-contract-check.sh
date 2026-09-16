#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"

probe="false"
if (($# == 1)) && [[ "$1" == "--extensibility-probe" ]]; then
  probe="true"
elif (($# != 0)); then
  printf 'usage: scripts/godot/feature-contract-check.sh [--extensibility-probe]\n' >&2
  exit 2
fi

run_contract() {
  if [[ "${probe}" == "true" ]]; then
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

#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"
project_root="${repository_root}/apps/mornlea-godot"
iterations=100

usage() {
  printf 'usage: %s [--iterations POSITIVE_INTEGER]\n' "${0##*/}" >&2
}

while (($# > 0)); do
  case "$1" in
    --iterations)
      (($# >= 2)) || { usage; exit 2; }
      iterations="$2"
      shift 2
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

[[ "${iterations}" =~ ^[1-9][0-9]*$ ]] || {
  printf 'Godot smoke iterations must be a positive integer: %s\n' "${iterations}" >&2
  exit 2
}

require_marker_order() {
  local output="$1"
  shift
  local previous=0
  local marker line
  for marker in "$@"; do
    line="$(awk -v needle="${marker}" 'index($0, needle) { print NR; exit }' <<<"${output}")"
    if [[ -z "${line}" || "${line}" -le "${previous}" ]]; then
      printf 'Godot lifecycle marker is missing or out of order: %s\n' "${marker}" >&2
      return 1
    fi
    previous="${line}"
  done
}

# Materialize both exact native units once; each iteration exercises only project startup
# and teardown so compilation time cannot hide lifecycle leaks or ordering failures.
"${script_dir}/build-python-runtime.sh" --verify --offline >/dev/null
"${script_dir}/build-extension.sh" --verify >/dev/null

for ((iteration = 1; iteration <= iterations; iteration++)); do
  if ! output="$("${script_dir}/godot.sh" \
    --headless \
    --path "${project_root}" \
    --quit-after 30 2>&1)"; then
    printf '%s\n' "${output}" >&2
    printf 'Godot lifecycle smoke failed on iteration %s.\n' "${iteration}" >&2
    exit 1
  fi
  if [[ "${output}" == *"ERROR:"* || "${output}" == *"SCRIPT ERROR:"* || "${output}" == *"leaked at exit"* ]]; then
    printf '%s\n' "${output}" >&2
    printf 'Godot lifecycle smoke reported an error or leak on iteration %s.\n' "${iteration}" >&2
    exit 1
  fi
  require_marker_order "${output}" \
    "[mornlea-lifecycle] rust-init=scene" \
    "[mornlea-lifecycle] rust-init=main-loop" \
    "[mornlea-lifecycle] python-init=host" \
    "[mornlea-lifecycle] python-deinit=features" \
    "[mornlea-lifecycle] python-deinit=bridge" \
    "[mornlea-lifecycle] rust-deinit=main-loop" \
    "[mornlea-lifecycle] rust-deinit=scene" || {
      printf '%s\n' "${output}" >&2
      printf 'Godot lifecycle smoke order failed on iteration %s.\n' "${iteration}" >&2
      exit 1
    }
done

printf 'Godot lifecycle smoke passed: iterations=%s.\n' "${iterations}"

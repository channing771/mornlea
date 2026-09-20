#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"

# shellcheck source=version.env
source "${script_dir}/version.env"

mode="fetch"
target="darwin-universal"
cache_root="${MORNLEA_GODOT_CACHE_DIR:-/tmp/mornlea-godot-cache}"
partial_path=""

fail() {
  printf 'godot fetch: %s\n' "$*" >&2
  exit 1
}

usage() {
  printf '%s\n' \
    'usage: scripts/godot/fetch.sh [--verify-only] [--target darwin-universal] [--cache-dir ABSOLUTE_PATH]'
}

cleanup() {
  if [[ -n "${partial_path}" && -f "${partial_path}" ]]; then
    rm -f -- "${partial_path}"
  fi
}
trap cleanup EXIT

while (($# > 0)); do
  case "$1" in
    --verify-only)
      mode="verify"
      shift
      ;;
    --target)
      (($# >= 2)) || fail "--target requires a value"
      target="$2"
      shift 2
      ;;
    --cache-dir)
      (($# >= 2)) || fail "--cache-dir requires a value"
      cache_root="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      fail "unsupported argument: $1"
      ;;
  esac
done

[[ "${target}" == "darwin-universal" ]] || fail "unsupported Godot desktop target: ${target}"
[[ "${cache_root}" == /* ]] || fail "cache directory must be an absolute path outside the repository"

mkdir -p -- "${cache_root}"
cache_root="$(cd -- "${cache_root}" && pwd -P)"
case "${cache_root}/" in
  "${repository_root}/"*) fail "cache directory must stay outside the repository: ${cache_root}" ;;
esac

artifact_dir="${cache_root}/${GODOT_VERSION}/${target}"
mkdir -p -- "${artifact_dir}"
engine_path="${artifact_dir}/Godot_v${GODOT_VERSION}_macos.universal.zip"
templates_path="${artifact_dir}/Godot_v${GODOT_VERSION}_export_templates.tpz"

sha256_file() {
  local file_path="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -- "${file_path}" | awk '{print $1}'
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "${file_path}" | awk '{print $1}'
    return
  fi
  fail "neither shasum nor sha256sum is available"
}

verify_file() {
  local label="$1"
  local file_path="$2"
  local expected="$3"
  local actual
  actual="$(sha256_file "${file_path}")"
  [[ "${actual}" == "${expected}" ]] || fail "${label} checksum mismatch: got ${actual}, want ${expected}"
}

verify_remote() {
  local label="$1"
  local url="$2"
  curl --fail --silent --show-error --location --head --retry 2 --retry-all-errors "${url}" >/dev/null || \
    fail "${label} official release URL is not reachable"
}

download_artifact() {
  local label="$1"
  local url="$2"
  local expected="$3"
  local destination="$4"
  if [[ -f "${destination}" ]]; then
    verify_file "${label}" "${destination}" "${expected}"
    printf '%s already verified: %s\n' "${label}" "${destination}"
    return
  fi
  partial_path="${destination}.partial.$$"
  curl --fail --show-error --location --retry 3 --retry-all-errors --output "${partial_path}" "${url}"
  verify_file "${label}" "${partial_path}" "${expected}"
  mv -- "${partial_path}" "${destination}"
  partial_path=""
  printf '%s downloaded and verified: %s\n' "${label}" "${destination}"
}

if [[ "${mode}" == "verify" ]]; then
  if [[ -f "${engine_path}" ]]; then
    verify_file "Godot Standard" "${engine_path}" "${GODOT_MACOS_UNIVERSAL_SHA256}"
  else
    verify_remote "Godot Standard" "${GODOT_MACOS_UNIVERSAL_URL}"
  fi
  if [[ -f "${templates_path}" ]]; then
    verify_file "Godot export templates" "${templates_path}" "${GODOT_EXPORT_TEMPLATES_SHA256}"
  else
    verify_remote "Godot export templates" "${GODOT_EXPORT_TEMPLATES_URL}"
  fi
  printf 'Godot %s metadata verified for %s; cache=%s\n' "${GODOT_VERSION}" "${target}" "${artifact_dir}"
  exit 0
fi

download_artifact "Godot Standard" "${GODOT_MACOS_UNIVERSAL_URL}" "${GODOT_MACOS_UNIVERSAL_SHA256}" "${engine_path}"
download_artifact "Godot export templates" "${GODOT_EXPORT_TEMPLATES_URL}" "${GODOT_EXPORT_TEMPLATES_SHA256}" "${templates_path}"

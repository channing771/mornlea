#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"
project_root="${repository_root}/apps/mornlea-godot"

# shellcheck source=python-version.env
source "${script_dir}/python-version.env"
# shellcheck source=py4godot/build-inputs.env
source "${script_dir}/py4godot/build-inputs.env"
# shellcheck source=version.env
source "${script_dir}/version.env"

mode=""
offline="false"
target="${PY4GODOT_TARGET}"
cache_root="${MORNLEA_PY4GODOT_CACHE_DIR:-/tmp/mornlea-py4godot-cache}"
godot_cache_root="${MORNLEA_GODOT_CACHE_DIR:-/tmp/mornlea-godot-cache}"
qualification_root=""

fail() {
  printf 'Py4Godot qualification: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  local exit_status="$?"
  if [[ -n "${qualification_root}" && -d "${qualification_root}" ]]; then
    case "${qualification_root}" in
      "${cache_root}/"*) rm -rf -- "${qualification_root}" ;;
      *) printf 'Py4Godot qualification: refusing to remove unexpected path: %s\n' "${qualification_root}" >&2 ;;
    esac
  fi
  return "${exit_status}"
}
trap cleanup EXIT

usage() {
  printf '%s\n' \
    'usage: scripts/godot/python-runtime-check.sh (--qualify|--coexistence) [--offline] [--target darwin-arm64] [--cache-dir ABSOLUTE_PATH]'
}

while (($# > 0)); do
  case "$1" in
    --qualify)
      mode="qualify"
      shift
      ;;
    --coexistence)
      mode="coexistence"
      shift
      ;;
    --offline)
      offline="true"
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

[[ "${mode}" == "qualify" || "${mode}" == "coexistence" ]] || \
  fail "--qualify or --coexistence is required"
[[ "${target}" == "darwin-arm64" ]] || fail "unsupported Py4Godot desktop target: ${target}"
[[ "$(uname -s)" == "Darwin" && "$(uname -m)" == "arm64" ]] || \
  fail "unsupported Py4Godot desktop target: $(uname -s)-$(uname -m)"
[[ "${cache_root}" == /* ]] || fail "cache directory must be an absolute path outside the repository"
[[ "${godot_cache_root}" == /* ]] || fail "Godot cache directory must be an absolute path outside the repository"

builder_args=(--verify --target "${target}" --cache-dir "${cache_root}")
if [[ "${offline}" == "true" ]]; then
  builder_args+=(--offline)
fi
"${script_dir}/build-python-runtime.sh" "${builder_args[@]}"
cache_root="$(cd -- "${cache_root}" && pwd -P)"

sha256_file() {
  local file_path="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -- "${file_path}" | awk '{print $1}'
    return
  fi
  sha256sum -- "${file_path}" | awk '{print $1}'
}

destination="${project_root}/addons/py4godot"
python_root="${destination}/cpython-${PY4GODOT_CPYTHON_VERSION}-darwin64/python"
bundled_python="${python_root}/bin/python3.14"
python_identity="$(${bundled_python} -I -s -E -c 'import platform, sys; print(platform.machine(), platform.python_version(), sys.flags.isolated)')"
[[ "${python_identity}" == "arm64 ${PY4GODOT_CPYTHON_VERSION} 1" ]] || \
  fail "unexpected embedded Python identity: ${python_identity}"
if "${bundled_python}" -I -s -E -m pip --version >/dev/null 2>&1; then
  fail "runtime package installer must be unavailable"
fi

godot_binary="${MORNLEA_GODOT_BIN:-/Applications/Godot.app/Contents/MacOS/Godot}"
[[ -x "${godot_binary}" ]] || fail "Godot executable is unavailable: ${godot_binary}"
actual_godot_version="$("${godot_binary}" --version)"
[[ "${actual_godot_version}" == 4.7.2.stable* ]] || \
  fail "Godot version mismatch: got ${actual_godot_version}, want 4.7.2-stable"

isolation_bin="${cache_root}/isolated-path"
forbidden_pythonpath="${cache_root}/forbidden-pythonpath"
invocation_marker="${cache_root}/external-runtime-invoked"
mkdir -p -- "${isolation_bin}" "${forbidden_pythonpath}"
touch "${forbidden_pythonpath}/mornlea_external_runtime_poison.py"
rm -f -- "${invocation_marker}"

run_isolated() {
  sandbox-exec -p '(version 1) (allow default) (deny network*)' \
    /usr/bin/env \
      PATH="${isolation_bin}" \
      PYTHONNOUSERSITE=1 \
      PYTHONPATH="${forbidden_pythonpath}" \
      PYTHONUSERBASE="${cache_root}/forbidden-user-base" \
      PIP_CONFIG_FILE=/dev/null \
      MORNLEA_EXTERNAL_RUNTIME_MARKER="${invocation_marker}" \
      "${godot_binary}" "$@"
}

run_isolated_executable() {
  sandbox-exec -p '(version 1) (allow default) (deny network*)' \
    /usr/bin/env \
      PATH="${isolation_bin}" \
      PYTHONNOUSERSITE=1 \
      PYTHONPATH="${forbidden_pythonpath}" \
      PYTHONUSERBASE="${cache_root}/forbidden-user-base" \
      PIP_CONFIG_FILE=/dev/null \
      MORNLEA_EXTERNAL_RUNTIME_MARKER="${invocation_marker}" \
      "$@"
}

if [[ "${mode}" == "coexistence" ]]; then
  "${script_dir}/build-extension.sh" --verify >/dev/null
  coexistence_output="$(run_isolated --headless --path "${project_root}" --quit-after 120 \
    res://tests/scenes/bridge_host_check.tscn 2>&1)" || {
    printf '%s\n' "${coexistence_output}" >&2
    fail "Py4Godot and mornlea_godot bridge-host coexistence probe failed"
  }
  [[ "${coexistence_output}" == *"Python bridge host check passed."* ]] || \
    fail "Python bridge-host coexistence marker is missing"
  [[ "${coexistence_output}" != *"ERROR:"* && "${coexistence_output}" != *"SCRIPT ERROR:"* ]] || {
    printf '%s\n' "${coexistence_output}" >&2
    fail "bridge-host coexistence probe reported an extension error"
  }
  [[ ! -e "${invocation_marker}" ]] || fail "an external Python or package installer was invoked"
  printf 'Py4Godot and mornlea_godot coexist through the isolated Python bridge host.\n'
  exit 0
fi

editor_output="$(run_isolated --headless --path "${project_root}" --editor --quit 2>&1)" || {
  printf '%s\n' "${editor_output}" >&2
  fail "editor failed to load the Python extension"
}
[[ "${editor_output}" != *"ERROR:"* && "${editor_output}" != *"SCRIPT ERROR:"* ]] || {
  printf '%s\n' "${editor_output}" >&2
  fail "editor reported an extension error"
}

iterations="${MORNLEA_PY4GODOT_QUALIFY_ITERATIONS:-100}"
[[ "${iterations}" =~ ^[1-9][0-9]*$ ]] || fail "qualification iteration count must be a positive integer"
for ((iteration = 1; iteration <= iterations; iteration++)); do
  headless_output="$(run_isolated --headless --path "${project_root}" --quit-after 120 \
    res://tests/scenes/python_runtime_probe.tscn 2>&1)" || {
    printf '%s\n' "${headless_output}" >&2
    fail "headless Python probe failed on iteration ${iteration}"
  }
  [[ "${headless_output}" == *"Py4Godot runtime check passed."* ]] || \
    fail "headless Python success marker is missing on iteration ${iteration}"
  [[ "${headless_output}" != *"ERROR:"* && "${headless_output}" != *"SCRIPT ERROR:"* ]] || {
    printf '%s\n' "${headless_output}" >&2
    fail "headless Python probe reported an error on iteration ${iteration}"
  }
done

"${script_dir}/build-extension.sh" --verify >/dev/null
coexistence_output="$(run_isolated --headless --path "${project_root}" \
  --quit-after 120 res://tests/scenes/bridge_host_check.tscn 2>&1)" || {
  printf '%s\n' "${coexistence_output}" >&2
  fail "Py4Godot and mornlea_godot coexistence probe failed"
}
[[ "${coexistence_output}" == *"Python bridge host check passed."* ]] || \
  fail "mornlea_godot coexistence marker is missing"
[[ "${coexistence_output}" != *"ERROR:"* && "${coexistence_output}" != *"SCRIPT ERROR:"* ]] || {
  printf '%s\n' "${coexistence_output}" >&2
  fail "coexistence probe reported an extension error"
}

template_archive="${godot_cache_root}/${GODOT_VERSION}/darwin-universal/Godot_v${GODOT_VERSION}_export_templates.tpz"
if [[ ! -f "${template_archive}" ]]; then
  [[ "${offline}" == "false" ]] || fail "offline export-template cache miss: ${template_archive}"
  "${script_dir}/fetch.sh" --target darwin-universal --cache-dir "${godot_cache_root}"
fi
[[ "$(sha256_file "${template_archive}")" == "${GODOT_EXPORT_TEMPLATES_SHA256}" ]] || \
  fail "Godot export-template checksum mismatch"

CARGO_NET_OFFLINE=true "${script_dir}/build-extension.sh" --profile release --verify >/dev/null
qualification_root="$(mktemp -d "${cache_root}/qualification.XXXXXX")"
qualification_project="${qualification_root}/project"
qualification_home="${qualification_root}/home"
qualification_output="${qualification_root}/output"
template_dir="${qualification_home}/Library/Application Support/Godot/export_templates/4.7.2.stable"
mkdir -p -- "${template_dir}" "${qualification_output}"
unzip -oqj "${template_archive}" templates/macos.zip -d "${template_dir}"
ditto --norsrc --noextattr "${project_root}" "${qualification_project}"
rm -rf -- "${qualification_project}/.godot" "${qualification_project}/tests/scripts/__pycache__"
cp -- "${script_dir}/fixtures/python-runtime-export-presets.cfg" \
  "${qualification_project}/export_presets.cfg"
sed -i '' \
  's#run/main_scene="res://app/bootstrap/bootstrap.tscn"#run/main_scene="res://tests/scenes/python_runtime_probe.tscn"#' \
  "${qualification_project}/project.godot"

exported_app="${qualification_output}/MornleaPythonQualification.app"
export_output="$(sandbox-exec -p '(version 1) (allow default) (deny network*)' \
  /usr/bin/env HOME="${qualification_home}" "${godot_binary}" --headless \
  --path "${qualification_project}" --export-release "macOS Python Qualification" "${exported_app}" 2>&1)" || {
  printf '%s\n' "${export_output}" >&2
  fail "macOS qualification export failed"
}
[[ "${export_output}" != *"ERROR:"* && "${export_output}" != *"SCRIPT ERROR:"* ]] || {
  printf '%s\n' "${export_output}" >&2
  fail "macOS qualification export reported an error"
}

exported_resources="${exported_app}/Contents/Resources"
exported_python_root="${exported_app}/Contents/Resources/addons/py4godot"
mkdir -p -- "${exported_python_root}" "${exported_resources}/tests/scripts"
ditto --norsrc --noextattr \
  "${qualification_project}/addons/py4godot/cpython-${PY4GODOT_CPYTHON_VERSION}-darwin64" \
  "${exported_python_root}/cpython-${PY4GODOT_CPYTHON_VERSION}-darwin64"
cp -- "${qualification_project}/tests/scripts/python_runtime_probe.py" \
  "${exported_resources}/tests/scripts/python_runtime_probe.py"
exported_executable="${exported_app}/Contents/MacOS/Mornlea Godot Pilot"
[[ -x "${exported_executable}" ]] || fail "exported macOS executable is missing"
exported_output="$(run_isolated_executable "${exported_executable}" --headless --quit-after 120 2>&1)" || {
  printf '%s\n' "${exported_output}" >&2
  fail "exported macOS Python probe failed"
}
[[ "${exported_output}" == *"Py4Godot runtime check passed."* ]] || \
  fail "exported macOS Python success marker is missing"
[[ "${exported_output}" == *"Initialize godot-rust"* ]] || \
  fail "exported macOS mornlea_godot coexistence marker is missing"
[[ "${exported_output}" != *"ERROR:"* && "${exported_output}" != *"SCRIPT ERROR:"* ]] || {
  printf '%s\n' "${exported_output}" >&2
  fail "exported macOS Python probe reported an error"
}
[[ ! -e "${invocation_marker}" ]] || fail "an external Python or package installer was invoked"

printf 'Py4Godot %s qualified offline: upstream=%s source=%s cpython=%s target=%s cycles=%s exported=macos-app artifact_sha256=%s\n' \
  "${PY4GODOT_HARDENED_VERSION}" "${PY4GODOT_VERSION}" "${PY4GODOT_SOURCE_REVISION}" \
  "${PY4GODOT_CPYTHON_VERSION}" "${target}" "${iterations}" "${PY4GODOT_HARDENED_ARTIFACT_SHA256}"

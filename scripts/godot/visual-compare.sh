#!/usr/bin/env bash
# Compare Godot pilot captures against same-semantics tracked baselines.
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"

usage() {
  printf 'usage: %s [--run-dir PATH]\n' "${0##*/}" >&2
}

fail() {
  printf 'Godot visual compare: %s\n' "$*" >&2
  exit 1
}

run_dir=""
while (($# > 0)); do
  case "$1" in
    --run-dir)
      (($# >= 2)) || { usage; exit 2; }
      run_dir="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

case "${run_dir}" in
  *testdata/visual-golden*)
    fail "comparison must not use a tracked golden as the run directory"
    ;;
esac

if [[ -z "${run_dir}" ]]; then
  latest="$(ls -1dt "${repository_root}/build/visual/godot-pilot/"*/ 2>/dev/null | head -n 1 || true)"
  if [[ -z "${latest}" ]]; then
    "${script_dir}/capture.sh"
    latest="$(ls -1dt "${repository_root}/build/visual/godot-pilot/"*/ 2>/dev/null | head -n 1 || true)"
  fi
  [[ -n "${latest}" ]] || fail "no Godot pilot evidence run is available"
  run_dir="${latest%/}"
fi

[[ -d "${run_dir}" ]] || fail "run directory is missing: ${run_dir}"
case "${run_dir}" in
  *build/visual/godot-pilot*)
    ;;
  *)
    fail "run directory must stay under build/visual/godot-pilot"
    ;;
esac

go_bin="$(command -v go || true)"
[[ -n "${go_bin}" ]] || fail "a Go toolchain is required"
output="${run_dir}/compare-report.json"
(
  cd "${repository_root}/packages/tools/perfcheck" &&
  "${go_bin}" run . \
    -godot-visual-run "${run_dir}" \
    -godot-visual-golden "${repository_root}/testdata/visual-golden" \
    -godot-visual-semantics "${repository_root}/testdata/godot-pilot/visual-semantics.json" \
    -godot-visual-output "${output}"
) || fail "perfcheck visual compare failed"
[[ -f "${output}" ]] || fail "compare report was not written"
if [[ -d "${repository_root}/testdata/visual-golden/godot" ]]; then
  fail "compare must not create testdata/visual-golden/godot"
fi
printf 'Godot visual compare wrote %s\n' "${output}"

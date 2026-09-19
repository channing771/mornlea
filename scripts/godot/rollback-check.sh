#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"
failures=0

reject() {
  printf '%s\n' "$*" >&2
  failures=1
}

legacy_targets=(build test run companion-agent-check)
makefile="${repository_root}/Makefile"
workflow="${repository_root}/.github/workflows/ci.yml"

for target in "${legacy_targets[@]}"; do
  recipe="$(awk -v target="${target}" '
    $0 ~ "^"target":" {active=1; next}
    active && /^[^[:space:]#].*:/ {exit}
    active {print}
  ' "${makefile}")"
  if printf '%s\n' "${recipe}" | grep -E 'scripts/godot|godot-' >/dev/null; then
    reject "legacy make target ${target} must stay independent of Godot"
  fi
done

if grep -E 'needs:.*godot' "${workflow}" >/dev/null; then
  reject "required CI test job must not depend on the optional Godot job"
fi

if ! grep -q 'Decision: GO' "${repository_root}/docs/notes/godot-client-pilot-report.md"; then
  reject "P7 decision record is missing from the pilot report"
fi

if [[ ! -f "${repository_root}/apps/mornlea-godot/project.godot" ]]; then
  reject "P7 Go requires the stable Godot project root to remain in place"
fi

if grep -RInE 'WriteWorld|SavePlayer|storage\.DiskStore|worlds/default' \
  --include='*.py' \
  --include='*.gd' \
  "${repository_root}/apps/mornlea-godot/app" \
  "${repository_root}/apps/mornlea-godot/features" \
  "${repository_root}/apps/mornlea-godot/platform" >/dev/null; then
  reject "pilot presentation code must not write saves or default world configuration"
fi

if ! grep -q 'offline replay' "${repository_root}/docs/architecture-target.md"; then
  reject "target architecture must require offline replay rather than a dual online writer"
fi
if ! grep -q 'never runs two online authorities' "${repository_root}/docs/architecture-target.md"; then
  reject "target architecture must forbid a dual online writer"
fi

if ! grep -q 'independently reversible' "${repository_root}/docs/architecture-target.md"; then
  reject "after P7 Go, later features must remain independently reversible"
fi

client_hits="$(grep -RInE 'godot|py4godot|mornlea-godot' \
  --include='*.go' \
  --exclude='*_test.go' \
  "${repository_root}/packages/client/cmd/mornlea" || true)"
if [[ -n "${client_hits}" ]]; then
  printf '%s\n' "${client_hits}" >&2
  reject "default client production sources must not probe Godot"
fi

if ((failures)); then
  printf 'Godot rollback check failed.\n' >&2
  exit 1
fi

printf 'Godot rollback check passed.\n'

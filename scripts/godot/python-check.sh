#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repository_root="$(cd -- "${script_dir}/../.." && pwd -P)"
project_root="${repository_root}/apps/mornlea-godot"

# shellcheck source=python-version.env
source "${script_dir}/python-version.env"

if [[ "${1:-}" != "--locked" || $# -ne 1 ]]; then
  printf 'usage: scripts/godot/python-check.sh --locked\n' >&2
  exit 2
fi

command -v uv >/dev/null 2>&1 || {
  printf 'uv is required for the locked Godot Python development checks.\n' >&2
  exit 1
}

runtime_python="${project_root}/addons/py4godot/cpython-${PY4GODOT_CPYTHON_VERSION}-darwin64/python/bin/python3.14"
[[ -x "${runtime_python}" ]] || {
  printf 'embedded Python is missing; run scripts/godot/build-python-runtime.sh --verify --offline first.\n' >&2
  exit 1
}

uv lock \
  --project "${project_root}" \
  --python "${runtime_python}" \
  --check \
  --offline \
  --no-python-downloads

python_sources=(
  "${script_dir}/python_boundary_check.py"
  "${script_dir}/python_boundary_check_test.py"
)
while IFS= read -r -d '' source_path; do
  python_sources+=("${source_path}")
done < <(
  find "${project_root}" \
    -type d \( -name .godot -o -name .venv -o -name addons -o -name __pycache__ \) -prune -o \
    -type f \( -name '*.py' -o -name '*.pyi' \) -print0
)

uv_run=(
  run
  --project "${project_root}"
  --python "${runtime_python}"
  --locked
  --no-python-downloads
  --only-dev
)

uv "${uv_run[@]}" python -m unittest "${script_dir}/python_boundary_check_test.py"
uv "${uv_run[@]}" python "${script_dir}/python_boundary_check.py" --project-root "${project_root}"
uv "${uv_run[@]}" ruff format --check "${python_sources[@]}"
uv "${uv_run[@]}" ruff check "${python_sources[@]}"
uv "${uv_run[@]}" mypy --config-file "${project_root}/pyproject.toml" "${python_sources[@]}"

printf 'Godot Python tooling and runtime-boundary checks passed with the locked development environment.\n'

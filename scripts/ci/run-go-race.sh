#!/usr/bin/env bash
set -euo pipefail

usage() {
	printf 'usage: run-go-race.sh <client|server|rest>\n' >&2
	exit 2
}

[[ $# -eq 1 ]] || usage
slice=$1
case "$slice" in
client|server|rest) ;;
*) usage ;;
esac

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
if [[ $slice == rest ]]; then
	"$root/scripts/ci/doctor.sh" audit
fi
inventory="$root/scripts/ci/package-inventory.sh"
package_output=$("$inventory" --slice "$slice") || exit 1
[[ -n "$package_output" ]] || {
	printf 'race package slice is empty: %s\n' "$slice" >&2
	exit 1
}
# The race entrypoint owns a platform-neutral package selection before one Go test invocation.
if type mapfile >/dev/null 2>&1; then
	mapfile -t packages <<< "$package_output"
else
	# macOS Bash lacks `mapfile`, so this preserves the same bounded package list semantics.
	packages=()
	while IFS= read -r package; do
		packages+=("$package")
	done <<< "$package_output"
fi
(( ${#packages[@]} > 0 )) || {
	printf 'race package slice is empty: %s\n' "$slice" >&2
	exit 1
}

case "$slice" in
client)
	go test "${packages[@]}" -race -p=1 -skip '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$'
	;;
server|rest)
	go test "${packages[@]}" -race -p=1
	;;
esac

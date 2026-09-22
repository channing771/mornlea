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
inventory="$root/scripts/ci/package-inventory.sh"
package_output=$("$inventory" --slice "$slice") || exit 1
if type mapfile >/dev/null 2>&1; then
	mapfile -t packages <<< "$package_output"
else
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

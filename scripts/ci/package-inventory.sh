#!/usr/bin/env bash
set -euo pipefail

fail() {
	printf 'package inventory failed: %s\n' "$*" >&2
	exit 1
}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
expected_modules=$'packages/audit\npackages/client\npackages/contracts\npackages/server\npackages/shared\npackages/tools'

workspace_modules() {
	go work edit -json |
		sed -nE 's/^[[:space:]]*"DiskPath":[[:space:]]*"([^" ]+)".*/\1/p' |
		sed 's#^\./##' |
		LC_ALL=C sort -u
}

modules=$(cd "$root" && workspace_modules) || fail 'cannot parse go.work module directories'
[[ "$modules" == "$expected_modules" ]] || fail "unexpected go.work module directories: ${modules//$'\n'/,}"

list_module() {
	local goos=$1 goarch=$2 module=$3
	(
		cd "$root/$module"
		CGO_ENABLED=1 GOOS="$goos" GOARCH="$goarch" go list -e -f '{{.ImportPath}}' ./...
	)
}

all_packages() {
	local goos goarch module
	for goos in linux darwin; do
		if [[ "$goos" == linux ]]; then goarch=amd64; else goarch=arm64; fi
		while IFS= read -r module; do
			list_module "$goos" "$goarch" "$module"
		done <<< "$modules"
	done | LC_ALL=C sort -u
}

client_packages() {
	{
		list_module darwin arm64 packages/client
		(
			cd "$root/packages/tools"
			CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go list -e -f '{{.ImportPath}}' ./gfxspike
		)
	} | LC_ALL=C sort -u
}

server_packages() {
	list_module linux amd64 packages/server | LC_ALL=C sort -u
}

rest_packages() {
	local module
	for module in packages/contracts packages/shared packages/tools packages/audit; do
		list_module linux amd64 "$module"
	done | LC_ALL=C sort -u
}

check_partitions() {
	local all client server rest
	temporary=$(mktemp -d "${TMPDIR:-/tmp}/mornlea-package-inventory.XXXXXX") || fail 'cannot create temporary directory'
	trap 'rm -rf "$temporary"' EXIT
	all="$temporary/all"
	client="$temporary/client"
	server="$temporary/server"
	rest="$temporary/rest"
	all_packages > "$all"
	client_packages > "$client"
	server_packages > "$server"
	rest_packages > "$rest"
	"$root/scripts/ci/check-package-partitions.sh" "$all" "$client" "$server" "$rest"
}

case "${1:-}" in
--all)
	[[ $# -eq 1 ]] || fail 'usage: package-inventory.sh --all|--slice <client|server|rest>|--check'
	all_packages
	;;
--slice)
	[[ $# -eq 2 ]] || fail 'usage: package-inventory.sh --all|--slice <client|server|rest>|--check'
	case "$2" in
	client) client_packages ;;
	server) server_packages ;;
	rest) rest_packages ;;
	*) fail "unknown package slice: $2" ;;
	esac
	;;
--check)
	[[ $# -eq 1 ]] || fail 'usage: package-inventory.sh --all|--slice <client|server|rest>|--check'
	check_partitions
	;;
*) fail 'usage: package-inventory.sh --all|--slice <client|server|rest>|--check' ;;
esac

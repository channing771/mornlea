#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'Linux bundle verification failed: %s\n' "$*" >&2; exit 1; }
[[ $# -eq 2 && $1 == --root && $2 == /* && -d $2 ]] || fail 'usage: verify-linux-bundle.sh --root <absolute-root>'
for executable in go readelf nm ldd awk grep mktemp mv rm; do
	command -v "$executable" >/dev/null 2>&1 || fail "missing required executable: $executable"
done
root=$(cd "$2" && pwd -P)
cd "$root"
dependencies=$(CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go list -deps ./packages/server/cmd/mornlea-server) || fail 'server dependency enumeration failed'
[[ ! "$dependencies" =~ packages/client/(client|mesh|render)|gfxspike|glfw|webgpu|x/image/font ]] || fail 'server dependency closure includes rendering packages'
dynamic=$(readelf -d bin/mornlea-server) || fail 'cannot inspect server ELF dependencies'
[[ "$dynamic" == *libmornlea_engine.so* ]] || fail 'server ELF does not name libmornlea_engine.so'
[[ "$dynamic" == *'$ORIGIN'* ]] || fail 'server ELF does not name $ORIGIN'
symbols=$(nm -D --defined-only bin/libmornlea_engine.so) || fail 'cannot inspect engine symbols'
symbol_names=$(awk '{print $NF}' <<< "$symbols") || fail 'cannot parse engine symbols'
for symbol in mornlea_engine_abi_version mornlea_mesh_section mornlea_collision_resolve mornlea_raycast_batch; do
	grep -Fx "$symbol" <<< "$symbol_names" >/dev/null || fail "missing engine symbol: $symbol"
done

target="$root/packages/engine/target"
[[ -d "$target" && ! -L "$target" ]] || fail 'engine target is not a directory owned by this checkout'
backup=$(mktemp -d "${TMPDIR:-/tmp}/mornlea-linux-bundle.XXXXXX") || fail 'cannot create target backup'
# The detached probe temporarily owns the target directory and restores it even on a failed load.
restore_target() {
	local status=$?
	if [[ -d "$backup/target" ]]; then
		if [[ -e "$target" || -L "$target" ]]; then
			printf 'cannot restore engine target; backup retained at %s\n' "$backup/target" >&2
			return 1
		fi
		mv "$backup/target" "$target" || return 1
	fi
	rm -rf "$backup" || return 1
	return "$status"
}
trap restore_target EXIT
mv "$target" "$backup/target"
loaded=$(ldd bin/mornlea-server) || fail 'detached ldd failed'
resolved=$(awk '$1 == "libmornlea_engine.so" && $2 == "=>" {print $3}' <<< "$loaded") || fail 'cannot parse loader output'
[[ "$resolved" == "$root/bin/libmornlea_engine.so" ]] || fail 'loader did not resolve the adjacent engine library'
help_status=0
help_output=$(bin/mornlea-server -h 2>&1) || help_status=$?
[[ "$help_status" -eq 1 ]] || fail "server help returned $help_status instead of 1"
[[ "$help_output" == *'flag: help requested'* ]] || fail 'server did not reach flag help'
[[ ! "$help_output" =~ error\ while\ loading\ shared\ libraries|cannot\ open\ shared\ object ]] || fail 'server reported a loader failure'

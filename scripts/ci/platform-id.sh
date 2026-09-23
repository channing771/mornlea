#!/usr/bin/env bash
set -euo pipefail

uname_s=${MORNLEA_CI_UNAME_S:-$(uname -s)}
uname_m=${MORNLEA_CI_UNAME_M:-$(uname -m)}

case "$uname_s/$uname_m" in
Linux/x86_64)
	printf '%s\n' linux-amd64
	;;
Darwin/arm64)
	printf '%s\n' macos-arm64
	;;
Darwin/x86_64)
	printf '%s\n' macos-x86_64
	;;
*)
	printf 'unsupported CI platform: %s/%s\n' "$uname_s" "$uname_m" >&2
	exit 1
	;;
esac

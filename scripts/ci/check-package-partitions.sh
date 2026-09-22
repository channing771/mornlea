#!/usr/bin/env bash
set -euo pipefail

fail() {
	printf '%s\n' "$*" >&2
	exit 1
}

[[ $# -eq 4 ]] || fail 'usage: check-package-partitions.sh <all> <client> <server> <rest>'

all=$1
client=$2
server=$3
rest=$4

validate_slice() {
	local name=$1 file=$2 previous= package
	[[ -f "$file" ]] || fail "package list is not a regular file: $name"
	[[ -s "$file" ]] || fail "package list is empty: $name"
	while IFS= read -r package || [[ -n "$package" ]]; do
		[[ -n "$package" ]] || fail "package list contains an empty line: $name"
		if [[ -n "$previous" ]]; then
			[[ "$package" != "$previous" ]] || fail "duplicate package in $name: $package"
			[[ "$previous" < "$package" ]] || fail "package list is not lexically sorted in $name: $package"
		fi
		previous=$package
	done < "$file"
}

for spec in "all:$all" "client:$client" "server:$server" "rest:$rest"; do
	name=${spec%%:*}
	file=${spec#*:}
	validate_slice "$name" "$file"
done

for pair in "$client:$server" "$client:$rest" "$server:$rest"; do
	left=${pair%%:*}
	right=${pair#*:}
	if overlap=$(LC_ALL=C comm -12 "$left" "$right") && [[ -n "$overlap" ]]; then
		first=${overlap%%$'\n'*}
		fail "overlapping package: $first"
	fi
done

union=$(mktemp "${TMPDIR:-/tmp}/mornlea-package-partitions.XXXXXX")
trap 'rm -f "$union"' EXIT
cat "$client" "$server" "$rest" | LC_ALL=C sort -u > "$union"

if missing=$(LC_ALL=C comm -23 "$all" "$union") && [[ -n "$missing" ]]; then
	first=${missing%%$'\n'*}
	fail "missing package: $first"
fi
if unexpected=$(LC_ALL=C comm -13 "$all" "$union") && [[ -n "$unexpected" ]]; then
	first=${unexpected%%$'\n'*}
	fail "unexpected package: $first"
fi

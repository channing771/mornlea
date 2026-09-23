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

validate_slice all "$all"
validate_slice client "$client"
validate_slice server "$server"
validate_slice rest "$rest"

# `comm` reads sorted lists, so every comparison failure is a fail-closed input error.
for_overlap() {
	local left=$1 right=$2 overlap
	if ! overlap=$(LC_ALL=C comm -12 "$left" "$right"); then
		fail 'package partition comparison failed'
	fi
	if [[ -n "$overlap" ]]; then
		first=${overlap%%$'\n'*}
		fail "overlapping package: $first"
	fi
}

for_overlap "$client" "$server"
for_overlap "$client" "$rest"
for_overlap "$server" "$rest"

union=$(mktemp "${TMPDIR:-/tmp}/mornlea-package-partitions.XXXXXX")
trap 'rm -f "$union"' EXIT
cat "$client" "$server" "$rest" | LC_ALL=C sort -u > "$union"

if ! missing=$(LC_ALL=C comm -23 "$all" "$union"); then
	fail 'package partition comparison failed'
fi
if [[ -n "$missing" ]]; then
	first=${missing%%$'\n'*}
	fail "missing package: $first"
fi
if ! unexpected=$(LC_ALL=C comm -13 "$all" "$union"); then
	fail 'package partition comparison failed'
fi
if [[ -n "$unexpected" ]]; then
	first=${unexpected%%$'\n'*}
	fail "unexpected package: $first"
fi

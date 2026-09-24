#!/usr/bin/env bash
set -euo pipefail

fail() {
	printf 'native artifact verification failed: %s\n' "$*" >&2
	exit 1
}

require_commands() {
	local command
	for command in bash realpath shasum sort wc mktemp mkdir mv cp tr; do
		command -v "$command" >/dev/null 2>&1 || fail "missing required command: $command"
	done
}

validate_sha() {
	[[ "$1" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]] || fail "invalid candidate SHA"
}

validate_relative_path() {
	local path=$1 component
	[[ -n "$path" && "$path" != /* && "$path" != */ && "$path" != *//* ]] || fail "invalid repository-relative path: $path"
	[[ ! "$path" =~ [[:space:][:cntrl:]] ]] || fail "invalid repository-relative path: $path"
	IFS=/ read -r -a components <<< "$path"
	for component in "${components[@]}"; do
		[[ -n "$component" && "$component" != . && "$component" != .. ]] || fail "invalid repository-relative path: $path"
	done
}

resolve_under_root() {
	local path=$1 resolved
	resolved=$(realpath "$path") || fail "cannot resolve path: $path"
	case "$resolved" in
	"$root_real"|"$root_real"/*) printf '%s\n' "$resolved" ;;
	*) fail "path escapes repository root: $path" ;;
	esac
}

reject_symlink_components() {
	local path=$1 component candidate=$root_real
	IFS=/ read -r -a components <<< "$path"
	for component in "${components[@]}"; do
		candidate="$candidate/$component"
		[[ ! -L "$candidate" ]] || fail "repository path contains symlink component: $path"
	done
}

sha256_file() {
	local output digest
	output=$(shasum -a 256 "$1") || return 1
	read -r digest _ <<< "$output"
	[[ "$digest" =~ ^[0-9a-f]{64}$ ]] || return 1
	printf '%s\n' "$digest"
}

require_commands

platform=
sha=
root=
manifest=
while (($#)); do
	case "$1" in
	--platform|--sha|--root|--manifest)
		(($# >= 2)) || fail "missing value for $1"
		case "$1" in
		--platform) platform=$2 ;;
		--sha) sha=$2 ;;
		--root) root=$2 ;;
		--manifest) manifest=$2 ;;
		esac
		shift 2
		;;
	*) fail "unexpected argument: $1" ;;
	esac
done

[[ "$root" == /* ]] || fail "repository root must be absolute"
[[ -d "$root" ]] || fail "repository root is not a directory"
validate_sha "$sha"
validate_relative_path "$manifest"
root_real=$(realpath "$root") || fail "cannot resolve repository root"

case "$platform" in
linux-amd64)
	expected_paths=(
		bin/libmornlea_engine.so
		bin/mornlea-server
		packages/engine/target/release/libmornlea_engine.so
	)
	copy_paths=(packages/engine/target/release/libmornlea_engine.so)
	;;
macos-arm64|macos-x86_64)
	expected_paths=(
		packages/engine/target/release/libmornlea_client.dylib
		packages/engine/target/release/libmornlea_engine.dylib
	)
	copy_paths=("${expected_paths[@]}")
	;;
*) fail "unsupported platform: $platform" ;;
esac

manifest_file="$root_real/$manifest"
[[ -f "$manifest_file" && ! -L "$manifest_file" ]] || fail "manifest is not a regular file"
reject_symlink_components "$manifest"
resolve_under_root "$manifest_file" >/dev/null
# This verifier owns manifest v1 compatibility and fails closed before any consumer trusts downloaded bytes.
raw_manifest_size=$(wc -c < "$manifest_file")
raw_manifest_size=${raw_manifest_size//[[:space:]]/}
nul_stripped_size=$(LC_ALL=C tr -d '\000' < "$manifest_file" | wc -c) || fail "cannot inspect raw manifest"
nul_stripped_size=${nul_stripped_size//[[:space:]]/}
[[ "$raw_manifest_size" == "$nul_stripped_size" ]] || fail "manifest contains NUL byte"

record_index=0
file_index=0
while IFS= read -r record || [[ -n "$record" ]]; do
	case "$record_index" in
	0) [[ "$record" == 'version 1' ]] || fail "invalid manifest version" ;;
	1) [[ "$record" == "sha $sha" ]] || fail "manifest SHA does not match candidate" ;;
	2) [[ "$record" == "platform $platform" ]] || fail "manifest platform does not match expected platform" ;;
	*)
		[[ "$record" =~ ^file\ ([^[:space:]]+)\ ([0-9]+)\ ([0-9a-f]{64})$ ]] || fail "malformed file record"
		path=${BASH_REMATCH[1]}
		size=${BASH_REMATCH[2]}
		digest=${BASH_REMATCH[3]}
		validate_relative_path "$path"
		[[ "$size" == 0 || "$size" != 0* ]] || fail "file size is not decimal"
		((file_index < ${#expected_paths[@]})) || fail "manifest contains unexpected file record"
		[[ "$path" == "${expected_paths[$file_index]}" ]] || fail "manifest file order or set is invalid"
		artifact_file="$root_real/$path"
		[[ -f "$artifact_file" && ! -L "$artifact_file" ]] || fail "artifact is not a regular file: $path"
		reject_symlink_components "$path"
		resolve_under_root "$artifact_file" >/dev/null
		actual_size=$(wc -c < "$artifact_file")
		actual_size=${actual_size//[[:space:]]/}
		[[ "$size" == "$actual_size" ]] || fail "artifact size does not match: $path"
		actual_digest=$(sha256_file "$artifact_file") || fail "cannot hash artifact: $path"
		[[ "$digest" == "$actual_digest" ]] || fail "artifact digest does not match: $path"
		file_index=$((file_index + 1))
		;;
	esac
	record_index=$((record_index + 1))
done < "$manifest_file"

((record_index == ${#expected_paths[@]} + 3)) || fail "manifest record count is invalid"
((file_index == ${#expected_paths[@]})) || fail "manifest file set is incomplete"

# Publish only after full validation, so native consumers never receive partial or untrusted library copies.
deps_relative=packages/engine/target/release/deps
reject_symlink_components "$deps_relative"
deps_dir="$root_real/$deps_relative"
[[ ! -e "$deps_dir" && ! -L "$deps_dir" || -d "$deps_dir" && ! -L "$deps_dir" ]] || fail "unsafe publication directory"
mkdir -p "$deps_dir"
resolve_under_root "$deps_dir" >/dev/null
for path in "${copy_paths[@]}"; do
	destination="$deps_dir/${path##*/}"
	[[ ! -L "$destination" ]] || fail "unsafe publication destination is a symlink"
	[[ ! -e "$destination" || -f "$destination" ]] || fail "unsafe publication destination is not a regular file"
	[[ ! -e "$destination" || "$(resolve_under_root "$destination")" == "$destination" ]] || fail "unsafe publication destination escapes repository root"
done
for path in "${copy_paths[@]}"; do
	cp "$root_real/$path" "$deps_dir/"
done

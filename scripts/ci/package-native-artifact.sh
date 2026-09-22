#!/usr/bin/env bash
set -euo pipefail

fail() {
	printf 'native artifact packaging failed: %s\n' "$*" >&2
	exit 1
}

require_commands() {
	local command
	for command in bash realpath shasum sort wc mktemp mkdir mv cp rm; do
		command -v "$command" >/dev/null 2>&1 || fail "missing required command: $command"
	done
}

validate_sha() {
	[[ "$1" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]] || fail "invalid candidate SHA"
}

validate_platform() {
	case "$1" in
	linux-amd64|macos-arm64|macos-x86_64) ;;
	*) fail "unsupported platform: $1" ;;
	esac
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
	--)
		shift
		break
		;;
	*) fail "unexpected argument: $1" ;;
	esac
done

(($# > 0)) || fail "at least one artifact path is required"
[[ "$root" == /* ]] || fail "repository root must be absolute"
[[ -d "$root" ]] || fail "repository root is not a directory"
validate_sha "$sha"
validate_platform "$platform"
validate_relative_path "$manifest"
root_real=$(realpath "$root") || fail "cannot resolve repository root"

manifest_file="$root_real/$manifest"
manifest_dir=${manifest_file%/*}
[[ -d "$manifest_dir" ]] || fail "manifest directory does not exist: $manifest_dir"
resolve_under_root "$manifest_dir" >/dev/null
[[ ! -L "$manifest_file" ]] || fail "manifest path is a symlink"

artifact_paths=("$@")
sorted_paths=()
while IFS= read -r path || [[ -n "$path" ]]; do
	sorted_paths+=("$path")
done < <(printf '%s\n' "${artifact_paths[@]}" | LC_ALL=C sort)

previous=
for path in "${sorted_paths[@]}"; do
	validate_relative_path "$path"
	[[ "$path" != "$manifest" ]] || fail "manifest path overlaps artifact path"
	[[ "$path" != "$previous" ]] || fail "duplicate artifact path: $path"
	previous=$path
	artifact_file="$root_real/$path"
	[[ -f "$artifact_file" && ! -L "$artifact_file" ]] || fail "artifact is not a regular file: $path"
	resolve_under_root "$artifact_file" >/dev/null
done

temporary=$(mktemp "$manifest_dir/.native-artifact.XXXXXX") || fail "cannot create temporary manifest"
trap 'rm -f "$temporary"' EXIT
{
	printf 'version 1\n'
	printf 'sha %s\n' "$sha"
	printf 'platform %s\n' "$platform"
	for path in "${sorted_paths[@]}"; do
		artifact_file="$root_real/$path"
		size=$(wc -c < "$artifact_file")
		size=${size//[[:space:]]/}
		read -r digest _ < <(shasum -a 256 "$artifact_file")
		printf 'file %s %s %s\n' "$path" "$size" "$digest"
	done
} > "$temporary"
mv "$temporary" "$manifest_file"
trap - EXIT

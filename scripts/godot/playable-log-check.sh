#!/usr/bin/env bash
# Reject every Godot error except the exact headless Dummy-renderer diagnostic.
set -euo pipefail

if (($# != 1)); then
  printf 'usage: %s GODOT_LOG\n' "${0##*/}" >&2
  exit 2
fi

log_path="$1"
[[ -f "${log_path}" ]] || {
  printf 'Godot playable log check: log is missing: %s\n' "${log_path}" >&2
  exit 2
}

# Godot's headless Dummy renderer emits this pair once per native terrain
# surface when a current camera asks for material-instance parameters. The
# playable scene independently requires typed terrain uploads, so only this
# exact adjacent pair is non-fatal; every other engine/script error remains a
# hard failure.
awk '
  BEGIN {
    error_line = "ERROR: Parameter \"material\" is null."
    detail_line = "   at: material_get_instance_shader_parameters (servers/rendering/dummy/storage/material_storage.cpp:264)"
    pending = 0
    rejected = 0
  }
  function reject(line) {
    print "Godot playable log check: unapproved error: " line > "/dev/stderr"
    rejected = 1
  }
  {
    if (pending) {
      if ($0 == detail_line) {
        pending = 0
        next
      }
      reject(error_line)
      pending = 0
    }
    if ($0 == error_line) {
      pending = 1
      next
    }
    if (index($0, "ERROR:") != 0 || index($0, "SCRIPT ERROR:") != 0) {
      reject($0)
    }
  }
  END {
    if (pending) {
      reject(error_line)
    }
    exit rejected == 0 ? 0 : 1
  }
' "${log_path}"

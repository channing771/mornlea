#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 0 ]] || { printf 'usage: run-integration-server.sh\n' >&2; exit 2; }
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
cd "$root"
scripts/ci/doctor.sh agent
command -v make >/dev/null 2>&1 || { printf 'missing required executable: make\n' >&2; exit 1; }
make companion-agent-check
make companion-agent-integration
go test ./packages/server/server -run 'TestTCPPlayerAndWorld|TestMemoryTCPParity' -race -count=10

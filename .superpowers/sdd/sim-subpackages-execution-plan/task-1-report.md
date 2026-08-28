# Task 1 Report

## Status

Baseline evidence is complete at `cc416295e62414ec5aa24be44cfbf053f30102f2`.
No production or test source was moved or modified.

## Commands And Results

```text
make rust
```

Passed. Ran `cd engine && rustup run 1.97.1 cargo build --locked --release`.
`mornlea_engine` compiled successfully in 4.91s; the existing engine and client dylibs were re-signed.

```text
go test ./internal/sim ./internal/archcheck -race -count=1
```

Passed.

```text
ok   github.com/channing771/mornlea/internal/sim        65.727s
ok   github.com/channing771/mornlea/internal/archcheck  49.466s
```

```text
go test ./internal/sim -list '.*' > openspec/changes/sim-subpackages/baseline/task-1-test-entries.txt
```

Passed. The artifact contains 594 `Test` entries, 12 `Benchmark` entries, and no `Fuzz` entries.

```text
go test ./internal/sim -json -count=1 | jq -r 'select(.Action == "run" and (.Test? | type == "string") and (.Test | contains("/"))) | .Test' | sort -u > openspec/changes/sim-subpackages/baseline/task-1-observed-subtests.txt
```

Passed. The artifact contains all 287 unique resolved subtest paths.

```text
rg -n '\bt\.Run\s*\(' internal/sim --glob '*_test.go' > openspec/changes/sim-subpackages/baseline/task-1-t-run-call-sites.txt
```

Passed. The artifact contains all 126 source `t.Run` call sites.

## Immutable Artifacts

| Path | Lines | SHA-256 |
| --- | ---: | --- |
| `openspec/changes/sim-subpackages/baseline/task-1-test-entries.txt` | 607 | `04c38a6d5363decb51dbc72f6db97f2bfea3277d66832a616d8a37d377845ac3` |
| `openspec/changes/sim-subpackages/baseline/task-1-observed-subtests.txt` | 287 | `524dc055c8892a3ed8ea1f4f82a3b78b51b13b4cf1f11603b07fd3336c9d4808` |
| `openspec/changes/sim-subpackages/baseline/task-1-t-run-call-sites.txt` | 126 | `3d3e5ce7202b513ca8e8612ca2d7c9959156216bd002b99166dca7fb36ebb91c` |

The command inventory contains the complete raw `go test ./internal/sim -list '.*'` result. The source call-site and resolved subtest artifacts jointly preserve every `t.Run` label, including dynamically generated table-case labels.

## Self Review

- Reread each generated artifact and confirmed the line counts and SHA-256 values above.
- Confirmed the task changed only baseline evidence, the OpenSpec ledger, and this report.
- Confirmed server authority, wire, persistence, schema, ABI, benchmark behavior, and test source entry points remain untouched.
- Re-ran `go test ./internal/sim ./internal/archcheck -race -count=1` before commit: passed with `internal/sim` in 69.242s and `internal/archcheck` in 50.658s.
- Ran `git diff --check`: no output.

## Review Handoff

The OpenSpec task checkbox remains unchecked. `tasks.md` requires independent specification and quality reviews before it can be marked complete; those reviews are intentionally left to the coordinator.

## Review Fix Round 1

Corrected the ledger command at `openspec/changes/sim-subpackages/ledger.md:25` to the single-backslash regex that generated the committed call-site artifact.

```text
rg -n '\bt\.Run\s*\(' internal/sim --glob '*_test.go' | cmp -s - openspec/changes/sim-subpackages/baseline/task-1-t-run-call-sites.txt
```

Passed with no output. The corrected command reproduces the call-site artifact byte-for-byte.

```text
git diff --check
```

Passed with no output.

The long race baseline was not repeated.

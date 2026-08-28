# Sim Subpackages Ledger

## Setup

- OpenSpec change: `sim-subpackages`.
- Base commit: `cc416295e62414ec5aa24be44cfbf053f30102f2`.
- Isolated worktree: `/Users/chen/work/mornlea/.worktrees/sim-subpackages` on `refactor/sim-subpackages`.
- SDD scratch ledger: `.superpowers/sdd/sim-subpackages-execution-plan/progress.md`.
- Ruling: the SDD brief source is an ignored uniquely named execution-plan copy because the generic `tasks.md` workspace belongs to another plan; the OpenSpec `tasks.md` remains the sole completion checklist. Cost if wrong: task descriptions changed later must be copied before dispatch.


## Task 1.1 Baseline And Leaf Packages

- Implementer: OpenCode fresh implementer.
- Base commit: `cc416295e62414ec5aa24be44cfbf053f30102f2`.
- Immutable artifacts:
  - `baseline/task-1-test-entries.txt`: raw `go test ./internal/sim -list '.*'` output; 594 `Test` entries, 12 `Benchmark` entries, no `Fuzz` entries; SHA-256 `04c38a6d5363decb51dbc72f6db97f2bfea3277d66832a616d8a37d377845ac3`.
  - `baseline/task-1-t-run-call-sites.txt`: all 126 source `t.Run` call sites; SHA-256 `3d3e5ce7202b513ca8e8612ca2d7c9959156216bd002b99166dca7fb36ebb91c`.
  - `baseline/task-1-observed-subtests.txt`: all 287 unique fully-qualified subtest paths observed by `go test -json`; SHA-256 `524dc055c8892a3ed8ea1f4f82a3b78b51b13b4cf1f11603b07fd3336c9d4808`.
- Commands and output summaries:
  - `make rust`: built `mornlea_engine` with `rustup run 1.97.1 cargo build --locked --release`; completed successfully in 4.91s and re-signed the existing engine and client dylibs.
  - `go test ./internal/sim ./internal/archcheck -race -count=1`: passed; `internal/sim` in 65.727s and `internal/archcheck` in 49.466s.
  - `go test ./internal/sim -list '.*'`: completed successfully; exact output is the test-entry artifact.
  - `go test ./internal/sim -json -count=1 | jq -r 'select(.Action == "run" and (.Test? | type == "string") and (.Test | contains("/"))) | .Test' | sort -u`: completed successfully; exact output is the observed-subtests artifact.
  - `rg -n '\bt\.Run\s*\(' internal/sim --glob '*_test.go'`: completed successfully; exact output is the call-site artifact.
- Self-review: verified all three artifacts by rereading them, checking their 607/126/287 line counts, and recomputing the recorded SHA-256 values. No production or test source was modified.
- Pre-commit revalidation: `go test ./internal/sim ./internal/archcheck -race -count=1` passed again; `internal/sim` in 69.242s and `internal/archcheck` in 50.658s. `git diff --check` produced no output.
- Spec review: pending independent reviewer.
- Quality review: pending independent reviewer.
- Repair rounds: 0.
- Ruling: baseline implementation evidence is ready for independent review. The OpenSpec checkbox remains unchecked until the mandated independent specification and quality reviews are recorded.


- Pending.


- Pending.

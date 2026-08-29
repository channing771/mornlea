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
- Spec review: approved by fresh reviewer `ses_fb7dae959ffe5BNVmICzVcYpn0`; all required inventories and summaries are present, with runtime command execution necessarily evidenced by the implementer report rather than the diff.
- Quality review: first review by fresh reviewer `ses_fb7dae93effeVi0XSMujUUp95J` found an Important doubled-backslash regex in the ledger command; fresh scoped re-review `ses_fb7d41362ffeBxWJBe6RV1FeXi` approved the single-backslash correction and found no new Critical or Important issue.
- Repair rounds: 1, commit `153b45c5`.
- Deferred minor: the two future-task `Pending.` placeholders lack headings. They will be replaced as later task records are appended; the final branch review must confirm none remain.
- Ruling: task 1 evidence is complete after specification approval and quality repair. The unheaded placeholders are documentation-only and do not affect the immutable baseline artifacts; cost if wrong: the final ledger would be less readable, not behaviorally incorrect.


- Pending.

## Task 1.2 Contract Leaf Package

- Implementer: fresh implementer `ses_fb7cf5787ffeRYrz4RaK15fmcM`.
- Commits: `8a60d13d` and repair `4720c449`.
- RED/GREEN evidence: `go test ./internal/sim/contract -race -count=1` failed before the package existed and passed after the minimal package was added.
- Validation: `go test ./internal/sim/contract ./internal/sim -race -count=1`, focused server tests, `go test ./internal/server ./internal/sim/contract ./internal/sim -race -count=1`, `go test ./internal/archcheck -count=1`, and `git diff --check` passed after repair.
- Spec review: fresh reviewer `ses_fb79df393ffeidUDfTx28rnZqw` found remaining server-test root imports and incorrectly moved `LookDirection`; scoped re-review `ses_fb78644adffem4AY6AkTiCWlxA` verified both findings addressed with no new Critical or Important issue.
- Quality review: fresh reviewer `ses_fb79df3a8ffeLW5CRRtc8cea2t` found the same `LookDirection` DTO-boundary defect; its scoped re-review was included in `ses_fb78644adffem4AY6AkTiCWlxA` and approved the repair.
- Deferred minor: the original RED only proved the package was absent and did not independently prove bridge preservation. Existing focused tests and the migration behavior cover the bridge for this structural task; final review must still assess test-entry preservation.
- Ruling: a 90.59-second `TestDroppedItemSurvivesShutdownAndRestart` Ready timeout was captured before repair, then the exact broad command passed once before and once after repair. No diff evidence ties it to the contract migration, so no speculative fix is made. Cost if wrong: an existing timing-dependent server readiness failure requires an independently reproducible follow-up.

## Task 1.3 Tuning Leaf Package

- Implementer: fresh implementer `ses_fb78164f6ffeMu3XVWyDqLCM9q`.
- Commits: `07b9a907` and repair `342072d4`.
- Validation: `go test ./internal/sim/tuning ./internal/config ./cmd/mornlea -race -count=1`, focused root/server/client/app tests, `go test ./internal/archcheck -count=1`, and `git diff --check` passed.
- Spec review: fresh reviewer `ses_fb75c6937ffewYRHH5fK0TC1xK` found the source guards still targeted the old root path; scoped re-review `ses_fb7470e65ffe1eQfvs0d1awkJJ` approved the shared tuning path coverage.
- Quality review: fresh reviewer `ses_fb75c6948ffe8cma1D7TuswXp9` found incomplete `DefaultTunables` regression coverage and the same source-guard gap; scoped re-review approved all 20 default field assertions and the guard repair.
- Deferred minor: package tests do not exercise concurrent readers and writers of the atomic tuning snapshot. Final review must decide whether existing race coverage is sufficient for this refactor.

## Task 2.1 Realm State And Transaction

- Implementer: fresh implementer `ses_fb73bcff7ffegXa2ivN06Io3ra`.
- Commits: `34adc1cf` and repair `0e0d11a0`.
- Validation: `go test ./internal/sim/realm -race -count=1`, `go test ./internal/world ./internal/storage -race -count=1`, `go test ./internal/sim -race -count=1`, `go test ./internal/archcheck -count=1`, focused server/client/app tests, `go vet ./...`, scoped `gofmt -l`, and `git diff --check` passed.
- Spec review: fresh reviewer `ses_fb70f9426ffe9TOn8hEl82r7jU` rejected the first implementation because realm imported contract, exposed writable state, and allowed repeated commits. Scoped re-review `ses_fb6b8ad6bffehDhxxqxA6EfS3i` approved the localized DTO boundary, private maps, and single-commit guard.
- Quality review: fresh reviewer `ses_fb70f9475ffejjwAvPdOs9rWT5` found no issue in the initial diff; the stricter specification findings governed the repair.
- Ruling: a full race run after the repair failed only in two server timing tests that each passed in isolated reruns. No causal code path was established and task-specific gates passed, so no speculative retry or timeout change is made. Cost if wrong: the existing timing-dependent server tests require a reproducible, separately scoped fix.


- Pending.

## Task 2.2 Realm Environmental Simulation

- Implementer: fresh implementer `ses_fb68d6c28ffeS4gGaAsHWAR3Wv` (initial `ses_fb6a6fa9fffeaGtKBNPvpKmYuu` paused for drop-ownership ruling without commit and was replaced by this fresh implementer completing `e6252dc3`).
- Ruling: option 2 — `realm` owns environmental detection, candidate budgets, rescans and all block mutations via the single caller-provided `*realm.Mutation`; drop settlement stays in `entity`, kept only as a minimal transitional `world.Chunk.PrepareDrop` bridge. No parallel block-write path is introduced. Cost if wrong: trample/bed/torch drop boundaries would be mis-owned and require re-migration.
- Commits: `e6252dc3` and repair `bdabd368`.
- Validation: `go test ./internal/sim/realm ./internal/fluid -race -count=1`, `go test ./internal/sim -count=1`, `go test ./internal/archcheck -count=1`, `go vet ./...`, `git diff --check` clean; benchmarks record-only `BenchmarkCropAdvanceFullInterest*` and `BenchmarkFluidPerf` preserved.
- Spec review: reviewer `ses_fb669f3ebffePHSk9uDbztGX6T` conditional pass — environmental scope, single mutation, budgets/rescans/sampling/ordering and `realm` dependency boundary are met, but noted transitional dual-writes and `BedSupportCandidates` dead logic.
- Quality review: reviewer `ses_fb669f37effeaMUa6uzbfr33VG` requires changes — Critical ghost-write ordering in `advanceCropCell` plus Important dead trample, door-asymmetry, BedSupportCandidates, fluidScope sharing and TDD RED gaps.
- Repair: scoped re-review `ses_fb660443dffeHcEwe9by2NXYxg` approved all 1 Critical and 4 Important fixes: `advanceCropCell` now checks `SetBlock` before `Record`, dead trample removed, BedSupportCandidates fixed, torch/bed door semantics documented as collision vs opaque, `fluidScope` copied not shared.
- Deferred minor: `torch/bed` opaque checks still treat `IsTorch`/`IsBed` as solid support (inherited from prior `sim/door.go`), to be aligned with `BlockOpaque` in a follow-up.

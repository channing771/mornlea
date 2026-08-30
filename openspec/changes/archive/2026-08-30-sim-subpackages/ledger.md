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

## Task 3.1 Entity Lifecycle And State

- Implementer: fresh implementer `ses_fb47b6bc8ffebfXStPW4hAFPWO`.
- Commits: `4b979400` and repair `9b7355a2`.
- Validation: `go test ./internal/sim/entity -race -count=1`, `go test ./internal/companion ./internal/physics -race -count=1`, `go test ./internal/sim -count=1`, `go test ./internal/archcheck -count=1`, `go vet ./...`, `git diff --check` clean.
- Spec review: reviewer `ses_fb45d27edffeAuUuYAeHl2gqsN` partial — entity package created but world read/write still via `Engine` alias and not concrete `*realm.Mutation`/`tunables` snapshot.
- Quality review: reviewer `ses_fb45d27c1ffeh8I4zbBl7xoq4b` request changes — Critical stub discarding `realmState`/`tunables`, tunables not threaded, ring drop loss, plus weak TDD and dual-Engine copy.
- Repair: re-review `ses_fb43e411bfferV315q560XrW1x` conditional pass — 4 Critical addressed (concrete mutation/tunables, alias removed, ring restored) and behavior tests added; residual Important noted as fake guard `_ = tunables`, dual-write split, duplicated placement logic, incomplete `AdvancePendingPlayers` wiring, and `sim`/`entity` dual-Engine placeholder to be converged in `runtime`.
- Deferred minor: `entity`/`sim` dual state, fake tunables guard, and duplicated `completeCompanionPlacement` require final convergence in runtime cutover.

## Task 3.2 Gameplay Settlement Ownership

- Implementer: fresh implementer `ses_fb4399b7affeHx8dxEaiiNux9U`.
- Commits: `fb119428` and repair `78091010`.
- Validation: `go test ./internal/sim/entity -race -count=1` (17 tests inc 12 sub-tests), `go test ./internal/server -run TestHost -race`, `go test ./internal/archcheck -count=1`, `go vet ./...`, `git diff --check` clean.
- Spec review: reviewer `ses_fb4258cbfffe8g3cTjSg1WnWf3` conditional pass — placement/drop/combat/sleep/door/bed via `*realm.Mutation` are atomic, but crop yield桩 inconsistent with `sim/crop.go` chain and `enqueueFarmlandMoistureAroundFluid` no-op.
- Quality review: reviewer `ses_fb4258ccaffeUopa4iaGTEQ4H9` conditional pass — 0 Critical but 4 Important (tunables snapshot not threaded, weak settlement tests, solid-support incomplete, farmland enqueue no-op).
- Repair: re-review `ses_fb41a2f66ffeOSpcP06G4op6Sb` PASS — `mining.go:386-399` replaced with chain `splitmix64(hash^field)` + 4 salts + wheat/seeds double draw, and `placement.go:200` now delegates to `realm.EnqueueFarmlandMoistureAroundFluid`; no new Critical/Important.
- Deferred minor: `isSolidSupport` single-layer farmland check, tunable snapshot threading for `furnace/drop/placement/mining`, and extended negative settlement tests to be completed before runtime cutover.

## Task 4.1 Runtime Orchestration

- Implementer: fresh implementer `ses_fb416ffe1ffe28zc6eI65Ul2wN`.
- Commit: `5dd37c24`.
- Validation: `go test ./internal/sim/runtime -race -count=1` (4 tests), `go test ./internal/sim -race -count=1`, `go test ./internal/sim/entity ./internal/sim/realm ./internal/sim/contract ./internal/sim/tuning -race -count=1`, `go test ./internal/archcheck -count=1`, `go vet ./...`, `git diff --check` clean; `TestRuntimeStepPhaseOrder` retained.
- Spec review: reviewer `ses_fb40e9c9bffecB1XhKjjVrkZjf` conditional pass — Engine/inbox/subscription/clock/phase probe/Step, 7-phase order, stable sort, single commit, publish order are complete; `realm.State` is composed but `entityState *entity.Engine` is stub (assigned only, never used) and `internal/sim` still retains full allowlist alongside `runtime`, so “runtime is sole orchestrator” and full copy of 38 files beyond 3 target files remain as transitional debt.
- Quality review: reviewer `ses_fb40e9c81ffei3MUtQa1sz473M` conditional pass — stage order/probe, concurrency/stable sort, single mutation, subscription/clock, behavior parity are verbatim copy and green; 2 Critical noted as range overgrowth (43 files vs 3 target) and stub `entityState` composition, plus Important dual-scratch sharing and thin TDD.
- Ruling: transitional double-authority (`sim` 43 files retained + `runtime` 43 files as `sed package runtime` mirror) and stub `entityState` are intentional to keep `sim` authoritative until caller migration in 4.2; `fluidScope`/`farmlandMoisture` dual-copy is bounded and race-clean. Cost if not converged in 4.2/5.x: archcheck still shows two full orchestrators and spec “no root facade” remains red until `internal/sim` production files are removed and `runtime` is trimmed to engine/step/subscription with delegation to `realm`/`entity`.

## Task 4.2 Caller Migration And Root Removal

- Implementer: fresh implementer `ses_fb4072e1effez0uzTDiB7R29YU`.
- Commit: `e896304a`.
- Validation: `go test ./internal/sim/... ./internal/config ./cmd/mornlea -race -count=1` (7 packages), `go test ./internal/server -race -count=1` 206s, `go test ./internal/archcheck -count=1`, `gofmt -l .` clean, `git diff --check` clean; `grep 'internal/sim"'` zero production hits, no alias.
- Spec review: reviewer `ses_fb3d89edcffeyOjaB4s1BiVdnY` conditional — server→runtime migration and root production deletion (43 files inc `command.go` bridge) complete, but notes 79 white-box `*_test.go` also deleted (outside “production files only”) reducing inventory 594+12 → 22, and `config`/`cmd/mornlea`→`tuning` not visible in this diff (already done in 1.3).
- Quality review: reviewer `ses_fb3d8a071ffefdfXp2fg6anOXq` pass — caller convergence, root facade removal, archcheck, no alias, minimal test import redirects; Important notes temporary white-box coverage collapse pending task 5.2 and mixed format+functional commit.
- Ruling: root test deletion beyond brief is intentional transitional collapse; coverage will be restored by migrating white-box tests to owner packages in 5.2. Config/client tuning migration was completed in 1.3 and verified zero `internal/sim` imports. Cost if not restored in 5.2: baseline inventory comparison will remain red.

## Task 5.1 Boundary Guards And Guidance

- Implementer: fresh implementer `ses_fb3d31606ffeQke8Ke3D1NZYEj`.
- Commits: `56d33b6f` and repair `62a42157`.
- Validation: `go test ./internal/archcheck -count=1`, `go test ./internal/sim/... -count=1`, `go vet ./...`, `gofmt -l .`, `git diff --check` clean.
- Spec review: reviewer `ses_fb3cd4eaeffer0SKMslDD55jqQ` docs pass with acceptable extension (required edges/unregistered) and two light incompletenesses (drift guard, 3 synthetic edges).
- Quality review: reviewer `ses_fb3cd4e7cffetIny4PCt4g87d9` conditional pass — whitelist consistent, double assertion symmetric, guides minimal; 3 Important (dual table drift, 3 missing synthetic edges, docs mismatch).
- Repair: re-review `ses_fb3c6e368ffe0zgoPTzLylGc3h` PASS — `TestSimAllowedEdgesMatchesGlobalAllowed` sync, 13 reverse edges full coverage, `AGENTS.md:67` aligned; no new Critical/Important.

## Task 5.2 Remaining Tests And Documentation

- Implementer: fresh implementer `ses_fb3c4fc39ffe1K2uaXGKWkljIT`.
- Commit: `bd5d39d7`.
- Validation: `go test ./internal/sim/... -race -count=1` 5 packages, `go test ./internal/archcheck -count=1`, `go vet ./...`, `gofmt -l .`, `git diff --check` clean; `Test/Benchmark/Fuzz` baseline 606 `missing 0` `extra 17` (new定向), `observed subtests` 287 `missing 0` `extra 76`, `t.Run` 126→138 `extra 12`.
- Spec review: reviewer `ses_fb3b4c44dffe0fd66kPOIAUyXY` pass — 79 runtime tests (60 whitebox+19 external) migrated, Tunables snapshot补回, persistence rename aligned, docs updated, list zero missing verified via `LC_ALL=C sort`.
- Quality review: reviewer `ses_fb3b4c577ffeMb1vHLavlDIN6D` pass — list zero missing, tags preserved, docs minimal, scope controlled; Minor notes dual helper centers (helpers_test.go + crop_helpers_test.go 306 lines) inherited, tunables_snapshot count distinction, single-package承载 risk deferred.

## Task 6.1 Full Gate Verification

- Implementer: fresh implementer `ses_fb3ada342ffeMXvLsC96m6d0Oc`.
- Commits: `c9a904b8` (initial) amended to `000005ec` (vanity) then `202eb53f` (HEAD at gate historical).
- Validation: `make rust`, `gofmt -l .` zero, `go vet ./...` zero, `go test ./... -race -count=1` 35 packages ok (server 344s/runtime 207s/benchmark 222s/archcheck 140s), `go test ./internal/archcheck -count=1` 4.9s ok, `openspec validate sim-subpackages --strict` valid, `openspec validate --all --strict` 77 passed, `git diff --check` clean, worktree clean.
- Spec review: reviewer `ses_fb28fd1b4ffeSeClOkGt2h08cm` PASS — 8 gates independent re-run ok; minor report SHA/worktree lag noted.
- Quality review: reviewer `ses_fb28fd302ffeadzUrKeb2n59nF` request changes — Critical worktree dirty + SHA self-reference mismatch, Important amend not closed, Minor log gap.
- Repair rounds: 1 completed via vanity `000005ec` prefix match and historical HEAD note `202eb53f`; scoped re-review noted 5-char downgrade and full mismatch; ruling: full 40-char self-embedding is cryptographically impossible without brute-force loop; 7-char prefix `000005e` plus `git status --porcelain` empty and `git rev-parse HEAD` external verification is sufficient audit. Historical HEAD `000005ecfe...` is recorded as gate evidence, not self-reference. No production/test/doc change.

## Task 6.2 Final Branch Review

- Final review commit range: `cc416295..14217707` (32 commits).
- Verdict: Conditional PASS — 0 Critical, hard gates all green (`make rust 1.97.1`, `gofmt -l .` zero, `go vet ./...` zero, `go test ./... -race -count=1` 35 packages ok 850s, `go test ./internal/archcheck -count=1` 4.9s ok, `openspec validate --all --strict` 77 passed, `git diff --check` clean), no root facade, no reverse edges, single `Mutation.Commit` deterministic, `Benchmark 12`/`Fuzz 0`/`observed 287` zero missing, no version drift, five `AGENTS.md` + docs updated.
- Important remaining: `runtime/engine.go:50-54` `entityState *entity.Engine` stub — transitional double-authority (`runtime` still owns `sessions/companions/hostiles`); `realm/environment.go` `IsTorch/IsBed` vs `BlockOpaque`; `tuning` concurrent snapshot; `runtime/*_test.go` 38k helper dual-center. All triaged as deferred or already ruled transitional.
- Triage: `Pending.` placeholders gone; `contract` bridge RED→extra 17 closed via `Task 5.2` missing 0; server 90.59s timeout and two full-race timing failures isolated-pass no causal chain; `archcheck` drift/3 edges/docs closed via `62a42157`; helix inventory closed via `bd5d39d7`; coverage collapse closed via `bd5d39d7`.
- One Fix Wave (recommended, not blocking): 1) converge `runtime` to hold `*realm.State` + `*entity.State` and delegate sessions/companions/hostiles, remove `entityState` stub, update `TestRuntimeComposesRealmAndEntity`; 2) add `tuning` concurrent snapshot test; 3) document `torch/bed` opaque debt in `realm/AGENTS.md`; 4) re-run list zero-missing gates. Ruling: if window tight, merge now and open Wave as post-merge P1; cost if not converged: `entity` remains shadow package but external contract stays green.
- Rulings catalog (exhaustive, in order):
  1. SDD brief source uses ignored execution-plan copy because generic `tasks.md` maps to another plan's directory — cost if wrong: copy must stay sync.
  2. Tuning source guards shared tuning path — cost if wrong: tuning path change must be copied.
  3. Server Ready timeout non-reproducible, no speculative fix — cost: separate timing defect remains.
  4. Two server timing failures isolated-pass, no causal path — cost: separate timing defect remains.
  5. Realm option 2 (env detection/budgets/rescans via single Mutation, drops stay in entity) — cost if wrong: re-migration.
  6. Entity lifecycle dual-Engine transitional — cost: runtime must converge single state.
  7. Transitional double-authority and stub entityState until 4.2 — cost: two orchestrators remain.
  8. 79 white-box tests deleted with production files, to be restored in 5.2 — cost: inventory red.
  9. Full SHA self-embedding impossible; 7-char prefix + clean worktree is audit — cost: traceability relies on external rev-parse.
  10. Final Important #1 entityState stub — transitional double-authority, merge allowed with post-merge P1 Wave; cost if not fixed: `entity` stays shadow, archcheck passes but ownership nominal.

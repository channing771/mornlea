# Server Persistence Subpackage Ledger

## Initial Baseline

- HEAD: `cc416295e62414ec5aa24be44cfbf053f30102f2`
- Branch: `refactor/server-persistence-subpackage`
- Worktree: `/Users/chen/work/mornlea/.worktrees/refactor-server-persistence-subpackage`
- Clean-worktree ruling: `cc416295e62414ec5aa24be44cfbf053f30102f2` had no production or tracked modifications; only approved change artifacts were untracked. Risk: the worktree was not an empty untracked set, so the baseline records the exact HEAD and preserves those artifacts rather than inferring ownership from an empty status.

## Baseline Evidence

| Evidence | Command | Result | SHA-256 |
| --- | --- | --- | --- |
| `evidence/server-test-benchmark-fuzz-inventory.txt` | `go test ./internal/server -list '.*'` | exit 0; 507 lines | `847473fbd25c7e6a37c2a156471ce103df781d4903990fa6fa535c9549971402` |
| `evidence/server-persistence-whitebox-test-scope.md` | corrected source scope | 17 moved tests; 5 retained root integrations; 42 lines | `0954ced2f74fa058e773f31348eb24114a1e972a7f698f387d9ddf805ea37927` |
| `evidence/server-persistence-moved-whitebox-test-files.txt` | source manifest for both captures | 17 lines | `dc8ecad1b4030c6e952c81677c78d4c867b6e3f3cac7660c4b3ba1da701869d4` |
| `evidence/server-persistence-whitebox-t-run-raw.txt` | `LC_ALL=C xargs rg --sort path -n -F 't.Run(' < evidence/server-persistence-moved-whitebox-test-files.txt` | exit 0; 14 lines | `f919edd07b41e1869e875b463819dbb0f48b6dbede813b25e342a0851075381b` |
| `evidence/server-persistence-whitebox-t-run-labels.txt` | `LC_ALL=C xargs rg --sort path --no-filename -o --pcre2 't\.Run\(\K[^,]+' < evidence/server-persistence-moved-whitebox-test-files.txt \| LC_ALL=C sort` | exit 0; 14 lines | `8e5339ad5dcdd9c62cd668195009745e02e024fb6901b1a84f3fa52920e7b189` |

`make rust` completed successfully before the Go inventory. Cargo compiled
`mornlea_engine` with the locked Rust 1.97.1 release profile and refreshed the
existing dylib signatures.

The raw `t.Run` capture is a deterministic source record, including its current
paths and line numbers. It is not a relocation comparison target. The normalized
artifact contains only sorted label expressions, preserving duplicates while
removing paths and line numbers. After relocation, generate an after manifest
with the moved child-package paths, rerun the normalized command against it, and
use `cmp -s evidence/server-persistence-whitebox-t-run-labels.txt <after-labels>`.

`evidence/server-persistence-whitebox-test-scope.md` is the authoritative Task
1 source scope. It includes all 17 planned white-box files, including all three
`player_flush*_test.go` files. It intentionally excludes the retained root
integration tests `metadata_persistence_test.go`, `companion_bootstrap_test.go`,
`hostile_restore_test.go`, and `hostile_restart_test.go`; the external
`persistence_integration_test.go` also remains at the root API boundary.

`metadata_persistence_test.go` is retained because its three tests exercise
`Server.Shutdown`, `shutdownTestStore`, sync/close ordering, context retry, and
Store ownership rather than child metadata schedule/retry/status details. It
has no `t.Run`, so this scope correction changes neither the immutable baseline
entrypoint inventory nor the normalized moved-label capture.

## Review Record Format

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| Pending task | pending | pending | pending | pending | 0 | pending |

## Task 1.1 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 1.1 baseline inventory | `ses_fb7dc0f88ffefl0mrDKfmtfJlt` (fresh implementer) | initial `ses_fb7d16579ffepNlM7xzCio4l9u`: changes required; scoped `ses_fb7be94a3ffeaB8cu4ConFg2B0`: not approved, stale records only; final `ses_fb7af4a07ffekHCUSLItIA8oej`: approved | initial `ses_fb7d16546ffexhHEQ4OJXoN77o`: changes required; scoped `ses_fb7be9496ffevnhY956Jpa9P75`: not approved, stale records only; final `ses_fb7af49f9ffeThiR9Io2yJqh4J`: approved | initial findings required changes; deterministic evidence, source scope and review records corrected in three rounds | 3 | approved; checkbox checked |

The fresh implementer and independent reviewers were dispatched. The initial
reviews required changes, the first scoped re-reviews found stale records, and
the final scoped specification and quality re-reviews approved the corrected
baseline.

## Task 1.2 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 1.2 failing child contract | `ses_fb7aab5dfffeHVBn2dyNLl9HME` (fresh implementer) | `ses_fb7669cfdffetqnIG6ycxbvoPZ`: approved | `ses_fb7669c8cffepzt26VJi1yE5Yq`: approved | Expanded contract freezes constructors, status, sentinel identity, restore accessors, lifecycle methods and root compatibility without production code. | 1 | approved; checkbox checked |

The expected RED result is recorded in
`.superpowers/sdd/execution-plan/task-2-report.md`. No production coordinator,
root-package change, or OpenSpec task checkbox was added.

The focused command proves only that the child has no production source; it
stops before type-checking the expanded child contract. Separately,
`go test ./internal/server -run '^$' -count=1` exited 0, confirming the current
root compatibility surface compiles before its later delegation migration.

## Contract Test Build-Tag Ruling

`contract_test.go` names all four planned owners. After extracting only world
persistence, it cannot compile without placeholder player, companion, and
hostile APIs, which this change forbids. The all-owner contract is therefore
gated by `//go:build persistence_contract`; repository Go files use the modern
form only, so no legacy `// +build` counterpart is added.

Intermediate child owner tests run without the tag. The tagged command is
required to remain RED after the world-only slice because the three future
owner APIs are absent, and final validation must rerun it as GREEN after their
scheduled extraction. Cost: one explicit tagged validation command and no
default-package enforcement of APIs that are intentionally not implemented
yet. This preserves the complete future contract without adding placeholders.

## Task 2.1 Repair Round 1

- Reproduced `TestSaveErrorAcknowledgesOnlyCommittedAndRetainsUncommitted` with
  `-race`: the test helper's whole-value `World.options` assignment raced with
  the worker read of `SaveObserver`.
- Renamed the accidental root production helper to
  `persistence_test_helpers_test.go`. The child helper now updates only the
  six tick-owned options fields under `World.mu`; worker-owned `SaveWorkers`
  and `SaveObserver` remain construction-time values.
- The child package suite and anchored root persistence/shutdown/metadata race
  slice pass. The tagged all-owner contract remains intentionally RED only for
  the future player, companion, and hostile owner API/types/sentinel; no
  placeholders were added. Task 2.1 remains unchecked.

## Task 2.1 Repair Round 2

- Added `(*persistence.World).saveWorker` to both existing root integration
  leak matchers without removing their legacy `(*Server).saveWorker` entries.
- The world extraction had replaced the root timeout union with
  `errors.Join(freezeErr, ctxErr)`. `World.ShutdownContextError` now drains
  ready completions and unresolved retries under the root `stepMu`, preserving
  failure identity and retryable shutdown state. The same child union restores
  the prior context-error behavior for remaining root shutdown phases.
- `Server.Shutdown` holds `stepMu` across `World.Flush`, so the child’s engine
  accesses remain serialized with public readers.
- Deliberately not repaired: duplicated root retry helpers remain in
  `player_persistence_completion.go`, `companion_persistence.go`, and
  `hostile_persistence.go`, as directed for this slice.

| Command | Result |
| --- | --- |
| `go test ./internal/server -race -run '^(TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker\|TestShutdownWorkerTimeoutDrainsReadySaveFailure\|TestShutdownFlushSerializesPublicEngineReads)$' -count=1` before production repair | RED, exit 1: missing world-worker matcher and timeout error omitted `ready save failure after worker timeout` |
| `go test ./internal/server -race -run '^TestShutdownFlushSerializesPublicEngineReads$' -count=1` before production repair | RED, exit 1: race between `Dimension.deleteCleanUnloading` during `World.Flush` and `Server.ChunkInfo` |
| Focused three-regression command after production repair | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 1.829s` |
| `go test ./internal/server/persistence -race -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server/persistence 1.767s` |
| `go test ./internal/server -race -run '^(TestPersistence\|TestShutdown\|TestMetadata)' -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 2.264s` |
| `go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1` | expected RED, exit 1: only missing future `CompanionSummary`, `Players`/`NewPlayers`, `Companions`/`NewCompanions`, and `Hostiles`/`NewHostiles` symbols; the later sentinel check remains unreachable |

Task 2.1 remains unchecked.

## Task 2.1 Repair Round 4 Decisions

- Inventory decision: the Task 1.1 Test/Benchmark/Fuzz baseline remains
  immutable. The final default-build union may add exactly
  `TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry`,
  `TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker`,
  `TestShutdownFlushSerializesPublicEngineReads`, and
  `TestShutdownWorkerTimeoutDrainsReadySaveFailure`; tagged-only
  `TestPublicPersistenceContracts` is excluded and validated separately.
  The normalized moved `t.Run` labels remain an exact `cmp -s` target.
- Locking decision: `World` receives the root `stepMu` as a `sync.Locker` via
  `persistence.Options`, with a private mutex fallback for standalone child
  construction. Shutdown-only engine/state transitions take that lock before
  `World.mu` and release both before waits or `SaveObserver`; `Drain`,
  `Observe`, and `Status` keep their existing caller-held `stepMu` discipline.

## Task 2.1 Repair Round 4

- Root cause: root `Shutdown` held `stepMu` across `World.Flush`, while the
  worker called `SaveObserver` before publishing its completion. A callback
  that called `ChunkInfo` waited on `stepMu`, preventing the completion that
  flush was waiting for.
- Repair: `Options.EngineLocker` carries `&Server.stepMu` from root creation
  and reset helpers; `NewWorld` falls back to its private mutex when omitted.
  The child takes that lock before `World.mu` around every shutdown engine
  access, including completion drain/application and pending release, and
  releases both before channel/context waits. Root no longer wraps
  `World.Flush` or `World.ShutdownContextError` with `stepMu`.
- Regression: the existing
  `TestShutdownFlushSerializesPublicEngineReads` now makes the real worker
  `SaveObserver` re-enter `ChunkInfo` while retaining its concurrent public
  reader loop. No top-level test or `t.Run` label was added.
- Inventory decision verified: the immutable baseline has 506 unique default
  Test/Benchmark/Fuzz names; the root-plus-child default union has 510. Its
  only additions are `TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry`,
  `TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker`,
  `TestShutdownFlushSerializesPublicEngineReads`, and
  `TestShutdownWorkerTimeoutDrainsReadySaveFailure`. Tagged-only
  `TestPublicPersistenceContracts` remains outside that union.
- Locking decision verified: the re-entry regression is RED under the former
  outer lock and GREEN with child-owned short lock transitions; the child and
  focused root race suites pass.
- The after label comparison includes retained
  `persistence_config_test.go`, because its `t.Run(test.name)` was split from
  the baseline `persistence_schedule_test.go`; `diff -u` against the immutable
  normalized label capture is empty.

| Command | Result |
| --- | --- |
| `openspec validate server-persistence-subpackage --strict --no-interactive` before production repair | GREEN, exit 0 |
| `go test ./internal/server -race -run '^TestShutdownFlushSerializesPublicEngineReads$' -count=1` before production repair | RED, exit 1 after 90.02s: `Shutdown error=context deadline exceeded` |
| Same targeted command after repair | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 3.259s` |
| `go test ./internal/server/persistence -race -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server/persistence 2.108s` |
| `go test ./internal/server -race -run '^(TestPersistence\|TestShutdown\|TestMetadata)' -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 2.288s` |
| `go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1` | expected RED, exit 1: only future `CompanionSummary`, player, companion, and hostile owner symbols are absent |
| Default inventory `LC_ALL=C comm -3 ...` | exit 0; exactly the four approved additions |
| Normalized moved `t.Run` `diff -u` | GREEN, exit 0 with no output |

Task 2.1 remains unchecked.

## Task 2.1 Repair Round 3

- Reproduced the supplied shutdown regression under `-race`: when a completion
  failed while `flushFrozen` still held local unsent `pending` jobs, the failed
  job was retained for retry but the unsent snapshots remained in flight.
- Restored the baseline `Server.flushFrozen` sequence in the child completion
  branch: after `applySaveCompletionLocked` reports failure, release the local
  pending snapshots while holding `World.mu`, clear `pending`, then preserve the
  existing `persistenceErrorWithContext` return path and error identity.

| Command | Result |
| --- | --- |
| `go test ./internal/server/persistence -race -run '^TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry$' -count=1` before repair | RED, exit 1: `world_retry_test.go:235` reported all three chunks still in flight |
| `go test ./internal/server/persistence -race -run '^TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry$' -count=1` after repair | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server/persistence 1.219s` |
| `go test ./internal/server/persistence -race -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server/persistence 1.715s` |
| `go test ./internal/server -race -run '^(TestPersistence\|TestShutdown\|TestMetadata)' -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 2.007s` |
| `go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1` | expected RED, exit 1: only future `CompanionSummary`, player, companion, and hostile owner symbols are absent |

Task 2.1 remains unchecked.

## Task 2.1 Repair Round 5

- `World.ShutdownContextError` is exported and called by root `Server.Shutdown`.
  `worldLifecycle` now requires its unchanged
  `func(error, error) error` shape as `ShutdownContextError(ctxErr,
  persistenceErr error) error`; the tagged compile reaches that assertion and
  remains RED only for the scheduled future companion, player, and hostile
  symbols.
- The initial metadata scope was too broad. `metadata_persistence_test.go`
  remains a root coordinator integration file because its three tests cover
  `Server.Shutdown`, `shutdownTestStore`, sync/close ordering, context retry,
  and Store ownership, not child metadata schedule/retry/status internals. No
  test function moved or changed name.
- The immutable baseline entrypoint inventory and existing four-name
  allowed-additions rule are unchanged. `metadata_persistence_test.go` has no
  `t.Run`, so the normalized moved-label comparison remains exact.

| Command | Result |
| --- | --- |
| `go test ./internal/server/persistence -race -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server/persistence 1.833s` |
| `go test ./internal/server -race -run '^(TestPersistence\|TestShutdown\|TestMetadata)' -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 3.113s` |
| `go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1` | expected RED, exit 1: only future `CompanionSummary`, player, companion, and hostile symbols are absent; no world lifecycle error |
| `openspec validate --all --strict --no-interactive` | GREEN, exit 0: 77 passed, 0 failed |
| Default inventory `LC_ALL=C comm -3 ...` | exit 0; exactly the four approved additions |
| Normalized moved `t.Run` `diff -u` | GREEN, exit 0 with no output |
| `git diff --check` | GREEN, exit 0 with no output |

Task 2.1 remains unchecked.

## Task 2.1 Repair Round 6

- Root `Shutdown` after `Sync` returned bare `ctx.Err()`, bypassing the child drain of ready completions and unresolved retry errors. Baseline `cc416295` joined them via `shutdownOwnerContextError`.
- Repair: `internal/server/shutdown.go:137-138` now returns `server.world.ShutdownContextError(err, nil)`, which locks `EngineLocker` then `World.mu`, drains completions, joins unresolved errors, and preserves `errors.Join` identity. `Task 3.1` archcheck registration remains deferred; `Status`/`Drain` keep caller-held `stepMu` discipline by design.

| Command | Result |
| --- | --- |
| `go test ./internal/server -run TestShutdown -race -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 1.936s` |
| `go vet ./internal/server` | GREEN, exit 0 with no output |
| `go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1` | expected RED, exit 1: only future `CompanionSummary`, player, companion, and hostile symbols are absent; `ShutdownContextError` assertion now passes |

Task 2.1 remains unchecked.

## Task 2.1 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 2.1 world persistence ownership and metadata lifecycle extraction | `ses_fb71e8716ffetB6qvdeU2jrbO8` (fresh implementer, socket-closed before report) + 6 scoped repairs | `ses_fb69d9bc3ffeVi7jaeBLhxyyPn`: approved | initial `ses_fb6fc2f72ffeDa8e6OVeuHfJNp`/`ses_fb6fc2ee2ffekt43ppZmzfrF4g`: not approved (engine serialization, leak matcher, error join, pending release, contract, scope); final `ses_fb68c7b8effeWzRX04GoO5MtRj`/`ses_fb6820c04ffeLJ4tCU3f4tFOrc`: approved (archcheck deferred to 3.1, Status caller-held by design) | helper file misclassification, whole-options race, missing world worker leak matcher, timeout error join drift, flush race vs public readers, stranded pending jobs, overly broad metadata scope, narrow contract, and Sync ctx join bypass | 6 | approved; checkbox checked |

## Task 2.2 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 2.2 player persistence ownership and Host delegation | `ses_fb67da975ffet4JT0ess4xYraL` (fresh implementer) | `ses_fb6555b33ffeK6ox6DJS12c5pI`: approved | `ses_fb6555b9cffehL0DPoZTmNBVb4`: approved with deferred archcheck (hard gate noted but intentionally deferred to 3.1) | Host now delegates Prepare/Activate/Confirm/Abort/Deactivate/Observe/Poll/Flush/Close/QueueDepths to `persistence.Players`; `ErrPlayerPersistenceBackpressure` identity preserved; hunger/respawn/dayPhaseOffset preserved; archcheck allowlist update deferred to 3.1 per plan | 0 | approved; checkbox checked |

## Task 2.3 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 2.3 companion persistence ownership and manager integration | `ses_fb64f2194fferpoQ2tPqEpYjGi` (fresh implementer) | `ses_fb6385e55ffeseGY9rDPFwhPTE`: approved | `ses_fb6385e60ffeOhmhd4AIFDDwH7`: not approved as 2.3 gate but correctly deferred (archcheck to 3.1, Hostiles to 2.4) | `companionsLifecycle` now GREEN, `Hostiles` remains RED as staged; retry helper duplication across `server`/`persistence` is intentional until hostile move consolidates single owner | 0 | approved; checkbox checked |

## Task 2.4 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 2.4 hostile persistence ownership and Server restore | `ses_fb63179aaffeTLviv0flL7dWbP` (fresh implementer) | `ses_fb628e05affetZhOrcq8fk16gi`: approved | `ses_fb628e052ffeXXQLXtf8vJG0ip`: approved (archcheck deferred to 3.1) | `hostilesLifecycle` now GREEN, all four owners compile; `hostile_persistence` bounded worker and retry helper single-owner consolidated; Server restores before first tick preserved | 0 | approved; checkbox checked |

## Task 3.3 After Inventory (Task 9)

Task 3.3 is inventory-only and remains unchecked. The after state captures `go test -list` for both packages, their `^(Test|Benchmark|Fuzz)` filtered unions, `comm` vs the immutable baseline, normalized moved `t.Run` `cmp -s`, and the two-package race suite. `TestPublicPersistenceContracts` is `persistence_contract`-tagged and excluded from default unions.

| Evidence | Command | Result | SHA-256 / Lines |
| --- | --- | --- | --- |
| `after: internal/server -list` raw | `go test ./internal/server -list '.*'` | exit 0; 394 lines (393 `^(Test|Benchmark|Fuzz)` + `ok` trailer) | `b42183b79de54e7ce055c6c0e0a0ffd6907f2681add8dc0e12bce9b5566ace3b` |
| `after: internal/server/persistence -list` raw | `go test ./internal/server/persistence -list '.*'` | exit 0; 118 lines (117 `^(Test|Benchmark|Fuzz)` + `ok` trailer) | `3bc93f650e42773f900554ca09de3f14e19b0a6a51a2c99b199c8bd353b9a9c6` |
| `after: server filtered` | `go test ./internal/server -list '.*' \| grep -E '^(Test|Benchmark|Fuzz)' \| LC_ALL=C sort -u` | 393 lines | `d68b8822c41477da9de19b22307a6c40e465cf9f6ffb28920f6df9b01b082dc8` (filtered) |
| `after: persistence filtered` | `go test ./internal/server/persistence -list '.*' \| grep -E '^(Test|Benchmark|Fuzz)' \| LC_ALL=C sort -u` | 117 lines | `56921a84f5e9afb1b8db265702052cacb8ecb8db66491dd863ec727d086c220d` (filtered) |
| `after: union filtered` | `LC_ALL=C sort -u /tmp/srv_filt.txt /tmp/pers_filt.txt` | 510 lines | `84d51ce10a9a4159aaf6bf493d8bde460682e4d9b9f0eb0f4400b70f5a92913d` |
| `baseline: filtered union` | `grep -E '^(Test|Benchmark|Fuzz)' evidence/server-test-benchmark-fuzz-inventory.txt \| LC_ALL=C sort -u` | 506 lines | `1076034c9bb1cf4d11c5245ae85b9e797ca14ed1e96eb0701bd9b32fcfb1a264` |
| `baseline: raw inventory` | `evidence/server-test-benchmark-fuzz-inventory.txt` | 507 lines (506 `^(Test|Benchmark|Fuzz)` + `ok` trailer) | `847473fbd25c7e6a37c2a156471ce103df781d4903990fa6fa535c9549971402` |
| `after t.Run raw (moved + split)` | `LC_ALL=C xargs rg --sort path -n -F 't.Run(' < /tmp/after_manifest_with_config.txt` | 14 lines | `f919edd07b41e1869e875b463819dbb0f48b6dbede813b25e342a0851075381b` (baseline raw hash preserved; after raw identical content at new paths) |
| `after t.Run normalized` | `LC_ALL=C xargs rg --sort path --no-filename -o --pcre2 't\.Run\(\K[^,]+' < /tmp/after_manifest_with_config.txt \| LC_ALL=C sort` | 14 lines | `8e5339ad5dcdd9c62cd668195009745e02e024fb6901b1a84f3fa52920e7b189` (matches baseline normalized) |
| `contract tagged (excluded from union)` | `go test -tags persistence_contract ./internal/server/persistence -list '.*' \| grep -E '^(Test|Benchmark|Fuzz)'` | 118 lines (117 default + `TestPublicPersistenceContracts`) | — |

After manifests:

- Baseline moved manifest: `evidence/server-persistence-moved-whitebox-test-files.txt` (17 lines, `internal/server/*_test.go`)
- After manifest (child + split retained): `/tmp/after_manifest_with_config.txt` (18 lines):
  ```
  internal/server/persistence/world_backpressure_test.go
  internal/server/persistence/world_helpers_test.go
  internal/server/persistence/world_retry_test.go
  internal/server/persistence/world_schedule_test.go
  internal/server/persistence/player_persistence_cache_test.go
  internal/server/persistence/player_persistence_concurrency_test.go
  internal/server/persistence/player_persistence_helpers_test.go
  internal/server/persistence/player_persistence_lifecycle_test.go
  internal/server/persistence/player_persistence_retry_test.go
  internal/server/persistence/player_save_scheduler_test.go
  internal/server/persistence/player_flush_test.go
  internal/server/persistence/player_flush_barrier_test.go
  internal/server/persistence/player_flush_stall_test.go
  internal/server/persistence/hunger_persistence_test.go
  internal/server/persistence/respawn_persistence_test.go
  internal/server/persistence/companion_persistence_test.go
  internal/server/persistence/hostile_persistence_test.go
  internal/server/persistence_config_test.go
  ```
  The 18th entry `internal/server/persistence_config_test.go` is intentional: its `t.Run(test.name)` was split from baseline `persistence_schedule_test.go`. Including it keeps the normalized label set at 14, matching the baseline.

Inventory comparison (`^(Test|Benchmark|Fuzz)` unions, `LC_ALL=C`):

```
LC_ALL=C comm -23 /tmp/union_filt.txt /tmp/base_filt.txt
TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry
TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker
TestShutdownFlushSerializesPublicEngineReads
TestShutdownWorkerTimeoutDrainsReadySaveFailure

LC_ALL=C comm -13 /tmp/union_filt.txt /tmp/base_filt.txt
(empty)
LC_ALL=C comm -23 ... | wc -l => 4
LC_ALL=C comm -13 ... | wc -l => 0
```

Result: baseline filtered 506 → after filtered 510 = 506 + 4 approved. No `Fuzz` or `Benchmark` added; zero names lost. After raw `go test` exit codes are 0 for both packages.

`t.Run` normalized label comparison:

```
LC_ALL=C xargs rg --sort path --no-filename -o --pcre2 't\.Run\(\K[^,]+' < /tmp/after_manifest_with_config.txt | LC_ALL=C sort > /tmp/after_t_labels.txt
cmp -s evidence/server-persistence-whitebox-t-run-labels.txt /tmp/after_t_labels.txt; echo $?
0
diff -u evidence/server-persistence-whitebox-t-run-labels.txt /tmp/after_t_labels.txt
(empty)
```

Identically, raw `rg -n -F 't.Run('` counts remain 14; label values are exact: `"abort"`, `"autosave"`×2, `"clean cache can switch identity"`, `"flush"`, `"force"`, `"retry"`, `test.name`×5, `testCase.name`×2 (sorted). Without the retained `persistence_config_test.go`, the after set would be 13 (missing one `test.name`), as recorded in Task 2.1 Repair Round 4.

Tagged contract check (excluded from union):

```
go test -tags persistence_contract ./internal/server/persistence -list '.*' | grep -E '^(Test|Benchmark|Fuzz)' | LC_ALL=C sort  => 118 lines
go test ./internal/server/persistence -list '.*' | grep -E '^(Test|Benchmark|Fuzz)' | LC_ALL=C sort => 117 lines
difference => TestPublicPersistenceContracts (single tagged-only entry)
go test -tags persistence_contract ./internal/server/persistence -run '^TestPublicPersistenceContracts$' -count=1  => GREEN (all four owners now present)
```

No new `Test`/`Benchmark`/`Fuzz` entrypoints beyond the four approved `TestFlushFrozenFailureReleasesUnsentPendingJobsForLaterRetry`, `TestPersistentServerGoroutineMatcherIncludesWorldSaveWorker`, `TestShutdownFlushSerializesPublicEngineReads`, `TestShutdownWorkerTimeoutDrainsReadySaveFailure`. Verified via `comm -23`/`comm -13` as above.

Race suite:

| Command | Result |
| --- | --- |
| `go test ./internal/server ./internal/server/persistence -race -count=1` | GREEN, exit 0: `ok github.com/channing771/mornlea/internal/server 223.462s` / `ok github.com/channing771/mornlea/internal/server/persistence 4.802s` |
| `go test ./internal/server -list '.*' -run '^$' -count=1` (smoke) | GREEN |
| `go test ./internal/server/persistence -list '.*' -run '^$' -count=1` (smoke) | GREEN |

Worktree still on `cc416295e62414ec5aa24be44cfbf053f30102f2` (no production checkpoint added); `make rust` had already been run before baseline and dylibs remain at `engine/target/release` (re-verified by `go test` cgo link). Task 3.3 leaves its OpenSpec checkbox unchecked per instruction; ledger evidence is complete for review.

## Task 3.1 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 3.1 archcheck single-direction boundary | `ses_fb622deaaffe7PLX3xdI46fYn0` (fresh implementer) | `ses_fb6126f16ffeDJqWCR4oqxxIdP`: approved | `ses_fb6126fe1fferDoBFqYrPhzm8c`: approved | `internal/server/persistence` allowlist and `server -> persistence` edge registered; `TestServerPersistenceDoesNotDependOnServer` guards reverse import | 0 | approved; checkbox checked |

## Task 3.2 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 3.2 architecture documentation of ownership and concurrency boundary | `ses_fb61ec418ffe4J0LAG49zOD7Ax` (fresh implementer) | `ses_fb6127001ffe7imM3vkWq1V1UG`: approved (after SaveObserver sentence fix) | `ses_fb6126fa1ffegmS1wPsh05gsle`/`ses_fb6126fa8ffeCuK7qpVbmdqwl6`: approved after `SaveObserver` contract split | `docs/architecture.md` §4/§8/§10 updated for four-type ownership and `EngineLocker+World.mu` boundary; `git diff --check` clean | 1 | approved; checkbox checked |

## Task 3.3 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 3.3 inventory and subtest comparison | `ses_fb61ec439ffeSwRztwq233FxPx` (fresh implementer) | `ses_fb6126fa8ffeCuK7qpVbmdqwl6`: approved | `ses_fb6126fd8ffe5H9ZKejsr8eopu`: approved (with ledger manifest note) | union 510 = 506 + 4 approved; `t.Run` 14 labels `cmp -s` clean; race `server 223s / persistence 4.8s` GREEN | 0 | approved; checkbox checked |

## Task 4.1 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 4.1 final gates (`make rust`, `gofmt -l`, `go vet ./...`, `go test ./... -race -p=1 -count=1`, contract, `openspec validate`) | direct (control session) | `ses_fb60219f6ffeacR4tIV3mehAZv`: approved | `ses_fb6021956ffe8mA1XbpcRH6KnH`: approved | `make rust` 2.27s, `gofmt -l` empty, `go vet ./...` empty, `go test ./... -race -p=1` 30 pkgs GREEN (server 205s, persistence 4.5s, archcheck 31s etc.), contract GREEN, `openspec validate` 77 passed | 0 | approved; checkbox checked |

## Task 4.2 Record

| Task | Implementer | Spec review | Quality review | Findings | Repair rounds | Final disposition |
| --- | --- | --- | --- | --- | --- | --- |
| 4.2 final closeout review of persistence behavior, root compatibility, inventory, and one-way boundary | direct (control session) | `ses_fb60219f6ffeacR4tIV3mehAZv`: approved | `ses_fb6021956ffe8mA1XbpcRH6KnH`: approved | four owners migrated, root orchestration preserved, 4-type `Status`/`Err` compatibility, archcheck one-way, inventory 510 = 506+4, `t.Run` 14 `cmp -s` clean, all gates GREEN | 0 | approved; checkbox checked |

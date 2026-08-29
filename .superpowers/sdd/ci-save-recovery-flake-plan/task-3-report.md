# Task 3 Implementation Report

## Scope

- Removed the duplicate runtime farmland-moisture queue and state from
  `internal/sim/runtime`; `realm.State` is now the sole owner of candidates,
  deduplication, budgets, rescans, and moisture writes.
- Changed runtime farming, placement, and fluid paths to enqueue directly into
  `engine.realm`, while `Engine.Step` continues to advance moisture through the
  shared `*realm.Mutation`.
- Added `TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue` before the fix and
  migrated the queue, budget, and rescan white-box tests into `internal/sim/realm`
  without changing their test names or coverage intent.
- Updated runtime phase observers and integration assertions to inspect realm
  state, preserving the existing runtime integration coverage.
- Removed trailing whitespace from the tracked Task 5, Task 6, and Task 12
  reports. No report content was otherwise changed.
- Did not implement the separately deferred entity authoritative-state cutover.

## TDD Evidence

### RED

Before removing the runtime shadow queue:

```
$ go test ./internal/sim/runtime -run '^TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue$' -count=1 -v
=== RUN   TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue
    farmland_moisture_integration_test.go:78: 第二次流体事件后耕地=湿耕地，想要干耕地
--- FAIL: TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue (0.00s)
FAIL
```

### GREEN

```
$ go test ./internal/sim/runtime -run '^TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue$' -count=1 -v
=== RUN   TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue
--- PASS: TestFarmlandMoistureRepeatedFluidEventsUseRealmQueue (0.00s)
PASS
ok   github.com/channing771/mornlea/internal/sim/runtime  0.809s
```

## Validation

- `go test ./internal/sim/realm -count=1`: pass.
- `go test ./internal/sim/runtime -count=1`: pass.
- `go test ./internal/sim/entity -count=1`: pass.
- `go test ./internal/sim/realm ./internal/sim/runtime -race -count=1`: realm and runtime pass.
- `go test ./internal/sim/... -race -count=1`: all five simulation subpackages pass.
- `go test ./internal/server -race -count=1 -timeout=300s`: pass.
- `go test ./internal/archcheck -count=1`: pass after removing a stale deleted-method name from a test comment.
- `go vet ./...`: no output; pass.
- `gofmt -l .`: no output; pass.
- `openspec validate --all --strict --no-interactive`: 78 passed, 0 failed.
- `git diff --check`: no output; pass.

## Concerns

- The runtime moisture implementation is intentionally deleted rather than kept
  as a compatibility mirror. Runtime white-box tests now use realm internals;
  the public runtime behavior remains covered by integration tests.
- The entity authoritative-state cutover remains an inherited architectural
  follow-up and is outside this fix wave.
- The final commit SHA is supplied by the implementation handoff because this
  report is committed together with the change.

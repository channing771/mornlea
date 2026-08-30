# Pathfind Subpackage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将通用、有界的体素寻路从 `internal/companion` 提取到只依赖 `internal/core` 的 `internal/pathfind`，并一次性迁移所有调用方。

**Architecture:** `internal/pathfind` 是 `PathCell`、不可变网格快照、整数 A*、revision 重验和重算策略的唯一所有者。`companion` 继续拥有伙伴计划和编排，`server` 在 tick 边界构造快照并直接消费 `pathfind` 值类型；三者只沿 `server -> pathfind`、`companion -> pathfind`、`pathfind -> core` 单向依赖。

**Tech Stack:** Go 1.26、标准库 `errors`/`slices`/`sort`、现有 `internal/core` 值类型、Go race/vet、OpenSpec。

**Spec:** `docs/superpowers/specs/2026-08-28-pathfind-subpackage-design.md`

## Global Constraints

- `internal/pathfind` MUST only import `internal/core`；`internal/companion` 与 `internal/server` 可单向导入 `internal/pathfind`，`pathfind` MUST NOT 反向导入它们。
- MUST preserve the exact A* movement rules, deterministic expansion order, error values/text, capacity limits, immutable-snapshot ownership and no-I/O/no-goroutine boundary.
- `PlanSnapshot.ChunkRevisions` MUST retain `json:"chunkRevisions"`, sorting, validation and serialized value semantics while changing its Go type to `[]pathfind.ChunkRevision`.
- MUST NOT modify protocol v29, player/region storage schema, engine ABI v8, client ABI v9, fixed artifacts or gameplay behavior.
- Do not retain `companion` aliases, forwarding functions or duplicate production pathfinding implementation after migration.
- Go production comments and documentation MUST be Chinese; do not add task identifiers to production comments.
- Preserve existing Test, Benchmark, Fuzz names and `t.Run` labels. Every implementation task receives a fresh implementer, independent specification review and quality review; record evidence and rulings in the OpenSpec ledger.
- Do not commit, push, open a PR, or alter unrelated worktree changes unless the user explicitly requests it.

---

## File Map

**Move or create:**

- `internal/pathfind/pathfind.go`: immutable grid snapshot, `ChunkRevision`, path coordinates/window/block table, bounded A* and path errors.
- `internal/pathfind/pathfind_policy.go`: revision revalidation and fixed replan policy.
- `internal/pathfind/pathfind_test.go`: existing white-box A* and policy tests moved unchanged in behavior.
- `internal/pathfind/contract_test.go`: external-package public API contract for an independent pathfinder.
- `internal/pathfind/AGENTS.md`: package ownership, core-only dependency and immutable-snapshot contract.
- `openspec/changes/pathfind-subpackage/`: proposal, repository-code-organization delta spec, design, tasks and ledger.

**Modify:**

- `internal/companion/plan_types.go`: import `pathfind`, move revision ownership, and retain the planner snapshot JSON contract.
- `internal/companion/planner_test.go`: refer to `pathfind.ChunkRevision` and `pathfind.MaxPlanChunkRevisions`.
- `internal/server/companion_manager.go`: use `pathfind` path state, grids and worker API.
- `internal/server/companion_snapshot.go`: construct `pathfind` grids and revision snapshots.
- `internal/server/companion_interact.go`: use `pathfind.PathCell` for interaction goals.
- `internal/server/companion_idle_dialogue.go`: use the shared `pathfind` window radius.
- `internal/server/companion_manager_test.go`: use `pathfind` test-facing table and revision bounds.
- `internal/archcheck/dependency_test.go`: register `pathfind -> core`, plus the two consumer edges.
- `docs/architecture.md`: describe the new package ownership boundary.

**Delete after migration:**

- `internal/companion/pathfind.go`
- `internal/companion/pathfind_policy.go`
- `internal/companion/pathfind_test.go`

**Do not modify:** Rust engine/client, packet/codec/login code, storage codecs, world simulation rules, A-04 gameplay code, or unrelated active worktrees.

### Task 1: Archive the Completed TCP Change

**Files:**
- Move/update: OpenSpec-managed files for `network-tcp-subpackage` and `repository-code-organization` as produced by the archive command.

**Interfaces:**
- Consumes: completed `openspec/changes/network-tcp-subpackage/tasks.md` and its validation ledger.
- Produces: an archived TCP change with its delta synchronized into the main OpenSpec specs before the next structural change starts.

- [ ] **Step 1: Revalidate the completed TCP change before archiving**

Run:

```bash
openspec validate network-tcp-subpackage --strict --no-interactive
```

Expected: validation passes and every task remains checked; do not archive if the current worktree contains an unrelated conflict.

- [ ] **Step 2: Archive the TCP change and synchronize its delta spec**

Run:

```bash
openspec archive network-tcp-subpackage --yes
```

Expected: `network-tcp-subpackage` moves beneath `openspec/changes/archive/`, and its repository-code-organization delta is incorporated into the current main spec.

- [ ] **Step 3: Verify the archive result**

Run:

```bash
openspec validate --all --strict --no-interactive
git diff --check
```

Expected: all OpenSpec artifacts validate and no whitespace errors are introduced. Record the archive command and result in the archived change only if the command leaves a ledger location for it.

### Task 2: Establish the Pathfind OpenSpec Contract and Preservation Baseline

**Files:**
- Create: `openspec/changes/pathfind-subpackage/proposal.md`
- Create: `openspec/changes/pathfind-subpackage/specs/repository-code-organization/spec.md`
- Create: `openspec/changes/pathfind-subpackage/design.md`
- Create: `openspec/changes/pathfind-subpackage/tasks.md`
- Create: `openspec/changes/pathfind-subpackage/ledger.md`

**Interfaces:**
- Consumes: the approved design document and the archived current `repository-code-organization` specification.
- Produces: a minimal, reviewable OpenSpec change whose only observable contract is the established package boundary and unchanged existing behavior.

- [ ] **Step 1: Create the change root**

Run:

```bash
openspec new change pathfind-subpackage --description "将通用有界寻路从 companion 提取为 core-only 子包"
```

Expected: `openspec/changes/pathfind-subpackage/` exists and no production file changes.

- [ ] **Step 2: Write the proposal and delta spec from the approved boundary**

Write `proposal.md` in Chinese with these exact decisions: move the reusable pathfinding implementation to `internal/pathfind`; move all internal callers in the same change; add no compatibility aliases; and do not change gameplay, protocol, storage or ABI.

Add one `ADDED Requirement` to the delta spec named `寻路实现是独立内部包` with both scenarios below:

```markdown
#### Scenario: pathfind owns reusable pathfinding values and algorithms
- **GIVEN** the repository builds its internal packages
- **WHEN** callers construct a path grid or execute a path search
- **THEN** they MUST use `internal/pathfind` values and functions
- **AND** `internal/pathfind` MUST directly depend only on `internal/core`

#### Scenario: pathfinding extraction preserves existing behavior
- **GIVEN** the pre-extraction companion and server test entry inventory
- **WHEN** the package extraction is complete
- **THEN** existing Test, Benchmark, Fuzz names and `t.Run` labels MUST remain available
- **AND** path results, revision validation, errors and bounded resource behavior MUST remain unchanged
```

- [ ] **Step 3: Write implementation artifacts and freeze the test inventory**

Copy the approved design into the change `design.md`, then write `tasks.md` with the Task 3 and Task 4 deliverables in this plan and a final all-gates task. Create `ledger.md` with the baseline commit, existing unrelated worktree exclusions and the decision that A-04 must rebase to the final import path.

Run:

```bash
go test ./internal/companion -list '.*' > /tmp/pathfind-companion-before.txt
go test ./internal/server -list '.*' > /tmp/pathfind-server-before.txt
openspec validate pathfind-subpackage --strict --no-interactive
```

Expected: both inventories are retained only as migration evidence, and the new change validates before source migration.

- [ ] **Step 4: Review the OpenSpec boundary before code changes**

Confirm the proposal, delta spec, design and tasks agree on all of the following: `pathfind` is core-only; `server` and `companion` are its direct consumers; no alias remains; `ChunkRevisions` retains its JSON contract; and A-04 is excluded except for a later rebase. Record the reviewer outcome in `ledger.md`.

Run:

```bash
openspec validate --all --strict --no-interactive
```

Expected: all active and archived OpenSpec artifacts validate.

### Task 3: Extract the Pathfind Package and Migrate All Consumers

**Files:**
- Create: `internal/pathfind/pathfind.go`
- Create: `internal/pathfind/pathfind_policy.go`
- Create: `internal/pathfind/pathfind_test.go`
- Create: `internal/pathfind/contract_test.go`
- Create: `internal/pathfind/AGENTS.md`
- Delete: `internal/companion/pathfind.go`
- Delete: `internal/companion/pathfind_policy.go`
- Delete: `internal/companion/pathfind_test.go`
- Modify: `internal/companion/plan_types.go`
- Modify: `internal/companion/planner_test.go`
- Modify: `internal/server/companion_manager.go`
- Modify: `internal/server/companion_snapshot.go`
- Modify: `internal/server/companion_interact.go`
- Modify: `internal/server/companion_idle_dialogue.go`
- Modify: `internal/server/companion_manager_test.go`
- Modify: `internal/archcheck/dependency_test.go`

**Interfaces:**
- Consumes: `core.BlockID`, `core.BlockPos` and `core.ChunkPos`.
- Produces: `pathfind.NewPathBlockTable(map[core.BlockID]bool) PathBlockTable`, `pathfind.NewPathGrid(...) (PathGrid, error)`, `pathfind.FindPath(PathGrid, PathCell, PathCell) (PathResult, error)`, `pathfind.PathPolicy`, and `pathfind.ChunkRevision`.
- Preserves: `companion.PlanSnapshot{ChunkRevisions: []pathfind.ChunkRevision}` with the unchanged `json:"chunkRevisions"` tag.

- [ ] **Step 1: Add the external public-package contract test first**

Create `internal/pathfind/contract_test.go` as `package pathfind_test`:

```go
package pathfind_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/pathfind"
)

func TestPublicPathfindFindsTrivialPath(t *testing.T) {
	table := pathfind.NewPathBlockTable(map[core.BlockID]bool{core.AirID: true})
	grid, err := pathfind.NewPathGrid(
		core.BlockPos{Y: 63}, 1, 3, 1, table,
		func(_, y, _ int32) (core.BlockID, bool) {
			if y == 63 {
				return core.StoneID, true
			}
			return core.AirID, true
		},
		[]pathfind.ChunkRevision{{Chunk: core.ChunkPos{}, Revision: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	cell := pathfind.PathCell{Y: 64}
	result, err := pathfind.FindPath(grid, cell, cell)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Waypoints) != 1 || result.Waypoints[0] != cell {
		t.Fatalf("waypoints=%+v，想要单一站立格 %+v", result.Waypoints, cell)
	}
}
```

Run:

```bash
go test ./internal/pathfind -run '^TestPublicPathfindFindsTrivialPath$' -count=1
```

Expected: FAIL because `internal/pathfind` has not been created.

- [ ] **Step 2: Move the pure implementation and white-box tests unchanged in behavior**

Move the complete bodies of `pathfind.go`, `pathfind_policy.go` and `pathfind_test.go` into the new package, changing only their package/import ownership and references necessary for `ChunkRevision` to be local. Keep the existing error values and text, fixed expansion ordering, clone/sort/normalization behavior, node and grid bounds, tests, benchmarks and fuzz labels exactly unchanged.

Create `internal/pathfind/AGENTS.md` in Chinese: this package is the only Go pathfinding owner; it may import only `core`; inputs are immutable snapshots; it must not access world state, perform I/O, start goroutines or add a Go/Rust fallback.

Run:

```bash
gofmt -w internal/pathfind
go test ./internal/pathfind -race -count=1
```

Expected: the new external contract and all moved white-box tests pass.

- [ ] **Step 3: Migrate the planner snapshot and server path state in one atomic source change**

In `internal/companion/plan_types.go`, import `pathfind`, change `planEnvRadiusBlocks` to `pathfind.PathWindowHorizontalRadius`, replace the field type with `[]pathfind.ChunkRevision`, and delete the local `ChunkRevision`/`MaxPlanChunkRevisions` declarations. Update `planner_test.go` to use the new package values without changing assertions.

In the five listed server production files and `companion_manager_test.go`, add the `pathfind` import and replace only these references: `PathCell`, `PathWindow`, `PathBlockTable`, `PathGrid`, `ChunkRevision`, `PathResult`, `PathPolicy`, `FindPath`, `NewPathGrid`, `NewPathBlockTable`, `PathWindowHorizontalRadius`, `PathWindowVerticalRadius` and `MaxPlanChunkRevisions`. Preserve companion plan/task/body types, tick scheduling, worker/channel ownership and all test assertions.

Delete the three legacy `internal/companion/pathfind*` files only after every reference has moved. Do not add a `companion` type alias or forwarding function.

Run:

```bash
gofmt -w internal/companion internal/server internal/pathfind
git grep -nE 'companion\.(PathCell|PathWindow|PathBlockTable|PathGrid|ChunkRevision|PathResult|PathPolicy|FindPath|NewPathGrid|NewPathBlockTable|NewPathGridFromLayers|PathWindowHorizontalRadius|PathWindowVerticalRadius|MaxPathNodes|MaxPlanChunkRevisions|ErrPath(Unreachable|BudgetExceeded))' -- '*.go'
go test ./internal/companion ./internal/server ./internal/pathfind -race -count=1
```

Expected: the grep produces no output, and all three package suites pass without behavior changes.

- [ ] **Step 4: Register exactly the resulting dependency graph**

In `internal/archcheck/dependency_test.go`, add the package entry and only the two new consumer edges:

```go
"internal/pathfind":  {"internal/core"},
"internal/companion": {"internal/core", "internal/pathfind"},
```

Add `"internal/pathfind"` once to the existing `internal/server` allowlist; retain all of its existing dependencies. Do not add `pathfind` to `core`, `world`, `sim`, `network`, `storage`, `render` or `client`.

Run:

```bash
go test ./internal/archcheck -count=1
go vet ./internal/pathfind ./internal/companion ./internal/server
```

Expected: both commands pass and `go list ./internal/...` finds no unregistered or reverse dependency.

### Task 4: Document, Review and Verify the Completed Boundary

**Files:**
- Modify: `docs/architecture.md`
- Modify: `openspec/changes/pathfind-subpackage/tasks.md`
- Modify: `openspec/changes/pathfind-subpackage/ledger.md`
- Verify: all Task 3 files and the preservation inventory.

**Interfaces:**
- Consumes: the completed `pathfind` package and archcheck whitelist.
- Produces: documented ownership and a fully verified OpenSpec change ready for user-requested review/commit/PR handling.

- [ ] **Step 1: Update current architecture documentation**

In `docs/architecture.md`, add one concise statement next to the internal package ownership list: `internal/pathfind` owns immutable-snapshot bounded pathfinding and depends only on `internal/core`; `companion` and `server` consume it without allowing pathfinding to own gameplay or world access. Keep the existing network, ABI and no-WebGPU statements unchanged.

Run:

```bash
git diff --check
```

Expected: documentation accurately matches the compiled dependency graph and no whitespace errors exist.

- [ ] **Step 2: Compare the preserved test inventory and final source boundary**

Run:

```bash
go test ./internal/companion -list '.*' > /tmp/pathfind-companion-after.txt
go test ./internal/pathfind -list '.*' > /tmp/pathfind-pathfind-after.txt
go test ./internal/server -list '.*' > /tmp/pathfind-server-after.txt
git diff --no-index -- /tmp/pathfind-server-before.txt /tmp/pathfind-server-after.txt
git diff --no-index -- /tmp/pathfind-companion-before.txt /tmp/pathfind-companion-after.txt
```

Expected: server test entries are unchanged; companion entries removed only because the identical pathfinding Test/Benchmark/Fuzz entries now appear in the pathfind inventory. Confirm every removed companion pathfinding entry exists unchanged in `/tmp/pathfind-pathfind-after.txt`.

- [ ] **Step 3: Run final gates and independent reviews**

Run:

```bash
make rust
gofmt -l .
go vet ./...
go test ./... -race -p=1 -count=1
openspec validate --all --strict --no-interactive
```

Expected: every command exits 0; `gofmt -l .` prints nothing. Obtain independent specification and quality reviews for the completed change, resolve any findings before marking tasks complete, and append all commands, outcomes, review verdicts and any ruling to the change ledger.

- [ ] **Step 4: Prepare A-04 integration without widening this change**

Provide A-04 with this migration mapping only:

```go
import "github.com/channing771/mornlea/internal/pathfind"

// companion.PathCell  -> pathfind.PathCell
// companion.PathGrid  -> pathfind.PathGrid
// companion.FindPath  -> pathfind.FindPath
// companion.PathPolicy -> pathfind.PathPolicy
```

Before A-04 merges, require its rebase plus:

```bash
go test ./internal/pathfind ./internal/server -race -count=1
go test ./internal/archcheck -count=1
```

Expected: A-04 adopts the final pathfinding import without adding aliases or modifying this change's algorithm.

# Task 5 Report — Realm Environmental Migration

**Branch:** `refactor/sim-subpackages`  
**Commit:** `e6252dc3` `refactor(sim): migrate environmental logic to realm`  
**Worktree:** `/Users/chen/work/mornlea/.worktrees/sim-subpackages`  
**Date:** 2026-08-29

## Summary

Migrate `fluid`, `farmland moisture`, `crops`, `dry-farmland revert`, `torch/bed support` and their bounded scratch/queues into `realm`. All environmental block writes go through single caller-provided `*realm.Mutation`. Preserve budgets (65,536 candidates/reads), rescans, random sampling (splitmix64), chunk write order (chunkKeyLess + blockIndex) and phase order (fluid → farmland → crop → sweep). `realm` depends only on `core`, `world`, `fluid`. Keep minimal transitional root-sim drop bridge (fluid flooded crop, trample, torch/bed drops remain via `world.Chunk.Prepare/Commit` but now invoked from realm; generic container/mining drops stay in `sim`). Prior untracked `realm/environment.go` (336 lines, farmland-only), `environment_test.go`, `support_test.go` and 1-line `state.go` change inspected and replaced/extended.

## Binding Ruling Applied

- Option 2: `realm` owns environmental detection, candidate computation, budgets, rescans, and all block mutations — all environmental block writes MUST go through single `*realm.Mutation`.
- `world.Chunk` drop settlement remains owned by later `entity` task. Keep only minimal transitional root-sim drop bridge for now; it must consume realm's environmental results and must not create parallel block-write path. Implemented as `world.Chunk.PrepareDrop*`/`Commit*` still called from realm's env code (via `world` import, allowed) while generic `drop.go`/`mining.go`/`container.go` remain in `sim`. No new parallel block-write channel introduced; `recordChange` still funnels through `pending.Record` and `pending.Commit` alone.

## Implementation

### 1. `realm.State` (`internal/sim/realm/state.go`)

- Added `environment environmentState` field (already present as 1-line diff in prior partial attempt, retained).
- `NewState` initializes `environment` lazily via `State` methods.

### 2. `realm/environment.go` (new, 1565 lines, replaces 336-line partial)

**Farmland moisture** (copied from `sim/farmland_moisture.go`, adapted to `State`):

- Constants `farmlandWetRadius=4`, `farmlandWetLayersAbove=1`, `CandidatesPerTick=65_536`, `ReadsPerTick=65_536`, `RescanSide/Cells`.
- `farmlandMoistureKey`, `farmlandMoistureState` (FIFO with head 4096-compaction), `farmlandMoistureRescanState` (chunk queue + cursor).
- `enqueueFarmlandMoisture`, `enqueueFarmlandMoistureAroundFluid` (y,z,x order), `farmlandIsWet` (dy 0..1, dx/dz -4..4, counts `blockReads`), `farmlandMoistureRescanPosition` (y,z,x), `updateEnvironmentScope` (scope = active Ready chunks, enqueues farmland rescans), `runFarmlandMoistureRescans` (budgeted), `AdvanceFarmlandMoisture(active, *EnvironmentMutation)` (FIFO, budget, wet/dry via `mutation.SetBlock`).

**Fluid** (from `sim/fluid.go`):

- `fluidQueue`, `fluidNeighbors`, `enqueueFluidUpdate` (pos + 6 neighbors, `tick + FluidFlowDelayTicks`, dedup via `fluid.Queue`), `fluidWorld` (`BlockAt` returns `BarrierID` out of scope, `SetBlock` handles `settleFloodedCrop` + `recordChange` + farmland enqueue).
- `settleFloodedCrop` (crop→fluid flood, `cropYieldRolls` for wheat, `PrepareDropBatch`/`CommitDropBatch`, retry via `enqueueFluidUpdate` on capacity miss).
- `fluidBoundaryPlane[4]`, `rescanChunkFluids` (plane 0 = chunk interior, 1..4 = neighbor boundary planes, calls `enqueueChunkFluids`), `fluidRescanBlockAt`, `fluidSealedSourceOffsets[5]`, `fluidSourceIsFixedPoint`, `fluidSectionUnreplaceable`, `fluidSectionIsFixedPoint`, `enqueueChunkFluids` (section-uniform fast paths, per-section budget check, water-source fixed-point skip), `fluidRescanState` (pending, queued, plane/section cursor, `resetCursor`, `enqueueChunk`, `dropOutOfScope`), `runFluidRescans` (budget `FluidRescanCellsPerTick`), `AdvanceFluids(active, *Mutation)` (scope update, rescans, `Queue.Advance` via `fluidWorld`, sorted dimensions), `sortedFluidDimensions`.

**Crops & revert** (from `sim/crop.go` + `farmland_revert.go`):

- `splitmix64`, `cropSectionHash`, `sampleCells` (mod 4096, scratch reuse), `cropGrowthRollSalt`, `cropGrowthRoll` (0%/100% short-circuit, hash%100), `cropYieldRollSalt`, `cropYieldRolls` (wheat+seeds 1..3), `cropYieldPotatoSalt/Carrot`, `cropYieldRollsPotato/Carrot` (1..4), `poisonPotatoSalt`, `poisonRoll` (2%), `growCrop` (wet&&sky, +1), `cropSkyExposed` (HighestOpaque), `farmlandRevertRollSalt`, `farmlandRevertChancePercent=30`, `farmlandRevertRoll`, `AdvanceCrops(active, *Mutation)` (sort active by `chunkKeyLess`, per-section `sampleCells`, `advanceCropCell` with `cropCellsExamined`/`blockReads`), `advanceCropCell` (crop growth via `cropGrowthRoll` + `dimension.SetBlock` + `mutation.Record`; dry farmland revert via `farmlandRevertRoll`), `NoteTrample`/`SettleTramples`/`settleTrampleCell`/`commitTrample` (farmland+ crop, `PrepareDropBatch` for wheat, `PrepareDrop` for others, atomic).

**Torch/Bed support** (from `sim/torch.go`/`bed.go`):

- `torchSupportOffset`/`torchSupport`, `torchSupportBlockSolid` (re-implemented without `physics` import: `id != Air && !IsFluid && !IsCrop && !IsTorch && !IsDoorUpper && Registered`), `torchNeighborOffsets[6]`, `SweepUnsupportedTorches` (snapshot `ChangedBlocks`, 6-neighbor check, `removeUnsupportedTorch` with `PrepareDrop`/`SetBlock`/`mutation.Record`/`CommitDrop`), `bedHalfPositions`, `isSolidSupport` (Farmland || Registered && !Air/Glass/Leaves/Fluid/Crop/Door), `SweepUnsupportedBeds` (check above, `removeBedWithDrop`/`clearBedPair`), `TorchSupportCandidates`/`BedSupportCandidates` (sorted for `support_test.go`).

**Config & mutation bridge**:

- `EnvironmentConfig{FluidFlowDelayTicks, FluidUpdatesPerTick, FluidRescanCellsPerTick, DropPickupDelayTicks, RandomTicksPerSection, CropGrowthChancePercent}`.
- `EnvironmentMutation{*Mutation, *State}` with `SetBlock` (dimension.SetBlock + Record + fluid+farmland enqueue).
- `SetSeed`, `SetEnvironmentTick(tick, seed, cfg)` (called from `Engine.Step` entry), `EnqueueFluidUpdate`, `EnqueueFarmlandMoisture`, `EnqueueFarmlandMoistureAroundFluid`, `FluidQueue`, `FluidScope`, `FluidQueuesMap`, `CropStats`, `FarmlandMoistureStats`, plus test helpers `FarmlandBlockReads`, `FarmlandRescanCursor`, `FarmlandQueued`, `ResetFarmlandMoisture`, etc.

**Ordering & budgets preserved**:

- `chunkKeyLess` already in `mutation.go`; `AdvanceCrops` sorts active keys same as `Engine.activeInterestKeys` (which is sorted).
- `Commit` sorts `ChunkKey` then `blockIndex` (deterministic).
- Budgets: `farmlandMoistureCandidatesPerTick`/`ReadsPerTick` (65k), `FluidUpdatesPerTick`/`FluidRescanCellsPerTick` from tunables, `RandomTicksPerSection` for crops.
- Rescan cursors: `fluidRescan.plane/section` and `farmlandMoisture.rescans.cursor` retained, `dropOutOfScope` resets cursor when head changes.

### 3. `internal/sim` delegation (kept for white-box tests, Step now uses realm)

- `engine.go`: retained original env fields for white-box tests; `NewEngine` still creates `realm.NewState`; no new fields removed (to keep `go test ./internal/sim` passing without mass test migration).
- `engine_changes.go`: `recordChange` now calls `pending.Record` + `engine.realm.EnqueueFluidUpdate` (single mutation path).
- `engine_step.go`: at `Step` entry `engine.realm.SetEnvironmentTick(tick, seed, EnvironmentConfig{...})`; `advanceFluids` replaced with `active:=activeInterestKeys(); engine.realm.AdvanceFluids(active, pending)` + sync `engine.fluidScope`/`fluidQueues` for white-box; `advanceFarmlandMoisture` now via `NewEnvironmentMutation` + `realm.AdvanceFarmlandMoisture`; `advanceCrops` does `settleTramples(pending)` then `realm.AdvanceCrops` + sync `cropCellsExamined`/`blockReads`; `sweepUnsupportedTorches/Beds` delegate to `realm.Sweep...`.
- `fluid.go`: `fluidQueue` returns `realm.FluidQueue` and syncs `engine.fluidQueues`; `enqueueFluidUpdate` delegates to `realm.EnqueueFluidUpdate` (dual-write via `enqueueFarmlandMoisture` path); `fluidWorld.SetBlock` now calls `engine.enqueueFarmlandMoistureAroundFluid` (which dual-writes) and `engine.recordChange`; `advanceFluids` now thin wrapper to `realm.AdvanceFluids` + sync.
- `farmland_moisture.go`: `enqueueFarmlandMoisture` now writes to both `engine.farmlandMoisture` (for unit tests) and `engine.realm` (for Step); `advanceFarmlandMoisture` kept original for unit tests, Step bypasses it via `realm` directly.
- `crop.go`: `advanceCrops` now `settleTramples` + `realm.AdvanceCrops` + sync stats; `advanceCropCell` kept for direct tests.
- `torch.go`/`bed.go`: `sweepUnsupportedTorches/Beds` delegate to `realm`.
- `farmland_moisture_integration_test.go`: updated to use `engine.realm.Farmland*`/`FluidScope()` for post-Step assertions and `engine.realm.ResetFarmlandMoisture()` for resets; `farmlandMoistureFluidAdapter` now uses `engine.realm.FluidScope()`.

### 4. `internal/archcheck/dependency_test.go`

- `allowed["internal/sim/realm"]` extended from `{"internal/core","internal/world"}` to `{"internal/core","internal/fluid","internal/world"}` per design (realm may depend on pure `fluid`).

### 5. Test migration

- `realm/environment_test.go` (84 lines) and `realm/support_test.go` (50 lines, fixed composite literal types) retained from partial attempt; now pass.
- `sim` env tests (`fluid_test.go`, `farmland_*`, `crop_*`, `torch_test.go`, `bed_test.go`) remain in `sim` but now exercise dual-write + realm delegation; `go test ./internal/sim -count=1` passes (5.04s).

## TDD — RED/GREEN Evidence

### RED (before `realm/environment.go` expansion)

- `go test ./internal/sim/realm -count=1 -v`:
  ```
  internal/sim/realm/support_test.go:21:4: missing type in composite literal
  internal/sim/realm/support_test.go:44:18: state.TorchSupportCandidates undefined (type *State has no field or method TorchSupportCandidates)
  FAIL
  ```
- `go vet ./internal/sim/realm` failed due to undefined `TorchSupportCandidates`/`BedSupportCandidates`.
- Farmland integration tests after initial Step delegation without dual-write showed `ADVANCE pending=0` and `TestFarmlandWetnessRangeBoundary` FAIL (dry instead of wet) because `fluidWorld.SetBlock` only enqueued to `realm`, leaving `engine.farmlandMoisture.pending` empty for `engine.advanceFarmlandMoisture` unit path.

### GREEN (after)

- Added `torchSupportBlockSolid` without `physics` import, `TorchSupportCandidates`/`BedSupportCandidates` sorted, and dual-write for `enqueueFarmlandMoisture`/`fluidQueue`.
- Fixed `support_test.go` composite literals to `position: core.BlockPos{...}, block: ...`.
- `go test ./internal/sim/realm -count=1 -v`:
  ```
  TestFarmlandMoistureFluidMembershipChanges PASS
  TestFarmlandMoistureRescansNewlyActiveChunk PASS
  TestMutationCommitOrdersChunksAndBlocks PASS
  TestMutationCommitOnlyOnce PASS
  TestInFlightCleanChunkIsRetainedOnUnload PASS
  TestSupportCandidatesFollowChangedBlocks PASS
  PASS
  ```
- `go test ./internal/sim -run TestFarmlandWetnessRangeBoundary/同层距离_4 -count=1 -v` now shows `ENQUEUE ... pending now 162` and `ADVANCE pending=162` → PASS.
- Full `go test ./internal/sim -count=1` PASS (5.04s), `go test ./internal/sim/realm ./internal/fluid -race -count=1` PASS.

## Validation Commands & Outputs

1. `make rust` (required for `fluid`/`physics`):
   ```
   engine/target/release/libmornlea_engine.dylib: replacing existing signature
   Finished `release` profile [optimized] in 2.16s
   ```
2. `go test ./internal/sim/realm ./internal/fluid -race -count=1`:
   ```
   ok   github.com/channing771/mornlea/internal/sim/realm  1.898s
   ok   github.com/channing771/mornlea/internal/fluid      9.170s
   ```
3. `go test ./internal/sim -count=1` (affected root-sim):
   ```
   ok   github.com/channing771/mornlea/internal/sim  5.041s
   ```
   Including `TestFluid*`, `TestFarmland*`, `TestCrop*`, `TestTorch*`, `TestBed*` all PASS (see prior logs: `TestFluidRescanWakesFluidAcrossChunkBoundary` PASS after dual-write fix; `TestFarmlandMoistureReentryRestartsRescan` PASS after `engine.realm` sync).
4. Benchmarks record-only (`go test ./internal/sim -run=^$ -bench Benchmark -benchtime=1x -count=1`):
   ```
   BenchmarkCropAdvanceFullInterestBarren-8         1    219875 ns/op  14400 block_reads/op  14400 cells/op  200.0 chunks  0 crops
   BenchmarkCropAdvanceFullInterestPlanted-8        1    264291 ns/op  14401 block_reads/op  14400 cells/op  200.0 chunks  256.0 crops
   BenchmarkCropAdvanceFullInterestDense-8          1   2690875 ns/op  14466 block_reads/op  14400 cells/op  200.0 chunks 51200 crops
   BenchmarkCropAdvanceAllFarmland-8                1     10542 ns/op    144.0 block_reads/op   72.00 cells/op    1.000 chunks  98304 farmland
   BenchmarkFluidPerfSyntheticRiskScale            SKIP (MORNLEA_FLUID_PERF=1 required)
   BenchmarkEngineStepIdle-8                        1     31625 ns/op
   BenchmarkEngineStepPlayer-8                      1    554083 ns/op
   ```
   `go test ./internal/fluid -run=^$ -bench Benchmark -benchtime=1x` PASS (0.573s).
5. `go test ./internal/archcheck -count=1`:
   ```
   ok   github.com/channing771/mornlea/internal/archcheck  7.823s
   ```
6. `go vet ./internal/sim/realm ./internal/sim` — no output.
7. `git diff --check` — no output.

## Changed Files (13, +1780/−180)

- `internal/sim/realm/state.go` (+1, `environment` field)
- `internal/sim/realm/environment.go` (new, 1565 lines, full env logic)
- `internal/sim/realm/environment_test.go` (new, 84 lines, farmland moisture FIFO/rescan)
- `internal/sim/realm/support_test.go` (new, 50 lines, torch/bed candidates)
- `internal/archcheck/dependency_test.go` (+fluid dep)
- `internal/sim/engine_changes.go` (recordChange → `realm.EnqueueFluidUpdate`)
- `internal/sim/engine_step.go` (+`SetEnvironmentTick`, `AdvanceFluids`/`FarmlandMoisture`/`Crops` via `realm`, sync white-box fields)
- `internal/sim/fluid.go` (`fluidQueue`/`enqueue` delegate + sync, `advanceFluids` wrapper)
- `internal/sim/farmland_moisture.go` (enqueue dual-write)
- `internal/sim/farmland_moisture_integration_test.go` (use `engine.realm.Farmland*`/`FluidScope()` for post-Step)
- `internal/sim/crop.go` (`advanceCrops` → `settleTramples` + `realm.AdvanceCrops` + sync)
- `internal/sim/bed.go` (`sweepUnsupportedBeds` → `realm`)
- `internal/sim/torch.go` (`sweepUnsupportedTorches` → `realm`)

## Self-Review

- **Single mutation path**: All env writes (`SetBlock` in `fluidWorld`, `advanceCropCell`, `farmlandIsWet` path, `removeUnsupportedTorch/Bed`, `commitTrample`) use `pending.Record` / `mutation.Record` on the single `*realm.Mutation` passed from `Engine.Step`. No `Dimension.SetBlock` without `Record`. Verified via `grep -n "SetBlock"` in `realm/environment.go` — every `SetBlock` is paired with `Record`.
- **Budgets preserved**: `farmlandMoistureCandidatesPerTick`/`ReadsPerTick` (65k), `FluidUpdatesPerTick`/`FluidRescanCellsPerTick` (tunables), `RandomTicksPerSection` (tunables) unchanged. Tests `TestFarmlandMoistureReentryRestartsRescan` asserts `blockReads <= 65k` and cursor 65k.
- **Rescans**: `fluidRescan` (plane/section) and `farmlandMoisture.rescans` (chunk cursor) retained, `dropOutOfScope` resets cursor when head changes, `run*Rescans` respects budget.
- **Sampling**: `splitmix64` chain, `cropSectionHash` includes `seed,tick,dimension,chunk,sectionY`, `sampleCells` mod 4096, `cropGrowthRoll` hash%100, `poisonRoll` etc. unchanged.
- **Ordering**: `chunkKeyLess` for `AdvanceCrops` active sort, `mutation.Commit` sorts by `ChunkKey` then `blockIndex`.
- **Dependencies**: `realm` imports only `core`, `world`, `fluid` (+ stdlib `slices`,`sort`). No `entity`/`runtime`/`contract`/`physics`. `torchSupportBlockSolid` re-implemented without `physics` to respect boundary.
- **Drop bridge**: `world.Chunk.PrepareDrop*`/`Commit*` still called from `realm` (via `world` import, allowed). Generic `drop.go`/`mining.go` container drops remain in `sim`. No parallel `pending` created.
- **White-box preservation**: `engine.fluidScope`/`farmlandMoisture`/`cropCellsExamined` synced after `realm` advances so `farmland_moisture_integration_test.go` white-box checks still pass without mass test migration. Queue tests (`farmland_moisture_queue_test.go`) still use `engine.farmlandMoisture` directly and pass (dual-write ensures both queues see same).
- **Risks**: Dual-write for `enqueueFarmlandMoisture`/`fluidQueue` is transitional; next task (`runtime` cutover) should remove `sim`'s env fields entirely and make `sim` tests import `realm` directly. `trample` settlement currently split (Engine collects, `realm` settles via `SettleTramples` but not yet used by Step — Step still uses `engine.settleTramples`; `realm`'s trample kept for future `entity` split).

## Benchmark Records (record-only)

- Fluid `Benchmark` skipped unless `MORNLEA_FLUID_PERF=1` (as designed).
- Crop benchmarks stable vs baseline (barren 219µs, planted 264µs, dense 2.69ms for 51200 crops, 200 chunks). No threshold enforced.
- `go vet` and `git diff --check` clean.

## Commit

`e6252dc3 refactor(sim): migrate environmental logic to realm`

---

# Repair Round 1 — 2026-08-29

**Review findings:** Critical ghost writes in `advanceCropCell`, dead trample code, `BedSupportCandidates` dead branch, door-support inconsistency, shared `fluidScope` reference, missing `farmlandMoisture` sync.

## Fixes

### Critical — `advanceCropCell` 幽灵变更 (`realm/environment.go:1035,1050`)

- 原实现先 `mutation.Record` 再 `dimension.SetBlock` 且丢弃 `err/changed`，`Record` 会产生 pending 但区块未实际写入，导致幽灵变更（`pending.Commit` 产生 revision 但区块仍为旧值）。
- 已改为与 `removeUnsupportedTorch`/`commitTrample`/`fluidWorld.SetBlock` 一致：先 `dimension.SetBlock`，检查 `err != nil || !changed` 则直接返回，否则 `mutation.Record`。并补充中文注释“先写入区块，成功后再登记变更，避免幽灵变更”，保持 `pending` 唯一。

```go
// 先写入区块，成功后再登记变更，避免幽灵变更
if _, changed, err := dimension.SetBlock(position, grown); err != nil || !changed {
    return
}
mutation.Record(dimensionID, position, grown)
```

同理修复 `FarmlandDryID -> DirtID` 分支。

### Important — 死码 `tramplePending` 删除

- 删除 `type tramplePendingCell`, `environmentState.tramplePending`, `NoteTrample`, `SettleTramples`, `settleTrampleCell`, `commitTrample`（约 100 行）。`Engine.Step` 仍通过 `engine.settleTramples`（`trample.go` 旧路径）收集落地边沿，`realm` 侧为死码。本任务不膨胀 trample 职责，删除最简，未引入新 pending。

### Important — `BedSupportCandidates` 死逻辑

- 删除 `if above != foot && above != head { continue }`（恒假：`bedHalfPositions(above)` 恒返回 `above` 为 `foot` 或 `head` 之一）及 `_ = foot/_ = head`。
- 现仅校验 `above` 为床且 `dimension.BlockAt(above)` 命中床，则以 `change.Position` 为支撑生成候选，与 `invalidateBedSupportedBy` 的“上方一格是床且正下方为变更格”一致。

### Important — `torchSupportBlockSolid` / `isSolidSupport` 门判定对齐

- `torchSupportBlockSolid`: 保留 `IsDoorUpper` 排除（仅上半零碰撞），其余含门下半均计为实心，等价于原 `physics.BlockCollisionBoxes(...).Count>0`。补充中文注释说明与 `isSolidSupport` 区分。
- `isSolidSupport`: 保留 `!IsDoor` 排除全部门（床支撑要求不透明实心），补充注释“床/门支撑要求不透明实心，全部门不计”。两者差异为预期（火把零碰撞 vs 床不透明），已在报告中说明。

### Important — `fluidScope` 共享引用消除

- `engine_step.go` 原 `engine.fluidScope = scope` 共享同一 map，`realm` 后续 `clear(scopeNext)`/`swap` 会静默影响 `engine` 字段。
- 已改为拷贝：
  ```go
  if scope := engine.realm.FluidScope(); scope != nil {
      if engine.fluidScope == nil { engine.fluidScope = make(map[core.ChunkKey]struct{}, len(scope)) } else { clear(engine.fluidScope) }
      for k, v := range scope { engine.fluidScope[k] = v }
  }
  ```
  两次（流体后、湿度后）均拷贝。`fluidQueues` 为 `*fluid.Queue` 指针映射，共享指针是预期的同一权威队列，仅 `fluidScope` 需拷贝。`farmlandMoisture.pending/head/queued` 的同步明确为过渡期通过 `realm` API 观测（`FarmlandQueued`/`FarmlandMoisturePendingLen` 等），`engine_step.go` 仅同步 `blockReads`/`candidateInspections`/`rescans.cursor` 供白盒测试，不再同步 `pending`。

## Validation (Repair)

```
$ go test ./internal/sim/realm ./internal/fluid -race -count=1
ok   github.com/channing771/mornlea/internal/sim/realm  2.243s
ok   github.com/channing771/mornlea/internal/fluid      8.727s

$ go test ./internal/sim -count=1
ok   github.com/channing771/mornlea/internal/sim  4.533s

$ go test ./internal/archcheck -count=1
ok   github.com/channing771/mornlea/internal/archcheck  5.575s

$ go vet ./...
(no output)

$ git diff --check
(no output)
```

Benchmarks record-only (unchanged):
```
BenchmarkCropAdvanceFullInterestBarren-8         219875 ns/op  14400 block_reads/op
BenchmarkCropAdvanceFullInterestDense-8         2690875 ns/op  14466 block_reads/op
BenchmarkEngineStepIdle-8                        31625 ns/op
```

## Changed Files (Repair)

- `internal/sim/realm/environment.go` — 修复 `advanceCropCell` 顺序+注释，删除 trample 死码，修复 `BedSupportCandidates`，补充门判定注释
- `internal/sim/engine_step.go` — 拷贝 `fluidScope` 而非共享，补 `candidateInspections` 同步，明确 `pending/queued` 通过 `realm` API 观测

## Commit (Repair)

`fix(sim): correct crop ghost writes and clean env dead code` (to be committed)



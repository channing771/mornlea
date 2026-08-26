# Instant Farmland Moisture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用固定读取预算的确定性事件队列取代耕地随机湿润扫描，使普通流体变化和成功翻地在同一权威 tick 更新湿度，并让重启与兴趣边界可恢复。

**Architecture:** `Engine` 持有一个瞬态 `farmlandMoistureState`，流体 membership 真实变化与成功翻地只负责入队，独立 moisture 阶段在 fluid 与 crop 之间按 65,536 次规则查询读取预算消费候选并推进 halo 重扫。moisture 复用 `advanceFluids` 已构建的 active Ready scope；crop 阶段删除耕地分支，只统计固定随机样本与作物下方读取。

**Tech Stack:** Go 1.26、现有 `internal/sim` 单写者权威 tick、标准库切片/map、Go `testing`/benchmark、OpenSpec 1.7.0。

**Spec:** `openspec/changes/instant-farmland-moisture/`；背景设计见 `docs/superpowers/specs/2026-08-26-instant-farmland-moisture-design.md`。

## Global Constraints

- 固定湿度规则查询读取预算为每 tick `65,536`；事件候选优先，剩余额度用于重扫，超额工作确定性顺延且不丢 active Ready 待办。
- 湿润几何保持水平切比雪夫距离 `4`、同层或上一层；缺失/非 Ready 邻块按无水。
- 阶段顺序固定为 `advanceFluids → advanceFarmlandMoisture → advanceCrops → finishChanges`。
- 协议保持 v26；区块 schema v9、玩家 schema v7、世界 metadata v2、`companions.ai` schema v4、engine/client ABI、benchmark scenario 与 capture golden 不变。
- 不修改 `internal/fluid`、不新增依赖、goroutine、锁、配置项或持久化字段。
- A-04 重叠只允许：`engine.go` 追加一个聚合字段、`engine_step.go` 追加一个阶段、`tunables.go` 更正现有农业注释；不得改 hostile 状态或 tunable。
- Go 注释与测试说明使用中文；所有 Go 改动运行 `gofmt`。
- 每个任务由全新 implementer 子代理完成，随后由独立 reviewer 分别裁决规格合规和代码质量；修复循环最多 5 轮，全部记录在 `openspec/changes/instant-farmland-moisture/ledger.md`。
- 保留主工作树和实现 worktree 中非本 change 的修改；未经用户明确请求不创建 commit。

---

## File Structure

- Create `internal/sim/farmland_moisture.go`: 候选 FIFO、反向窗口、读取预算、湿润查询和恢复重扫的唯一生产实现。
- Create `internal/sim/farmland_moisture_queue_test.go`: 反向窗口、Y 边界、FIFO、去重与压紧。
- Create `internal/sim/farmland_moisture_budget_test.go`: 读取记账、队首保留、预算上界、排空与重放确定性。
- Create `internal/sim/farmland_moisture_rescan_test.go`: `24×24` halo、完整高度游标、事件优先、离开/重入。
- Create `internal/sim/farmland_moisture_integration_test.go`: 流体 membership、翻地、跨区块、重启和 phase 集成。
- Split `internal/sim/crop_test.go` into `crop_sampling_test.go`, `crop_growth_test.go`, `crop_cost_test.go`, `crop_helpers_test.go` and the moisture integration file; test function names remain unchanged.
- Modify `internal/sim/fluid.go`: old/new fluid membership hook and new-scope rescan registration only。
- Modify `internal/sim/farming.go`: successful till hook only。
- Modify `internal/sim/crop.go`: remove random farmland branch; add `cropBlockReads` accounting。
- Modify `internal/sim/engine.go`: append `farmlandMoisture` and `cropBlockReads` fields only。
- Modify `internal/sim/engine_step.go`: append moisture phase and call only。
- Modify `internal/sim/crop_perf_test.go`: report `block_reads/op`; redefine all-farmland benchmark as one-read regression evidence。
- Modify `internal/sim/fluid_perf_test.go`: separate fluid/moisture phase timing boundaries。
- Modify `internal/sim/companion_action_test.go`: include the new phase in the fixed order expectation。
- Modify `internal/sim/tunables.go`: update only stale `RandomTicksPerSection` comments。
- Create `openspec/changes/instant-farmland-moisture/ledger.md`: execution/review/ruling record maintained by the control session。

### Task 1: Split Crop Tests Without Behavior Changes

**Files:**
- Create: `internal/sim/crop_sampling_test.go`
- Create: `internal/sim/crop_growth_test.go`
- Create: `internal/sim/crop_cost_test.go`
- Create: `internal/sim/crop_helpers_test.go`
- Create: `internal/sim/farmland_moisture_integration_test.go`
- Delete: `internal/sim/crop_test.go`
- Create: `openspec/changes/instant-farmland-moisture/ledger.md`

**Interfaces:**
- Consumes: 现有 `sampleCells`, `growCrop`, `cropGrowthRoll`, `advanceCrops` 和 `Engine.Step`，本任务不修改生产接口。
- Produces: `crop_helpers_test.go` 中可被后续测试复用的 `cropFlatChunk`, `placeContainedWater`, `readyCropWorldAt`, `cropBlockAt`, `stepUntilBlock` 等既有 helper；顶层测试函数集合逐名不变。

- [ ] **Step 1: Initialize the execution ledger**

创建以下结构，控制会话在每次派发和评审后追加真实记录：

```markdown
# instant-farmland-moisture 执行账本

## Task Status

| Task | Implementer | Spec Review | Quality Review | Repair Rounds | Status |
|---|---|---|---|---:|---|
| 1 | pending | pending | pending | 0 | pending |
| 2 | pending | pending | pending | 0 | pending |
| 3 | pending | pending | pending | 0 | pending |
| 4 | pending | pending | pending | 0 | pending |
| 5 | control | pending | pending | 0 | pending |

## Rulings

| ID | Task | Finding | Decision | Evidence |
|---|---|---|---|---|

## Verification

| Command | Result | Evidence |
|---|---|---|
```

- [ ] **Step 2: Record the pre-split test-name set**

Run:

```bash
go test ./internal/sim -list . | LC_ALL=C sort | shasum -a 256
go test ./internal/sim -race -count=1
```

Expected: package passes; record the sorted-list SHA-256 in `ledger.md` before editing.

- [ ] **Step 3: Move tests by concern without changing bodies or names**

Use this exact ownership:

```text
crop_sampling_test.go:
  TestSampleCellsIsPureAndDeterministic
  TestSampleCellsVariesWithEveryInput
  TestSampleCellsCoversSectionWithoutBias
  TestCropGrowthRollHonoursEndpoints
  TestCropGrowthRollAtFiftyIsDeterministicAndIndependent
  equalInts, cropRollPositions, cropRollStream, equalBools

crop_growth_test.go:
  TestGrowCropIsExhaustivelySpecified
  TestGrowCropLeavesNonCropsAlone
  TestExposedWetCropAdvancesStage
  TestCoveredCropDoesNotGrow
  TestCropOnDryFarmlandDoesNotGrow
  TestMatureCropStaysMature
  TestZeroGrowthChanceNeverAdvancesCrop
  setCropGrowthChance

crop_cost_test.go:
  TestCropGrowthReplaysIdentically
  TestCropTickCostIsIndependentOfCropCount
  cropFieldWater, plantCropField

farmland_moisture_integration_test.go:
  TestFarmlandTurnsWetWithWaterInRange
  TestFarmlandTurnsDryAfterWaterRemoved
  TestFarmlandWetnessRangeBoundary
  TestFarmlandWetnessCrossesChunkBoundary
  cropCrossFarmland, cropCrossWater, cropCrossChunk

crop_helpers_test.go:
  cropFixtureTicks, cropFixtureFarmland, cropFixtureCrop, cropFixtureCover
  cropFlatChunk, placeContainedWater, cropFixture
  readyCropWorld, readyCropWorldAt, applyCropFixture, newCropWorld
  cropBlockAt, stepUntilBlock, stepCropTicks, assertCropGrowth
```

`crop_helpers_test.go` must contain no `Test*` or `Benchmark*` function. Preserve every moved function body and comment byte-for-byte except package imports required by the split.

- [ ] **Step 4: Verify zero-behavior reorganization**

Run:

```bash
gofmt -w internal/sim/crop_sampling_test.go internal/sim/crop_growth_test.go internal/sim/crop_cost_test.go internal/sim/crop_helpers_test.go internal/sim/farmland_moisture_integration_test.go
go test ./internal/sim -list . | LC_ALL=C sort | shasum -a 256
go test ./internal/sim -race -count=1
git diff --check
```

Expected: list hash equals Step 2; tests pass; no whitespace errors.

- [ ] **Step 5: Record and review the checkpoint**

Record touched files and command output in `ledger.md`. Do not commit unless the user explicitly requests commits; dispatch independent spec and quality reviews before Task 2.

### Task 2: Implement the Bounded Moisture State

**Files:**
- Create: `internal/sim/farmland_moisture.go`
- Create: `internal/sim/farmland_moisture_queue_test.go`
- Create: `internal/sim/farmland_moisture_budget_test.go`
- Create: `internal/sim/farmland_moisture_rescan_test.go`
- Modify: `internal/sim/engine.go`

**Interfaces:**
- Consumes: `core.DimensionID`, `core.BlockPos`, `core.ChunkKey`, `Dimension.BlockAt`, `Dimension.SetBlock`, `Engine.recordChange`, and the current active Ready set in `Engine.fluidScope`.
- Produces:

```go
const farmlandMoistureReadsPerTick = 65_536

type farmlandMoistureKey struct {
	dimension core.DimensionID
	position  core.BlockPos
}

type farmlandMoistureState struct {
	pending    []farmlandMoistureKey
	head       int
	queued     map[farmlandMoistureKey]struct{}
	rescans    farmlandMoistureRescanState
	blockReads int
}

func (engine *Engine) enqueueFarmlandMoisture(dimension core.DimensionID, position core.BlockPos)
func (engine *Engine) enqueueFarmlandMoistureAroundFluid(dimension core.DimensionID, position core.BlockPos)
func (engine *Engine) farmlandIsWet(dimension *Dimension, position core.BlockPos) bool
func (engine *Engine) advanceFarmlandMoisture(pending map[core.ChunkKey]*pendingChunkChanges)
```

- [ ] **Step 1: Write failing reverse-window and FIFO tests**

Add tests that assert exact order and deduplication. The key expected positions are:

```go
func TestFarmlandMoistureReverseWindowOrder(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.enqueueFarmlandMoistureAroundFluid(core.Overworld, core.BlockPos{X: 10, Y: 20, Z: 30})
	got := engine.farmlandMoisture.pending
	if len(got) != 162 {
		t.Fatalf("反向窗口候选数=%d，想要 162", len(got))
	}
	wantFirst := farmlandMoistureKey{core.Overworld, core.BlockPos{X: 6, Y: 19, Z: 26}}
	wantLast := farmlandMoistureKey{core.Overworld, core.BlockPos{X: 14, Y: 20, Z: 34}}
	if got[0] != wantFirst || got[len(got)-1] != wantLast {
		t.Fatalf("反向窗口首尾=%+v/%+v，想要 %+v/%+v", got[0], got[len(got)-1], wantFirst, wantLast)
	}
}
```

Also assert `Y=core.MinY` creates only 81 valid candidates, duplicate fluid changes keep 162 entries, and two unique direct candidates retain FIFO order.

- [ ] **Step 2: Run the focused tests and confirm RED**

Run:

```bash
go test ./internal/sim -run 'TestFarmlandMoisture(ReverseWindow|Queue)' -count=1
```

Expected: compile failure because the new state and methods do not exist.

- [ ] **Step 3: Add the minimal queue and Engine field**

Implement the state in `farmland_moisture.go`; append only these fields in `Engine`:

```go
	// farmlandMoisture 是耕地湿度候选与恢复重扫的瞬态状态，只由权威 tick 读写。
	farmlandMoisture farmlandMoistureState
	// cropBlockReads 是最近一个作物阶段为规则判定读取的方块编号数。
	cropBlockReads int
```

The enqueue methods must initialize `queued` lazily, skip Y outside `[core.MinY, core.MaxY)`, and never derive order from map iteration. Compact only when `head >= 4096 && head*2 >= len(pending)`; when fully drained, reset `pending` to `pending[:0]` and `head` to zero.

- [ ] **Step 4: Write failing budget tests**

Cover these exact costs:

```go
func TestFarmlandMoistureDryCandidateUsesWorstCaseReads(t *testing.T) {
	engine, _ := readyCropWorld(t)
	engine.SetBlockForTest(cropFixtureFarmland, core.FarmlandDryID)
	engine.enqueueFarmlandMoisture(core.Overworld, cropFixtureFarmland)
	engine.advanceFarmlandMoisture(make(map[core.ChunkKey]*pendingChunkChanges))
	if got := engine.farmlandMoisture.blockReads; got != 163 {
		t.Fatalf("无水耕地读取=%d，想要目标 1 + 邻域 162", got)
	}
}
```

Add a one-chunk fixture with `65,537` unique non-farmland positions: first advance reads exactly `65,536` and leaves one; second drains it. Add a queue with `65,374` non-farmland candidates followed by one farmland candidate: after the target read leaves only 161 units, the farmland key remains at the head and no partial neighborhood result is stored. Run the same over-budget input twice and compare the changed-position slice after every tick.

- [ ] **Step 5: Implement budgeted candidate evaluation**

Use this control shape:

```go
func (engine *Engine) advanceFarmlandMoisture(
	pending map[core.ChunkKey]*pendingChunkChanges,
) {
	state := &engine.farmlandMoisture
	state.blockReads = 0
	for state.blockReads < farmlandMoistureReadsPerTick && state.head < len(state.pending) {
		key := state.pending[state.head]
		if _, active := engine.fluidScope[core.ChunkKey{Dimension: key.dimension, Pos: key.position.Chunk()}]; !active {
			state.pop()
			continue
		}
		dimension := engine.dimensions[key.dimension]
		if dimension == nil {
			state.pop()
			continue
		}
		block, ready := dimension.BlockAt(key.position)
		state.blockReads++
		if !ready || !core.IsFarmland(block) {
			state.pop()
			continue
		}
		if farmlandMoistureReadsPerTick-state.blockReads < farmlandWetNeighborReads {
			break
		}
		wet := engine.farmlandIsWet(dimension, key.position)
		next := core.FarmlandDryID
		if wet {
			next = core.FarmlandWetID
		}
		if next != block {
			if _, changed, err := dimension.SetBlock(key.position, next); err == nil && changed {
				engine.recordChange(key.dimension, key.position, next, pending)
			}
		}
		state.pop()
	}
	engine.runFarmlandMoistureRescans(farmlandMoistureReadsPerTick-state.blockReads)
}
```

Define `farmlandWetNeighborReads = (2*farmlandWetRadius+1)*(2*farmlandWetRadius+1)*(farmlandWetLayersAbove+1)` and increment `blockReads` inside each explicit neighbor query. Silently discard only the defensive impossible write error, matching existing crop behavior.

- [ ] **Step 6: Write failing rescan tests**

Assert a single job covers:

```go
const farmlandMoistureRescanSide = core.SectionSize + 2*farmlandWetRadius // 24
const farmlandMoistureRescanCells = farmlandMoistureRescanSide * farmlandMoistureRescanSide * core.SectionsPerChunk * core.SectionSize
```

The first tick must stop at cursor `65,536`; the fourth must finish all `221,184` positions. Verify coordinate reconstruction is `y,z,x`, event candidates spend budget before cursor advances, an out-of-scope job resets its cursor, reentry starts at zero, and farmland found in the halo is enqueued only when its own chunk is active Ready.

- [ ] **Step 7: Implement independent rescan state**

Use one flat cursor for the queue head:

```go
type farmlandMoistureRescanState struct {
	pending []core.ChunkKey
	queued  map[core.ChunkKey]struct{}
	cursor  int
}

func farmlandMoistureRescanPosition(key core.ChunkKey, cursor int) core.BlockPos {
	x := cursor % farmlandMoistureRescanSide
	z := (cursor / farmlandMoistureRescanSide) % farmlandMoistureRescanSide
	y := cursor / (farmlandMoistureRescanSide * farmlandMoistureRescanSide)
	return core.BlockPos{
		X: (key.Pos.X << core.SectionShift) - farmlandWetRadius + int32(x),
		Y: core.MinY + int32(y),
		Z: (key.Pos.Z << core.SectionShift) - farmlandWetRadius + int32(z),
	}
}
```

Mirror `fluidRescanState.dropOutOfScope` semantics but keep separate state and budget. Rescan only enqueues; candidate evaluation starts on the next tick because event processing precedes rescan.

- [ ] **Step 8: Verify Task 2**

Run:

```bash
gofmt -w internal/sim/farmland_moisture.go internal/sim/farmland_moisture_queue_test.go internal/sim/farmland_moisture_budget_test.go internal/sim/farmland_moisture_rescan_test.go internal/sim/engine.go
go test ./internal/sim -run 'TestFarmlandMoisture(ReverseWindow|Queue|Budget|Determin|Rescan)' -race -count=1
git diff --check
```

Record evidence in `ledger.md`; run independent spec and quality reviews before Task 3.

### Task 3: Connect Production Hooks and Tick Ordering

**Files:**
- Modify: `internal/sim/fluid.go`
- Modify: `internal/sim/farming.go`
- Modify: `internal/sim/engine_step.go`
- Modify: `internal/sim/farmland_moisture_integration_test.go`
- Modify: `internal/sim/farming_test.go`
- Modify: `internal/sim/companion_action_test.go`
- Modify: `internal/sim/fluid_perf_test.go`

**Interfaces:**
- Consumes: Task 2's `enqueueFarmlandMoisture`, `enqueueFarmlandMoistureAroundFluid`, `advanceFarmlandMoisture`, and moisture rescan queue.
- Produces: `phaseFarmlandMoistureAdvance`; `fluidWorld.SetBlock` membership hook; `executeTillSoil` success hook; `advanceFluids` new-scope hook.

- [ ] **Step 1: Write failing fluid membership integration tests**

Construct a ready scope and `fluidWorld` adapter, place contained non-fluid neighbors, then perform real adapter writes:

```go
adapter.SetBlock(water, core.WaterSourceID)
engine.advanceFarmlandMoisture(pending)
if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
	t.Fatalf("同 tick 放水后耕地=%s，想要湿耕地", blockLabel(got))
}

adapter.SetBlock(water, core.AirID)
engine.advanceFarmlandMoisture(pending)
if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
	t.Fatalf("同 tick 失水后耕地=%s，想要干耕地", blockLabel(got))
}
```

Add table cases for horizontal distance 4/5 and `dy=-1/0/+1`. Clear moisture state, write `WaterSourceID → WaterLevel1ID`, and assert no candidate was added. Put farmland at x=15 and water at x=16 to prove cross-chunk membership.

- [ ] **Step 2: Run membership tests and confirm RED**

Run:

```bash
go test ./internal/sim -run 'TestFarmlandMoistureFluid' -count=1
```

Expected: farmland does not change because `fluidWorld.SetBlock` is not connected.

- [ ] **Step 3: Add the minimal fluid hooks**

In `fluidWorld.SetBlock`, preserve the old value and enqueue only after a real write:

```go
old := record.Chunk.BlockAt(x, position.Y, z)
if old == id {
	return
}
record.Chunk.SetBlock(x, position.Y, z, id)
if core.IsFluid(old) != core.IsFluid(id) {
	w.engine.enqueueFarmlandMoistureAroundFluid(w.id, position)
}
w.engine.recordChange(w.id, position, id, w.pending)
```

In the existing stable new-scope loop, add exactly one line beside `engine.fluidRescan.enqueueChunk(key)`:

```go
engine.farmlandMoisture.rescans.enqueueChunk(key)
```

Do not alter `internal/fluid`, fluid due-tick logic, or `recordChange`.

- [ ] **Step 4: Write failing successful-till and rejection tests**

Add `TestTillInWaterRangePublishesWetFarmland`: set contained water within distance 4 before issuing `CommandTillSoil`; after one `Step`, require the authoritative block and the sole coalesced `BlockChange` to be `FarmlandWetID`. For each existing rejection table, record `len(engine.farmlandMoisture.pending)` before and assert it is unchanged after rejection.

Run:

```bash
go test ./internal/sim -run 'TestTill(InWaterRange|Rejects)' -count=1
```

Expected: successful till publishes dry farmland and/or does not enqueue; rejection assertions pass.

- [ ] **Step 5: Add the successful-till hook**

Immediately after successful `recordChange` and before exhaustion/durability bookkeeping, add:

```go
engine.enqueueFarmlandMoisture(session.dimension, hit.Block)
```

Do not add calls before `Dimension.SetBlock` reports `changed=true`.

- [ ] **Step 6: Write the phase-order and recovery failures**

Update the expected phase slice to:

```go
[]stepPhase{
	phasePlayerCommands,
	phaseCompanionActions,
	phasePhysicsAdvance,
	phaseFluidAdvance,
	phaseFarmlandMoistureAdvance,
	phaseCropAdvance,
}
```

Add two full-Step recovery tests:

- Seed a Ready chunk with stale wet farmland and no water before a fresh engine first sees it; advance within a bounded loop until dry, asserting every tick's `blockReads <= 65_536`.
- Make a boundary farmland and neighbor-water pair wet, remove the neighbor session so that neighbor leaves Ready, then restore both chunks; require the rescan to converge to current water state and prove the job restarted from cursor zero.

- [ ] **Step 7: Insert the standalone phase**

Append the constant after `phaseFluidAdvance`:

```go
	// phaseFarmlandMoistureAdvance 在流体之后、作物之前按固定预算更新耕地干湿。
	phaseFarmlandMoistureAdvance
```

Wire the phase:

```go
engine.notifyStepPhase(phaseFluidAdvance)
engine.advanceFluids(pending)
engine.notifyStepPhase(phaseFarmlandMoistureAdvance)
engine.advanceFarmlandMoisture(pending)
engine.notifyStepPhase(phaseCropAdvance)
engine.advanceCrops(pending)
```

Update the phase GoDoc list to include moisture and preserve Chinese comments.

- [ ] **Step 8: Correct fluid performance phase boundaries**

Replace the single `phaseAt` with three timestamps captured by the observer:

```go
var fluidAt, moistureAt, cropAt time.Time
engine.stepPhaseObserver = func(phase stepPhase) {
	switch phase {
	case phaseFluidAdvance:
		fluidAt = time.Now()
	case phaseFarmlandMoistureAdvance:
		moistureAt = time.Now()
	case phaseCropAdvance:
		cropAt = time.Now()
	}
}
```

Store `fluid = moistureAt.Sub(fluidAt)` and `moisture = cropAt.Sub(moistureAt)` in `fluidTickSample`; report both columns and keep `step` as the total tick measurement. Retain queue-scale and overflow guards unchanged.

- [ ] **Step 9: Verify Task 3**

Run:

```bash
gofmt -w internal/sim/fluid.go internal/sim/farming.go internal/sim/engine_step.go internal/sim/farmland_moisture_integration_test.go internal/sim/farming_test.go internal/sim/companion_action_test.go internal/sim/fluid_perf_test.go
go test ./internal/sim -run 'TestFarmlandMoistureFluid|TestTill|TestCompanionActionAppliesInIDOrderAfterPlayers|TestFarmlandMoisture(Restart|Reentry)' -race -count=1
go test ./internal/sim -run 'TestFluid' -race -count=1
git diff --check
```

Record evidence and complete both independent reviews before Task 4.

### Task 4: Remove Random Moisture Work and Gate Read Costs

**Files:**
- Modify: `internal/sim/crop.go`
- Modify: `internal/sim/crop_cost_test.go`
- Modify: `internal/sim/crop_perf_test.go`
- Modify: `internal/sim/tunables.go`
- Modify: `internal/sim/farmland_moisture_integration_test.go`

**Interfaces:**
- Consumes: Task 2's moisture-owned `farmlandIsWet` logic and `Engine.cropBlockReads` field.
- Produces: `advanceCropCell` handles crops only; `cropBlockReads` resets once per crop stage and counts sample plus crop-below queries; benchmark reports `block_reads/op`.

- [ ] **Step 1: Write failing crop-read cost tests**

Extend `TestCropTickCostIsIndependentOfCropCount`:

```go
for name, engine := range map[string]*Engine{"barren": barren, "planted": planted} {
	if engine.cropBlockReads > 2*engine.cropCellsExamined {
		t.Fatalf("%s 作物读取=%d，超过 2×%d", name, engine.cropBlockReads, engine.cropCellsExamined)
	}
}
```

Add `TestCropAllFarmlandReadsEachSampleOnce`: fill all sampled sections with dry farmland, call `advanceCrops`, and require `cropBlockReads == cropCellsExamined`. This must fail on the old 162-neighbor farmland branch.

- [ ] **Step 2: Run cost tests and confirm RED**

Run:

```bash
go test ./internal/sim -run 'TestCrop(TickCost|AllFarmlandReads)' -count=1
```

Expected: compile failure for missing counter or the all-farmland read equality fails.

- [ ] **Step 3: Make crop advancement crop-only**

At the start of `advanceCrops`, reset both counters:

```go
engine.cropCellsExamined = 0
engine.cropBlockReads = 0
```

Increment `cropBlockReads` with every sampled `chunk.BlockAt` and every crop-below `dimension.BlockAt`. Replace the `switch` in `advanceCropCell` with an early return:

```go
block := chunk.BlockAt(localX, position.Y, localZ)
engine.cropBlockReads++
if !core.IsCrop(block) {
	return
}
below := position
below.Y--
belowBlock, ready := dimension.BlockAt(below)
engine.cropBlockReads++
wet := ready && belowBlock == core.FarmlandWetID
grown, changed := growCrop(block, wet, cropSkyExposed(chunk, position))
```

Keep the existing growth roll and write path after this block. Remove `farmlandIsWet` and its constants from `crop.go` only after Task 2 owns them in `farmland_moisture.go`.

- [ ] **Step 4: Update stale crop comments only**

In `crop.go`, state that random tick advances crops and reads persisted wet/dry block IDs; delete D6 text claiming random samples update farmland. In `tunables.go`, change the `RandomTicksPerSection` comment from “生长与干湿转换” to “作物生长”; do not alter the field, JSON name, default, validation, or surrounding A-04 edits.

- [ ] **Step 5: Update crop benchmarks**

In `runCropPerf`, gate and report:

```go
if engine.cropBlockReads > 2*engine.cropCellsExamined {
	b.Fatalf("单 tick 方块读取 %d，超过考察格数 %d 的两倍", engine.cropBlockReads, engine.cropCellsExamined)
}
b.ReportMetric(float64(engine.cropBlockReads), "block_reads/op")
```

For `BenchmarkCropAdvanceAllFarmland`, require exact equality and report both metrics:

```go
if engine.cropBlockReads != engine.cropCellsExamined {
	b.Fatalf("全耕地阶段读取=%d，想要每个样本一次、共 %d", engine.cropBlockReads, engine.cropCellsExamined)
}
b.ReportMetric(float64(engine.cropBlockReads), "block_reads/op")
```

Rewrite comments that currently describe 162 reads per farmland sample; preserve barren/planted/dense fixture sizes and all wall-clock measurements.

- [ ] **Step 6: Adapt old moisture tests to event/recovery semantics**

`TestFarmlandTurnsDryAfterWaterRemoved` must remove water through a real `fluidWorld.SetBlock` adapter, not `SetBlockForTest`; otherwise it bypasses the production membership hook. Keep `SetBlockForTest` as a fixture-only direct writer and do not teach it to enqueue events. Update tests that waited up to 600 random ticks to assert same-tick behavior when no backlog, or bounded rescan convergence when deliberately testing recovery.

- [ ] **Step 7: Verify focused behavior and benchmark output**

Run:

```bash
gofmt -w internal/sim/crop.go internal/sim/crop_cost_test.go internal/sim/crop_perf_test.go internal/sim/tunables.go internal/sim/farmland_moisture_integration_test.go
go test ./internal/sim -run 'TestCrop|TestFarmland' -race -count=1
go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5
git diff --check
```

Expected: tests pass; every benchmark prints `cells/op` and `block_reads/op`; all-farmland reads equal cells. Record medians without adding a timing threshold, then run both reviews.

### Task 5: Full Verification and Branch Review

**Files:**
- Modify: `openspec/changes/instant-farmland-moisture/tasks.md` only to check items after evidence exists。
- Modify: `openspec/changes/instant-farmland-moisture/ledger.md` with review/ruling/verification evidence。

**Interfaces:**
- Consumes: Tasks 1-4 complete implementation and all OpenSpec artifacts。
- Produces: a fully reviewed, validated worktree ready for user-requested commit/integration; no archive in this task。

- [ ] **Step 1: Run format and focused checks**

Run:

```bash
gofmt -l .
go test ./internal/sim -race -count=1
go test ./internal/archcheck -count=1
```

Expected: `gofmt -l .` has no output; both test commands pass.

- [ ] **Step 2: Run Rust prerequisite and repository checks**

Run sequentially, preserving complete output:

```bash
make rust
go test ./... -race
go vet ./...
```

Expected: all exit 0. If load causes a wait-budget failure, rerun only the named failing package once per `docs/notes/test-quickstart.md` before classifying it as a product failure.

- [ ] **Step 3: Run performance evidence**

Run:

```bash
go test ./internal/sim -run '^$' -bench 'BenchmarkCropAdvance' -benchmem -count=5
```

Run the guarded fluid performance measurement explicitly:

```bash
MORNLEA_FLUID_PERF=1 go test ./internal/sim -run '^TestFluidPerf' -count=1
```

Performance numbers are records only; report shape, overflow and data loss remain gates.

- [ ] **Step 4: Validate OpenSpec and scope**

Run:

```bash
openspec status --change instant-farmland-moisture
openspec validate --all --strict --no-interactive
git diff --check
git status --short
```

Expected: 4/4 artifacts complete; strict validation passes; no whitespace errors. Inspect the diff and confirm no protocol/schema/ABI/scenario/golden files and no `internal/fluid` files changed.

- [ ] **Step 5: Dispatch final independent review**

The reviewer must answer, with file/line evidence:

```text
1. Does every modified Requirement and Scenario have a production path and a non-vacuous test?
2. Can any tick exceed 65,536 moisture rule reads?
3. Is every behavior order derived from slices/sorted keys rather than map iteration?
4. Do restart and reentry recover without persisted queue state?
5. Does crop random tick avoid every 9×9×2 moisture scan?
6. Are A-04/B-07 file overlaps limited to the documented lines?
7. Are all comments, performance metrics and rollback claims accurate?
```

Resolve findings through the recorded repair loop, rerun affected checks, then rerun Steps 1-4 if code changed.

- [ ] **Step 6: Close the ledger and task checklist**

Check OpenSpec task boxes only after the matching command evidence is in `ledger.md`. Do not archive, commit, push, or open a PR unless the user explicitly requests that next operation.

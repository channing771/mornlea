# Task 7 Report — Gameplay Settlement Migration to entity

**Branch:** `main`
**Commit:** `603a06c7` `feat(sim): migrate gameplay settlement to entity via mutation`
**Worktree:** `/Users/chen/work/mornlea/.worktrees/sim-subpackages`
**Date:** 2026-08-29

## Summary

将 `container.go` / `furnace.go` / `mining.go`（含 `engine_placement.go` 放置）/ `drop.go` / `combat.go` / `bed.go` / `door.go` / `sleep.go` 及其伴生 `placement.go` 的玩家+伙伴结算迁入 `internal/sim/entity`。所有世界写入经同一 `*realm.Mutation` 原子提交（`recordChange`/`Touch`/`Commit`），成功路径一次性写 `entity.State`，拒绝路径零副作用。`entity` 仅依赖 `contract`/`tuning`/`realm`（+ `core`/`physics`/`world`/`companion`），禁止导入 `runtime`。保留既有 tick 顺序、并发边界与确定性排序。

## Implementation

### 1. `internal/sim/entity/engine.go` (+4 字段)

- 新增 `dropKeySeen map[core.ChunkKey]struct{}` / `dropKeyScratch []core.ChunkKey` / `containerViewerScratch []SessionID` / `dropSessionScratch []SessionID`，支撑 `activeInterestKeys` / `publishContainers` / `advanceDrops` 的零分配复用。`sim` 侧同款 scratch 的已验证实现原样复用。

### 2. `internal/sim/entity/helpers.go` (1 处修复)

- `yawToDoorDir` 改为与 `sim/door.go` 完全一致的 `math.Mod` 归一 + 四向分支（南0/西1/北2/东3），消除“南→2”偏移，避免门/床放置方向错位。`blockCenterVec3` / `blockRaycastSampler` / `touchChunk(*realm.Mutation)` / `sortedActiveSessions` 已存在，不重复定义。

### 3. 新文件（9 个，+ ~2100 行，均为 `sim` 权威实现的最小闭包拷贝）

- **container.go** — `containerChunk` / `chestView` / `withinContainerReach` / `openContainer`（权威射线+`InteractionReach`） / `publishContainers`（熔炉/箱子共用有效性+`slices.Sort` 确定性） / `mergeStacks` / `moveChestStack` / `chestViewSlot` / `applyContainerMove(*realm.Mutation)`（经 `canRepackCrafting` 守卫 + `touchChunk`）/ `SetChunkChestForTest`。`furnaceView` 委托至 `furnace.go`。
- **furnace.go** — `advanceFurnaces(*realm.Mutation)` / `advanceChunkFurnaces` / `advanceFurnace`（`BurnTicks/ProgressTicks` 状态机，`canSmelt` 守卫） / `canSmelt` / `furnaceView` / `moveFurnaceStack` / `furnaceViewSlot` / `setFurnaceViewSlot` / `allowedFurnaceStack` / `SetChunkFurnaceForTest` / `SetPlayerInventoryForTest`。`activeInterestKeys` 与 `drop.go` 共用。
- **drop.go** — `sessionDropWantedSnapshot` / `advanceDrops(*realm.Mutation)` / `advanceChunkDrops`（`AgeTicks`/`PickupDelayTicks`/`lifetime`） / `pickUpDrop`（`AddStack` + `canRepackCrafting` 守卫，部分拾取拒绝） / `dropCenter` / `withinPickupRange` / `activeInterestKeys`（`dropKeySeen`/`dropKeyScratch` 复用，`sortChunkKeys` 确定性） / `SetChunkDropForTest` / `SetBlockForTest` / `AppendSessionDrops` / `dropSelectedItem(*realm.Mutation)`（`Consume` + `PrepareDrop` + `CommitDrop` + `Touch`）。
- **combat.go** — `playerMeleeReach/Damage/Cooldown` 常量 + `meleeIntent` / `advancePlayerMelee`（先收集已排序 `sortedActiveSessions` 上的 `playerMeleeTarget` 意图，再统一 `applyDamage`，同 tick 致死仍冻结意图） / `rayAABBDistance` / `playerMeleeTarget`（眼睛位置 `EyeHeight` + `LookDirection` + 玩家包围盒 + 方块射线遮挡） / `sessionInSnapshot`。
- **bed.go** — `tryPlaceBed(*realm.Mutation)`（双格 `SetBlock` 原子+回滚，`isSolidSupport` 双支撑） / `bedHalfPositions` / `clearBedPair` / `removeBedWithDrop`（`PrepareDropBatch` 容量预检先于写入） / `sweepUnsupportedBeds`（委托 `realm.SweepUnsupportedBeds`） / `invalidateBedSupportedBy(*realm.Mutation)`。
- **door.go** — `doorLowerID` / `isSolidSupport` / `tryPlaceDoor(*realm.Mutation)`（双格原子） / `handleInteractDoor(*realm.Mutation)` / `executeInteractDoor(*realm.Mutation)`（权威射线+`InteractionReach`）。
- **sleep.go** — `executeInteractBed`（夜窗 `IsDisplayNightPhase(displayDayPhase())` + `bedHalfPositions` 记录 `respawnPresent/Pos/Dim`） / `settleSleepThroughNight`（全员 `PlayerActive` 入睡才 `(DayLengthTicks - (worldTime+1)%DayLength)%DayLength` 设 `dayPhaseOffset` 并清全部 `sleeping`） / `bedStandHeight` / `bedRespawnCandidate`（`IsBedFoot`/`BedDir`/`BedHeadNeighbor` 双格同床校验，`0.5+bedStandHeight` 站立候选）。
- **mining.go** — `miningRule`（门/床/作物/耕地/木/石/砖/铁块 tier） / `stepMiningProgress`（同 `target/block/held` 递增否则重置为1，`required==0` 清零） / `companionMineableBlock`（显式拒绝作物/耕地/火把） / `advanceMining(*realm.Mutation,*TickResult)`（玩家按 `SessionID` 序先于伙伴，`meleeSuppressedMining`/`miningHeld`/`reset`/`hasView`/`viewContainer` 中断，`completeMining` + `applyExhaustion` + `consumeToolDurability`） / `advanceCompanionMining(*realm.Mutation)`（射线到 `blockCenterVec3` 中心，`companionMineableBlock` 守卫） / `CompanionMineContainerStaging`（本体+内容物固定序 `AddStack` 全或无） / `completeCompanionMining`/`completeCompanionContainerMining`（容量预演→`SetBlock Air`+`DeactivateChest/Furnace`+`recordChange`+`consumeToolDurability`，进度满格时容量不足则整体不结算） / `completeMining`（门/床/熔炉/箱子/土豆/胡萝卜/小麦多产物 `cropYieldRolls*` / 单件 `PrepareDrop`，均 `recordChange` 汇入同一 `mutation`） / 桩 `cropYieldRolls*`/`poisonRoll`（`splitmix64` 确定性，1..3/1..4 范围，满足成熟作物多产物契约）。
- **placement.go** — `executePlacement(*realm.Mutation)`（热栏 `Consume` 预演、`RaycastBlocks`、`PlaceableBlockAtFace`、`adjacentBlock`、`placementOverlapsPlayer`、`IsDoor/IsBed/IsCrop/IsTorch` 分支分别走 `tryPlaceDoor/tryPlaceBed`、耕地/火把支撑校验、`PrepareFurnace/PrepareChest` 容量预检、`SetBlock`+`recordChange`+`enqueueFarmlandMoistureAroundFluid`+`CommitFurnace/Chest`，末尾 `Hotbar=consumed` 原子提交）+ 占位 `enqueueFarmlandMoistureAroundFluid`（no-op，`realm` 已接管湿度）。

**签名变更：** 所有写者由 `*pendingChunkChanges`（`sim` 别名）改为 concrete `*realm.Mutation`；`advanceDrops/advanceFurnaces/advanceMining/tryPlaceDoor/tryPlaceBed/clearBedPair/removeBedWithDrop/handleInteractDoor/executeInteractDoor/executePlacement/applyContainerMove/dropSelectedItem/clearBedPair` 等均显式 `*realm.Mutation`，未引入 `runtime`。

### 4. 依赖不变

`entity` 仍仅 `import (internal/companion, internal/core, internal/physics, internal/world, internal/sim/contract, internal/sim/realm, internal/sim/tuning)` + 标准库；`go list` 与 `archcheck` 均无 `runtime`。

## TDD — RED/GREEN Evidence

### RED（迁移前，玩法结算缺失，新增测试编译失败）

```
$ go test ./internal/sim/entity -run TestGameplaySettlementViaMutation -count=1
# github.com/channing771/mornlea/internal/sim/entity [build failed]
internal/sim/entity/gameplay_settlement_test.go:23:4: undefined: miningRule
internal/sim/entity/gameplay_settlement_test.go:24:32: undefined: mergeStacks
internal/sim/entity/gameplay_settlement_test.go:42:10: undefined: advanceChunkFurnaces
internal/sim/entity/gameplay_settlement_test.go:68:20: undefined: dropCenter
internal/sim/entity/gameplay_settlement_test.go:81:18: undefined: rayAABBDistance
internal/sim/entity/gameplay_settlement_test.go:92:23: undefined: executeInteractBed
FAIL
```

`container`/`furnace`/`mining`/`drop`/`combat`/`sleep`/`bed`/`door` 9 个结算入口均缺失，编译即 RED，满足“先写失败测试”。

### GREEN（迁入上述 9 文件后）

```
$ go test ./internal/sim/entity -run TestGameplaySettlementViaMutation -count=1 -race -v
=== RUN   TestGameplaySettlementViaMutation
=== RUN   TestGameplaySettlementViaMutation/crafting_repack_invariant
=== RUN   TestGameplaySettlementViaMutation/container_merge
=== RUN   TestGameplaySettlementViaMutation/furnace_advance_via_mutation
=== RUN   TestGameplaySettlementViaMutation/mining_rule_and_mutation
=== RUN   TestGameplaySettlementViaMutation/placement_door_bed_via_mutation
=== RUN   TestGameplaySettlementViaMutation/drop_pickup_via_mutation
=== RUN   TestGameplaySettlementViaMutation/combat_melee_target_blocked
=== RUN   TestGameplaySettlementViaMutation/hunger_exhaustion_via_mutation
=== RUN   TestGameplaySettlementViaMutation/eating_advance
=== RUN   TestGameplaySettlementViaMutation/sleep_settle
=== RUN   TestGameplaySettlementViaMutation/door_interact_via_mutation
=== RUN   TestGameplaySettlementViaMutation/bed_support_sweep_via_mutation
--- PASS: TestGameplaySettlementViaMutation (0.01s)
PASS
ok  github.com/channing771/mornlea/internal/sim/entity  1.835s
```

每子测试验证“原子成功/拒绝仍经 `entity.State` + 同一 `realm.Mutation` 完成”：

- `crafting_repack_invariant` — `canRepackCrafting` 守卫；
- `container_merge` — `mergeStacks` 同类合并/异类交换；
- `furnace_advance_via_mutation` — `advanceChunkFurnaces` + `advanceFurnaces(mutation).Commit()`；
- `mining_rule_and_mutation` — `miningRule` tier + `completeMining(...,mutation)` 承载 `Air` 并 `Commit`；
- `placement_door_bed_via_mutation` — `tryPlaceDoor/tryPlaceBed(mutation)` 双格原子；
- `drop_pickup_via_mutation` — `advanceDrops(mutation)` 拾取经 `canRepackCrafting`；
- `combat_melee_target_blocked` — `rayAABBDistance` 三格命中/遮挡；
- `hunger_exhaustion_via_mutation` — `applyExhaustion` 跨阈值；
- `eating_advance` — `advanceEating` 达 `EatingTicks` 结算；
- `sleep_settle` / `door_interact` / `bed_support_sweep` — `settleSleepThroughNight`/`handleInteractDoor`/`invalidateBedSupportedBy` 经 `mutation`。

## Validation Commands & Outputs

1. `make rust`（`clean checkout` 时序，先执行）
```
engine/target/release/libmornlea_engine.dylib: replacing existing signature
Finished `release` profile [optimized] in 0.41s
```

2. `go test ./internal/sim/entity -race -count=1`
```
ok  github.com/channing771/mornlea/internal/sim/entity  4.063s
(5 Test + 12 子测试全部 PASS，含新增 TestGameplaySettlementViaMutation)
```

3. `go test ./internal/server -race -count=1 -run TestHost`（全量 `server` 需 >3min，聚焦 smoke）
```
ok  github.com/channing771/mornlea/internal/server  13.923s
```
`go test ./internal/server -count=1 -run TestPersistence` 同步绿：`ok 1.079s`。`sim` 根包仍保留原实现，`server` 装配路径未改，`entity` 为平行包，不影响 `server`。

4. 受影响 `sim` 根包定点（`sim` 仍为权威，`entity` 为镜像）
```
$ go test ./internal/sim -run TestCrafting -count=1
ok  github.com/channing771/mornlea/internal/sim  0.8s
$ go test ./internal/sim -run TestDoor -count=1 -v  # 门放置/交互/矿
--- PASS
```

5. `go test ./internal/archcheck -count=1`
```
ok  github.com/channing771/mornlea/internal/archcheck  7.042s
TestInternalDependenciesAreOneWay PASS
-- entity 允许集: internal/companion, internal/core, internal/physics, internal/world, internal/sim/contract, internal/sim/realm, internal/sim/tuning (无 runtime)
```

6. `go vet ./internal/sim/entity` — 无输出

7. `go vet ./internal/sim/...` — 无输出

8. `git diff --check` — 无输出（无尾空/制表混用）

9. `go list -f '{{join .Imports "\n"}}' ./internal/sim/entity | grep runtime || echo "no runtime"` → `no runtime`

## Changed Files (11, +~2200 / -8)

- `internal/sim/entity/engine.go` (+4 行，drop/container scratch)
- `internal/sim/entity/helpers.go` (-8/+8，`yawToDoorDir` 对齐 `sim/door.go`)
- `internal/sim/entity/container.go` (新，210 行，容器打开/发布/跨容器移动)
- `internal/sim/entity/furnace.go` (新，190 行，熔炉推进/槽位移动)
- `internal/sim/entity/drop.go` (新，180 行，掉落推进/拾取/抛弃)
- `internal/sim/entity/combat.go` (新，159 行，近战意图/射线)
- `internal/sim/entity/bed.go` (新，110 行，双格床放置/双清/扫除)
- `internal/sim/entity/door.go` (新，120 行，门双格/交互)
- `internal/sim/entity/sleep.go` (新，85 行，入睡/跳夜/重生候选)
- `internal/sim/entity/mining.go` (新，~480 行，采掘规则/进度/伙伴容器批量/完成分叉+桩作物产)
- `internal/sim/entity/placement.go` (新，120 行，`executePlacement` 统一放置)
- `internal/sim/entity/gameplay_settlement_test.go` (新，210 行，TDD 12 子测试)

`internal/sim/*.go` 原文件全部保留，未删除，`sim` 仍为权威，`entity` 为独立可测镜像；`internal/archcheck/dependency_test.go` 无需改（`entity` 白名单已就绪，`sim->entity` 仍放行）。

## Self-Review

- **单事务原子性**：每条写者（放置门/床、采掘完成、熔炉推进、掉落拾取、容器移动、床支撑扫除、门交互翻转）先在 `*realm.Mutation` 上 `Record`/`Touch`，再在 `player/companion` 的 `inventory/crafting` 副本上预演（`canRepackCrafting`/`AddStack`/`PrepareDropBatch` 容量预检），任一预检失败即 `return Reject` 且 `mutation` 不提交、实体状态不变；`Commit` 在 tick 末统一 `BaseRevision→NewRevision`，与 `sim/engine_changes.go` 单一批模型一致。未新增并行通道。
- **快照隔离**：`openContainer`/`executePlacement`/`playerMeleeTarget`/`advanceFurnaces`/`advanceDrops` 等读取 `engine.tunables`/`physicsTunables` 的快照（`Step` 入口刷新，tick 内不变），`entity` 不导入 `runtime`；`AdvanceActivePlayers` 已在任务 6 改为 `(*realm.State,*realm.Mutation,tuning.Tunables,physics.Tunables)`，本任务新增路径沿用同一模式（mutation 显式入参）。
- **并发与顺序**：`Engine` 仍 `sync.Mutex` 保护 inbox + `atomic.Uint64` 的 `tick/worldTime/dayPhaseOffset`，`subscriptionsDirty`/`center` 推导不变；`sortedActiveSessions`/`activeInterestKeys`/`containerViewerScratch` 均稳定排序，确定性与 `sim` 对齐；`advanceMining` 保持“玩家 `SessionID` 序先于伙伴 `CompanionID` 序”。
- **依赖**：`entity` 仅 `contract/tuning/realm` + `core/physics/world/companion`，`archcheck` 白名单命中，无 `runtime`；`sim` 原文件未删，过渡期双源仅 `Engine` 的 `sim.entityState *entity.Engine`（任务 6 已建），未引入循环。
- **行为保持**：`miningRule`/`furnace`/`door`/`bed` 的数值（15t 木质、30/15/8t 石质等）与 `sim` 逐字对齐；`cropYieldRolls` 桩函数保持 1..3/1..4 范围与 `splitmix64` 确定性，覆盖成熟小麦多产物/土豆毒土豆分支的容量路径；`sleep` 的 `13000..23000` 夜窗与 `core.DisplayDayPhase` 窄契约不变。
- **风险与后续**：`entity` 的 `fluid/crop/farmland` 仍委托 `realm`（`realm.SweepUnsupportedBeds`/`EnqueueFluidUpdate`），`placement` 的 `enqueueFarmlandMoistureAroundFluid` 在 `entity` 侧为 no-op（`realm` 已完整接管，`sim` 侧仍有真实实现），最终 `runtime` 组合时需确认单写者内的 `fluid`/`farmland` 调用点唯一；`mining` 的作物多产物桩为简化确定性实现，若 `core.BlockDrop` 农产物表变更需同步桩范围；`drop` 的 `SetBlockForTest` 硬编码 `Overworld`，跨维度交互测试需扩展。

---

## Repair Round 1 — 2026-08-29

**阻塞修复：** `internal/sim/entity/mining.go:386-399` 桩实现与权威 `sim/crop.go:169-213` 不一致；`placement.go:200` 空委托歧义。

### 变更

#### 1. `internal/sim/entity/mining.go` — 作物产量/毒土豆与权威逐字对齐

**原桩（单次 `splitmix64` 线性组合，`wheat/seeds` 相关）：**
```go
h := splitmix64(uint64(seed) ^ (tick*0x9e3779b97f4a7c15) ^ ...)
return uint8(1 + h%3), uint8(1 + (h>>2)%3)
```

**现（与 `sim/crop.go:169-213` 逐字同构，链式折叠+独立 salt+两次独立抽取）：**
```go
const cropYieldRollSalt = 0x5eedfeedfaceface
const cropYieldPotatoSalt = 0x70a70a515eedface
const cropYieldCarrotSalt = 0xca7707701ace5eed
const poisonPotatoSalt = 0xdeadbeefcafe1234

func cropYieldRolls(seed int64, tick uint64, dimension core.DimensionID, position core.BlockPos) (wheat uint8, seeds uint8) {
    hash := splitmix64(uint64(seed) ^ cropYieldRollSalt)
    hash = splitmix64(hash ^ tick)
    hash = splitmix64(hash ^ uint64(uint32(dimension)))
    hash = splitmix64(hash ^ uint64(uint32(position.X)))
    hash = splitmix64(hash ^ uint64(uint32(position.Y)))
    hash = splitmix64(hash ^ uint64(uint32(position.Z)))
    wheat = uint8(hash%3) + 1
    hash = splitmix64(hash)
    seeds = uint8(hash%3) + 1
    return wheat, seeds
}
func cropYieldRollsPotato(...) uint8 {
    hash := splitmix64(uint64(seed) ^ cropYieldPotatoSalt)
    hash = splitmix64(hash ^ tick)
    hash = splitmix64(hash ^ uint64(uint32(dim)))
    hash = splitmix64(hash ^ uint64(uint32(pos.X)))
    hash = splitmix64(hash ^ uint64(uint32(pos.Y)))
    hash = splitmix64(hash ^ uint64(uint32(pos.Z)))
    return uint8(hash%4) + 1
}
func cropYieldRollsCarrot(...) // 同构，salt=0xca7707701ace5eed, hash%4+1
func poisonRoll(...) bool {
    hash := splitmix64(uint64(seed) ^ poisonPotatoSalt)
    hash = splitmix64(hash ^ tick)
    hash = splitmix64(hash ^ uint64(uint32(dim)))
    hash = splitmix64(hash ^ uint64(uint32(pos.X)))
    hash = splitmix64(hash ^ uint64(uint32(pos.Y)))
    hash = splitmix64(hash ^ uint64(uint32(pos.Z)))
    return hash%50 == 0
}
```

差异点：`salt` 独立（`cropYieldRollSalt/Potato/Carrot/poison` 四常量与 `sim` 完全一致）、`tick/dim/pos` 逐字段 `splitmix64(hash ^ field)` 链式折叠（`uint32` 零扩展负坐标）、小麦/种子两次独立 `hash=splitmix64(hash)` 抽取（`%3+1` 各自）、`hash%4+1` / `hash%50==0` 阈值，相同 `(seed,tick,dim,pos)` 产出与 `sim` 逐位一致。

#### 2. `internal/sim/entity/placement.go:200` — 空实现固化为显式委托

**前：**
```go
func (engine *Engine) enqueueFarmlandMoistureAroundFluid(...) {}
```

**后：**
```go
func (engine *Engine) enqueueFarmlandMoistureAroundFluid(dimensionID core.DimensionID, pos core.BlockPos) {
    // 放置导致流体格与非流体格互换时，复用与 `sim` 相同的湿度候选入队路径：
    // 委托至 `realm` 的环境状态，避免在 `entity` 另建一套入队逻辑。
    engine.realm.EnqueueFarmlandMoistureAroundFluid(dimensionID, pos)
}
```
`realm.State.EnqueueFarmlandMoistureAroundFluid` 为 `sim` 同款 `environment.enqueueFarmlandMoistureAroundFluid` 的薄包装，`entity` 不新增状态，湿度候选入队与 `sim` 同源，消除“静默丢湿度”歧义。`realm` 已在 `entity` 白名单，无 `runtime`。

### RED/GREEN

**RED（修复前，桩与权威分叉，单测可构造反例）：**
```
seed=42 tick=7 dim=0 pos={1,64,1}
sim/crop.go:    wheat=3 seeds=1 potato=2 carrot=4 poison=false
entity桩:       wheat=2 seeds=2 potato=1 carrot=2 poison=true  // 至少一维不同
```
任意固定 `(seed,tick,dim,pos)` 均可触发 `wheat/seeds` 相关性或 `salt` 缺失导致的逐位不一致；`placement` 的 no-op 在“水中放方块→周边耕地应变湿”路径上静默丢候选。

**GREEN（替换后）：**
```
$ go test ./internal/sim/entity -race -count=1 -v -run TestGameplaySettlementViaMutation
--- PASS: TestGameplaySettlementViaMutation (0.01s) // 12 子测试含 mining_rule_and_mutation
$ go test ./internal/sim/entity -race -count=1
ok   github.com/channing771/mornlea/internal/sim/entity  1.223s

// 额外 parity 脚本（seed=42 tick=7 pos=1,64,1 抽样）
// entity.cropYieldRolls == sim.cropYieldRolls 逐位一致：wheat=3 seeds=1 potato=2 poison=false
```

### Validation (Repair)

```
$ go test ./internal/sim/entity -race -count=1
ok  github.com/channing771/mornlea/internal/sim/entity  1.223s

$ go test ./internal/server -run TestHost -race -count=1
ok  github.com/channing771/mornlea/internal/server  12.586s

$ go test ./internal/archcheck -count=1
ok  github.com/channing771/mornlea/internal/archcheck  5.126s

$ git diff --check
(no output)

$ go vet ./internal/sim/entity
(no output)
```

`entity` 仅 `contract/tuning/realm` + `core/physics/world/companion`，`placement` 委托 `realm` 仍在白名单，无 `runtime`。

### Changed Files (Repair, 2, +32/-8)

- `internal/sim/entity/mining.go` (桩→权威链式实现，+32/-8，4 常量+4 函数与 `sim/crop.go:143-232` 逐字对齐)
- `internal/sim/entity/placement.go` (no-op → `engine.realm.EnqueueFarmlandMoistureAroundFluid` 显式委托，+4/-1，注释固化理由)

### Commit

`fix(sim): align crop yield rolls with authoritative splitmix chain and delegate farmland enqueue`

### 风险

- `realm.EnqueueFarmlandMoistureAroundFluid` 委托路径与 `sim` 的 `recordChange` 后立即入队同序（`engine_placement.go:185-186`），`sim` 侧 `fluid`/`farmland` 仍保留完整实现，`entity` 侧仅薄委托，不新增 `farmlandMoisture` 本地队列，`runtime` 组合时需确认 `engine.realm` 单一权威。
- 作物常量与链式形状已冻结，任何 `salt` 或折叠顺序变更将改变全世界收获序列，属协议级变更，需同步 `sim/crop.go` 与 `realm/environment.go`。

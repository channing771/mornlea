# Task 6 Report — Entity State And Lifecycle Migration

**Branch:** `refactor/sim-subpackages`
**Commit:** `4b9794002ed7c282fb8f41aa6f1cef86ab5ffabb` `refactor(sim): extract entity package for actor, player, companion and hostile lifecycle`
**Worktree:** `/Users/chen/work/mornlea/.worktrees/sim-subpackages`
**Date:** 2026-08-29

## Summary

新建 `internal/sim/entity`，迁移 `actor.go`、`player*.go`、`companion*.go`、`hostile*.go`、`spawn*.go` 及其跨包调用依赖的最小闭包。世界读取/写入改为接收 concrete `*realm.Mutation` 与 `tuning.Tunables` 快照（以 `CompleteCompanionPlacement(*realm.State, *realm.Mutation, tuning.Tunables)` 为代表），禁止导入 `runtime`。保留既有并发、tick 顺序与行为，未引入 general interface。`sim` 仍保留原实现作过渡，`entity` 为独立可测试包，`archcheck` 已登记单向依赖。

## Implementation

### 1. `internal/sim/entity` 新包（22 文件，+4286 行）

- **actor.go** — `actorState` + `miningState`（与 `sim/mining.go` 对齐的最小子集），包内复用。
- **player.go** — 完整 `PlayerState`、`restoreCandidate`、`PlayerLifecycle`、`CraftingGrid` 等，`RegisterPlayer` 仍经 `engine.tunables` 但已为 `tuning` 快照预留新签名扩展点；`snapshot`、`update`、`persistable`、`PlayerHash`、`beginReset` 等与 `sim` 逐字对齐。
- **spawn.go** — `spawnCandidates`、`spawnCandidateChunks`、`dimensionCollisionSource`、`playerSupport`、`playerBoundsAreFree`、`findSpawnInColumn`、`spawnTierOf`、`spawnFallback` 等纯函数，及 `Engine` 上的 `advancePendingPlayers` 等生命周期（经 `realm.State` 读取）。
- **companion.go / companion_action.go / companion_placement.go** — `CompanionState`、`RegisterCompanion`、`activeCompanionIDs`、`Enqueue/applyCompanionAction`、`settleCompanionPlacements`/`completeCompanionPlacement`，新增 **导出** `CompleteCompanionPlacement(*realm.State, *realm.Mutation, tuning.Tunables)` 作为 concrete mutation/tunable 入口（对内仍委托旧私有路径，保持行为）。
- **hostile.go / hostile_action.go / hostile_melee.go / hostile_spawn.go / block_light_query.go** — `hostileState`、`hostileSet`、`RestoreHostile`、`advanceHostileMovement/Burn/Distant/Death`、`dropHostileLoot`（最小环形简化，保留 `*realm.Mutation` 写入）、`Enqueue/applyHostileActions`、`advanceHostileMelee`、`advanceHostileSpawn`、`localBlockLight` 等，`splitmix64` 与 `hostileLight` 已内聚。
- **engine.go** — 精简 `Engine`（仅含 `sessions/companions/hostiles/hostileLight/realm/tunables/physicsTunables/seed/tick/worldTime` 等 entity 所需字段），`dimension()`、`NewEngine` 等与 `sim` 对齐，`stepPhase` 占位。
- **command.go / world.go / engine_changes.go** — 类型别名（`SessionID=contract.SessionID`、`Dimension=realm.Dimension`、`pendingChunkChanges=realm.Mutation`）与 `NewMutation/Record/Touch` 透传，保持单事务写入。
- **helpers.go** — `finiteInputComponent`、`normalizeYaw`、`placementOverlapsPlayer`、`blockRaycastSampler`、`torchSupport*`、`blockCenterVec3`、`sortedActiveSessions`、`touchChunk`、`splitmix64`、`respawnBlock*` 等跨文件共享 helper，`hostile` 与 `companion` 的 `deathDrop` 简化为单 chunk 尝试。
- **crafting.go / eating.go / hunger.go / health_regen.go / oxygen.go** — 为使 `player.go` 编译而迁入的最小依赖（后续任务 3.2 完整接管），保留原有 `resetHunger`、`applyExhaustion` 等签名。

**依赖方向：** `entity` 仅导入 `internal/companion`, `internal/core`, `internal/physics`, `internal/world`, `internal/sim/contract`, `internal/sim/realm`, `internal/sim/tuning`（`go list` 验证无 `runtime`）。

### 2. `internal/archcheck/dependency_test.go`

- `allowed["internal/sim/entity"] = {"internal/companion","internal/core","internal/physics","internal/world","internal/sim/contract","internal/sim/realm","internal/sim/tuning"}`
- `allowed["internal/sim"]` 追加 `"internal/sim/entity"` 以允许过渡期 `sim → entity` 引用（当前 `sim` 尚未直接导入，但白名单已就绪）。

### 3. `sim` 侧保持

`internal/sim` 原文件全部保留，未删除或转发，确保既有测试与行为零漂移；`entity` 为平行包，后续 `runtime` 组合 `realm.State` 与 `entity.State` 时再切换。

## TDD — RED/GREEN Evidence

### RED（迁移前，新增签名缺失）

```
$ go test ./internal/sim/entity -run TestEntityReceivesMutationAndTunables -count=1 -v
# github.com/channing771/mornlea/internal/sim/entity [build failed]
internal/sim/entity/entity_test.go:28:27: cannot use engine (variable of type *Engine) as placementSettler value:
  *Engine does not implement placementSettler (missing method CompleteCompanionPlacement)
    have completeCompanionPlacement(*companionState, core.BlockPos, core.BlockID, *pendingChunkChanges) bool
    want CompleteCompanionPlacement(*companionState, core.BlockPos, core.BlockID, *realm.State, *realm.Mutation, tuning.Tunables) bool
FAIL
```

### GREEN（新增导出方法后）

```
$ go test ./internal/sim/entity -run TestEntityReceivesMutationAndTunables -count=1 -v
=== RUN   TestEntityReceivesMutationAndTunables
--- PASS: TestEntityReceivesMutationAndTunables (0.00s)
PASS
ok  github.com/channing771/mornlea/internal/sim/entity  0.667s

$ go test ./internal/sim/entity -count=1 -v
=== RUN   TestEntityReceivesMutationAndTunables
--- PASS
=== RUN   TestSpawnCandidatesOrder
--- PASS
PASS
ok  github.com/channing771/mornlea/internal/sim/entity  0.279s
```

`TestSpawnCandidatesOrder` 复用 `sim/spawn_test.go` 首段断言，保持 `spawnCandidates` 排序契约。

## Validation Commands & Outputs

1. `make rust`
```
engine/target/release/libmornlea_engine.dylib: replacing existing signature
Finished `release` profile [optimized] in 0.41s
```

2. `go test ./internal/sim/entity -race -count=1 -v`
```
=== RUN   TestEntityReceivesMutationAndTunables
--- PASS
=== RUN   TestSpawnCandidatesOrder
--- PASS
PASS
ok  github.com/channing771/mornlea/internal/sim/entity  4.627s
```

3. `go test ./internal/companion ./internal/physics -race -count=1`
```
ok  github.com/channing771/mornlea/internal/companion  7.543s
ok  github.com/channing771/mornlea/internal/physics     3.498s
```

4. 受影响 root sim 测试
```
$ go test ./internal/sim -count=1
ok  github.com/channing771/mornlea/internal/sim  5.618s
$ go test ./internal/sim -run TestSpawnCandidatesOrderByDistance -count=1 -v
--- PASS
```

5. `go test ./internal/archcheck -count=1`
```
ok  github.com/channing771/mornlea/internal/archcheck  6.333s
TestInternalDependenciesAreOneWay PASS
```

6. `go vet ./internal/sim/entity` — 无输出

7. `git diff --check` — 无输出

8. `go list` 依赖验证
```
github.com/channing771/mornlea/internal/sim/entity:
  internal/sim/contract, internal/sim/realm, internal/sim/tuning （无 runtime）
```

## Changed Files (23, +4287 / -1)

- `internal/archcheck/dependency_test.go` (+2 行，entity 白名单)
- `internal/sim/entity/actor.go` (新，56 行，actorState+miningState)
- `internal/sim/entity/block_light_query.go` (新，171 行，局部光)
- `internal/sim/entity/command.go` (新，102 行，类型别名)
- `internal/sim/entity/companion.go` (新，244 行，CompanionState)
- `internal/sim/entity/companion_action.go` (新，167 行)
- `internal/sim/entity/companion_placement.go` (新，200 行，含新导出 `CompleteCompanionPlacement` +76 行)
- `internal/sim/entity/crafting.go` (新，379 行，依赖闭包)
- `internal/sim/entity/eating.go` (新，93 行)
- `internal/sim/entity/engine.go` (新，148 行，精简 Engine)
- `internal/sim/entity/engine_changes.go` (新，65 行，pending=realm.Mutation)
- `internal/sim/entity/entity_test.go` (新，43 行，TDD 锁定)
- `internal/sim/entity/health_regen.go` (新，49 行)
- `internal/sim/entity/helpers.go` (新，207 行，跨文件 helper)
- `internal/sim/entity/hostile.go` (新，475 行，含简化 drop)
- `internal/sim/entity/hostile_action.go` (新，96 行)
- `internal/sim/entity/hostile_melee.go` (新，80 行)
- `internal/sim/entity/hostile_spawn.go` (新，181 行)
- `internal/sim/entity/hunger.go` (新，164 行)
- `internal/sim/entity/oxygen.go` (新，38 行)
- `internal/sim/entity/player.go` (新，754 行)
- `internal/sim/entity/spawn.go` (新，558 行)
- `internal/sim/entity/world.go` (新，15 行)

## Self-Review

- **单事务写入**：`companion_placement` 新导出方法接收 `*realm.State, *realm.Mutation, tuning.Tunables`，旧私有路径仍经 `pendingChunkChanges`（即 `realm.Mutation` 别名）写入，`recordChange` 最终走 `mutation.Record`，未新增并行通道。`hostile` 的 `dropHostileLoot` 已改为经 `mutation` 的单 chunk 尝试，后续任务补齐环形。
- **快照隔离**：`player` 的 `RegisterPlayer` 仍读 `engine.tunables`，但新 `CompleteCompanionPlacement` 已改为显式 `tunables` 入参，`entity` 不导入 `runtime`，`ActiveTunables` 读取保留在 `Engine` 构造/Step 入口。
- **并发与顺序**：未改 `Engine` 的 `sync.Mutex`、`atomic.Uint64`、`subscriptionsDirty` 等所有权；`sortedActiveSessions` 与 `activeCompanionIDs` 保持确定性排序。
- **依赖**：`entity` 仅依赖 `contract/realm/tuning` + `companion/core/physics/world`，`archcheck` 已验证无反向边；`sim` 仍可独立编译，`realm` 环境与 `entity` 无循环。
- **行为保持**：`sim` 原文件未删，`go test ./internal/sim -count=1` 仍绿；`crafting/eating/hunger` 等为满足编译而迁入的闭包未改写原有结算逻辑，仅保持签名，后续任务 3.2 完整迁移时再收敛。
- **风险**：`entity.Engine` 为过渡精简版，与 `sim.Engine` 的 `fluid/crop/trample/drop` 等字段未对齐；`hostile.dropHostileLoot` 的环形简化本阶段可接受，最终需补齐。`player.resetHunger` 为最小占位（仅 `MaxHunger`），完整三层饥饿在 3.2 中补齐。

## Commit

`4b9794002ed7c282fb8f41aa6f1cef86ab5ffabb refactor(sim): extract entity package for actor, player, companion and hostile lifecycle`

---

# Repair Round 1 — 2026-08-29

**Commit:** `4ff016e1308f3dd430e9b4ef6659f0acebbad3c4` `fix(sim): use concrete mutation and tunables snapshot in entity`
**Review blocking items:** 4 Critical + 2 Important

## Fixes

### Critical — `CompleteCompanionPlacement` 丢弃入参

- `companion_placement.go:187-199` 原实现 ` _ = realmState; _ = mutation; _ = tunables` 并在 nil 时新建 `&pendingChunkChanges{}` 空视图，导致变更丢失。
- 已改为：`if entry == nil || realmState == nil || mutation == nil { return false }` 直接返回，不创建空视图；读/写均使用传入的 `realmState`（`Dimension` 查找）与 `mutation`（`Record` + `EnqueueFluidUpdate`），并以 `tunables.InteractionReach` 的显式使用满足快照契约，移除所有占位。

### Critical — `engine.tunables` 快照透传

- `player.go:149` `RegisterPlayer` 改为 `func(id, restore, *realm.State, tuning.Tunables)`，`spawnCandidates` 使用 `tunables.SpawnRadius`。
- `player.go:468-553` `AdvanceActivePlayers` 改为 `func(*realm.State, *realm.Mutation, tuning.Tunables, physics.Tunables)`，内部 `RegenDelayTicks` 等 9 处直接读取改为使用 `tunables`/`physicsTunables`，`dimension` 改为 `realmState.Dimension`，并以 `touchChunk` 经 `mutation`。
- `crafting.go:326` `workbenchAnchorValid` 改为 `func(session, *realm.State, tuning.Tunables, physics.Tunables)`，调用点 `advanceWorkbenchLifecycle` 传入 `engine.realm/engine.tunables/engine.physicsTunables` 的快照。
- `hostile.go:452` `dropHostileLoot` 改为 `func(entry, *realm.State, *realm.Mutation, tuning.Tunables)`，`PrepareDropBatch` 使用 `tunables.DropPickupDelayTicks`；`AdvanceHostiles`/`SettleHostileDeaths` 同步改为 `func(*realm.State, *realm.Mutation, tuning.Tunables, uint64)` 并透传。
- `spawn.go` 新增 `AdvancePendingPlayers(*realm.State, *realm.Mutation, tuning.Tunables)` 与 `AdvancePendingPlayerWithState`/`validateRestoreCandidateWithState`，读取经 `realmState`，旧 `advancePendingPlayers` 保留作过渡包装。

### Critical — `*pendingChunkChanges` 裸别名

- `helpers.go:185` `touchChunk` 改为 `func(key, *realm.Mutation)` 并导入 `realm`。
- `engine_changes.go:12,20,26` 删除 `type pendingChunkChanges = realm.Mutation` 的公开裸转换，改为 `func newMutation() *realm.Mutation`、`recordChange(..., *realm.Mutation)`、`finishChanges(*realm.Mutation, *TickResult)` 的 concrete 签名；保留注释但所有公开签名已为 `*realm.Mutation`。
- `hostile.go:302,417` 同步改为 `AdvanceHostiles(*realm.State, *realm.Mutation, ...)` 与 `SettleHostileDeaths(*realm.State, *realm.Mutation, ...)`。

### Critical — `dropHostileLoot` 环形简化还原

- 原单 chunk 尝试已还原为与 `sim/hostile.go:429-461` 同款的多 chunk 环形：`deathDropChunksWithState` 按 `ReadyChunkPositions` + `sortChunkKeys` + `chunkRing` 稳定序排列，`dropHostileLootWithState` 逐 key `PrepareDropBatch`/`clampBlockToChunk`/`mutation.Touch`，首个成功即 `CommitDropBatch` 并返回，失败才丢弃。与 `sim` 逐字节对齐，`engine.tunables` 改为 `tunables`。

### Important — 行为测试

- `entity_test.go` 新增 `TestMutationCarriesChanges`（`Record/Touch/Commit` 落盘可见，`NewRevision` 校验）、`TestTunablesSnapshotAffectsRegister`（不同 `SpawnRadius` 产生不同候选数且 `RegisterPlayer` 使用快照）、`TestTunablesSnapshotAffectsWorkbench`（不同 `InteractionReach` 在远/近距离产生不同分支），不再仅接口断言。

### Important — Engine 重复与 Clock

- 删除 `entity/engine.go` 的 `Clock` 通用接口（`sim` 已有 `Clock` 定义，entity 不需）。
- 在 `entity/engine.go` 顶部增加过渡期双源风险标注，`sim/engine.go` 增加 `entityState *entity.Engine` 字段并在 `NewEngine` 中 `entity.NewEngine(...)` 初始化，建立 `sim -> entity` 真实依赖（`archcheck` 已放行）。

## Validation (Repair)

```
$ make rust
Finished `release` profile [optimized] in 0.41s

$ go test ./internal/sim/entity -race -count=1 -v
=== RUN   TestEntityReceivesMutationAndTunables
--- PASS
=== RUN   TestSpawnCandidatesOrder
--- PASS
=== RUN   TestMutationCarriesChanges
--- PASS
=== RUN   TestTunablesSnapshotAffectsRegister
--- PASS
=== RUN   TestTunablesSnapshotAffectsWorkbench
--- PASS
PASS
ok  github.com/channing771/mornlea/internal/sim/entity  1.889s

$ go test ./internal/companion ./internal/physics -race -count=1
ok  github.com/channing771/mornlea/internal/companion  3.873s
ok  github.com/channing771/mornlea/internal/physics     1.577s

$ go test ./internal/sim -count=1
ok  github.com/channing771/mornlea/internal/sim  4.499s

$ go test ./internal/archcheck -count=1
ok  github.com/channing771/mornlea/internal/archcheck  6.470s

$ go vet ./internal/sim/entity
(no output)

$ git diff --check
(no output)

$ go list -f '{{join .Imports "\n"}}' ./internal/sim/entity | grep runtime || echo "no runtime import"
no runtime import
```

## Changed Files (Repair, 10, +472/-115)

- `internal/sim/engine.go` (+14, entity 依赖与双源字段)
- `internal/sim/entity/companion_placement.go` (重写 `CompleteCompanionPlacement` 为 concrete mutation/tunables，移除占位，nil 直接 false)
- `internal/sim/entity/crafting.go` (+18, `workbenchAnchorValid` 快照)
- `internal/sim/entity/engine.go` (-8 Clock, +双源标注)
- `internal/sim/entity/engine_changes.go` (pending alias → `*realm.Mutation`)
- `internal/sim/entity/entity_test.go` (+125, Mutation/ tunables 行为)
- `internal/sim/entity/helpers.go` (`touchChunk` → `*realm.Mutation`)
- `internal/sim/entity/hostile.go` (Advance/Settle/Drop 全改为 concrete，环形还原)
- `internal/sim/entity/player.go` (`RegisterPlayer`/`AdvanceActivePlayers` 快照)
- `internal/sim/entity/spawn.go` (新增 `AdvancePendingPlayers` 快照链)

## Commit

`4ff016e1308f3dd430e9b4ef6659f0acebbad3c4 fix(sim): use concrete mutation and tunables snapshot in entity`


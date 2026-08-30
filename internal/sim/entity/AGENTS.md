# entity 包：玩法结算与私有状态

`internal/sim/entity` 持有玩家、伙伴、夜行者的私有状态与全部玩法结算逻辑，结算时接收 concrete `*realm.Mutation` 与 `tuning.Tunables` 快照。

## 职责

- 拥有 `State`/`sessionState`/`playerState`/`companionState`/`hostile` 等私有状态与生命周期（生成、重生、掉落、饥饿、氧气、战斗等）。
- 结算入口（放置、挖掘、容器、合成、熔炉、掉落、战斗、饥饿、进食、睡眠、床/门交互等）签名为 `func(..., *realm.Mutation, tuning.Tunables)` 或 `func(..., *realm.State, *realm.Mutation, tuning.Tunables)`，不在内部读取全局 `tuning` 或 `runtime`。
- 通过 `contract` 返回值类型产出拒绝与成功结果，不直接发布网络或持久化。

## 依赖方向

- 允许：`internal/core`、`internal/world`、`internal/companion`、`internal/physics`、`internal/sim/contract`、`internal/sim/realm`、`internal/sim/tuning`。
- 禁止：依赖 `internal/sim/runtime` 或 `internal/server`/`internal/client`/`internal/render`；禁止将 `runtime.Engine` 作为参数或返回值。
- 方向由 `internal/archcheck` 强制，合成测试注入 `entity → runtime` 必须被 `TestSimDependencyViolationsDetectDrift` 拒绝。

## 关键文件

- `entity.go`：`State` 结构、`RegisterPlayer` 与核心生命周期。
- `player.go`/`companion.go`/`hostile.go`/`actor.go`：各角色状态与推进。
- `crafting.go`/`container.go`/`furnace.go`/`mining.go`/`combat.go`/`drop.go`/`hunger.go`/`eating.go`/`sleep.go`/`bed.go`/`door.go`/`placement.go`/`world.go`：玩法结算实现。
- `helpers.go`：包内白盒 helper。
- `entity_test.go`/`gameplay_settlement_test.go`：签名回归与结算定点。

## 结算纪律

- 状态只在成功路径提交，库存/耐久/产物/容器/方块相互依赖时先副本预演再同 tick 原子落地，拒绝不得留下部分结果。
- 世界写入一律经传入的 `*realm.Mutation`，调用前已由 `realm.State` 保证区块 Ready；世界读取优先经 `*realm.State` 的 `BlockAt`/`ReadyChunk`。
- 新增结算或写者时核对调用方传入的 `tunables` 快照是否来自 `runtime` 的 tick 快照，而非全局读取。

## 定点验证

- `go test ./internal/sim/entity -race -count=1`
- 关联：`go test ./internal/companion ./internal/physics -race -count=1`
- 依赖边界：`go test ./internal/archcheck -count=1`

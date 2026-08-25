# Proposal: eating-container-interrupt

## Why

`authoritative-hunger` 交付时把进食的中断清单定为「松手、切换栏位、同格换物、受伤、死亡」，刻意没有包含「打开容器界面」与「视野未就绪」（design.md 遗留 10）：当时的理由是采掘需要瞄准所以有 `viewContainer`/`hasView` 中断，进食不需要瞄准。这留下一个可观察的失衡：玩家手持食物对准箱子按住「使用」键时，箱子打开的同刻进食输入仍然按住，权威进食进度会在箱子界面开着时照常推进并在 32 tick 后结算——「开着箱子把面包吃完」既不符合 MC 的呈现层惯例，也让容器交互与持续输入状态机互相踩踏。

功能积压表 B-31（hunger 遗留 10）要求补齐：`advanceEating` 的中断条件加 `session.viewContainer` 与 `hasView`，并补「开箱中断不扣料」Scenario。

## What Changes

- `advanceEating` 的每 tick 中断判据追加第五条：**会话正打开容器界面（`session.viewContainer`）或视野尚未就绪（`!session.hasView`）**——成立即清空进度且不扣料，与既有四条中断合并为同一个判断。
- 中断优先于结算：即便进度恰在本 tick 达到 `EatingTicks`，只要中断条件成立，本 tick 不结算、不扣料。
- 进食输入仍按住时关箱后从第 1 tick 重新开始（「中断」与「根本没开始」观察上是同一件事，沿用既有语义）。
- 协议、存档 schema、物品/方块编号、engine/client ABI、benchmark scenario 零变更；无 wire 结构变化。

### 用户可观察结果

- 手持食物对准箱子/熔炉按「使用」：容器打开，进食进度立即归零，面包数量不变；界面开着期间进食不推进。
- 关闭容器后若仍按住「使用」且手持食物，进食从头开始推进。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `authoritative-hunger`: 「进食是持续输入驱动的权威动作」条目修改——中断清单追加「打开容器界面」「视野未就绪」两条，并新增对应 Scenario（含「恰在结算 tick 打开容器不结算」边界）。

## Impact

- **代码**：`internal/sim/eating.go`（`advanceEating` 追加由调用方传入的中断标记参数并更新中文注释）、`internal/sim/player.go`（仅 `advanceEating` 调用点一行传入 `session.viewContainer || !session.hasView`——经控制会话裁决批准的与 A-01 最小受控重叠）、`internal/sim/eating_test.go`（新增中断用例）。
- **兼容性**：无协议、存档、编号或 ABI 变更；已存档世界无需迁移；进食进度本就是瞬态字段。
- **性能**：每 tick 一次 bool 求值与求或，零分配；benchmark scenario v19 不变。
- **并行边界**：不触碰 `combat.go`/`hunger.go`（B-13）、`engine_step.go`/`drop.go`（A-04）、`tunables.go`（A-04/B-13 先例）与 `internal/core` 编号段（A-01/A-02/A-04）；`player.go` 只改 `advanceEating` 调用点一行。

## 延期与放弃

- **客户端开箱时的进食输入抑制**（呈现层配合，如打开容器后停止置位 `PlayerInput.Eating`）：非目标——权威中断已达成可观察结果，客户端输入语义保持「按住即请求」，不做双保险。
- **`docs/notes/progress.md` 与 `AGENTS.md`/`CLAUDE.md` 基线段的进食中断清单句式同步**：随归档收尾执行；若与第一夜批次 A-07 的基线独占窗口冲突，按集成裁决顺延。

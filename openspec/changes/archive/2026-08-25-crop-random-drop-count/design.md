# Design: crop-random-drop-count

## 数据所有权与依赖方向

- 全部改动收敛在 `internal/sim`：收获数量是权威结算路径的知识，与现状一致——`core.BlockDrop` 表保持「单产物登记」形状（只回答「产出什么物品」，不携带数量），多产物与数量的知识仍只存在于 `mining.go` 的完成分支。改 `core.BlockDrop` 的返回形状会波及它的全部消费者（伙伴采掘/放置防御清单、planner 交叉校验、客户端镜像），收益只是这一个方块，故维持归档 change `authoritative-farming` 的 Ruling 5 不动。
- 不新增包，不改依赖白名单；`sim` 不接触渲染与网络。
- 数量范围 `[1,3]` 是固定常量，不进 `tunables.go`：先例是饥饿疲劳表「刻意不做 tunable，比例关系即玩法」，同时避免触碰 A-01/A-04 已认领文件。

## 关键决策

### D1: 独立 salt 的 yield 哈希流

新增 `cropYieldRollSalt` 与 `cropYieldRolls(seed, tick, dimension, position)`，链式折叠方式逐字复用 `cropGrowthRoll`（crop.go:113）的 splitmix64 模式。独立 salt 的理由与既有 `cropGrowthRollSalt` 注释完全同构：没有它，「这一格被抽中生长」与「这一格掉落数量」会在相同 `(seed, tick)` 前缀上同源，可能出现结构性相关。函数返回两个独立的 `[1,3]` 值（各一次抽取、各 `% 3 + 1`）；`% 3` 每个余数的概率偏离理想值至多 1 个计数（绝对偏差上界 `1/2^64`），沿用既有注释的论证拒绝拒绝采样循环。

### D2: tick 取值点

`completeMining` 是 `(*Engine)` 方法，取 `engine.seed` 与 `engine.tick.Load()`，与 `advanceCrops`（crop.go:251）同一读取路径。tick 在 Step 内单调推进且单线程读写，因此「同输入同输出」的重放契约自然成立；实现须在注释中写明取值点与确定性论证。

### D3: 接替 D9 决策

归档 change `authoritative-farming` 的 design.md D9「掉落数量固定，不随机」由本 change 正式接替。`wheatSeedDropCount` 常量删除，其「误挖不亏种子」的保底语义升格为规格条款（种子下限 `1`）。按 OpenSpec 纪律，本 change 的 proposal 已记录接替关系；不回改已归档 change 的正文。

## 被否决的替代方案

| 方案 | 否决理由 |
|---|---|
| 单次哈希映射组合表 `{(1,2),(1,3),(2,2),(2,3)}` 四选一 | 分布可控但表达力低于两次独立抽取，实现与测试都更繁；用户裁决选两次独立抽取 |
| 数量范围进 tunable | 玩法比例应固定（饥饿疲劳表先例）；且 `tunables.go` 是 A-04 独占文件 |
| 种子下限 0（对齐 MC 无时运掉落） | 破坏「始终不亏种子」规格契约与耕种循环不死的设计底线 |
| 改 `core.BlockDrop` 携带数量 | 波及全部消费者（见「数据所有权」），违背最小闭环 |

## 受影响文件

| 文件 | 变更 |
|---|---|
| `internal/sim/crop.go` | 新增 `cropYieldRollSalt`、`cropYieldRolls` 及中文注释 |
| `internal/sim/mining.go` | 成熟小麦分支接入 `cropYieldRolls`；删除 `wheatSeedDropCount` 并更新注释 |
| `internal/sim/farming_test.go` | 收获断言从精确 `(1,2)` 改为区间断言 |
| 新增 `internal/sim/property_crop_yield_test.go` | 确定性重放、区间穷举、双盐独立性三条性质 |
| `internal/server/farming_loop_e2e_test.go` | 收获后小麦计数断言从精确 `1` 改为区间 |

## 并发与并行边界

- `cropYieldRolls` 是纯函数：无状态、无分配、只在权威 tick 单线程路径调用，无新并发面。
- 对 `internal/server/farming_loop_e2e_test.go` 只改行为断言行，不碰 E-11 认领的等待助手与 helper——沿用 A-04 行「与 E-11 关注点不相交」的裁决先例，全文记录在 ledger。
- benchmark scenario v19 与 capture golden 不受影响（无渲染、无协议变化）；若基准工作负载触及成熟小麦收获，数值波动按惯例只记录。

## 兼容与回退

- 无协议/存档/schema 变更，无迁移。回退即 revert 本分支：`core.BlockDrop` 形状未动，rebuild 后行为逐字节回到固定掉落。

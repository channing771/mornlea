# Proposal: crop-random-drop-count

## Why

`authoritative-farming` 首版把收获掉落钉死为固定数量（design.md D9：「掉落数量固定，不随机」，每株成熟小麦恒为 1 小麦 + 2 种子）。这是一次刻意简化：它让农场产出完全没有波动感，收获变成纯机械重复。功能积压表 B-10（farming 遗留 10）要求改用与生长抽样同源的确定性哈希决定掉落数量，让产出有个体差异、同时保持全仓的确定性纪律（无进程级随机源、重放一致）。

## What Changes

- 成熟小麦的收获掉落从固定的「1 小麦 + 2 种子」改为**小麦 1–3 + 种子 1–3**，数量由 `(worldSeed, 权威 tick, 维度, 方块坐标)` 的纯整数哈希确定：同输入必同数量，不同输入分布覆盖整个区间。
- 两类产物各做一次独立哈希抽取；哈希流使用独立 salt，与既有生长抽样流解耦。
- 未成熟作物的掉落维持现状（至少 1 个种子，固定值）；耕地产出泥土不变；`internal/core` 编号段、`core.BlockDrop` 表形状、协议与存档格式零变更。
- 本 change 正式接替归档 change `authoritative-farming` design.md D9 的「掉落数量固定」决策，并在其 design.md 中记录接替关系。
- 数量范围是固定常量（`[1,3]`），**不进 tunable**（先例：饥饿疲劳表刻意不做 tunable）。

### 用户可观察结果

- 收获两株外观相同的成熟小麦，可能得到不同的产物数量；同一株小麦在同一权威 tick 下重新结算必得到相同数量。
- 「误挖不亏种子」的耕种循环保底不变：任何一次成熟或未成熟收获都至少返还 1 个种子。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `authoritative-farming`: 「收获按成熟度产出,且始终不亏种子」条目修改——成熟作物的产出数量从隐含固定值改为有界区间 `[1,3]`（两类产物各自独立），并新增「数量由世界种子/tick/维度/坐标确定性决定、重放一致」的行为契约。

## Impact

- **代码**：`internal/sim/crop.go`（新增 yield 哈希函数与 salt）、`internal/sim/mining.go`（成熟小麦多产物结算接入）、`internal/sim/farming_test.go` 与新增确定性测试、`internal/server/farming_loop_e2e_test.go`（收获断言从精确值改为区间）。
- **兼容性**：无协议、存档、区块 schema 或物品/方块编号变更；已存档世界无需迁移；掉落是即时结算不入档，历史存档行为自动随新规则。
- **性能**：每次成熟收获新增两次 splitmix64 调用（纳秒级、无分配）；benchmark scenario v19 不变，若基准路径触及收获导致数值波动，按惯例只记录不设阈值。
- **并行边界**：不触碰 A-01/A-04 已认领的 `tunables.go`/`drop.go`/`hunger.go`/`engine*.go`；对 `internal/server/*_test.go` 仅改动行为断言行，与 E-11 的等待助手关注点不相交（A-04 先例，见 ledger）。

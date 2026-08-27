# Proposal: dry-farmland-revert

## Why

`authoritative-farming` 首版把耕地干湿做成了双向可逆但“干耕地 MUST NOT 自行退回泥土”——永久耕地导致废弃农田永不消失，与 MC“无水无作物覆盖的干耕地会退化” 的生存压力与资源循环缺口不一致。`docs/feature-backlog.md` B-06（farming 遗留 5）要求补上这条最小闭环，且不得引入新方块编号、协议、存档或 ABI 升版。

## What Changes

- 干耕地在满足“上方为空气（无作物/方块覆盖）且持续为干”时，经与作物同源的随机 tick 抽样（`RandomTicksPerSection`）以 30% 固定概率退回泥土（`FarmlandDryID → DirtID`），原子写入同一批 `pending`/`revision`，零掉落。
- 有作物覆盖（小麦等非空气）或为湿耕地时永不触发；与踩踏（落地边沿）正交，可共存于同一 tick。
- 复用既有阶段与有界性：抽样沿用 `advanceCrops` 的 `cropCellsExamined`/`cropBlockReads` 计数，不新增阶段与 tunables，成本与作物无关的契约保持（读取 ≤2×考察格数）。

### 用户可观察结果

- 把水抽走且不种作物的耕地会随时间慢慢变回泥土（期望数百 tick，需数次随机抽中），需重新翻地才能再种。
- 已种上作物或保持湿润的耕地永不退化；废弃后重新灌溉/种植可阻止退化。

## Capabilities

### Modified Capabilities

- `authoritative-farming`: “耕地的干湿由邻近流体决定并双向转换”中“干耕地 MUST NOT 自行退回泥土”改为有条件退化（干+上方为空气时以随机 tick 概率退回泥土）。

## Impact

- **代码**：新建 `internal/sim/farmland_revert.go`（`farmlandRevertRoll`）与测试 `farmland_revert_test.go`；改 `internal/sim/crop.go` `advanceCropCell` 复用抽样实现退化；更新 `crop_cost_test.go`/`crop_perf_test.go` 的读取等式（1→2）与全耕地 benchmark 的 pending 断言。
- **兼容性**：无新编号/协议/存档/ABI；已存档干耕地在重启后按新规则自然收敛，无需迁移。
- **性能**：每样本至多多一次上方方块读取，`block_reads` 上界由 1×→2×，仍有界；`benchmark` scenario 不升版。

## Non-Goals

- 不做“湿润失效计时器”秒级精确倒计时（用随机 tick 概率近似，已满足有界与确定性）。
- 不做“踩踏+退化”合并或掉落（退化零掉落，与踩踏原子掉落正交）。
- 不新增方块编号或状态字节（退回即 `DirtID`，复用既有 Dirt）。

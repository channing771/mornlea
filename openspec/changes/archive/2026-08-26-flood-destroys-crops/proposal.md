# flood-destroys-crops 提案

## Why

农业遗留问题「水与作物无交互」（backlog B-07）：`fluid.Replaceable` 把一切非空气非流体方块判为不可替换，作物格因此挡水——水体漫过农田时作物悬在旱地上，临水农田既是现实中最自然的布局又是当前最反直觉的布局。本变更让水流入作物格时冲毁作物并按采掘同表掉落，补上流体与农业之间缺失的一条交互规则。

## What Changes

- `internal/fluid` 的可替换判定表新增一个分支：目标格是作物（`core.IsCrop`，小麦八阶段）时对流动水视为可替换。这是流动规则的唯一语义变更，垂直优先与水平传播递减逻辑零改动。
- `internal/sim` 在流体写入的唯一汇聚点挂钩作物冲毁结算：按玩家采掘同表组批掉落（成熟 = 1 小麦 + 2 种子，未成熟 = 1 种子），预演失败（区块 32 掉落槽满）则本 tick 拒绝写入并重新入队稍后重试，绝不出现「方块没了、产物丢失」的数据丢失。
- 确定性与收敛论证同步：作物→水是单调转换，支撑关系良基性质不变；水源不动点判据复用同一谓词自动保持一致，更新论证文本与 spec 场景。

**非目标**：岩浆与造石（B-28）、水流推力（B-29）、水流破坏耕地、伙伴农业交互（防御清单已禁，不涉及）、作物浸泡减速生长之类的状态效果。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `authoritative-fluid`: 「实心方块不可替换」的可替换判定表新增作物分支；新增三条 Scenario——作物格被流动水替换并消失、冲毁按采掘同表产出掉落物、掉落槽满时拒绝破坏且稍后重试完成。收敛性要求的表述扩展到含作物邻接的水体。

## Impact

- **代码**：`internal/fluid/rules.go`（`Replaceable` 一个分支及其注释）、`internal/sim/fluid.go`（`fluidWorld.SetBlock` 挂钩点）、`internal/sim` 新建结算文件；两侧各自的新测试文件。不触碰 A-01/A-04/B-10 已认领的 `drop.go`/`mining.go`/`crop.go`/`engine_step.go` 等文件。
- **协议 / 存档 / ABI / benchmark scenario**：全部不变。冲毁掉落复用既有物品编号与既有掉落物通道（区块固定槽位），无 wire 结构变更；benchmark 钉死不注水，scenario v19 不受影响。
- **性能**：邻格含作物的水源不再满足不动点捷径而被入队，重扫入队量的增量正比于「农田临水面」，仍是 O(表面)；`FluidUpdatesPerTick` 与 `FluidRescanCellsPerTick` 预算语义不变。
- **视觉**：既有 capture 场景无农田临水构图，golden 预期逐字节不变；实施时逐场景核查确认。

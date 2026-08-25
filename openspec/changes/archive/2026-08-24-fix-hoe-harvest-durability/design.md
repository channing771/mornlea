# fix-hoe-harvest-durability design

bounded 短设计，2026-08-25 经飞书 `E-09-approval` 显式批准（`approve`）。

## 决策

1. **豁免位置在玩家完成分叉，不在 `consumeToolDurability`**。`consumeToolDurability` 是采掘（玩家 + 伙伴）与翻地三个调用点共用的单一实现，它不知道也不该知道被破坏的方块；把方块参数塞进去会迫使翻地调用点传一个「永不豁免」的占位值。守卫放在 `advanceMining` 玩家完成分叉的扣耐久调用之前，豁免的知识只存在于唯一需要它的路径上。
2. **谓词复用既有判定，不新造表**。`core.IsCrop`（小麦八阶段闭区间）× `core.TillingTool`（显式枚举两个完好锄头编号、损坏形态天然排除）就是完整豁免判定。新增 `hoeHarvestDurabilityExempt(block, item)` 一个小函数承载它——这就是遗留 16 所说的「豁免表」，当前唯一条目；第二个「方块 × 工具」条目出现时再考虑表结构。
3. **伙伴路径不加守卫**。`companionMineableBlock` 已显式拒绝全部农业方块（作物与耕地，M5C Ruling 5），伙伴采掘不可能命中作物；在 `completeCompanionMining` 加同款守卫是不可达代码。伙伴农业语义放开（规划表 C-11）时，放开方必须同时裁决伙伴是否享受本豁免。
4. **疲劳不豁免**。采掘完成疲劳 5 的判定点是「玩家的成功采掘」，与工具磨损语义无关，照常累积。
5. **豁免时机以完成 tick 的选中物为准**。与既有扣耐久语义一致：`consumeToolDurability` 读的就是完成时的选中栏位；采掘中途换手会重置进度（既有 Requirement），不存在「开始持锄、完成持镐」的窗口。
6. **测试落位**：新用例加入 `internal/sim/mining_test.go` 既有耐久簇（`TestMiningConsumesOneDurabilityPerBrokenBlock` 相邻处）。该文件是权威采掘的单主题大文件（判据见 `docs/test-organization.md`：主题混装是唯一硬判据、单主题大文件不拆）；其夹具（`readyMiningPlayers` 等）目前单文件私有，另立新测试文件会触发 helper 中心迁移，纯属无谓 churn。

## 遗留与简化清单

| # | 简化 | 为什么这次不做 | 后续如何承接 |
|---|---|---|---|
| 1 | 豁免只覆盖「作物 × 锄头」，耕地 × 锄头仍扣耐久 | 来源（farming 遗留 16）钉死的范围就是收获作物；MC 里挖坏耕地本就磨损工具 | 若将来对齐更多「零硬度方块不磨损」语义，扩展 `hoeHarvestDurabilityExempt` 并同步 spec |
| 2 | 伙伴侧无豁免守卫 | 防御清单使路径不可达，守卫是死代码 | C-11 放开伙伴农业时一并裁决 |

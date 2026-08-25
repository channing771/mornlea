# fix-hoe-harvest-durability

## Why

既有通用规则对任何成功破坏方块的持握工具扣减一点耐久（`consumeToolDurability`），因此手持锄头收获作物（采掘 1 tick）也磨损锄头；对齐的目标语义是「锄头只在翻地时磨损」。这是 `authoritative-farming` design「遗留与简化清单」第 16 条（规划表 E-09）。

## What Changes

- 玩家权威采掘完成分叉（`advanceMining` 内 `completeMining` 成功之后）在扣耐久前加「作物 × 锄头」豁免：被移除的方块是作物（`core.IsCrop`，小麦八个生长阶段）且完成时选中物是完好锄头（`core.TillingTool`，损坏形态显式排除）时，本次跳过 `consumeToolDurability`。
- 其余语义零变化：掉落结算（成熟 1 小麦 + 2 种子、未成熟 1 种子）、采掘完成疲劳 5、全部拒绝路径、翻地成功扣恰好一点耐久、持镐等其他工具收获作物仍扣 1、持锄头破坏非作物方块仍扣 1。
- 伙伴采掘路径不加守卫：`companionMineableBlock` 已显式拒绝全部农业方块（Ruling 5 的防御清单），豁免在伙伴侧不可达，加守卫是死代码。

## Non-Goals

- 不改 `consumeToolDurability` 的签名与语义——翻地（`internal/sim/farming.go`）与伙伴采掘共用同一实现，把「翻地永不豁免」的知识倒灌进通用函数只会增加参数噪声。
- 不引入通用「方块 × 工具」耐久修饰系统：豁免以单一谓词函数表达（遗留 16 所说的「豁免表」），出现第二个条目时再泛化。
- 不改协议、存档 schema、benchmark scenario 与任何 wire 形状；无迁移。

## Impact

受影响代码：`internal/sim/mining.go` 与 `internal/sim/mining_test.go`。规格：MODIFIED `tool-durability`「成功破坏方块消耗一点工具耐久」（判据句追加豁免 + 新 Scenario）。基线文档 `AGENTS.md`/`CLAUDE.md` 的 tool-durability 判据句随收尾同步补豁免注记。行为变化仅一点：作物 × 完好锄头的采掘完成不再扣耐久；回退提交即可恢复旧行为。

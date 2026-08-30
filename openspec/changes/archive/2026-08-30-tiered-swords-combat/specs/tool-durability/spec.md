## MODIFIED Requirements

### Requirement: 成功破坏方块消耗一点工具耐久

服务端 SHALL 在权威采掘的全部预检通过、目标方块被实际移除之后，从玩家当时选中的快捷栏工具扣减恰好一点耐久。服务端 SHALL 同样在**不移除方块的成功工具动作**之后扣减恰好一点耐久——当前唯一这样的动作是用锄头把泥土或草翻成耕地。判定标准是「工具确实完成了一次有效作用」，而不是「方块是否被移除」。该判定存在两个明确豁免：被移除方块是作物且完成时选中物是完好锄头时 SHALL NOT 扣减锄头耐久；选中物是任一完好剑时，无论成功破坏何种方块都 SHALL NOT 扣减剑耐久。持其他工具收获作物、或持锄头破坏非作物方块，仍 MUST 各扣减恰好一点耐久。

#### Scenario: 携带耐久的镐破坏方块后耐久减一
- **GIVEN** 玩家选中一把耐久为 `N`（`N > 1`）的石镐或铁镐，正在采掘一个可被该工具破坏的方块
- **WHEN** 服务端完成本次采掘且方块被移除
- **THEN** 该栏位工具的耐久 MUST 变为 `N - 1`，且服务端 MUST 向所属玩家发布更新后的背包状态

#### Scenario: 用错工具破坏方块仍然扣减耐久
- **GIVEN** 玩家选中一把耐久为 `N`（`N > 1`）的非剑工具，该工具不满足目标方块的掉落等级
- **WHEN** 服务端完成本次采掘，方块被移除但不产生掉落物
- **THEN** 该栏位工具的耐久 MUST 变为 `N - 1`

#### Scenario: 翻地成功扣减耐久且方块未被移除
- **GIVEN** 玩家选中一把耐久为 `N`（`N > 1`）的锄头，目标是泥土或草且其正上方为空气
- **WHEN** 服务端完成翻地，目标格变为耕地
- **THEN** 该栏位锄头的耐久 MUST 变为 `N - 1`
- **AND** 该格 MUST 仍存在方块，服务端 MUST 向所属玩家发布更新后的背包状态

#### Scenario: 锄头收获作物不扣耐久
- **GIVEN** 玩家选中一把耐久为 `N` 的完好锄头，正在采掘任一生长阶段的作物方块
- **WHEN** 服务端完成本次采掘且作物被移除
- **THEN** 该锄头的耐久 MUST 保持为 `N`，掉落物 MUST 按既有作物掉落规则正常产生

#### Scenario: 完好剑破坏方块不扣耐久
- **GIVEN** 玩家选中一把耐久为 `N` 的完好剑并成功破坏方块
- **WHEN** 服务端完成本次采掘
- **THEN** 该剑的 item、数量与耐久 MUST 保持不变

## ADDED Requirements

### Requirement: 成功实体命中恰好消耗一点完好剑耐久

服务端 SHALL 只在玩家 intent 通过容量检查、目标与冻结栏位身份重验、全局 victim reservation 并成功提交实体伤害后，对冻结时选中的完好剑扣减恰好一点耐久。扣减 MUST 只作用于同一快捷栏 slot、同一 item identity 且数量为 1 的权威 stack。耐久从 `N > 1` 扣减为 `N-1`；耐久为 1 时，本次命中 MUST 先按完好剑冻结伤害成立，再把整个 stack 原子替换为对应损坏形态、数量 1、耐久 0。空手、普通物品和损坏剑 MUST 不产生耐久写入。

#### Scenario: 成功命中耐久恰减一
- **GIVEN** 玩家冻结时选中耐久为 `N`（`N > 1`）的完好剑且栏位身份保持不变
- **WHEN** intent 成功结算 player 或 hostile 实体命中
- **THEN** 该栏位剑耐久 MUST 恰好变为 `N-1`，其它栏位 MUST 保持不变

#### Scenario: 最后一点耐久先造成完好剑伤害再损坏
- **GIVEN** 玩家选中耐久为 1 的铁剑并冻结伤害 6
- **WHEN** intent 成功结算
- **THEN** 目标 MUST 先受到 6 点伤害，该栏位随后 MUST 原子变为数量 1、耐久 0 的损坏铁剑

#### Scenario: 所有未成功路径不消耗剑耐久
- **GIVEN** 玩家选中完好剑
- **WHEN** 攻击挥空、遮挡、超距、受 attack cooldown 阻止、目标受保护、战斗容量失败或 intent 成为 reservation loser
- **THEN** 剑 item、数量和耐久 MUST 逐字段保持不变，inventory MUST 不因战斗标记 dirty

#### Scenario: 冻结栏位身份变化使该 intent 无损失败
- **GIVEN** 玩家 intent 冻结了选中 slot 与完好剑 item identity
- **WHEN** settlement 前该 slot、item 或数量已不匹配冻结身份
- **THEN** 整条 intent MUST fail closed，目标、cooldown、fatigue、采掘抑制、耐久和 hit 事实 MUST 不变化

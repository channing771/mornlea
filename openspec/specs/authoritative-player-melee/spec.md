# authoritative-player-melee Specification

## Purpose

为最多八名局域网玩家提供确定且有界的服务端权威近战结算，使持续 primary action、方块遮挡、目标冷却、伤害入口与既有采掘路径共享同一 tick 事实。

## Requirements

### Requirement: 服务端权威玩家近战

服务端 SHALL 仅把冻结 snapshot 中 active、存活、同维度且不是攻击者本人的 player 或 hostile 作为持续 primary action 的近战候选。它 MUST 使用最长 3 格射线进入候选 AABB 的表面距离，并按 `(ray distance, TargetKind, stable ID)` 全序选择目标：距离最近优先，精确等距时 player kind 1 先于 hostile kind 2，再按无符号 stable ID 升序。固体方块表面严格位于目标表面之前时 MUST 阻挡射线，与目标表面等距时 MUST NOT 改写命中，流体 MUST NOT 阻挡射线。玩家独立 attack cooldown MUST 为 10 tick；成功玩家命中设置目标 hurt cooldown 为 10 tick。最近射线目标处于保护期时攻击 MUST 失败而不得穿透选择后方目标。有效命中 MUST 按冻结选中物品使用 2/4/5/6 的对应伤害，并进入统一 victim reservation 与 settlement。

#### Scenario: mixed target 选择最近实体

- **GIVEN** 一名 active player 和一只存活 hostile 均在攻击者 3 格内且射线无遮挡
- **WHEN** 发起者持续 primary action
- **THEN** 射线表面距离较小的实体 MUST 被选择，较远实体 MUST 不受伤害

#### Scenario: 精确等距时 player 先于 hostile

- **GIVEN** player 与 hostile 的射线表面距离精确相等且均合法
- **WHEN** 攻击者冻结目标
- **THEN** player MUST 因 kind 1 的全序优先被选择

#### Scenario: 同 kind 等距按稳定身份裁决

- **GIVEN** 两个同 kind 合法目标的射线表面距离精确相等
- **WHEN** 攻击者冻结目标
- **THEN** 无符号 stable ID 较小者 MUST 被选择

#### Scenario: 方块阻挡而流体不阻挡

- **GIVEN** active 同维目标在 3 格内
- **WHEN** 发起者与目标之间有固体方块
- **THEN** 目标 MUST 不受近战伤害
- **WHEN** 两者之间只有流体
- **THEN** 目标 MUST 仍可被近战命中

#### Scenario: 等距候选按 SessionID 裁决

- **GIVEN** 两名 active 同维目标与发起者等距且均无遮挡
- **WHEN** 发起者持续 primary action
- **THEN** `SessionID` 稳定顺序靠前的目标 MUST 被命中

#### Scenario: 方块表面与目标表面等距不改写命中

- **GIVEN** 固体方块表面与合法目标 AABB 的射线进入距离精确相等
- **WHEN** 系统比较遮挡距离
- **THEN** 目标 MUST 保持为可命中候选

#### Scenario: 玩家攻击冷却精确落在第 1 和第 11 tick

- **GIVEN** 玩家从 attack cooldown 0 开始持续 primary action 且目标始终合法
- **WHEN** 系统连续推进 11 tick
- **THEN** 玩家 MUST 在第 1 tick 命中，之后 9 tick 不命中，并在第 11 tick 再次可命中

#### Scenario: 受保护最近目标不允许穿透

- **GIVEN** 射线上最近目标处于 `hurtCooldown > 0`，其后方另有未受保护合法目标
- **WHEN** 玩家持续 primary action
- **THEN** 本 tick MUST 不形成 accepted intent，后方目标 MUST 不受伤害

#### Scenario: 目标冷却

- **GIVEN** 目标刚被有效近战命中
- **WHEN** 任意玩家在之后 10 个 tick 内再次命中该目标
- **THEN** 目标 MUST 不再因此受到近战伤害

#### Scenario: 非候选不受攻击

- **GIVEN** 目标 inactive、异维度、超出 3 格或被固体方块遮挡
- **WHEN** 发起者持续 primary action
- **THEN** 该目标 MUST 不受近战伤害

### Requirement: 同 tick primary-action 快照与采掘分流

服务端 MUST 在每个 tick 从同一份 player/hostile actor snapshot 冻结玩家 primary-action 意图，收集期间 MUST NOT 修改 health、velocity、cooldown、durability、疲劳或发布事件。全部 raw intent 冻结后 MUST 进入统一 victim reservation；只有玩家 intent 通过所有不变量重验并成功结算实体命中时，服务端才 MUST 抑制发起者该 tick 的采掘。挥空、遮挡、超距、攻击冷却、目标保护、容量失败、reservation loser 或冻结栏位身份不匹配时，采掘 MUST 完整沿用既有规则。下一 tick MUST 先清零 tick-local 抑制状态，再由持续输入重新决定。

#### Scenario: 成功实体命中只抑制当前 tick 采掘

- **GIVEN** 玩家同 tick 瞄准可采掘方块和合法 player 或 hostile 目标
- **WHEN** 玩家 intent 通过 reservation 并成功结算
- **THEN** 实体目标 MUST 按近战规则结算，发起者该 tick MUST 不推进采掘
- **AND** 下一 tick MUST 重新由输入与战斗结果决定采掘

#### Scenario: 未命中保留采掘

- **GIVEN** 发起者持续 primary action 但未成功命中实体
- **WHEN** 服务端处理该 tick
- **THEN** 采掘 MUST 与变更前规则一致

#### Scenario: 失败和竞争淘汰保留采掘

- **GIVEN** 玩家持续 primary action 且可按既有规则采掘方块
- **WHEN** 近战挥空、被遮挡、处于 cooldown、目标受保护或玩家 intent 成为 reservation loser
- **THEN** 采掘 MUST 与变更前规则一致，剑耐久、疲劳和 hit 事实 MUST 不变化

#### Scenario: 快照不依赖处理先后

- **GIVEN** 多名玩家在同一 tick 同时持续 primary action
- **WHEN** 服务端按稳定顺序结算该 tick
- **THEN** 每次近战候选 MUST 基于该 tick 的同一意图快照

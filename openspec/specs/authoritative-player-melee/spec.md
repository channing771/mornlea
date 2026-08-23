# authoritative-player-melee Specification

## Purpose

为最多八名局域网玩家提供确定且有界的服务端权威近战结算，使持续 primary action、方块遮挡、目标冷却、伤害入口与既有采掘路径共享同一 tick 事实。

## Requirements

### Requirement: 服务端权威玩家近战

服务端 SHALL 仅把 active、同维度玩家作为持续 primary action 的近战候选。它 MUST 使用最长 3 格射线，固体方块 MUST 阻挡射线，流体 MUST NOT 阻挡射线；多个候选时 MUST 选择最近命中，等距时 MUST 按 `SessionID` 选择。有效命中 MUST 通过既有伤害入口造成 2 点伤害，同一目标在其最近一次成功命中后的 10 个 tick 内 MUST NOT 再被近战命中。

#### Scenario: 最近未遮挡同维玩家被命中

- **GIVEN** 发起者和两名 active 同维玩家在 3 格内且射线无遮挡
- **WHEN** 发起者持续 primary action
- **THEN** 最近玩家 MUST 受到 2 点伤害
- **AND** 更远玩家 MUST 不受伤害

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

#### Scenario: 目标冷却

- **GIVEN** 目标刚被有效近战命中
- **WHEN** 任意玩家在之后 10 个 tick 内再次命中该目标
- **THEN** 目标 MUST 不再因此受到近战伤害

#### Scenario: 非候选不受攻击

- **GIVEN** 目标 inactive、异维度、超出 3 格或被固体方块遮挡
- **WHEN** 发起者持续 primary action
- **THEN** 该目标 MUST 不受近战伤害

### Requirement: 同 tick primary-action 快照与采掘分流

服务端 MUST 在每个 tick 使用同一份 primary-action 意图快照决定近战。若 primary action 合法命中玩家，服务端 MUST 只抑制发起者该 tick 的采掘；若未命中合法玩家，采掘 MUST 完整沿用既有规则。下一 tick MUST 继续由持续输入决定，不得遗留命中抑制状态。

#### Scenario: 命中抑制仅当前 tick 的采掘

- **GIVEN** 发起者同 tick 瞄准可采掘方块和合法玩家目标
- **WHEN** 服务端处理该 tick 的 primary action
- **THEN** 玩家目标 MUST 按近战规则结算
- **AND** 发起者该 tick MUST 不推进采掘

#### Scenario: 未命中保留采掘

- **GIVEN** 发起者持续 primary action 但没有合法玩家命中
- **WHEN** 服务端处理该 tick
- **THEN** 采掘 MUST 与变更前规则一致

#### Scenario: 快照不依赖处理先后

- **GIVEN** 多名玩家在同一 tick 同时持续 primary action
- **WHEN** 服务端按稳定顺序结算该 tick
- **THEN** 每次近战候选 MUST 基于该 tick 的同一意图快照

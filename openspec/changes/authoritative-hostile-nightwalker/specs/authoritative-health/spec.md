# authoritative-health Specification

## ADDED Requirements

### Requirement: 敌怪近战伤害进入既有权威结算

夜行者的近战伤害 MUST 与摔落、饥饿伤害走同一权威伤害入口：命中后 MUST 重置玩家回血计时并产生与其它伤害相同的可观察后果（回血中断、死亡按既有的同 tick 死亡结算）。夜行者伤害随权威玩家状态发布；客户端 MUST NOT 预测敌怪伤害。玩家生命降到 0 时 MUST 复用既有死亡结算（背包掉落、回出生锚点、满血、速度归零），且中途 MUST NOT 对外发布生命值为 0 的状态。

#### Scenario: 敌怪伤害重置回血计时

- **GIVEN** 玩家生命值 10 且已连续 100 tick 未受伤、正在回复
- **WHEN** 夜行者造成 3 点伤害
- **THEN** 回血 MUST 立即中断，且 MUST 重新连续 100 tick 未受伤才能再次开始

#### Scenario: 敌怪致死走既有死亡结算

- **GIVEN** 玩家生命值 3 且背包有物品
- **WHEN** 夜行者造成 3 点伤害
- **THEN** 该 tick 内 MUST 完成既有死亡结算：背包逐格掉落、玩家回出生锚点、生命值回 20

#### Scenario: 客户端只显示确认后的生命值

- **GIVEN** 客户端上一份确认生命值为 12
- **WHEN** 夜行者攻击且下一份确认状态为 9
- **THEN** HUD MUST 显示 9 且红色伤害反馈 MUST 触发
- **AND** 在确认到达前 MUST NOT 显示任何预测的 9 或更低值

## MODIFIED Requirements

### Requirement: 疲劳来源是固定表

系统 SHALL 只在固定的一组动作上累积疲劳，每种动作的疲劳量 MUST 固定：跳跃、身体浸没时的水平移动（按移动距离）、采掘完成、翻地完成、玩家实体近战成功命中（按每次命中）以及自然回血（按回复的生命值）。不在表中的动作 MUST NOT 累积疲劳。玩家实体近战成功命中指 player intent 通过容量检查、目标与冻结栏位重验、全局 victim reservation 并实际提交对 player 或 hostile 的伤害；每次 MUST 增加 100 milli fatigue。挥空、遮挡、距离超限、attack cooldown、目标 hurt cooldown、容量 fail closed、reservation loser 或冻结栏位身份不匹配 MUST NOT 累积疲劳；hostile 攻击 MUST NOT 给 hostile 或受击玩家增加这项近战疲劳。

#### Scenario: 跳跃累积疲劳
- **GIVEN** 玩家疲劳值为 0
- **WHEN** 玩家完成一次起跳
- **THEN** 疲劳值 MUST 增加固定的跳跃疲劳量

#### Scenario: 平地行走不累积疲劳
- **GIVEN** 玩家疲劳值为 0
- **WHEN** 玩家在平地上持续行走
- **THEN** 疲劳值 MUST 保持 0

#### Scenario: 采掘完成累积疲劳
- **GIVEN** 玩家开始一次权威采掘
- **WHEN** 玩家成功完成采掘
- **THEN** 疲劳值 MUST 增加固定的采掘疲劳量，被拒绝或中断的采掘 MUST NOT 累积

#### Scenario: 命中 player 或 hostile 增加 100 milli fatigue
- **GIVEN** 玩家 intent 指向合法 player 或 hostile 并通过统一 reservation
- **WHEN** 系统成功提交一次实体命中
- **THEN** 攻击者疲劳值 MUST 恰好增加 100 milli，目标疲劳值 MUST 不因该命中变化

#### Scenario: 近战失败不累积疲劳
- **GIVEN** 玩家保持 primary action
- **WHEN** 攻击挥空、被遮挡、超距、受 cooldown 阻止、目标受保护、容量失败、竞争淘汰或冻结栏位不匹配
- **THEN** 攻击者疲劳值 MUST 保持不变

#### Scenario: hostile 攻击不产生玩家攻击疲劳
- **GIVEN** 夜行者成功命中一名玩家
- **WHEN** 系统提交 hostile intent
- **THEN** 受击玩家与夜行者 MUST 不因这次命中增加 100 milli 玩家近战疲劳

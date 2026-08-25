# authoritative-hunger 变更（delta）

## MODIFIED Requirements

### Requirement: 疲劳来源是固定表

系统 SHALL 只在固定的一组动作上累积疲劳,每种动作的疲劳量 MUST 固定:跳跃、身体浸没时的水平移动(按移动距离)、采掘完成、翻地完成、近战命中成功(按每次命中)以及自然回血(按回复的生命值)。不在表中的动作 MUST NOT 累积疲劳。近战命中成功指权威近战在该 tick 实际冻结了一次有效意图:存在三格内的同维玩家目标、射线未被固体方块遮挡、且目标不在受击冷却免疫窗口;落空、被遮挡与免疫窗口内的输入 MUST NOT 累积疲劳。

#### Scenario: 跳跃累积疲劳

- **GIVEN** 玩家疲劳值为 0
- **WHEN** 玩家完成一次起跳
- **THEN** 疲劳值 MUST 增加固定的跳跃疲劳量

#### Scenario: 平地行走不累积疲劳

- **GIVEN** 玩家疲劳值为 0
- **WHEN** 玩家在平地上持续行走
- **THEN** 疲劳值 MUST 保持 0

#### Scenario: 采掘完成累积疲劳

- **WHEN** 玩家完成一次权威采掘
- **THEN** 疲劳值 MUST 增加固定的采掘疲劳量,且被拒绝或中断的采掘 MUST NOT 累积

#### Scenario: 近战命中累积疲劳

- **GIVEN** 攻击者与另一名玩家同维,目标在三格内且无方块遮挡
- **WHEN** 攻击者以连续 primary action 命中目标一次
- **THEN** 攻击者疲劳值 MUST 增加固定的近战疲劳量,目标疲劳值 MUST 保持不变

#### Scenario: 近战落空不累积疲劳

- **GIVEN** 攻击者保持 primary action 按住
- **WHEN** 视线内没有合法玩家目标,或射线被固体方块遮挡
- **THEN** 攻击者疲劳值 MUST 保持不变

#### Scenario: 受击冷却免疫窗口内不累积疲劳

- **GIVEN** 目标玩家仍处于受击冷却窗口内
- **WHEN** 攻击者对该目标保持 primary action 按住
- **THEN** 该 tick 不形成命中,攻击者疲劳值 MUST 保持不变

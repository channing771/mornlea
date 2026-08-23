## MODIFIED Requirements

### Requirement: primary action 未命中玩家时保持权威采掘

服务端 SHALL 继续以既有权威采掘规则处理持续 primary action。只有该 action 在同一 tick 合法命中玩家时，服务端 MUST 抑制该 tick 的采掘；没有合法玩家命中时，采掘进度、目标校验、拒绝和结算 MUST 与变更前完全一致。命中抑制 MUST NOT 跨 tick 保留。

#### Scenario: 未命中玩家仍采掘

- **GIVEN** 玩家持续 primary action，瞄准一个按既有规则可采掘的方块且没有合法玩家命中
- **WHEN** 服务端处理该 tick
- **THEN** 采掘 MUST 按既有规则推进或结算

#### Scenario: 命中玩家只抑制当前 tick

- **GIVEN** 玩家本 tick 合法命中另一玩家并持续 primary action
- **WHEN** 服务端处理该 tick
- **THEN** 本 tick MUST 不推进采掘
- **WHEN** 下一 tick 已没有合法玩家命中且输入仍持续
- **THEN** 采掘 MUST 再按既有规则处理

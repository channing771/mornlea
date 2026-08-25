# authoritative-hunger Specification

## ADDED Requirements

### Requirement: 腐肉是可食用的低效食物且无状态效果

系统 SHALL 把 `ItemRottenFlesh` 注册为可食物品：饥饿恢复 4、饱和恢复 0、堆叠上限 64；其进食 MUST 复用既有进食状态机（进度、中断清零、原子结算、饥饿钳到 20、饱和不超过饥饿值），且 MUST NOT 产生中毒或任何持续性状态效果。非饥饿状态 MUST NOT 因进食腐肉变化。

#### Scenario: 进食腐肉恢复饥饿

- **GIVEN** 玩家手持 2 个腐肉、饥饿值 10、饱和度 0
- **WHEN** 玩家保持进食输入达到固定 tick 数
- **THEN** 腐肉 MUST 变为 1，饥饿值 MUST 变为 14，饱和度 MUST 保持 0

#### Scenario: 进食腐肉无状态效果

- **GIVEN** 玩家完成一次腐肉进食
- **WHEN** 系统继续推进 200 tick
- **THEN** 除三层饥饿状态外 MUST NOT 出现任何额外生命/状态效果

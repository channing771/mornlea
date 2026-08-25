# authoritative-hunger Specification

## MODIFIED Requirements

### Requirement: 食物由固定表定义且可由小麦合成

系统 SHALL 以固定表定义可食物品及其饥饿与饱和恢复值。面包 MUST 可由固定配方用小麦合成。非食物物品 MUST NOT 可进食。

#### Scenario: 小麦合成面包

- **GIVEN** 玩家持有足量小麦
- **WHEN** 玩家合成面包配方
- **THEN** 小麦 MUST 被原子扣除，玩家 MUST 获得面包

#### Scenario: 非食物不可进食

- **GIVEN** 玩家手持小麦或泥土
- **WHEN** 玩家保持进食输入任意 tick
- **THEN** 物品数量 MUST 不变，饥饿值 MUST 不变

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

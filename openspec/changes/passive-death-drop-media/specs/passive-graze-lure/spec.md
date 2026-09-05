## MODIFIED Requirements

### Requirement: 牛周期性低头吃草并把脚下草方块变为泥土

牛 SHALL 以有界频率触发吃草事件：仅当站立方块为草方块、chunk 已完整加载、且本 tick 经世界种子与世界时间派生的确定性抽选命中时触发，MUST NOT 读取全局随机数或遍历 map。事件 SHALL 持续 20 tick 低头，结束时若仍站在同一草方块上，系统 MUST 经既有 mutation 路径把该格草方块变为泥土；事件期间受击、移动或脚下方块变化 MUST 中断事件且 MUST NOT 写块。每牛每事件 MUST 至多写 1 格。吃草事件态为瞬态，MUST NOT 落盘，重启后不恢复。

#### Scenario: 吃草把草变泥土

- **GIVEN** 一头牛站在草方块上且本 tick 抽选命中
- **WHEN** 系统连续推进 20 tick 且期间无中断
- **THEN** 第 20 tick 该草方块 MUST 变为泥土，牛 MUST 恢复漫游位姿

#### Scenario: 受击中断吃草不写块

- **GIVEN** 一头正在低头的牛
- **WHEN** 玩家对其造成 1 次有效伤害
- **THEN** 吃草事件 MUST 立即终止，牛 MUST 进入逃跑，脚下草方块 MUST 保持为草

#### Scenario: 非草方块不触发

- **GIVEN** 牛站在泥土/耕地/石头上
- **WHEN** 系统推进任意 tick
- **THEN** MUST NOT 触发吃草事件，世界 MUST 无写入

#### Scenario: 未加载 chunk 不写块

- **GIVEN** 吃草事件结束时牛脚下 chunk 不完整加载
- **WHEN** 系统结算该事件
- **THEN** MUST NOT 写入方块，MUST NOT 为吃草触发同步加载

### Requirement: 牛闲时面向附近玩家

漫游态（非逃跑、非吃草事件、非引诱跟随）的牛， SHALL 在同维最近 active 玩家进入水平 6 格时把身体朝向转向该玩家（每 tick 有界转向角，不瞬移）；玩家离开 6 格 MUST 恢复漫游朝向派生。朝向调整 MUST NOT 改变位置、速度或任何持久化字段，且 MUST 与引诱/逃跑优先级正交（逃跑与引诱生效时本规则让路）。

#### Scenario: 闲时看人

- **GIVEN** 一头漫游中的牛与一名静立玩家相距 4 格
- **WHEN** 系统推进若干 tick
- **THEN** 牛头/身体朝向 MUST 转向玩家，位置 MUST 不变

#### Scenario: 逃跑时不看人

- **GIVEN** 一头逃跑中的牛附近有玩家
- **WHEN** 系统推进 tick
- **THEN** 牛 MUST 保持远离伤害源方向，MUST NOT 转向玩家

### Requirement: 被引诱的牛始终面向持麦玩家

处于引诱跟随的牛（含 2.5 格止步状态），其身体朝向 MUST 每 tick 指向持麦玩家（有界转向角），头部朝向 MUST 与身体一致；玩家切走/超距恢复漫游后朝向约束解除。本规则与“止步”语义正交：止步只冻结位移，不冻结朝向。

#### Scenario: 止步后面向玩家

- **GIVEN** 一头已在 2.5 格止步的跟随牛
- **WHEN** 持麦玩家横向移动到牛的另一侧
- **THEN** 牛 MUST 原地转向玩家，位置 MUST 不变

# passive-graze-lure Specification

## REMOVED Requirements

### Requirement: 牛闲时面向附近玩家并靠近

（替换为「牛闲时面向附近玩家」：闲时看人不再产生位移，消除不持小麦也被贴身跟随的观感缺陷。）

## ADDED Requirements

### Requirement: 牛闲时面向附近玩家

漫游态（非逃跑、非吃草事件、非引诱跟随）的牛，SHALL 在同维最近 active 玩家进入水平 6 格时把身体朝向转向该玩家（每 tick 有界转向角，不瞬移），且 MUST NOT 因闲时看人产生任何位移；玩家离开 6 格 MUST 恢复漫游朝向派生。靠近玩家的唯一途径 SHALL 保持为手持小麦的引诱（2.5 格止步语义见既有引诱需求）。朝向调整 MUST NOT 改变速度以外的权威字段，且 MUST 与引诱/逃跑优先级正交（逃跑与引诱生效时本规则让路）。

#### Scenario: 闲时只看不靠近

- **GIVEN** 一头漫游中的牛与一名静立玩家相距 4 格，玩家未持小麦
- **WHEN** 系统推进若干 tick
- **THEN** 牛 MUST 原地转向玩家，水平位置 MUST 保持不变（无靠近位移）

#### Scenario: 离开视线恢复漫游

- **GIVEN** 一头正在闲时面向玩家的牛
- **WHEN** 玩家退出水平 6 格
- **THEN** 牛 MUST 恢复漫游朝向派生，MUST NOT 继续追踪该玩家

#### Scenario: 逃跑时不看人

- **GIVEN** 一头逃跑中的牛附近有玩家
- **WHEN** 系统推进 tick
- **THEN** 牛 MUST 保持远离伤害源方向，MUST NOT 转向玩家

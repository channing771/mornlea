# passive-graze-lure Specification

## Purpose

定义被动牛的吃草与小麦引诱跟随行为：周期性低头吃草并把脚下草方块变为泥土，手持小麦的玩家可在近距离引诱牛跟随。本能力只覆盖服务端权威行为事实与客户端位姿呈现；放牧标志位的线上传输见 `passive-mob-protocol` 的同期变更。

## Requirements

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

### Requirement: 手持小麦的玩家可引诱牛跟随

同维最近的 active 玩家手持选中格为小麦且与牛水平距离 ≤8 格时，牛 SHALL 朝该玩家移动并在水平 2.5 格处止步；玩家切走小麦或超出 8 格 MUST 恢复漫游。逃跑 SHALL 优先于引诱，引诱 SHALL 优先于漫游。引诱 MUST NOT 消耗小麦，MUST NOT 触发繁殖。手持判定 MUST 以服务端权威背包为准（选中格物品），MUST NOT 新增协议字段。

#### Scenario: 持麦靠近被跟随

- **GIVEN** 一头漫游中的牛与一名手持小麦的玩家相距 6 格
- **WHEN** 系统推进若干 tick
- **THEN** 牛 MUST 朝玩家移动，直到水平距离 ≤2.5 格后止步

#### Scenario: 切走小麦恢复漫游

- **GIVEN** 一头正在跟随的牛
- **WHEN** 玩家把选中格切到非小麦物品
- **THEN** 牛 MUST 在下一 tick 恢复漫游，MUST NOT 继续跟随

#### Scenario: 逃跑优先于引诱

- **GIVEN** 一头正在跟随的牛
- **WHEN** 玩家对其造成 1 次有效伤害
- **THEN** 牛 MUST 进入逃跑并远离该玩家，即使该玩家仍手持小麦

### Requirement: 低头位姿随放牧标志呈现

客户端 SHALL 在牛的放牧标志置位时把牛头下压（复用既有头部俯仰通道），清位时恢复；位姿 MUST 完全由权威 state 驱动，MUST NOT 由客户端自行推测或随机摆动。

#### Scenario: 低头与恢复

- **GIVEN** 客户端收到某牛放牧标志置位的 state
- **WHEN** 渲染该牛
- **THEN** 牛头 MUST 下压；收到清位 state 后 MUST 恢复常态位姿

### Requirement: 吃草与引诱预算有界

吃草抽选 MUST 为常数时间哈希判定；引诱的目标扫描 MUST 复用既有最近玩家模式且每牛每 tick 有界；世界写入 MUST 每事件 ≤1 格。权威 tick MUST 不因本能力产生无界工作。

#### Scenario: 满负载 tick 保持预算

- **GIVEN** 全服 32 头牛、8 名 active 玩家
- **WHEN** 系统推进一个 tick
- **THEN** 吃草判定与引诱扫描 MUST 在常数/线性有界内完成，tick MUST 正常完成

### Requirement: 牛闲时面向附近玩家并靠近

漫游态（非逃跑、非吃草事件、非引诱跟随）的牛， SHALL 在同维最近 active 玩家进入水平 6 格时把身体朝向转向该玩家（每 tick 有界转向角，不瞬移），并以漫游速度靠近到水平 1.5 格止步（只冻位移不冻朝向）；玩家离开 6 格 MUST 恢复漫游朝向派生。朝向调整 MUST NOT 改变速度以外的权威字段，且 MUST 与引诱/逃跑优先级正交（逃跑与引诱生效时本规则让路）。

#### Scenario: 闲时看人并靠近

- **GIVEN** 一头漫游中的牛与一名静立玩家相距 4 格
- **WHEN** 系统推进若干 tick
- **THEN** 牛 MUST 转向玩家并靠近到约 1.5 格止步，止步后 MUST 保持面向玩家

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


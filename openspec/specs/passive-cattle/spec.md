# passive-cattle Specification

## Purpose

定义被动牛的可观察行为闭环：确定性昼间生成、漫游与受击逃跑、死亡掉落生牛肉，以及生/熟牛肉食物链。本能力只覆盖服务端权威模拟事实与食物语义；持久化、协议与呈现分别在 `passive-mob-persistence`、`passive-mob-protocol` 与 `passive-cattle-presentation`。

## Requirements

### Requirement: 牛具有稳定非零身份并受全服与局部上限约束

系统 SHALL 为每头牛维护一个非零稳定 ID，由世界种子、世界时间与候选坐标确定性派生；相同世界状态与相同输入序列下重放 MUST 派生逐位相同的 ID 序列。全服同时存在的牛 MUST 不超过 32 头；任一 active 玩家水平 48 格半径内 MUST 不超过 6 头。牛集合 MUST 按 ID 严格升序维护；第 33 头或某玩家附近第 7 头 MUST 被拒绝生成。

#### Scenario: 相同状态重放派生相同 ID

- **GIVEN** 相同的世界种子、世界时间与玩家集合
- **WHEN** 两个独立权威引擎各自推进相同 tick 数
- **THEN** 两个引擎生成的牛数量、位置与 ID 序列 MUST 逐项相同

#### Scenario: 全服第 33 头被拒绝

- **GIVEN** 全服已存在 32 头牛且一个合法的生成候选通过全部条件
- **WHEN** 系统推进该候选的验证 tick
- **THEN** 本 tick MUST 不生成该候选，牛总数 MUST 保持 32

#### Scenario: 恢复顺序保持升序

- **GIVEN** 一份包含非升序 ID 的被动持久记录
- **WHEN** 系统在加载时读取该记录
- **THEN** 系统 MUST 拒绝整份记录（见 `passive-mob-persistence`），不得产生乱序的内部集合

### Requirement: 昼间在草地上确定性生成

系统 SHALL 仅当全部条件同时成立时才生成牛：显示相位为白昼；候选来自锚点玩家且水平距离在 `24..48`（含）；候选格为草方块正上方双格空气；候选下方支撑格为 solid；候选格与支撑格均非流体；候选所在 chunk 完整加载。生成判定 MUST 从世界种子与 `WorldTimeTicks` 导出，MUST NOT 读取全局随机数或遍历 map；每 tick MUST 至多验证一个生成候选。任一条件不成立时，该候选 MUST 被拒绝且本 tick MUST NOT 生成任何牛。

#### Scenario: 昼间草地生成

- **GIVEN** 显示相位为白昼、锚点玩家安全、候选水平距离 36、双格空气且下方为草方块支撑
- **WHEN** 系统推进该候选验证
- **THEN** 该候选 MUST 被生成，初始生命 MUST 为满值、路径 MUST 为空

#### Scenario: 夜间不生成

- **GIVEN** 显示相位为夜晚
- **WHEN** 系统推进任意 tick
- **THEN** MUST NOT 生成任何牛

#### Scenario: 非草地或流体候选被拒绝

- **GIVEN** 候选下方不是草方块，或候选格为流体，或仅一格竖向空气
- **WHEN** 系统推进该候选验证
- **THEN** 本 tick MUST NOT 生成牛

#### Scenario: 每 tick 至多验证一个候选

- **GIVEN** 一枚候选的派生与验证预算
- **WHEN** 一个 tick 内完成该候选但条件不满足
- **THEN** 本 tick 结果 MUST NOT 再消耗其它候选，下一次候选验证 MUST 在下一 tick

### Requirement: 漫游与受击逃跑且不反击

牛 SHALL 在无威胁时以有界步长漫游（不离开出生 chunk 邻域、不穿墙）；受到玩家或夜行者伤害的当 tick 起进入逃跑状态，沿远离伤害来源方向移动固定时长，逃跑期间 MUST NOT 还击、MUST NOT 主动接近任何玩家。逃跑结束后恢复漫游。

#### Scenario: 受击后逃跑不反击

- **GIVEN** 一头漫游中的牛
- **WHEN** 玩家对其造成 1 次有效伤害
- **THEN** 牛 MUST 进入逃跑状态并远离该玩家，且 MUST NOT 对玩家造成任何伤害

#### Scenario: 无路径时不穿墙

- **GIVEN** 牛前方为实心方块且无可用绕行路径
- **WHEN** 系统推进移动
- **THEN** 牛 MUST 停止或转向，MUST NOT 进行穿墙直线移动

### Requirement: 死亡在单 tick 移除并确定性掉落生牛肉

牛生命归零时，系统 SHALL 在同一权威 tick 内移除其身体，并 MUST 经既有掉落契约在死亡位置所在 chunk 放置 1 个 `ItemRawBeef`；该 chunk 掉落槽已满时 MUST 按已排序 Ready chunk 顺序环形尝试，全部已加载可用 chunk 均满时 MUST 确定性省略掉落但仍完成死亡。掉落放置顺序 MUST 可复现。死亡结果 MUST 在同一 tick 内完成，MUST NOT 留下半移除状态。

#### Scenario: 死亡掉 1 个生牛肉

- **GIVEN** 牛生命降至 0 且死亡 chunk 有充足掉落槽
- **WHEN** 系统完成该 tick
- **THEN** 牛 MUST 从集合移除，世界中 MUST 出现 1 个 `ItemRawBeef` 掉落物

#### Scenario: 槽满时确定性省略掉落

- **GIVEN** 死亡 chunk 与全部已加载相邻 chunk 的掉落槽已满
- **WHEN** 系统完成该 tick
- **THEN** 牛 MUST 被移除，MUST NOT 生成掉落物，MUST NOT 静默销毁任何已有物品

### Requirement: 生熟牛肉食物链闭环

系统 SHALL 提供 `ItemRawBeef` 与 `ItemCookedBeef`（堆叠 64、不可放置）；熔炉 SHALL 支持 1 生牛肉→1 熟牛肉的固定配方；`FoodValue` SHALL 为生牛肉与熟牛肉分别返回固定饥饿/饱和值（熟牛肉严格高于生牛肉）；腐肉、面包既有语义 MUST 不变。

#### Scenario: 熔炼生牛肉得熟牛肉

- **GIVEN** 熔炉输入为 1 个生牛肉且燃料充足
- **WHEN** 熔炼完成
- **THEN** 系统 MUST 产出 1 个熟牛肉，生牛肉 MUST 被消费

#### Scenario: 熟牛肉恢复值高于生牛肉

- **GIVEN** 相同饥饿状态的玩家
- **WHEN** 分别进食 1 个生牛肉与 1 个熟牛肉
- **THEN** 熟牛肉恢复的饥饿值 MUST 大于生牛肉，且两者 MUST 均可进食结算

### Requirement: 预算上限固定且不随世界规模放大

每 tick 至多 1 个生成候选验证、全服 32 头、每玩家附近 6 头——上述数值 MUST 全部由边界测试锁定，MUST NOT 随玩家数或牛数放大；权威 tick MUST 不因牛系统产生无界工作。

#### Scenario: 满负载 tick 保持预算

- **GIVEN** 全服 32 头牛、8 名 active 玩家
- **WHEN** 系统推进一个 tick
- **THEN** 该 tick MUST 至多验证 1 个生成候选，tick MUST 正常完成

# passive-cattle Specification

## MODIFIED Requirements

### Requirement: 漫游与受击逃跑且不反击

牛 SHALL 在无威胁时以有界步长漫游（不离开出生 chunk 邻域、不穿墙）；漫游朝向 MUST 按固定长度 tick 段组织：段内（段长 40 tick）目标朝向由世界种子、段序号与牛 ID 确定性派生且保持不变，段间目标朝向变化 MUST 经每 tick 有界转向角过渡，MUST NOT 出现每 tick 重抽朝向或朝向瞬跳。受到玩家或夜行者伤害的当 tick 起进入逃跑状态，沿远离伤害来源方向移动固定时长，逃跑期间 MUST NOT 还击、MUST NOT 主动接近任何玩家。逃跑结束后恢复漫游。

#### Scenario: 段内朝向稳定不打转

- **GIVEN** 一头漫游中的牛与逐 tick 递进的权威 tick
- **WHEN** 系统在同一个 40 tick 段内连续推进若干 tick 并记录牛朝向
- **THEN** 段内牛的目标朝向 MUST 保持不变，MUST NOT 每 tick 重抽

#### Scenario: 段间有界转向

- **GIVEN** 牛跨越漫游段边界、新旧段目标朝向不同
- **WHEN** 系统逐 tick 推进
- **THEN** 牛朝向 MUST 按既有界转向角逐步过渡到新段朝向，单 tick 转角 MUST NOT 超过有界上限，MUST NOT 瞬跳

#### Scenario: 受击后逃跑不反击

- **GIVEN** 一头漫游中的牛
- **WHEN** 玩家对其造成 1 次有效伤害
- **THEN** 牛 MUST 进入逃跑状态并远离该玩家，且 MUST NOT 对玩家造成任何伤害

#### Scenario: 无路径时不穿墙

- **GIVEN** 牛前方为实心方块且无可用绕行路径
- **WHEN** 系统推进移动
- **THEN** 牛 MUST 停止或转向，MUST NOT 进行穿墙直线移动

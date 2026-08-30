## MODIFIED Requirements

### Requirement: 追逐最近的 active 玩家且路径结果有界重验

夜行者 SHALL 选择最近的 active 同维 live 玩家作为目标（等距按 `PlayerID` 字节序）。每 tick 系统 MUST 至多为到期夜行者构造 2 份不可变路径快照，其余顺延；路径结果 MUST 在 tick 边界按 ID 升序应用，已过期（目标变化、路径上下文变化或路径已重算）的结果 MUST 被丢弃；每个路径点到达前 MUST 重验对应 chunk revision；目标超出路径窗口时终点 MUST 钳到朝玩家方向的窗口边缘可站立格；距目标水平 1.8 格内 MUST 停止移动，并在每个 tick 向权威模拟冻结一次包含既有追逐目标的攻击意图。manager MUST NOT 使用上一份 hostile cooldown snapshot 预先过滤范围内意图；sim MUST 在 cooldown 递减后执行唯一权威准入。无可用路径时夜行者 MUST NOT 进行穿墙直线移动。

#### Scenario: 选择最近玩家
- **GIVEN** 两名 active 玩家与一只夜行者，A 距 20 格、B 距 10 格
- **WHEN** 系统规划夜行者路径
- **THEN** 目标 MUST 为 B

#### Scenario: 等距按身份决定
- **GIVEN** 两名 active 玩家与夜行者等距
- **WHEN** 系统规划
- **THEN** 目标 MUST 为 `PlayerID` 字节序较小者

#### Scenario: 过期路径结果被丢弃
- **GIVEN** 夜行者的一个已过期路径结果在最新结果之后到达
- **WHEN** tick 边界应用路径结果
- **THEN** 过期结果 MUST 被丢弃，移动 MUST 使用最新结果

#### Scenario: revision 失效触发重规划
- **GIVEN** 夜行者正沿路径移动且其前方路径点所在 chunk revision 已变化
- **WHEN** 系统重验该路径点
- **THEN** 夜行者 MUST 停止沿旧路径移动，路径清空，下一 tick 排入重算

#### Scenario: 范围内每 tick 冻结意图且 manager 不预过滤 cooldown
- **GIVEN** 夜行者与追逐目标水平距离 1.7 格，manager snapshot 中攻击 cooldown 为 1
- **WHEN** manager 消费移动结果并把意图交给本 tick sim
- **THEN** 夜行者 MUST 停止水平移动并冻结一次攻击意图
- **AND** sim MUST 在 cooldown 从 1 递减为 0 后允许该 intent 在同 tick 进入统一仲裁

### Requirement: 近战攻击按固定数值结算且同 tick 冻结全部意图

夜行者近战 MUST 只消费 manager 已冻结的追逐目标，并在 sim 中重验目标 active、存活、同维和水平距离 ≤1.8；不得执行方块射线或改选后方目标。合法 hostile intent MUST 在统一 actor snapshot 上与全部玩家 intent 一起冻结，并由 sim 在 cooldown 递减后执行唯一准入。accepted hostile intent MUST 造成 3 点伤害、增加 0.35 水平击退，把攻击者 attack cooldown 设为 20 tick并把 player 目标 hurt cooldown 设为 20 tick。目标处于保护期、攻击者 cooldown 未就绪或跨类型 victim reservation 失败时 MUST 无副作用。同一 victim 的全局 reservation MUST hostile-first，同 kind hostile 按 ID 升序；所有 accepted intent MUST 在死亡处理前完成。

#### Scenario: 攻击造成 3 点伤害并进入 20 tick 冷却
- **GIVEN** 夜行者与满血玩家水平距离 1.5、攻击冷却就绪且玩家未受保护
- **WHEN** 系统统一结算该 tick 的 accepted intent
- **THEN** 玩家生命 MUST 降 3，夜行者 attack cooldown 与玩家 hurt cooldown MUST 均置为 20 tick

#### Scenario: 第 20 tick 边界由 sim 唯一准入
- **GIVEN** 夜行者刚完成一次攻击且玩家持续处于攻击距离内
- **WHEN** 系统继续推进 cooldown
- **THEN** 冷却期间 MUST 不重复扣血，并 MUST 在 cooldown 递减到 0 的 tick 由 sim 重新允许攻击，不得额外等待一 tick

#### Scenario: 超出攻击距离或目标无效不攻击
- **GIVEN** manager 冻结的追逐目标已死亡、inactive、异维或水平距离变为 2.5
- **WHEN** sim 重验 hostile intent
- **THEN** MUST NOT 造成伤害、击退、cooldown 或事件副作用

#### Scenario: 同目标跨类型竞争 hostile 优先
- **GIVEN** 一只 hostile 与一名 player 在同一 tick 各自冻结指向同一 victim 的合法 intent
- **WHEN** 系统执行全局 reservation
- **THEN** hostile intent MUST 成为唯一 accepted intent，player intent MUST 无副作用

#### Scenario: 玩家和夜行者可以同 tick 互击
- **GIVEN** player→hostile 与 hostile→player intent 指向不同 victim 且均合法
- **WHEN** 系统完成统一 settlement
- **THEN** 两次伤害、击退与来源相关 cooldown MUST 全部成立，即使任一方随后死亡

## ADDED Requirements

### Requirement: hostile burn、死亡与远离消失按固定阶段推进

统一战斗 settlement 完成后，系统 SHALL 依次推进 hostile burn、结算 hostile death/drop，再推进 distant despawn，最后处理 player death。夜行者同 tick 因战斗或 burn 生命归零且 `DistantTicks` 达到远离阈值时 MUST 先按死亡规则移除并尝试掉落 1 个腐肉，不得被无掉落 distant despawn 吞掉。

#### Scenario: 灼烧致死优先于同 tick distant despawn
- **GIVEN** 夜行者 health 为 1、burn 在本 tick 到期且 `DistantTicks=599`
- **WHEN** 系统推进该 tick
- **THEN** 夜行者 MUST 先因 burn 死亡并按既有确定规则尝试掉落腐肉，MUST NOT 走无掉落 distant despawn

#### Scenario: 剑杀死的夜行者观察结算后物品状态
- **GIVEN** 玩家用最后一点耐久的完好剑对夜行者造成致死伤害
- **WHEN** 统一战斗与随后 hostile death 阶段完成
- **THEN** 剑 MUST 已转为对应损坏形态，夜行者死亡与腐肉掉落 MUST 在同一 tick 完成

# authoritative-hostile-nightwalker Specification

## Purpose
定义夜行者的可观察行为闭环：确定性夜间生成、有界追逐与近战攻击、白昼露天灼烧与远离消失、死亡掉落、稳定身份与固定上限。本能力只覆盖服务端权威模拟事实；持久化与协议分别在 `hostile-mob-persistence` 与 `hostile-mob-protocol`，呈现契约在 `rust-client-render-entities`。
## Requirements
### Requirement: 夜行者具有稳定非零身份并受全服与局部上限约束

系统 SHALL 为每只夜行者维护一个非零稳定 ID，由世界种子、世界时间与候选坐标确定性派生；相同世界状态与相同输入序列下重放 MUST 派生逐位相同的 ID 序列。全服同时存在的夜行者 MUST 不超过 64 只；任一 active 玩家水平 48 格半径内 MUST 不超过 8 只。夜行者集合 MUST 按 ID 严格升序维护；第 65 只或某玩家附近第 9 只 MUST 被拒绝生成。

#### Scenario: 相同状态重放派生相同 ID

- **GIVEN** 相同的世界种子、世界时间与玩家集合
- **WHEN** 两个独立权威引擎各自推进相同 tick 数
- **THEN** 两个引擎生成的夜行者数量、位置与 ID 序列 MUST 逐项相同

#### Scenario: 全服第 65 只被拒绝

- **GIVEN** 全服已存在 64 只夜行者且一个合法的生成候选通过全部条件
- **WHEN** 系统推进该候选的验证 tick
- **THEN** 本 tick MUST 不生成该候选，夜行者总数 MUST 保持 64

#### Scenario: 玩家附近第 9 只被拒绝

- **GIVEN** 某 active 玩家附近已存在 8 只夜行者且该玩家是本次锚点
- **WHEN** 派生出的候选位于该玩家 48 格内
- **THEN** 本 tick MUST 不生成该候选

#### Scenario: 恢复顺序保持升序

- **GIVEN** 一份包含非升序 ID 的 hostile 持久记录
- **WHEN** 系统在加载时读取该记录
- **THEN** 系统 MUST 拒绝整份记录（见 `hostile-mob-persistence`），不得产生乱序的内部集合

### Requirement: 夜间在暗处确定性生成

系统 SHALL 仅当全部条件同时成立时才生成夜行者：显示相位在 `13000..23000`（含端点）；候选来自锚点玩家（从已排序 active session 按 `WorldTimeTicks % 会话数` 选锚点）且水平距离在 `24..48`（含）；候选格局部区块光 MUST ≤7；候选为双格空气（高度 2 的竖向空间）；候选下方支撑格为 solid；候选格与支撑格均非流体；候选所在 chunk 完整加载。生成判定 MUST 从世界种子与 `WorldTimeTicks` 导出，MUST NOT 读取全局随机数或遍历 map；每 tick MUST 至多验证一个生成候选。任一条件不成立时，该候选 MUST 被拒绝且本 tick MUST NOT 生成任何夜行者。

#### Scenario: 夜间暗处生成

- **GIVEN** 显示相位 13000、锚点玩家安全、候选水平距离 36、局部区块光 4、双格空气且下方 solid
- **WHEN** 系统推进该候选验证
- **THEN** 该候选 MUST 被生成，初始生命 MUST 为 20、攻击冷却 MUST 为 0、路径 MUST 为空

#### Scenario: 白日不生成

- **GIVEN** 显示相位 2400（白昼）
- **WHEN** 系统推进任意 tick
- **THEN** MUST NOT 生成任何夜行者

#### Scenario: 过亮候选被拒绝

- **GIVEN** 候选局部区块光为 8
- **WHEN** 系统推进该候选验证
- **THEN** 本 tick MUST NOT 生成夜行者

#### Scenario: 距离窗口外被拒绝

- **GIVEN** 候选水平距锚点玩家 23 或 49
- **WHEN** 系统推进该候选验证
- **THEN** 本 tick MUST NOT 生成夜行者

#### Scenario: 流体或支撑不足被拒绝

- **GIVEN** 候选格为流体，或候选下方支撑格为空气/流体，或候选仅一格竖向空气
- **WHEN** 系统推进该候选验证
- **THEN** 本 tick MUST NOT 生成夜行者

#### Scenario: 未加载 chunk 不生成

- **GIVEN** 候选落在尚未完整加载的 chunk
- **WHEN** 系统推进该候选验证
- **THEN** MUST NOT 生成夜行者，MUST NOT 为生成而触发同步加载

#### Scenario: 每 tick 至多验证一个候选

- **GIVEN** 一枚候选的派生与验证预算
- **WHEN** 一个 tick 内完成该候选但条件不满足
- **THEN** 本 tick 结果 MUST NOT 再消耗其它候选，下一次候选验证 MUST 在下一 tick

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

### Requirement: 白昼露天灼烧与远离消失

显示相位为白昼且夜行者格上方无遮挡方块（露天含天空光判据）时，系统 SHALL 每 20 tick 扣 1 点生命；遮顶或夜间 MUST 重置灼烧计时。距全部 active 玩家水平距离 >64 格时系统 SHALL 累计 `DistantTicks`，累计满 600 active ticks MUST 移除该夜行者且不产生掉落；回到范围内 MUST 清零累计。灼烧致死 MUST 走与普通死亡相同的移除与掉落路径（见「死亡在单 tick 移除」）。

#### Scenario: 露天白昼周期性灼烧

- **GIVEN** 显示相位为白昼、夜行者露天
- **WHEN** 系统推进 40 tick
- **THEN** 生命 MUST 下降 2，灼烧计时 MUST 连续

#### Scenario: 遮顶重置灼烧计时

- **GIVEN** 灼烧计时为 10 的夜行者上方被遮挡
- **WHEN** 系统继续推进 60 tick
- **THEN** 生命 MUST 不再因灼烧下降，且新计时 MUST 从 0 开始

#### Scenario: 远离 64 格累计 600 tick 消失

- **GIVEN** 夜行者距全部 active 玩家 70 格且 `DistantTicks` 为 599
- **WHEN** 系统推进 1 tick
- **THEN** 夜行者 MUST 被移除，MUST 不产生掉落

#### Scenario: 回到范围内清零累计

- **GIVEN** 夜行者距最近 active 玩家 50 格且 `DistantTicks` 为 599
- **WHEN** 系统推进任意 tick
- **THEN** `DistantTicks` MUST 归零，夜行者 MUST 保留

### Requirement: 死亡在单 tick 移除并确定性掉落腐肉

夜行者生命归零时，系统 SHALL 在同一权威 tick 内移除其身体，并 MUST 经既有掉落契约在死亡位置所在 chunk 放置 1 个 `ItemRottenFlesh`；该 chunk 掉落槽已满时 MUST 按已排序 Ready chunk 顺序环形尝试，全部已加载可用 chunk 均满时 MUST 确定性省略掉落但仍完成死亡。掉落放置顺序 MUST 可复现。死亡结果 MUST 在同一 tick 内完成，MUST NOT 留下半移除状态。

#### Scenario: 死亡掉 1 个腐肉

- **GIVEN** 夜行者生命降至 0 且死亡 chunk 有充足掉落槽
- **WHEN** 系统完成该 tick
- **THEN** 夜行者 MUST 从集合移除，世界中 MUST 出现 1 个 `ItemRottenFlesh` 掉落物

#### Scenario: 槽满时确定性省略掉落

- **GIVEN** 死亡 chunk 与全部已加载相邻 chunk 的掉落槽已满
- **WHEN** 系统完成该 tick
- **THEN** 夜行者 MUST 被移除，MUST NOT 生成掉落物，MUST NOT 静默销毁任何已有物品

#### Scenario: 灼烧致死同样掉肉

- **GIVEN** 夜行者因白昼灼烧生命归零
- **WHEN** 系统完成该 tick
- **THEN** 移除与掉落行为 MUST 与普通死亡相同

### Requirement: 预算上限固定且不随世界规模放大

每 tick 至多 1 个生成候选验证、至多 2 个路径快照构造、单消息至多 64 条记录、全服 64 只、每玩家附近 8 只、攻击冷却 20 tick、灼烧间隔 20 tick、消失累计 600 tick、局部区块光判定半径 14——上述数值 MUST 全部由边界测试锁定，MUST NOT 随玩家数或夜行者数放大；权威 tick MUST 不因夜行者系统产生无界工作。

#### Scenario: 满负载 tick 保持预算

- **GIVEN** 全服 64 只夜行者、8 名 active 玩家且全部夜行者到期需规划
- **WHEN** 系统推进一个 tick
- **THEN** 该 tick MUST 至多构造 2 份路径快照，其余夜行者 MUST 顺延，tick MUST 正常完成

# authoritative-mining Specification

## Purpose

为玩家提供由服务端按固定 tick 推进、工作量有界且多人结果确定的持续采掘，使工具等级、破坏时长、方块掉落和客户端反馈共享同一份权威状态。
## Requirements
### Requirement: 采掘由持续输入与权威状态推进

客户端 SHALL 只发送是否持续 primary action、视角和既有移动输入；服务端 MUST 在每个 20 Hz 权威 tick 先按统一战斗规则判定 primary action 是否成功结算 player 或 hostile 实体命中。只有成功实体命中时，服务端才 MUST 抑制该玩家本 tick 的采掘；未命中、失败或竞争淘汰时，服务端 MUST 使用玩家的权威位置、视角、当前选中快捷栏物品和六格内首个命中方块重新判定采掘状态。每名玩家 SHALL 最多维护一个独立目标，不同玩家的进度不得共享或累加。采掘进度、目标校验、拒绝和结算在未成功命中实体时 MUST 与变更前完全一致，且命中抑制 MUST NOT 跨 tick 保留。

#### Scenario: 按住后开始权威进度
- **GIVEN** Ready 玩家持续 primary action、未成功结算实体命中且权威射线命中一个可采掘方块
- **WHEN** 服务端处理第一份有效持续输入并推进一个 tick
- **THEN** 该玩家的权威状态 MUST 报告目标方块、进度 `1` 和对应总 tick

#### Scenario: 持续命中同一状态递增
- **GIVEN** 玩家上一 tick 正在采掘某方块、本 tick 未成功结算实体命中且目标方块与选中物品都没有变化
- **WHEN** 玩家继续持续 primary action 并推进下一个 tick
- **THEN** 权威进度 MUST 恰好增加 `1`

#### Scenario: 松开立即取消
- **GIVEN** 玩家已有非零采掘进度
- **WHEN** 服务端处理 primary action 为 false 的下一有效输入
- **THEN** 本 tick 发布的采掘状态 MUST 清零且方块不变

#### Scenario: 目标或工具变化重新开始
- **GIVEN** 玩家已有非零采掘进度且本 tick 未成功结算实体命中
- **WHEN** 权威射线目标、目标方块 ID 或选中物品发生变化且玩家仍持续 primary action
- **THEN** 旧进度 MUST 被丢弃，新状态从当前目标的第 `1` tick 开始

#### Scenario: 无效目标正常取消
- **GIVEN** 玩家正在采掘且本 tick 未成功结算实体命中
- **WHEN** 玩家超出六格、命中空气、区块未就绪、打开容器、断线或玩家状态 reset
- **THEN** 系统 MUST 清零进度且不得按每个 tick 生成拒绝消息

#### Scenario: 未成功命中实体仍采掘
- **GIVEN** 玩家持续 primary action，瞄准可采掘方块且近战挥空、遮挡、超距、处于 cooldown、目标受保护或成为 reservation loser
- **WHEN** 服务端处理该 tick
- **THEN** 采掘 MUST 按既有规则推进或结算

#### Scenario: 成功实体命中只抑制当前 tick
- **GIVEN** 玩家本 tick 成功结算 player 或 hostile 实体命中并持续 primary action
- **WHEN** 服务端处理该 tick
- **THEN** 本 tick MUST 不推进采掘
- **WHEN** 下一 tick 已没有成功实体命中且输入仍持续
- **THEN** 采掘 MUST 再按既有规则处理

### Requirement: 完好剑参与采掘不消耗耐久

完好木剑、石剑和铁剑 SHALL 按普通非采掘工具参与既有方块采掘判定，并 MUST NOT 因成功破坏任何方块而消耗耐久或转为损坏形态。镐、锄以及既有作物×锄头豁免的采掘时间、掉落和耐久语义 MUST 保持不变。

#### Scenario: 三把完好剑成功采掘泥土后保持完整
- **GIVEN** 玩家分别选中一把部分磨损的木剑、石剑或铁剑并持续采掘泥土
- **WHEN** 泥土按普通物品规则被成功移除
- **THEN** 对应剑 stack 的 item、数量和耐久 MUST 逐字段保持不变，MUST 不产生耐久导致的额外 inventory dirty

#### Scenario: 剑采掘不改变既有工具规则
- **GIVEN** 一名玩家用完好剑采掘、另一名玩家用既有镐或锄执行相同既有动作
- **WHEN** 两次动作分别完成
- **THEN** 剑 MUST 不磨损，镐、锄和作物×锄头豁免 MUST 继续按变更前规则结算
### Requirement: 采掘时长与掉落等级固定
系统 SHALL 使用以下固定规则：土和草使用任意手持状态均为 `5` tick 且正常掉落；石头在裸手或错误工具下为 `30` tick、石镐为 `15` tick、铁镐为 `8` tick，只有空选中格或任一镐可以取得掉落；石砖、熔炉、箱子、发光块、煤矿和铁矿分别为 `30/15/8` tick 且至少石镐才能取得掉落，其中发光块由正确镐破坏后 MUST 掉落一个发光块物品；铁块在裸手或错误工具下为 `40` tick、石镐为 `20` tick、铁镐为 `10` tick，只有铁镐可以取得掉落；作物在任意手持状态下均为 `1` tick(最小权威量子)，成熟作物 MUST 掉落小麦与至少一个种子、未成熟作物 MUST 掉落至少一个种子；耕地在任意手持状态下均为 `5` tick 且 MUST 掉落泥土；基岩 MUST 不推进且不可破坏。手持普通非工具物品不得视为裸手。

#### Scenario: 裸手石头形成启动例外
- **GIVEN** 玩家的权威选中快捷栏为空且持续命中石头
- **WHEN** 权威进度达到 `30` tick
- **THEN** 石头 MUST 被破坏并在原位置创建或合并一个石头掉落物

#### Scenario: 普通物品不是裸手
- **GIVEN** 玩家选中泥土物品并持续命中石头
- **WHEN** 权威进度达到 `30` tick
- **THEN** 石头 MUST 被破坏但不得产生石头掉落物

#### Scenario: 石镐取得矿石掉落
- **GIVEN** 玩家选中石镐并持续命中煤矿或铁矿
- **WHEN** 权威进度达到 `15` tick
- **THEN** 方块 MUST 被破坏并分别产生煤炭或粗铁掉落物

#### Scenario: 错误工具破坏矿石不掉落
- **GIVEN** 玩家未选中石镐或铁镐并持续命中煤矿或铁矿
- **WHEN** 权威进度达到 `30` tick
- **THEN** 方块 MUST 被破坏但不得产生煤炭或粗铁掉落物

#### Scenario: 铁块只接受铁镐采集
- **GIVEN** 玩家持续命中铁块
- **WHEN** 石镐达到 `20` tick 或铁镐达到 `10` tick
- **THEN** 两种情况都破坏铁块，但只有铁镐情况产生铁块掉落物

#### Scenario: 基岩永不推进
- **GIVEN** 玩家使用任意手持状态持续命中基岩
- **WHEN** 服务端推进任意数量的权威 tick
- **THEN** 基岩 MUST 保持不变且采掘状态保持清零

#### Scenario: 石镐取得箱子掉落
- **GIVEN** 玩家选中石镐并持续命中箱子
- **WHEN** 权威进度达到 `15` tick
- **THEN** 箱子 MUST 被破坏并产生箱子本体掉落物

#### Scenario: 裸手破坏箱子不掉落
- **GIVEN** 玩家未选中石镐或铁镐并持续命中箱子
- **WHEN** 权威进度达到 `30` tick
- **THEN** 箱子 MUST 被破坏但不得产生箱子本体掉落物

#### Scenario: 无正确镐破坏发光块不掉落
- **GIVEN** 玩家空手或手持普通非工具物品持续命中发光块
- **WHEN** 权威进度达到 `30` tick
- **THEN** 发光块 MUST 被破坏但不得产生发光块物品

#### Scenario: 石镐挖回发光块
- **GIVEN** 玩家选中石镐并持续命中发光块
- **WHEN** 权威进度达到 `15` tick
- **THEN** 发光块 MUST 被破坏并产生一个发光块物品

#### Scenario: 铁镐挖回发光块
- **GIVEN** 玩家选中铁镐并持续命中发光块
- **WHEN** 权威进度达到 `8` tick
- **THEN** 发光块 MUST 被破坏并产生一个发光块物品

#### Scenario: 任意手持状态一个 tick 内收获成熟作物
- **GIVEN** 玩家以任意手持状态命中一株成熟作物
- **WHEN** 权威进度达到 `1` tick
- **THEN** 该作物 MUST 被移除并掉落小麦与至少一个种子

#### Scenario: 误挖未成熟作物仍掉落种子
- **GIVEN** 玩家以任意手持状态命中一株未成熟作物
- **WHEN** 采掘完成
- **THEN** 该作物 MUST 被移除并掉落至少一个种子

#### Scenario: 采掘耕地掉落泥土
- **GIVEN** 玩家以任意手持状态持续命中耕地
- **WHEN** 权威进度达到 `5` tick
- **THEN** 耕地 MUST 被破坏并掉落泥土
### Requirement: 完成结果原子且多人顺序确定
服务端 MUST 把采掘完成产生的方块修改、熔炉状态、掉落物和区块 revision 作为同一区块的一次原子操作提交。容量或状态校验失败时所有值 MUST 保持不变。多个玩家同 tick 完成采掘时 SHALL 按稳定的 session 顺序串行处理。

#### Scenario: 掉落容量不足不破坏可采集方块
- **GIVEN** 玩家工具等级足够但目标区块无法接收应产生的方块掉落物
- **WHEN** 采掘达到完成 tick
- **THEN** 系统 MUST 拒绝一次并清零进度，方块、掉落物和 revision 全部保持不变

#### Scenario: 错误工具不占用方块掉落容量
- **GIVEN** 玩家工具等级不足且目标不是含物品的熔炉
- **WHEN** 采掘达到完成 tick
- **THEN** 系统 MUST 直接破坏方块且不得创建或预留方块掉落物

#### Scenario: 同 tick 竞争只提交一次
- **GIVEN** 两名玩家在同一 tick 达到同一目标方块的完成 tick
- **WHEN** 服务端处理该 tick
- **THEN** 较小 session 的结果先提交，另一名玩家看到空气后清零且不得产生第二份掉落

### Requirement: 客户端只呈现权威采掘状态
图形客户端 SHALL 在屏幕中心下方使用固定容量 HUD 显示最后确认的采掘进度；可取得掉落时显示绿色，完成后不会取得掉落时显示橙色。客户端 MUST NOT 根据本地按键提前推进进度、删除方块或修改物品。

#### Scenario: 未确认输入不推进 HUD
- **GIVEN** 玩家刚按下采掘键但尚未收到新权威玩家状态
- **WHEN** 客户端绘制下一帧
- **THEN** HUD MUST 保持最后确认状态且世界镜像不变

#### Scenario: 权威状态决定颜色和比例
- **GIVEN** 客户端收到进度 `6/15` 且可掉落标记为 true 的有效状态
- **WHEN** 客户端绘制下一帧
- **THEN** HUD MUST 显示比例为 `0.4` 的绿色进度条

#### Scenario: reset 清除旧进度
- **GIVEN** 客户端正在显示非零采掘进度
- **WHEN** 客户端断开、重新登录或收到玩家 reset
- **THEN** 下一帧 MUST 不再显示旧进度

### Requirement: 采掘热路径严格有界
服务端每 tick MUST 最多为八名活动玩家各执行一次六格射线和固定次数的标量或数组访问；稳定态采掘推进 MUST NOT 产生堆分配、启动 goroutine、创建动态目标表或扩展无界队列。

#### Scenario: 八名玩家持续采掘不分配
- **GIVEN** 八名 Ready 玩家各自持续命中一个有效目标
- **WHEN** 系统推进一个权威 tick
- **THEN** 采掘推进 MUST 只执行最多八次六格射线且不得产生堆分配

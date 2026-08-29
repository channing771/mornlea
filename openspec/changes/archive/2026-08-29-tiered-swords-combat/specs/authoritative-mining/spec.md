## MODIFIED Requirements

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

## ADDED Requirements

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

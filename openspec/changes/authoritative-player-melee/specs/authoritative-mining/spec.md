## MODIFIED Requirements

### Requirement: 采掘由持续输入与权威状态推进

客户端 SHALL 只发送是否持续 primary action、视角和既有移动输入；服务端 MUST 在每个 20 Hz 权威 tick 先按玩家近战规则判定 primary action 是否合法命中玩家。命中时，服务端 MUST 只抑制该玩家本 tick 的采掘；未命中时，服务端 MUST 使用玩家的权威位置、视角、当前选中快捷栏物品和六格内首个命中方块重新判定采掘状态。每名玩家 SHALL 最多维护一个独立目标，不同玩家的进度不得共享或累加。采掘进度、目标校验、拒绝和结算在未命中玩家时 MUST 与变更前完全一致，且命中抑制 MUST NOT 跨 tick 保留。

#### Scenario: 按住后开始权威进度

- **GIVEN** Ready 玩家持续 primary action、未命中合法玩家且权威射线命中一个可采掘方块
- **WHEN** 服务端处理第一份有效持续输入并推进一个 tick
- **THEN** 该玩家的权威状态 MUST 报告目标方块、进度 `1` 和对应总 tick

#### Scenario: 持续命中同一状态递增

- **GIVEN** 玩家上一 tick 正在采掘某方块、未命中合法玩家且目标方块与选中工具都没有变化
- **WHEN** 玩家继续持续 primary action 并推进下一个 tick
- **THEN** 权威进度 MUST 恰好增加 `1`

#### Scenario: 松开立即取消

- **GIVEN** 玩家已有非零采掘进度
- **WHEN** 服务端处理 primary action 为 false 的下一有效输入
- **THEN** 本 tick 发布的采掘状态 MUST 清零且方块不变

#### Scenario: 目标或工具变化重新开始

- **GIVEN** 玩家已有非零采掘进度且本 tick 未命中合法玩家
- **WHEN** 权威射线目标、目标方块 ID 或选中工具物品发生变化且玩家仍持续 primary action
- **THEN** 旧进度 MUST 被丢弃，新状态从当前目标的第 `1` tick 开始

#### Scenario: 无效目标正常取消

- **GIVEN** 玩家正在采掘且本 tick 未命中合法玩家
- **WHEN** 玩家超出六格、命中空气、区块未就绪、打开容器、断线或玩家状态 reset
- **THEN** 系统 MUST 清零进度且不得按每个 tick 生成拒绝消息

#### Scenario: 未命中玩家仍采掘

- **GIVEN** 玩家持续 primary action，瞄准一个按既有规则可采掘的方块且没有合法玩家命中
- **WHEN** 服务端处理该 tick
- **THEN** 采掘 MUST 按既有规则推进或结算

#### Scenario: 命中玩家只抑制当前 tick

- **GIVEN** 玩家本 tick 合法命中另一玩家并持续 primary action
- **WHEN** 服务端处理该 tick
- **THEN** 本 tick MUST 不推进采掘
- **WHEN** 下一 tick 已没有合法玩家命中且输入仍持续
- **THEN** 采掘 MUST 再按既有规则处理

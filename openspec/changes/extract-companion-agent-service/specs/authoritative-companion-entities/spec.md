## MODIFIED Requirements

### Requirement: 伙伴是独立的服务端权威移动 actor

服务端 SHALL 为每个 active 定义持有一个独立伙伴实体。伙伴 MUST 保持独立于玩家会话语义：MUST NOT 进入玩家登录、心跳、生命、伤害、死亡、掉落或重生语义。伙伴身体状态 MUST 只在权威 tick 边界变化，并 MUST 按 `CompanionID` 字节序发布确定性状态。伙伴 MAY 经权威任务移动；移动 MUST 只由 Go Task Runner 提交的 `CompanionAction` 驱动，MUST NOT 由玩家输入、客户端指令、Python Agent、模型输出或 MCP 工具结果直接触发；Agent 与 MCP 只可返回候选计划或只读校验结论。聊天寻址、Agent 请求、MCP 调用与候选计划验证本身 MUST NOT 改变身体、背包、任务或世界。

#### Scenario: 乱序配置产生稳定状态

- **GIVEN** 服务端以非 ID 顺序配置多个伙伴并推进多个 tick
- **WHEN** 每个 tick 取得伙伴状态
- **THEN** 状态 MUST 按 `CompanionID` 严格升序，无任务伙伴的位置、朝向和背包 MUST 保持不变

#### Scenario: 寻址入队不移动身体

- **GIVEN** 一个已激活伙伴与一条成功寻址并创建任务的聊天指令
- **WHEN** 任务尚未进入 Running
- **THEN** 伙伴位置、朝向、背包与世界方块 MUST 不变，首个移动 MUST 只发生在任务进入 Running 之后

#### Scenario: Agent 或 MCP 不能直接提交动作

- **GIVEN** Agent 输出合法候选计划，或模型尝试请求名为 move/mine/place 的工具
- **WHEN** 结果到达服务端
- **THEN** 合法候选 MUST 仍经过 Go 最终校验与 Task Runner，未知写工具 MUST 被拒绝；任一路径都 MUST NOT 绕过 `CompanionAction` inbox 修改伙伴或世界

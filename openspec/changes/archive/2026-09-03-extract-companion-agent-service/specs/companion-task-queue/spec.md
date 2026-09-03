## MODIFIED Requirements

### Requirement: 编排只在 tick 边界且不阻塞

Companion Manager SHALL 由 Go 服务端在权威 tick 边界驱动：接收聊天、入队或同步拒绝、构造 snapshot、应用 Agent Planner、MCP validator、寻路与 Dialogue/memory 结果、推进 Task Runner 并发布事件。Agent HTTP、MCP、模型、寻路、SQLite 与存档 I/O MUST 在 worker 或独立服务上处理有界不可变值，结果经有界 channel 回到 tick 边界应用；tick、渲染与网络热路径 MUST NOT 等待网络、磁盘、模型、MCP 或 JSON 编码。Agent/MCP 失败 MUST NOT 改写计划、插入 fallback 行为或重试：Planner 任务 MUST 以 `PlannerUnavailable` 或非法计划语义终结并推进 FIFO，Dialogue 失败 MUST 跳过台词，memory accepted reservation MUST 只暂停后续 Dialogue。

安全关服 SHALL 先停止接受新 `ChatCommand`，取消并等待在途 Agent/Planner/Dialogue/MCP/寻路 worker，冻结队列与 actor 状态，完成最终 `companions.ai` v5 保存，再关闭 MCP 和世界存储。若最终伙伴保存或世界 flush 失败，状态 MUST 保持可重试，MCP MUST 在重试期间保持可用且世界存储不得被不可逆关闭。

#### Scenario: 挂起的模型请求不阻塞权威模拟

- **GIVEN** 四个伙伴各有任务处于 Planning 且 Agent、MCP 或模型全部挂起
- **WHEN** 权威模拟连续推进多个 tick
- **THEN** 每个 tick MUST 按既有节拍完成，玩家命令、Running 伙伴任务与世界模拟 MUST 不因外部等待变化

#### Scenario: 台词结果应用不触碰任务事实

- **GIVEN** 一个任务的终态 Dialogue proposal 或 commit 结果到达 tick 边界
- **WHEN** Companion Manager 应用结果
- **THEN** 系统 MUST 只建立/解除 Dialogue reservation、广播合格 `CompanionSpeech` 与更新恢复镜像，任务状态、步骤索引、事实事件与 FIFO MUST 不因模型台词变化

#### Scenario: Agent 服务不可用仍推进 FIFO

- **GIVEN** 一个任务进入 Planning 时 Agent 服务不可连接且 FIFO 中还有下一条指令
- **WHEN** worker 返回失败到 tick 边界
- **THEN** 当前任务 MUST 以 `PlannerUnavailable` 失败并发布规范事件，下一任务 MUST 按 FIFO 进入 Planning，世界 tick MUST 不停止

#### Scenario: 关服顺序保证状态一致

- **GIVEN** 一个 Agent run、一条 memory commit 和一个寻路 worker 在途，FIFO 中还有待执行指令
- **WHEN** 服务端执行安全关服
- **THEN** 可观察顺序 MUST 是停止接受聊天、取消并等待全部 worker、冻结队列与 actor、最终 v5 保存、世界持久同步、关闭 MCP 与世界存储；任一持久化失败 MUST 保持可重试且不得提前关闭 MCP 或存储

# companion-task-queue Specification

## Purpose
定义每伙伴任务 FIFO、任务状态机、世界时间超时与权威 tick 边界编排，使指令执行顺序可预期、可持久、可观察，且模型与磁盘延迟不影响权威模拟。
## Requirements
### Requirement: 每伙伴 FIFO 有界且严格按序

寻址成功并 Accepted 的指令 SHALL 进入该伙伴独立 FIFO，容量 MUST 为 16 条，按服务端接收顺序排列。同一伙伴同一时刻 MUST 最多只有一个非终态任务；前一个任务进入终态后 MUST 立即开始执行原队首任务。伙伴 FIFO 已有 16 条待执行指令时，后续指令 MUST 以 `QueueFull` 同步拒绝且回复仅发送给发令者，MUST NOT 调用模型，也 MUST NOT 影响既有队列内容。

#### Scenario: 指令按接收顺序依次执行

- **GIVEN** 玩家连续向同一伙伴发送三条合法指令
- **WHEN** 服务端依次完成各任务
- **THEN** 任务 MUST 按接收顺序逐个进入 Running 并到达终态，后一条 MUST NOT 先于前一条开始

#### Scenario: 队列满同步拒绝

- **GIVEN** 一个伙伴的 FIFO 已有 16 条待执行指令
- **WHEN** 玩家再发送一条寻址该伙伴的合法指令
- **THEN** 服务端 MUST 只向发令者回复 `QueueFull` 拒绝事件，MUST NOT 发起 Planner 请求，既有 16 条 MUST 保持不变

### Requirement: 任务状态机与公开事件

任务 SHALL 按 `Queued → Planning → Validating → Running → Completed/Failed/TimedOut/Stopped` 推进，`Completed` 与 `TimedOut` MUST 只能从 `Running` 进入；`Failed` MAY 从 `Planning`、`Validating` 或 `Running` 进入——模型与校验失败必须能够终结尚未运行的任务；`Stopped` MUST 只能从 `Running` 的持续跟随任务经停止指令进入。任务进入 Running MUST 广播 `TaskStarted`，一个计划步骤完成 MUST 广播 `TaskProgress`，终态 MUST 分别广播 `TaskCompleted`、`TaskFailed`、`TaskTimedOut` 或 `TaskStopped`，且每次状态迁移 MUST 标记 AI 存档 dirty。`TaskFailed` MUST 携带稳定的服务器侧失败原因枚举（M5C 起 `TaskFailWorldChanged` 由跟随目标离线产生）。Task Runner MUST NOT 改写计划、插入计划外行为或为失败任务自动降级；任一终态 MUST 产生事件并推进 FIFO 下一项。

#### Scenario: 完整生命周期事件序列

- **GIVEN** 一条可用假模型成功规划的 `go_to` 指令且路径可达
- **WHEN** 任务从入队推进到完成
- **THEN** 玩家 MUST 依次观察到 Accepted、`TaskStarted` 与 `TaskCompleted`（或逐步骤 `TaskProgress`）事件，EventID 严格递增

#### Scenario: 失败任务推进 FIFO 下一项

- **GIVEN** 一个伙伴 FIFO 中第一条任务因模型不可用失败且队列还有下一条
- **WHEN** 失败终态事件产生
- **THEN** 服务端 MUST 公开 `TaskFailed` 及其原因，并 MUST 立即开始处理下一条指令

#### Scenario: 停止终态推进 FIFO 下一项

- **GIVEN** 一个伙伴当前持续跟随任务被停止且队列还有下一条
- **WHEN** `TaskStopped` 事件产生
- **THEN** 服务端 MUST 立即开始处理下一条指令，队列 MUST 不变

#### Scenario: Stopped 只能来自持续跟随

- **GIVEN** 一个伙伴当前任务是一次普通 `go_to`
- **WHEN** 停止指令到达
- **THEN** 任务 MUST NOT 转入 `Stopped`，停止 MUST 被同步拒绝

### Requirement: 普通任务超时使用持久化世界时间

每个普通任务 SHALL 在进入 Running 时记录 deadline 为当时的 `WorldTimeTicks` 加配置的执行分钟数；`taskTimeoutMinutes` MUST 在 `1..60` 范围内，缺省为 10。任务到达 deadline 时 MUST 转入 `TimedOut` 终态并广播事件。deadline MUST 随任务持久化；服务端停止运行期间世界时间不推进，安全关服期间 MUST NOT 消耗任务执行时长。持续跟随任务 MUST NOT 记录 deadline，也 MUST NOT 因执行时长转入 `TimedOut`——它只能由停止指令或目标离线终结。

#### Scenario: 到达 deadline 转入超时终态

- **GIVEN** 一个 Running 普通任务的世界时间 deadline 即将到达
- **WHEN** 权威 tick 推进过该 deadline
- **THEN** 任务 MUST 转入 `TimedOut`，广播对应事件，且移动 MUST 停止在当前位置

#### Scenario: 关服重启不消耗执行时长

- **GIVEN** 一个 Running 任务剩余一半执行时长时服务端安全关服
- **WHEN** 服务端重启并恢复该任务
- **THEN** 恢复后的 deadline 与关服前一致，任务 MUST 从已完成的步骤之后继续执行

#### Scenario: 持续跟随不设 deadline

- **GIVEN** 一个持续跟随任务
- **WHEN** 其运行时长超过任意配置的 `taskTimeoutMinutes`
- **THEN** 任务 MUST NOT 转入 `TimedOut`，持久化记录 MUST NOT 为其保存 deadline

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


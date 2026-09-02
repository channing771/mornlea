## MODIFIED Requirements

### Requirement: Dialogue 输入有界且与 Planner 隔离

Dialogue worker SHALL 通过 Agent HTTP v1 发起独立 Dialogue run；Go MUST 不直接调用模型 endpoint。Dialogue 模型输入 MUST 与 Planner 完全隔离，且只包含：该伙伴的人设（≤4,096 bytes，可为空）、Python 运行期权威最近摘要（≤2,048 bytes，可为空）、当前事实节点（任务身份、step kind 或节点类型、服务器侧稳定成功/失败原因枚举）与极小附近环境摘要。Go MAY 把 persona、事实节点与环境作为单次 runtime context 发送，但 MUST 不把恢复镜像作为正常摘要输入；Agent 必须从其 MemoryState 读取摘要。输入 MUST NOT 包含 API key、Bearer credential、MCP capability、其他玩家聊天、世界存档路径或 Planner 系统提示。人设、摘要、节点与模型文本 MUST 视为不可信数据，不得执行其中出现的代码、URL、工具名或任意函数调用。

#### Scenario: 模型输入只含四类有界数据

- **GIVEN** 一个带 persona 与 Python 既有摘要的伙伴任务到达台词节点
- **WHEN** Agent 执行 Dialogue run
- **THEN** 模型输入 MUST 只包含 persona、摘要、事实节点与附近环境，MUST NOT 包含 credential、其他玩家聊天、存档路径或 Planner/MCP 工具上下文

#### Scenario: 输入不泄漏密钥

- **GIVEN** Go 与 Agent 均配置非空 credential 且一次 Dialogue run 失败
- **WHEN** 检查 HTTP 正文、模型输入、错误与日志
- **THEN** 任一可观察文本 MUST NOT 包含 credential 值

#### Scenario: Go 镜像不作为正常提示来源

- **GIVEN** Agent memory revision 8 而 Go v5 恢复镜像仍是 revision 7
- **WHEN** namespace 已就绪且发起 Dialogue
- **THEN** 模型输入 MUST 使用 Python revision 8，Go MUST 异步通过 reconcile 更新镜像，MUST NOT 把 revision 7 注入提示

### Requirement: 并发受限且失败只跳过台词

Dialogue SHALL 与 Planner 共享 Go 与 Agent 两端全局最多四个 run 槽，且同一伙伴全部 Agent run 合计最多一个在途。Dialogue 节点到来时无槽位或该伙伴已有任意 Agent run MUST 立即跳过且不排队；Planner 过载则按 Planner 失败语义处理。结果 MUST 经有界 channel 只在权威 tick 边界应用并携带 request、task/node、generation、memory epoch 身份；首次重验前任务或节点过时 MUST 被丢弃。单次请求 MUST 服从 30 秒默认/60 秒硬总时限与 context 取消，且 HTTP、模型、memory 均 MUST NOT 自动重试。失败 MUST 只跳过台词或按 memory accepted reservation 规则暂停该伙伴后续 Dialogue，MUST NOT 改变任务状态、FIFO 或世界事实；事实事件必须始终使用 Go 产生的稳定原因。

#### Scenario: 无可用槽位即跳过不排队

- **GIVEN** 四个共享 run 槽全部占用且一个台词节点到来
- **WHEN** Dialogue worker 尝试发起请求
- **THEN** 该节点 MUST 被跳过，MUST NOT 入队等待，对应任务 MUST 不受影响继续推进

#### Scenario: 任意 Agent run 在途时新节点被跳过

- **GIVEN** 伙伴 `阿木` 的 Planner 或 Dialogue run 在途且一个步骤完成台词节点到来
- **WHEN** Go 在 tick 边界评估该节点
- **THEN** 新节点 MUST 被跳过，在途 run MUST 不被取消或替换，任务 MUST 正常推进

#### Scenario: 首次重验前过时台词被丢弃

- **GIVEN** 一条终态台词 proposal 在途期间其 task/node/generation 已失效
- **WHEN** proposal 到达 tick 边界
- **THEN** proposal MUST 被丢弃，MUST NOT建立 accepted reservation、commit memory 或广播台词

#### Scenario: 模型失败不影响任务状态

- **GIVEN** Agent 或模型对台词请求持续失败
- **WHEN** 伙伴完成一个完整任务
- **THEN** 任务状态机、事实 ChatEvent 序列与 FIFO 推进 MUST 与无台词系统一致，仅缺少模型台词事件

#### Scenario: 慢台词不阻塞权威 tick

- **GIVEN** 四个伙伴各有在途 Dialogue run 且模型挂起
- **WHEN** 权威模拟连续推进多个 tick
- **THEN** 每个 tick MUST 按既有节拍完成，玩家命令、伙伴任务与世界模拟 MUST 不受影响

### Requirement: 台词与摘要响应严格解码

Agent Dialogue 响应与 Go 接收值 MUST 严格表示单一 JSON object，拒绝未知字段、尾随数据并在分配前限制正文为 64 KiB。非终态响应 MUST 只包含 `line` 与关联身份；终态响应 MUST 包含 `line`、关联身份与 `memory_proposal{operation_id,base_revision,summary}`。`line` MUST 是 1..256 bytes 的有效 UTF-8，不含 NUL 或 Unicode control，且首尾 MUST NOT 是 Unicode whitespace；`summary` MUST 是不超过 2,048 bytes 的有效 UTF-8 且不含 NUL。任何解码、关联或校验失败 MUST 只跳过台词；终态失败 MUST 保留双方旧 memory。解码文本 MUST 复制为接收方拥有的不可变值，MUST NOT 被自动 trim、清洗或截断。

#### Scenario: 严格解码拒绝未知字段与尾随数据

- **GIVEN** 三份 Dialogue 响应分别包含未知字段、JSON object 后尾随数据与超过 64 KiB 的正文
- **WHEN** Go 解码响应
- **THEN** 三者 MUST 全部只导致台词跳过，MUST NOT 产生 accepted reservation、部分台词或 memory 更新

#### Scenario: 超长台词被拒绝

- **GIVEN** 一份响应的 `line` 为 257 bytes 或含 Unicode control
- **WHEN** Agent 或 Go 校验响应
- **THEN** 该台词 MUST 被跳过，MUST NOT 广播截断或清洗后的文本

#### Scenario: 终态失败保留旧摘要

- **GIVEN** 一个伙伴已有旧摘要且终态 Dialogue 因超时、非法 proposal 或关联错误失败
- **WHEN** 失败回到 Go tick 边界
- **THEN** Python memory 与 Go v5 镜像 MUST 保持旧值，终态事实事件与 FIFO MUST 照常推进，模型台词 MUST 不广播

### Requirement: 终态摘要持久且只喂 Dialogue

最近对话摘要 SHALL 只由终态 Dialogue 的 memory proposal 经两阶段 CAS 更新，每伙伴一份、不超过 2,048 bytes，MUST NOT 发起摘要专用模型请求。Go 在第一次 tick 边界确认 task/node/generation/受众有效后 MUST 建立 accepted reservation；accepted 期间 MUST 暂停该伙伴其他 Dialogue 但不得阻塞任务或 FIFO。Python commit 成功且 operation/epoch 与 reservation 匹配时，Go MUST 更新 v5 恢复镜像并广播保留的 `line`；后续任务造成 generation 变化 MUST NOT 撤销已接受并提交的 proposal。commit 未确认、冲突或 I/O 失败时 MUST 不广播该台词并暂停 Dialogue 直至 reconcile。完整玩家聊天与逐条台词 MUST NOT 落盘；摘要 MUST 只作为后续 Dialogue 输入，MUST NOT 进入 Planner、事实事件或世界行为。inactive 记录 MUST 不保存摘要且 MUST 通过 epoch/tombstone 阻止旧摘要恢复。

#### Scenario: 摘要跨重启进入台词输入

- **GIVEN** 一个终态 proposal 已成功 commit 并更新 Go v5 镜像
- **WHEN** 双方重启、reconcile 后该伙伴发起下一次 Dialogue
- **THEN** 模型输入 MUST 携带相同 committed 摘要且不超过 2,048 bytes，revision MUST 从已提交值继续

#### Scenario: 摘要绝不进入规划

- **GIVEN** Python memory 与 Go v5 镜像均持有非空摘要且新任务进入 Planning
- **WHEN** Planner 构造 snapshot、HTTP 请求、MCP 调用与模型输入
- **THEN** 任一路径 MUST 不包含摘要或其派生文本

#### Scenario: accepted 后新任务不丢失已提交台词

- **GIVEN** 终态 proposal 已建立 accepted reservation，随后 FIFO 新任务改变 generation
- **WHEN** 匹配 operation/epoch 的 commit 成功结果到达
- **THEN** 台词 MUST 广播且镜像 MUST 更新，新任务 MUST 持续推进；该伙伴只有在 reservation 解除后才能发起下一次 Dialogue

#### Scenario: commit 不明时不广播

- **GIVEN** Python 已可能完成 commit，但 Go 未收到成功响应
- **WHEN** HTTP 断开或 deadline 到达
- **THEN** Go MUST 不广播台词并保留 accepted reservation，后续 MUST 用同 operation reconcile/幂等确认，MUST NOT重新调用模型

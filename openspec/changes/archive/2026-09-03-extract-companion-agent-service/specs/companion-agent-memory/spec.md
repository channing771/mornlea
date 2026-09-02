## Purpose

定义伙伴 Dialogue 的有界摘要记忆、版本化提交与跨 Go/Python 恢复协议，使 Python 在运行期保持唯一权威，同时避免迟到台词、复制世界或停用伙伴复活旧记忆。

## ADDED Requirements

### Requirement: 运行期 memory 只含有界摘要和幂等元数据

Agent 服务 SHALL 为每个 `AgentNamespaceID + CompanionID + MemoryEpoch` 维护一条 SQLite MemoryState，只包含有效 UTF-8 且不超过 2,048 bytes 的摘要、单调 `revision` 与最近提交 `operation_id`。Planner graph MUST 不配置持久 checkpointer，Dialogue graph MUST 只使用不落盘的 transient state；完整玩家聊天、逐条台词、persona、Planner snapshot、模型消息、计划、任务和 FIFO MUST NOT 进入 SQLite。这些值 MUST 只作为单次 runtime context 使用。Planner MUST NOT 读取摘要或其派生文本。

#### Scenario: graph 上下文不落盘

- **GIVEN** 一次 Dialogue 请求包含 persona、事实节点、附近环境与既有摘要
- **WHEN** run 完成并检查 Agent SQLite
- **THEN** SQLite MUST 只含 compact MemoryState 与 CAS/namespace lease 所需元数据，MUST NOT 含 graph checkpoint、persona、事实节点、环境、模型消息或原始台词

#### Scenario: 摘要绝不进入 Planner

- **GIVEN** 一个伙伴在 Agent 服务中持有非空摘要
- **WHEN** 同一伙伴发起 Planner run
- **THEN** Planner prompt、MCP 参数与模型可见工具结果 MUST 不包含摘要文本或其派生内容

### Requirement: 终态摘要使用两阶段 CAS 提交

终态 Dialogue SHALL 返回严格的 `line` 与 `memory_proposal{operation_id,base_revision,summary}`，但 MUST NOT 在生成响应时修改 MemoryState。Go MUST 在权威 tick 边界重验任务、节点、generation 与受众资格；仅有效结果 MAY 建立稳定 accepted reservation 并发起 memory commit。accepted reservation MUST 暂停该伙伴后续 Dialogue，但 MUST NOT 阻塞任务状态机或 FIFO；其后新任务导致的 generation 变化 MUST NOT 撤销已接受 proposal。commit MUST 以当前 lease、namespace、companion、epoch、base revision 和 operation ID 做 CAS；成功或完全相同参数/摘要的同 operation 幂等重放 MUST 返回同一 committed revision，同 operation 但 lease、epoch、base revision 或摘要不同 MUST 返回 conflict。revision 到达 `uint64` 最大值时 MUST 硬失败且保持旧 state。Go MUST 只在 commit 成功结果回到 tick 边界且 operation 与 epoch 匹配 accepted reservation 后广播台词并更新恢复镜像。

#### Scenario: 首次重验前过时的终态不提交也不广播

- **GIVEN** 终态 Dialogue 在途期间 queue generation 或节点身份发生变化
- **WHEN** proposal 到达 Go tick 边界
- **THEN** Go MUST 丢弃 proposal，不调用 commit、不广播台词且不改变 Python memory 或 v5 镜像

#### Scenario: accepted commit 不被后续任务撤销

- **GIVEN** 一个终态 proposal 已在 Go tick 边界通过重验并建立 accepted reservation，随后 FIFO 推进使 generation 变化
- **WHEN** 匹配 operation 与 epoch 的 commit 成功结果返回
- **THEN** Go MUST 广播保留的台词并更新 v5 镜像，MUST NOT 因新 generation 丢弃已提交 memory；accepted 期间新任务 MUST 正常推进但该伙伴不得发起其他 Dialogue

#### Scenario: commit 响应丢失可幂等恢复

- **GIVEN** Agent 已提交 operation 但成功响应在网络中丢失
- **WHEN** Go 以相同 operation ID 重试 commit 或执行 reconcile
- **THEN** Agent MUST 返回原 committed revision，MUST NOT 再次递增或生成分叉摘要；Go 只有确认提交后才可广播一次台词

#### Scenario: 同 operation 不同载荷发生冲突

- **GIVEN** 一个 operation 已提交，随后相同 operation ID 携带不同 epoch、base revision、lease 或摘要
- **WHEN** Agent 执行 commit
- **THEN** Agent MUST 返回 `memory_conflict`，revision 与摘要 MUST 保持不变

#### Scenario: revision 溢出硬失败

- **GIVEN** 当前 memory revision 已为 `uint64` 最大值
- **WHEN** 新 operation 尝试 commit
- **THEN** commit MUST 硬失败并保留旧 memory，MUST NOT回绕到零或广播 proposal 台词

### Requirement: Python 是运行期权威而 Go 镜像只用于恢复

Agent 服务可用时，Dialogue SHALL 只读取 Python MemoryState；Go `companions.ai` v5 镜像 MUST NOT 作为正常提示来源。namespace acquire 后双方 MUST reconcile 每个 active 伙伴：Python 缺失或 revision 较低时 MUST 从 Go 镜像恢复；Python revision 较高时 MUST 把其状态返回给 Go 更新镜像；双方相等时 MUST no-op。同 epoch、同 revision 但 operation 或摘要字节不同 MUST 报告 `memory_conflict`，MUST NOT last-write-wins。Go 的身体、任务或 FIFO autosave MUST 把已有镜像当作不可分割的 opaque metadata 原样携带，MUST NOT 从旧 direct Dialogue 的裸 summary 推导 revision/operation 或改写镜像；只有 Agent commit/reconcile 成功结果回到权威 tick 边界并通过 epoch 以及该状态适用的 replay identity 关联后，Go 才可整体更新镜像并 mark dirty。active canonical-zero 的 replay identity 是 namespace、companion、epoch、active 与 canonical-zero 五元状态而不是 operation；active nonzero 使用 mirror operation，inactive 使用 tombstone operation。

#### Scenario: Agent SQLite 丢失后由镜像恢复

- **GIVEN** Go v5 保存 revision 7 的摘要而 Agent 数据库中该 thread 缺失
- **WHEN** namespace acquire 后 reconcile
- **THEN** Agent MUST 以相同 epoch、revision、operation 和摘要恢复 thread，后续 Dialogue MUST 从 revision 7 继续

#### Scenario: 同 revision 内容分叉

- **GIVEN** Go 与 Python 对同一 epoch/revision 保存了不同 operation 或摘要
- **WHEN** reconcile
- **THEN** 双方 MUST 保持原值并返回 `memory_conflict`；该伙伴 Dialogue MUST 暂停，Planner、任务、FIFO 与世界 tick MUST 继续

#### Scenario: 任务 autosave 不伪造 memory operation

- **GIVEN** Go v5 镜像保存 revision 7、operation 与摘要，随后只有身体、任务、FIFO 或旧 direct Dialogue 裸 summary 发生变化
- **WHEN** 伙伴 persistence autosave 或 Flush
- **THEN** 既有 epoch、revision、operation 与摘要 MUST 逐字段保持，MUST NOT 从裸 summary 生成 operation；只有后续 Agent commit/reconcile 成功回到 tick 边界并匹配该状态适用的 replay identity 才可整体替换镜像

### Requirement: MemoryEpoch 与 tombstone 阻止旧记忆复活

Go SHALL 是伙伴 lifecycle 与 `MemoryEpoch` 的唯一权威；active↔inactive 每次转换都 MUST 推进 epoch，推进将溢出 `uint64` 时 MUST 硬失败并保留旧状态。伙伴变为 inactive 时 MUST 持久化幂等 delete tombstone，inactive 记录 MUST 不保存摘要；重新 active 时 MUST 使用再次推进的新 epoch 与空摘要。Agent delete 可在服务恢复后重放，即使旧 thread 仍存在，旧 epoch 的 proposal、commit 或 reconcile MUST 被拒绝。当本次配置没有伙伴时，Go MUST 先执行不读取或解码正文的 metadata-only existence probe；已有 `companions.ai` 时 MUST 加载文件、把原 active 记录转为带新 epoch/tombstone 的 inactive 记录并同步写回，随后 MUST 不启动 MCP 或联系 Agent；文件不存在时 MUST 除该 probe 外不读取、创建或保存 `companions.ai`。

reconcile SHALL 按 epoch、active/tombstone、revision 的顺序裁决。Go `memory_epoch` 高于 Python 时，该 higher epoch 本身 MUST 在同一事务中 fence 全部旧 epoch 并原子恢复 Go 当前状态，不要求 HTTP 或 v5 layout 之外的 transition operation：higher active canonical-zero 以精确 `{namespace, companion, epoch, active=true, revision=0, operation=null, summary=""}` 状态作为 replay key；higher active nonzero 以 mirror operation 和完整 mirror 作为 replay identity；higher inactive 以 tombstone operation 和 inactive state 作为 replay identity。Agent 离线期间 Go 连续推进多个合法 lifecycle epoch 时，reconcile MAY 从 Agent 的旧 epoch 直接跳到 Go 当前 higher epoch，无需先重放已被后续 active 状态取代的中间 tombstone。Python epoch 高于 Go，或同 epoch 的 active/tombstone 状态不同，MUST 返回 `memory_conflict`。同 epoch active 才比较 revision、operation 与 summary；同 epoch inactive 只按 tombstone operation 幂等重放。相同 nonzero mirror/tombstone operation 与载荷的重放，以及相同 active canonical-zero replay key 的重放，MUST no-op 成功；相同 operation 不同载荷 MUST conflict。合法 higher epoch 或新 tombstone MUST 优先于任何旧 epoch commit/reconcile，不同 epoch 的 revision MUST NOT 相互比较。

#### Scenario: 离线停用后重新启用

- **GIVEN** Agent 服务离线时一个带摘要的伙伴被停用，随后以同一 CompanionID 重新启用
- **WHEN** Agent 服务恢复并执行 reconcile/delete
- **THEN** 新 active 伙伴 MUST 使用推进后的 epoch 与空摘要，旧摘要 MUST 不进入 Dialogue，旧 epoch thread MAY 被物理清理但 MUST 永远不可重新绑定

#### Scenario: 离线跨两次 lifecycle 后以 active canonical-zero fence 旧 epoch

- **GIVEN** Agent 离线时仍保存伙伴 active epoch N，Go 已依次持久化 inactive epoch N+1 与 tombstone、再持久化 active epoch N+2 的 canonical-zero memory
- **WHEN** Agent 恢复后 Go 只 reconcile 当前 active epoch N+2，并重放一次完全相同的请求
- **THEN** epoch N+2 MUST 自身 fence 旧 epoch N 并原子建立 active canonical-zero，第二次请求 MUST 按 `{namespace, companion, epoch N+2, active, canonical-zero}` no-op 成功，MUST NOT 要求伪造 operation 或先重放已被取代的 N+1 tombstone；任何迟到 epoch N commit/reconcile MUST 被拒绝

#### Scenario: 迟到旧 epoch commit 被拒绝

- **GIVEN** 一个终态 proposal 生成后伙伴被停用并推进 epoch
- **WHEN** 旧 proposal 的 commit 到达 Agent 服务
- **THEN** commit MUST 返回 memory conflict 或 not found，MUST NOT 写入新 epoch、更新 Go 镜像或广播旧台词

#### Scenario: 全量停用写入 retirement 后不启动 Agent

- **GIVEN** 世界已有包含 active 伙伴的 `companions.ai`，新配置的伙伴列表为空
- **WHEN** Go 启动并完成配置合并
- **THEN** Go MUST 将全部 active 记录转为 inactive、各推进一次 epoch、写入 delete tombstone 并同步保存，已 inactive 记录 MUST 幂等保持原 epoch/tombstone；随后 MUST 保持 AI 关闭且不启动 MCP、不连接 Python；若文件原本不存在则 MUST 只执行 metadata-only existence probe，Load/Save 次数必须为零且不得创建 `companions.ai`

### Requirement: Memory 持久化故障不改变事实平面

Memory reconcile、commit、delete 或 SQLite I/O 失败 MUST 不改变任务状态、FIFO、事实事件或世界状态。非终态与空闲 Dialogue MUST 不更新 memory；终态 commit 未确认时 MUST 不广播对应模型台词，并 MUST 暂停该伙伴后续 Dialogue 直到 reconcile 成功。commit 与 active nonzero reconcile MUST 由 mirror operation 安全重试，delete/inactive reconcile MUST 由 tombstone operation 安全重试，active canonical-zero reconcile MUST 由精确 `{namespace, companion, epoch, active, canonical-zero}` replay key 安全重试；模型生成 MUST NOT 自动重试。

#### Scenario: commit 失败保留任务结果

- **GIVEN** 一个任务已由 Go 权威状态机进入 Completed，但其终态 memory commit 因 SQLite I/O 失败
- **WHEN** 失败回到 Go tick 边界
- **THEN** `TaskCompleted` 事实 MUST 保持，终态模型台词 MUST 不广播，FIFO MUST 继续，后续 Dialogue MUST 暂停等待 reconcile

#### Scenario: 空闲台词不改变 memory

- **GIVEN** 一个合法空闲 Dialogue 返回台词且伙伴已有非空摘要
- **WHEN** Go 在 tick 边界接受并广播台词
- **THEN** Python revision、summary、Go v5 镜像与 tombstone MUST 全部保持不变

## MODIFIED Requirements

### Requirement: 存档格式有界且损坏不可静默覆盖

`companions.ai` MUST 保持 32-byte magic `MCAI` envelope v1，并保存 schema v5、聚合 revision、记录数、payload 长度与覆盖 schema header 和 payload 的 CRC32C。v5 payload MUST 先保存 16-byte canonical UUIDv4 namespace，再按 `CompanionID` 严格升序保存记录；每条记录 MUST 依次保存 221-byte body、1-byte flags 与 `uint64` epoch。flags bit0 MUST 表示 active、bit1 MUST 表示 task、bit2 MUST 表示 FIFO，bit3..7 MUST 为零。active 记录 MUST 再依次保存 `uint64` memory revision、16-byte operation、`uint16` summary length 和 summary bytes，之后才按 flags 保存 task/FIFO；inactive 记录 MUST 只再保存 16-byte tombstone，flags MUST 为零，MUST NOT 携带 mirror、task 或 FIFO。active+inactive 总数 MUST 不超过 64，active MUST 不超过 4。schema v4、v3、v2 与 v1 文件 MUST 按只读迁移加载，encoder MUST 只写 v5，未来版本 MUST 被拒绝。单条记录任务字段 MUST 有界：原始指令与 FIFO 每条指令不超过 1,024 bytes，计划步骤数不超过 5,000，FIFO 不超过 16 条；active memory 恢复镜像摘要不超过 2,048 bytes。v5 `MaxFileLength` MUST 精确为 `32 + 16 + 4×94,774 + 60×246 = 393,904` bytes，其中可达最大 task 为 76,052 bytes、FIFO 为 16,418 bytes；解码 MUST 在 CRC、payload copy 或任何 record/task/slice 分配前拒绝超长。未来版本、CRC 错误、截断、超长、非法数值、非法任务状态、变长步骤错位、非法 namespace/epoch/revision/operation/tombstone、reserved flags、非法字段耦合、非法摘要或非法背包 MUST 被拒绝；保存 MUST NOT 覆盖损坏或未来版本的正式文件。

#### Scenario: 第六十五个身份被拒绝且旧记录保留

- **GIVEN** 存档已经包含 64 个不同的 active+inactive ID
- **WHEN** 配置尝试引入第六十五个新 ID
- **THEN** 启动 MUST 失败，正式文件和全部旧记录 MUST 保持不变

#### Scenario: 损坏文件不被新保存掩盖

- **GIVEN** 正式 `companions.ai` 的 CRC、长度、schema 或 v5 memory 元数据无效
- **WHEN** 服务端加载或尝试保存伙伴状态
- **THEN** 操作 MUST 返回可识别的损坏或未来版本错误，并 MUST NOT 用新文件覆盖正式文件

#### Scenario: v5 最大长度在分配前生效

- **GIVEN** 文件声明合法 schema v5 但物理长度为 393,905 bytes
- **WHEN** decoder 接收文件
- **THEN** decoder MUST 在按记录数、计划或摘要分配内存前拒绝，正式文件 MUST 保持原样

#### Scenario: v5 最大合法结构逐字节可回读

- **GIVEN** 一个 schema v5 文件包含 4 条可达最大 active 记录与 60 条最大 inactive 记录，物理长度精确为 393,904 bytes
- **WHEN** encoder 写出并由 decoder 回读
- **THEN** 文件 MUST 被接受并规范往返，active flags/mirror/task/FIFO 与 inactive tombstone MUST 不错位，encoder MUST NOT 产生更长的合法文件

### Requirement: 配置合并保留 inactive 记录

服务端 SHALL 先验证配置再加载已有 `companions.ai`。已存且仍配置的 ID MUST 恢复身体、任务/FIFO 与 v5 memory 元数据；新配置 ID MUST 以世界 metadata 的 `SpawnDimension`、出生锚区块坐标换算的 `X=SpawnAnchor.X×16+0.5` 与 `Z=SpawnAnchor.Z×16+0.5`、`Y=core.MaxY+1`、零 yaw/pitch、合法空背包创建规范 provisional body，并使用 epoch 1、空任务/FIFO和 canonical-zero memory；missing aggregate MUST 以 revision 1 首存。该 provisional body MUST 在模拟 ready/出生扫描前同步保存，之后才由模拟激活结果覆盖位置。已存但不再配置的 ID MUST 转为 inactive，仅保留身体、推进后的 memory epoch 与 delete tombstone，不得注册到模拟或保存摘要。配置为空时，服务端 MUST 通过 Memory 与 Disk 同语义、且不读取或解码正文的 mandatory metadata existence probe 区分 missing 与 existing：missing MUST 只有 probe、零 Load、零 Save、零文件创建；existing MUST Load，且只把 active 记录退休并在确有变化时同步写出 v5。已有 legacy 即使零记录也 MUST 因 schema/namespace 迁移同步保存 v5；已经全 inactive 的合法 v5 MUST 保持 epoch/revision/tombstone 不变且零 Save。退休完成后 MUST 不构造 persistence worker、world companion、MCP 或 Agent client，tombstone 待未来 Agent 可用时幂等处理。

#### Scenario: 暂时移除配置不会删除身体

- **GIVEN** 存档含一个带任务与摘要镜像的伙伴记录，但当前非空配置没有该 ID
- **WHEN** 服务端启动并保存
- **THEN** 该记录 MUST 作为 inactive 保留身体、推进 epoch 并带 delete tombstone，不得保存任务、FIFO 或摘要，也不得出现在模拟或客户端

#### Scenario: 清空配置保持文件原样

- **GIVEN** 世界目录已有包含 active 记录的 `companions.ai` 且当前 `ai.companions` 为空
- **WHEN** 服务端启动
- **THEN** 服务端 MUST 同步写出全部记录 retirement 的 v5 文件，再保持 AI 关闭且不启动 MCP、不联系 Python

#### Scenario: 空配置且无文件零触碰

- **GIVEN** 当前 `ai.companions` 为空且世界目录没有 `companions.ai`
- **WHEN** 服务端启动并安全关闭
- **THEN** 服务端 MUST 只执行 metadata existence probe，保持 AI 关闭，Load 与 Save 次数均为零且不创建 `companions.ai`，不启动 MCP也不联系 Python

#### Scenario: 已退休 v5 重启不重复推进

- **GIVEN** 当前配置为空且已有合法 schema v5 文件中的记录全部 inactive
- **WHEN** 服务端连续重启两次
- **THEN** 每次 MUST 先 probe 后 Load，MUST NOT Save，aggregate revision、每条 epoch 与 tombstone MUST 逐字节保持不变

## ADDED Requirements

### Requirement: schema v5 保存 Agent namespace 与 memory 恢复元数据

companion schema SHALL 升级到 v5。聚合 MUST 保存稳定 canonical UUIDv4 `AgentNamespaceID`；每条记录 MUST 保存非零 `MemoryEpoch` 与 active/inactive 状态，active↔inactive 每次转换 MUST checked 推进 epoch，达到 `uint64` 最大值时 MUST 返回损坏语义的 overflow 错误而不回绕。active 记录 MUST 保存恢复镜像 `{revision, operation_id, summary}`；规范零 memory MUST 且只能使用零 revision、全零 operation 与空摘要，非零 revision MUST 携带 canonical UUIDv4 operation，summary 必须是无 NUL 的有效 UTF-8 且不超过 2,048 bytes。inactive 记录 MUST 清空 revision、operation 与摘要，MUST 不保存 task/FIFO，且 MUST 保存幂等 delete tombstone。v1..v4 MUST 被视为隐式 epoch 0并在迁移时固定落 epoch 1：v4 active 非空摘要 MUST 原字节成为 revision 1 mirror并生成 fresh memory operation；v1..v3 与 v4 active 空摘要 MUST 使用 canonical-zero memory且不得伪造另一个 active migration operation；legacy inactive MUST 生成 fresh tombstone。identity entropy 消费顺序 MUST 唯一：缺失 namespace 时先且只生成一个 namespace，随后按 `CompanionID` 严格升序遍历，每个本次需要生成的 nonzero mirror operation 或 tombstone 各恰好消费一个 UUID，canonical-zero active 与无需新身份的 unchanged v5 记录 MUST 不消费 entropy。名称、persona、模型消息、credential、MCP capability 与完整聊天 MUST 不写入文件。

#### Scenario: v4 摘要迁移为初始 memory

- **GIVEN** 一个 schema v4 文件含 active 伙伴的非空合法摘要
- **WHEN** 服务端只读迁移到 v5
- **THEN** 身体、任务与 FIFO MUST 无损恢复，服务端 MUST 生成并保存稳定 namespace/epoch/operation，摘要 MUST 原字节成为初始非零 revision 镜像

#### Scenario: v4 空摘要迁移为规范空 memory

- **GIVEN** 一个 schema v4 active 记录没有摘要
- **WHEN** 首次保存为 v5
- **THEN** 该记录 MUST 使用 epoch 1 与规范零 memory，operation MUST 为全零，MUST NOT伪造另一个 active migration operation、非空摘要或 revision

#### Scenario: legacy inactive 使用 retirement operation

- **GIVEN** 一个 v1..v4 文件包含当前配置已移除的伙伴
- **WHEN** 服务端迁移到 v5
- **THEN** 该记录 MUST 从隐式 epoch 0 落为 inactive epoch 1，清除 task/FIFO/summary并生成 fresh tombstone，MUST NOT 保存 active mirror operation

#### Scenario: identity entropy 按规范顺序精确消费

- **GIVEN** 一个缺失 namespace 的 legacy aggregate，按 ID 排序后同时含需要 nonzero mirror operation 的 active、canonical-zero active 与需要 tombstone 的 inactive 记录
- **WHEN** bootstrap 使用可观测 entropy source 迁移到 v5
- **THEN** 第一个 UUID MUST 是 namespace，随后 MUST 只按记录 ID 升序为每个 nonzero mirror operation 或 tombstone 各消费一个 UUID，canonical-zero active MUST 消费零个且整个迁移 MUST 不多消费任何身份

#### Scenario: namespace 在重启后稳定

- **GIVEN** 一个 v5 世界已生成 AgentNamespaceID 并安全保存
- **WHEN** 同一世界多次重启
- **THEN** namespace MUST 逐字节稳定，MUST NOT 因 Agent 服务、配置顺序或进程 instance 变化而重生

### Requirement: v5 身份在首次 Agent 使用前原子落盘

加载任一 v1..v4 文件时，无论当前配置是否含 active 伙伴，服务端 MUST 生成缺失的 namespace、epoch 以及按上一要求合法存在的 memory operation 或 retirement tombstone，并以既有原子替换纪律同步保存 v5；零伙伴配置完成 retirement 后不联系 Agent。新世界或新伙伴没有既有 v5 identity 时，也 MUST 把规范 provisional body、稳定 namespace、epoch 与 canonical-zero memory同步原子保存，再构造 persistence worker、world/simulation、Agent 或 MCP。identity entropy、capacity 或 epoch/aggregate revision overflow MUST 在 Save 前令启动失败，输入值与旧正式字节保持不变；任一同步 Save 错误也 MUST 令启动失败，且所有这些失败的后续 persistence/world/Agent/MCP 构造次数 MUST 为零。仅 pre-rename 的 temp create/write/sync/close 或 rename 失败 MUST 保证旧正式 v4 字节不变；rename 成功后的 parent directory sync 失败 MUST 仍返回错误并停止启动，此时 official path MUST 是完整可解码 v5 而非半文件，MUST NOT 再要求旧 v4 保持。v5 不提供向 v4 的降级写回；回滚旧程序必须同时恢复事先备份的 v4 文件和旧配置。

#### Scenario: pre-rename 迁移失败保留旧 v4 且不联系 Agent

- **GIVEN** 一个合法 v4 文件与 active 配置，但 v5 原子替换在 temp create/write/sync/close 或 rename 阶段失败
- **WHEN** 服务端启动
- **THEN** 启动 MUST 失败，正式 v4 文件 MUST 保持原样，MCP 与 Agent HTTP MUST 未启动或未调用

#### Scenario: parent directory sync 失败只留下完整 v5

- **GIVEN** 一个合法 v4 文件已成功 rename 为完整 v5，但随后 parent directory sync 失败
- **WHEN** 同步 bootstrap Save 返回
- **THEN** 启动 MUST 失败且 persistence/world/Agent/MCP MUST 未构造，official path MUST 只包含完整可 decode 的 v5，MUST NOT 包含半文件或被要求恢复为旧 v4

#### Scenario: 零伙伴旧文件也完成 v5 retirement

- **GIVEN** 世界已有 v1..v4 文件且当前伙伴配置为空
- **WHEN** 服务端启动
- **THEN** 服务端 MUST 生成 namespace/epoch/tombstone 并原子写 v5，失败则启动失败；成功后 MUST 不启动 MCP或联系 Agent

#### Scenario: 新世界身份先落盘再 acquire

- **GIVEN** 新世界首次配置伙伴且尚无 `companions.ai`
- **WHEN** 服务端准备启动 Agent bridge
- **THEN** provisional body、namespace、每个伙伴 epoch 1 与 canonical-zero memory MUST 先同步原子写入 v5，随后才可构造 persistence/world/Agent/MCP 或发出 Agent 请求；模拟 ready 后的权威身体 MUST 覆盖 provisional position

#### Scenario: aggregate revision overflow 原子失败

- **GIVEN** 一个需要迁移、lifecycle transition 或新增伙伴的 aggregate revision 已为 `uint64` 最大值
- **WHEN** bootstrap 尝试产生 v5 保存
- **THEN** 操作 MUST 返回损坏语义的 overflow 错误，不消费 identity entropy、不修改输入、不调用 Save，后续 persistence/world/Agent/MCP MUST 全部未构造

#### Scenario: 回滚不由新程序降级写回

- **GIVEN** 世界已成功写为 schema v5
- **WHEN** 操作者尝试用只支持 v4 的旧程序打开
- **THEN** 旧程序 MUST 按未来版本拒绝；恢复旧版本必须使用独立备份，当前程序 MUST 不生成 v4 降级文件

### Requirement: v5 元数据跨身体与任务保存无损携带

伙伴 persistence SHALL 在身体、任务、FIFO 的 autosave、retry 与 Flush 中深拷贝并逐字段携带 namespace、lifecycle、epoch、active mirror 或 inactive tombstone。身体或任务观察 MUST NOT 从旧 direct Dialogue 的裸 `Summary string` 推导或改写 v5 mirror，也 MUST NOT 伪造 revision/operation；该裸 summary 只可 transient 使用。系统 MUST 只在 Agent memory commit/reconcile 成功结果回到权威 tick 边界并通过 epoch 与该状态适用的 replay identity 关联后，整体替换 `{epoch, revision, operation, summary}` 并 mark dirty。aggregate revision 的每次持久 mutation MUST checked 增加一次；达到 `uint64` 最大值时 MUST 返回损坏语义的 overflow 错误，不 dispatch Save、不回绕且不丢失既有 metadata。

#### Scenario: 身体与任务 autosave 保持 mirror

- **GIVEN** 一个 active v5 记录持有 revision 7 的恢复 mirror，随后只有身体位置、任务或 FIFO 发生变化
- **WHEN** persistence autosave 或 Flush 写出下一 revision
- **THEN** namespace、lifecycle、epoch、memory revision、operation 与 summary MUST 逐字段保持，inactive metadata MUST 同样不丢失，保存 MUST NOT 根据裸 Dialogue summary 生成新 operation

#### Scenario: persistence revision overflow 不派发

- **GIVEN** persistence 已持久化 aggregate revision 为 `uint64` 最大值且收到新的身体或任务 dirty observation
- **WHEN** Poll 或 Flush 尝试构造下一保存
- **THEN** 调用 MUST 返回损坏语义的 overflow 错误，Save 调用次数 MUST 为零，revision MUST NOT 回绕且既有 namespace/lifecycle/mirror/tombstone MUST 保持可重试

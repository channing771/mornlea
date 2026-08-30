## MODIFIED Requirements

### Requirement: 存档格式有界且损坏不可静默覆盖

`companions.ai` MUST 使用 magic `MCAI`、envelope v1、schema v5、聚合 revision、记录数、payload 长度与覆盖 schema header 和 payload 的 CRC32C。schema v4、v3、v2 与 v1 文件 MUST 按只读迁移加载，未来版本 MUST 被拒绝。记录 MUST 按 `CompanionID` 严格升序且不重复，active+inactive 总数 MUST 不超过 64。单条记录任务字段 MUST 有界：原始指令与 FIFO 每条指令不超过 1,024 bytes，计划步骤数不超过 5,000，FIFO 不超过 16 条；active memory 恢复镜像摘要不超过 2,048 bytes。实现 MUST 从 v5 固定字段和所有有界变长字段重新推导并钉住物理文件最大长度，解码 MUST 在任何解析与分配前拒绝超长。未来版本、CRC 错误、截断、超长、非法数值、非法任务状态、变长步骤错位、非法 namespace/epoch/revision/operation/tombstone、非法摘要或非法背包 MUST 被拒绝；保存 MUST NOT 覆盖损坏或未来版本的正式文件。

#### Scenario: 第六十五个身份被拒绝且旧记录保留

- **GIVEN** 存档已经包含 64 个不同的 active+inactive ID
- **WHEN** 配置尝试引入第六十五个新 ID
- **THEN** 启动 MUST 失败，正式文件和全部旧记录 MUST 保持不变

#### Scenario: 损坏文件不被新保存掩盖

- **GIVEN** 正式 `companions.ai` 的 CRC、长度、schema 或 v5 memory 元数据无效
- **WHEN** 服务端加载或尝试保存伙伴状态
- **THEN** 操作 MUST 返回可识别的损坏或未来版本错误，并 MUST NOT 用新文件覆盖正式文件

#### Scenario: v5 最大长度在分配前生效

- **GIVEN** 文件声明合法 schema v5 但物理长度超过重新推导的固定最大值
- **WHEN** decoder 接收文件
- **THEN** decoder MUST 在按记录数、计划或摘要分配内存前拒绝，正式文件 MUST 保持原样

### Requirement: 配置合并保留 inactive 记录

服务端 SHALL 先验证配置再加载已有 `companions.ai`。已存且仍配置的 ID MUST 恢复身体、任务/FIFO 与 v5 memory 元数据；新配置 ID MUST 从世界出生点创建空背包、无任务身体、新 epoch 和空 memory；已存但不再配置的 ID MUST 转为 inactive，仅保留身体、推进后的 memory epoch 与 delete tombstone，不得注册到模拟或保存摘要。当配置为空且文件存在时，服务端 MUST 读取文件、把所有 active 记录退休并同步写出 v5 后保持 AI 关闭；文件不存在时 MUST 不读取、创建或保存 `companions.ai`，但允许用于判定存在性的 metadata probe。退休完成后 MUST 不启动 MCP 或联系 Agent，tombstone 待未来 Agent 可用时幂等处理。

#### Scenario: 暂时移除配置保留身体并退休 memory

- **GIVEN** 存档含一个带任务与摘要镜像的伙伴记录，但当前非空配置没有该 ID
- **WHEN** 服务端启动并保存
- **THEN** 该记录 MUST 作为 inactive 保留身体、推进 epoch 并带 delete tombstone，不得保存任务、FIFO 或摘要，也不得出现在模拟或客户端

#### Scenario: 清空配置同步退休已有文件

- **GIVEN** 世界目录已有包含 active 记录的 `companions.ai` 且当前 `ai.companions` 为空
- **WHEN** 服务端启动
- **THEN** 服务端 MUST 同步写出全部记录 retirement 的 v5 文件，再保持 AI 关闭且不启动 MCP、不联系 Python

#### Scenario: 空配置且无文件零触碰

- **GIVEN** 当前 `ai.companions` 为空且世界目录没有 `companions.ai`
- **WHEN** 服务端启动并安全关闭
- **THEN** 服务端 MUST 保持 AI 关闭，不读取、创建或保存 `companions.ai`（允许存在性 metadata probe），不启动 MCP也不联系 Python

## ADDED Requirements

### Requirement: schema v5 保存 Agent namespace 与 memory 恢复元数据

companion schema SHALL 升级到 v5。聚合 MUST 保存稳定 canonical UUIDv4 `AgentNamespaceID`；每条记录 MUST 保存非零 `MemoryEpoch` 与 active/inactive 状态，active↔inactive 每次转换 MUST 推进 epoch，达到 `uint64` 最大值时 MUST 硬失败而不回绕。active 记录 MAY 保存恢复镜像 `{revision, operation_id, summary}`，其中 revision 为单调 `uint64`、operation ID 为 canonical UUIDv4、summary 为有效 UTF-8 且不超过 2,048 bytes；空 memory MUST 使用规范零 revision/空 operation/空摘要表示。inactive 记录 MUST 不保存摘要或 active operation，且 MUST 保存幂等 delete tombstone。名称、persona、模型消息、credential、MCP capability 与完整聊天 MUST 不写入文件。

#### Scenario: v4 摘要迁移为初始 memory

- **GIVEN** 一个 schema v4 文件含 active 伙伴的非空合法摘要
- **WHEN** 服务端只读迁移到 v5
- **THEN** 身体、任务与 FIFO MUST 无损恢复，服务端 MUST 生成并保存稳定 namespace/epoch/operation，摘要 MUST 原字节成为初始非零 revision 镜像

#### Scenario: v4 空摘要迁移为规范空 memory

- **GIVEN** 一个 schema v4 active 记录没有摘要
- **WHEN** 首次保存为 v5
- **THEN** 该记录 MUST 使用非零 epoch 与规范零 memory，MUST NOT伪造非空摘要或 revision

#### Scenario: namespace 在重启后稳定

- **GIVEN** 一个 v5 世界已生成 AgentNamespaceID 并安全保存
- **WHEN** 同一世界多次重启
- **THEN** namespace MUST 逐字节稳定，MUST NOT 因 Agent 服务、配置顺序或进程 instance 变化而重生

### Requirement: v5 身份在首次 Agent 使用前原子落盘

加载任一 v1..v4 文件时，无论当前配置是否含 active 伙伴，服务端 MUST 生成缺失的 namespace、epoch 与幂等 migration/retirement operation，并以既有原子替换纪律同步保存 v5；零伙伴配置完成 retirement 后不联系 Agent。新世界或新伙伴没有既有 v5 identity 时，也 MUST 在 acquire namespace、启动 MCP 或发出任一 Agent 请求前同步原子保存稳定 namespace/epoch。生成与保存失败 MUST 令启动失败，旧正式文件 MUST 保持可读且 MUST NOT 以进程内临时身份联系 Agent。v5 不提供向 v4 的降级写回；回滚旧程序必须同时恢复事先备份的 v4 文件和旧配置。

#### Scenario: 迁移保存失败不联系 Agent

- **GIVEN** 一个合法 v4 文件与 active 配置，但 v5 原子替换被注入 I/O 失败
- **WHEN** 服务端启动
- **THEN** 启动 MUST 失败，正式 v4 文件 MUST 保持原样，MCP 与 Agent HTTP MUST 未启动或未调用

#### Scenario: 零伙伴旧文件也完成 v5 retirement

- **GIVEN** 世界已有 v1..v4 文件且当前伙伴配置为空
- **WHEN** 服务端启动
- **THEN** 服务端 MUST 生成 namespace/epoch/tombstone 并原子写 v5，失败则启动失败；成功后 MUST 不启动 MCP或联系 Agent

#### Scenario: 新世界身份先落盘再 acquire

- **GIVEN** 新世界首次配置伙伴且尚无 `companions.ai`
- **WHEN** 服务端准备启动 Agent bridge
- **THEN** namespace 与每个伙伴初始 epoch MUST 先同步原子写入 v5，随后才可 acquire、启动 MCP 或发出 Agent 请求

#### Scenario: 回滚不由新程序降级写回

- **GIVEN** 世界已成功写为 schema v5
- **WHEN** 操作者尝试用只支持 v4 的旧程序打开
- **THEN** 旧程序 MUST 按未来版本拒绝；恢复旧版本必须使用独立备份，当前程序 MUST 不生成 v4 降级文件

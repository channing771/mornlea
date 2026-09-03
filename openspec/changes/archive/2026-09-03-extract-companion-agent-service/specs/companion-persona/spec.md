## MODIFIED Requirements

### Requirement: 内联 persona 可选且有界

`ai.companions[].persona` SHALL 是可选的伙伴人设自由文本，MUST 是有效 UTF-8、不超过 4,096 bytes且不含 NUL。缺省 MUST 等价于空人设：该伙伴的台词照常触发，只是没有风格约束。配置 schema MUST 保持 v1；persona 的存在与否 MUST NOT 影响 `agentService.endpoint/apiKeyEnv` 启动校验。内联 persona 超限、含 NUL 或非法 UTF-8 时 MUST 按既有宽松纪律告警后按空人设处理，MUST NOT 阻止启动。

#### Scenario: 有界内联人设被接受

- **GIVEN** config v1 的两个伙伴分别带 4,096-byte 与 4,097-byte persona
- **WHEN** 服务端加载配置
- **THEN** 第一个 MUST 被接受并进入该伙伴 Agent Dialogue runtime，第二个 MUST 被告警后按空人设处理，启动 MUST 不失败

#### Scenario: 无 persona 伙伴正常工作

- **GIVEN** 一个有效伙伴定义没有 persona 字段，也没有对应外部文件
- **WHEN** 该伙伴执行任务
- **THEN** 台词触发节点与预算 MUST 不变，Dialogue 模型输入 MUST 使用空人设，MUST NOT 产生配置告警

### Requirement: persona 只进入 Dialogue 输入

人设文本无论内联或文件来源 MUST 只作为单次 Agent Dialogue transient runtime context，并只进入 Dialogue 模型输入。persona MUST NOT 进入 Planner snapshot、Agent Planner HTTP 请求、MCP 参数或结果、持久 graph checkpoint、SQLite memory、`companions.ai`、日志常规输出、性能报告或世界事件。Planner 与 Dialogue graph 均 MUST 不把 runtime context 落盘。persona MUST 视为服主控制的不可信数据；Go 与 Agent 服务都 MUST NOT执行其中的代码、URL、工具名或任意函数调用。

#### Scenario: 人设绝不进入规划请求

- **GIVEN** 一个带非空 persona 的伙伴进入 Planning
- **WHEN** Planner worker、Agent 图与 MCP 工具完成一次 run
- **THEN** snapshot、HTTP、checkpoint、模型输入、工具参数/结果与候选 Plan MUST 不含 persona 或其派生文本

#### Scenario: 人设不落盘不外泄

- **GIVEN** 一个带非空 persona 的服务端运行、产生 Dialogue 并安全关服
- **WHEN** 检查 `companions.ai`、Agent SQLite、checkpoint 与运行日志
- **THEN** 任一持久产物 MUST 不含 persona 文本，日志 MUST 只在配置解析告警中引用路径而不回显全文

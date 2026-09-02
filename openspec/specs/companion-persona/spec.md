# companion-persona Specification

## Purpose
TBD - created by archiving change m5d-companion-persona-dialogue. Update Purpose after archive.
## Requirements
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

### Requirement: persona 可从约定目录的外部文件读取

服务端 SHALL 在启动加载配置时按约定目录解析外部人设文件：目录为配置文件所在目录下的 `personas/`，文件名为 `<canonical 伙伴名称>.txt`。内联 `persona` 字段存在时 MUST 优先生效；此时若同名外部文件也存在，MUST `slog.Warn` 提示该文件被忽略。内联缺失时 MUST 读取该文件（若存在）；文件不存在 MUST 静默得到空人设。文件不可读、超过 4,096 bytes、非法 UTF-8 或含 NUL 时 MUST `slog.Warn` 后按空人设处理，MUST NOT 阻止启动。外部文件 MUST 只在启动时读取一次，运行期 MUST NOT 热更新；文件名 MUST 由已验证的 canonical 名称构成，MUST NOT 接受路径穿越。

#### Scenario: 外部文件提供人设

- **GIVEN** 配置文件位于 `world/config.json`，伙伴 `阿木` 无内联 persona，且 `world/personas/阿木.txt` 存在且为 1 KiB 合法文本
- **WHEN** 服务端启动加载配置
- **THEN** `阿木` 的 Dialogue 请求 MUST 携带该文件内容作为人设

#### Scenario: 内联优先并告警忽略文件

- **GIVEN** 伙伴 `阿木` 同时配置了内联 persona 与存在的 `personas/阿木.txt`
- **WHEN** 服务端启动加载配置
- **THEN** 内联 persona MUST 生效，日志 MUST 告警外部文件被忽略，启动 MUST 不失败

#### Scenario: 损坏文件按空人设降级

- **GIVEN** `personas/阿木.txt` 为 5 KiB 或含 NUL 字节
- **WHEN** 服务端启动加载配置
- **THEN** 系统 MUST 告警精确文件路径并按空人设继续，启动 MUST 不失败

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


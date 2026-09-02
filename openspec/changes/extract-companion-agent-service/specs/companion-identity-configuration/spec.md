## MODIFIED Requirements

### Requirement: AI 伙伴配置可选且数量有界

配置 schema SHALL 保持 v1，并 MAY 包含可选的 `ai` 组与 `ai.companions`。缺少 `ai`、`ai` 为 `null`、缺少 `companions` 或伙伴列表为空时，AI MUST 关闭，并 MUST 只通过不读取或解码正文的 metadata-only existence probe 判断是否已有 `companions.ai`；不存在文件时 MUST 不要求 Agent endpoint、key 或 timeout，也不得 Load/Save/create companion 存档。非空列表 MUST 包含 `1..4` 个有效定义。非空伙伴配置 MUST 包含 `ai.agentService.endpoint` 与 `ai.agentService.apiKeyEnv`，且环境变量值非空；缺少任一项时内置与专用服务端 MUST 启动失败。`taskTimeoutMinutes` MUST 是 `1..60` 的整数，缺省为 10。`ai.companions[]` SHALL 继续识别可选 `persona`；尚未交付的其他字段 MUST 按既有未知字段纪律告警后忽略。空配置但已有 `companions.ai` 的 retirement 行为由 `companion-persistence` 定义，完成后 MUST 保持 AI 关闭。

#### Scenario: 空配置且无存档保持 AI 关闭

- **GIVEN** 一份有效 config v1 没有伙伴，且世界目录没有 `companions.ai`
- **WHEN** 内置或专用服务端读取配置
- **THEN** 服务端 MUST 只执行 metadata-only existence probe 并保持 AI 关闭，不创建伙伴，Load/Save 次数均为零且不创建 `companions.ai`，不启动 MCP或联系 Python，也不得要求 Agent endpoint、key、timeout 或 persona

#### Scenario: 超过四个定义被拒绝

- **GIVEN** 一份 config v1 包含五个分别有效的伙伴定义
- **WHEN** 服务端验证配置
- **THEN** 启动 MUST 失败，且不得只激活前四个伙伴

#### Scenario: 缺 Agent 服务配置的伙伴被拒绝启动

- **GIVEN** 一份配置包含两个有效伙伴定义但缺少 `agentService.endpoint` 或 `agentService.apiKeyEnv`
- **WHEN** 内置或专用服务端启动
- **THEN** 启动 MUST 失败并给出可定位错误，MUST NOT 以关闭 AI、移除伙伴或 direct-model fallback 继续

#### Scenario: 任务时长边界生效

- **GIVEN** `taskTimeoutMinutes` 分别为 0、1、60 与 61
- **WHEN** 服务端验证配置
- **THEN** 0 与 61 MUST 被拒绝，1 与 60 MUST 被接受，缺省值 MUST 为 10

#### Scenario: 后续字段不提前启用

- **GIVEN** 一个伙伴定义或 `ai` 组包含 persona 与已交付 Agent 字段之外的未知字段
- **WHEN** 读取配置
- **THEN** 系统 MUST 对精确字段路径告警并忽略未知字段，且有效结果 MUST 不变；`persona` 与 `agentService` MUST 按各自规则解析

### Requirement: 模型 endpoint 与密钥边界受严格约束

Go 配置中的 `ai.agentService.endpoint` MUST 是无 userinfo、query 与 fragment、host 为 loopback IP 字面量的 `http` URL；第一阶段 MUST 拒绝 hostname、非 loopback、`https` 远程 endpoint 与重定向。`ai.agentService.apiKeyEnv` MUST 命名一个非空环境变量，Go MUST 仅将其作为 Agent HTTP Bearer credential，MUST NOT 把它用作模型 key。模型 base URL、model 与 provider key MUST 只由 Python Agent 服务配置和读取，MUST NOT 进入 Go 进程配置、世界存档或游戏事件。双方 credential MUST NOT 写入配置文件、日志、错误、性能报告、checkpoint 或世界存档。

#### Scenario: 非 loopback Agent endpoint 被拒绝

- **GIVEN** endpoint 分别为带 userinfo、带 query、`http://example.com`、`http://localhost:8000`、`https://127.0.0.1` 与 `http://127.0.0.1:8000`
- **WHEN** Go 验证配置
- **THEN** 前五者 MUST 被拒绝，仅最后一个 MAY 被接受

#### Scenario: provider key 不进入 Go 进程

- **GIVEN** Python 配置了 OpenAI-compatible provider key，Go 配置只含 Agent credential env 名
- **WHEN** Planner 与 Dialogue 正常运行及失败
- **THEN** Go 配置、请求日志、事件与 `companions.ai` MUST 不含 provider key 或其环境变量名

#### Scenario: Agent credential 不出现在日志与错误中

- **GIVEN** Agent HTTP 因 5xx 或认证失败
- **WHEN** 双方记录错误并向玩家发布失败事件
- **THEN** 日志与事件 MUST 不含 credential 或响应正文原文

## ADDED Requirements

### Requirement: 旧 direct-model 配置硬切换并给出迁移提示

Go config v1 loader SHALL 识别已退役的 `ai.endpoint`、`ai.model` 与 `ai.apiKeyEnv`，但 MUST NOT赋予生产语义。伙伴非空时出现任一旧字段 MUST 令启动失败，并列出对应 `ai.agentService` 迁移路径以及模型字段应迁往 Python 配置；伙伴为空时旧字段 MUST 被告警后忽略。系统 MUST 不提供 legacy/direct/service backend 开关、隐式转换或运行期 fallback。

#### Scenario: 旧启用配置被明确拒绝

- **GIVEN** 配置包含非空伙伴与原有 `ai.endpoint/model/apiKeyEnv`
- **WHEN** 服务端启动
- **THEN** 启动 MUST 失败并给出迁移提示，MUST NOT向旧 endpoint 发请求、静默关闭伙伴或自动复制 provider key

#### Scenario: AI 关闭时旧字段只告警

- **GIVEN** 配置没有伙伴但残留旧 direct-model 字段，且没有已有 `companions.ai`
- **WHEN** 服务端读取配置
- **THEN** 服务端 MUST 告警并忽略旧字段，保持 AI 关闭且不要求 Agent 配置

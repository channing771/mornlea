# companion-identity-configuration Specification

## Purpose

为每个世界提供数量有界、名称无歧义且独立于玩家身份的静态伙伴定义，使服务端、存档、协议和客户端能够稳定引用同一个伙伴。
## Requirements
### Requirement: AI 伙伴配置可选且数量有界

配置 schema SHALL 保持 v1，并 MAY 包含可选的 `ai` 组与 `ai.companions`。缺少 `ai`、`ai` 为 `null`、缺少 `companions` 或伙伴列表为空时，AI MUST 关闭，并 MUST 只通过不读取或解码正文的 metadata-only existence probe 判断是否已有 `companions.ai`；不存在文件时 MUST 不要求 Agent endpoint、key 或 timeout，也不得 Load/Save/create companion 存档。非空列表 MUST 包含 `1..4` 个有效定义。非空伙伴配置 MUST 包含 `ai.agentService.endpoint` 与 `ai.agentService.apiKeyEnv`，且环境变量值非空；缺少任一项时内置与专用服务端 MUST 启动失败。`taskTimeoutMinutes` MUST 是 `1..60` 的整数，缺省为 10。`ai.companions[]` SHALL 继续识别可选 `persona`；尚未交付的其他字段 MUST 按既有未知字段纪律告警后忽略。空配置但已有 `companions.ai` 的 retirement 行为由 `companion-persistence` 定义，完成后 MUST 保持 AI 关闭。

#### Scenario: 旧配置保持 AI 关闭

- **GIVEN** 一份有效 config v1 没有伙伴，且世界目录没有 `companions.ai`
- **WHEN** 内置或专用服务端读取配置
- **THEN** 服务端 MUST 只执行 metadata-only existence probe 并保持 AI 关闭，不创建伙伴，Load/Save 次数均为零且不创建 `companions.ai`，不启动 MCP或联系 Python，也不得要求 Agent endpoint、key、timeout 或 persona

#### Scenario: 超过四个定义被拒绝

- **GIVEN** 一份 config v1 包含五个分别有效的伙伴定义
- **WHEN** 服务端验证配置
- **THEN** 启动 MUST 失败，且不得只激活前四个伙伴

#### Scenario: 缺模型配置的伙伴被拒绝启动

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

### Requirement: CompanionID 是独立规范身份

每个伙伴 SHALL 使用独立的 16-byte `CompanionID`。文本形式 MUST 是 canonical UUIDv4；零值、非 v4、非 canonical 或重复 ID MUST 被拒绝。`CompanionID` MUST NOT 被当作 `PlayerID`、`SessionID` 或网络会话身份。

#### Scenario: canonical UUIDv4 往返稳定

- **GIVEN** 一个 canonical UUIDv4 伙伴 ID
- **WHEN** 系统解析、序列化并再次解析该 ID
- **THEN** 两次得到的 16-byte `CompanionID` MUST 相同，输出文本 MUST 与 canonical 输入相同

#### Scenario: 重复或非法身份被拒绝

- **GIVEN** 配置包含重复 ID、零 ID、非 UUIDv4 或非 canonical UUID 文本之一
- **WHEN** 系统验证伙伴定义
- **THEN** 整组定义 MUST 被拒绝，且不得创建部分伙伴

### Requirement: 伙伴名称规范且大小写敏感唯一

伙伴名称 MUST 是 canonical、有效 UTF-8，包含 `1..32` 个 Unicode 字符且不超过 128 UTF-8 bytes，并 MUST NOT 含 Unicode control 或 Unicode whitespace。唯一性 SHALL 区分大小写，因此 `A` 与 `a` MAY 同时存在；完全相同的名称 MUST 被拒绝。

#### Scenario: 大小写不同的名称可共存

- **GIVEN** 两个 ID 不同且名称分别为 `A` 与 `a` 的伙伴定义
- **WHEN** 系统验证整组定义
- **THEN** 两个定义 MUST 同时有效，后续寻址 MUST 按精确大小写区分它们

#### Scenario: 非 canonical 或含空白名称被拒绝

- **GIVEN** 名称包含普通空格、其他 Unicode whitespace、control 字符或非 canonical Unicode 表示之一
- **WHEN** 系统验证伙伴定义
- **THEN** 整组定义 MUST 被拒绝，且不得把名称静默改写后接受

#### Scenario: 字符和字节上限同时生效

- **GIVEN** 名称超过 32 个 Unicode 字符或超过 128 UTF-8 bytes
- **WHEN** 系统验证伙伴定义
- **THEN** 该名称 MUST 被拒绝，即使另一个上限尚未超过

### Requirement: 模型 endpoint 与密钥边界受严格约束

Go 配置中的 `ai.agentService.endpoint` MUST 是无 userinfo、query 与 fragment、host 为 loopback IP 字面量的 `http` URL；第一阶段 MUST 拒绝 hostname、非 loopback、`https` 远程 endpoint 与重定向。`ai.agentService.apiKeyEnv` MUST 命名一个非空环境变量，Go MUST 仅将其作为 Agent HTTP Bearer credential，MUST NOT 把它用作模型 key。模型 base URL、model 与 provider key MUST 只由 Python Agent 服务配置和读取，MUST NOT 进入 Go 进程配置、世界存档或游戏事件。双方 credential MUST NOT 写入配置文件、日志、错误、性能报告、checkpoint 或世界存档。

#### Scenario: 非法 endpoint 被拒绝

- **GIVEN** endpoint 分别为带 userinfo、带 query、`http://example.com`、`http://localhost:8000`、`https://127.0.0.1` 与 `http://127.0.0.1:8000`
- **WHEN** Go 验证配置
- **THEN** 前五者 MUST 被拒绝，仅最后一个 MAY 被接受

#### Scenario: provider key 不进入 Go 进程

- **GIVEN** Python 配置了 OpenAI-compatible provider key，Go 配置只含 Agent credential env 名
- **WHEN** Planner 与 Dialogue 正常运行及失败
- **THEN** Go 配置、请求日志、事件与 `companions.ai` MUST 不含 provider key 或其环境变量名

#### Scenario: 密钥不出现在日志与错误中

- **GIVEN** Agent HTTP 因 5xx 或认证失败
- **WHEN** 双方记录错误并向玩家发布失败事件
- **THEN** 日志与事件 MUST 不含 credential 或响应正文原文

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


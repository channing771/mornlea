## Purpose

定义 Go 权威服务端与独立 Python 伙伴 Agent 服务之间可版本化、可取消且严格有界的 HTTP 边界，使 Agent 故障不会阻塞世界或取得世界写权限。

## ADDED Requirements

### Requirement: Agent HTTP v1 仅接受同机认证连接

伙伴 Agent HTTP 接口 SHALL 使用 application contract v1，并 MUST 只监听和连接 loopback IP 字面量。服务端地址 MUST 不含 userinfo、query 或 fragment，Go 与 Agent 两端 MUST 拒绝非 loopback 地址、hostname 重解析、重定向和跨主机回调。除存活探针外的 v1 请求 MUST 使用从环境变量读取的 Bearer credential；credential MUST NOT 写入配置、日志、错误、checkpoint、MCP 工具结果、性能报告或世界存档。

#### Scenario: 非 loopback 服务地址被拒绝

- **GIVEN** 启用了伙伴的配置把 Agent endpoint 设为公网 IP、域名或带 query 的 URL
- **WHEN** Go 服务验证配置
- **THEN** 启动 MUST 失败并定位 endpoint，MUST NOT 尝试连接或回退到 direct-model 路径

#### Scenario: 密钥不进入可观察产物

- **GIVEN** Go 与 Agent 服务使用非空 Bearer credential 且一次请求因 5xx 失败
- **WHEN** 检查双方日志、错误响应、checkpoint 与 `companions.ai`
- **THEN** 任一产物 MUST NOT 包含 credential 值或上游模型响应正文

### Requirement: HTTP envelope 严格、有界且可关联

除健康探针外，每个 v1 请求与响应 SHALL 使用严格单一 JSON object，并 MUST 拒绝未知字段、尾随数据、错误 Content-Type、非法 UTF-8 与非法或不匹配的 contract version。所有操作请求 MUST 携带 `request_id`、Host `client_instance_id` 与 `namespace_id`；acquire 不携带 lease，heartbeat/release 携带当前 `lease_id`，run 携带 lease、`companion_id`、`generation` 与 snapshot/memory 身份，memory 操作携带 lease、companion、epoch/revision/operation，cancel 携带 lease 与 run identity。成功响应 MUST 回显全部适用关联字段；错误响应 MUST 只包含 `contract_version`、可空 `request_id` 与稳定 `error.code`，调用方 MUST 用本地在途记录关联错误而不得信任错误正文中的领域身份。认证失败或严格解析 request ID 前的失败 MUST 返回 null request ID；仅在完整验证 canonical request ID 后 MAY 回显它。请求正文 MUST 不超过 256 KiB，响应正文 MUST 不超过 64 KiB，HTTP header MUST 不超过 16 KiB，超限 MUST 在进一步解析和分配前被拒绝。

#### Scenario: 关联身份不匹配被拒绝

- **GIVEN** Agent 返回的 `request_id`、namespace、companion 或 generation 与请求不一致
- **WHEN** Go worker 校验响应
- **THEN** 结果 MUST 作为 `agent_unavailable` 失败并被丢弃，MUST NOT 进入任务、Dialogue、memory 或世界状态

#### Scenario: 未知字段与超限内容被拒绝

- **GIVEN** 三个请求分别包含未知字段、JSON 后尾随数据与超过 256 KiB 的正文
- **WHEN** Agent HTTP v1 接收请求
- **THEN** 三者 MUST 在执行图、模型、MCP 或 memory 操作前被拒绝，并返回不含原始正文的稳定错误

### Requirement: HTTP v1 暴露固定操作全集

Agent 服务 SHALL 只公开 `/livez`、`/readyz`、namespace acquire/heartbeat/release、`/v1/plan`、`/v1/dialogue`、memory reconcile/commit/delete 与 run cancel。namespace acquire MUST 建立 namespace 与唯一 Host instance 的 15 秒租约并返回不可复用的 fencing `lease_id`；持有方 MUST 每 5 秒 heartbeat，已被未过期租约占用的 namespace MUST 以 `namespace_conflict` 拒绝第二 Host。每个 plan、dialogue、memory mutation/reconcile 与 cancel MUST 验证 namespace 当前未过期 owner、client instance 与 lease ID。租约过期后 reacquire MUST 产生新 lease ID、取消旧 owner 的 run，并以 `not_found` 拒绝旧 lease 的 heartbeat、run、memory、cancel、迟到结果或 commit。`/livez` MUST 只表示进程可响应，`/readyz` MUST 仅在配置、模型适配、SQLite 与服务执行面可接受请求时成功。未列出的路径与方法 MUST 被拒绝。

#### Scenario: 复制世界发生 namespace 冲突

- **GIVEN** 两个运行中的 Host 使用同一 `AgentNamespaceID`，第一个租约尚未过期
- **WHEN** 第二个 Host acquire 同一 namespace
- **THEN** Agent 服务 MUST 返回 `namespace_conflict`，第二个世界 MUST 继续 tick 但其 Agent 请求 MUST 失败，双方 MUST NOT 自动改写 namespace

#### Scenario: 进程存活但执行面未就绪

- **GIVEN** Agent 进程可接收 HTTP，但 SQLite 初始化或模型配置校验失败
- **WHEN** 调用健康探针
- **THEN** `/livez` MUST 成功而 `/readyz` MUST 失败，Planner 与 Dialogue MUST NOT 被接受

#### Scenario: 旧 lease 的迟到 commit 被 fencing

- **GIVEN** Host A 的 lease 已过期且 Host B 已 reacquire 同一 namespace，Host A 的旧 memory commit 随后到达
- **WHEN** Agent 服务检查 owner 与 lease ID
- **THEN** commit MUST 被拒绝且旧 run MUST 已取消，MUST NOT 修改 Host B 的 memory 或返回幂等成功

### Requirement: Agent 运行并发、超时与取消有硬边界

Agent 服务 SHALL 全局最多执行 4 个 Planner/Dialogue run，且同一 namespace 与 companion 的全部 run 合计 MUST 最多一个在途；没有容量时 MUST 立即返回 `overloaded`，MUST NOT 建立等待队列。每个请求 MUST 服从调用方 deadline 与显式 cancel，取消 MUST 传播到图、MCP 和模型调用。服务、模型与普通 run MUST NOT 自动重试；memory commit/delete/reconcile MAY 由 Go 以相同 operation/lease 显式重放以确认不明结果，服务 MUST 按幂等/CAS 契约处理而不得重新调用模型。迟到结果 MUST 仍携带原关联身份并由 Go 在 tick 边界丢弃。

#### Scenario: 第五个 run 不排队

- **GIVEN** 四个 Agent run 均在执行且第五个合法请求到达
- **WHEN** Agent 服务检查容量
- **THEN** 第五个请求 MUST 立即返回 `overloaded`，MUST NOT 排队、重试或启动模型/MCP 调用

#### Scenario: Go 取消传播到执行面

- **GIVEN** 一个 Planner run 正等待模型且其任务已终止或 Host 开始关服
- **WHEN** Go 发送 run cancel 或取消 HTTP context
- **THEN** Agent MUST 取消图及其在途 MCP/模型调用，任何随后产生的结果 MUST NOT 被应用

### Requirement: 稳定错误不泄漏不可信正文

Agent HTTP v1 MUST 只返回 `invalid_request`、`unauthorized`、`unsupported_version`、`namespace_conflict`、`overloaded`、`deadline_exceeded`、`agent_unavailable`、`invalid_model_output`、`memory_conflict`、`not_found` 或 `internal_error` 之一及 request ID。错误响应和日志 MUST NOT 回显 credential、persona、摘要、世界快照、玩家指令、模型原文或 MCP token。Go MUST 将 `invalid_model_output` 映射为非法计划语义，将传输、认证、版本、过载、deadline、内部与 MCP 可用性错误映射为 `PlannerUnavailable`；Dialogue 的任一此类失败 MUST 只跳过台词。

#### Scenario: 模型非法输出与传输失败可区分

- **GIVEN** 一次 Planner 分别因最终 JSON 非法和 Agent 服务无法连接失败
- **WHEN** Go 应用稳定错误
- **THEN** 前者 MUST 以非法计划结束，后者 MUST 以 `PlannerUnavailable` 结束，二者都 MUST 不泄漏原始响应或重试

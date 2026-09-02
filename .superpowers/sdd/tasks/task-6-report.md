# Task 6 report

## 实现摘要

- 将 `ai` 配置硬切换为 `ai.agentService.endpoint`/`apiKeyEnv`，仅允许 loopback IP literal 的 `http` URL，并在启用伙伴时要求非空 Agent credential 环境变量。
- 旧 direct-model 字段在伙伴启用时给出迁移错误；AI 关闭时告警并忽略。
- 新增有界 Agent HTTP v1 client、固定 route 类型、Bearer/identity encoding、无 proxy/redirect、响应限制、稳定错误与基础关联校验。

## 文件

- `internal/config/config.go`
- `internal/config/agent_service_test.go`
- `internal/companion/agent_config.go`
- `internal/companion/agent_client.go`
- `internal/companion/agent_client_test.go`
- 更新的配置迁移、persona 与未知字段测试；删除已退役 direct-model 配置测试。

## TDD RED

`go test ./internal/config -run 'AIConfigAgent|AIConfigLegacy' -race -count=1` 失败，原因是 `AI.AgentService` 尚不存在。

`go test ./internal/companion -run 'Agent(Contract|Client)' -race -count=1` 失败，原因是 `NewAgentClient`、Agent settings、route request/response 类型与 fixture codec 尚不存在。

这两项失败均为新增 API 尚未实现导致的预期编译失败。

## GREEN

`go test ./internal/config ./internal/companion -run 'Agent|AIConfig|Contract' -race -count=1`：PASS。

`go test ./internal/config -race -count=1`：PASS。

`go test ./internal/companion -race -count=1`：PASS。

`go vet ./internal/config ./internal/companion`：PASS。

`go test ./internal/archcheck -count=1`：PASS。

`gofmt` clean 与 `git diff --check`：PASS。

## 自审

检查了配置环境变量与错误文本不包含 credential 值；HTTP client 禁用环境 proxy、压缩和 redirect，并对请求/响应 body、header 施加边界。当前应用装配尚未接入新 client，保留后续 Planner/Dialogue cutover 的边界。

## Concerns

`config.AI.ModelSettings` 以 `json:"-"` 仅保留源码兼容，现有 cmd/server 的旧 direct client 装配尚待后续 cutover 移除。client 的部分复杂嵌套 payload 使用 `json.RawMessage`，需要后续任务接线时进一步收紧为领域类型。

## Follow-up: cancellation fixture lifecycle

独立核对发现早期 focused client tests 遗留进程。以 `go test ./internal/companion -run TestAgentClientRejectsCorrelationMismatchAndCancellation -count=1 -timeout=3s -v` 重现：测试超时，堆栈显示 `httptest.Server.Close` 等待 handler，而 handler 无界等待 `r.Context().Done()`。客户端已向调用方返回 `context.Canceled`，但该 server-side context 在 keep-alive connection 上不保证在测试清理前结束；这是 fixture lifecycle leak，不是 Agent client 生命周期泄漏。

RED 后恢复 fixture 的明确 `release` channel：handler 可由请求 context 或测试完成后的 release 退出，避免 `Server.Close` 无界等待。遗留 PID 已 SIGQUIT 取证后 TERM 清理；随后 focused 命令连续运行两遍均自行退出。

## Round 1 review repair

### RED

`go test ./internal/companion -run 'AgentClientRound1' -count=1` 最初失败：duplicate `status` 被 `encoding/json` 覆盖后接受；`contract_version=v2` 的 Acquire 已到达 `httptest` handler；`/readyz` 503 `not_ready` 被当作 unavailable。

`go test ./internal/config -run 'AIConfigAgentServiceRejectsCaseFold' -count=1` 最初失败：`endpoint` 与 `ENDPOINT` 同时出现时 `map` 遍历顺序决定生效值。

### GREEN

- `strictDecodeJSON` 先拒绝非法 UTF-8、孤立 surrogate 与任意 object depth 的 duplicate key，再执行 strict typed decode。
- 所有已暴露的 route request 在 marshal 前检查 v1、canonical UUID 与 route identity/range；不合法 request 返回 `ErrAgentUnavailable` 且不触网。
- `/readyz` 的 manifest 503 `not_ready` 作为 typed 成功值处理；每次 request 的 lifetime merge 使用可停止 `context.AfterFunc`，避免正常调用残留 goroutine。
- Agent transport 由 client 专有创建，外部 `http.Client` 不能带入 proxy/redirect/compression/header 策略；发送前也检查 header 字节边界。
- 配置 parser 检测大小写 collision，`agentService` 逐字段解析并对 nested unknown 精确告警。

### 验证

`go test ./internal/companion -run 'Agent(Client|Contract)' -count=1 -timeout=30s`：PASS。

`go test ./internal/config -run 'Agent|AIConfig' -count=1`：PASS。

### Concerns

复核仍需要继续完成全部 HTTP schema 的 nested DTO 替换与每条 manifest route 的 status/error/correlation matrix；本次变更只处理了本轮新增 RED 覆盖的 transport/config 问题。

## Round 1 continuation

### RED

`go test ./internal/companion -run AgentContractGolden -count=1` 在新的 checked-in golden DTO harness 下失败：IPv6 loopback MCP endpoint、terminal fact variants、strict nullable dialogue line 和 error envelope 尚未由实际 DTO validator 表达。这些失败证明此前字符串/map fixture helper 不能覆盖 production codec。

### GREEN

- 生产 Agent DTO 不再持有 `json.RawMessage`：Plan、fact node、environment、memory state/proposal 和 reconcile nullable members 都改为 closed structural DTO；Dialogue wire 字段为 `fact_node`，reconcile 总是输出 `mirror` 与 `tombstone_operation_id`。
- strict decoder 检查 duplicate key、非法 UTF-8 与孤立 surrogate；DTO validators 对 text/identity/variant、plan/memory 状态、常量和 correlation 进行拒绝并返回零值。
- 每条当前 client route 的 success correlation 加入 request/client/namespace/lease/run/companion/generation/snapshot/epoch/operation 核对；稳定 errors 加入 manifest status/path allowlist。
- `TestAgentContractGoldenDrivesActualDTOCodecs` 读取 checked-in valid/invalid golden，并直接执行 production strict decoder 和 typed DTO validators。

### GREEN/验证

`go test ./internal/config ./internal/companion -run 'Agent|AIConfig|Contract' -race -count=1` 连续两遍：PASS。

`go test ./internal/config -race -count=1`、`go test ./internal/companion -race -count=1`、`go vet ./internal/config ./internal/companion`、`go test ./internal/archcheck -count=1`、`gofmt`、`git diff --check`：PASS。

### 剩余风险

需要继续补完整 manifest 11-route `httptest` dispatch matrix 和由 manifest 解析出的 method/status/identity assertions；当前 golden test 已覆盖每个 checked-in schema fixture 的实际 DTO codec，但尚不是所有 route 的端到端 transport case。

## Round 2 manifest matrix repair

### RED

`go test ./internal/companion -run 'AgentManifest|AgentClientStrict' -race -count=1` 首轮暴露未声明 `201` 被接受并返回已填充 DTO，Acquire contract version、reconcile tombstone、memory delete epoch 等关联不匹配也会留下部分值；strict nested required/variant、非法 request 与 error envelope 仍有漏网。

`go test ./internal/companion -run TestAgentStrictJSONAllowsEscapedLiteralSurrogateText -count=1` 失败为 `invalid strict JSON`，证明原 surrogate scanner 把转义后的字面 `\\ud800` 误判为孤立 surrogate。

`go test ./internal/companion -run TestAgentContractGoldenDrivesActualDTOCodecs -count=1` 在 `memory_state_zero` panic，证明旧 golden harness 对未识别 invalid schema 直接返回 false 而伪绿。

`go test ./internal/companion -run 'TestAgentClientStrictNestedRequiredAndVariantFields/nonterminal_proposal_explicit_null' -count=1` 返回已填充 Dialogue DTO，证明显式 `memory_proposal:null` 被错误等同为字段缺席。

`go test ./internal/companion -run 'TestAgentClientRejectsInvalidRequestsBeforeDispatch/MCP_(encoded_path|invalid_port)' -count=1` 的两个 case 都到达 `httptest` handler，证明 encoded `/mcp` 与越界端口未在触网前拒绝。

`go test ./internal/config -run TestAIConfigDisabledFormsIgnoreAgentAndLegacySettings -count=1` 因 disabled 配置的数值 `agentService` 解析失败，证明空伙伴仍提前要求 service/timeout 形状。

### GREEN 与 manifest 覆盖

- `manifest.json` 直接驱动 11 条公开 route 的实际 `AgentClient` 方法、method/path、Bearer/匿名、精确 Content-Type、identity profile、success status 与 error allowlist；全部 request 均由 production DTO marshal 后进入真实 `httptest` transport，全部 response/error 均经过 production strict decoder 与 correlation。
- success 共 14 个实际 dispatch：12 个 manifest response，加上 Dialogue terminal/nonterminal 与 reconcile active/inactive 展开；包含 `/readyz` 503 `not_ready`。59 个 route/error 声明组合逐一验证，并为每条 route 验证错误 status/code 错配、未声明 code 与未声明 success status 均返回 typed 零值。
- 由 manifest identity profile 驱动 79 个 response correlation mutation；覆盖 contract/request/client/namespace/lease/run/companion/generation/snapshot/epoch/operation，以及 reconcile tombstone 和 delete new-epoch 映射。golden 本身若不满足同名关联会直接失败，不再静默跳过。
- production contract boundary 无 `json.RawMessage`。所有顶层及 nested DTO 使用 closed typed object/union decoder，required/unknown/null 与 variant presence 精确区分；duplicate、非法 UTF-8、孤立高低 surrogate、错误 surrogate pair、NaN、trailing、null、type、missing、unknown 均失败且丢弃部分值，合法 surrogate pair 与转义字面量正常接受。
- request 在触网前校验 v1、canonical UUID、range/deadline、text byte/控制字符、非空数组、variant、loopback MCP URL 的 256-byte/exact raw path/有效端口，以及 256 KiB body；response 校验精确 status/常量/enum/variant、64 KiB body 与 16 KiB header。
- request/response body 的 exact 与 `+1` byte、chunked overflow、声明 Content-Length overflow、request/response header 的 exact 与 `+1` byte、Content-Type 变体都有独立边界测试。
- client 使用独立构造的 transport，不继承外部 client 或可变全局默认 transport；proxy nil、identity encoding、compression/keep-alive/redirect 禁用，断连 GET 仅一次请求。caller cancel/deadline 与并发幂等 `Close` 都取消 in-flight 并返回零值；正常调用无 goroutine 累积，`%v`/`%+v`/`%#v` 不泄漏 credential。
- disabled/missing/null/empty companions 不解析 Agent service、timeout 或 secret；active 配置覆盖 legacy 单字段迁移、IPv4/IPv6 loopback、case-fold collision、nested unknown 精确路径，以及 timeout 0/1/60/61。

### 最终验证

`go test ./internal/config ./internal/companion -run 'Agent|AIConfig|Contract' -race -count=1 -timeout=120s` 连续两遍：PASS。

`go test ./internal/config -race -count=1`：PASS。

`go test ./internal/companion -race -count=1`：PASS。

`go vet ./internal/config ./internal/companion`：PASS。

`go test ./internal/archcheck -count=1`：PASS。

`gofmt` 与 `git diff --check`：PASS。

### 剩余风险

consolidated findings 1–7 在本轮范围内均已闭环。Planner/Dialogue wiring、persistence 与 Task 7 之后的实现仍按后续任务推进，本轮未触碰。

## Round 3 strict/lifecycle repair

### RED

新增 `agent_client_round3_test.go` 后运行：

`go test ./internal/companion ./internal/config -run 'ExplicitNull|NullableFields|OutboundHeader|DuplicateJSONContentType|AfterResponseHeaders|LinearizesAdmission|FormattingAndLogging|LargestValidTypedRequest|AgentServiceEndpointMatrix' -count=1 -timeout=30s`

真实失败包括：Plan `x/y/z:null`、Dialogue `base_revision:null`、memory `summary:null` 与 cancel `cancelled:null` 被解成零值并作为部分成功返回；raw TCP listener 观察到 outbound header 实际为 16,371 bytes 而测试要求的 Python 同 scope 预算是 16,384 bytes；重复两条 `Content-Type: application/json` 被接受；解引用后的 `AgentClient` `%v` 泄漏 credential；`http://127.0.0.1:0` 被配置接受。

共享 worktree 的整包命令同时被并行任务的预期 RED 阻断：Task 7A 正在修订的 MCP fixture 令三个 `ContractFixture` case 暂时失败，Task 8 正在新增的 storage v5 test 因尚未实现 `companionFlagActive` 令 archcheck 暂时编译失败。本轮没有修改、暂存或借豁免绕过这些并行文件。

### GREEN

- closed DTO 解码在 typed unmarshal 前拒绝所有 non-nullable required/optional field 的显式 `null`；nullable allowlist 只含 error `request_id`、memory state `operation_id` 及 reconcile `mirror`/`memory`/`tombstone_operation_id`，variant validator 继续裁决合法组合。public client matrix 覆盖 identity/nested/string/bool/number、`x/y/z`、`base_revision`、`revision`、`summary`、`active`、`cancelled`，失败全部返回 typed 零值。
- outbound header 固定 `Host`、`User-Agent`、`Content-Length`、`Connection: close` 与显式 headers，并按 `len(name)+2+len(value)+2` 对每条真实 line 计数。独立 raw listener 证明 16,384 bytes 可触网成功、16,385 bytes 在触网前拒绝；response `Content-Type` 使用 `Header.Values`，只接受唯一一条精确 `application/json`。inbound transport/header 边界仍覆盖 exact 与 `+1`。
- client lifecycle 改为 mutex 保护的 closed admission 与 active cancel registry；`Close` 同步标记 closed、取消全部 active request、等待并发 `Close` 的首个关闭过程，之后的新公开调用不触网。caller cancel/deadline 与 `Close` 在 response headers 已 flush、body 阻塞时都返回对应 context error 与 typed 零值；正常调用不创建 lifetime goroutine。
- credential 使用 redacted wrapper，并为 `AgentClient` 的 pointer/value method set 提供安全 `fmt.Formatter` 与 `slog.LogValuer` 表示；pointer 与 dereferenced value 的 `%v`、`%+v`、`%#v`、`%q` 及 structured log 均不泄漏。
- Agent service endpoint 的显式 port 只接受 `1..65535`；IPv4/IPv6 loopback 的 1、8080、65535 通过，0 与 70000 拒绝。
- 生产 public preflight 发送最大维度合法 Dialogue request：4,096-byte 最坏 JSON expansion persona、256 exposed blocks、1,089 heights、最大整数 identity fields，真实 wire 为 94,799/262,144 bytes；更大的 public typed request 在触网前拒绝。由此证明合法 DTO 的理论最大值远低于 request cap，而 cap 不可被 public path 绕过。

### 隔离验证

为隔离并行 Task 7A/8 的预期 RED，只显式暂存六个 Task 6 文件，在 detached `148b935c` 临时 worktree 通过 pipe 应用 `git diff --cached --binary`；首次 Go 命令只因 clean worktree 尚无 `libmornlea_engine` 链接失败，按仓库规则运行 `make rust` 后从头验证：

- `make rust`：PASS。
- `go test ./internal/config ./internal/companion -run 'Agent|AIConfig|Contract' -race -count=1 -timeout=120s` 连续两遍：PASS。
- `go test ./internal/config -race -count=1 -timeout=120s`：PASS。
- `go test ./internal/companion -race -count=1 -timeout=120s`：PASS。
- `go vet ./internal/config ./internal/companion`：PASS。
- `go test ./internal/archcheck -count=1 -timeout=120s`：PASS。
- Task 6 文件 `gofmt -l` 无输出，`git diff --check`：PASS。
- 隔离 worktree 删除前确认无相关 `go test` 残留进程，随后只移除该明确临时 worktree。

### 结果

Round 3 指定的 strict null、真实 header scope、body-read cancellation、Close admission、secret representation、endpoint port 与 256 KiB preflight 证据均已闭环；未触碰 Task 7+ 生产实现、contracts、OpenSpec 或 storage 文件。

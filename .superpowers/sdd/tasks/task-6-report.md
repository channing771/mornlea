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

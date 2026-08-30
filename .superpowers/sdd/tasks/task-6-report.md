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

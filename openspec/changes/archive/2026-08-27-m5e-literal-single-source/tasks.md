# 任务

> 每个任务由全新 implementer 子代理执行（brief 为唯一需求来源）；TDD 红绿循环对本变更
> 表现为「既有行为级锁测试全程保持绿」，无新增行为故无新增测试（design.md 验证口径）。

## 1. L1：capture 侧 `[32]network.ChatEvent` 裸字面同源化（递延 4）

- [x] 1.1 `cmd/mornlea/capture_scene.go` 两处（场景构造与重置路径）`[32]network.ChatEvent{}`
      → `[client.ChatEventCapacity]network.ChatEvent{}`；确认 `client` 包已导入。
- [x] 1.2 `cmd/mornlea/capture_ai_companion_test.go` 断言一处 `[32]network.ChatEvent{}`
      → `[client.ChatEventCapacity]network.ChatEvent{}`；确认测试文件 `client` 导入。
- [x] 1.3 全仓 grep 确认 `[32]network\.ChatEvent` 零残留。
- [x] 1.4 验证：`go test ./cmd/mornlea -race -count=1`。

## 2. L2：ChatCommand 编解码字面与错误文案同源化（递延 5）

- [x] 2.1 `internal/network/message_companion.go` 在 `chatCommandMaxWireBytes` 推导注释块旁
      新增 `chatCommandTextMaxBytes = companion.MaxPlanCommandBytes`，配中文注释说明三点收敛
      （校验 `validateCommandText` / 编码 / 解码同源）。
- [x] 2.2 `internal/network/codec_client.go` 编码端 `e.string(message.Text, 1024)`
      → `e.string(message.Text, chatCommandTextMaxBytes)`。
- [x] 2.3 同文件解码端 `d.string(1024, 1024)` → `d.string(chatCommandTextMaxBytes, chatCommandTextMaxBytes)`，
      注释说明 rune 上限与字节上限同值系现状保持。
- [x] 2.4 payload 上限守卫错误文案改 `fmt.Errorf("network: chat command payload exceeds %d bytes", chatCommandMaxWireBytes)`；
      补 `fmt` 导入；确认格式化输出与原串逐字节相同。
- [x] 2.5 验证：`go test ./internal/network -race -count=1`（既有锁测试
      `TestChatCommandAccepts1024BytesAndRejects1025`、`TestCompanionMessagesHaveFixedMaximumWireLengths`
      必须原样全绿）。

## 3. 收尾（实现者）

- [x] 3.1 `gofmt -l .` 无输出；`go vet ./...` 干净。
- [x] 3.2 `go test ./... -race`（或 `make test-race-short` 迭代 + 提交前全量）通过。
- [x] 3.3 `openspec validate --all --strict --no-interactive` 通过；本表全部勾选核对。
- [x] 3.4 ledger 记录评审结论、终审证据与最终裁决；未决项誊入 proposal「延期与放弃」。

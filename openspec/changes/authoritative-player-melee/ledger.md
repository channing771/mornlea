# Ledger

| 日期 | 任务 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-22 | 1 | v25 仅保留 `Mining` wire 位并扩展其 primary-action 语义；v24 起握手拒绝。 | `make rust`、网络/入口/架构测试、OpenSpec 严格校验。 |
| 2026-08-23 | 5 | 全量门禁已串行执行；`make rust`、`make rust-check`、网络/模拟 race、架构、vet、gofmt、基线一致、OpenSpec 与 diff 检查通过。服务端 race 及全仓 race 复现既有四项集成失败：两条共享箱子传输关闭、八人 memory tick 偏差、八人磁盘重启位置偏差；本任务不改产品行为，Task 5 不勾选。 | `go test ./internal/network ./internal/sim ./internal/server -race -count=1`、单独 `TestChestSharedByTwoPlayersOverMemory`、`go test ./... -race`。 |

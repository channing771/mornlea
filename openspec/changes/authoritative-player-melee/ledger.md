# Ledger

| 日期 | 任务 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-22 | 1 | v25 仅保留 `Mining` wire 位并扩展其 primary-action 语义；v24 起握手拒绝。 | `make rust`、网络/入口/架构测试、OpenSpec 严格校验。 |
| 2026-08-23 | 5 | 全量门禁首次运行揭露四个旧集成夹具缺少「采掘射线没有 active 同维玩家候选」前提：两条共享箱子传输关闭、八人 memory tick 偏差、八人磁盘重启位置偏差。 | `go test ./internal/network ./internal/sim ./internal/server -race -count=1`、单独 `TestChestSharedByTwoPlayersOverMemory`、`go test ./... -race`。 |
| 2026-08-23 | 5 修复 1 | 仅隔离旧夹具的近战候选：箱子另一查看者从 +Z 侧回看；八人脚本保持独立 X 车道并沿车道移动；重启玩家用独立 X 车道及对应起点断言。v25 近战、协议和断言强度不变。 | 四个失败用例逐个 race 通过；近战 focused、server race、全仓 race、archcheck、OpenSpec、diff、gofmt 与基线一致均通过。 |

# Ledger

| 日期 | 任务 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-22 | 1 | v25 仅保留 `Mining` wire 位并扩展其 primary-action 语义；v24 起握手拒绝。 | `make rust`、网络/入口/架构测试、OpenSpec 严格校验。 |
| 2026-08-23 | 5 | 全量门禁首次运行揭露四个旧集成夹具缺少「采掘射线没有 active 同维玩家候选」前提：两条共享箱子传输关闭、八人 memory tick 偏差、八人磁盘重启位置偏差。 | `go test ./internal/network ./internal/sim ./internal/server -race -count=1`、单独 `TestChestSharedByTwoPlayersOverMemory`、`go test ./... -race`。 |
| 2026-08-23 | 5 修复 1 | 仅隔离旧夹具的近战候选：箱子另一查看者从 +Z 侧回看；八人脚本保持独立 X 车道并沿车道移动；重启玩家用独立 X 车道及对应起点断言。v25 近战、协议和断言强度不变。 | 四个失败用例逐个 race 通过；近战 focused、server race、全仓 race、archcheck、OpenSpec、diff、gofmt 与基线一致均通过。 |
| 2026-08-23 | 5 修复 3 | PR #66 [CI `32635107688`](https://github.com/channing771/mornlea/actions/runs/32635107688) 在 `TestMultiplayerTCPClientsSeeMoveEditAndDespawn` 未收敛后溢出 `Receiver`；旧夹具让 A/B 初始碰撞盒重叠，A 移动约 0.1 格后其 `Mining` 射线在 B 盒内以距离 0 合法命中 v25 近战，因而持续抑制原定方块采掘。仅将 B 预置到独立 X 车道，并在采掘前断言包围盒与 yaw=0 射线隔离；未改产品、协议、容量、时限或断言强度。 | 新断言在旧重叠位置 RED；目标用例 race GREEN 及 `-count=5`，sim/server 近战 focused、`go test ./internal/server -race -count=1`、archcheck、OpenSpec strict、diff、gofmt 与基线一致均通过。 |
| 2026-08-23 | 整分支终审修复 1 | **Ruling:** 服从五路并行计划的长期文档边界，撤回完整近战能力段，只保留协议 v25 的机械版本同步；恢复八人夹具的双玩家同块竞争与双客户端 oracle，用独立 X 车道和斜向 yaw 排除玩家 AABB；补齐命中后下一 tick 采掘恢复；近战排序测试及唯一专属 helper 迁入现有近战 parity 文件。未改产品代码、超时、容量或协议。 | 双竞争旧 yaw RED 为两份掉落，斜向 yaw GREEN 为单份共享掉落；采掘抑制清零 mutation RED/GREEN；focused race 多次、完整 sim/server 与全仓 race、Rust、archcheck、vet、gofmt、OpenSpec、diff/cmp、`go test -list` 集合均通过。 |
| 2026-08-23 | 五路归档收尾 | 已验证远端事实：PR #66 合入 `ee3a8eba7257a0b507dae7a622de80397a815784`（`Merge pull request #66 from channing771/codex/authoritative-player-melee`）；最终 GitHub Actions [run `32640869549`](https://github.com/channing771/mornlea/actions/runs/32640869549) 的 8/8 logical job/child 均成功，整分支 SPEC/QUALITY 终审 PASS。最后一项全量门禁/终审据此完成；无残余人工产品验收，change 保持 active，等待控制会话归档。 |

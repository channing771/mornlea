## 1. 恒脏自旋有界终止（TDD）

- [x] 1.1 新增 `internal/server/player_flush_stall_test.go`：controllable store 在每次保存完成前 `Observe` 新快照模拟恒脏；先跑出 RED（当前实现自旋或静默成功），断言 Flush 返回 `errPlayerFlushStalled`、store 恰收到 1 次 save、随后一次 Flush 把最新快照落盘并返回 nil。验证：`go test ./internal/server -run TestPlayerFlushStall -race -count=1`（先失败）。
- [x] 1.2 按 design A′ 改 `internal/server/player_flush.go`：`attempted` 换为 `map[core.PlayerID]playerFlushSlots` 双类名额（继承预占 retry、retry 派发占 retry、fresh 派发占双类），`!inFlight && !dispatched` 退出路径对无失败记录的残余脏玩家计入 `errPlayerFlushStalled`（键 `revision = persisted + 1`）。新增标识符补中文 GoDoc。验证：`go test ./internal/server -run 'TestPlayerFlush|TestHostShutdown' -race -count=1` 与新测试转 GREEN。
- [x] 1.3 全包回归：`go test ./internal/server -race -count=1`；确认 10 条既有 Flush 测试零改动通过。

## 2. 收尾

- [x] 2.1 `gofmt -l .` 无输出；`go vet ./...`；`go test ./internal/archcheck -count=1`。
- [x] 2.2 `openspec validate fix-player-flush-stall --strict --no-interactive`。
- [x] 2.3 `scripts/agents/gates.sh` 全量门禁（含 `make rust` 与全仓 race）；整分支终审；结论记入 `ledger.md`。

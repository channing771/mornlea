## 1. 出生支撑顶面闭环

- [ ] 1.1 按 TDD 在 `internal/sim/spawn_support_top_test.go` 先加入真实耕地 `15/16` 顶面的出生、登录恢复与 grounded safe 点行为测试，用 `go test ./internal/sim -run 'Test(Spawn|PlayerRestore|Safe).*FarmlandSupportTop' -count=1` 观察预期 RED；再最小修改 `internal/sim/spawn.go`，让 `findSpawnInColumn` 枚举每格最多 8 个碰撞盒、按世界顶面降序构造候选，并仅在 `playerBoundsAreFree` 与 `playerSupport` 的 `completeSupport` 同时成立后进入既有 `spawnTierOf`。保持候选列顺序、区块 ready 等待和流体三档不变。完成后运行 `gofmt`、`go test ./internal/sim -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./...`、`go test ./... -race`、`test -z "$(gofmt -l .)"` 与 `openspec validate --all --strict --no-interactive`，并把 RED/GREEN 与全量输出摘要写入 `ledger.md`。


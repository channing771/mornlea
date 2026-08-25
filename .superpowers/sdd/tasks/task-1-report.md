# Task 1 报告

## RED / GREEN

- RED：`go test ./internal/sim -run 'Test(Spawn|PlayerRestore|Safe).*FarmlandSupportTop' -count=1` 失败；`TestSpawnFarmlandSupportTop` 实际出生 `Position:[0.5 1 0.5]`，期望耕地顶面 `y=0.9375`。
- GREEN：同一 focused 命令通过。

## 验证

- `go test ./internal/sim -race -count=1`：通过。
- `go test ./internal/archcheck -count=1`：通过。
- `go vet ./...`：通过。
- `test -z "$(gofmt -l .)"`：通过。
- `openspec validate --all --strict --no-interactive`：63 项通过。
- `go test ./... -race`：已启动；本 worktree 会话未返回最终输出，需提交前由集成会话确认。

## 改动与自审

- `internal/sim/spawn.go`：每格最多 8 个盒按顶面降序枚举，候选同时通过身体无碰撞和 `playerSupport` 完整支撑后才分档。
- `internal/sim/spawn_support_top_test.go`：覆盖出生、登录恢复、grounded safe 点的真实耕地 `15/16` 顶面。
- `openspec/changes/fix-spawn-support-top/tasks.md`、`ledger.md`：勾选并记录执行证据。
- 自审：未改变列顺序、ready 等待、流体三档或共享出生路径；未新增依赖、配置、生产测试接口。

# Tasks: dry-farmland-revert

- [x] 1. 新建 `internal/sim/farmland_revert.go`（`farmlandRevertRoll` + 30% 常量，独立 salt，`splitmix64` 确定性）
- [x] 2. 修改 `internal/sim/crop.go` `advanceCropCell`：复用抽样实现干+上方为空气时以概率退回 `DirtID`，原子写入 `pending`
- [x] 3. 新增 `internal/sim/farmland_revert_test.go`：干+空气可退化、有作物不退、湿不退、确定性重放
- [x] 4. 更新 `internal/sim/crop_cost_test.go` 与 `crop_perf_test.go` 的读取/pending 等式以容纳新增上方读取与顶层退化
- [x] 5. 运行 `go test ./internal/sim -race -count=1`、`go test ./internal/archcheck -count=1`、`gofmt -l .`、`go vet ./...`、`openspec validate --all --strict --no-interactive`
- [x] 6. 更新 `openspec/specs/authoritative-farming/spec.md` 主规格（经 `openspec sync` 或归档时直编）并回填 `docs/feature-backlog.md` B-06 → 已完成

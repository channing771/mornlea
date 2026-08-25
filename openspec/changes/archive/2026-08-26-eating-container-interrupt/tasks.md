# Tasks: eating-container-interrupt

## Task 1: 进食中断判据（TDD red → green）

- [x] 实现与测试完成（`d53b87d2` + R1 `b7623286`；SPEC/QUALITY 双评审 PASS，见 ledger）。

- **目标文件**：`internal/sim/eating_test.go`（先写失败测试）、`internal/sim/eating.go`（`advanceEating` 追加 `suspended bool` 参数并入中断分支、更新中文注释）、`internal/sim/player.go`（仅 `internal/sim/player.go:496` 一处调用点传 `session.viewContainer || !session.hasView`）。
- **测试内容**（遵循 design.md「测试策略」）：
  1. 进度在 `(0, 32)` 内、容器打开（`suspended=true`）且输入按住 → 进度清零、面包数与饥饿/饱和精确不变；
  2. 进度到 `EatingTicks-1` 后下一 tick `suspended=true` → 零结算；
  3. `suspended=true` 连续保持 → 进度恒为零、不扣料；
  4. 中断后 `suspended=false` 且输入仍按住 → 从第 1 tick 重新推进并到时正常结算（关箱重启语义）；
  5. 既有用例全部保持绿（松手/切格/饥饿满/结算原子性回归）。
- **验证命令**：`go test ./internal/sim -race -count=1 -run Eating` 先红后绿；随后 `go test ./internal/sim -race -count=1` 全包。
- **完成判据**：新用例在无实现时红、实现后绿；`gofmt -l internal/sim` 无输出；`player.go` 的 diff 恰好一行（裁决边界）。

## Task 2: 变异验证与全量门禁收尾

- [x] 完成见 ledger（双向变异各杀 4 用例、全量门禁全绿、冻结集核对通过）。

- **变异验证**：临时把调用点改为恒传 `false`（或删除 `eating.go` 的 `suspended` 判据）跑 `go test ./internal/sim -race -count=1 -run Eating`，确认新增用例变红；恢复后复绿（变异不提交）。
- **全量门禁**：`gofmt -l .` 无输出、`go vet ./...`、`go test ./... -race -count=1`、`openspec validate --all --strict --no-interactive`。
- **收尾核对**：`git diff` 只含 design.md 冻结集文件；本 tasks.md 全部勾选；ledger 记录门禁输出摘要（数值只记录）。

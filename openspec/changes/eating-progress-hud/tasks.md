# Tasks: eating-progress-hud

## Task 1: 进食进度呈现（TDD red → green）

- **目标文件**：`internal/client/eating_progress.go` + 新测试、`internal/render/hud/eating.go` + `eating_test.go`、`internal/render/hud/renderer.go`/`layout.go`（签名追加，最小行）、`cmd/mornlea` 三处裁决点位（`app.go` 字段、`app_frame.go` 构造与实参、`app_lifecycle.go` 复位）+ 新建 `eating_overlay_test.go`。
- **实现要点**（遵循 design.md D1–D5）：状态机按 20 ms/tick 累积、复位三源（输入位/切格/换物）、满格钳制；`appendEatingBar` 复用 `appendMiningBar` 的锚点与几何常量、无帽、收到采掘激活时不追加（互斥在布局函数内判定）；`Prepare`/布局签名追加 `EatingOverlay` 参数同既有 overlay 形态传递。
- **测试内容**：速率精确值、复位三源、钳制、零时长不激活、互斥（采掘激活时零进食 quad）、容量红线（进食激活帧 quad 数 ≤ 采掘激活帧，容量常量 267/46912 不变）、三点位定点（构造取值与 lifecycle 复位）。
- **验证命令**：`go test ./internal/client ./internal/render/hud -race -count=1`；`go test ./cmd/mornlea -race -count=1 -short -run 'Eating|HUD|Overlay'`（无匹配则跑包级 `-short`）；`gofmt -l` 无输出；`git diff` 恰为冻结集、三个 app 点位各不超所裁行数。
- **完成判据**：新用例无实现时红、实现后绿；`maxHotbarQuads`/`hotbarUploadBytes` 常量与既有断言零改动。

## Task 2: 变异验证与全量门禁收尾

- **变异验证**（不提交）：a) 注释互斥判定 → 互斥/容量用例红；b) 去掉任一复位源 → 对应复位用例红；c) 分母或周期常量改错 → 速率用例红。恢复后复绿。
- **全量门禁**：`gofmt -l .`、`go vet ./...`、`go test ./... -race -count=1`、`openspec validate --all --strict --no-interactive`、`go test ./internal/archcheck -count=1`。
- **收尾核对**：merge-base 计的分支 diff 恰为冻结集；本 tasks.md 全勾；ledger 记录门禁输出摘要。

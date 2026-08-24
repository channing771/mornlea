# Task 6 报告 — capture: main-menu 场景与既有 golden 零影响

## Step 0: 需求核对与既有 diff 审阅

唯一需求来源：`.superpowers/sdd/add-egui-main-menu/task-6-brief.md`，规格：
`openspec/changes/add-egui-main-menu/specs/visual-verification/spec.md`。

前置（Task 5）已交付 `menuOverride`/`uiSegment()` 与 `client.UIMenu`（
`internal/client/render.go` 的 `UIMenu` + `EncodeUIMenu`）。

当前未提交 diff（两个前序 implementer 已完成、未提交，会话在 `make visual-check`
期间中断）：

- `cmd/mornlea/capture.go`（+56）：`captureScene` 新增 `Menu *client.UIMenu` 字段；
  `main-menu` 场景（`Name: "main-menu"`、`WarmupFrames: 8`、`Menu` 标题 Mornlea/
  版本 dev/错误「存档无法打开」/四按钮由 `menuButtons()` 复用）插在 `far-horizon`
  之前；`captureSceneImage` 在 `scene.Apply` 之后设 `app.menuOverride = scene.Menu`
  （nil 即清除）。
- `cmd/mornlea/capture_ai_companion_test.go`（+48）：`TestCaptureSceneOrderAndAICompanionDeterminism`
  顺序列表加入 `main-menu`（位于 `far-horizon` 之前）；新增
  `TestMainMenuCaptureScenePosition`。

逐一核对符号存在性（grep 全仓确认）：

- `func menuButtons() []client.UIButton` → `cmd/mornlea/app_menu.go:64`（进入/多人/
  设置/退出，enabled 为 true/false/false/true，与测试断言一致）
- `func captureSceneByName(t, name)` → `capture_ai_companion_test.go:18`
- `application.menuOverride *client.UIMenu` → `cmd/mornlea/app.go:175`
- `type UIMenu struct { Visible; Title; Version; Error; Buttons }` → `internal/client/render.go:422`
- `func resetCapturePresentation` → `cmd/mornlea/capture_scene.go:481`
- `func cameraChunk` → `cmd/mornlea/app_render.go:126`
- `mgl32`（mathgl）已在 capture.go import

结论：diff 完整符合 brief 第 1-3 条，无需补齐。

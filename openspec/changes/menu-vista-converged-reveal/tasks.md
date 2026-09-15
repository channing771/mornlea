# Tasks

## 1. 失败测试先行：揭示门

- 目标文件：`packages/client/cmd/mornlea/app/app_menu_vista_test.go`
- 钉住：未收敛帧不提交全景几何（门谓词单测 + 帧路径分支断言）、收敛后 tick 从 0 推进、构建失败降级不回退
- 验证：`go test ./packages/client/cmd/mornlea/app -run TestMenuVista -count=1`（实现前必须失败）

## 2. 实现揭示门与泵速

- 目标文件：`packages/client/cmd/mornlea/app/app_menu_vista.go`、`app_frame.go`
- 未收敛仅天空清屏分支、tick 仅揭示帧推进、`menuVistaChunksPerFrame` 4 → 12
- 验证：`go test ./packages/client/cmd/mornlea/app -race -count=1`

## 3. 视觉与收尾门禁

- `SCENES=main-menu,settings-menu make visual-check`（零漂移）；`gofmt`；六模块 `go vet` 或 `make dev-check`；`openspec validate menu-vista-converged-reveal --strict --no-interactive`

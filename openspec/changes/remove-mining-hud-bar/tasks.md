# remove-mining-hud-bar 任务

约束：fresh implementer + TDD + 双评审；注释中文无任务编号；提交单行英文。
基线 SHA `dca9197e`。

## 1. 三端删除采掘进度条

- [ ] 1.1 失败测试先行：前端 vitest 先改（`HudRoot.test`/`hud.assert.test`
  断言「UI 状态携带 mining 分节也绝不渲染采掘轨道」「进食条不再被采掘
  抑制」）；Go 测试先改（`ui_hud_state_test` 断言 JSON 无 `mining` 分节、
  `app_ui_state_test`/`eating_overlay_test` 断言 eating 无门控、
  `app_mining_overlay_test`/capture 夹具去 `Harvestable`）。
- [ ] 1.2 实现：schema.json/client.ts/HudRoot/ProgressTrack/hud.css/
  tokens.css/geometry.ts 删 mining；Go 删 `UIHudMining`、`assembleHUDState`
  mining 分节与 `!Active` 门控、`MiningOverlay.Harvestable` 字段及全部
  赋值点；`make frontend-check` 重建 dist；前端 visual fixture 只留
  eating 并重拍 `hud-progress.png`。
- [ ] 1.3 验证：`make frontend-check`；`go test ./internal/client
  ./cmd/mornlea/app ./cmd/mornlea/capture -race -count=1`；
  `go test ./internal/render/... -race -count=1`。

## 2. 收尾门禁

- [ ] 2.1 `gofmt -l .` 无输出；`go vet ./...`；
  `make visual-check` 24 场景零差异（golden 不动）；
  `openspec validate --all --strict --no-interactive`；
  文档同步（`cmd/mornlea/AGENTS.md` 或相关局部指南若提及采掘条）。

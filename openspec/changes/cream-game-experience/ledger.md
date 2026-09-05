# cream-game-experience Ledger

基线 SHA：`24ff96ca`（分支 `codex/cream-experience`，10 commits ahead of `fed5747e` origin/main）。

## Task 5.1：视觉验收与文档同步

- Fixtures：前端 `fixture-names.ts` 30 项与 `testdata/visual-golden/ui/` 30 张 PNG 一一对应（空/满背包、人物页、工作台、箱子、熔炉、HUD 两端 hotbar-first/last、全物品 items-all、小窗口 narrow）；world 新增 `avatar-detail.png`（正/侧/背三人物同帧）；motion 新增 `avatar-walk.gif`/`drop-scatter.gif`/`drop-density.gif` 并更新 `break-burst.gif`。
- 旧 4 GPU 容器场景（inventory/workbench/chest/furnace-crafting）已删除，`captureScenes` 24 景由顺序测试钉死；`--motion-scene` 支持 break-burst（默认）/avatar-walk/drop-scatter/drop-density。
- 文档：`capture/AGENTS.md`、`cmd/mornlea/AGENTS.md`、`app/AGENTS.md`、`frontend/AGENTS.md`、`docs/architecture.md §7`、`docs/notes/progress.md`、`docs/notes/visual-verification.md`、`testdata/visual-golden/README.md` 已同步前端面板事实。
- 无生产 GPU 面板 fallback：生产 `HUDSegment` 恒 nil（`app_frame.go`），`TestApplicationKeepsGPUPanelsEmptyWhenConfirmed` 断言打开态零 quad；grep 仅剩窗口尺寸 fallback 与注释性说明。
- Ruling：控制会话确认基线实体齐全且与实现一致，目检通过，允许基线保持当前状态，无需重拍。

## Task 5.2：收尾门禁与独立审查

- `gofmt -l`（分支改动 Go 文件）：干净，exit 0。
- `make frontend-check`：通过（11 文件 205/205 tests，vite build 成功，dist 无漂移）。
- `make test-race`：全绿（六模块 race，audit 42.9s 通过）。
- `make dev-check`：exit 0（六模块 vet/short + Rust fmt/clippy/workspace tests，mornlea_client/engine 236+ 通过）。
- `openspec validate --all --strict --no-interactive`：96/96 通过。
- 整分支独立审查：SPEC pass（语义桥四端一致、两次点击闭环、输入隔离、物品同源、人物 6 件与步态公式、掉落 800 有界分组、GPU 面板退役）；QUALITY pass（无新依赖、无版权素材、无任务编号注释、10 commits 均为单行英文 conventional、回放隔离仅换测试生成器且生产零改动）。无阻塞缺陷。
- Ruling：Task 5 可关闭；不推送、不合并，等待发布请求。

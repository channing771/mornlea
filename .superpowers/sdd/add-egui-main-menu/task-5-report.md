# Task 5 报告 — Go: 主菜单状态机与延迟世界装配

## 状态

**SPEC: PASS … QUALITY: PASS（implementer 自评；由独立 task reviewer 定稿）**

提交：`d8a3f05a`（`feat: defer world boot behind main menu`，仅追加 commit，未 amend）。

实现范围：仅 `cmd/mornlea`（package main）；未触碰 `internal/*`、未改 AGENTS/CLAUDE、未提交 change 产物。非 StartAtMenu（`-connect`/benchmark/capture）路径经既有测试护栏确认逐字节不变。

## 实现清单

1. `applicationOptions.StartAtMenu bool`（app.go）；`main.go` 在与 CompanionDefinitions 注入相同的交互本地条件（`Connect == "" && !Benchmark && CaptureDir == ""`）内置 true。
2. `application` 新增 `startupOptions`、`startupDeps`、`menu`、`menuOverride` 字段。
3. `app_menu.go`（新文件）：`menuPhase`（game=0 零值 / menu / starting）、`menuState`（phase/starting/title/version/error）、`menuActionStart/Multiplayer/Settings/Quit` 常量、`menuVersion()`、`menuButtons()`、`menuState.uiMenu()`、`(a) uiSegment()`、`(a) handleMenuEvent(id) quit`。
4. `newApplicationWithDependencies`（app_startup.go）：`StartAtMenu` 时跳过 store/openStore/assemble/attachLodScheduler/receiver（与既有 nil store 逻辑各归其位），仍创建 window/renderer/glyphAtlas/hotbar/材质上传；快照 `startupOptions`/`startupDeps`；初始化 `app.menu`（`menuPhaseMenu`）。非 StartAtMenu 路径保持游戏相位（零值）。
5. `(a) startWorld()`：抽出既有装配链路（`openStore` → `assembleLocalApplicationConnection` → `attachLodScheduler`），复用既有函数与错误包装；成功置 `menu.phase=game`，失败返回 error。
6. `runInteractive` 改两相位（interactive.go）：`runInteractive` 按 `menu.phase != game` 路由到 `runMenuPhase` 或 `runGamePhase`；`runMenuPhase` 不捕获光标、不驱动 WASD/面板/聊天/快捷栏，Poll → DrainUIEvents → 分派 → 渲染（含 UI 段）；`runGamePhase` 为既有循环体原样。
7. `renderFrame`（app_frame.go）向 `client.RenderFrame` 追加 `UISegment: a.uiSegment()`。
8. （必要配套）字体上传：`newApplicationWithDependencies` 在渲染器创建后、非 benchmark 时 `rustRenderer.UploadUIFont(render.EmbeddedCJKFont())`——spec 要求菜单可渲染，缺失时 Rust 以编程错误拒绝 UI 帧。

## 相位转移图

```
StartAtMenu 构造 ──► menu（phase=menu，starting=false）
    │ 点击「进入游戏」(handleMenuEvent start)
    │    starting=true，phase=starting
    │    startWorld（store 打开 → assembleLocal → attachLodScheduler）
    ├── 成功 ──► phase=game，starting=false，SetCursorCaptured(true)
    │           └──► runGamePhase（既有循环体）
    └── 失败 ──► phase=menu，starting=false，error=<错误串>（菜单保持可见，可重试/退出）
    │ 点击「退出游戏」(handleMenuEvent quit) ──► 返回 quit=true ──► runInteractive 返回 nil
    │ 点击 多人/设置/未知 id ──► 忽略（不改变状态）
    │ 窗口关闭 ──► 返回 nil
```

## 与 design.md 的偏差

- **[设计原文]** `menuVersion()`：“`Main.Version` 非空用之，否则 `"dev"`”。**[实现]** 对 `(devel)`（本地未打版本标签的构建）也返回 `"dev"`。理由：spec「版本行 SHALL 显示应用版本号，无法取得时显示 `dev`」；`(devel)` 视为“无法取得真实版本号”。属最小、可辩护的偏离。
- **[设计原文]** 装配成功“立即 `SetCursorCaptured(true)` 并刷新 lastMouse 基线”。**[实现]** 光标捕获放在 `handleMenuEvent`（start 成功分支，窗口非 nil 时执行），`runMenuPhase` 在检测到 `phase==game` 后直接交给游戏相位（避免重复捕获）；lastMouse 基线由 `runGamePhase` 顶部重读 `CursorPos()` 刷新。行为一致。
- **[设计原文]** “`runInteractive`：菜单相位循环…`renderFrame`（带 UI 段）”。**[实现]** 抽出 `runMenuPhase`/`runGamePhase`，`runMenuPhase` 每帧渲染（含 UI 段）；渲染在事件分派之后、与 design 一致。
- **[设计原文]** “无 UI 帧 egui 零参与”。**[实现]** `appendTLV(frameTagUI, frame.UISegment)` 空段自动缺席（既有 client 行为），游戏相位 `UISegment=nil` → 帧逐字节不变（capture golden 验证）。
- **[配套新增（超出 brief 字面）]** 字体上传接线：brief 需求未列但 spec 要求菜单可渲染，缺失时 Rust 拒绝 UI 帧（输入隔离测试暴露 panic）。按 design「交互与 capture 都上传；benchmark 不上传、菜单零参与」实现。

## 测试与命令输出（本机实测）

- `go test ./cmd/mornlea -race -count=1`：`ok  ... 257.035s`（含既有 -connect/benchmark/capture/多玩家等全量 + 新 menu 测试）。
- `go test ./internal/archcheck -count=1`：`ok  ... 6.253s`。
- `go vet ./...`：通过（无输出）。
- `gofmt -l .`：无输出（全仓格式干净）。
- `go vet ./cmd/mornlea`：通过（实跑记录）。
- 新增测试（`cmd/mornlea/app_menu_test.go`）全部 PASS：`TestStartAtMenuConstructsMenuWithoutWorld`、`TestHandleMenuEventStartAssemblesWorld`、`TestHandleMenuEventStartFailureKeepsMenu`、`TestHandleMenuEventStartIgnoredWhileStarting`、`TestHandleMenuEventQuitRequestsClose`、`TestHandleMenuEventUnknownIDIgnored`、`TestMenuPhaseInputIsolation`、`TestUISegmentRealMenuEncodingLock`。

## git status（提交后）

```
 M  cmd/mornlea/app.go
 M  cmd/mornlea/app_frame.go
 A  cmd/mornlea/app_menu.go
 A  cmd/mornlea/app_menu_test.go
 M  cmd/mornlea/app_startup.go
 M  cmd/mornlea/interactive.go
 M  cmd/mornlea/main.go
（待提交的既有未跟踪文件未纳入：.gocache-task5/、.review-gocache/、docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md；报告 .superpowers/sdd/... 位于 gitignored 目录，未 git add）
```

## SPEC 自评

- 交互客户端启动停留主菜单、世界未装配（server/host nil、菜单相位、按钮/版本行正确）：覆盖在构造测试 + 依赖零调用断言。
- 「进入游戏」延迟装配：startWorld 复用 openStore/assemble/attachLod；成功 phase=game + 光标捕获。
- 装配期间重复点击只装配一次：starting 防重入守卫测试（构造中忽略、正常一次）。
- 装配失败展示错误且可退出：menu.error 非空、相位回菜单、quit 可用。
- 退出游戏关闭客户端：quit 返回退出信号。
- 菜单期间游戏输入不生效：菜单相位不读取 WASD/点击/文本、不捕获光标（借间谍窗口运行一帧菜单循环验证）。
- 真实菜单跨语言段编码（Ruling 8 非 4 对齐）：uiSegment 字节长度与算术预期一致且非 4 对齐。

## QUALITY 自评

- 复用既有 `openApplicationStore`/`assembleLocalApplicationConnection`/`attachLodScheduler` 与既有错误包装，不复制逻辑；装配配置收拢为 `buildApplicationServerConfig` 供两条路径共用。
- 注释中文，新标识符有中文 GoDoc；Go 标识符在注释中尽量用反引号（`menuPhaseGame`、`startWorld`、`uiSegment` 等）。
- 只改 `cmd/mornlea`；非 StartAtMenu 路径通过既有测试全量护栏确认不变；capture golden 未变。
- 新增 8 个测试先失败后实现，复用假窗口/依赖注入，无真实窗口，GPU 测试 skip 保护。

## 遗留担忧

1. **字体上传范围**：按 design「交互与 capture 都上传」接入 `!options.Benchmark`。spec「capture 仅在 main-menu 场景需要时上传字体」字面上更严；本实现 capture 也上传（功能无害、golden 未变，因非 main-menu 场景无 UI 段 → egui 零参与）。若评审要求严格 scene 门控，可挪到 Task 6（capture main-menu 场景）按 scene 上传。
2. **`menuVersion()` 对 `(devel)` 的处理**：将其映射为 `"dev"`，与 design 字面“非空用之”略有出入；如需严格字面，可去掉 `(devel)` 特判。
3. **`startWorld` 失败清理**：store 打开失败/assembleLocal 失败由既有函数自行清理；`attachLodScheduler` 失败由 `releaseWorldConnection` 清理半装配连接并复位字段（可重试）。若未来出现并发重试，需注意 hostDone 通道不被二次消费（当前单线程菜单循环无此风险）。
4. **菜单相位未覆盖 Escape**：菜单期 Escape 被 Rust egui 抢占（不产生游戏动作）；spec「菜单期间游戏输入不生效」满足，但「菜单显示期间 Escape 退出」未在本任务范围（设计未要求）。
# add-world-loading-screen 任务

约束：每任务 fresh implementer + TDD（red→green→refactor）+ SPEC/QUALITY 双评审；
代码注释一律中文且不得出现任务编号；提交信息单行英文
`<type>(<scope>): <subject>`。基线 SHA `06809724`。

跨端切分说明：桥 schema（`frontend/src/bridge/schema.json`）的扩展随任务 1
落地（Go 钉值测试以它为单源）；TS `client.ts` 与 UI 随任务 2 同步——分支内
暂态与 `add-mining-crack-overlay` 的 Go/Rust 分批先例一致，合入前闭环。

## 1. Go 相位机、加载循环与桥 schema

- [x] 1.1 失败测试先行（`cmd/mornlea/app`）：`MenuPhaseLoading` 的
  `uiPhase()` 映射 `"loading"`；`startWorld` 成功置 `MenuPhaseLoading`
  （既有期望 `MenuPhaseGame` 的测试同步改判）；`handleMenuEvent` 装配成功后
  不再捕获光标（窗口替身断言无 `SetCursorCaptured(true)`），失败路径回菜单
  语义不变；`runMenuPhase` 在相位变为 loading 时返回；`buildUIState` 在
  loading 相位组装 `loading` 分节（`loaded=len(loadedChunks)`、
  `total=LoadedChunkTarget`），其余相位分节缺席；`runLoadingPhase` 收敛即置
  `MenuPhaseGame` 并捕获光标、窗口关闭返回 nil、`receiver.Err` 走
  `CloseClientSession` 返回错误（沿既有 fake/直构测试模式）。
- [x] 1.2 实现：`app_menu.go` 加相位与映射并移除装配成功处的光标捕获；
  `app_startup.go` `startWorld` 置 loading；`interactive.go` `RunInteractive`
  loading 路由 + `runLoadingPhase`（Poll → pumpDevCapture → drain 桥事件
  （未知告警忽略）→ `Frame(MessageDrainMax, MessageDrainMax, dt)` →
  `ApplicationLoadComplete` 检查；dt 钳制 100ms；每 5s 进度日志）；
  `app_ui_state.go` `uiStateJSON.Loading` 与组装。
- [x] 1.3 schema 单源：`frontend/src/bridge/schema.json` `phase` 枚举追加
  `"loading"`、新增 `$defs/loadingState`（`loaded`≥0、`total`≥1、
  additionalProperties false）、`uiState` 属性与描述同步；`app_ui_state_test.go`
  钉值测试用同一文件校验 loading 文档。
- [x] 1.4 验证：`go test ./cmd/mornlea/app -race -count=1`；
  `go test ./internal/archcheck -count=1`；`gofmt -l` 无输出。

## 2. 前端加载屏组件

- [x] 2.1 失败测试先行（`frontend/src`）：`bridge/client.ts` 相位联合与
  `loading` 分节守卫（合法文档接受、未知相位/越界字段拒绝）；`App` 在
  `phase="loading"` 渲染 LoadingScreen 且不渲染 MainMenu；
  `routeKeyDown` loading 档对 Enter/Esc 零上行；`LoadingScreen` 比例钳制
  `[0,1]`、计数格式 `区块 x / y`、`total<=0` 或缺省安全降级。
- [x] 2.2 实现：`bridge/client.ts` 类型与守卫；新组件 `ui/LoadingScreen.tsx`
  （不透明全屏遮罩 + 标题 + 进度条 + 计数行；文案入 `copy.ts`；颜色/几何走
  `tokens.css` 单源，遵守 `prefers-reduced-motion`）；`App.tsx` 两处 switch
  补 loading 档；`ui/ui.css` 样式。
- [x] 2.3 部件基线：`visual/fixture-names.ts` + `visual/fixtures.tsx` 注册
  `panel-loading`（18→19）；`make frontend-visual-update` 生成基线 PNG 并人工
  确认；`make frontend-visual-check` 通过。
- [x] 2.4 验证：`make frontend-check`（含 vitest 与 dist 逐字节门禁）。

## 3. Rust 相位清单与收尾门禁

- [x] 3.1 失败测试先行：`mornlea_client/src/webview.rs` 菜单族相位清单测试
  追加 `"loading"`（`state_wants_visible_parsed` 为真、参与模式 `Menu`）；
  `overlay.rs` 模块文档菜单族枚举补 loading（文档级，无行为断言变化）。
- [x] 3.2 实现 + 验证：`make rust`；
  `(cd engine && cargo test -p mornlea_client --locked)`；`go build ./...`。
- [x] 3.3 收尾门禁：`gofmt -l .` 无输出；`go vet ./...`；
  `go test ./... -race`；`make rust-check`；
  `openspec validate --all --strict --no-interactive`；
  `make visual-check`（世界 golden 零变化）。

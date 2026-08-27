## 1. Rust: layout v3 解码与面板控件

- [x] 1.1 在 `engine/crates/mornlea_client/src/ui.rs` 新增 `UI_DEBUG_LAYOUT_VERSION: u32 = 3` 常量、`DEBUG_PANEL_ACTION_*` 常量（选中移动、进入编辑、编辑值、确认、取消、关闭）与 `MAX_DEBUG_PANEL_ROWS = 64`、`MAX_DEBUG_PANEL_RUNES_PER_SIDE = 24` 常量，参照既有 `UI_LAYOUT_VERSION`/`UI_SETTINGS_LAYOUT_VERSION` 写 GoDoc 注释；在此文件补单元测试：合法 v3 段通过、未知 layout 拒绝、行数/标签/值超上界拒绝、尾随字节拒绝、非法 UTF-8 拒绝。验证：`cargo test -p mornlea_client` 全绿。
- [x] 1.2 实现 `decode_ui_frame` 中 layout v3 分支解码（段头 + 定宽行记录 → egui 可用视图）；新建 debug 面板 egui 绘制函数（顶部只读读数区 + 参数行列表 + 编辑态 UI 元素），并接入既有 egui 主循环绘制。验证：既有 `cargo test -p mornlea_client` 全绿，新增 UI 绘制函数的 parser 单元测试通过。
- [x] 1.3 在 `ffi.rs`（若需将面板状态经 CGo 传递给 Go，或事件批回传新增面板动作类型）新增/扩展相关 ABI 出口与常量；同步 `engine/include/mornlea_client.h`。验证：`cargo test -p mornlea_client` 全绿，`go build ./internal/client` 无编译错误。

## 2. Go: layout v3 段编码与事件回传

- [x] 2.1 在 `cmd/mornlea/debug_panel.go` 将 `panelFrameInput()` 输出改为编码 layout v3 段（保留 `remote()`、`panelModeLabel()`、`applyPanelChange()`），并新增 `decodeDebugPanelEvents()` 解析 Rust 回传的面板动作事件（按 `UI_ACTION_DEBUG_*` 类型）。验证：`go test ./cmd/mornlea -run DebugPanel -count=1 -race` 全绿。
- [x] 2.2 在 `internal/client/window.go` 新增编辑键（方向键/Enter/Esc 的既有 `KeyUp/KeyDown/KeyLeft/KeyRight/KeyEnter/KeyEscape` 之外，加入 debug 面板编辑所需的文本输入/光标移动键枚举）并同步 `input.rs` 映射；保持既有 `KeyF3` 触发。验证：`go test ./internal/client -count=1 -race` 全绿，`go test ./...` 无回归。

  **实现注**：未新增 Go 键枚举——`internal/client/window.go` 与 `input.rs` 本变更未改动。方向键/Enter/Esc 的既有 `Key*` 映射在 egui 获得焦点后已可直接消费，无需新枚举即可驱动 egui 行选中与编辑会话；数字/字符串文本输入复用 D-01 既有 winit→`UiEvent::Text` 管线承载，egui `TextEdit` 从其消费字符事件，Go 侧只经既有结构化事件批接收 `UI_ACTION_DEBUG_*` 动作（`decodeDebugPanelEvents`）。无既有键可复用的场景不存在，新增枚举只会增加 ABI 面而无消费方。
- [x] 2.3 在 `cmd/mornlea/debug_panel.go` 接线：F3 边沿仍由 Go 检测；面板可见时全捕获（复用 `panelBlocked` 既有逻辑）；从 Rust 事件批取选中移动/编辑/确认/取消并更新 `panelState` + 调 `applyPanelChange()`；`remote()` 时强制只读。验证：`go test ./cmd/mornlea -run DebugPanel -count=1 -race` 全绿。

## 3. Go: 删除旧程序化渲染路径

- [x] 3.1 在 `internal/render/debug_panel.go` 删除程序化绘制（`Prepare`/`QuadCount`/`GlyphCount`/`FrameStreams` 与 `nRenderer` 相关），保留 `PanelRow`/`PanelReadout` 类型与 `config.Fields()`/`rows()` 数据源翻译层（供 layout v3 编码复用）。验证：`go test ./internal/render ./cmd/mornlea -count=1 -race` 全绿。
- [x] 3.2 在 `internal/render/frame_streams.go`/`app_startup.go` 删除对旧 `nRenderer` 的程序化初始化与接线，改为新的 egui 面板路径构造。验证：`go test ./internal/render ./cmd/mornlea -count=1 -race` 全绿，`go build ./...` 无错误。

## 4. 集成与验证

- [x] 4.1 更新 `cmd/mornlea` 的 debug 面板交互测试：F3 边沿开关、面板可见/隐藏捕获、行选中/编辑/写回、非法值拒绝、远程只读、面板隐藏复位的测试断言。验证：`go test ./cmd/mornlea -run DebugPanel -count=1 -race` 全绿。
- [x] 4.2 更新 `engine/crates/mornlea_client` 集成测试：debug panel 状态段编码的往返一致性（Go 编码段 → Rust 解析 → Golang 断言），非法段拒绝，可见态捕获事件回传。验证：`go test ./... -run Panel -count=1 -race`（或相应 Rust 集成 crate）全绿。
- [x] 4.3 收尾验证：`gofmt -l .`、`go vet ./...`、`go test ./... -race -count=1`、`make rust`/`cargo test -p mornlea_client`、`openspec validate --all --strict --no-interactive` 全部通过。

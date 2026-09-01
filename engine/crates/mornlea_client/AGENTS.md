# Rust 图形客户端

## 所有权

- 本 crate 持有 Darwin 窗口、事件采集、GPU 资源与 pass、shader、进程内 WKWebView 菜单层和 client ABI 出口。菜单呈现层源码在本 crate 的 `frontend/`（Vite + TypeScript + React），桥协议单源为 `frontend/src/bridge/schema.json`；前端纪律见 `frontend/AGENTS.md`，本 crate 只经桥出口与 WebView 交互，不导入前端源码。
- Darwin 本地提示音的 `AudioQueue` 实现在 Go `internal/audio`，不属于本 crate；本 crate 只呈现设置 UI 中的音量值。
- Linux 专用服务端不得依赖本 crate；非 Darwin workspace 构建保持空平台实现，不引入窗口或 GPU 运行时。
- WebView 参与模式恰有两态，由 Go 经既有桥下行相位驱动，不新增 C ABI 出口：
  `Menu`（菜单相位，挂载并消费输入，菜单 chrome 可交互）与 `GameOverlay`（游戏
  相位常显阶段，可见并透明合成于 wgpu 画面之上承载常显 HUD，WKWebView 子类
  `hitTest:` 返回 `nil` 使鼠标/键盘/滚轮全部穿透到既有 winit 采集路径——不产生
  任何上行桥事件、不改变光标捕获状态）。生产两态实现住 `src/overlay.rs`
  （`OverlayMode`，由下行 phase 与 `debug.visible` 推导）；`-connect`、benchmark
  与 capture 路径永不创建 WebView。

## Client ABI

- ABI 变化同批更新 `engine/include/mornlea_client.h`、本 crate FFI、`internal/client` bridge、版本和跨语言测试。
- FFI 先校验 handle、线程、pointer、length、layout 和输出容量，panic 转稳定状态码，失败不写部分结果。

## 渲染与无头路径

- GPU pass 顺序、资源池上限、instance/frame 布局和 overflow 都是渲染契约；修改时以代码和测试为真相，不在指南复制场景数或容量数字。
- 预热后热路径不得动态创建每帧资源。容量不足应显式失败或按既有整批语义跳过，不能截断成表面成功。
- offscreen capture/benchmark 不创建交互窗口、不聚焦或捕获光标；它们也不得间接触发 Go 侧音频设备。

## spike 遗留（`src/overlay_spike.rs`、`src/spike_auto.rs`）

- 这两个私有模块是 `webview-game-ui-unification` Phase 1 的前置 spike 产物，属
  验收后待移除的遗留：`overlay_spike.rs` 承载 `hitTest:` 分级与 present 边界帧
  探针的最小验证实现，`spike_auto.rs` 承载进程内自动进入游戏、合成 `NSEvent`
  注入与逐条断言的自驱动档。
- 这些入口都由环境变量门控（`MORNLEA_SPIKE_OVERLAY`/`MORNLEA_SPIKE_FPS`/
  `MORNLEA_SPIKE_AUTO`），默认全关即生产行为逐字节不变；生产路径不得消费 spike
  模块，也不得为它新增 C ABI 出口。两态参与的生产真相是 `src/overlay.rs`。

## UI 部件视觉基线（`frontend/visual/`）

- HUD 部件的呈现与类名住 `frontend/src/hud/`，类名一律 `hud-*` 前缀以与
  retroui 产物带入的通用工具类名隔离；`frontend/visual` 的 HUD 部件 fixture
  白名单恰为六个：`hud-status`、`hud-hotbar`、`hud-progress`、
  `hud-popup-crosshair`、`hud-container-open`、`hud-chat`，基线 PNG 在
  `frontend/visual/golden/`。
- 基线管线、双阈值与更新纪律见 `frontend/AGENTS.md` 的「UI 部件视觉基线」一节；
  本 crate 不读取这些基线，也不把部件像素验收下放到 capture golden。

## 定点验证与入口

- 测试：`cd engine && cargo test -p mornlea_client --locked`。
- 当前文档入口：`openspec/specs/rust-client-window/spec.md`、`openspec/specs/rust-client-render-cutover/spec.md`。

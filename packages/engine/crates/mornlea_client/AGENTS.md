# Rust 图形客户端

## 所有权

- 本 crate 持有 Darwin 窗口、事件采集、GPU 资源与 pass、shader、进程内 WKWebView 菜单层和 client ABI 出口。菜单呈现层源码在本 crate 的 `frontend/`（Vite + TypeScript + React），桥协议单源为 `frontend/src/bridge/schema.json`；前端纪律见 `frontend/AGENTS.md`，本 crate 只经桥出口与 WebView 交互，不导入前端源码。
- Darwin 本地提示音的 `AudioQueue` 实现在 Go `packages/client/audio`，不属于本 crate；本 crate 只呈现设置 UI 中的音量值。
- Linux 专用服务端不得依赖本 crate；非 Darwin workspace 构建保持空平台实现，不引入窗口或 GPU 运行时。
- WebView 的透明合成与输入参与分别由下行相位和 `game.cursorFree` 驱动，不新增 C ABI 出口。菜单、暂停、设置与调试面板消费键鼠；游戏中自由光标或背包/人物/工作台/箱子/熔炉面板也由 WebView 消费输入，保持透明世界背景。捕获态 `GameOverlay` 的 `hitTest:` 返回 `nil`，输入完全穿透至 winit。
- 交互窗口（包括 `-connect`）首次下行即挂载同一前端；首次挂载必须应用参与状态。每次 WebView/winit 焦点交接清理原生键鼠残留。Tab 释放光标，前端 Tab/世界背景恢复捕获；E/Esc 与数字键通过带视图 token 的严格 `game-action` 回 Go。capture/benchmark 没有交互窗口且从不挂载 WebView。
- 游戏面板与 tooltip 只在前端绘制，生产帧不提交它们的 GPU quad/glyph。固定 client ABI v15 的兼容布局、容量与编解码保持不变。

## Client ABI

- ABI 变化同批更新 `packages/engine/include/mornlea_client.h`、本 crate FFI、`packages/client/client` bridge、版本和跨语言测试。
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
  原有六个常显部件：`hud-status`、`hud-hotbar`、`hud-progress`、
  `hud-popup-crosshair`、`hud-container-open`、`hud-chat`，另有个人背包、工作台、箱子、熔炉、人物及窄屏面板 fixture；基线 PNG 在
  `testdata/visual-golden/ui/`。
- 基线管线、双阈值与更新纪律见 `frontend/AGENTS.md` 的「UI 部件视觉基线」一节；
  本 crate 不读取这些基线，也不把部件像素验收下放到 capture golden。

## 定点验证与入口

- 测试：`cd packages/engine && cargo test -p mornlea_client --locked`。
- 当前文档入口：`openspec/specs/rust-client-window/spec.md`、`openspec/specs/rust-client-render-cutover/spec.md`。

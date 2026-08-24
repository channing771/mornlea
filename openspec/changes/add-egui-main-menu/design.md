# 设计：egui 主菜单（客户端首个工具型 UI 竖切）

## Context

行为契约见两个 delta spec（`egui-tool-ui`、`visual-verification`），动机与范围见 `proposal.md`。技术栈选型（egui、弃 iced/Slint/RAUI/bevy_ui/Go GUI/webview 的理由）见 `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md`，本设计实现该选择的集成约束并交付主菜单竖切。

当前渲染架构：Go CPU 半部做布局与编码（quad/glyph 流经 `client.EncodeRenderFrame` 的 TLV 帧上传），Rust `mornlea_client` 独占 GPU（`OffscreenRenderer` 同时服务窗口 surface 与离屏 capture/benchmark，`FrameInput` 是每帧输入，`parse_frame` 在 ffi.rs 解码 TLV）。输入方向：`mornlea_client_window_poll` 返回固定 4160 字节快照（键位/鼠标位/光标/尺寸/文本），Go 在 `internal/client` 解码。

## 版本裁决（Ruling 基础）

选型文档假定 egui 0.36.1；crates.io 实测（2026-08-23）：`egui-wgpu` 0.36.1 声明 `wgpu ^30.0`，仅 0.35.x 声明 `wgpu ^29.0`。集成约束第 1 条「绝不单侧升级 wgpu」即裁决：**采用 egui 0.35.0 + egui-wgpu 0.35.0**（egui 0.35 rust-version 1.92 ≤ 固定 1.97.1；egui-wgpu 对 egui 的依赖为 default-features=false，故关闭 `default_fonts` 成立；egui-wgpu 0.35.0 的 wgpu 依赖 default-features=false + std/wgsl，与仓库直接 wgpu 29 同线）。不升级 wgpu 29 → 30，也不 patch egui-wgpu 源码（ABI 面大、维护成本高）。

## 数据所有权与依赖方向

- Go `cmd/mornlea`（package main）拥有菜单**语义状态**：相位（menu → starting → game → closed）、按钮表（id/label/enabled）、标题、版本行、装配错误行。Rust 不产生任何游戏/菜单语义（不内置按钮行为）。
- Rust `mornlea_client` 拥有 egui 的**呈现与输入**：`egui::Context`、字体表、纹理、每帧 `RawInput`、egui pass、事件队列（点击事件回 Go）。布局几何（按钮矩形/列位置）是呈现职责，在 Rust 绘制函数里确定性计算。
- 双向通信全走 client ABI v8：下行（Go→Rust）帧内 UI 段（TLV tag 9）与一次性字体上传；上行（Rust→Go）`render_drain_ui_events` 回读 u32 事件 id 批。
- 无新增 Go 包、无新增 Rust crate、不触碰 `internal/archcheck` 白名单、不改变 `engine/mornlea_engine`。

## Rust 侧结构与关键点

### 依赖（engine/crates/mornlea_client/Cargo.toml，仅 macos target 组）

```toml
egui = { version = "0.35", default-features = false }          # 关闭 default_fonts
egui-wgpu = { version = "0.35", default-features = false, features = ["macos-window-resize-jitter-fix", "wgpu/default"] }
```

说明：egui-wgpu 的 wgpu 依赖 default-features=false（std/wgsl），仓库已有直接 wgpu=29（默认 feature，含 metal 后端）；"macos-window-resize-jitter-fix" 转发 wgpu/metal。不启用 "webgl"/"winit"/"capture" feature；不引入 egui-winit。

### `src/ui.rs`（新模块，纯状态 + egui Context，可无 GPU 单测）

- `pub struct UiFrame`：`visible: bool`、`title: String`、`version: String`、`error: String`、`buttons: Vec<UiButton>`（`id: u32`、`label: String`、`enabled: bool`）。`decode_ui_frame(bytes: &[u8]) -> Result<UiFrame, ()>` 实现 ABI 解码与校验：layout v1（u32）、flags（u32，bit0=visible）、按钮数（u32，≤ `MAX_UI_BUTTONS=8`）、每按钮 `id u32 + label_len u32 + label UTF-8`（label ≤ 64 字节）、`title`（≤128 字节）、`version`（≤64 字节）、`error`（≤256 字节）；整段 ≤ `MAX_UI_SEGMENT_BYTES=4096`；任何越界/非 UTF-8 返回 Err（ffi 层转 INVALID_ARGUMENT）。
- `pub struct UiState`：`egui::Context`、字体（`set_fonts` 在首次有字体字节时安装：proportional 与 monospace 族 = 上传的 Noto CJK，不装任何内嵌字体）、`pending_events: Vec<u32>`、`font_loaded: bool`。方法：
  - `install_font(&mut self, bytes: &[u8])`：每渲染器一次（重复上传幂等/替换）；
  - `run_frame(&mut self, raw: egui::RawInput, frame: &UiFrame, pixels_per_point: f32) -> Option<egui::FullOutput>`：`ctx.run` 内按 `frame` 绘制：全屏不透明深灰背景面板（MC 标题画面基调，egui Style 调参，不 hack shader）、大标题「Mornlea」（RichText heading 白字）、中心纵排按钮（宽约 220px、高约 40px，间距 8px，`add_enabled(enabled, Button)`，样式走 egui Style 常量）、底部版本行（小字，左下 12px 边距）。`frame.error` 非空时在按钮列下方显示红色错误行。点击响应（`response.clicked()`）把按钮 `id` 压入 `pending_events`；禁用按钮不产生点击事件。
  - `drain_events(&mut self) -> Vec<u32>`：排空前读取；
  - `has_font(&self) -> bool`。
- `pub fn raw_input(events: &[UiEvent], screen_rect: egui::Rect, pixels_per_point: f32, time: Option<f64>) -> egui::RawInput`：把 `UiEvent` 翻译成 `egui::Event` 序列；`RawInput` 的 `screen_rect`、`pixels_per_point`、`viewports`（ROOT 视口 `inner_rect`）、`modifiers`（从事件推导）、`focused: true`；时间戳：菜单绘制无动画，固定 `time: None`（golden 确定性）。
- `pub enum UiEvent`：`CursorMoved(f64, f64)`、`MouseButton(bool /*primary*/, bool /*pressed*/)`、`Key { key: egui::Key, pressed: bool, modifiers: egui::Modifiers }`、`Text(char)`、`Scroll(f32, f32)`、`CursorGone`。翻译函数 `pub fn winit_to_ui_events(...)` 以纯函数形式存在以便单测（输入 winit 事件类型，输出 `Vec<UiEvent>`）。
- 输入传输：`thread_local! { static UI_EVENTS: RefCell<Vec<UiEvent>> }`——`window.rs` 的 `App` 在事件回调里 push（仅当窗口与渲染器同线程；Go `LockOSThread` + 渲染器与窗口同栈，单测直接测翻译函数与队列 API，不建真实窗口）。`render_frame` 在运行 egui 前 `UI_EVENTS.take()`；**无 UI 段的帧同样 take 并丢弃**（防止积压：菜单只在 Go 菜单相位产生 UI 段，游戏帧丢弃窗口期输入是「菜单关闭时 egui 不消费」的实现）。
- 单测（无 GPU）：decode_ui_frame 边界（截断/越界/非 UTF-8/空帧）、menu 布局与命中（给固定逻辑尺寸合成 `UiFrame`，用 `RawInput` 在按钮矩形中心派发 Press+Release，断言 `drain_events` 序列；禁用按钮无事件；同一指针点只命中一个按钮）、`raw_input` 翻译（事件 → egui::Event 精度）、字体安装后 `has_font`、无动画静态帧两次相等（same RawInput → same FullOutput shapes 摘要）。

### `src/render/egui.rs`（新模块，GPU 半部）

- `pub struct EguiPass { renderer: egui_wgpu::Renderer, ui: UiState, textures: HashMap<egui::TextureId, wgpu::Texture>, font: Option<Vec<u8>>, screen: egui_wgpu::ScreenDescriptor }`。`new(device, color_format, width, height)`：`egui_wgpu::Renderer::new(device, color_format, None, 1, None)`（`None` = 不用 depth、不抖动）。
- `set_size(&mut self, w, h)`：更新 `ScreenDescriptor` 与 `UiState` 的 screen 尺寸（resize 路径调用）。
- `upload_font(&mut self, bytes: &[u8]) -> Result<(), ()>`：字节量校验（>0 且 ≤ 32 MiB），存 `font`，并 `UiState::install_font`。
- `run_and_record(&mut self, device, queue, encoder, frame_view, frame: &UiFrame, events: Vec<UiEvent>, pixels_per_point, size) -> Option<()>`：无字体/无 UI 段均返回 None（零工作）；否则 `full = ui.run_frame(...)`；`full.textures_delta` 经 wgpu 建/删 `TextureId` 纹理（egui-wgpu 0.35 的 texture API 以 docs.rs 为准：`update_texture/free_texture` 或 `update_buffers` 内嵌路径）；tessellate 得 `PaintJobs`；`renderer.update_buffers(...)`；随后 `begin_render_pass(load: LoadOp::Load, store)` 且 `renderer.render(pass, jobs, screen_descriptor)`。pass 标签 `"egui pass"`，不写 depth、无 resolve。
- `drain_events(&mut self) -> Vec<u32>`：转发 `UiState::drain_events`。

### `render/mod.rs` 集成

- `FrameInput` 追加 `ui_segment: Vec<u8>`（空 = 本帧无 UI）；`empty_passes` 语义不含 UI。
- `OffscreenRenderer` 追加 `egui_pass: Option<EguiPass>`（创建即 Some；`new_windowed` 与离屏 `new` 同构）。
- `render_frame` 在 debug pass **之后**追加：先 `decode_ui_frame`（Err → 按既有语义校验失败即拒绝且不触碰 target，Go 侧表现为 panic 编程错误），`events = UI_EVENTS.take()`；`ui_segment 为空` 时丢弃 events 且不再动作；否则 `egui_pass.run_and_record(...)`（像素密度：窗口模式取窗口 scale_factor，离屏固定 1.0）。
- `resize` 转发 `egui_pass.set_size`。
- 帧排版注意：egui pass 在 HUD/debug 之后 = 最上层；无深度测试，与 HUD 一致的 screen-space 语义。

### `ffi.rs` / header / `lib.rs`

- `CLIENT_ABI_VERSION = 8`；注释写明 v8 增量（两个新出口 + 帧 UI 段），v7/v6 历史保留。
- `mornlea_client_render_upload_ui_font(abi, handle, bytes, len) -> u32`：ABI 校验 → 句柄 → 参数（bytes 非空、len ≤ 32 MiB）→ `egui_pass.upload_font`；状态码复用 OK/ABI/INVALID_ARGUMENT/WINDOW/PANIC，字体超限返回 CAPACITY（头文件已有该值）。
- `mornlea_client_render_drain_ui_events(abi, handle, out, out_len) -> u32`：返回写入的 u32 事件数；`out == null` 或 `out_len % 4 != 0` → INVALID_ARGUMENT；事件数超过 `out_len/4` 时写满并丢弃余下（调用方每帧排空）。
- `parse_frame` 增加 TLV tag 9（`FRAME_TAG_UI`），`FrameInput.ui_segment` 原样携带（合法性由 `decode_ui_frame` 校验）。
- ffi 单测：abi_version_is_eight、非法 UI 段渲染拒绝、drain 长度校验。
- `engine/include/mornlea_client.h`：`MORNLEA_CLIENT_ABI_VERSION 8u` + 两个出口声明 + 注释（v8 增量、v7 不可混装）；`lib.rs` 顶部文档更新。

## Go 侧结构与关键点

### `internal/client/render.go`

- `frameTagUI = 9`（TLV）；`RenderFrame.UISegment []byte`；`hasPassSegments` 计入。
- `type UIButton struct { ID uint32; Label string; Enabled bool }`；`type UIMenu struct { Visible bool; Title, Version, Error string; Buttons []UIButton }`；`EncodeUIMenu(menu UIMenu) []byte`：与 Rust `decode_ui_frame` 逐字节对应（u32 layout=1、flags、按钮数、id+len+label、title、version、error）；超界（按钮 >8、label >64 字节、title >128、version >64、error >256）panic（编程错误，与既有 segment 编码口径一致）。`EncodeRenderFrame` 在 TLV 末尾（water 之后）追加 `frameTagUI`。
- `Renderer.UploadUIFont(font []byte)`、`Renderer.DrainUIEvents() []uint32`（每次调用排空）；cgo 序言追加 `noescape/nocallback` 声明。
- 测试：`EncodeUIMenu` 的字节级 golden（含空按钮、含 error、最大规模）、越界 panic、`hasPassSegments` 计入 UI 段、drain 返回协议——Rust 解码侧有黄金字节夹具，Go 与 Rust 各持一份字节级测试，跨语言一致性由此锁定。

### `internal/render/font_atlas.go`

- 追加导出访问器 `EmbeddedCJKFont() []byte`（返回 `go:embed` 的原始 OTF 字节，只读）；不改变既有 glyph 生成路径。

### `cmd/mornlea` 启动与相位

- `applicationOptions` 追加 `StartAtMenu bool`；`main.go` 在交互本地条件（`Connect == "" && !Benchmark && CaptureDir == ""`，与 CompanionDefinitions 注入同分支）置 true；`-connect`/benchmark/capture 保持 false（行为不变）。
- `newApplicationWithDependencies`：`StartAtMenu` 时跳过 store 打开、`assembleLocalApplicationConnection`、`attachLodScheduler`（保持 window/renderer/atlas/glyph/UI 字体上传创建），把 `applicationOptions` 与 `applicationDependencies` 快照存到 `application`（`startupOptions`/`startupDeps`）；构造 `app.menu`（相位 `menuPhase`，错误为空）。非 `StartAtMenu` 路径逐字节保持既有行为。
- `application.menu`：`type menuState struct { phase menuPhase; title, version, error string; starting bool }`。按钮 id 常量：`menuActionStart=1 / menuActionMultiplayer=2 / menuActionSettings=3 / menuActionQuit=4`。版本行来源：`runtime/debug.ReadBuildInfo().Main.Version` 非空用之，否则 `"dev"`。
- `(a *application) startWorld() error`：把既有的 store 打开 + `assembleLocalApplicationConnection` + 登录种子 → `attachLodScheduler` 移到此处（复用既有函数，改签名最小化）；成功 → `menu.phase = game`、捕获光标；失败 → 返回 error。
- `runInteractive`：菜单相位循环（不捕获光标、不驱动移动/面板/聊天）：`Poll` → `DrainUIEvents` → 事件分派（start：`starting=true` 防重入 → `startWorld`；quit：返回；其它 id 忽略）→ `renderFrame`（带 UI 段：`client.UIMenu{...}`）；进入游戏后走既有循环体。菜单期 Escape/点击不产生游戏动作；装配成功立即 `SetCursorCaptured(true)` 并刷新 lastMouse 基线。

### capture 场景

- `captureScene` 增加可选字段 `Menu *captureMenuFixture`（Visible/Title/Version/Error + 按钮表；默认 nil）。`captureSceneImage` 在 Prepare 前把 `app.menuOverride` 置为 `scene.Menu`（nil 即清除），随后 settle 与最终帧在 `renderFrame` 里读取 override 生成 UI 段——每个场景天然清空上一场景的菜单，无 teardown 钩子。
- 新场景 `main-menu`：`Name: "main-menu"`、`WarmupFrames: 8`、`Menu: &captureMenuFixture{标题「Mornlea」、四按钮（多人/设置禁用）、版本 "dev"}`、Apply：`resetCapturePresentation(app)` + 相机钉在出生点上方（面板不透明覆盖全屏）；插在 `far-horizon` **之前**（场景表顺序、`far-horizon` 倒数第二、`water-underwater` 最后不变）。
- golden：仅新增 `main-menu.png`；其余 16 张 golden 文件不动，跑一遍黄金比对证明逐字节不变（egui pass 只在 UI 段存在时提交，是「既有场景零影响」的实现前提）。
- 字体上传：`newApplication` 在渲染器创建后，`!options.Benchmark` 时 `rustRenderer.UploadUIFont(render.EmbeddedCJKFont())`（交互与 capture 都上传；benchmark 不上传、菜单零参与）。

## 性能与资源

- 无 UI 帧：egui 上下文零运行（返回 None 提前），零额外 GPU 提交；既有帧预算不变。
- 菜单帧：每帧 O(按钮数) 的 egui 绘制与一次 pass；纹理按需生成（标题/按钮字形经字体 atlas，上限小），纹理释放跟随 egui `textures_delta`。
- UI 段 ≤ 4096 字节/帧、事件 ≤ 64/帧、字体 ≤ 32 MiB（实际 16 MiB OTF 一次上传）；全部有界。

## 兼容与迁移

- client ABI v7 → v8：同版本 = 同表面的不可混装契约；Go 与 Rust 同仓同构建产物交付，无历史存档/协议兼容问题。专服（`mornlea-server`）不链接 `mornlea_client`，不受影响；Linux 非 darwin 构建仍为空库（egui 依赖只在 macos target 组）。
- 协议 v26、engine ABI v6、benchmark scenario v19、存档各 schema 均不变。
- 配置格式不变（菜单不含设置项）；`-dev`、`-connect`、`-benchmark`、`-capture` 行为只以「跳过菜单」方式收敛到既有语义。

## 验证

- Rust：`cargo test --workspace --locked`、`cargo clippy --workspace --all-targets -- -D warnings`、`cargo fmt --check`；无 GPU 依赖部分完全覆盖菜单逻辑与 ABI 校验（真实窗口与 GPU 不进自动测试）。
- Go：`go test ./internal/client ./internal/render ./cmd/mornlea -race -count=1` + `go test ./internal/archcheck -count=1` + `go vet ./...` + `gofmt -l .`。
- capture：只更新 golden 一次，git diff 确认仅新增 `main-menu.png`；随后无更新跑的视觉门禁全绿。
- benchmark：运行记录性能数值（只记录，不改退出状态）；确认 scenario v19 版本常量未变。
- 收尾：`openspec validate --all --strict --no-interactive`、`git diff --check`。

## 被否决的方案

- **egui 0.36.1**：wgpu ^30，违反「绝不单侧升级 wgpu」；也否决升级 wgpu 29→30（跨后端一致性与全仓改动面失控）。
- **引入 egui-winit**：把 winit 版本耦合给 egui-winit，且菜单输入翻译量很小；裁决手工 RawInput（选型文档既定）。
- **egui 字体内嵌到 Rust crate**：16 MiB OTF 双份二进制进仓；改走 ABI 上传单一来源（Go embed 已存在，含 provenance）。
- **菜单期间世界继续在菜单后面运行（overlay）**：玩家在菜单期已存在于权威世界并会被模拟，「进入游戏」只剩隐藏遮罩语义；裁决延迟装配（proposal 的 Why）。
- **Go 侧做菜单布局与命中，Rust 只画**：等于继续手写布局/焦点/文本输入，与选型初衷相悖；布局几何归 Rust（egui 即时模式），语义状态归 Go。
- **capture 用「世界 + 菜单」组合帧做 main-menu 场景**：引入世界内容耦合与机器相关帧差异；裁决纯 UI 帧（面板不透明、无动画、time=None）。

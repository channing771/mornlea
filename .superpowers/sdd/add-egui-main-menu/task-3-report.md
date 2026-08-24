# Task 3 报告 — Rust: egui wgpu pass、帧集成与 client ABI v8

## 状态

完成。提交 f3b93252（feat: egui wgpu pass and client ABI v8 exports），7 个文件、+587/-20。

## 实现清单

### engine/crates/mornlea_client/src/render/egui.rs（新建）

- EguiPass：renderer: egui_wgpu::Renderer、screen: egui_wgpu::ScreenDescriptor、ui: crate::ui::UiState、font: Option<Vec<u8>>；EguiError 枚举与 MAX_UI_FONT_BYTES = 32*1024*1024。
- new(device, color_format, width, height, pixels_per_point)：以 Renderer::new(device, format, RendererOptions { msaa_samples: 1, depth_stencil_format: None, dithering: false, ..Default::default() }) 构造；ScreenDescriptor.size_in_pixels 取物理像素、pixels_per_point 传入。
- set_size(w, h, ppp)：更新 screen.size_in_pixels 与 pixels_per_point。
- upload_font(bytes) -> Result<(), EguiError>：空/超上限返回 Err(FontInvalid)；成功 UiState::install_font 并缓存字节。
- drain_events() -> Vec<u32>：转发 UiState::drain_events。
- run_and_record(device, queue, encoder, frame_view, frame, events) -> Result<bool, EguiError>：
  - !frame.visible || !has_font 时 Ok(false)；
  - raw = crate::ui::raw_input(&events, screen_rect, ppp, None)（time=None 保 golden 确定性）；
  - ui.run_frame(raw, frame, ppp) 得 FullOutput，None 时 Ok(false)；
  - 对 full.textures_delta.set 逐条 renderer.update_texture(device, queue, id, delta)，对 free 逐条 renderer.free_texture(&id)；
  - let jobs = self.ui.ctx().tessellate(full.shapes, full.pixels_per_point)；
  - let callback_buffers = self.renderer.update_buffers(device, queue, encoder, &jobs, &self.screen)；
  - begin_render_pass（label 为 "egui pass"，load=Load/store=Store、无 depth）后 let pass = pass.forget_lifetime(); 再 renderer.render(&mut pass, &jobs, &self.screen)；
  - queue.submit(callback_buffers)（回调缓冲随主编码器一并提交）；最后 Ok(true)。

### engine/crates/mornlea_client/src/render/mod.rs

- 新增 pub mod egui;；FrameInput 增 ui_segment: Vec<u8>（不参与 empty_passes，保持纯地形 v1 判定语义）。
- OffscreenRenderer 增 egui_pass: Option<EguiPass>（创建即 Some；new/new_windowed 都经 build_renderer 构造，离屏 ppp=1.0、窗口取 window.scale_factor()）。
- render_frame 在 debug pass 之后：let ui_events = take_ui_events();（任何情况都 take 丢弃）→ ui_segment 非空时 decode_ui_frame 解包，frame.visible && !has_font 返回 Invalid（编程错误），否则 egui_pass.run_and_record(...).is_err() 返回 Invalid。
- resize 转发 egui_pass.set_size(width, height, ppp)（ppp 窗口取 scale_factor、离屏 1.0）。
- 新增 pub fn upload_ui_font(&mut self, &[u8]) -> Result<(), EguiError> 与 pub fn drain_ui_events(&mut self) -> Vec<u32>。
- 顶层模块文档补 egui pass 段（最上层、无 depth、无 UI 段零 GPU 工作）。

### engine/crates/mornlea_client/src/ffi.rs

- CLIENT_ABI_VERSION: u32 = 8（注释写明 v8 增量 = 两出口 + 帧 TLV tag 9；v7=雾 setter、v6=远环 tile）。
- FRAME_TAG_UI: u32 = 9；parse_frame 的 seen: [false; 10]、白名单 1..=9，FRAME_TAG_UI => ui_segment = payload.to_vec() 并在 parse_frame 内做 decode_ui_frame 语义校验（Err 则 None → INVALID_ARGUMENT，先于渲染器状态、不触碰帧 target）。
- 新出口 mornlea_client_render_upload_ui_font(abi, handle, bytes, len)：ABI → 参数（bytes 非空、len>0 则 INVALID_ARGUMENT；>32MiB 则 CAPACITY）→ with_renderer → renderer.upload_ui_font。
- 新出口 mornlea_client_render_drain_ui_events(abi, handle, out, out_len)：ABI → 参数（out 非空、out_len.is_multiple_of(4)）→ with_renderer → renderer.drain_ui_events() → write_ui_events 写 u32 批并返回写入个数（超出容量写满截断）。
- 全部新出口包 catch_unwind；校验失败不触碰调用方缓冲。
- 单测：abi_version_is_eight；TLV tag 9 解析（合法段经 render_frame 回调 → 句柄未知 WINDOW 证明接受；非法 UI 段 → INVALID_ARGUMENT 同路径）；无 tag → 空段合法；upload_ui_font 参数校验（null/0 长度/超限）；drain_ui_events 参数校验（null out/非 4 倍数）；write_ui_events 纯函数截断（写满、不足、空）。

### engine/include/mornlea_client.h

- MORNLEA_CLIENT_ABI_VERSION 8u + 顶部注释补 v8 段落；声明 mornlea_client_render_upload_ui_font 与 mornlea_client_render_drain_ui_events。

### engine/crates/mornlea_client/src/lib.rs

- 顶层模块文档 当前 v7 改为 当前 v8，并补 v8 增量一句。

### 范围外但必需的两处最小改动（同 crate、已记录）

- src/ui.rs：pub fn ctx(&self) -> &egui::Context —— design.md 要求 tessellate 把 FullOutput::shapes 转三角网格，需要持有字体图集的 egui::Context；UiState 私有持有该上下文且 context::tessellate 需要 &Context，故只经此只读访问器暴露。这是 run_and_record 能 tessellate 的必要条件。
- src/render/water_tests.rs：water_is_the_only_added_render_pass 的 render pass 调用点总数守卫 5 -> 6 —— egui pass 是新增的合法 render pass（src/render 下 begin_render_pass 调用点+1），守卫必须随之修订；mod.rs 的 label 断言（terrain/water/screen tint）不受影响（egui pass 在 egui.rs 内）。

## egui-wgpu 0.35 API 实测用法（以 cargo 依赖源码为准）

- Renderer::new(device, output_color_format, options: RendererOptions) -> Self —— 0.35 构造签名是结构体 RendererOptions，不再是旧版元组 (depth, msaa, dithering)。RendererOptions 有 Default。
- Renderer::update_texture(&mut self, device, queue, id: TextureId, image_delta: &ImageDelta)；Renderer::free_texture(&mut self, id: &TextureId) —— 纹理表内部管理，本模块不自行创建 wgpu 纹理。TextureId/ImageDelta 来自 egui::TexturesDelta 迭代（类型推断，无需显式命名）。
- Renderer::update_buffers(&mut self, device, queue, encoder, jobs: &[ClippedPrimitive], screen: &ScreenDescriptor) -> Vec<CommandBuffer> —— 返回回调命令缓冲（常规为空），需随主编码器一并提交（本实现 queue.submit(callback_buffers)）。
- Renderer::render(&self, render_pass: &mut RenderPass<static>, jobs: &[ClippedPrimitive], screen: &ScreenDescriptor) —— 参数是 RenderPass<static>，故对 begin_render_pass 产物调用 pass.forget_lifetime() 擦除生命周期。
- ScreenDescriptor { size_in_pixels: [u32;2], pixels_per_point: f32 }（size_in_pixels 为物理像素）。
- tessellate：0.35 的 FullOutput 无 into_parts()（仅有 append）；用 egui::Context::tessellate(&self, shapes: Vec<ClippedShape>, pixels_per_point: f32) -> Vec<ClippedPrimitive>。因此必须拿到 UiState 的 &egui::Context（见 ui.rs ctx()）。

## 与 design.md 的偏差（以 brief 为准实现，指出冲突点）

1. run_and_record 签名：brief 为 (device, queue, encoder, frame_view, frame, events) -> Result<bool, EguiError>；design.md 为 (…, pixels_per_point, size) -> Option<()>。采用 brief 版本——brief 的 EguiPass 把 ppp/尺寸存进 self.screen（set_size(w,h,ppp)），run_and_record 直接用 self.screen 读出 ppp 并换算 screen_rect，语义与 design 等价；Result<bool, EguiError> 比 Option<()> 更能区分「零工作」与「错误」。
2. set_size 签名：brief set_size(w, h, ppp)；design set_size(w, h)。采用 brief（ppp 需要一处落点，resize 里由窗口 scale_factor/1.0 提供）。
3. new 签名：brief 无显式说明，但 EguiPass 持有带 ppp 的 ScreenDescriptor；实现为 new(device, color_format, w, h, ppp)，ppp 初始值来自构造时窗口 scale_factor（窗口）或 1.0（离屏）。
4. raw_input 的 ppp/mods：brief 说 raw_input(..., mods)，但 Task 1/2 交付的 crate::ui::raw_input(events, screen_rect, pixels_per_point, time) 实际参数为 time: Option<f64>（modifiers 在函数内从事件推导）。采用实际 API，传 None（菜单无动画，golden 确定性）。
5. upload_font 错误类型：brief Result<(), EguiError>；design Result<(), ()>。采用 brief（FFI 层需要区分 FontInvalid 与正常）。
6. client ABI 校验先于句柄查找：brief/design 列出顺序为「ABI → 句柄 → 参数」，但既有 set_lod_fog 先例与无头测试要求「参数校验先于句柄查找」（非法参数配未知句柄仍应报 INVALID_ARGUMENT）。采用既有先例：参数校验在 with_renderer 之前，与 set_lod_fog_validates_arguments_before_handle_lookup 一致。
7. 范围外最小改动：ui.rs 增 ctx()、water_tests.rs 守卫 5->6（见上文）。这两是让 design 的 tessellate 与既有 render-pass 守卫成立的必要改动，未触碰 Go/engine crate/其他文件。

## 测试与命令输出

- cargo test --workspace --locked：全绿（mornlea_engine 160 passed、mornlea_client 95 passed，0 failed；含 abi_version_is_eight、TLV tag 9 解析/非法段拒绝、drain_ui_events 参数校验、upload_ui_font 参数校验、drain_write_truncates_to_capacity）。
- cargo clippy --workspace --all-targets -- -D warnings：通过（修复了 collapsible_if、manual_is_multiple_of 两处 lint）。
- cargo fmt --check：干净（cargo fmt 自动格式化后校验通过）。
- git diff --check：无空白错误。

## 提交 hash 与 git status

- 提交：f3b93252 feat: egui wgpu pass and client ABI v8 exports（7 文件、+587/-20、新建 render/egui.rs）。
- git status：M openspec/changes/add-egui-main-menu/ledger.md（非本次改动，未提交）与 ?? docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md（既有未跟踪文件，未提交）。本次提交未包含这两个文件。

## SPEC / QUALITY 自评

SPEC：与 brief/delta spec 一致——EguiPass 字段与 run_and_record 五步流程、FrameInput.ui_segment + empty_passes 不含 UI、render_frame 末尾集成（无 UI 段也 take 丢弃、字体未装可见返回 Invalid、复用既有 FrameResult 变体）、parse_frame 白名单 1..=9 + tag 9 语义校验、ABI v8 + 两出口 + 头文件/注释同步。

QUALITY：注释全部中文、导出项有中文 doc comment；无新增 crate/依赖（egui 0.35 沿用）；不引入 egui-winit；GPU 路径只编译、不进自动测试（本任务无窗口/GPU 驱动的单测；main-menu capture 属 Task 6）；纹理表不归 EguiPass 管（update/free）、不自行建 wgpu 纹理；dithering=false 固定 golden；无 UI 段帧零 GPU 提交（既有场景逐字节不变）。

## 遗留担忧

1. TestBaselineVersionsMatchCode 会临时失败：header 已升 v8，但 AGENTS.md/CLAUDE.md「项目定位」段的 client ABI v7 尚未同步（本任务范围纪律「不碰其他文件」禁止改这两份；且二者须逐字节一致）。该 Go 门禁会在基线文档同步到 v8 前变红，预期由后续收尾/文档任务补齐。
2. run_and_record 与 design 签名差异采用 brief 版本（自洽）；若评审坚持以 design 的显式 ppp/size 参数，需改 render_frame 传参并调整 EguiPass 状态读取。
3. 字体缺失=编程错误的 Invalid 触发点偏晚：在 debug pass 之后返回 Invalid 会丢弃已录制的命令（不提交），虽不触碰 GPU 状态，但相比早期校验多执行了 CPU 端记录；Go 启动即上传字体可避免此路径。
4. drain 截断的 GPU 路径（真实渲染器 + 事件队列 → 写满截断）仅由纯函数 write_ui_events 单测覆盖，端到端依赖 Task 6 capture 场景验证；FFI 层无 GPU 路径只测参数校验（符合 brief 允许）。
5. water_is_the_only_added_render_pass 守卫消息仍写着「新增额外半透明阶段」的措辞，本次 5->6 实为 egui pass（最上层 screen-space）；守卫仍捕捉增量，措辞稍欠精确。

## brief 返回消息契约

- 状态：完成
- commit hash：f3b93252
- 测试摘要：cargo test --workspace --locked 全绿（engine 160 + client 95）、cargo clippy --workspace --all-targets -- -D warnings 通过、cargo fmt --check 干净。
- 担忧：见上「遗留担忧」1（archcheck 基线文档 client ABI v7 需后续同步为 v8）、2（run_and_record 采用 brief 签名与 design 有差异）、3（字体缺失 Invalid 触发点偏晚）、4（drain 端到端待 Task 6）、5（守卫消息措辞）。
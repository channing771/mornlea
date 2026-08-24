# Tasks

以 `subagent-driven-development` 执行：每个 Task 派发独立 implementer 子代理（brief 见对应小节，是唯一需求来源），任务评审做规格合规（SPEC）与代码质量（QUALITY）双裁决，修复循环单任务最多 5 轮，全部任务后整分支终审；执行进度、评审结论与裁决记录在 `ledger.md`。先读 `proposal.md`、两个 delta spec 与 `design.md` 再动手；失败测试先写，再最小实现。

## 1. Rust：egui 依赖与无 GPU 的 UI 模型（对应 UiFrame/RawInput/菜单绘制）

- [ ] 1.1 重新读取 `proposal.md`、`specs/egui-tool-ui/spec.md`、`specs/visual-verification/spec.md` 与 `design.md`；用 `git rev-parse HEAD` 记录起始 commit 到 `ledger.md`；确认 `cargo build --workspace` 在 `engine/` 下行得通（先跑 `make rust` 或对应 target）。
- [ ] 1.2 在 `engine/crates/mornlea_client/Cargo.toml` 的 `[target.'cfg(target_os = "macos")'.dependencies]` 追加 `egui = { version = "0.35", default-features = false }` 与 `egui-wgpu = { version = "0.35", default-features = false, features = ["macos-window-resize-jitter-fix", "wgpu/default"] }`；运行 `cargo tree -p egui-wgpu`（或 `cargo tree | grep wgpu`）确认 `wgpu` 解析为 29.x、无 `egui-winit` 进入依赖树；更新 `Cargo.lock`（提交进仓）。
- [ ] 1.3 新建 `engine/crates/mornlea_client/src/ui.rs`，实现 `design.md` 规定的 `UiFrame`/`UiButton`/`decode_ui_frame`、`UiState`（`install_font`/`run_frame`/`drain_events`/`has_font`，菜单绘制：不透明深灰全屏背景、标题「Mornlea」、中心纵排按钮列（进入游戏/多人游戏/设置/退出游戏，按 `UiFrame` 传入 id 与 enabled）、底部版本行、错误行）、`UiEvent` 与 `raw_input()`；在 `lib.rs` 注册模块（`#[cfg(target_os = "macos")] pub mod ui;`）。
- [ ] 1.4 在 `src/ui.rs` 内写无 GPU 单测（先失败再实现）：decode 边界（空段、截断、按钮数 >8、label/title/version/error 越界、非 UTF-8、layout 版本非 1）；按钮命中（1280×720 逻辑尺寸，在按钮矩形中心派发 pointer press+release，断言 `drain_events` 返回对应 id 且仅一次）；禁用按钮点击不产生事件；两个按钮同点只命中一个；`raw_input` 翻译（CursorMoved/鼠标键/文本/修饰键 → `egui::Event`）；安装字体后 `has_font()`；相同 RawInput 两次 `run_frame` 的 FullOutput 文本/shape 摘要一致（无动画）。
- [ ] 1.5 运行 `cargo test --workspace --locked`、`cargo clippy --workspace --all-targets -- -D warnings`、`cargo fmt --check` 全绿；把命令与关键计数记入 `ledger.md`。
- [ ] 1.6 implementer 自证通过后提交候选 `git commit -m "feat: add egui deps and headless menu ui model"`；记录 `BASE`（候选的 parent）与 `HEAD`，生成含 `git diff --stat` 与 `git diff -U10 $BASE..$HEAD` 的 scratch review package（放入 `.superpowers/sdd/add-egui-main-menu/`），交独立 task reviewer 同给 SPEC 与 QUALITY 裁决（reviewer 不得评审未提交工作区 diff）。
- [ ] 1.7 任一 finding 只以追加 fix commit 修复、不得 amend；每轮重跑 1.5、更新 review package 并由同一 scoped reviewer 复审，最多 5 轮；SPEC+QUALITY 双 PASS 后把结论回填 `ledger.md`。

## 2. Rust：winit 事件 → UiEvent 桥与 RawInput 组装（输入半部）

- [ ] 2.1 在 `src/window.rs` 的 `App` 事件回调中把 winit 事件翻译为 `crate::ui::UiEvent`（CursorMoved → CursorMoved、MouseInput → MouseButton、MouseWheel → Scroll、KeyboardInput/Ime → Key/Text、窗口丢失 → CursorGone），push 进 `ui::UI_EVENTS` 线程局部队列；`#[cfg(test)]` 里用纯函数 `winit_to_ui_events`（不建真实窗口，winit 事件类型可构造）单测翻译；同时补 `ui.rs` 的 `raw_input` 已按 `design.md` 组装（screen_rect 用 framebuffer/pixels_per_point 推导、viewports=ROOT、modifiers 从事件累积、time=None）。
- [ ] 2.2 处理边界：窗口句柄跨线程不在本任务范围（既有 thread-local 契约不变）；`poll` 每次泵完事件后队列保留，由渲染侧 `take`；队列容量上限 1024（超出丢最旧并置 overflow 标志到快照 reserved 位——不改变既有快照布局，overflow 只供调试；若改变既定布局则回退为丢新事件）。
- [ ] 2.3 单测（无窗口）：给定 WindowEvent 序列 → 期望 UiEvent 序列（含 Ime::Commit 与 KeyboardInput 文本去控制字符）；MODIFIERS 推导（Shift/Alt/Control 位漂移）；同 poll 两事件顺序保持；容量溢出行为。
- [ ] 2.4 运行 `cargo test --workspace --locked`、`cargo clippy --workspace --all-targets -- -D warnings`、`cargo fmt --check`；记录到 `ledger.md`。
- [ ] 2.5 提交 `git commit -m "feat: bridge winit events into egui raw input"`；记录 BASE/HEAD、生成 review package、独立 task reviewer 双裁决；findings 只以追加 fix commit 修复并 scoped 复审，最多 5 轮；结论回填 `ledger.md`。

## 3. Rust：egui wgpu pass、帧集成与 client ABI v8

- [ ] 3.1 新建 `src/render/egui.rs`：`EguiPass`（`egui_wgpu::Renderer`、纹理表、`ScreenDescriptor`、`upload_font`、`run_and_record`，pass 标签 `"egui pass"`，load=Load/store=Store、不写 depth）；在 `render/mod.rs` 的 `OffscreenRenderer::new`/`new_windowed` 创建，`FrameInput.ui_segment` 字段、`resize` 转发，`render_frame` 在 debug pass 之后按 `design.md` 集成（无 UI 段或字体未装 → `UI_EVENTS.take()` 丢弃后直接跳过）。
- [ ] 3.2 `ffi.rs`：`CLIENT_ABI_VERSION = 8`（注释写明 v8 增量）；新增 `mornlea_client_render_upload_ui_font`（参数校验、≤32 MiB、CAPACITY 超限）与 `mornlea_client_render_drain_ui_events`（out null / out_len%4≠0 → INVALID_ARGUMENT；写满截断）；`parse_frame` 增加 TLV tag 9（`FRAME_TAG_UI = 9`）→ `FrameInput.ui_segment`；`engine/include/mornlea_client.h` 同步（v8 版本号 + 两出口声明 + 注释）；`lib.rs` 顶层文档补 v8。
- [ ] 3.3 单测：`abi_version_is_eight`；TLV tag 9 解析进 `ui_segment`（normal/缺 tag/版本非法）；`decode_ui_frame` 已在 Task 1 覆盖，这里补 FFI 接收端（非法 UI 段 → `INVALID_ARGUMENT` 且不触碰帧 target——沿用既有拒绝路径断言）；drain 出口参数校验；font 上传出口校验（空指针/超限）。
- [ ] 3.4 运行 `cargo test --workspace --locked`、`cargo clippy --workspace --all-targets -- -D warnings`、`cargo fmt --check`；记录到 `ledger.md`。
- [ ] 3.5 提交 `git commit -m "feat: egui wgpu pass and client ABI v8 exports"`；BASE/HEAD、review package、独立 task reviewer 双裁决；fix 循环最多 5 轮，结论回填 `ledger.md`。

## 4. Go：client ABI 绑定与菜单帧编码

- [ ] 4.1 `internal/client/render.go`：`frameTagUI = 9`；`RenderFrame.UISegment []byte`；`UIButton`/`UIMenu`/`EncodeUIMenu`（与 `design.md` 字节布局逐字段一致：u32 layout=1、u32 flags(bit0 visible)、u32 按钮数 ≤8、每按钮 u32 id+u32 label_len+UTF-8 label ≤64B、title ≤128B、version ≤64B、error ≤256B；越界 panic）；`hasPassSegments` 计入；`EncodeRenderFrame` 在 water 之后追加 tag 9；`Renderer.UploadUIFont(font []byte)` 与 `Renderer.DrainUIEvents() []uint32`（排空语义）；`render.go`/`window.go` 的 cgo 序言补 `noescape/nocallback` 声明。
- [ ] 4.2 测试（先失败再实现）：`EncodeUIMenu` 字节级 golden（最小菜单/四按钮+错误行/最大规模边界）；越界 panic（按钮 9 个、任一字串超字节界）；`hasPassSegments` 计入 UISegment；`EncodeRenderFrame` 含 tag 9 的字节序列；在 `internal/client` 用固定字节断言与 Rust `decode_ui_frame` 的夹具一致（跨语言锁定：把 Task 1 的 golden hex 复制到 Go 测试注释与断言中，两条测试同值）。
- [ ] 4.3 `internal/render/font_atlas.go`：新增导出 `EmbeddedCJKFont() []byte`（`go:embed` 字节原样返回，不改既有 glyph 路径）；测试：返回长度与 sha256 等于 provenance 记录（2c76254f...）。
- [ ] 4.4 运行 `go test ./internal/client ./internal/render -race -count=1`、`gofmt -l internal/client internal/render`、`go test ./internal/archcheck -count=1`；记录到 `ledger.md`。
- [ ] 4.5 提交 `git commit -m "feat: bind client ABI v8 ui font and event drain in Go"`；BASE/HEAD、review package、独立 task reviewer 双裁决；fix 循环最多 5 轮，结论回填 `ledger.md`。

## 5. Go：主菜单状态机与延迟世界装配

- [ ] 5.1 `cmd/mornlea`：`applicationOptions.StartAtMenu bool`；`main.go` 在交互本地条件下置 true；`newApplicationWithDependencies` 在 `StartAtMenu` 时跳过 store/Host/登录/LOD 装配（其余不变），把 options 与 deps 快照存到 `application`；`menuState`（phase/starting/title/version/error）与 `menuPhase`/按钮 id 常量；版本行经 `runtime/debug.ReadBuildInfo` 计算（空 → `"dev"`）。
- [ ] 5.2 `(a *application) startWorld() error`：把既有装配链路（`openApplicationStore` + `assembleLocalApplicationConnection` + `attachLodScheduler`）原语义迁移；成功置 `menu.phase=game` 并捕获光标；失败返回 error 且菜单保持（`error` 文本显示）。
- [ ] 5.3 `runInteractive` 菜单相位：不捕获光标、不处理游戏按键/移动/聊天/面板；每帧 `Poll` → `renderer.DrainUIEvents` → 分派（start：`starting` 防重入 → `startWorld`；quit 返回；其余忽略）→ `renderFrame`（菜单态生成 `client.UIMenu`，错误行填入）；装配成功切游戏循环（既有循环体）。
- [ ] 5.4 测试（先失败再实现；复用 `app_test_helpers_test.go` 的依赖注入）：`StartAtMenu` 启动后 world 未装配（server 为空、store 未打开）且菜单帧含四按钮与版本行；点击 start（DrainUIEvents 返回 id=1）装配成功 → 相位 game、光标捕获；装配失败（注入错误的 newHost/新 openStore）→ 菜单保持 + 错误行 + quit 仍可用；starting 期间重复 start 只装配一次；`-connect`/benchmark/capture 构造不受 StartAtMenu 影响（既有测试保持全绿）。
- [ ] 5.5 运行 `go test ./cmd/mornlea -race -count=1`、`go test ./internal/archcheck -count=1`、`gofmt -l cmd/mornlea`；记录到 `ledger.md`。
- [ ] 5.6 提交 `git commit -m "feat: defer world boot behind main menu"`；BASE/HEAD、review package、独立 task reviewer 双裁决；fix 循环最多 5 轮，结论回填 `ledger.md`。

## 6. capture 主菜单场景与既有 golden 零影响验证

- [ ] 6.1 `captureScene` 增 `Menu *captureMenuFixture`；`captureSceneImage` 在 Prepare 前设置 `app.menuOverride = scene.Menu`（nil 清除）；`renderFrame` 在菜单态（override 或交互相位）生成 UI 段；新场景 `main-menu`（标题「Mornlea」、四按钮、版本 "dev"、错误空）插在 `far-horizon` 之前，Apply 调 `resetCapturePresentation` 并把相机钉在出生点上空。
- [ ] 6.2 更新场景表断言：`capture_ai_companion_test.go` 的完整场景顺序列表加入 `main-menu`（位于 `far-horizon` 之前）；`far-horizon` 倒数第二、`water-underwater` 最后两断言不变；新增 `TestMainMenuCaptureScenePosition`。
- [ ] 6.3 先跑无更新 capture（应只见 main-menu 缺少 golden 而失败、其余 16 场景与现有 golden 全绿 —— 证明既有场景逐字节不变）；再 `-capture -update` 生成新 golden；`git status` 确认只有 `main-menu.png` 为新增、无既有 golden 变动。
- [ ] 6.4 运行 `go test ./cmd/mornlea -race -count=1`（含 capture 测试；无 `-short`）、`go test ./internal/archcheck -count=1`、`gofmt -l cmd/mornlea`、`git diff --check`；记录数值到 `ledger.md`。
- [ ] 6.5 提交 `git commit -m "feat: add main-menu capture scene and golden"`；BASE/HEAD、review package、独立 task reviewer 双裁决；fix 循环最多 5 轮，结论回填 `ledger.md`。

## 7. 收尾：全量门禁与变更产物核对

- [ ] 7.0 同步 `AGENTS.md` 与 `CLAUDE.md`（两份必须逐字节相同）的项目定位中 `client ABI v7` → `client ABI v8`——`internal/archcheck` 的 `TestBaselineVersionsMatchCode` 是机械门禁，client ABI 升版后不改基线文档即红；能力描述（egui 工具型 UI 已交付）按选型文档留到归档时同步，本任务只改版本号。
- [ ] 7.1 `make rust`（或 `cargo build --workspace`）+ `cargo test --workspace --locked` + `cargo clippy --workspace --all-targets -- -D warnings`。
- [ ] 7.2 `gofmt -l .` 无输出；`go vet ./...` 通过；`go test ./... -race` 全绿（无 -short）。
- [ ] 7.3 运行 `openspec validate --all --strict --no-interactive`；核对 tasks 全部勾选、spec/design 与代码一致（不一致先改 change 产物）。
- [ ] 7.4 benchmark 场景记录一次（`go test ./cmd/mornlea -run Benchmark -count=1` 或等价），确认 scenario v19 断言通过；性能数值只记录不改退出状态。
- [ ] 7.5 把 Task 1–6 的 ledger 汇总核对，提交最终的 change 产物与 `ledger.md`（bookkeeping commit）。

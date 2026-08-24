# Task final-fix report — add-egui-main-menu

执行者：implementer 子代理（终审 fix wave）。
分支：`add-egui-main-menu`；本 wave 追加 commit：`fa4ddaf2 refactor: final review wave optional cleanups`（parent `131a8ce1`，单一追加、不 amend）。
变更前基线为任务描述中的 `f84bd672`（执行期间控制会话在分支上又落了一个 docs commit `131a8ce1`，本 wave 依序追加在其上）。

## 逐项改动与选择理由

### 1. `_window` 改名（终审 T3-可选）— engine/crates/mornlea_client/src/render/mod.rs
`TargetMode::Windowed` 的 `_window: std::sync::Arc<winit::window::Window>` 改为 `window`。字段在 `resize` 中被读取（`window.scale_factor()`），因此不再需要下划线前缀；同步三处：结构体定义、构造处（`new_windowed`→`build_renderer`，`_window: window` → `window`）、`resize` 解构（`TargetMode::Windowed { window, .. } => window.scale_factor()`）。
- 理由：字段已被读取，保持下划线只会掩盖真实用途；改名让字段名与语义一致，无行为变更。`render_frame` 处的 `{ surface, .. }` 解构不绑定该字段，无需改动；`build_renderer` 匹配处（`Some((surface, window))`）名称未变。

### 2. EguiPass font 冗余（T3-可选）— engine/crates/mornlea_client/src/render/egui.rs
删除 `EguiPass.font: Option<Vec<u8>>` 字段（及其 doc 注释）；`has_font()` 从 `self.font.is_some() || self.ui.has_font()` 改为只问 `self.ui.has_font()`；`upload_font()` 移除 `self.font = Some(bytes.to_vec())` 这一份多余拷贝，只调用 `self.ui.install_font(bytes)`。同步把 `EguiPass` 结构体 doc 注释里「已上传字体」与 `upload_font` doc 注释「并记住已安装」改为准确表述（已安装状态由 `UiState::install_font`/其 `font_loaded` 记录）。
- 理由：`font` 字段只服务于 `has_font`，与 `UiState.font_loaded` 是双真相源；改后单一真相源在 `UiState`，并删掉 `upload_font` 的额外 `to_vec()` 拷贝。`upload_font` 成功后 `ui.has_font()` 恒为真，行为逐字节不变。

### 3. 142B 夹具常量（T4-可选）— engine/crates/mornlea_client/src/ffi.rs
`ui_segment_four_button_error`（140 行夹具，段长 142 字节）里硬编码的 `&1u32.to_le_bytes()`（layout）与 `&1u32.to_le_bytes()`（flags visible）分别改用 `&crate::ui::UI_LAYOUT_VERSION.to_le_bytes()` 与 `&crate::ui::UI_FLAG_VISIBLE.to_le_bytes()`，与姊妹 helper（同文件 `ui_segment_...` 处已用此命名常量）保持一致。
- 理由：纯命名常量替换，字节值不变（均为 `1u32`），杜绝「魔法常量与真实 ABI 值漂移」。

### 4. Menu nil-clear 聚焦单测（T6-a 可选）— cmd/mornlea/app_menu_test.go
新增 `TestUISegmentMenuOverrideNilClear`：用零值 `application{}` 最小构造（`menu.phase` 零值为 `menuPhaseGame`）；先设 `app.menuOverride = &client.UIMenu{...}`（真实菜单内容，复用 `menuButtons()`）断言 `app.uiSegment()` 非空；再设 `app.menuOverride = nil` 断言 `app.uiSegment()` 为 nil。
- 理由：直接锁定 `menuOverride` 的 nil 清除语义（先前仅由 golden 逐字节不变间接证明）。零值 application 走 `menu.phase != menuPhaseGame` 为假 → 返回 nil，无需其它依赖。

### 5. capture.go 注释例外标注（T6-b 可选）— cmd/mornlea/capture.go
在场景表顶部「新增场景应追加在列表末尾…」注释末尾追加一句例外说明：`main-menu` 因 spec 排序约束（MUST 排在 `far-horizon` 之前）被插入表中部（water-surface-slope 与 far-horizon 之间），属 spec/brief 硬性例外。
- 理由：通用约定与 spec 硬约束冲突时不应静默，读者看到长注释中的「追加在末尾」会误以为 `main-menu` 违反了该约定；加一句例外让文档自洽。

### 6. 标题 gating（终审新增 minor）— engine/crates/mornlea_client/src/ui.rs
`draw_menu` 的标题绘制不再被 `rects.first()`（按钮数 ≥1）gating：改为无条件绘制，`title_y = rects.first().map_or(screen.center().y - MENU_TITLE_EMPTY_CENTER_GAP, |first| first.min.y - MENU_TITLE_BUTTON_GAP)`。新增常量 `MENU_TITLE_EMPTY_CENTER_GAP: f32 = 120.0`（无按钮时标题底边距屏幕中心的向上偏移，落在屏幕上半部）。
- **错误行选择**：保持既有「≥1 按钮」语义（仍 `rects.last()` 锚定）——错误文本语义上依附按钮列，且现有既有场景（含 capture `main-menu` 的 4 按钮 + 错误行）golden 依赖该行为，改成无条件需另立固定锚点、引入多余改动；标题则无条件（标题是菜单本身的标识，0 按钮帧也须可见）。此选择在报告说明。
- **无 GPU 单测**：新增 `menu_title_drawn_with_zero_buttons`：构造 0 按钮帧（`encode_frame(UI_LAYOUT_VERSION, UI_FLAG_VISIBLE, &[], "Mornlea", "dev", "")`），装字体后 `run_frame`，断言 shapes 中含标题「Mornlea」的 `egui::Shape::Text`（`t.galley.job.text.contains("Mornlea")`）。有按钮时 `title_y` 与改动前完全一致（首按钮上方 `- MENU_TITLE_BUTTON_GAP`），故既有 `main-menu` capture golden 像素不变。

## 验证（实跑记录）

- `cd engine && cargo test --workspace --locked`：**exit 0**。mornlea_client lib 98 通过、0 失败；另一 160 通过、0 失败。（cargo 不在默认 PATH，补 `PATH=$HOME/.cargo/bin:$PATH` 后执行。）
  - 新增单测单独复核：`cargo test -p mornlea_client menu_title_drawn_with_zero_buttons` → `test ui::tests::menu_title_drawn_with_zero_buttons ... ok`（1 通过）。
- `cd engine && cargo clippy --workspace --all-targets -- -D warnings`：**exit 0**。
- `cd engine && cargo fmt --check`：**exit 0**（最初对 `map_or` 块报一次格式 diff，已按 rustfmt 建议修正后通过）。
- `go test ./cmd/mornlea -race -count=1`：**ok（235.084s）**。
- `go test ./cmd/mornlea -race -count=1 -run 'Menu|Capture|UISegment'`：**ok（5.342s）**。
- `go test ./internal/client -race -count=1`：**ok（8.689s）**（未改动 internal/client，作为安全兜底实跑）。
- `gofmt -l cmd/mornlea`：**无输出**。
- `git diff --check`：**无输出**。
- 注：Go 测试需要可写 build cache，本次用 `GOCACHE=$(pwd)/.review-gocache`（沙箱限定 workspace 写）跑通后已删除，未入仓。

## 提交

- 单一追加 commit：`fa4ddaf2 refactor: final review wave optional cleanups`
- 文件：`cmd/mornlea/app_menu_test.go`、`cmd/mornlea/capture.go`、`engine/crates/mornlea_client/src/ffi.rs`、`engine/crates/mornlea_client/src/render/egui.rs`、`engine/crates/mornlea_client/src/render/mod.rs`、`engine/crates/mornlea_client/src/ui.rs`（共 6 文件，75+/25-）。
- 未提交：缓存目录（无）；未跟踪文件 `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md`（保持未跟踪）。`.review-gocache` 已清理。

## 新 HEAD

`fa4ddaf2`

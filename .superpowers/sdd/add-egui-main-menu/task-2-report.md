# Task 2 报告 — Rust：winit 事件 → UiEvent 桥与输入队列（输入半部）

**子代理 implementer 交付报告**。来源：`.superpowers/sdd/add-egui-main-menu/task-2-brief.md`（唯一需求来源）；冲突裁决以 design.md + Task 1 已实现代码（commit `64ec093f`）为准。

## 一、实现清单

只在 `engine/crates/mornlea_client/src/window.rs` 与 `src/ui.rs` 内改动，其余文件零触碰。

### `src/ui.rs`（新增） — 输入队列 + 窗口事件翻译

1. **进程内输入队列**（模块级 thread_local）：
   - `pub const UI_EVENT_QUEUE_CAPACITY: usize = 1024`：容量上限。
   - `thread_local! { static UI_EVENTS: RefCell<VecDeque<UiEvent>> = const { RefCell::new(VecDeque::new()) }; }`，注释写明「窗口事件桥与渲染器同线程（Go 主线程 `LockOSThread`），队列是进程内桥，无跨线程暴露」。
   - `pub fn push_ui_event(UiEvent)`：尾部追加；超过 1024 时从头部丢弃**最旧**；
   - `pub fn take_ui_events() -> Vec<UiEvent>`：排空返回（读前清空）；
   - `pub fn clear_ui_events()`：丢弃全部。
   - 队列用 `VecDeque` 而非 design.md 写的 `RefCell<Vec<UiEvent>>`：理由在注释中说明——满时丢最旧、尾部追加都是 O(1)，若用 `Vec` 则满时删首元素是 O(n)。

2. **`pub fn key_from(code: KeyCode) -> Option<egui::Key>`**：winit 物理键码 → egui 常用键。覆盖 Escape/Enter/Backspace/ArrowUp/Down/Left/Right、空格、字母 `KeyA`..`KeyZ`、数字 `Digit0`..`Digit9`、`ShiftLeft`/`ControlLeft`/`AltLeft`；其余返回 `None`。

3. **`fn key_event_to_ui_events(...)`**（私有辅助）：单键盘事件 → `Vec<UiEvent>`。亮点：修饰键状态**先更新再发射**（顺序敏感）；文本在非 IME 激活且 pressed 时发射、控制字符过滤。

4. **`pub fn winit_to_ui_events(events: &[WindowEvent], scale: f64, ime_active: bool, modifiers: &mut egui::Modifiers) -> Vec<UiEvent>`**：
   - `CursorMoved` → `CursorMoved`（`position.to_logical::<f64>(scale)` 逻辑坐标）；
   - `CursorLeft` → `CursorGone`；
   - `MouseInput` → `MouseButton(primary, pressed)`（Left/Right；其余忽略）；
   - `MouseWheel` → `Scroll`：`LineDelta(x,y)` → `(x*60.0, y*60.0)`（egui 惯例，注释说明 0.35 `MouseWheelUnit::Point` 按点、一行 60 点），`PixelDelta(p)` → `(p.x as f32, p.y as f32)`；
   - `KeyboardInput` → `Key`（经 `key_from`，未知键 None 跳过）＋ 文本（非 IME 激活时过滤控制字符）；
   - `Ime(Ime::Commit(text))` → 每个去控制字符的字符一个 `Text`；
   - `_`（含 Enabled/Disabled/Preedit、Resized、CloseRequested 等）→ 无事件。
   - 不判断菜单可见性（渲染侧 take；菜单不可见时事件丢弃是设计）。

### `src/window.rs`（改动） — App 事件回调接入桥

1. `App` 新增字段 `ui_modifiers: egui::Modifiers`（当前修饰键状态，进程内持续累积，与 `InputState` 游戏按键状态独立），`App::new` 初始化为 `default()`。
2. `window_event` 开头调用 `ui::winit_to_ui_events(std::slice::from_ref(&event), scale, self.ime_active, &mut self.ui_modifiers)` 并把返回事件逐个 `ui::push_ui_event`；`scale = window.scale_factor()`（窗口为 None 时回退 1.0）。
3. 既有游戏输入路径（`self.input.*`）**逐字节保持不变**；`Ime::Enabled/Disabled` 仍更新 `self.ime_active`，网格事件桥读取同一 `ime_active` 快照。

## 二、与 design.md / Task 1 接口的偏差

| # | 偏差 | 说明 |
|---|------|------|
| 1 | **`repeat` 无法下传** | brief 要求「repeat 标志由 `egui::Event::Key.repeat` 按 winit `repeat` 填」。但 design.md 与 Task 1（HEAD）的 `UiEvent::Key { key, pressed, modifiers }` **没有 `repeat` 字段**，`raw_input` 固定 `repeat: false`。按 brief「与本 brief 冲突时以代码+design.md 为准」裁决：不改 `UiEvent` 形状、不改 `raw_input`；repeat 键盘事件仍发射 `Key{pressed:true}`（对 egui 是一次新按下，与菜单无文本/无动画的诉求不冲突），但标志本身不传播。已在 `key_event_to_ui_events` 注释与单测中说明。 |
| 2 | **队列内部用 `VecDeque`** | design.md 写 `RefCell<Vec<UiEvent>>`。brief 明确「用 VecDeque 更合适」。public API（push/take/clear）与 design.md 一致，仅存储容器不同（含注释说明）。 |
| 3 | **容量上限未同步到快照 reserved 位** | tasks.md §2.2 提到「超出丢最旧并置 overflow 标志到快照 reserved 位……若改变既定布局则回退为丢新事件」。brief（唯一需求来源）只要求「满时丢最旧 + 1025 保留近端 1024 单测」，**未要求** overflow 标志；且改快照布局需触碰 `input.rs`（超出本任务仅改 window.rs/ui.rs 的范围纪律）。故只实现丢最旧语义，不写 overflow 标志。 |
| 4 | **`key_from` 字母覆盖 `KeyA`..`KeyZ`** | brief 写「字母（KeyW..KeyZ）」；我映射全部 26 个字母（`KeyA`..`KeyZ` → `A`..`Z`），以满足「常用键齐全」并避免遗漏。 |
| 5 | **`winit_to_ui_events` 增加 `ime_active` 参数** | brief 示例签名为 `(events, scale, modifiers) -> Vec<UiEvent>` 或「等价」；为实现「非 IME 激活才发射键盘文本」，需以 `ime_active` 快照为输入。返回类型保持 `Vec<UiEvent>` 不变。 |
| 6 | **KeyboardInput 单测经辅助函数** | winit 0.30 的 `KeyEvent` 含 `pub(crate) platform_specific` 字段，**外部 crate 无法构造** `KeyEvent`（`DeviceId::dummy()` 可用，但 `KeyEvent` 无公开构造器）。故把键盘翻译拆成 `key_event_to_ui_events` 私有纯函数直接单测；`winit_to_ui_events` 测试覆盖可构造的 CursorMoved/CursorLeft/MouseInput/MouseWheel/Ime。 |

## 三、测试与命令输出摘要

新增单测（均通过，与实现同文件 `#[cfg(test)]`）：

- `winit_to_ui_events_cursor_uses_logical_coords_and_left`：scale=2 下物理 (200,100) → 逻辑 (100,50)；CursorLeft → CursorGone。
- `winit_to_ui_events_mouse_input_both_buttons`：左/右键映射、中键忽略。
- `winit_to_ui_events_mouse_wheel_line_and_pixel`：Line 60 点换算、Pixel 原样 f32。
- `winit_to_ui_events_ime_commit_emits_text_per_char`：`Ime::Commit` 中文/字母逐字符为 Text，`\n` 过滤。
- `key_events_accumulate_modifiers_in_order`：Shift 按下→事件携带 shift=true；按住按 A→Key 带 shift、Text('A')；Shift 释放→状态翻回；Control/Alt 左变体各自翻转。验证「先更新再发射」顺序。
- `key_events_filter_control_chars`：`\n`、`\t` 过滤。
- `key_events_ime_active_suppresses_text`：IME 激活时无文本。
- `key_events_unknown_code_produces_no_event`：F1 无事件、不影响修饰状态。
- `key_from_maps_common_keys_and_unknown_none`：常用键齐全、字母区间、未知键（F1/Home/ShiftRight/ControlRight/AltRight）为 None。
- `ui_queue_push_take_drains_in_order`：push/take 排空、二次 take 空。
- `ui_queue_capacity_keeps_latest_drops_oldest`：1000 条全量；1025 条保留最近 1024、最旧丢失（首=事件1、末=事件1024）。
- `ui_queue_clear_empties`：clear 后 take 空。
- 另 9 个 Task 1 既有测试（decode/raw_input/menu 命中/字体）保持通过——证明未回归。

命令输出（迭代期间修过 2 个 clippy 告警：`missing_const_for_thread_local`、`collapsible_match`，均已修复）：

- `cargo build -p mornlea_client --locked` → Finished dev（OK）。
- `cargo test --workspace --locked` → **90 passed / 0 failed**（mornlea_client）+ **160 passed / 0 failed**（mornlea_engine）+ doc-tests 0。
- `cargo clippy --workspace --all-targets -- -D warnings` → Finished，**0 告警**。
- `cargo fmt --check` → FMT_OK（`cargo fmt` 已应用）。

执行环境：`PATH` 前插 `$HOME/.rustup/toolchains/1.97.1-aarch64-apple-darwin/bin`，`CARGO_HOME=/tmp/mornlea_cargo_home`（该 cargo home 已缓存 egui-0.35.0/winit-0.30.13 等依赖；默认 `~/.cargo` 无此类依赖）。

## 四、提交 hash

- **提交**：`1f2c38c2` — `feat: bridge winit events into egui raw input`
- 仅含 `engine/crates/mornlea_client/src/ui.rs`（+527）与 `src/window.rs`（+22），共 549 插入、0 删除。
- BASE = `64ec093f`（Task 1 的 HEAD）。未 amend；仅追加一个 commit。

## 五、git status 确认

`git status --short`（提交后）：

```
 M openspec/changes/add-egui-main-menu/ledger.md
?? docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md
```

- `ledger.md` 的改动**在任务开始前就已存在**（控制会话的 ledger，非本任务范围），保持未提交、未修改。
- `docs/superpowers/specs/2026-08-23-egui-tool-ui-selection-design.md` 为既有未跟踪文件，**保持未跟踪、未提交、未修改**。
- 本任务未触碰 GPU/Go/其他 crate；无既有未跟踪文件被提交。

## 六、SPEC+QUALITY 自评

**SPEC 合规**：brief 需求 1（队列 + 1024 丢最旧 + 单测）、需求 2（九类窗口事件→UiEvent 翻译、修饰键先更新再发射、`key_from`、不可见性不判断）、需求 3（单测覆盖：序列翻译、队列排空/容量/clear、key_from 映射）全部满足。与 design.md（UiEvent 枚举形状、raw_input、线程局部队列、翻译纯函数化、无 GPU 单测）一致——除前述 6 项已记录的偏差外，均按 design.md/代码口径落实。

**QUALITY**：注释全中文、导出项（`UI_EVENT_QUEUE_CAPACITY`/`push_ui_event`/`take_ui_events`/`clear_ui_events`/`key_from`/`winit_to_ui_events`）均有中文 doc comment；关键决策（drop-oldest 用 VecDeque、scale 换算、egui 60 点惯例、IME 抑制、顺序敏感、repeat 受限）都有中文注释解释权衡；测试与实现同文件（`ui.rs` 的 `#[cfg(test)] mod tests`）；无真实窗口/GPU 测试；`cargo test/clippy/fmt` 全绿。未引入 `egui-winit`；wgpu 保持 29.x；egui 保持 `0.35` 且 `default-features=false`。

## 七、遗留担忧

1. **repeat 不传播**（偏差 #1）：按住方向键/回车在 egui 中会表现为连续「新按下」，而非 `repeat:true`。菜单导航无文本输入、无动画，实际影响几乎为零；但若未来菜单需要「按住滚动/重复激活」语义，需要在 `UiEvent::Key` 增加 `repeat` 字段并同步 `raw_input`（会改 design.md 枚举，特此记录）。
2. **overflow 标志未入快照**（偏差 #3）：丢最旧是「静默」的，调试期无法从快照感知队列曾溢出。如需诊断可后续在 `input.rs` 快照 reserved 位加标志（超出本任务范围）。
3. **`MouseInput` 中键/非主次键忽略**：与「菜单只关心主/次键」一致；若未来菜单需要中键/滚轮点击语义需扩展翻译。
4. **`App` 每事件都计算 `scale`**：即使事件不产生 UI 事件也调用 `winit_to_ui_events`（返回空）。开销为每事件一次小函数调用与一次 `scale_factor()`，在事件频率下可忽略；如需可加快速路径短路。
5. **契约版本号未推进**：本任务不改 client ABI（仍 v7，Task 3 才到 v8）、不改协议/benchmark scenario，符合「只做输入半部」。

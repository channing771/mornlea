//! 菜单 UI 模型:ABI 帧解码、egui 布局/绘制、RawInput 组装与事件队列。
//!
//! 本模块是纯状态逻辑,不创建真实窗口或 GPU 资源,可无头单测。设计目标:
//! 客户端菜单的所有**呈现**(按钮几何、标题/版本/错误行的确定性布局)与
//! **输入翻译**([`UiEvent`] -> egui 事件)都归 Rust,而菜单**语义**(相位、
//! 按钮 id/禁用、文本)留在 Go,经 client ABI v8 的 UI 段与事件出口双向传递。
//!
//! 无 GPU 时 egui 的 [`egui::Context`] 仍可跑纯 CPU 布局(字体经
//! [`UiState::install_font`] 上传,不依赖 default_fonts),因此本模块的所有
//! 行为都能在不建窗口的单测中验证。egui 0.35 的 `run_ui` 从 raw input 的
//! ROOT 视口 [`ViewportInfo::native_pixels_per_point`] 读取缩放,故
//! [`UiState::run_frame`] 负责把像素密度写进 viewport。

use std::cell::RefCell;
use std::collections::VecDeque;
use std::sync::Arc;

use egui::{
    Align2, Color32, CornerRadius, Direction, FontData, FontDefinitions, FontFamily, FontId, Key,
    Layout, RawInput, Rect, RichText, UiBuilder, ViewportId, pos2, vec2,
};
use winit::event::{ElementState, Ime, MouseButton, MouseScrollDelta, WindowEvent};
use winit::keyboard::{KeyCode, PhysicalKey};

// ---------------------------------------------------------------------------
// ABI 布局常量(与 Go `EncodeUIMenu` 逐字节对应,小端;任何改动必须同时改 Go)。
// ---------------------------------------------------------------------------

/// UI 段布局版本。
pub const UI_LAYOUT_VERSION: u32 = 1;
/// flags 中表示「菜单可见」的位(bit0)。
pub const UI_FLAG_VISIBLE: u32 = 1;
/// 一帧菜单允许的最大按钮数。
pub const MAX_UI_BUTTONS: usize = 8;
/// UI 段总字节数上界(帧输入段 ≤ 4096 字节/帧)。
pub const MAX_UI_SEGMENT_BYTES: usize = 4096;
/// 单个按钮 label 的字节上界。
pub const MAX_UI_LABEL_BYTES: usize = 64;
/// 标题字节上界。
pub const MAX_UI_TITLE_BYTES: usize = 128;
/// 版本行字节上界。
pub const MAX_UI_VERSION_BYTES: usize = 64;
/// 错误行字节上界。
pub const MAX_UI_ERROR_BYTES: usize = 256;

// ---------------------------------------------------------------------------
// 菜单布局常量(全部为逻辑点,绘制函数里确定性计算,不依赖字体度量)。
// ---------------------------------------------------------------------------

/// 按钮列宽(逻辑点)。
pub const MENU_BUTTON_WIDTH: f32 = 220.0;
/// 按钮高度(逻辑点)。
pub const MENU_BUTTON_HEIGHT: f32 = 40.0;
/// 按钮纵向间距(逻辑点)。
pub const MENU_BUTTON_SPACING: f32 = 8.0;
/// 标题「Mornlea」字号。
pub const MENU_TITLE_FONT_SIZE: f32 = 32.0;
/// 按钮文本字号。
pub const MENU_BUTTON_FONT_SIZE: f32 = 18.0;
/// 底部版本行字号。
pub const MENU_VERSION_FONT_SIZE: f32 = 12.0;
/// 错误行字号。
pub const MENU_ERROR_FONT_SIZE: f32 = 14.0;
/// 版本行距左下角的边距。
pub const MENU_VERSION_MARGIN: f32 = 12.0;
/// 标题底边与首按钮顶边的间距。
pub const MENU_TITLE_BUTTON_GAP: f32 = 24.0;
/// 末按钮底边与错误行顶边的间距。
pub const MENU_ERROR_BUTTON_GAP: f32 = 16.0;
/// 全屏不透明深灰背景色(参考经典标题画面基调)。
pub const MENU_BACKGROUND: Color32 = Color32::from_rgb(32, 36, 42);
/// 标题/版本行的白色。
pub const MENU_TEXT_COLOR: Color32 = Color32::WHITE;
/// 错误行红色。
pub const MENU_ERROR_COLOR: Color32 = Color32::from_rgb(240, 90, 90);

// ---------------------------------------------------------------------------
// 菜单帧数据模型与 ABI 解码。
// ---------------------------------------------------------------------------

/// 单个菜单按钮的 Rust 表示。
///
/// `enabled` 在 ABI v1 线格式中逐按钮编码(见 [`decode_ui_frame`]):每按钮在
/// `label` 之后带一个 u32 `enabled`(0=禁用,1=启用,其余值解码为 `Err`),
/// 因此禁用态可以经 wire 从 Go 传到 Rust,`decode_ui_frame` 会据实填入。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct UiButton {
    /// 按钮唯一 id,点击后经事件队列回传 Go。
    pub id: u32,
    /// 按钮文本。
    pub label: String,
    /// 是否可点击;禁用按钮不产生点击事件。
    pub enabled: bool,
}

/// 一帧完整菜单的 Rust 表示,由 [`decode_ui_frame`] 从 ABI 段解码。
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct UiFrame {
    /// 菜单是否可见;不可见时 [`UiState::run_frame`] 返回 `None`。
    pub visible: bool,
    /// 大标题文本。
    pub title: String,
    /// 底部版本行文本。
    pub version: String,
    /// 装配错误行文本(空串表示无错误)。
    pub error: String,
    /// 中心纵排的按钮列表。
    pub buttons: Vec<UiButton>,
}

/// 越界/截断/非 UTF-8/layout 版本错误统一映射为 `Err(())`,FFI 层再转
/// `INVALID_ARGUMENT`;调用方只需关心「解不出来」,不必区分具体坏点。
///
/// 错误类型固定为 `()` 是 ABI 契约(FFI 层只区分「可解/不可解」),
/// 故显式允许 `clippy::result_unit_err` 风格告警。
#[allow(clippy::result_unit_err)]
pub fn decode_ui_frame(bytes: &[u8]) -> Result<UiFrame, ()> {
    if bytes.len() > MAX_UI_SEGMENT_BYTES {
        return Err(());
    }
    let mut reader = Reader::new(bytes);
    let layout = reader.u32()?;
    if layout != UI_LAYOUT_VERSION {
        return Err(());
    }
    let flags = reader.u32()?;
    let visible = flags & UI_FLAG_VISIBLE != 0;
    let button_count = reader.u32()? as usize;
    if button_count > MAX_UI_BUTTONS {
        return Err(());
    }
    let mut buttons = Vec::with_capacity(button_count);
    for _ in 0..button_count {
        let id = reader.u32()?;
        let label = reader.string_field(MAX_UI_LABEL_BYTES)?;
        // ABI v1 逐按钮携带 enabled u32:只接受 0(禁用)/1(启用),其余视为非法。
        let enabled = reader.u32()?;
        if enabled > 1 {
            return Err(());
        }
        buttons.push(UiButton {
            id,
            label,
            enabled: enabled == 1,
        });
    }
    let title = reader.string_field(MAX_UI_TITLE_BYTES)?;
    let version = reader.string_field(MAX_UI_VERSION_BYTES)?;
    let error = reader.string_field(MAX_UI_ERROR_BYTES)?;
    Ok(UiFrame {
        visible,
        title,
        version,
        error,
        buttons,
    })
}

/// 无符号小端游标读取器;任何读越界都能安全返回 `Err(())` 而不 panic。
struct Reader<'a> {
    bytes: &'a [u8],
    pos: usize,
}

impl<'a> Reader<'a> {
    fn new(bytes: &'a [u8]) -> Self {
        Self { bytes, pos: 0 }
    }

    /// 读一个 u32(小端);越界返回 `Err(())`。
    #[allow(clippy::result_unit_err)]
    fn u32(&mut self) -> Result<u32, ()> {
        if self.pos + 4 > self.bytes.len() {
            return Err(());
        }
        let val = u32::from_le_bytes(
            self.bytes[self.pos..self.pos + 4]
                .try_into()
                .map_err(|_| ())?,
        );
        self.pos += 4;
        Ok(val)
    }

    /// 读一个「u32 长度 + UTF-8 字节」字符串字段;长度不过上界、字节在界内且合法。
    #[allow(clippy::result_unit_err)]
    fn string_field(&mut self, max_bytes: usize) -> Result<String, ()> {
        let len = self.u32()? as usize;
        if len > max_bytes || self.pos + len > self.bytes.len() {
            return Err(());
        }
        let s = std::str::from_utf8(&self.bytes[self.pos..self.pos + len])
            .map_err(|_| ())?
            .to_owned();
        self.pos += len;
        Ok(s)
    }
}

// ---------------------------------------------------------------------------
// egui 上下文状态:字体、绘制、事件队列。
// ---------------------------------------------------------------------------

/// 持有 [`egui::Context`] 与菜单点击事件队列的纯状态容器。
///
/// 不建窗口、不碰 GPU;`run_frame` 全 CPU 布局,`install_font` 上传字体,
/// `drain_events` 把点击 id 交给 Go。字体字节经 ABI 从 Go 一次性上传,
/// Rust 侧不内嵌字体二进制。
pub struct UiState {
    ctx: egui::Context,
    /// 点击回传 Go 的按钮 id 队列。
    pending_events: Vec<u32>,
    /// 字体是否已成功安装(空字节安装失败保持 false)。
    font_loaded: bool,
}

impl Default for UiState {
    fn default() -> Self {
        Self::new()
    }
}

impl UiState {
    /// 新建空状态:无字体、空事件队列。
    pub fn new() -> Self {
        Self {
            ctx: egui::Context::default(),
            pending_events: Vec::new(),
            font_loaded: false,
        }
    }

    /// 安装字体,proportional 与 monospace 两个族都使用上传的字节。
    ///
    /// 空字节返回 `false` 且不改变状态;非空则替换现有字体并置
    /// `font_loaded = true`。重复安装同一字体是幂等的(egui 按字体内容比较)。
    pub fn install_font(&mut self, bytes: &[u8]) -> bool {
        if bytes.is_empty() {
            return false;
        }
        let mut fonts = FontDefinitions::default();
        let name = "mornlea-ui".to_owned();
        let data = FontData::from_owned(bytes.to_vec());
        fonts.font_data.insert(name.clone(), Arc::new(data));
        for family in [FontFamily::Proportional, FontFamily::Monospace] {
            if let Some(list) = fonts.families.get_mut(&family) {
                list.push(name.clone());
            }
        }
        self.ctx.set_fonts(fonts);
        self.font_loaded = true;
        true
    }

    /// 是否已安装字体。
    pub fn has_font(&self) -> bool {
        self.font_loaded
    }

    /// 运行一帧菜单:无字体或菜单不可见时返回 `None`(零工作)。
    ///
    /// `pixels_per_point` 被写进 ROOT 视口的 `native_pixels_per_point`,
    /// 这是 egui 0.35 的缩放来源(不再作为 `run_ui` 的独立参数)。
    pub fn run_frame(
        &mut self,
        mut raw: RawInput,
        frame: &UiFrame,
        pixels_per_point: f32,
    ) -> Option<egui::FullOutput> {
        if !self.font_loaded || !frame.visible {
            return None;
        }
        if let Some(info) = raw.viewports.get_mut(&ViewportId::ROOT) {
            info.native_pixels_per_point = Some(pixels_per_point);
        }
        // 字段级分离借用:ctx 只读,pending_events 可变,二者互不冲突。
        let pending = &mut self.pending_events;
        let frame_ref = frame;
        let output = self.ctx.run_ui(raw, |ui| draw_menu(ui, frame_ref, pending));
        Some(output)
    }

    /// 排空并返回累积的点击事件 id(读前清空)。
    pub fn drain_events(&mut self) -> Vec<u32> {
        std::mem::take(&mut self.pending_events)
    }

    /// 暴露内部 egui 上下文,供 GPU 半部做 tessellation。
    ///
    /// egui 的 `Context::tessellate` 需要持有字体纹理图集的上下文,
    /// 才能把 `FullOutput::shapes` 转成 GPU 可画的三角网格;该上下文
    /// 由 [`UiState`] 私有持有,故只经此只读访问器对外暴露(不改写任何状态)。
    pub fn ctx(&self) -> &egui::Context {
        &self.ctx
    }
}

// ---------------------------------------------------------------------------
// 菜单绘制(确定性、无动画)。
// ---------------------------------------------------------------------------

/// 计算 `n` 个按钮在 `screen` 内的纵向居中矩形列;几何只依赖屏幕与按钮数,
/// 不依赖任何字体度量,因此既是唯一布局来源,也是测试里命中几何的权威。
fn menu_button_layout(screen: Rect, n: usize) -> Vec<Rect> {
    if n == 0 {
        return Vec::new();
    }
    let nf = n as f32;
    let total_h = nf * MENU_BUTTON_HEIGHT + (nf - 1.0) * MENU_BUTTON_SPACING;
    let x = screen.center().x - MENU_BUTTON_WIDTH / 2.0;
    let top_y = screen.center().y - total_h / 2.0;
    (0..n)
        .map(|i| {
            let y = top_y + i as f32 * (MENU_BUTTON_HEIGHT + MENU_BUTTON_SPACING);
            Rect::from_min_size(pos2(x, y), vec2(MENU_BUTTON_WIDTH, MENU_BUTTON_HEIGHT))
        })
        .collect()
}

/// 在给定 `&mut Ui`(根层,`max_rect` = 全屏)上绘制一帧菜单。
///
/// 全部布局几何由 [`menu_button_layout`] 确定性计算;文本用 `Painter::text`
/// 以固定锚点定位(不参与流式布局,因此不受字体度量影响),按钮用
/// `scope_builder + add_enabled` 落在固定矩形并响应点击。已点击且
/// `enabled` 的按钮 id 压入 `pending`。
fn draw_menu(ui: &mut egui::Ui, frame: &UiFrame, pending: &mut Vec<u32>) {
    let screen = ui.max_rect();

    // 全屏不透明深灰背景。
    ui.painter()
        .rect_filled(screen, CornerRadius::ZERO, MENU_BACKGROUND);

    let rects = menu_button_layout(screen, frame.buttons.len());

    // 标题:位于按钮列上方、水平居中。
    if let Some(first) = rects.first() {
        ui.painter().text(
            pos2(screen.center().x, first.min.y - MENU_TITLE_BUTTON_GAP),
            Align2::CENTER_BOTTOM,
            &frame.title,
            FontId::proportional(MENU_TITLE_FONT_SIZE),
            MENU_TEXT_COLOR,
        );
    }

    // 中心纵排按钮列:固定宽高,`add_enabled` 实现禁用态。
    for (button, rect) in frame.buttons.iter().zip(rects.iter()) {
        let child = UiBuilder::new()
            .max_rect(*rect)
            .layout(Layout::centered_and_justified(Direction::TopDown));
        let response = ui
            .scope_builder(child, |ui| {
                let label = RichText::new(&button.label).size(MENU_BUTTON_FONT_SIZE);
                ui.add_enabled(button.enabled, egui::Button::new(label))
            })
            .inner;
        if button.enabled && response.clicked() {
            pending.push(button.id);
        }
    }

    // 版本行:左下固定边距。
    ui.painter().text(
        pos2(MENU_VERSION_MARGIN, screen.max.y - MENU_VERSION_MARGIN),
        Align2::LEFT_BOTTOM,
        &frame.version,
        FontId::proportional(MENU_VERSION_FONT_SIZE),
        MENU_TEXT_COLOR,
    );

    // 错误行:按钮列下方、水平居中、红色;空串不绘制。
    if !frame.error.is_empty()
        && let Some(last) = rects.last()
    {
        ui.painter().text(
            pos2(screen.center().x, last.max.y + MENU_ERROR_BUTTON_GAP),
            Align2::CENTER_TOP,
            &frame.error,
            FontId::proportional(MENU_ERROR_FONT_SIZE),
            MENU_ERROR_COLOR,
        );
    }
}

// ---------------------------------------------------------------------------
// 输入翻译:UiEvent -> egui 事件流。
// ---------------------------------------------------------------------------

/// 供渲染器与窗口事件桥共享的菜单输入事件,是无头可测的翻译输入。
///
/// 与 winit 事件一一对应(见 Task 2 的 `winit_to_ui_events`);`Modifiers`
/// 由事件自身携带,`RawInput.modifiers` 取最近一次携带修饰键的事件值。
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum UiEvent {
    /// 指针移动到绝对逻辑坐标。
    CursorMoved(f64, f64),
    /// 指针离开窗口。
    CursorGone,
    /// 鼠标键位:`primary` 为 true 表示主键,`pressed` 为 true 表示按下。
    MouseButton(bool, bool),
    /// 键盘键位,`modifiers` 携带事件发生时的修饰键状态。
    Key {
        key: Key,
        pressed: bool,
        modifiers: egui::Modifiers,
    },
    /// 单字符文本输入。
    Text(char),
    /// 滚轮位移(逻辑点)。
    Scroll(f32, f32),
}

/// 把 [`UiEvent`] 序列组装成 [`egui::RawInput`]。
///
/// * `screen_rect` 为可供 egui 使用的逻辑点矩形;
/// * `pixels_per_point` 写入 ROOT 视口,是 egui 0.35 的缩放来源;
/// * `time` 由调用方传入,菜单绘制固定传 `None` 保证 golden 确定性;
/// * `modifiers` 由事件推导(取最近一次携带修饰键的事件),`focused` 恒真;
/// * 指针按下事件的 `pos` 取最近一次 `CursorMoved` 位置,无则用屏幕中心。
pub fn raw_input(
    events: &[UiEvent],
    screen_rect: Rect,
    pixels_per_point: f32,
    time: Option<f64>,
) -> RawInput {
    let mut modifiers = egui::Modifiers::default();
    let mut last_cursor = screen_rect.center();
    let mut out = Vec::with_capacity(events.len());
    for event in events {
        match event {
            UiEvent::CursorMoved(x, y) => {
                last_cursor = pos2(*x as f32, *y as f32);
                out.push(egui::Event::PointerMoved(last_cursor));
            }
            UiEvent::CursorGone => out.push(egui::Event::PointerGone),
            UiEvent::MouseButton(primary, pressed) => {
                let button = if *primary {
                    egui::PointerButton::Primary
                } else {
                    egui::PointerButton::Secondary
                };
                out.push(egui::Event::PointerButton {
                    pos: last_cursor,
                    button,
                    pressed: *pressed,
                    modifiers,
                });
            }
            UiEvent::Key {
                key,
                pressed,
                modifiers: m,
            } => {
                modifiers = *m;
                out.push(egui::Event::Key {
                    key: *key,
                    physical_key: None,
                    pressed: *pressed,
                    repeat: false,
                    modifiers: *m,
                });
            }
            UiEvent::Text(ch) => out.push(egui::Event::Text(ch.to_string())),
            UiEvent::Scroll(dx, dy) => {
                out.push(egui::Event::MouseWheel {
                    unit: egui::MouseWheelUnit::Point,
                    delta: vec2(*dx, *dy),
                    phase: egui::TouchPhase::Move,
                    modifiers,
                });
            }
        }
    }
    // 用结构体更新构造 RawInput,避免 clippy::field_reassign_with_default;
    // ROOT 视口信息在构造后单独写(经方法访问,不触发该 lint)。
    let mut raw = RawInput {
        screen_rect: Some(screen_rect),
        focused: true,
        time,
        modifiers,
        events: out,
        ..Default::default()
    };
    if let Some(info) = raw.viewports.get_mut(&ViewportId::ROOT) {
        info.native_pixels_per_point = Some(pixels_per_point);
        info.inner_rect = Some(screen_rect);
        info.focused = Some(true);
    }
    raw
}

// ---------------------------------------------------------------------------
// 输入队列:窗口事件桥与渲染器之间的进程内桥。
// ---------------------------------------------------------------------------

/// UI 事件队列容量上限:满时丢弃**最旧**事件,保留最近的事件。
///
/// 菜单属于即时模式 UI,只关心最新输入状态,积压的旧事件没有价值;把
/// 上限做硬约束(而非无限增长)是为了防止菜单关闭期间(无 UI 段、渲染侧
/// 仍然 take 丢弃)输入疯狂堆积造成无界内存占用。
pub const UI_EVENT_QUEUE_CAPACITY: usize = 1024;

thread_local! {
    /// 菜单输入队列。
    ///
    /// 窗口事件桥与渲染器同线程(Go 主线程 `LockOSThread`),队列只是进程内
    /// 桥,无跨线程暴露;渲染器在运行 egui 前经 [`take_ui_events`] 取走。用
    /// [`VecDeque`] 是因为满时从头部丢弃最旧、尾部追加最新都是 O(1),若用
    /// `Vec` 则在满时移除首元素是 O(n)。
    static UI_EVENTS: RefCell<VecDeque<UiEvent>> = const { RefCell::new(VecDeque::new()) };
}

/// 把一个窗口事件翻译结果入队。
///
/// 超过 [`UI_EVENT_QUEUE_CAPACITY`] 时丢弃**最旧**事件(保持最近 1024 条),
/// 由 [`VecDeque`] 的 `push_back`/`pop_front` 以 O(1) 完成。
pub fn push_ui_event(event: UiEvent) {
    UI_EVENTS.with(|q| {
        let mut q = q.borrow_mut();
        q.push_back(event);
        while q.len() > UI_EVENT_QUEUE_CAPACITY {
            q.pop_front();
        }
    });
}

/// 排空并返回队列里的全部事件(读前清空)。
///
/// 渲染器每帧在运行 egui 前调用一次;没有 UI 段的帧同样取走并丢弃,
/// 防止菜单关闭期间的输入积压。
pub fn take_ui_events() -> Vec<UiEvent> {
    UI_EVENTS.with(|q| q.borrow_mut().drain(..).collect())
}

/// 清空队列,丢弃全部积压事件。
pub fn clear_ui_events() {
    UI_EVENTS.with(|q| q.borrow_mut().clear());
}

// ---------------------------------------------------------------------------
// 窗口事件翻译:winit 事件 -> UiEvent。
// ---------------------------------------------------------------------------

/// 把 winit 物理键码映射为 egui 常用键。
///
/// 只映射菜单真正关心的键:导航/回车/退格、字母(`KeyA`..`KeyZ`)、数字
/// (`Digit0`..`Digit9`)、空格与 `Shift`/`Control`/`Alt` 的左变体;其余返回
/// `None`,调用方静默跳过(egui 侧不识别这些键,不产生事件,也不误报按键)。
pub fn key_from(code: KeyCode) -> Option<Key> {
    let key = match code {
        KeyCode::Escape => Key::Escape,
        KeyCode::Enter => Key::Enter,
        KeyCode::Backspace => Key::Backspace,
        KeyCode::ArrowUp => Key::ArrowUp,
        KeyCode::ArrowDown => Key::ArrowDown,
        KeyCode::ArrowLeft => Key::ArrowLeft,
        KeyCode::ArrowRight => Key::ArrowRight,
        KeyCode::Space => Key::Space,
        KeyCode::ShiftLeft => Key::ShiftLeft,
        KeyCode::ControlLeft => Key::ControlLeft,
        KeyCode::AltLeft => Key::AltLeft,
        KeyCode::KeyA => Key::A,
        KeyCode::KeyB => Key::B,
        KeyCode::KeyC => Key::C,
        KeyCode::KeyD => Key::D,
        KeyCode::KeyE => Key::E,
        KeyCode::KeyF => Key::F,
        KeyCode::KeyG => Key::G,
        KeyCode::KeyH => Key::H,
        KeyCode::KeyI => Key::I,
        KeyCode::KeyJ => Key::J,
        KeyCode::KeyK => Key::K,
        KeyCode::KeyL => Key::L,
        KeyCode::KeyM => Key::M,
        KeyCode::KeyN => Key::N,
        KeyCode::KeyO => Key::O,
        KeyCode::KeyP => Key::P,
        KeyCode::KeyQ => Key::Q,
        KeyCode::KeyR => Key::R,
        KeyCode::KeyS => Key::S,
        KeyCode::KeyT => Key::T,
        KeyCode::KeyU => Key::U,
        KeyCode::KeyV => Key::V,
        KeyCode::KeyW => Key::W,
        KeyCode::KeyX => Key::X,
        KeyCode::KeyY => Key::Y,
        KeyCode::KeyZ => Key::Z,
        KeyCode::Digit0 => Key::Num0,
        KeyCode::Digit1 => Key::Num1,
        KeyCode::Digit2 => Key::Num2,
        KeyCode::Digit3 => Key::Num3,
        KeyCode::Digit4 => Key::Num4,
        KeyCode::Digit5 => Key::Num5,
        KeyCode::Digit6 => Key::Num6,
        KeyCode::Digit7 => Key::Num7,
        KeyCode::Digit8 => Key::Num8,
        KeyCode::Digit9 => Key::Num9,
        _ => return None,
    };
    Some(key)
}

/// 把单个键盘事件翻译为 [`UiEvent`] 序列(纯输入,可无窗口单测)。
///
/// winit 的 `KeyEvent` 含 `pub(crate)` 字段,无法从 crate 外构造,因此键盘
/// 翻译被拆成这个只依赖裸字段的辅助函数;`winit_to_ui_events` 从中取字段
/// 调用。修饰键状态在此**先更新再发射**(顺序敏感),使事件携带的
/// `modifiers` 反映该次按下/释放之后的累计状态。`repeat` 事件仍发射 `Key`
/// (`pressed=true`);`UiEvent::Key` 依 design.md 不带 `repeat` 字段,故
/// `repeat` 标志无法下传到 egui `Event::Key.repeat`(见报告)。
fn key_event_to_ui_events(
    code: KeyCode,
    pressed: bool,
    text: Option<&str>,
    ime_active: bool,
    modifiers: &mut egui::Modifiers,
) -> Vec<UiEvent> {
    // 先更新修饰键(左变体)再发射:Shift/Control/Alt 的按键动作本身也要让
    // 后续事件携带已翻转的修饰状态,保证事件顺序敏感。
    match code {
        KeyCode::ShiftLeft => modifiers.shift = pressed,
        KeyCode::ControlLeft => modifiers.ctrl = pressed,
        KeyCode::AltLeft => modifiers.alt = pressed,
        _ => {}
    }
    let mut out = Vec::new();
    if let Some(key) = key_from(code) {
        out.push(UiEvent::Key {
            key,
            pressed,
            modifiers: *modifiers,
        });
    }
    // IME 激活期间键盘文本改由 `Ime::Commit` 提供;重复事件同样压文本
    // (与既有 `push_text` 域一致);控制字符一律过滤。
    if !ime_active
        && pressed
        && let Some(text) = text
    {
        for ch in text.chars().filter(|ch| !ch.is_control()) {
            out.push(UiEvent::Text(ch));
        }
    }
    out
}

/// 把一批 winit 窗口事件翻译为 [`UiEvent`] 序列(纯函数,可无窗口单测)。
///
/// * `scale` 是窗口缩放因子,用于把物理像素坐标换算为 egui 逻辑坐标(与既有
///   `InputState` 的 `CursorMoved` 换算一致);
/// * `ime_active` 是当前 IME 组合状态:激活期间键盘 `text` 不发射(改由
///   `Ime::Commit` 提供),避免组合过程重复字符;
/// * `modifiers` 是进程内持续累积的修饰键状态,在事件处理点先更新再发射。
///
/// 本函数**不判断菜单可见性**:渲染侧每帧 `take` 队列;菜单不可见时事件被
/// 丢弃是设计(菜单只在 Go 菜单相位产生 UI 段,游戏帧此时 egui 不消费)。
pub fn winit_to_ui_events(
    events: &[WindowEvent],
    scale: f64,
    ime_active: bool,
    modifiers: &mut egui::Modifiers,
) -> Vec<UiEvent> {
    let mut out = Vec::with_capacity(events.len());
    for event in events {
        match event {
            WindowEvent::CursorMoved { position, .. } => {
                let logical = position.to_logical::<f64>(scale);
                out.push(UiEvent::CursorMoved(logical.x, logical.y));
            }
            WindowEvent::CursorLeft { .. } => out.push(UiEvent::CursorGone),
            WindowEvent::MouseInput { state, button, .. } => {
                let pressed = *state == ElementState::Pressed;
                match button {
                    MouseButton::Left => out.push(UiEvent::MouseButton(true, pressed)),
                    MouseButton::Right => out.push(UiEvent::MouseButton(false, pressed)),
                    _ => {}
                }
            }
            WindowEvent::MouseWheel { delta, .. } => {
                // egui 惯例:把「行」换算为「点」——egui 0.35 的
                // `MouseWheelUnit::Point` 以逻辑点计,一行按 60 点;触控板
                // 上报的像素增量则原样 f32 化。
                let (dx, dy) = match delta {
                    MouseScrollDelta::LineDelta(x, y) => (x * 60.0, y * 60.0),
                    MouseScrollDelta::PixelDelta(p) => (p.x as f32, p.y as f32),
                };
                out.push(UiEvent::Scroll(dx, dy));
            }
            WindowEvent::KeyboardInput { event, .. } => {
                if let PhysicalKey::Code(code) = event.physical_key {
                    out.extend(key_event_to_ui_events(
                        code,
                        event.state.is_pressed(),
                        event.text.as_deref(),
                        ime_active,
                        modifiers,
                    ));
                }
            }
            WindowEvent::Ime(Ime::Commit(text)) => {
                for ch in text.chars().filter(|ch| !ch.is_control()) {
                    out.push(UiEvent::Text(ch));
                }
            }
            _ => {}
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 测试用最小 ABI 编码器(底层):第三个元素是**原始** u32 的 `enabled`(允许传
    /// 越界值如 2,用于失败路径)。与 Go `EncodeUIMenu` 的字节语义一致:
    /// layout u32、flags u32、按钮数 u32、每按钮 [id u32 + label_len u32 + label + enabled u32],
    /// 随后 title/version/error 依次 [len u32 + bytes]。
    fn encode_frame_raw(
        layout: u32,
        flags: u32,
        buttons: &[(u32, &str, u32)],
        title: &str,
        version: &str,
        error: &str,
    ) -> Vec<u8> {
        let mut b = Vec::new();
        b.extend_from_slice(&layout.to_le_bytes());
        b.extend_from_slice(&flags.to_le_bytes());
        b.extend_from_slice(&(buttons.len() as u32).to_le_bytes());
        for (id, label, enabled) in buttons {
            b.extend_from_slice(&id.to_le_bytes());
            b.extend_from_slice(&(label.len() as u32).to_le_bytes());
            b.extend_from_slice(label.as_bytes());
            b.extend_from_slice(&enabled.to_le_bytes());
        }
        b.extend_from_slice(&(title.len() as u32).to_le_bytes());
        b.extend_from_slice(title.as_bytes());
        b.extend_from_slice(&(version.len() as u32).to_le_bytes());
        b.extend_from_slice(version.as_bytes());
        b.extend_from_slice(&(error.len() as u32).to_le_bytes());
        b.extend_from_slice(error.as_bytes());
        b
    }

    /// 便捷高层封装:`bool` 的 `enabled` → 0/1,供成功路径与常规夹具使用。
    fn encode_frame(
        layout: u32,
        flags: u32,
        buttons: &[(u32, &str, bool)],
        title: &str,
        version: &str,
        error: &str,
    ) -> Vec<u8> {
        let raw = buttons
            .iter()
            .map(|(id, label, enabled)| (*id, *label, if *enabled { 1 } else { 0 }))
            .collect::<Vec<_>>();
        encode_frame_raw(layout, flags, &raw, title, version, error)
    }

    /// 主菜单四按钮夹具(与 spec 一致:多人/设置禁用,进入/退出启用)。
    fn four_button_frame() -> Vec<u8> {
        encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[
                (1, "进入游戏", true),
                (2, "多人游戏", false),
                (3, "设置", false),
                (4, "退出游戏", true),
            ],
            "Mornlea",
            "dev",
            "",
        )
    }

    /// 同布局但全部按钮启用的夹具(用于验证 enabled=1 的 wire 点击会返回 id)。
    fn four_button_frame_all_enabled() -> Vec<u8> {
        encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[
                (1, "进入游戏", true),
                (2, "多人游戏", true),
                (3, "设置", true),
                (4, "退出游戏", true),
            ],
            "Mornlea",
            "dev",
            "",
        )
    }

    #[test]
    fn decode_rejects_layout_version_mismatch() {
        assert!(decode_ui_frame(&encode_frame(2, UI_FLAG_VISIBLE, &[], "", "", "")).is_err());
    }

    #[test]
    fn decode_rejects_truncated_bytes() {
        let bytes = four_button_frame();
        for cut in [0usize, 3, 7, 11, 12] {
            assert!(decode_ui_frame(&bytes[..cut]).is_err(), "cut={cut}");
        }
    }

    #[test]
    fn decode_rejects_too_many_buttons() {
        let many = (1..=9u32).map(|i| (i, "x", true)).collect::<Vec<_>>();
        assert!(
            decode_ui_frame(&encode_frame(
                UI_LAYOUT_VERSION,
                UI_FLAG_VISIBLE,
                &many,
                "",
                "",
                ""
            ))
            .is_err()
        );
    }

    #[test]
    fn decode_rejects_field_overflow() {
        let long_label: String = "长".repeat(33); // 每字 3 字节 => 99 字节。
        assert!(
            decode_ui_frame(&encode_frame(
                UI_LAYOUT_VERSION,
                UI_FLAG_VISIBLE,
                &[(1, &long_label, true)],
                "",
                "",
                ""
            ))
            .is_err()
        );

        let long_title = "长".repeat(50); // 150 字节。
        assert!(
            decode_ui_frame(&encode_frame(
                UI_LAYOUT_VERSION,
                UI_FLAG_VISIBLE,
                &[],
                &long_title,
                "",
                ""
            ))
            .is_err()
        );

        let long_version = "长".repeat(25); // 75 字节。
        assert!(
            decode_ui_frame(&encode_frame(
                UI_LAYOUT_VERSION,
                UI_FLAG_VISIBLE,
                &[],
                "",
                &long_version,
                ""
            ))
            .is_err()
        );

        let long_error = "长".repeat(90); // 270 字节。
        assert!(
            decode_ui_frame(&encode_frame(
                UI_LAYOUT_VERSION,
                UI_FLAG_VISIBLE,
                &[],
                "",
                "",
                &long_error
            ))
            .is_err()
        );
    }

    #[test]
    fn decode_rejects_non_utf8() {
        let mut b = encode_frame(UI_LAYOUT_VERSION, UI_FLAG_VISIBLE, &[], "", "", "");
        // 在 title 长度字段(偏移 12)后追加 1 字节非法 UTF-8。
        b[12..16].copy_from_slice(&1u32.to_le_bytes());
        b.push(0xFF);
        assert!(decode_ui_frame(&b).is_err());
    }

    #[test]
    fn decode_rejects_segment_too_large() {
        let mut b = four_button_frame();
        b.resize(MAX_UI_SEGMENT_BYTES + 1, 0);
        assert!(decode_ui_frame(&b).is_err());
    }

    #[test]
    fn decode_minimal_success_fields_exact() {
        let frame = decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[],
            "",
            "",
            "",
        ))
        .unwrap();
        assert!(frame.visible);
        assert_eq!(frame.title, "");
        assert_eq!(frame.version, "");
        assert_eq!(frame.error, "");
        assert!(frame.buttons.is_empty());
    }

    #[test]
    fn decode_maximal_success_fields_exact() {
        let labels = (0..8u32).map(|i| (i, "button", true)).collect::<Vec<_>>();
        let title = "天".repeat(40); // 120 字节,<=128。
        let version = "v".repeat(64);
        let error = "错".repeat(40); // 120 字节,<=256。
        let frame = decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &labels,
            &title,
            &version,
            &error,
        ))
        .unwrap();
        assert!(frame.visible);
        assert_eq!(frame.buttons.len(), 8);
        assert_eq!(frame.buttons[7].id, 7);
        assert_eq!(frame.buttons[7].label, "button");
        assert!(frame.buttons.iter().all(|b| b.enabled));
        assert_eq!(frame.title, title);
        assert_eq!(frame.version, version);
        assert_eq!(frame.error, error);
    }

    #[test]
    fn decode_four_button_with_enabled_fields_exact() {
        // 四按钮 + 错误行的夹具:逐字段(含 enabled)精确断言。
        let frame = decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[
                (1, "进入游戏", true),
                (2, "多人游戏", false),
                (3, "设置", false),
                (4, "退出游戏", true),
            ],
            "Mornlea",
            "dev",
            "存档无法打开",
        ))
        .unwrap();
        assert!(frame.visible);
        assert_eq!(frame.title, "Mornlea");
        assert_eq!(frame.version, "dev");
        assert_eq!(frame.error, "存档无法打开");
        assert_eq!(frame.buttons.len(), 4);
        assert_eq!(
            frame.buttons[0],
            UiButton {
                id: 1,
                label: "进入游戏".into(),
                enabled: true
            }
        );
        assert_eq!(
            frame.buttons[1],
            UiButton {
                id: 2,
                label: "多人游戏".into(),
                enabled: false
            }
        );
        assert_eq!(
            frame.buttons[2],
            UiButton {
                id: 3,
                label: "设置".into(),
                enabled: false
            }
        );
        assert_eq!(
            frame.buttons[3],
            UiButton {
                id: 4,
                label: "退出游戏".into(),
                enabled: true
            }
        );
    }

    #[test]
    fn decode_rejects_enabled_out_of_range() {
        // enabled 只接受 0/1,其余值(如 2)视为非法。
        assert!(
            decode_ui_frame(&encode_frame_raw(
                UI_LAYOUT_VERSION,
                UI_FLAG_VISIBLE,
                &[(1, "A", 2)],
                "",
                "",
                ""
            ))
            .is_err()
        );
    }

    #[test]
    fn decode_rejects_enabled_truncated() {
        // 单按钮 label "A"(1 字节)时,其 enabled u32 落在偏移 21..25;截断到
        // 中间(22)或其后(21)都读不到完整字段 => Err。
        let bytes = encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[(1, "A", true)],
            "",
            "",
            "",
        );
        assert!(decode_ui_frame(&bytes[..21]).is_err());
        assert!(decode_ui_frame(&bytes[..22]).is_err());
    }

    fn screen_rect() -> Rect {
        Rect::from_min_size(pos2(0.0, 0.0), vec2(1280.0, 720.0))
    }

    fn test_font() -> &'static [u8] {
        include_bytes!("ui/testdata/demo.ttf")
    }

    /// 在 `center` 处完成一次完整点击:移动帧、按下帧、释放帧,共三帧。
    ///
    /// egui 的指针悬浮要在指针首次移动到的**下一帧**才被认定(debug 证实
    /// 同帧 Move+Press 时 hovered=false),故需先一帧建立悬浮,随后按下帧
    /// 置 down,最后释放帧登记 click。禁用按钮在所有帧都不产生事件。
    fn click_button(state: &mut UiState, frame: &UiFrame, center: egui::Pos2) {
        let move_only = raw_input(
            &[UiEvent::CursorMoved(center.x as f64, center.y as f64)],
            screen_rect(),
            1.0,
            None,
        );
        state.run_frame(move_only, frame, 1.0);
        let press = raw_input(
            &[
                UiEvent::CursorMoved(center.x as f64, center.y as f64),
                UiEvent::MouseButton(true, true),
            ],
            screen_rect(),
            1.0,
            None,
        );
        state.run_frame(press, frame, 1.0);
        let release = raw_input(
            &[
                UiEvent::CursorMoved(center.x as f64, center.y as f64),
                UiEvent::MouseButton(true, false),
            ],
            screen_rect(),
            1.0,
            None,
        );
        state.run_frame(release, frame, 1.0);
    }

    #[test]
    fn menu_hit_click_emits_event_once() {
        let mut state = UiState::new();
        assert!(state.install_font(test_font()));
        let frame = decode_ui_frame(&four_button_frame()).unwrap();

        let rects = menu_button_layout(screen_rect(), 4);
        // 点击启用的「退出游戏」(id=4)中心,应恰好回传该 id 一次。
        let target = rects[3].center();
        click_button(&mut state, &frame, target);
        assert_eq!(state.drain_events(), vec![4]);
        assert!(state.drain_events().is_empty());
    }

    #[test]
    fn menu_disabled_button_click_no_event() {
        let mut state = UiState::new();
        state.install_font(test_font());
        let rects = menu_button_layout(screen_rect(), 4);

        // 经 wire 的禁用路径:由 enabled=0 的字节解码出「多人游戏」禁用帧,点击其中心不产生事件。
        let frame = decode_ui_frame(&four_button_frame()).unwrap();
        assert!(!frame.buttons[1].enabled);
        click_button(&mut state, &frame, rects[1].center());
        assert!(state.drain_events().is_empty());

        // 同布局但 enabled=1:点击同样位置返回该按钮 id。
        let frame2 = decode_ui_frame(&four_button_frame_all_enabled()).unwrap();
        assert!(frame2.buttons[1].enabled);
        click_button(&mut state, &frame2, rects[1].center());
        assert_eq!(state.drain_events(), vec![2]);
    }

    #[test]
    fn menu_same_point_hits_at_most_one() {
        let mut state = UiState::new();
        state.install_font(test_font());
        let frame = decode_ui_frame(&four_button_frame()).unwrap();
        let rects = menu_button_layout(screen_rect(), 4);

        // 指针落在首按钮中心:恰好只命中按钮 1,且只产生一次事件。
        let center = rects[0].center();
        click_button(&mut state, &frame, center);
        assert_eq!(state.drain_events(), vec![1]);

        // 指针落在距所有按钮都远的点(屏幕上方标题区):不命中任何按钮。
        click_button(&mut state, &frame, egui::pos2(640.0, 40.0));
        assert!(state.drain_events().is_empty());
    }

    #[test]
    fn menu_buttons_do_not_overlap() {
        let rects = menu_button_layout(screen_rect(), 4);
        assert_eq!(rects.len(), 4);
        for pair in rects.windows(2) {
            assert!(pair[1].min.y >= pair[0].max.y, "按钮不得重叠: {pair:?}");
        }
        for r in &rects {
            assert_eq!(r.width(), MENU_BUTTON_WIDTH);
            assert_eq!(r.height(), MENU_BUTTON_HEIGHT);
        }
    }

    #[test]
    fn raw_input_translates_events_and_fields() {
        let mods = egui::Modifiers {
            shift: true,
            ..Default::default()
        };
        let raw = raw_input(
            &[
                UiEvent::CursorMoved(100.0, 200.0),
                UiEvent::CursorGone,
                UiEvent::MouseButton(true, true),
                UiEvent::Key {
                    key: egui::Key::A,
                    pressed: true,
                    modifiers: mods,
                },
                UiEvent::Text('好'),
                UiEvent::Scroll(1.0, -2.0),
            ],
            screen_rect(),
            2.0,
            None,
        );
        assert_eq!(raw.screen_rect, Some(screen_rect()));
        assert!(raw.focused);
        assert_eq!(raw.time, None);
        assert_eq!(raw.viewport().native_pixels_per_point, Some(2.0));
        assert!(raw.modifiers.shift);

        assert_eq!(raw.events.len(), 6);
        assert!(matches!(raw.events[0], egui::Event::PointerMoved(p) if p == pos2(100.0, 200.0)));
        assert!(matches!(raw.events[1], egui::Event::PointerGone));
        assert!(
            matches!(raw.events[2], egui::Event::PointerButton { button: egui::PointerButton::Primary, pressed: true, pos, .. } if pos == pos2(100.0, 200.0))
        );
        assert!(matches!(
            raw.events[3],
            egui::Event::Key {
                key: egui::Key::A,
                pressed: true,
                repeat: false,
                ..
            }
        ));
        assert!(matches!(&raw.events[4], egui::Event::Text(s) if s == "好"));
        assert!(
            matches!(raw.events[5], egui::Event::MouseWheel { delta, phase: egui::TouchPhase::Move, .. } if delta == vec2(1.0, -2.0))
        );
    }

    #[test]
    fn raw_input_defaults_pointer_pos_to_screen_center() {
        let raw = raw_input(
            &[UiEvent::MouseButton(true, true)],
            screen_rect(),
            1.0,
            None,
        );
        assert!(
            matches!(raw.events[0], egui::Event::PointerButton { pos, .. } if pos == screen_rect().center())
        );
    }

    #[test]
    fn menu_run_frame_is_deterministic_and_no_texture_churn() {
        let mut state = UiState::new();
        state.install_font(test_font());
        let frame = decode_ui_frame(&four_button_frame()).unwrap();
        let raw = raw_input(&[], screen_rect(), 1.0, None);

        let first = state
            .run_frame(raw.clone(), &frame, 1.0)
            .expect("应产出布局");
        let second = state.run_frame(raw, &frame, 1.0).expect("应产出布局");
        assert_eq!(first.shapes.len(), second.shapes.len());
        assert!(
            second.textures_delta.is_empty(),
            "第二次同输入不应再上传纹理"
        );
        assert!(!first.shapes.is_empty());
    }

    #[test]
    fn menu_hidden_or_no_font_returns_none() {
        let mut state = UiState::new();
        let frame = decode_ui_frame(&four_button_frame()).unwrap();
        assert!(
            state
                .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
                .is_none()
        );
        state.install_font(test_font());
        let mut hidden = frame.clone();
        hidden.visible = false;
        assert!(
            state
                .run_frame(raw_input(&[], screen_rect(), 1.0, None), &hidden, 1.0)
                .is_none()
        );
    }

    #[test]
    fn install_font_rejects_empty_and_is_idempotent() {
        let mut state = UiState::new();
        assert!(!state.install_font(&[]));
        assert!(!state.has_font());
        assert!(state.install_font(test_font()));
        assert!(state.has_font());
        assert!(state.install_font(test_font()));
        assert!(state.has_font());
    }

    // -----------------------------------------------------------------------
    // Task 2:窗口事件桥与输入队列。
    // -----------------------------------------------------------------------

    use winit::dpi::PhysicalPosition;
    use winit::event::{DeviceId, TouchPhase};

    fn cursor_moved(x: f64, y: f64) -> WindowEvent {
        WindowEvent::CursorMoved {
            device_id: DeviceId::dummy(),
            position: PhysicalPosition::new(x, y),
        }
    }

    fn cursor_left() -> WindowEvent {
        WindowEvent::CursorLeft {
            device_id: DeviceId::dummy(),
        }
    }

    fn mouse_input(button: MouseButton, state: ElementState) -> WindowEvent {
        WindowEvent::MouseInput {
            device_id: DeviceId::dummy(),
            state,
            button,
        }
    }

    fn mouse_wheel_line(dx: f32, dy: f32) -> WindowEvent {
        WindowEvent::MouseWheel {
            device_id: DeviceId::dummy(),
            delta: MouseScrollDelta::LineDelta(dx, dy),
            phase: TouchPhase::Moved,
        }
    }

    fn mouse_wheel_pixel(dx: f64, dy: f64) -> WindowEvent {
        WindowEvent::MouseWheel {
            device_id: DeviceId::dummy(),
            delta: MouseScrollDelta::PixelDelta(PhysicalPosition::new(dx, dy)),
            phase: TouchPhase::Moved,
        }
    }

    fn ime_commit(text: &str) -> WindowEvent {
        WindowEvent::Ime(Ime::Commit(text.to_owned()))
    }

    #[test]
    fn winit_to_ui_events_cursor_uses_logical_coords_and_left() {
        let mut mods = egui::Modifiers::default();
        // scale=2:物理 (200,100) -> 逻辑 (100,50)。
        let out = winit_to_ui_events(
            &[cursor_moved(200.0, 100.0), cursor_left()],
            2.0,
            false,
            &mut mods,
        );
        assert_eq!(
            out,
            vec![UiEvent::CursorMoved(100.0, 50.0), UiEvent::CursorGone]
        );
        assert!(mods.is_none());
    }

    #[test]
    fn winit_to_ui_events_mouse_input_both_buttons() {
        let mut mods = egui::Modifiers::default();
        let out = winit_to_ui_events(
            &[
                mouse_input(MouseButton::Left, ElementState::Pressed),
                mouse_input(MouseButton::Right, ElementState::Released),
                mouse_input(MouseButton::Middle, ElementState::Pressed),
            ],
            1.0,
            false,
            &mut mods,
        );
        // 中键被忽略,只保留主/次键。
        assert_eq!(
            out,
            vec![
                UiEvent::MouseButton(true, true),
                UiEvent::MouseButton(false, false)
            ]
        );
    }

    #[test]
    fn winit_to_ui_events_mouse_wheel_line_and_pixel() {
        let mut mods = egui::Modifiers::default();
        let out = winit_to_ui_events(
            &[mouse_wheel_line(1.0, -2.0), mouse_wheel_pixel(10.0, -20.0)],
            1.0,
            false,
            &mut mods,
        );
        // Line 按 60 点换算;Pixel 原样 f32 化。
        assert_eq!(
            out,
            vec![UiEvent::Scroll(60.0, -120.0), UiEvent::Scroll(10.0, -20.0)]
        );
    }

    #[test]
    fn winit_to_ui_events_ime_commit_emits_text_per_char() {
        let mut mods = egui::Modifiers::default();
        let out = winit_to_ui_events(
            &[ime_commit("你好ab\n"), ime_commit("x")],
            1.0,
            false,
            &mut mods,
        );
        // \n 是控制字符应被过滤;每个字符一个 Text。
        assert_eq!(
            out,
            vec![
                UiEvent::Text('你'),
                UiEvent::Text('好'),
                UiEvent::Text('a'),
                UiEvent::Text('b'),
                UiEvent::Text('x'),
            ]
        );
    }

    #[test]
    fn key_events_accumulate_modifiers_in_order() {
        let mut mods = egui::Modifiers::default();
        // Shift 按下:先更新修饰状态,再发射 Key 事件(事件携带 shift=true)。
        let shift_down = key_event_to_ui_events(KeyCode::ShiftLeft, true, None, false, &mut mods);
        assert!(mods.shift);
        assert_eq!(shift_down.len(), 1);
        assert_eq!(
            shift_down[0],
            UiEvent::Key {
                key: Key::ShiftLeft,
                pressed: true,
                modifiers: mods,
            }
        );

        // Shift 按住时按下 A:Key 事件携带 shift=true,文本 'A' 也发射。
        let a = key_event_to_ui_events(KeyCode::KeyA, true, Some("A"), false, &mut mods);
        assert_eq!(a.len(), 2);
        assert_eq!(
            a[0],
            UiEvent::Key {
                key: Key::A,
                pressed: true,
                modifiers: mods,
            }
        );
        assert_eq!(a[1], UiEvent::Text('A'));

        // Shift 释放:状态翻回,Key 事件仍携带更新后的(无 shift)状态。
        let shift_up = key_event_to_ui_events(KeyCode::ShiftLeft, false, None, false, &mut mods);
        assert!(!mods.shift);
        assert_eq!(
            shift_up[0],
            UiEvent::Key {
                key: Key::ShiftLeft,
                pressed: false,
                modifiers: egui::Modifiers::default(),
            }
        );

        // Control/Alt 左变体同样翻转各自位。
        key_event_to_ui_events(KeyCode::ControlLeft, true, None, false, &mut mods);
        assert!(mods.ctrl);
        key_event_to_ui_events(KeyCode::AltLeft, true, None, false, &mut mods);
        assert!(mods.alt);
    }

    #[test]
    fn key_events_filter_control_chars() {
        let mut mods = egui::Modifiers::default();
        // 文本含换行/制表等控制字符,只保留可见字符 'a'。
        let out = key_event_to_ui_events(KeyCode::KeyA, true, Some("a\n\t"), false, &mut mods);
        assert_eq!(
            out,
            vec![
                UiEvent::Key {
                    key: Key::A,
                    pressed: true,
                    modifiers: egui::Modifiers::default(),
                },
                UiEvent::Text('a'),
            ]
        );
    }

    #[test]
    fn key_events_ime_active_suppresses_text() {
        let mut mods = egui::Modifiers::default();
        // IME 激活期间键盘文本不发射(改由 Commit 提供),只留 Key 事件。
        let out = key_event_to_ui_events(KeyCode::KeyA, true, Some("a"), true, &mut mods);
        assert_eq!(out.len(), 1);
        assert!(matches!(
            out[0],
            UiEvent::Key {
                key: Key::A,
                pressed: true,
                ..
            }
        ));
    }

    #[test]
    fn key_events_unknown_code_produces_no_event() {
        let mut mods = egui::Modifiers::default();
        // F1 不在映射表,不产生任何 Key 事件(文本亦无)。
        let out = key_event_to_ui_events(KeyCode::F1, true, None, false, &mut mods);
        assert!(out.is_empty());
        // 未知键码的修饰键推导也不影响状态。
        assert!(mods.is_none());
    }

    #[test]
    fn key_from_maps_common_keys_and_unknown_none() {
        // 常用键齐全:导航、回车/退格、空格、字幕、数字、修饰键左变体。
        assert_eq!(key_from(KeyCode::Escape), Some(Key::Escape));
        assert_eq!(key_from(KeyCode::Enter), Some(Key::Enter));
        assert_eq!(key_from(KeyCode::Backspace), Some(Key::Backspace));
        assert_eq!(key_from(KeyCode::ArrowUp), Some(Key::ArrowUp));
        assert_eq!(key_from(KeyCode::ArrowDown), Some(Key::ArrowDown));
        assert_eq!(key_from(KeyCode::ArrowLeft), Some(Key::ArrowLeft));
        assert_eq!(key_from(KeyCode::ArrowRight), Some(Key::ArrowRight));
        assert_eq!(key_from(KeyCode::Space), Some(Key::Space));
        assert_eq!(key_from(KeyCode::ShiftLeft), Some(Key::ShiftLeft));
        assert_eq!(key_from(KeyCode::ControlLeft), Some(Key::ControlLeft));
        assert_eq!(key_from(KeyCode::AltLeft), Some(Key::AltLeft));
        assert_eq!(key_from(KeyCode::KeyW), Some(Key::W));
        assert_eq!(key_from(KeyCode::KeyZ), Some(Key::Z));
        assert_eq!(key_from(KeyCode::Digit0), Some(Key::Num0));
        assert_eq!(key_from(KeyCode::Digit9), Some(Key::Num9));
        // 字母区间全覆盖(A..Z)。
        for (code, key) in [
            (KeyCode::KeyA, Key::A),
            (KeyCode::KeyB, Key::B),
            (KeyCode::KeyM, Key::M),
            (KeyCode::KeyY, Key::Y),
            (KeyCode::KeyZ, Key::Z),
        ] {
            assert_eq!(key_from(code), Some(key));
        }
        // 未知键返回 None。
        assert_eq!(key_from(KeyCode::F1), None);
        assert_eq!(key_from(KeyCode::Home), None);
        assert_eq!(key_from(KeyCode::ShiftRight), None);
        assert_eq!(key_from(KeyCode::ControlRight), None);
        assert_eq!(key_from(KeyCode::AltRight), None);
    }

    #[test]
    fn ui_queue_push_take_drains_in_order() {
        clear_ui_events();
        push_ui_event(UiEvent::CursorMoved(1.0, 2.0));
        push_ui_event(UiEvent::CursorGone);
        push_ui_event(UiEvent::Text('a'));
        let taken = take_ui_events();
        assert_eq!(
            taken,
            vec![
                UiEvent::CursorMoved(1.0, 2.0),
                UiEvent::CursorGone,
                UiEvent::Text('a')
            ]
        );
        // take 是排空语义:再次 take 为空。
        assert!(take_ui_events().is_empty());
    }

    #[test]
    fn ui_queue_capacity_keeps_latest_drops_oldest() {
        clear_ui_events();
        for i in 0..1000 {
            push_ui_event(UiEvent::Text(char::from_u32('a' as u32 + i).unwrap_or('a')));
        }
        assert_eq!(take_ui_events().len(), 1000);

        clear_ui_events();
        // 塞满 1025 条:保留最近 1024 条,最旧的 1 条被丢弃。
        for i in 0..(UI_EVENT_QUEUE_CAPACITY as u32 + 1) {
            push_ui_event(UiEvent::CursorMoved(i as f64, 0.0));
        }
        let taken = take_ui_events();
        assert_eq!(taken.len(), UI_EVENT_QUEUE_CAPACITY);
        // 首元素是最新保留的最旧一条:i=1;末元素为最新 i=1024。
        assert_eq!(taken[0], UiEvent::CursorMoved(1.0, 0.0));
        assert_eq!(
            taken[UI_EVENT_QUEUE_CAPACITY - 1],
            UiEvent::CursorMoved(1024.0, 0.0)
        );
    }

    #[test]
    fn ui_queue_clear_empties() {
        clear_ui_events();
        push_ui_event(UiEvent::Text('x'));
        clear_ui_events();
        assert!(take_ui_events().is_empty());
    }
}

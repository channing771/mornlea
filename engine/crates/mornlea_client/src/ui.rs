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

use std::sync::Arc;

use egui::{
    Align2, Color32, CornerRadius, Direction, FontData, FontDefinitions, FontFamily, FontId, Key,
    Layout, RawInput, Rect, RichText, UiBuilder, ViewportId, pos2, vec2,
};

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
/// `enabled` 在 ABI v1 线格式中并无逐按钮编码(见 [`decode_ui_frame`]);
/// 解码侧一律置为 `true`,禁用态只能在 UI 语义层(Go 侧)或直接构造
/// [`UiFrame`] 时表达——这是 v1 布局的已知留白,详见任务报告。
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
        // ABI v1 不携带逐按钮 enabled,解码侧一律视为可用。
        let label = reader.string_field(MAX_UI_LABEL_BYTES)?;
        buttons.push(UiButton {
            id,
            label,
            enabled: true,
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

#[cfg(test)]
mod tests {
    use super::*;

    /// 测试用最小 ABI 编码器,与 Go `EncodeUIMenu` 的字节语义一致。
    fn encode_frame(
        layout: u32,
        flags: u32,
        buttons: &[(u32, &str)],
        title: &str,
        version: &str,
        error: &str,
    ) -> Vec<u8> {
        let mut b = Vec::new();
        b.extend_from_slice(&layout.to_le_bytes());
        b.extend_from_slice(&flags.to_le_bytes());
        b.extend_from_slice(&(buttons.len() as u32).to_le_bytes());
        for (id, label) in buttons {
            b.extend_from_slice(&id.to_le_bytes());
            b.extend_from_slice(&(label.len() as u32).to_le_bytes());
            b.extend_from_slice(label.as_bytes());
        }
        b.extend_from_slice(&(title.len() as u32).to_le_bytes());
        b.extend_from_slice(title.as_bytes());
        b.extend_from_slice(&(version.len() as u32).to_le_bytes());
        b.extend_from_slice(version.as_bytes());
        b.extend_from_slice(&(error.len() as u32).to_le_bytes());
        b.extend_from_slice(error.as_bytes());
        b
    }

    fn four_button_frame() -> Vec<u8> {
        encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[
                (1, "进入游戏"),
                (2, "多人游戏"),
                (3, "设置"),
                (4, "退出游戏"),
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
        let many = (1..=9u32).map(|i| (i, "x")).collect::<Vec<_>>();
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
                &[(1, &long_label)],
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
        let labels = (0..8u32).map(|i| (i, "button")).collect::<Vec<_>>();
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
        let target = rects[1].center();
        click_button(&mut state, &frame, target);
        assert_eq!(state.drain_events(), vec![2]);
        assert!(state.drain_events().is_empty());
    }

    #[test]
    fn menu_disabled_button_click_no_event() {
        let mut state = UiState::new();
        state.install_font(test_font());
        // ABI v1 线格式不编码 enabled,故此处绕过 decode 直接构造禁用帧。
        let mut frame = decode_ui_frame(&four_button_frame()).unwrap();
        frame.buttons[1].enabled = false;

        let rects = menu_button_layout(screen_rect(), 4);
        let target = rects[1].center();
        // 用两帧完整点击(按下+释放),禁用按钮仍不应产生事件。
        click_button(&mut state, &frame, target);
        assert!(state.drain_events().is_empty());
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
}

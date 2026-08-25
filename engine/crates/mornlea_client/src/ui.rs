//! 菜单 UI 模型:ABI 帧解码、egui 布局/绘制、RawInput 组装与事件队列。
//!
//! 本模块是纯状态逻辑,不创建真实窗口或 GPU 资源,可无头单测。设计目标:
//! 客户端菜单的所有**呈现**(按钮几何、标题/版本/错误行的确定性布局)与
//! **输入翻译**([`UiEvent`] -> egui 事件)都归 Rust,而菜单**语义**(相位、
//! 按钮 id/禁用、文本)留在 Go,经 client ABI v9 的 UI 段与结构化事件出口双向传递。
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
    Align, Align2, Color32, CornerRadius, Direction, FontData, FontDefinitions, FontFamily, FontId,
    Id, Key, Layout, RawInput, Rect, RichText, UiBuilder, ViewportId, pos2, vec2,
};
use winit::event::{ElementState, Ime, MouseButton, MouseScrollDelta, WindowEvent};
use winit::keyboard::{KeyCode, PhysicalKey};

// ---------------------------------------------------------------------------
// ABI 布局常量(与 Go `EncodeUIMenu` 逐字节对应,小端;任何改动必须同时改 Go)。
// ---------------------------------------------------------------------------

/// 主菜单 UI 段布局版本。
pub const UI_LAYOUT_VERSION: u32 = 1;
/// 设置页 UI 段布局版本。
pub const UI_SETTINGS_LAYOUT_VERSION: u32 = 2;
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
/// 设置页状态提示的字节上界。
pub const MAX_UI_STATUS_BYTES: usize = 256;
/// 设置页材质路径的字节上界。
pub const MAX_UI_SETTINGS_PATH_BYTES: usize = 1024;
/// 设置页「保存」动作编号；延续主菜单动作 1..=4。
pub const UI_ACTION_SETTINGS_SAVE: u32 = 5;
/// 设置页「取消更改」动作编号。
pub const UI_ACTION_SETTINGS_CANCEL: u32 = 6;
/// 设置页「返回」与 Escape 共用的动作编号。
pub const UI_ACTION_SETTINGS_BACK: u32 = 7;

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
/// 按钮数为 0(标题独占)时标题底边距屏幕中心的向上偏移(逻辑点)。
///
/// 标题绘制不依赖按钮列几何:有按钮时标题锚在首按钮上方,无按钮时用此
/// 固定偏移落在屏幕上半部,避免标题丢失。
pub const MENU_TITLE_EMPTY_CENTER_GAP: f32 = 120.0;
/// 全屏不透明深灰背景色(参考经典标题画面基调)。
pub const MENU_BACKGROUND: Color32 = Color32::from_rgb(32, 36, 42);
/// 标题/版本行的白色。
pub const MENU_TEXT_COLOR: Color32 = Color32::WHITE;
/// 错误行红色。
pub const MENU_ERROR_COLOR: Color32 = Color32::from_rgb(240, 90, 90);

// ---------------------------------------------------------------------------
// 设置页布局常量。
// ---------------------------------------------------------------------------

/// 设置页面板最大宽度。
const SETTINGS_PANEL_MAX_WIDTH: f32 = 560.0;
/// 设置页面板最大高度。
const SETTINGS_PANEL_MAX_HEIGHT: f32 = 620.0;
/// 设置页面板与屏幕边缘的最小总留白。
const SETTINGS_PANEL_SCREEN_GAP: f32 = 32.0;
/// 设置页面板内边距。
const SETTINGS_PANEL_PADDING: f32 = 20.0;
/// 设置页动作按钮高度。
const SETTINGS_ACTION_HEIGHT: f32 = 36.0;
/// 设置页深色面板背景。
const SETTINGS_PANEL_BACKGROUND: Color32 = Color32::from_rgb(43, 48, 56);
/// 设置页次要说明文字颜色。
const SETTINGS_MUTED_COLOR: Color32 = Color32::from_rgb(184, 190, 200);
/// 设置页正常状态文字颜色。
const SETTINGS_STATUS_COLOR: Color32 = Color32::from_rgb(134, 210, 160);
/// 设置页脏草稿提示颜色。
const SETTINGS_DIRTY_COLOR: Color32 = Color32::from_rgb(244, 196, 96);
/// 材质路径 TextEdit 的跨帧稳定 id 来源。
const SETTINGS_TEXTURE_PATH_ID_SOURCE: &str = "mornlea-settings-texture-path";

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

/// 一帧完整主菜单的 Rust 表示。
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct UiMenuFrame {
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

/// 设置页固定窗口预设。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UiSettingsWindow {
    /// 640×360 逻辑像素。
    Size640x360,
    /// 960×540 逻辑像素。
    Size960x540,
    /// 1280×720 逻辑像素。
    Size1280x720,
}

/// 设置页 headless 测试记录的关键几何标识。
///
/// 生产绘制同样走这些标识，但记录闭包为空；这样测试观测的是唯一生产布局，
/// 不需要复制一套会漂移的坐标算法。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum SettingsElement {
    Panel,
    ScrollViewport,
    ScrollContent,
    Audio,
    TexturePath,
    Window640x360,
    Window960x540,
    Window1280x720,
    Save,
    Cancel,
    Back,
}

impl UiSettingsWindow {
    fn decode(value: u32) -> Result<Self, ()> {
        match value {
            1 => Ok(Self::Size640x360),
            2 => Ok(Self::Size960x540),
            3 => Ok(Self::Size1280x720),
            _ => Err(()),
        }
    }

    fn encode(self) -> u32 {
        match self {
            Self::Size640x360 => 1,
            Self::Size960x540 => 2,
            Self::Size1280x720 => 3,
        }
    }
}

/// 一帧完整设置页的 Rust 表示。
#[derive(Debug, Clone, PartialEq)]
pub struct UiSettingsFrame {
    /// 设置页是否可见。
    pub visible: bool,
    /// 总音量，闭区间 `[0,1]`。
    pub audio_volume: f32,
    /// 固定窗口预设。
    pub window: UiSettingsWindow,
    /// 材质包目录原文。
    pub texture_pack_path: String,
    /// 草稿是否相对已保存值有变化。
    pub dirty: bool,
    /// 非错误状态提示。
    pub status: String,
    /// 有界错误提示。
    pub error: String,
}

/// 一帧 UI 下行快照；client ABI v9 保留主菜单 layout v1 并新增设置页
/// layout v2。
#[derive(Debug, Clone, PartialEq)]
pub enum UiFrame {
    /// 主菜单 layout v1。
    Menu(UiMenuFrame),
    /// 设置页 layout v2。
    Settings(UiSettingsFrame),
}

impl UiFrame {
    /// 报告当前布局是否可见。
    pub fn visible(&self) -> bool {
        match self {
            Self::Menu(frame) => frame.visible,
            Self::Settings(frame) => frame.visible,
        }
    }
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
    let frame = match layout {
        UI_LAYOUT_VERSION => UiFrame::Menu(decode_menu_frame(&mut reader)?),
        UI_SETTINGS_LAYOUT_VERSION => UiFrame::Settings(decode_settings_frame(&mut reader)?),
        _ => return Err(()),
    };
    if !reader.done() {
        return Err(());
    }
    Ok(frame)
}

fn decode_menu_frame(reader: &mut Reader<'_>) -> Result<UiMenuFrame, ()> {
    let flags = reader.u32()?;
    let visible = decode_bool(flags)?;
    let button_count = reader.u32()? as usize;
    if button_count > MAX_UI_BUTTONS {
        return Err(());
    }
    let mut buttons = Vec::with_capacity(button_count);
    for _ in 0..button_count {
        let id = reader.u32()?;
        let label = reader.string_field(MAX_UI_LABEL_BYTES)?;
        // ABI v1 逐按钮携带 enabled u32:只接受 0(禁用)/1(启用),其余视为非法。
        let enabled = decode_bool(reader.u32()?)?;
        buttons.push(UiButton { id, label, enabled });
    }
    let title = reader.string_field(MAX_UI_TITLE_BYTES)?;
    let version = reader.string_field(MAX_UI_VERSION_BYTES)?;
    let error = reader.string_field(MAX_UI_ERROR_BYTES)?;
    Ok(UiMenuFrame {
        visible,
        title,
        version,
        error,
        buttons,
    })
}

fn decode_settings_frame(reader: &mut Reader<'_>) -> Result<UiSettingsFrame, ()> {
    let visible = decode_bool(reader.u32()?)?;
    let audio_volume = reader.f32()?;
    if !valid_audio(audio_volume) {
        return Err(());
    }
    let window = UiSettingsWindow::decode(reader.u32()?)?;
    let texture_pack_path = reader.string_field(MAX_UI_SETTINGS_PATH_BYTES)?;
    if texture_pack_path.contains(['\r', '\n']) {
        return Err(());
    }
    let dirty = decode_bool(reader.u32()?)?;
    let status = reader.string_field(MAX_UI_STATUS_BYTES)?;
    let error = reader.string_field(MAX_UI_ERROR_BYTES)?;
    Ok(UiSettingsFrame {
        visible,
        audio_volume,
        window,
        texture_pack_path,
        dirty,
        status,
        error,
    })
}

fn decode_bool(value: u32) -> Result<bool, ()> {
    match value {
        0 => Ok(false),
        1 => Ok(true),
        _ => Err(()),
    }
}

fn valid_audio(value: f32) -> bool {
    value.is_finite() && (0.0..=1.0).contains(&value)
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

    /// 读一个 f32(小端)；数值域由调用方校验。
    #[allow(clippy::result_unit_err)]
    fn f32(&mut self) -> Result<f32, ()> {
        Ok(f32::from_bits(self.u32()?))
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

    fn done(&self) -> bool {
        self.pos == self.bytes.len()
    }
}

// ---------------------------------------------------------------------------
// egui 上下文状态:字体、绘制、事件队列。
// ---------------------------------------------------------------------------

/// 上行 `settings-changed` 事件携带的完整设置草稿。
#[derive(Debug, Clone, PartialEq)]
pub struct UiSettingsValues {
    /// 总音量，闭区间 `[0,1]`。
    pub audio_volume: f32,
    /// 固定窗口预设。
    pub window: UiSettingsWindow,
    /// 材质包目录原文。
    pub texture_pack_path: String,
}

/// client ABI v9 的结构化 UI 上行事件。
#[derive(Debug, Clone, PartialEq)]
pub enum UiOutputEvent {
    /// 由 Go 解释的动作 id。
    Action(u32),
    /// 一帧控件变化后的完整最终草稿。
    SettingsChanged(UiSettingsValues),
}

/// 结构化 UI 输出队列失败原因。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UiOutputError {
    /// 入队或排空缓冲容量不足。
    Capacity,
    /// 事件内部值违反线格式边界。
    Invalid,
}

/// 每个 renderer 最多积压的结构化 UI 输出事件数。
pub const UI_OUTPUT_QUEUE_CAPACITY: usize = 64;
/// 主菜单单帧最大输出数：最多八个按钮都各产生一个 action。
const MAX_MENU_OUTPUT_EVENTS_PER_FRAME: usize = MAX_UI_BUTTONS;
/// 设置页单帧最大输出数：一个最终草稿 change 加保存/取消/返回三个 action。
const MAX_SETTINGS_OUTPUT_EVENTS_PER_FRAME: usize = 1 + 3;
/// 结构化事件 batch 的布局版本。
pub const UI_EVENT_BATCH_LAYOUT: u32 = 1;
/// action 事件类型编号。
pub const UI_EVENT_KIND_ACTION: u32 = 1;
/// settings-changed 事件类型编号。
pub const UI_EVENT_KIND_SETTINGS_CHANGED: u32 = 2;

/// 每个 renderer 私有的 64 条有界结构化输出队列。
#[derive(Debug)]
pub struct UiOutputQueue {
    pending: VecDeque<UiOutputEvent>,
}

impl Default for UiOutputQueue {
    fn default() -> Self {
        Self {
            pending: VecDeque::with_capacity(UI_OUTPUT_QUEUE_CAPACITY),
        }
    }
}

impl UiOutputQueue {
    /// 创建空队列。
    pub fn new() -> Self {
        Self::default()
    }

    /// 返回当前待排空事件数。
    pub fn len(&self) -> usize {
        self.pending.len()
    }

    /// 报告队列是否为空。
    pub fn is_empty(&self) -> bool {
        self.pending.is_empty()
    }

    /// 报告队列能否完整容纳下一帧的保守事件上界。
    fn has_frame_capacity(&self, required: usize) -> bool {
        required <= UI_OUTPUT_QUEUE_CAPACITY - self.pending.len()
    }

    /// 入队单条事件。第 65 条显式返回 [`UiOutputError::Capacity`]，既不
    /// 丢最旧也不丢最新；非法设置快照返回 [`UiOutputError::Invalid`]。
    pub fn enqueue(&mut self, event: UiOutputEvent) -> Result<(), UiOutputError> {
        self.enqueue_frame(std::slice::from_ref(&event))
    }

    /// 原子追加同一 egui 帧生成的一组事件。整组放不下时队列保持不变，
    /// 从而保证 change-before-save 既不重排也不部分入队。
    pub fn enqueue_frame(&mut self, events: &[UiOutputEvent]) -> Result<(), UiOutputError> {
        if events.iter().any(|event| !valid_output_event(event)) {
            return Err(UiOutputError::Invalid);
        }
        if events.len() > UI_OUTPUT_QUEUE_CAPACITY - self.pending.len() {
            return Err(UiOutputError::Capacity);
        }
        self.pending.extend(events.iter().cloned());
        Ok(())
    }

    /// 把完整 batch 写进 `out` 并仅在成功后清空队列。容量不足时 `out` 与
    /// 队列均不变；空队列仍编码为 8 字节合法空 batch。
    pub fn drain_into(&mut self, out: &mut [u8]) -> Result<usize, UiOutputError> {
        let required = self.encoded_len();
        if out.len() < required {
            return Err(UiOutputError::Capacity);
        }
        let mut cursor = 0;
        write_u32(out, &mut cursor, UI_EVENT_BATCH_LAYOUT);
        write_u32(out, &mut cursor, self.pending.len() as u32);
        for event in &self.pending {
            match event {
                UiOutputEvent::Action(action_id) => {
                    write_u32(out, &mut cursor, UI_EVENT_KIND_ACTION);
                    write_u32(out, &mut cursor, 4);
                    write_u32(out, &mut cursor, *action_id);
                }
                UiOutputEvent::SettingsChanged(settings) => {
                    let payload_len = 12 + settings.texture_pack_path.len();
                    write_u32(out, &mut cursor, UI_EVENT_KIND_SETTINGS_CHANGED);
                    write_u32(out, &mut cursor, payload_len as u32);
                    write_u32(out, &mut cursor, settings.audio_volume.to_bits());
                    write_u32(out, &mut cursor, settings.window.encode());
                    write_u32(out, &mut cursor, settings.texture_pack_path.len() as u32);
                    let end = cursor + settings.texture_pack_path.len();
                    out[cursor..end].copy_from_slice(settings.texture_pack_path.as_bytes());
                    cursor = end;
                }
            }
        }
        debug_assert_eq!(cursor, required);
        self.pending.clear();
        Ok(required)
    }

    fn encoded_len(&self) -> usize {
        8 + self
            .pending
            .iter()
            .map(|event| match event {
                UiOutputEvent::Action(_) => 8 + 4,
                UiOutputEvent::SettingsChanged(settings) => {
                    8 + 12 + settings.texture_pack_path.len()
                }
            })
            .sum::<usize>()
    }

    #[cfg(test)]
    fn events(&self) -> Vec<UiOutputEvent> {
        self.pending.iter().cloned().collect()
    }
}

fn valid_output_event(event: &UiOutputEvent) -> bool {
    match event {
        UiOutputEvent::Action(_) => true,
        UiOutputEvent::SettingsChanged(settings) => {
            valid_audio(settings.audio_volume)
                && settings.texture_pack_path.len() <= MAX_UI_SETTINGS_PATH_BYTES
                && !settings.texture_pack_path.contains(['\r', '\n'])
        }
    }
}

fn write_u32(out: &mut [u8], cursor: &mut usize, value: u32) {
    let end = *cursor + 4;
    out[*cursor..end].copy_from_slice(&value.to_le_bytes());
    *cursor = end;
}

/// 持有 [`egui::Context`] 与结构化 UI 输出队列的纯状态容器。
///
/// 不建窗口、不碰 GPU;`run_frame` 全 CPU 布局,`install_font` 上传字体,
/// `drain_events` 把 action/settings-changed batch 交给 Go。字体字节经 ABI 从 Go 一次性上传,
/// Rust 侧不内嵌字体二进制。
pub struct UiState {
    ctx: egui::Context,
    /// 结构化事件输出队列。
    pending_events: UiOutputQueue,
    /// 字体是否已成功安装(空字节安装失败保持 false)。
    font_loaded: bool,
    /// 最近一帧设置页关键几何；只供无头测试验证生产布局。
    #[cfg(test)]
    settings_layout: Vec<(SettingsElement, Rect, Option<Id>)>,
    /// 下一帧在同一 egui pass 内设置的材质路径字符光标；只供无头测试。
    #[cfg(test)]
    settings_cursor_override: Option<usize>,
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
            pending_events: UiOutputQueue::new(),
            font_loaded: false,
            #[cfg(test)]
            settings_layout: Vec::new(),
            #[cfg(test)]
            settings_cursor_override: None,
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

    /// 运行一帧 UI:无字体或当前布局不可见时返回 `None`(零工作)。
    ///
    /// `pixels_per_point` 被写进 ROOT 视口的 `native_pixels_per_point`,
    /// 这是 egui 0.35 的缩放来源(不再作为 `run_ui` 的独立参数)。
    /// 可见布局会在消费 RawInput 前按其单帧最大事件数保守预留队列；即使本帧
    /// 实际不会产生事件，只要最坏情况装不下也返回 [`UiOutputError::Capacity`]。
    /// 这项刻意的提前拒绝换取可重试语义：错误时焦点、光标、滚动均未推进。
    pub fn run_frame(
        &mut self,
        mut raw: RawInput,
        frame: &UiFrame,
        pixels_per_point: f32,
    ) -> Result<Option<egui::FullOutput>, UiOutputError> {
        if !self.font_loaded || !frame.visible() {
            return Ok(None);
        }
        let max_frame_events = match frame {
            UiFrame::Menu(_) => MAX_MENU_OUTPUT_EVENTS_PER_FRAME,
            UiFrame::Settings(_) => MAX_SETTINGS_OUTPUT_EVENTS_PER_FRAME,
        };
        if !self.pending_events.has_frame_capacity(max_frame_events) {
            return Err(UiOutputError::Capacity);
        }
        if let Some(info) = raw.viewports.get_mut(&ViewportId::ROOT) {
            info.native_pixels_per_point = Some(pixels_per_point);
        }
        if matches!(frame, UiFrame::Settings(_)) {
            filter_settings_text_events(&mut raw);
        }
        let mut frame_events = Vec::new();
        let output = match frame {
            UiFrame::Menu(frame) => self
                .ctx
                .run_ui(raw, |ui| draw_menu(ui, frame, &mut frame_events)),
            UiFrame::Settings(frame) => {
                let initial = UiSettingsValues {
                    audio_volume: frame.audio_volume,
                    window: frame.window,
                    texture_pack_path: frame.texture_pack_path.clone(),
                };
                let mut draft = initial.clone();
                let mut actions = SettingsActions::default();
                #[cfg(test)]
                let settings_layout = &mut self.settings_layout;
                #[cfg(test)]
                let settings_cursor_override = &mut self.settings_cursor_override;
                let output = self.ctx.run_ui(raw, |ui| {
                    // egui 在需要重排时可能重跑同一帧；动作使用布尔汇总、草稿只保留
                    // 最终值，从源头保证每帧最多一个 change 和每种 action 一个事件。
                    #[cfg(test)]
                    settings_layout.clear();
                    #[cfg(test)]
                    if let Some(char_index) = settings_cursor_override.take() {
                        let id = Id::new(SETTINGS_TEXTURE_PATH_ID_SOURCE);
                        let mut text_state = egui::TextEdit::load_state(ui.ctx(), id)
                            .expect("设置页 TextEdit 状态应已初始化");
                        text_state
                            .cursor
                            .set_char_range(Some(egui::text::CCursorRange::one(
                                egui::text::CCursor::new(char_index),
                            )));
                        egui::TextEdit::store_state(ui.ctx(), id, text_state);
                    }
                    #[cfg(test)]
                    draw_settings(
                        ui,
                        frame,
                        &mut draft,
                        &mut actions,
                        &mut |element, rect, id| settings_layout.push((element, rect, id)),
                    );
                    #[cfg(not(test))]
                    draw_settings(ui, frame, &mut draft, &mut actions, &mut |_, _, _| {});
                });
                if draft != initial {
                    frame_events.push(UiOutputEvent::SettingsChanged(draft));
                }
                actions.append_events(&mut frame_events);
                output
            }
        };
        self.pending_events.enqueue_frame(&frame_events)?;
        Ok(Some(output))
    }

    /// 把累积结构化事件按 client ABI v9 完整 batch 排空到 `out`。
    pub fn drain_events(&mut self, out: &mut [u8]) -> Result<usize, UiOutputError> {
        self.pending_events.drain_into(out)
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

#[derive(Default)]
struct SettingsActions {
    save: bool,
    cancel: bool,
    back: bool,
}

impl SettingsActions {
    fn append_events(&self, pending: &mut Vec<UiOutputEvent>) {
        if self.save {
            pending.push(UiOutputEvent::Action(UI_ACTION_SETTINGS_SAVE));
        }
        if self.cancel {
            pending.push(UiOutputEvent::Action(UI_ACTION_SETTINGS_CANCEL));
        }
        if self.back {
            pending.push(UiOutputEvent::Action(UI_ACTION_SETTINGS_BACK));
        }
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
/// `enabled` 的按钮 id 以 [`UiOutputEvent::Action`] 压入 `pending`。
fn draw_menu(ui: &mut egui::Ui, frame: &UiMenuFrame, pending: &mut Vec<UiOutputEvent>) {
    let screen = ui.max_rect();

    // 全屏不透明深灰背景。
    ui.painter()
        .rect_filled(screen, CornerRadius::ZERO, MENU_BACKGROUND);

    let rects = menu_button_layout(screen, frame.buttons.len());

    // 标题:位于按钮列上方、水平居中;始终绘制,不被按钮数 gating。
    // 无按钮(标题独占)时用屏幕上半部固定偏移,有按钮时锚在首按钮上方。
    let title_y = rects
        .first()
        .map_or(screen.center().y - MENU_TITLE_EMPTY_CENTER_GAP, |first| {
            first.min.y - MENU_TITLE_BUTTON_GAP
        });
    ui.painter().text(
        pos2(screen.center().x, title_y),
        Align2::CENTER_BOTTOM,
        &frame.title,
        FontId::proportional(MENU_TITLE_FONT_SIZE),
        MENU_TEXT_COLOR,
    );

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
            pending.push(UiOutputEvent::Action(button.id));
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
// 设置页绘制（Go 草稿回显为权威，Rust 仅持有 egui 瞬态状态）。
// ---------------------------------------------------------------------------

fn settings_panel_rect(screen: Rect) -> Rect {
    let width = screen
        .width()
        .min(SETTINGS_PANEL_MAX_WIDTH)
        .min((screen.width() - SETTINGS_PANEL_SCREEN_GAP).max(1.0));
    let height = screen
        .height()
        .min(SETTINGS_PANEL_MAX_HEIGHT)
        .min((screen.height() - SETTINGS_PANEL_SCREEN_GAP).max(1.0));
    Rect::from_center_size(screen.center(), vec2(width, height))
}

/// 删除换行，并在本帧编辑越过 ABI 上限时拒绝该次编辑。
///
/// 下行 `original` 已被 ABI 解码器证明合法；越界时恢复它，能避免用户在文本
/// 中部插入多字节字符后由「截断末尾」误删原有路径内容。恢复的是完整合法
/// UTF-8 字符串，所以不可能落在字符中间。
fn sanitize_settings_path(original: &str, path: &mut String) {
    path.retain(|ch| ch != '\r' && ch != '\n');
    if path.len() > MAX_UI_SETTINGS_PATH_BYTES {
        original.clone_into(path);
    }
}

/// 在 `TextEdit::singleline` 把换行折叠为空格之前先移除 CR/LF。
///
/// 这条门禁既覆盖手工 RawInput，也覆盖未来可能接入的粘贴事件；随后
/// [`sanitize_settings_path`] 仍做最终防御，保证任何 egui 行为变化都不会把
/// 多行或超长字符串送入上行事件。
fn filter_settings_text_events(raw: &mut RawInput) {
    for event in &mut raw.events {
        match event {
            egui::Event::Text(text) | egui::Event::Paste(text) => {
                text.retain(|ch| ch != '\r' && ch != '\n');
            }
            _ => {}
        }
    }
}

fn settings_window_button(
    ui: &mut egui::Ui,
    draft: &mut UiSettingsValues,
    value: UiSettingsWindow,
    label: &str,
    width: f32,
) -> egui::Response {
    let selected = draft.window == value;
    let response = ui.add_sized(
        vec2(width, 32.0),
        egui::Button::new(label).selected(selected),
    );
    if response.clicked() {
        draft.window = value;
    }
    response
}

/// 绘制设置表单并把本帧控件的最终值写入 `draft`。
///
/// `record` 只用于测试记录生产几何；生产传入空闭包。动作先汇总到
/// `actions`，由 [`UiState::run_frame`] 在完整草稿事件之后统一追加。
fn draw_settings(
    ui: &mut egui::Ui,
    frame: &UiSettingsFrame,
    draft: &mut UiSettingsValues,
    actions: &mut SettingsActions,
    record: &mut impl FnMut(SettingsElement, Rect, Option<Id>),
) {
    let screen = ui.max_rect();
    ui.painter()
        .rect_filled(screen, CornerRadius::ZERO, MENU_BACKGROUND);

    let panel = settings_panel_rect(screen);
    record(SettingsElement::Panel, panel, None);
    ui.painter()
        .rect_filled(panel, CornerRadius::same(8), SETTINGS_PANEL_BACKGROUND);
    let inner = panel.shrink(SETTINGS_PANEL_PADDING);
    let child = UiBuilder::new()
        .max_rect(inner)
        .layout(Layout::top_down(Align::Min));
    let scroll = ui
        .scope_builder(child, |ui| {
            egui::ScrollArea::vertical()
                .id_salt("mornlea-settings-scroll")
                .auto_shrink([false, false])
                .show(ui, |ui| {
                    ui.set_width(ui.available_width());
                    ui.heading(RichText::new("设置").size(28.0));
                    ui.add_space(8.0);

                    let mut percent = draft.audio_volume * 100.0;
                    let audio = ui.add(
                        egui::Slider::new(&mut percent, 0.0..=100.0)
                            .text("总音量")
                            .suffix("%")
                            .max_decimals(0)
                            .step_by(1.0),
                    );
                    if audio.changed() {
                        draft.audio_volume = (percent / 100.0).clamp(0.0, 1.0);
                    }
                    record(SettingsElement::Audio, audio.rect, Some(audio.id));
                    ui.add_space(12.0);

                    ui.label(RichText::new("材质包目录").strong());
                    let original_path = draft.texture_pack_path.clone();
                    let mut path = original_path.clone();
                    let path_response = ui.add_sized(
                        vec2(ui.available_width(), 30.0),
                        egui::TextEdit::singleline(&mut path)
                            // 显式全局 id 让焦点/字符光标状态跨帧稳定。
                            .id(Id::new(SETTINGS_TEXTURE_PATH_ID_SOURCE))
                            .hint_text("留空使用内嵌材质")
                            .desired_width(f32::INFINITY),
                    );
                    sanitize_settings_path(&original_path, &mut path);
                    if path != draft.texture_pack_path {
                        draft.texture_pack_path = path;
                    }
                    record(
                        SettingsElement::TexturePath,
                        path_response.rect,
                        Some(path_response.id),
                    );
                    ui.label(
                        RichText::new("材质包路径保存后将在下次启动生效")
                            .small()
                            .color(SETTINGS_MUTED_COLOR),
                    );
                    ui.add_space(12.0);

                    ui.label(RichText::new("窗口大小").strong());
                    let spacing = ui.spacing().item_spacing.x;
                    let button_width = ((ui.available_width() - spacing * 2.0) / 3.0).max(80.0);
                    ui.horizontal(|ui| {
                        let response = settings_window_button(
                            ui,
                            draft,
                            UiSettingsWindow::Size640x360,
                            "640 × 360",
                            button_width,
                        );
                        record(
                            SettingsElement::Window640x360,
                            response.rect,
                            Some(response.id),
                        );
                        let response = settings_window_button(
                            ui,
                            draft,
                            UiSettingsWindow::Size960x540,
                            "960 × 540",
                            button_width,
                        );
                        record(
                            SettingsElement::Window960x540,
                            response.rect,
                            Some(response.id),
                        );
                        let response = settings_window_button(
                            ui,
                            draft,
                            UiSettingsWindow::Size1280x720,
                            "1280 × 720",
                            button_width,
                        );
                        record(
                            SettingsElement::Window1280x720,
                            response.rect,
                            Some(response.id),
                        );
                    });
                    ui.add_space(12.0);

                    if frame.dirty {
                        ui.label(
                            RichText::new("有未保存的更改")
                                .strong()
                                .color(SETTINGS_DIRTY_COLOR),
                        );
                    }
                    if !frame.status.is_empty() {
                        ui.label(RichText::new(&frame.status).color(SETTINGS_STATUS_COLOR));
                    }
                    if !frame.error.is_empty() {
                        ui.label(RichText::new(&frame.error).strong().color(MENU_ERROR_COLOR));
                    }
                    ui.add_space(12.0);
                    ui.separator();
                    ui.add_space(8.0);

                    let spacing = ui.spacing().item_spacing.x;
                    let button_width = ((ui.available_width() - spacing * 2.0) / 3.0).max(80.0);
                    ui.horizontal(|ui| {
                        let save = ui.add_sized(
                            vec2(button_width, SETTINGS_ACTION_HEIGHT),
                            egui::Button::new("保存"),
                        );
                        actions.save |= save.clicked();
                        record(SettingsElement::Save, save.rect, Some(save.id));

                        let cancel = ui.add_sized(
                            vec2(button_width, SETTINGS_ACTION_HEIGHT),
                            egui::Button::new("取消更改"),
                        );
                        actions.cancel |= cancel.clicked();
                        record(SettingsElement::Cancel, cancel.rect, Some(cancel.id));

                        let back = ui.add_sized(
                            vec2(button_width, SETTINGS_ACTION_HEIGHT),
                            egui::Button::new("返回"),
                        );
                        actions.back |= back.clicked();
                        record(SettingsElement::Back, back.rect, Some(back.id));
                    });
                    ui.add_space(4.0);
                })
        })
        .inner;
    record(SettingsElement::ScrollViewport, scroll.inner_rect, None);
    record(
        SettingsElement::ScrollContent,
        Rect::from_min_size(
            scroll.inner_rect.min - scroll.state.offset,
            scroll.content_size,
        ),
        None,
    );

    // Escape 与返回按钮只产生同一个 typed action；是否允许离开由 Go 决定。
    actions.back |= ui.input(|input| input.key_pressed(Key::Escape));
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

// 测试模块按真实关注点拆分；共享夹具唯一收口在 test_support。
#[cfg(test)]
#[path = "ui/input_queue_tests.rs"]
mod input_queue_tests;
#[cfg(test)]
#[path = "ui/menu_abi_tests.rs"]
mod menu_abi_tests;
#[cfg(test)]
#[path = "ui/menu_render_tests.rs"]
mod menu_render_tests;
#[cfg(test)]
#[path = "ui/raw_input_tests.rs"]
mod raw_input_tests;
#[cfg(test)]
#[path = "ui/settings_abi_tests.rs"]
mod settings_abi_tests;
#[cfg(test)]
#[path = "ui/settings_render_tests.rs"]
mod settings_render_tests;
#[cfg(test)]
#[path = "ui/test_support.rs"]
mod test_support;
#[cfg(test)]
#[path = "ui/winit_event_tests.rs"]
mod winit_event_tests;

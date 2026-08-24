//! winit 窗口封装:事件循环泵、事件到 [`InputState`] 的桥接与低频窗口操作。
//!
//! 控制权模型:Go 主线程每帧调用 [`ClientWindow::poll`],内部以零超时
//! `pump_app_events` 处理完积压事件后立即返回并编码快照;winit 从不拥有
//! 主循环。窗口相关逻辑依赖真实窗口系统,单元测试只覆盖纯尺寸计算
//! (`clamp_to_work_area`);事件桥接的可无头验证逻辑位于 [`crate::input`]。

use std::sync::Arc;
use std::time::Duration;

use winit::application::ApplicationHandler;
use winit::dpi::{LogicalSize, PhysicalSize};
use winit::event::{DeviceEvent, DeviceId, ElementState, Ime, MouseButton, WindowEvent};
use winit::event_loop::{ActiveEventLoop, EventLoop};
use winit::keyboard::PhysicalKey;
use winit::platform::pump_events::EventLoopExtPumpEvents;
use winit::window::{CursorGrabMode, Window, WindowId, WindowLevel};

use crate::input::InputState;
use crate::ui;

/// 窗口创建失败的稳定原因,FFI 层统一转为错误状态码。
#[derive(Debug)]
pub enum CreateError {
    /// 事件循环创建失败(通常是非主线程或重复创建)。
    EventLoop,
    /// 泵完首轮事件后窗口仍未建立。
    Window,
}

/// ApplicationHandler 实现:把 winit 事件写入输入状态机。
struct App {
    window: Option<Arc<Window>>,
    input: InputState,
    title: String,
    width: u32,
    height: u32,
    /// IME 组合是否激活;激活期间按键的 `text` 不直接入队,以 `Ime::Commit`
    /// 为准,避免组合过程中的重复字符。
    ime_active: bool,
    /// 菜单 UI 事件桥当前累积的修饰键状态(Shift/Control/Alt 左变体)。
    ///
    /// 由 [`ui::winit_to_ui_events`] 在每个键盘事件处理点先更新再发射,是
    /// 进程内持续状态;与 `InputState` 的游戏按键状态彼此独立。
    ui_modifiers: egui::Modifiers,
}

impl App {
    fn new(width: u32, height: u32, title: String) -> Self {
        Self {
            window: None,
            input: InputState::new(),
            title,
            width,
            height,
            ime_active: false,
            ui_modifiers: egui::Modifiers::default(),
        }
    }

    /// 从窗口读取当前物理/逻辑尺寸并写入状态机。
    fn refresh_sizes(&mut self) {
        if let Some(window) = &self.window {
            let physical = window.inner_size();
            let logical: LogicalSize<u32> = physical.to_logical(window.scale_factor());
            self.input.set_sizes(
                physical.width,
                physical.height,
                logical.width,
                logical.height,
            );
        }
    }
}

impl ApplicationHandler for App {
    fn resumed(&mut self, event_loop: &ActiveEventLoop) {
        if self.window.is_some() {
            return;
        }
        let attributes = Window::default_attributes()
            .with_title(self.title.clone())
            .with_inner_size(LogicalSize::new(self.width, self.height))
            // 最小尺寸与无窗口 capture 一致(640×360),防止窗口被拖得
            // 过小导致 HUD 与交互几何出界。
            .with_min_inner_size(LogicalSize::new(640.0, 360.0));
        match event_loop.create_window(attributes) {
            Ok(window) => {
                // 聊天需要 IME 提交的 Unicode 文本(GLFW char 回调的等价物)。
                window.set_ime_allowed(true);
                // 超屏钳制:物理尺寸超过显示器可用区域时按原宽高比缩小,
                // 保证 1x 屏/非整数缩放下窗口完整可见(标题栏不落到屏幕外)。
                if let Some(monitor) = window.current_monitor() {
                    let current = window.inner_size();
                    let monitor_size = monitor.size();
                    let limit = PhysicalSize::new(
                        (monitor_size.width as f64 * DISPLAY_CLAMP_RATIO).round() as u32,
                        (monitor_size.height as f64 * DISPLAY_CLAMP_RATIO).round() as u32,
                    );
                    let clamped = clamp_to_work_area(current, limit);
                    if clamped != current {
                        // 返回值是系统采用的新尺寸,这里不关心,与 `set_content_size`
                        // 的丢弃口径一致。
                        let _ = window.request_inner_size(clamped);
                    }
                }
                // Arc 包装:渲染器 surface 需要共享窗口所有权(wgpu
                // create_surface 的 'static 约束)。
                self.window = Some(Arc::new(window));
                self.refresh_sizes();
            }
            Err(_) => {
                // 创建失败留给 create() 以 window 缺失判定,不在回调里 panic。
            }
        }
    }

    fn window_event(
        &mut self,
        _event_loop: &ActiveEventLoop,
        _window_id: WindowId,
        event: WindowEvent,
    ) {
        // UI 事件桥:把 winit 事件翻译为 [`UiEvent`] 并压入输入队列,与游戏
        // 输入(InputState)并存。`scale` 与既有 `InputState` 的
        // `CursorMoved` 换算一致;这里**不判断菜单可见性**(渲染侧每帧 take,
        // 菜单不可见时事件被丢弃是设计)。修饰键状态经
        // [`ui::winit_to_ui_events`] 在此进程内累积。
        let scale = self.window.as_ref().map_or(1.0_f64, |w| w.scale_factor());
        for ui_event in ui::winit_to_ui_events(
            std::slice::from_ref(&event),
            scale,
            self.ime_active,
            &mut self.ui_modifiers,
        ) {
            ui::push_ui_event(ui_event);
        }

        match event {
            WindowEvent::CloseRequested => self.input.request_close(),
            WindowEvent::Resized(_) | WindowEvent::ScaleFactorChanged { .. } => {
                self.refresh_sizes();
            }
            WindowEvent::KeyboardInput { event, .. } => {
                if let PhysicalKey::Code(code) = event.physical_key {
                    self.input.key_event(code, event.state.is_pressed());
                }
                // 非 IME 路径的字符输入:与 GLFW char 回调域一致,过滤控制字符。
                if !self.ime_active
                    && event.state.is_pressed()
                    && let Some(text) = event.text.as_ref()
                {
                    for ch in text.chars().filter(|ch| !ch.is_control()) {
                        self.input.push_text(ch);
                    }
                }
            }
            WindowEvent::Ime(ime) => match ime {
                Ime::Enabled => self.ime_active = true,
                Ime::Disabled => self.ime_active = false,
                Ime::Commit(text) => {
                    for ch in text.chars().filter(|ch| !ch.is_control()) {
                        self.input.push_text(ch);
                    }
                }
                Ime::Preedit(..) => {}
            },
            WindowEvent::CursorMoved { position, .. } => {
                if let Some(window) = &self.window {
                    let logical = position.to_logical::<f64>(window.scale_factor());
                    self.input.cursor_moved(logical.x, logical.y);
                }
            }
            WindowEvent::MouseInput { state, button, .. } => {
                let pressed = state == ElementState::Pressed;
                match button {
                    MouseButton::Left => self.input.mouse_button(true, pressed),
                    MouseButton::Right => self.input.mouse_button(false, pressed),
                    _ => {}
                }
            }
            _ => {}
        }
    }

    fn device_event(
        &mut self,
        _event_loop: &ActiveEventLoop,
        _device_id: DeviceId,
        event: DeviceEvent,
    ) {
        if let DeviceEvent::MouseMotion { delta: (dx, dy) } = event {
            self.input.mouse_delta(dx, dy);
        }
    }
}

/// 一个活动窗口:事件循环与状态机的组合,所有方法都必须在创建线程调用
/// (FFI 层以 thread-local 存储保证)。
pub struct ClientWindow {
    event_loop: EventLoop<()>,
    app: App,
}

impl ClientWindow {
    /// 创建窗口:建立事件循环并泵一轮事件以触发 `resumed` 中的窗口创建。
    pub fn create(width: u32, height: u32, title: String) -> Result<Self, CreateError> {
        let mut event_loop = EventLoop::new().map_err(|_| CreateError::EventLoop)?;
        let mut app = App::new(width, height, title);
        event_loop.pump_app_events(Some(Duration::ZERO), &mut app);
        if app.window.is_none() {
            return Err(CreateError::Window);
        }
        Ok(Self { event_loop, app })
    }

    /// 每帧一次:泵完积压事件并把输入快照编码进 `out`
    /// (长度必须为 [`crate::input::SNAPSHOT_BYTES`],由 FFI 层校验)。
    pub fn poll(&mut self, out: &mut [u8]) {
        self.event_loop
            .pump_app_events(Some(Duration::ZERO), &mut self.app);
        self.app.input.encode_snapshot(out);
    }

    /// 切换光标捕获:捕获时隐藏并锁定光标(失败降级 Confined),
    /// 释放时恢复;状态机同步切换虚拟/绝对坐标来源。
    pub fn set_cursor_captured(&mut self, captured: bool) {
        if captured == self.app.input.captured() {
            return;
        }
        if let Some(window) = &self.app.window {
            if captured {
                window.set_cursor_visible(false);
                if window.set_cursor_grab(CursorGrabMode::Locked).is_err() {
                    // macOS 正常支持 Locked;降级 Confined 仍保证不逃出窗口。
                    let _ = window.set_cursor_grab(CursorGrabMode::Confined);
                }
            } else {
                let _ = window.set_cursor_grab(CursorGrabMode::None);
                window.set_cursor_visible(true);
            }
        }
        self.app.input.set_captured(captured);
    }

    /// 请求修改 content 尺寸(逻辑点);实际生效经 Resized 事件回写快照。
    pub fn set_content_size(&mut self, width: u32, height: u32) {
        if let Some(window) = &self.app.window {
            let _ = window.request_inner_size(LogicalSize::new(width, height));
        }
        self.app.refresh_sizes();
    }

    /// 设置窗口置顶(Go `SetFloating` 语义)。
    pub fn set_floating(&mut self, floating: bool) {
        if let Some(window) = &self.app.window {
            let level = if floating {
                WindowLevel::AlwaysOnTop
            } else {
                WindowLevel::Normal
            };
            window.set_window_level(level);
        }
    }

    /// 请求聚焦窗口。
    pub fn focus(&mut self) {
        if let Some(window) = &self.app.window {
            window.focus_window();
        }
    }

    /// 撤销关闭请求。
    pub fn cancel_close(&mut self) {
        self.app.input.cancel_close();
    }

    /// 返回窗口的共享引用,供 windowed 渲染器创建 wgpu surface。
    pub fn shared_window(&self) -> Option<Arc<Window>> {
        self.app.window.clone()
    }

    /// 返回 NSWindow 指针供 gfx 创建 Metal surface。
    ///
    /// winit 的 raw-window-handle 只暴露 NSView;此处经 objc `[view window]`
    /// 取回 NSWindow,与旧 GLFW `GetCocoaWindow` 语义一致,gfx 零改动。
    pub fn ns_window(&self) -> Option<usize> {
        use raw_window_handle::{HasWindowHandle, RawWindowHandle};
        let window = self.app.window.as_ref()?;
        let handle = window.window_handle().ok()?.as_raw();
        let RawWindowHandle::AppKit(appkit) = handle else {
            return None;
        };
        let ns_view: *mut objc2::runtime::AnyObject = appkit.ns_view.as_ptr().cast();
        // SAFETY: ns_view 来自活动窗口的有效句柄,`window` 消息在主线程发送。
        let ns_window: *mut objc2::runtime::AnyObject =
            unsafe { objc2::msg_send![ns_view, window] };
        if ns_window.is_null() {
            return None;
        }
        Some(ns_window as usize)
    }
}

/// 超屏钳制的显示器边距系数:winit 0.30 的 `MonitorHandle` 不暴露工作区
/// (系统菜单栏/Dock 之外的区域),按显示器全尺寸的 90% 近似,给系统栏与
/// 窗口标题栏留出空间,保证钳制后窗口仍然完整可见。
const DISPLAY_CLAMP_RATIO: f64 = 0.9;

/// 把请求的物理尺寸钳制到给定上限内:两轴取最小缩放比、保持宽高比,不
/// 超过上限时原样返回(绝不放大)。上限为 0 时返回原尺寸——窗口宁可照常
/// 创建,也不把尺寸吞成 0。
fn clamp_to_work_area(requested: PhysicalSize<u32>, work: PhysicalSize<u32>) -> PhysicalSize<u32> {
    if requested.width == 0 || requested.height == 0 || work.width == 0 || work.height == 0 {
        return requested;
    }
    let scale = (work.width as f64 / requested.width as f64)
        .min(work.height as f64 / requested.height as f64)
        .min(1.0);
    PhysicalSize::new(
        (requested.width as f64 * scale).round() as u32,
        (requested.height as f64 * scale).round() as u32,
    )
}

#[cfg(test)]
mod tests {
    use super::clamp_to_work_area;
    use winit::dpi::PhysicalSize;

    /// 超屏时按最小缩放比缩小,保持宽高比:16:9 请求在 16:9 工作区内两轴同比例。
    #[test]
    fn oversized_scales_down_preserving_aspect() {
        let got = clamp_to_work_area(PhysicalSize::new(2560, 1440), PhysicalSize::new(1920, 1080));
        assert_eq!(got, PhysicalSize::new(1920, 1080));
    }

    /// 比例不同的两轴取更小者,结果仍在工作区内。
    #[test]
    fn odd_work_area_uses_smallest_ratio() {
        let got = clamp_to_work_area(PhysicalSize::new(1280, 720), PhysicalSize::new(1000, 800));
        assert_eq!(got, PhysicalSize::new(1000, 563));
    }

    /// 尺寸不超过工作区时原样返回,绝不放大。
    #[test]
    fn fits_within_work_area_is_unchanged() {
        let got = clamp_to_work_area(PhysicalSize::new(1280, 720), PhysicalSize::new(2560, 1440));
        assert_eq!(got, PhysicalSize::new(1280, 720));
    }

    /// 工作区无效(0×0)时返回原尺寸,由调用方兜底,不吞掉窗口尺寸。
    #[test]
    fn zero_work_area_returns_requested() {
        let got = clamp_to_work_area(PhysicalSize::new(1280, 720), PhysicalSize::new(0, 0));
        assert_eq!(got, PhysicalSize::new(1280, 720));
    }
}

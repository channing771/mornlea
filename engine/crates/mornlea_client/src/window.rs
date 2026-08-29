//! winit 窗口封装:事件循环泵、事件到 [`InputState`] 的桥接与低频窗口操作。
//!
//! 控制权模型:Go 主线程每帧调用 [`ClientWindow::poll`],内部以零超时
//! `pump_app_events` 处理完积压事件后立即返回并编码快照;winit 从不拥有
//! 主循环。窗口相关逻辑依赖真实窗口系统,单元测试只覆盖纯尺寸与位置计算；
//! 事件桥接的可无头验证逻辑位于 [`crate::input`]。

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
use crate::webview::MenuWebview;

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
            // 真实工作区可能小于最小产品预设。最小值只锁定 16:9 的一个比例
            // 单元，避免 winit 的 640×360 下限反过来阻止 AppKit 安全缩窗。
            .with_min_inner_size(LogicalSize::new(16.0, 9.0));
        match event_loop.create_window(attributes) {
            Ok(window) => {
                // 聊天需要 IME 提交的 Unicode 文本(GLFW char 回调的等价物)。
                window.set_ime_allowed(true);
                // 创建与运行期调整共用 AppKit outer-frame 路径：真实
                // visibleFrame、当前 style/chrome 与窗口位置一次处理。
                apply_content_size_to_work_area(&window, LogicalSize::new(self.width, self.height));
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
        // 菜单层输入已迁 WebView:菜单相位下 WebView 是 firstResponder,键盘
        // 与指针在本窗口路径天然静默,经桥上行;游戏相位 WebView 隐藏并把
        // firstResponder 归还 winit 视图,本路径恢复独占采集。
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
    /// 菜单层 WebView;首个菜单状态推送时惰性挂载(基准/capture 等从不
    /// 推送的进程保持 None,零参与)。
    webview: Option<MenuWebview>,
    /// 挂载失败哨兵:非主线程或视图句柄缺失等失败不可自愈,置位后不再
    /// 重试——上层按「无 WebView」降级,菜单呈现在此类环境缺席。
    webview_attach_failed: bool,
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
        Ok(Self {
            event_loop,
            app,
            webview: None,
            webview_attach_failed: false,
        })
    }

    /// 下行菜单状态推送:首次「需要可见」的状态推送时把 WebView 挂到本窗口
    /// 的 contentView 之上(此后生命周期由 WebView 自管);游戏相位的推送在
    /// WebView 尚未挂载时是纯校验空操作——`-connect` 直进游戏与基准/capture
    /// 路径因此永不创建 WebView,零参与。挂载失败的环境里按「无 WebView」
    /// 降级。JSON 非法(非 UTF-8/缺 phase)返回 false,由 FFI 层转参数错误。
    pub fn push_ui_state(&mut self, json: &[u8]) -> bool {
        let wants_visible = match crate::webview::state_wants_visible(json) {
            Ok(wants_visible) => wants_visible,
            Err(_) => return false,
        };
        if self.webview.is_none() && !self.webview_attach_failed {
            if !wants_visible {
                // 状态合法但无需呈现:不挂载、不报错(纯校验空操作)。
                return true;
            }
            let (ns_window, ns_view) = match (self.ns_window(), self.ns_view()) {
                (Some(ns_window), Some(ns_view)) => (ns_window, ns_view),
                // 句柄未注册(窗口已销毁或非本线程表)视为无窗口降级:推送
                // 被接受但不挂载,不向 FFI 层报参数错误。
                _ => {
                    self.webview_attach_failed = true;
                    return true;
                }
            };
            // SAFETY: 两个指针来自活动窗口句柄,本方法与窗口创建同线程
            // (FFI thread-local 约束),attach 内部再验主线程。
            match unsafe {
                MenuWebview::attach(
                    ns_window as *mut objc2::runtime::AnyObject,
                    ns_view as *mut objc2::runtime::AnyObject,
                )
            } {
                Some(webview) => self.webview = Some(webview),
                None => self.webview_attach_failed = true,
            }
        }
        match &mut self.webview {
            Some(webview) => webview.push_state(json).is_ok(),
            // 挂载失败后的降级路径:推送被接受但不产生任何呈现。
            None => true,
        }
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
            // 设置页传入逻辑 point。AppKit 成功时直接调整 outer frame 并重定位；
            // `request_inner_size` 仅用于无法取得 Cocoa 句柄时的有界 fallback。
            // Retina 物理上限仍由 Go `fitFramebuffer` 独立负责。
            apply_content_size_to_work_area(window, LogicalSize::new(width, height));
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

    /// 返回本窗口的 NSView(winit 渲染视图)指针,供 WebView 挂载与游戏
    /// 相位归还 firstResponder。
    pub fn ns_view(&self) -> Option<usize> {
        use raw_window_handle::{HasWindowHandle, RawWindowHandle};
        let window = self.app.window.as_ref()?;
        let handle = window.window_handle().ok()?.as_raw();
        let RawWindowHandle::AppKit(appkit) = handle else {
            return None;
        };
        let ns_view = appkit.ns_view.as_ptr().cast::<objc2::runtime::AnyObject>();
        if ns_view.is_null() {
            return None;
        }
        Some(ns_view as usize)
    }

    /// 返回 NSWindow 指针供 gfx 创建 Metal surface。
    ///
    /// winit 的 raw-window-handle 只暴露 NSView;此处经 objc `[view window]`
    /// 取回 NSWindow,与旧 GLFW `GetCocoaWindow` 语义一致,gfx 零改动。
    pub fn ns_window(&self) -> Option<usize> {
        let ns_view = self.ns_view()? as *mut objc2::runtime::AnyObject;
        // SAFETY: ns_view 来自活动窗口的有效句柄,`window` 消息在主线程发送。
        let ns_window: *mut objc2::runtime::AnyObject =
            unsafe { objc2::msg_send![ns_view, window] };
        if ns_window.is_null() {
            return None;
        }
        Some(ns_window as usize)
    }
}

/// AppKit 不可用时的保守显示器边距系数。正常 Darwin 路径读取当前 NSWindow
/// 所在 `NSScreen.visibleFrame`；只有句柄、screen 或返回值无效时才退回此值。
const DISPLAY_CLAMP_RATIO: f64 = 0.9;

/// AppKit 不可用且 winit 也没有 monitor 时采用的有界 content fallback。
const FALLBACK_CONTENT_SIZE: LogicalSize<u32> = LogicalSize::new(640, 360);

/// monitor fallback 无法查询真实标题栏高度，保守预留 64 point；宽度 chrome
/// 通常为零，保留为常量便于纯函数明确职责。
const FALLBACK_CHROME_HEIGHT: f64 = 64.0;

/// AppKit 逻辑 point 坐标系中的矩形。与 `NSRect` 分离后，负 origin、位置约束
/// 与 chrome 扣减均可在无窗口单元测试里验证。
#[derive(Clone, Copy, Debug, PartialEq)]
struct LogicalRect {
    x: f64,
    y: f64,
    width: f64,
    height: f64,
}

impl LogicalRect {
    const fn new(x: f64, y: f64, width: f64, height: f64) -> Self {
        Self {
            x,
            y,
            width,
            height,
        }
    }

    fn valid(self) -> bool {
        self.x.is_finite()
            && self.y.is_finite()
            && self.width.is_finite()
            && self.height.is_finite()
            && self.width >= 1.0
            && self.height >= 1.0
    }
}

/// 创建与运行期共用的窗口调整入口。正常路径把 content 请求转换为 AppKit
/// outer frame 并重定位；Cocoa 查询失败才回退到 winit 尺寸请求。
fn apply_content_size_to_work_area(window: &Window, requested: LogicalSize<u32>) {
    if apply_appkit_content_frame(window, requested) {
        return;
    }
    let fallback = fallback_content_size(
        requested,
        window.current_monitor().map(|monitor| monitor.size()),
        window.scale_factor(),
    );
    let _ = window.request_inner_size(fallback.to_physical::<u32>(window.scale_factor()));
}

/// 读取当前 NSWindow、NSScreen visibleFrame、outer frame 与当前 style 对应的
/// chrome，把 content 等比缩入可用区域，再经 AppKit 位置约束设置 outer frame。
///
/// SAFETY：raw-window-handle 的 NSView 与由它借出的 NSWindow/NSScreen 只在活动
/// winit 窗口生命周期、本次调用栈内使用；`ClientWindow` 的 FFI thread-local
/// 约束保证这些消息在创建窗口的 OS 主线程发出。所有指针逐级判 nil，所有按值
/// `NSRect` 都验证有限性和正尺寸；失败立即进入有界 fallback，不持有 ObjC 借用。
fn apply_appkit_content_frame(window: &Window, requested: LogicalSize<u32>) -> bool {
    use objc2::runtime::AnyObject;
    use objc2_foundation::{NSPoint, NSRect, NSSize};
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    let Ok(handle) = window.window_handle() else {
        return false;
    };
    let handle = handle.as_raw();
    let RawWindowHandle::AppKit(appkit) = handle else {
        return false;
    };
    let ns_view: *mut AnyObject = appkit.ns_view.as_ptr().cast();
    // SAFETY：见函数安全说明；NSView 来自当前活动 winit 窗口。
    let ns_window: *mut AnyObject = unsafe { objc2::msg_send![ns_view, window] };
    if ns_window.is_null() {
        return false;
    }
    // SAFETY：NSWindow 的 `screen` 是借用返回值，只在当前调用栈立即使用。
    let screen: *mut AnyObject = unsafe { objc2::msg_send![ns_window, screen] };
    if screen.is_null() {
        return false;
    }
    // SAFETY：以下 `NSRect` 按值返回 ABI 由与 objc2 0.6 同线的
    // objc2-foundation 0.3 描述，不借用 ObjC 内存。
    let visible: NSRect = unsafe { objc2::msg_send![screen, visibleFrame] };
    let current_outer: NSRect = unsafe { objc2::msg_send![ns_window, frame] };
    let visible = logical_rect_from_ns(visible);
    let current_outer = logical_rect_from_ns(current_outer);
    if !visible.valid() || !current_outer.valid() {
        return false;
    }

    // 显式读取 style mask 以锁定当前 NSWindow chrome 语境；实例方法
    // `frameRectForContentRect:` 会按该当前 style 计算标题栏/边框，而不是使用
    // 固定 magic 高度。
    let _style_mask: usize = unsafe { objc2::msg_send![ns_window, styleMask] };
    let requested_content = NSRect::new(
        NSPoint::new(0.0, 0.0),
        NSSize::new(requested.width as f64, requested.height as f64),
    );
    // SAFETY：按值传入/返回 `NSRect`；receiver 是上方判 nil 的当前 NSWindow。
    let requested_outer: NSRect =
        unsafe { objc2::msg_send![ns_window, frameRectForContentRect: requested_content] };
    let chrome = LogicalSize::new(
        (requested_outer.size.width - requested.width as f64).max(0.0),
        (requested_outer.size.height - requested.height as f64).max(0.0),
    );
    if !chrome.width.is_finite() || !chrome.height.is_finite() {
        return false;
    }
    let fitted_content = fit_content_to_visible_frame(requested, visible, chrome);
    let fitted_content_rect = NSRect::new(
        NSPoint::new(0.0, 0.0),
        NSSize::new(fitted_content.width as f64, fitted_content.height as f64),
    );
    // SAFETY：再次让当前 style 计算缩小后 content 的精确 outer 尺寸。
    let fitted_outer: NSRect =
        unsafe { objc2::msg_send![ns_window, frameRectForContentRect: fitted_content_rect] };
    if !fitted_outer.size.width.is_finite()
        || !fitted_outer.size.height.is_finite()
        || fitted_outer.size.width < 1.0
        || fitted_outer.size.height < 1.0
    {
        return false;
    }
    let desired = constrain_outer_frame(
        LogicalRect::new(
            current_outer.x,
            current_outer.y,
            fitted_outer.size.width,
            fitted_outer.size.height,
        ),
        visible,
    );
    let desired_ns = ns_rect_from_logical(desired);
    // SAFETY：AppKit 的约束方法使用同一个当前 NSScreen，处理多屏菜单栏、Dock
    // 与系统保留边缘。返回位置再过纯函数防御，避免异常系统值把窗口推出区域。
    let system_constrained: NSRect =
        unsafe { objc2::msg_send![ns_window, constrainFrameRect: desired_ns, toScreen: screen] };
    let system = logical_rect_from_ns(system_constrained);
    let final_frame = if system.valid() {
        constrain_outer_frame(
            LogicalRect::new(system.x, system.y, desired.width, desired.height),
            visible,
        )
    } else {
        desired
    };
    // SAFETY：`setFrame:display:` 同步设置当前窗口 outer frame，不转移所有权；
    // bool 由 objc2 按 Objective-C BOOL ABI 转换。此调用不请求聚焦窗口。
    let _: () = unsafe {
        objc2::msg_send![ns_window, setFrame: ns_rect_from_logical(final_frame), display: true]
    };
    true
}

fn logical_rect_from_ns(rect: objc2_foundation::NSRect) -> LogicalRect {
    LogicalRect::new(
        rect.origin.x,
        rect.origin.y,
        rect.size.width,
        rect.size.height,
    )
}

fn ns_rect_from_logical(rect: LogicalRect) -> objc2_foundation::NSRect {
    use objc2_foundation::{NSPoint, NSRect, NSSize};
    NSRect::new(
        NSPoint::new(rect.x, rect.y),
        NSSize::new(rect.width, rect.height),
    )
}

/// 从 visibleFrame 扣除当前 style 的 chrome 后钳制 content；计算只处理逻辑
/// point，不接触 scale factor，因此 Retina 物理像素职责不会混入这里。
fn fit_content_to_visible_frame(
    requested: LogicalSize<u32>,
    visible: LogicalRect,
    chrome: LogicalSize<f64>,
) -> LogicalSize<u32> {
    if !visible.valid()
        || !chrome.width.is_finite()
        || !chrome.height.is_finite()
        || chrome.width < 0.0
        || chrome.height < 0.0
    {
        return requested;
    }
    let available_width = (visible.width - chrome.width).floor();
    let available_height = (visible.height - chrome.height).floor();
    if available_width < 1.0 || available_height < 1.0 {
        return requested;
    }
    clamp_logical_to_work_area(
        requested,
        LogicalSize::new(
            available_width.min(u32::MAX as f64) as u32,
            available_height.min(u32::MAX as f64) as u32,
        ),
    )
}

/// 只移动 outer frame 使四边落入 visibleFrame，不改变尺寸。origin 可为负，
/// 因而多屏排列在主屏左侧或下方时仍使用真实 NSScreen 坐标。
fn constrain_outer_frame(outer: LogicalRect, visible: LogicalRect) -> LogicalRect {
    if !outer.valid()
        || !visible.valid()
        || outer.width > visible.width
        || outer.height > visible.height
    {
        return outer;
    }
    LogicalRect::new(
        outer
            .x
            .clamp(visible.x, visible.x + visible.width - outer.width),
        outer
            .y
            .clamp(visible.y, visible.y + visible.height - outer.height),
        outer.width,
        outer.height,
    )
}

/// 在 AppKit 查询失败时，把物理 monitor 尺寸除以 scale factor 后再保守收进
/// 90%。返回单位始终是逻辑 point，Retina 2x 不会误把像素当 point。
fn fallback_work_area_points(monitor: PhysicalSize<u32>, scale_factor: f64) -> LogicalSize<u32> {
    let scale = if scale_factor.is_finite() && scale_factor > 0.0 {
        scale_factor
    } else {
        1.0
    };
    let logical: LogicalSize<f64> = monitor.to_logical(scale);
    LogicalSize::new(
        (logical.width * DISPLAY_CLAMP_RATIO).round().max(1.0) as u32,
        (logical.height * DISPLAY_CLAMP_RATIO).round().max(1.0) as u32,
    )
}

/// AppKit 失败时的纯 fallback：有 monitor 时从物理像素除 scale 得到 point，
/// 再保守扣除 chrome；无 monitor 时钳到 640×360。两条路径都保持请求比例且
/// 只缩不放大，不会完全跳过限制。
fn fallback_content_size(
    requested: LogicalSize<u32>,
    monitor: Option<PhysicalSize<u32>>,
    scale_factor: f64,
) -> LogicalSize<u32> {
    let Some(monitor) = monitor else {
        return clamp_logical_to_work_area(requested, FALLBACK_CONTENT_SIZE);
    };
    let work = fallback_work_area_points(monitor, scale_factor);
    fit_content_to_visible_frame(
        requested,
        LogicalRect::new(0.0, 0.0, work.width as f64, work.height as f64),
        LogicalSize::new(0.0, FALLBACK_CHROME_HEIGHT),
    )
}

/// 把请求的逻辑 point 尺寸钳制到给定上限内:两轴取最小缩放比、保持宽高比,不
/// 超过上限时原样返回(绝不放大)。上限为 0 时返回原尺寸——窗口宁可照常
/// 创建,也不把尺寸吞成 0。
fn clamp_logical_to_work_area(
    requested: LogicalSize<u32>,
    work: LogicalSize<u32>,
) -> LogicalSize<u32> {
    if requested.width == 0 || requested.height == 0 || work.width == 0 || work.height == 0 {
        return requested;
    }
    if requested.width <= work.width && requested.height <= work.height {
        return requested;
    }
    // 以最简宽高比的整数倍缩小，避免两个轴分别 round 后产生一像素的比例
    // 漂移。D-01 的三个请求均为 16:9，因此运行期与创建期都保持精确比例。
    let divisor = greatest_common_divisor(requested.width, requested.height);
    let unit_width = requested.width / divisor;
    let unit_height = requested.height / divisor;
    let units = (work.width / unit_width)
        .min(work.height / unit_height)
        .min(divisor);
    if units == 0 {
        return requested;
    }
    LogicalSize::new(unit_width * units, unit_height * units)
}

fn greatest_common_divisor(mut left: u32, mut right: u32) -> u32 {
    while right != 0 {
        (left, right) = (right, left % right);
    }
    left
}

#[cfg(test)]
mod tests {
    use super::{
        LogicalRect, clamp_logical_to_work_area, constrain_outer_frame, fallback_content_size,
        fallback_work_area_points, fit_content_to_visible_frame,
    };
    use winit::dpi::{LogicalSize, PhysicalSize};

    /// 超屏时按最小缩放比缩小,保持宽高比:16:9 请求在 16:9 工作区内两轴同比例。
    #[test]
    fn oversized_scales_down_preserving_aspect() {
        let got =
            clamp_logical_to_work_area(LogicalSize::new(2560, 1440), LogicalSize::new(1920, 1080));
        assert_eq!(got, LogicalSize::new(1920, 1080));
    }

    /// 比例不同的两轴取更小者,结果仍在工作区内。
    #[test]
    fn odd_work_area_uses_smallest_ratio() {
        let got =
            clamp_logical_to_work_area(LogicalSize::new(1280, 720), LogicalSize::new(1000, 800));
        assert_eq!(got, LogicalSize::new(992, 558));
        assert_eq!(got.width * 9, got.height * 16);
    }

    /// 尺寸不超过工作区时原样返回,绝不放大。
    #[test]
    fn fits_within_work_area_is_unchanged() {
        let got =
            clamp_logical_to_work_area(LogicalSize::new(1280, 720), LogicalSize::new(2560, 1440));
        assert_eq!(got, LogicalSize::new(1280, 720));
    }

    /// 工作区无效(0×0)时返回原尺寸,由调用方兜底,不吞掉窗口尺寸。
    #[test]
    fn zero_work_area_returns_requested() {
        let got = clamp_logical_to_work_area(LogicalSize::new(1280, 720), LogicalSize::new(0, 0));
        assert_eq!(got, LogicalSize::new(1280, 720));
    }

    /// `visibleFrame` 的 point 尺寸远小于全屏 90% 时必须直接使用真实工作区。
    #[test]
    fn real_visible_frame_beats_full_screen_ninety_percent() {
        let got =
            clamp_logical_to_work_area(LogicalSize::new(1280, 720), LogicalSize::new(900, 600));
        assert_eq!(got, LogicalSize::new(896, 504));
    }

    /// fallback 才从物理 monitor/scale 推导保守 point 工作区；Retina 2x 不得
    /// 把物理 2880×1800 误作逻辑点再交给窗口 API。
    #[test]
    fn fallback_separates_retina_pixels_from_logical_points() {
        let got = fallback_work_area_points(PhysicalSize::new(2880, 1800), 2.0);
        assert_eq!(got, LogicalSize::new(1296, 810));
    }

    /// 三个产品预设均保持精确 16:9，工作区足够时不放大也不缩小。
    #[test]
    fn all_presets_keep_sixteen_by_nine_without_upscale() {
        for requested in [
            LogicalSize::new(640, 360),
            LogicalSize::new(960, 540),
            LogicalSize::new(1280, 720),
        ] {
            let got = clamp_logical_to_work_area(requested, LogicalSize::new(1600, 900));
            assert_eq!(got, requested);
            assert_eq!(got.width * 9, got.height * 16);
        }
    }

    /// 工作区内容恰为 16:9 仍不能直接塞入同尺寸 content：标题栏属于 outer
    /// frame，必须先从 visibleFrame 扣除 chrome 再保持 16:9 缩小。
    #[test]
    fn chrome_reduces_content_even_when_visible_frame_is_sixteen_by_nine() {
        let got = fit_content_to_visible_frame(
            LogicalSize::new(1280, 720),
            LogicalRect::new(0.0, 0.0, 1280.0, 720.0),
            LogicalSize::new(0.0, 38.0),
        );
        assert_eq!(got, LogicalSize::new(1200, 675));
    }

    /// 位于右下边缘的窗口放大后必须平移回 visibleFrame；不能只改 inner size
    /// 而把新增的右边或上边留在工作区外。
    #[test]
    fn resize_near_bottom_right_repositions_outer_frame() {
        let got = constrain_outer_frame(
            LogicalRect::new(900.0, 650.0, 800.0, 500.0),
            LogicalRect::new(100.0, 50.0, 1000.0, 700.0),
        );
        assert_eq!(got, LogicalRect::new(300.0, 250.0, 800.0, 500.0));
    }

    /// 左侧或下侧显示器可以使用负 origin；约束必须在 NSScreen 坐标系内完成，
    /// 不能错误地把屏幕原点假定为 (0,0)。
    #[test]
    fn negative_visible_origin_is_preserved_when_repositioning() {
        let got = constrain_outer_frame(
            LogicalRect::new(-500.0, 700.0, 1000.0, 600.0),
            LogicalRect::new(-1440.0, -100.0, 1440.0, 900.0),
        );
        assert_eq!(got, LogicalRect::new(-1000.0, 200.0, 1000.0, 600.0));
    }

    /// AppKit 获取失败且没有 monitor 时，fallback 仍把任意大请求限制到有界的
    /// 640×360 content；不会完全跳过钳制。
    #[test]
    fn fallback_without_monitor_is_bounded() {
        let got = fallback_content_size(LogicalSize::new(2560, 1440), None, 2.0);
        assert_eq!(got, LogicalSize::new(640, 360));
    }
}

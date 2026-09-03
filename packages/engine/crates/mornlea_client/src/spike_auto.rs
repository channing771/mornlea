//! spike 自驱动档:进程内驱动 hitTest 分级(S1)与合成开销(S2)两项验证。
//!
//! `MORNLEA_SPIKE_AUTO=1` 时,本模块挂在既有事件泵
//! ([`crate::window::ClientWindow::poll`],每帧一次、主线程)上执行一条固定
//! 验证脚本,执行者不需要触碰键鼠:
//!
//! 1. **自动进入游戏**:等主菜单就绪后经 WebView `evaluateJavaScript` 查询
//!    `.menu-button`(文本为「进入游戏」的启用项)并 `.click()`——走真实
//!    React onClick → 桥上行 → Go 装配路径;以「下行相位从要求 WebView 可见
//!    变为不可见」作为装配完成的信号。
//! 2. **进程内合成事件**:经 `NSApplication postEvent:atStart:` 投递构造的
//!    `NSEvent`。这走的是真实事件管线(窗口 hitTest → responder 链 → winit
//!    视图),正是 hitTest 分级要验证的路径;不需要辅助功能权限。每一步都用
//!    输入计数探针([`crate::input::InputTaps`])断言事件确实到达 winit,并
//!    用 HUD 顶点流长度变化佐证玩法层确实响应。
//! 3. **S2 测量**:空载与持续采掘两组各 60 秒,组间经
//!    [`crate::overlay_spike::FrameProbe::begin_steady_window`] 丢弃预热帧,
//!    结束后读取环形窗口统计。
//! 4. **结果落盘**:写 `build/spike-result.json`(跨臂合并)与
//!    `build/spike-report.md`(按臂追加一节),随后 `std::process::exit`。
//!
//! 与生产路径的隔离:全部入口由 `MORNLEA_SPIKE_AUTO` 显式开启;计数点以一次
//! 布尔分支守卫,关闭时零写入;文件 I/O 只发生在退出前的一次性落盘,不进入
//! 渲染或权威 tick 热路径。验证完成后本模块随 spike 整体移除。
//!
//! 线程约束:全部入口都在创建窗口的 OS 主线程调用(与 [`crate::webview`]
//! 一致);互斥量只承担借用约束,不存在真实跨线程竞争。
//!
//! 自动化已知限制(三臂一致,不影响跨臂比较;结论只取跨臂差异):
//!
//! - **视角旋转(device-event)**:合成 `NSEvent` 不携带 delta 字段,winit
//!   的 `DeviceEvent::Motion` 在 NSApplication 级分发、本就不经过 WebView
//!   的 `hitTest`,记为非门禁观察。
//! - **右键(副键)**:`+mouseEventWithType:` 构造的右键事件被 AppKit 在
//!   进入 responder 链之前丢弃(左键同路径正常;三臂均未观察到副键计数),
//!   记为非门禁观察。左键与右键走同一条「窗口 hitTest → 内容视图」路径,
//!   左键在 GameOverlay 态与基线一致即足以支撑 hitTest 判据。

use std::ffi::CStr;
use std::path::Path;
use std::path::PathBuf;
use std::sync::Mutex;
use std::sync::OnceLock;
use std::time::Duration;
use std::time::Instant;

use block2::RcBlock;
use objc2::runtime::AnyObject;
use objc2::{class, msg_send};
use objc2_foundation::{NSPoint, NSString};

use crate::input::InputTaps;
use crate::overlay_spike::WindowStats;
use crate::webview::MenuWebview;

/// 结果输出目录(相对进程工作目录,`make run` 即仓库根);可用
/// `MORNLEA_SPIKE_OUT_DIR` 覆盖,目录不可用时回落 `/tmp/mornlea-spike`,
/// 保证失败路径的部分结果也能落盘。
const OUT_DIR_ENV: &str = "MORNLEA_SPIKE_OUT_DIR";
/// 默认输出目录。
const OUT_DIR_DEFAULT: &str = "build";
/// 结构化结果文件名(跨臂合并:每臂跑完读旧值、按臂 upsert、写回)。
const RESULT_JSON: &str = "spike-result.json";
/// 人读报告文件名(每臂追加一节)。
const REPORT_MD: &str = "spike-report.md";

/// 单步无进展超时;超过即按卡点落盘部分结果并以非零码退出。
const STEP_TIMEOUT_MS: u64 = 30_000;
/// 世界装配(进入游戏/退回主菜单后再进入)的等待上限。
const ASSEMBLY_TIMEOUT_MS: u64 = 120_000;
/// 整个脚本的总时长上限;兜底防止脚本自身缺陷让窗口永不关闭。
const TOTAL_TIMEOUT_MS: u64 = 600_000;
/// S2 每组测量时长。
const S2_WINDOW_MS: u64 = 60_000;
/// S2 采掘组的视角扫动周期(毫秒)。
const SWEEP_STEP_MS: u64 = 120;
/// S2 采掘组的快捷栏切换周期(毫秒),制造 HUD 动画(选中态/物品名弹条)。
const HOTBAR_CYCLE_MS: u64 = 2_400;
/// 单帧内最多连续执行的「无等待」操作数;脚本里连续的 Mark/Key 恒为有限个,
/// 该上限只是防御性护栏。
const MAX_OPS_PER_FRAME: usize = 16;

// ---------------------------------------------------------------------------
// 开关与臂位
// ---------------------------------------------------------------------------

/// 返回自驱动档是否开启;进程内只解析一次环境变量。
pub(crate) fn enabled() -> bool {
    static ENABLED: OnceLock<bool> = OnceLock::new();
    *ENABLED.get_or_init(crate::overlay_spike::auto_enabled_from_env)
}

/// 当前臂位:由 spike 强制档位推导,只用于结果标注。
///
/// 标注沿 A 组执行时的命名,但语义已随两态参与模型生产化迁移:不强制
/// (= 生产路径,参与模式由下行相位推导,游戏相位 GameOverlay 常驻可见)
/// 标为 `baseline`;强制 `Menu`(游戏相位回退旧隐藏语义)标为
/// 对照臂 `menu`;强制 `GameOverlay`(与生产的游戏相位行为
/// 一致)标为验证臂 `game`。**复测取基线必须用 `menu` 强制档**——生产化后
/// 缺省档就是 GameOverlay,拿它当基线是 GameOverlay 自比,判据恒过且无
/// 信息量(迁移说明见 `spike-checklist.md` 第 0/1/3 节与 design.md D7)。
fn arm_tag() -> &'static str {
    arm_tag_for(crate::overlay_spike::forced_mode_from_env())
}

/// [`arm_tag`] 的纯函数核,便于在无环境变量的测试里钉值。
fn arm_tag_for(forced: Option<crate::overlay::OverlayMode>) -> &'static str {
    match forced {
        Some(crate::overlay::OverlayMode::Menu) => "menu",
        Some(crate::overlay::OverlayMode::GameOverlay) => "game",
        None => "baseline",
    }
}

/// 解析结果输出目录;不可用时回落 `/tmp/mornlea-spike`。
fn out_dir() -> PathBuf {
    let base = std::env::var(OUT_DIR_ENV).unwrap_or_else(|_| OUT_DIR_DEFAULT.to_string());
    let path = PathBuf::from(base);
    if std::fs::create_dir_all(&path).is_ok() {
        return path;
    }
    let fallback = PathBuf::from("/tmp/mornlea-spike");
    let _ = std::fs::create_dir_all(&fallback);
    fallback
}

// ---------------------------------------------------------------------------
// 观察通道:窗口/桥/渲染三处回调写入,驱动脚本在此之上断言
// ---------------------------------------------------------------------------

/// 跨回调共享的观察值:下行相位、桥上行与渲染侧 HUD 读数。
///
/// 写入点都是主线程上的 FFI 回调,互斥量只承担借用约束;计数字段单调递增,
/// 断言按「自基准以来的增量」解读。
#[derive(Debug, Default, Clone)]
struct Observations {
    /// 最近一次下行状态是否要求 WebView 可见(`None` 表示尚未收到下行)。
    phase_wants_visible: Option<bool>,
    /// 相位翻转次数(进入/退出游戏与暂停开合都计入)。
    phase_flips: u64,
    /// 静默相位(不要求 WebView 可见,即游戏相位)内观察到的驱动帧数。
    quiet_frames: u64,
    /// 静默相位内到达桥上行的上行事件数;GameOverlay 穿透断言的数据源,
    /// 全程必须为 0。
    uplink_in_quiet_phase: u64,
    /// 全程到达桥上行的上行事件总数(菜单点击等合法上行也计入)。
    uplink_total: u64,
    /// 最近一帧 HUD 顶点流字节数。
    hud_bytes: u64,
    /// HUD 顶点流长度发生变化的次数(相邻两帧比较)。
    hud_changes: u64,
    /// 最近一帧是否绘制了目标方块轮廓(准星前有可交互方块)。
    outline_present: bool,
    /// 目标方块轮廓从无到有/从有到无的翻转次数。
    outline_flips: u64,
}

static OBSERVATIONS: Mutex<Option<Observations>> = Mutex::new(None);

/// 借出观察快照;尚无任何观察时返回默认值(只发生在首个驱动帧之前)。
fn observations_snapshot() -> Observations {
    OBSERVATIONS
        .lock()
        .expect("spike 自驱动观察锁中毒")
        .clone()
        .unwrap_or_default()
}

fn with_observations(f: impl FnOnce(&mut Observations)) {
    let mut guard = OBSERVATIONS.lock().expect("spike 自驱动观察锁中毒");
    f(guard.get_or_insert_with(Observations::default));
}

/// 记录一次下行相位;由 `ClientWindow::push_ui_state` 在每次下行推送时调用。
pub(crate) fn note_phase(wants_visible: bool) {
    if !enabled() {
        return;
    }
    with_observations(|obs| {
        if obs
            .phase_wants_visible
            .is_some_and(|previous| previous != wants_visible)
        {
            obs.phase_flips += 1;
        }
        obs.phase_wants_visible = Some(wants_visible);
    });
}

/// 记录一批到达桥的原始上行信封;由 `webview` 的脚本消息回调调用。
///
/// 只有自驱动档开启才解析(统计批内事件数);信封是否被接受由桥的入队
/// 返回值决定,这里只统计到达数,静默相位内的到达即穿透断言的异常信号。
pub(crate) fn note_uplink_envelope(bytes: &[u8]) {
    if !enabled() {
        return;
    }
    let events = serde_json::from_slice::<serde_json::Value>(bytes)
        .ok()
        .and_then(|value| {
            value
                .get("events")
                .and_then(|events| events.as_array())
                .cloned()
        })
        .map(|events| events.len())
        .unwrap_or(0);
    if events == 0 {
        return;
    }
    with_observations(|obs| {
        obs.uplink_total += events as u64;
        // 暂停覆盖层可见时,Esc 由页面侧键盘路由回传 pause-back,属合法上行;
        // 只有静默相位(WebView 不参与响应链)的上行才是 GameOverlay 穿透
        // 断言要抓的异常。
        if obs.phase_wants_visible == Some(false) {
            obs.uplink_in_quiet_phase += events as u64;
        }
    });
}

/// 记录一帧的 HUD 顶点流字节数;由渲染器的帧入口调用。
pub(crate) fn note_hud(bytes: usize) {
    if !enabled() {
        return;
    }
    with_observations(|obs| {
        let bytes = bytes as u64;
        if obs.hud_bytes != bytes {
            obs.hud_changes += 1;
        }
        obs.hud_bytes = bytes;
    });
}

/// 记录一帧是否绘制目标方块轮廓;与 [`note_hud`] 同一调用点。
pub(crate) fn note_outline(present: bool) {
    if !enabled() {
        return;
    }
    with_observations(|obs| {
        if obs.outline_present != present {
            obs.outline_flips += 1;
        }
        obs.outline_present = present;
    });
}

/// 每帧的静默相位观察计数;由驱动帧入口调用(非静默相位不计)。
fn note_quiet_frame(obs: &mut Observations) {
    if obs.phase_wants_visible == Some(false) {
        obs.quiet_frames += 1;
    }
}

// ---------------------------------------------------------------------------
// JavaScript 求值回执
// ---------------------------------------------------------------------------

/// 一条 `evaluateJavaScript` 回执。
#[derive(Debug, Clone)]
struct JsReply {
    /// 脚本是否成功求值且回执可读。
    ok: bool,
    /// 回执正文(脚本约定的 JSON 文本)或失败原因。
    body: String,
}

/// JavaScript 求值回执的收件箱:完成回调(主线程)写入,驱动帧读走。
static JS_INBOX: Mutex<Vec<JsReply>> = Mutex::new(Vec::new());

/// 完成回调写入回执。
fn push_js_reply(reply: JsReply) {
    JS_INBOX.lock().expect("spike 自驱动回执锁中毒").push(reply);
}

/// 取走最早的一条回执。
///
/// 驱动脚本严格串行,同一时刻至多一条求值在途,完成回调无法回填请求号
/// (WebKit 只回传结果对象),请求号由页面脚本在回执正文里回显,驱动侧据此
/// 校验回执归属。
fn take_js_reply() -> Option<JsReply> {
    let mut guard = JS_INBOX.lock().expect("spike 自驱动回执锁中毒");
    if guard.is_empty() {
        None
    } else {
        Some(guard.remove(0))
    }
}

/// 供 `webview` 侧构造 `evaluateJavaScript` 完成回调:把页面回执转写进
/// [`JS_INBOX`],由驱动帧消费。
pub(crate) fn js_completion_block() -> RcBlock<dyn Fn(*mut AnyObject, *mut NSError)> {
    RcBlock::new(move |result: *mut AnyObject, error: *mut NSError| {
        if !error.is_null() {
            // SAFETY: error 是 WebKit 传入的存活 NSError;localizedDescription
            // 返回 autoreleased NSString,本调用栈内立即拷贝为 String。
            let text = unsafe { read_error_description(error) };
            push_js_reply(JsReply {
                ok: false,
                body: format!("error: {text}"),
            });
            return;
        }
        if result.is_null() {
            push_js_reply(JsReply {
                ok: false,
                body: "nil".to_string(),
            });
            return;
        }
        // SAFETY: result 是 WebKit 传入的存活 NSString(脚本约定返回字符串)。
        let body = unsafe { read_utf8_string(result) };
        push_js_reply(JsReply { ok: true, body });
    })
}

/// 从 ObjC 字符串对象读 UTF-8 文本。
///
/// # Safety
///
/// `object` 必须是主线程上存续的有效对象;非 `NSString` 时退回
/// `description`,返回值在调用栈内立即拷贝,不持有 ObjC 借用。
unsafe fn read_utf8_string(object: *mut AnyObject) -> String {
    // SAFETY: isKindOfClass 只读消息,任意对象安全。
    let is_string: bool = unsafe { msg_send![object, isKindOfClass: class!(NSString)] };
    let value = if is_string {
        object
    } else {
        // 非字符串(脚本约定外)按 description 兜底,宁可留痕也不误判成功。
        // SAFETY: description 返回 autoreleased NSString,立即拷贝。
        let description: *mut AnyObject = unsafe { msg_send![object, description] };
        if description.is_null() {
            return String::new();
        }
        description
    };
    // SAFETY: value 是存活 NSString,UTF8String 指针在当前 autorelease pool
    // 内有效,立即拷贝。
    let bytes: *const std::ffi::c_char = unsafe { msg_send![value, UTF8String] };
    if bytes.is_null() {
        return String::new();
    }
    // SAFETY: 同上,指针非空且在 pool 生命周期内。
    unsafe { CStr::from_ptr(bytes) }
        .to_string_lossy()
        .into_owned()
}

/// 从 `NSError` 读 `localizedDescription`。
///
/// # Safety
///
/// `error` 必须是主线程上存续的有效 `NSError`。
unsafe fn read_error_description(error: *mut NSError) -> String {
    // SAFETY: localizedDescription 返回 autoreleased NSString,立即拷贝。
    let text: *mut AnyObject = unsafe { msg_send![error, localizedDescription] };
    if text.is_null() {
        return String::new();
    }
    // SAFETY: text 是存活 NSString。
    unsafe { read_utf8_string(text) }
}

/// 完成回调的 error 形参类型;objc2 绑定里为 `NSError`。
type NSError = objc2_foundation::NSError;

// ---------------------------------------------------------------------------
// 合成事件:构造 NSEvent 并投递到 NSApp 事件队列
// ---------------------------------------------------------------------------

// NSEventType 枚举值(AppKit 头文件)。objc2 面板未提供 NSEvent 构造器绑定,
// 经 `msg_send!` 走 `+keyEventWithType:`/`+mouseEventWithType:` 类方法,
// 参数类型与 objc2-app-kit 0.3 生成的同一选择器签名逐项对齐。
const EVENT_LEFT_MOUSE_DOWN: usize = 1;
const EVENT_LEFT_MOUSE_UP: usize = 2;
const EVENT_RIGHT_MOUSE_DOWN: usize = 3;
const EVENT_RIGHT_MOUSE_UP: usize = 4;
const EVENT_MOUSE_MOVED: usize = 5;
const EVENT_LEFT_MOUSE_DRAGGED: usize = 6;
const EVENT_KEY_DOWN: usize = 10;
const EVENT_KEY_UP: usize = 11;

/// 返回共享 `NSApplication`;AppKit 尚未初始化时为空。
fn ns_app() -> *mut AnyObject {
    // SAFETY: sharedApplication 在主线程调用(与全部 FFI 窗口出口一致)。
    let app: *mut AnyObject = unsafe { msg_send![class!(NSApplication), sharedApplication] };
    app
}

/// 构造一个键盘事件并投递到 NSApp 事件队列。
///
/// `vk` 是 macOS 虚拟键码(winit 以它还原 `PhysicalKey::Code`),`text` 是
/// 事件携带的字符(winit 侧 `event.text` 的来源,即聊天文本路径)。投递后
/// 事件在下一次 `pump_app_events` 中经 `sendEvent:` 进入 key window →
/// firstResponder(winit 视图)的 responder 链。
///
/// # Safety
///
/// 只能在主线程调用;事件对象在本调用栈内交给 NSApp 队列,不跨调用栈持有。
unsafe fn post_key_event(vk: u16, text: &str, down: bool, window_number: i64) -> bool {
    let app = ns_app();
    if app.is_null() {
        return false;
    }
    let chars = NSString::from_str(text);
    // SAFETY: 类方法构造 autoreleased NSEvent;参数类型与 AppKit 头文件 ABI
    // 一致(NSEventType/modifierFlags 为 NSUInteger、timestamp 为 f64、
    // windowNumber/eventNumber 为 NSInteger、isARepeat 为 BOOL、keyCode 为
    // c_ushort),receiver 是本进程唯一 NSApplication。
    let event: *mut AnyObject = unsafe {
        msg_send![
            class!(NSEvent),
            keyEventWithType: if down { EVENT_KEY_DOWN } else { EVENT_KEY_UP },
            location: NSPoint::new(0.0, 0.0),
            modifierFlags: 0usize,
            timestamp: 0.0f64,
            windowNumber: window_number,
            context: std::ptr::null_mut::<AnyObject>(),
            characters: &*chars,
            charactersIgnoringModifiers: &*chars,
            isARepeat: false,
            keyCode: vk,
        ]
    };
    if event.is_null() {
        return false;
    }
    // SAFETY: event 非空且在本调用栈内投递;NSApp 队列自行持有事件。
    let _: () = unsafe { msg_send![app, postEvent: event, atStart: false] };
    true
}

/// 构造一个鼠标事件并投递到 NSApp 事件队列。
///
/// `location` 是窗口坐标系(原点在 content 左下角,x 向右、y 向上)内的
/// 位置;`event_type` 由调用方按「是否有键按住」选择移动/拖拽类型,与真实
/// 硬件产生的事件类型保持一致,覆盖 winit 同一条分发分支。
///
/// # Safety
///
/// 只能在主线程调用;事件对象在本调用栈内交给 NSApp 队列,不跨调用栈持有。
unsafe fn post_mouse_event(
    event_type: usize,
    location: NSPoint,
    click_count: isize,
    window_number: i64,
) -> bool {
    let app = ns_app();
    if app.is_null() {
        return false;
    }
    // 按下事件必须带 1.0 压力:0 压力会被 AppKit 当作抬笔吞掉,实测右键
    // down 因此不进 responder 链;只有抬起事件才取 0。
    let pressure = if matches!(event_type, EVENT_LEFT_MOUSE_DOWN | EVENT_RIGHT_MOUSE_DOWN) {
        1.0f32
    } else {
        0.0f32
    };
    // SAFETY: 同 post_key_event。
    let event: *mut AnyObject = unsafe {
        msg_send![
            class!(NSEvent),
            mouseEventWithType: event_type,
            location: location,
            modifierFlags: 0usize,
            timestamp: 0.0f64,
            windowNumber: window_number,
            context: std::ptr::null_mut::<AnyObject>(),
            eventNumber: 0isize,
            clickCount: click_count,
            pressure: pressure,
        ]
    };
    if event.is_null() {
        return false;
    }
    // SAFETY: event 非空且在本调用栈内投递。
    let _: () = unsafe { msg_send![app, postEvent: event, atStart: false] };
    true
}

// ---------------------------------------------------------------------------
// 脚本:操作序列与断言
// ---------------------------------------------------------------------------

// 本脚本用到的 macOS 虚拟键码;[`go_bit`] 给出它们在 Go `client.Key` 位掩码
// 中的位序,位序与 [`crate::input::key_bit`] 的映射由测试互钉,断言用的位
// 掩码因此就是 Go 侧真实键位。
const VK_A: u16 = 0x00;
const VK_S: u16 = 0x01;
const VK_D: u16 = 0x02;
const VK_W: u16 = 0x0D;
const VK_E: u16 = 0x0E;
const VK_1: u16 = 0x12;
const VK_2: u16 = 0x13;
const VK_3: u16 = 0x14;
const VK_4: u16 = 0x15;
const VK_6: u16 = 0x16;
const VK_5: u16 = 0x17;
const VK_9: u16 = 0x19;
const VK_7: u16 = 0x1A;
const VK_8: u16 = 0x1C;
const VK_RETURN: u16 = 0x24;
const VK_ESCAPE: u16 = 0x35;

/// 数字键 1..9 的虚拟键码(macOS 键盘扫描码在中段不单调,需逐键枚举)。
const DIGIT_KEYS: [u16; 9] = [VK_1, VK_2, VK_3, VK_4, VK_5, VK_6, VK_7, VK_8, VK_9];

/// WASD 四向;`text` 与键位一致,让聊天文本路径同时被覆盖。
const WASD_KEYS: [(u16, &str); 4] = [(VK_W, "w"), (VK_A, "a"), (VK_S, "s"), (VK_D, "d")];

/// 聊天步注入的 ASCII 文本(纯 ASCII,IME 提交路径由既有测试覆盖)。
const CHAT_TEXT: &str = "spike-auto";

/// 虚拟键码 → Go `client.Key` 位序;与 [`crate::input::key_bit`] 的映射由
/// 测试互钉。
fn go_bit(vk: u16) -> Option<u32> {
    Some(match vk {
        VK_A => 1,
        VK_S => 2,
        VK_D => 3,
        VK_W => 0,
        VK_E => 17,
        VK_1 => 8,
        VK_2 => 9,
        VK_3 => 10,
        VK_4 => 11,
        VK_5 => 12,
        VK_6 => 13,
        VK_7 => 14,
        VK_8 => 15,
        VK_9 => 16,
        VK_RETURN => 26,
        VK_ESCAPE => 7,
        _ => return None,
    })
}

/// 数字键对应的 ASCII 字符。
fn digit_text(vk: u16) -> &'static str {
    match vk {
        VK_1 => "1",
        VK_2 => "2",
        VK_3 => "3",
        VK_4 => "4",
        VK_5 => "5",
        VK_6 => "6",
        VK_7 => "7",
        VK_8 => "8",
        VK_9 => "9",
        _ => "",
    }
}

/// ASCII 小写字母/连字符的虚拟键码;只覆盖脚本实际用到的字符,未知字符
/// 返回 `None`——`ascii_vk` 的完备性由测试钉住,脚本不会静默注入空文本。
fn ascii_vk(ch: char) -> Option<u16> {
    Some(match ch {
        'a' => 0x00,
        's' => 0x01,
        'd' => 0x02,
        'f' => 0x03,
        'h' => 0x04,
        'g' => 0x05,
        'z' => 0x06,
        'x' => 0x07,
        'c' => 0x08,
        'v' => 0x09,
        'b' => 0x0B,
        'q' => 0x0C,
        'w' => 0x0D,
        'e' => 0x0E,
        'r' => 0x0F,
        'y' => 0x10,
        't' => 0x11,
        'u' => 0x20,
        'i' => 0x22,
        'o' => 0x1F,
        'p' => 0x23,
        'l' => 0x25,
        'j' => 0x26,
        'k' => 0x28,
        'n' => 0x2D,
        'm' => 0x2E,
        '-' => 0x1B,
        _ => return None,
    })
}

/// 单条操作。脚本是一条平铺的操作序列,由 [`Driver`] 逐条驱动;断言类操作
/// 都以「最近一次 [`Op::Mark`] 以来」的增量解读,避免累计计数互相污染。
///
/// `Clone` 供驱动循环把当前操作从脚本里拷出来执行,避免 `&self.ops` 与
/// `&mut self`(断言记录)互相借用。
#[derive(Debug, Clone)]
enum Op {
    /// 记录断言基准(输入计数 + 观察)。
    Mark,
    /// 等待 `ms` 毫秒。
    Sleep { ms: u64 },
    /// 按下(`down`)或抬起一个键。
    Key {
        vk: u16,
        text: &'static str,
        down: bool,
    },
    /// 按下或抬起鼠标键;`primary` 为 true 表示左键。
    Mouse { primary: bool, down: bool },
    /// 以当前合成光标位置为基准再位移一次(窗口坐标,point)。
    MouseDelta { dx: f64, dy: f64 },
    /// 等待下行相位达到 `visible`(世界装配/暂停开合的信号);`soft` 为真时
    /// 超时只记为非门禁观察,不终止脚本(用于与 WebView 参与无关的应用
    /// 装配行为,三臂口径一致)。
    AwaitPhase {
        visible: bool,
        timeout_ms: u64,
        label: &'static str,
        soft: bool,
    },
    /// 经页面脚本按文本查找启用按钮并 `.click()`,等待页面回执;未命中按
    /// 250ms 重试(页面可能尚未渲染),直到 [`STEP_TIMEOUT_MS`]。
    ClickButton {
        text: &'static str,
        label: &'static str,
    },
    /// 断言自基准以来这些键位的按下与抬起都被 winit 观察到。
    ExpectKeys { mask: u64, label: &'static str },
    /// 断言自基准以来文本字符入队数不少于 `at_least`。
    ExpectChars { at_least: u64, label: &'static str },
    /// 断言自基准以来鼠标键的按下/抬起都被观察到(`None` 表示不断言该键)。
    ExpectMouse {
        primary: Option<bool>,
        secondary: Option<bool>,
        label: &'static str,
    },
    /// 断言自基准以来合成位移被 winit 观察到且累计幅值非零。
    ExpectMouseDelta { min_abs: f64, label: &'static str },
    /// 断言自基准以来 HUD 顶点流长度至少变化 `min_changes` 次。
    ExpectHudChange {
        min_changes: u64,
        label: &'static str,
    },
    /// 断言自基准以来静默相位内桥上行事件数为 0(GameOverlay 穿透断言)。
    ExpectUplinkQuiet,
    /// 记录目标方块轮廓观察(准星是否有可交互方块),不参与判据。
    ObserveOutline { label: &'static str },
    /// 记录 HUD 顶点流变化观察,不参与判据。
    ObserveHud { label: &'static str },
    /// 记录右键(副键)按下/抬起计数,不参与判据:AppKit 的
    /// `+mouseEventWithType:` 合成右键进不了 responder 链(见模块注释)。
    ObserveSecondary { label: &'static str },
    /// 开始一组 S2 测量:重置帧探针稳态窗口并丢弃预热帧。
    S2Begin { group: S2Group },
    /// 结束一组 S2 测量:读取环形窗口统计写入结果。
    S2End { group: S2Group },
    /// 脚本结束:汇总判据、落盘并退出。
    Finish,
}

/// S2 测量分组。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum S2Group {
    /// 空载:不注入任何鼠标事件。
    Idle,
    /// 持续采掘:左键按住 + 视角扫动 + 快捷栏切换。
    Mining,
}

impl S2Group {
    /// 组名(结果标注用)。
    fn label(self) -> &'static str {
        match self {
            S2Group::Idle => "idle",
            S2Group::Mining => "mining",
        }
    }

    /// 结果数组下标。
    fn index(self) -> usize {
        match self {
            S2Group::Idle => 0,
            S2Group::Mining => 1,
        }
    }
}

/// 一条断言/动作的记录。
#[derive(Debug, Clone)]
struct StepRecord {
    /// 断言名(人读)。
    label: String,
    /// 是否通过;动作类操作恒为 true,失败语义只来自断言与超时。
    passed: bool,
    /// 是否参与 S1 判据:与 WebView 参与无关的观察(装配时长、采样口径)
    /// 记为非门禁,失败照实展示但不推翻判据。
    gating: bool,
    /// 判读依据(计数、相位或页面回执)。
    detail: String,
}

/// 生成整条验证脚本。两条 S1 轮次内容一致,用于验证两态切换后行为不变;
/// 菜单两态回环放在所有测量之后,即便失败也不污染 S1/S2 数据。
fn plan_script() -> Vec<Op> {
    let mut ops = vec![
        // —— 自动进入游戏(真实 React onClick → 桥上行 → Go 装配)——
        Op::Mark,
        Op::ClickButton {
            text: "进入游戏",
            label: "menu:点击进入游戏",
        },
        Op::AwaitPhase {
            visible: false,
            timeout_ms: ASSEMBLY_TIMEOUT_MS,
            label: "assembly:进入游戏相位(世界装配完成)",
            soft: false,
        },
        Op::Sleep { ms: 1_500 },
        Op::Mark,
    ];
    // —— 瞄准:按住左键把视角压向地面,让后续采掘有可命中方块 ——
    // 按住期间真实硬件产生的是 LeftMouseDragged,合成路径保持同一类型;
    // 视角旋转走 winit 的 device-event 分支(NSApplication 级),不经过
    // WebView 的 hitTest,因此这条失败不推翻 hitTest 判据,记非门禁。
    ops.push(Op::Mouse {
        primary: true,
        down: true,
    });
    ops.push(Op::Sleep { ms: 60 });
    for _ in 0..12 {
        ops.push(Op::MouseDelta { dx: 0.0, dy: 6.0 });
        ops.push(Op::Sleep { ms: 25 });
    }
    ops.push(Op::Mouse {
        primary: true,
        down: false,
    });
    ops.push(Op::Sleep { ms: 200 });
    ops.push(Op::ExpectMouseDelta {
        min_abs: 30.0,
        label: "aim:合成拖拽位移被 winit 观察(device-event 分支)",
    });
    ops.push(Op::Sleep { ms: 400 });

    // —— S1 两轮 ——
    ops.extend(s1_round(1));
    ops.extend(s1_round(2));

    // —— S2 空载组:不注入任何鼠标事件 ——
    ops.push(Op::Mark);
    ops.push(Op::S2Begin {
        group: S2Group::Idle,
    });
    ops.push(Op::Sleep { ms: S2_WINDOW_MS });
    ops.push(Op::S2End {
        group: S2Group::Idle,
    });

    // —— S2 持续采掘组:左键按住 + 视角扫动 + 快捷栏切换 ——
    ops.push(Op::Mark);
    ops.push(Op::S2Begin {
        group: S2Group::Mining,
    });
    ops.push(Op::Mouse {
        primary: true,
        down: true,
    });
    // 每 120ms 扫动一次视角(横向交替 + 纵向缓慢下压),让准星反复掠过
    // 地形;按固定节奏切一次快捷栏制造 HUD 动画。
    let sweeps = (S2_WINDOW_MS / SWEEP_STEP_MS) as usize;
    let cycle_step = (sweeps / (S2_WINDOW_MS / HOTBAR_CYCLE_MS).max(1) as usize).max(1);
    for step in 0..sweeps {
        let yaw = if step % 2 == 0 { 12.0 } else { -12.0 };
        let pitch = if step % 8 == 0 { 3.0 } else { 0.0 };
        ops.push(Op::MouseDelta { dx: yaw, dy: pitch });
        ops.push(Op::Sleep { ms: SWEEP_STEP_MS });
        if step % cycle_step == cycle_step - 1 {
            let digit = DIGIT_KEYS[(step / cycle_step) % DIGIT_KEYS.len()];
            ops.push(Op::Key {
                vk: digit,
                text: digit_text(digit),
                down: true,
            });
            ops.push(Op::Sleep { ms: 40 });
            ops.push(Op::Key {
                vk: digit,
                text: digit_text(digit),
                down: false,
            });
            ops.push(Op::Sleep { ms: 40 });
        }
    }
    ops.push(Op::Mouse {
        primary: true,
        down: false,
    });
    // 抬起事件要等下一次事件泵才会进入计数,先等一拍再取数与断言。
    ops.push(Op::Sleep { ms: 300 });
    ops.push(Op::S2End {
        group: S2Group::Mining,
    });
    ops.push(Op::ExpectMouse {
        primary: Some(true),
        secondary: None,
        label: "s2:左键按住/抬起到达 winit",
    });
    ops.push(Op::ObserveHud {
        label: "s2:HUD 顶点流随采掘/快捷栏变化",
    });
    ops.push(Op::ExpectUplinkQuiet);

    // —— 菜单两态回环(暂停 → 退回主菜单 → 再次进入游戏)——
    ops.push(Op::Mark);
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: true,
    });
    ops.push(Op::Sleep { ms: 80 });
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: false,
    });
    ops.push(Op::AwaitPhase {
        visible: true,
        timeout_ms: 10_000,
        label: "pause:暂停覆盖层打开",
        soft: false,
    });
    ops.push(Op::Sleep { ms: 500 });
    ops.push(Op::ClickButton {
        text: "退回主菜单",
        label: "menu:点击退回主菜单",
    });
    ops.push(Op::AwaitPhase {
        visible: true,
        timeout_ms: 30_000,
        label: "menu:回到主菜单",
        soft: false,
    });
    ops.push(Op::Sleep { ms: 1_000 });
    ops.push(Op::ClickButton {
        text: "进入游戏",
        label: "menu:再次点击进入游戏",
    });
    ops.push(Op::AwaitPhase {
        visible: false,
        timeout_ms: ASSEMBLY_TIMEOUT_MS,
        label: "assembly:再次进入游戏相位(应用装配行为,非 WebView 参与)",
        soft: true,
    });
    ops.push(Op::Sleep { ms: 1_000 });
    ops.push(Op::ExpectUplinkQuiet);

    ops.push(Op::Finish);
    ops
}

/// 一轮 S1 输入序列:移动/采掘放置/快捷栏/聊天/暂停/背包。
///
/// 断言标签需要 `'static` 以保持 [`Op`] 字段简洁;标签随脚本常驻进程,
/// 与脚本同生命周期,不存在悬挂。
fn s1_round(round: u8) -> Vec<Op> {
    let label = move |name: &str| -> &'static str {
        Box::leak(format!("r{round}:{name}").into_boxed_str())
    };
    let mut ops = Vec::new();

    // 1) WASD 按住/释放。
    ops.push(Op::Mark);
    for (vk, text) in WASD_KEYS {
        ops.push(Op::Key {
            vk,
            text,
            down: true,
        });
        ops.push(Op::Sleep { ms: 150 });
        ops.push(Op::Key {
            vk,
            text,
            down: false,
        });
        ops.push(Op::Sleep { ms: 100 });
    }
    ops.push(Op::ExpectKeys {
        mask: key_mask(&WASD_KEYS.iter().map(|(vk, _)| *vk).collect::<Vec<_>>()),
        label: label("wasd:四向按下与抬起都被 winit 观察"),
    });

    // 2) 左键按住采掘 600ms + 右键单击放置。
    ops.push(Op::Mark);
    ops.push(Op::Mouse {
        primary: true,
        down: true,
    });
    ops.push(Op::Sleep { ms: 600 });
    ops.push(Op::Mouse {
        primary: true,
        down: false,
    });
    ops.push(Op::Sleep { ms: 200 });
    ops.push(Op::Mouse {
        primary: false,
        down: true,
    });
    ops.push(Op::Sleep { ms: 80 });
    ops.push(Op::Mouse {
        primary: false,
        down: false,
    });
    ops.push(Op::Sleep { ms: 200 });
    ops.push(Op::ExpectMouse {
        primary: Some(true),
        secondary: None,
        label: label("mining:左键按住采掘到达 winit"),
    });
    ops.push(Op::ObserveSecondary {
        label: label("mining:右键放置到达 winit"),
    });
    ops.push(Op::ObserveOutline {
        label: label("mining:准星前有可采掘目标(轮廓观察)"),
    });

    // 3) 数字键 1..9 切换快捷栏(滚轮不在输入快照契约内,口径见报告)。
    ops.push(Op::Mark);
    for vk in DIGIT_KEYS {
        ops.push(Op::Key {
            vk,
            text: digit_text(vk),
            down: true,
        });
        ops.push(Op::Sleep { ms: 120 });
        ops.push(Op::Key {
            vk,
            text: digit_text(vk),
            down: false,
        });
        ops.push(Op::Sleep { ms: 120 });
    }
    ops.push(Op::ExpectKeys {
        mask: key_mask(DIGIT_KEYS.as_ref()),
        label: label("hotbar:数字键 1..9 切换快捷栏"),
    });

    // 4) Enter 进入聊天 → ASCII 输入 → Enter 发送。
    ops.push(Op::Mark);
    ops.push(Op::Key {
        vk: VK_RETURN,
        text: "\r",
        down: true,
    });
    ops.push(Op::Sleep { ms: 60 });
    ops.push(Op::Key {
        vk: VK_RETURN,
        text: "\r",
        down: false,
    });
    ops.push(Op::Sleep { ms: 300 });
    for ch in CHAT_TEXT.chars() {
        // `ascii_vk` 对 `CHAT_TEXT` 的完备性由测试钉住;未知字符跳过,由
        // `ExpectChars` 的字符计数断言暴露,而不是注入虚拟键码 0。
        let Some(vk) = ascii_vk(ch) else { continue };
        let text: &'static str = Box::leak(ch.to_string().into_boxed_str());
        ops.push(Op::Key {
            vk,
            text,
            down: true,
        });
        ops.push(Op::Sleep { ms: 40 });
        ops.push(Op::Key {
            vk,
            text,
            down: false,
        });
        ops.push(Op::Sleep { ms: 40 });
    }
    ops.push(Op::Sleep { ms: 200 });
    ops.push(Op::Key {
        vk: VK_RETURN,
        text: "\r",
        down: true,
    });
    ops.push(Op::Sleep { ms: 60 });
    ops.push(Op::Key {
        vk: VK_RETURN,
        text: "\r",
        down: false,
    });
    ops.push(Op::Sleep { ms: 300 });
    ops.push(Op::ExpectKeys {
        mask: key_mask(&[VK_RETURN]),
        label: label("chat:Enter 开合聊天输入"),
    });
    ops.push(Op::ExpectChars {
        at_least: CHAT_TEXT.chars().count() as u64,
        label: label("chat:ASCII 文本入队完整"),
    });

    // 5) Esc 暂停 → 再 Esc 恢复(以暂停覆盖层可见性为玩法层证据)。
    ops.push(Op::Mark);
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: true,
    });
    ops.push(Op::Sleep { ms: 80 });
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: false,
    });
    ops.push(Op::AwaitPhase {
        visible: true,
        timeout_ms: 10_000,
        label: label("pause:暂停覆盖层打开"),
        soft: false,
    });
    ops.push(Op::Sleep { ms: 500 });
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: true,
    });
    ops.push(Op::Sleep { ms: 80 });
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: false,
    });
    ops.push(Op::AwaitPhase {
        visible: false,
        timeout_ms: 10_000,
        label: label("pause:暂停覆盖层恢复"),
        soft: false,
    });
    ops.push(Op::Sleep { ms: 400 });
    ops.push(Op::ExpectKeys {
        mask: key_mask(&[VK_ESCAPE]),
        label: label("pause:Esc 到达 winit(开层一侧)"),
    });
    ops.push(Op::ExpectUplinkQuiet);

    // 6) E 打开背包 → 合成单击容器格 → Esc 关闭。
    ops.push(Op::Mark);
    ops.push(Op::Key {
        vk: VK_E,
        text: "e",
        down: true,
    });
    ops.push(Op::Sleep { ms: 60 });
    ops.push(Op::Key {
        vk: VK_E,
        text: "e",
        down: false,
    });
    ops.push(Op::Sleep { ms: 500 });
    ops.push(Op::Mouse {
        primary: true,
        down: true,
    });
    ops.push(Op::Sleep { ms: 60 });
    ops.push(Op::Mouse {
        primary: true,
        down: false,
    });
    ops.push(Op::Sleep { ms: 500 });
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: true,
    });
    ops.push(Op::Sleep { ms: 60 });
    ops.push(Op::Key {
        vk: VK_ESCAPE,
        text: "\u{1b}",
        down: false,
    });
    ops.push(Op::Sleep { ms: 500 });
    ops.push(Op::ExpectKeys {
        mask: key_mask(&[VK_E, VK_ESCAPE]),
        label: label("container:E 开背包与 Esc 关闭都到达 winit"),
    });
    ops.push(Op::ExpectMouse {
        primary: Some(true),
        secondary: None,
        label: label("container:容器格合成单击到达 winit"),
    });
    ops.push(Op::ExpectHudChange {
        min_changes: 1,
        label: label("container:容器面板开合改变 HUD 顶点流"),
    });
    ops.push(Op::Sleep { ms: 300 });
    ops
}

/// 把一组虚拟键码折叠成 Go `client.Key` 位掩码。
fn key_mask(vks: &[u16]) -> u64 {
    vks.iter()
        .filter_map(|vk| go_bit(*vk))
        .fold(0u64, |acc, bit| acc | (1 << bit))
}

/// 生成「按文本查找启用按钮并 `.click()`」的页面脚本。
///
/// 选择器按精确 → 容器内回退的顺序尝试:先在 `.menu-button` 里找文本完全
/// 一致的启用项,未命中再在 `.menu-buttons` 容器内做「包含关键字的启用项」
/// 模糊匹配,最后退到容器内首个启用按钮;命中与选择路径都写进回执,便于
/// 如实记录。回执是 JSON 文本,`id` 用于驱动侧丢弃过期回执。
fn click_button_script(id: u32, text: &str) -> String {
    format!(
        "(function(){{var id={id};var want={text:?};var buttons=Array.prototype.slice.call(\
         document.querySelectorAll('.menu-button'));function enabled(b){{return b&&!b.disabled;}}\
         function text(b){{return (b.textContent||'').trim();}}\
         var hit=buttons.filter(function(b){{return enabled(b)&&text(b)===want;}})[0];\
         var selector=hit?'.menu-button':'';\
         if(!hit){{var wrap=document.querySelector('.menu-buttons');\
         if(wrap){{var all=Array.prototype.slice.call(wrap.querySelectorAll('button'));\
         hit=all.filter(function(b){{return enabled(b)&&(text(b).indexOf(want)>=0);}})[0];\
         if(!hit){{hit=all.filter(enabled)[0];}}\
         if(hit){{selector='fuzzy:.menu-buttons';}}}}}}\
         if(!hit){{return JSON.stringify({{ok:false,id:id,selector:'',count:buttons.length}});}}\
         var r=hit.getBoundingClientRect();hit.click();\
         return JSON.stringify({{ok:true,id:id,selector:selector,text:text(hit),\
         count:buttons.length,x:r.left+r.width/2,y:r.top+r.height/2,\
         vw:window.innerWidth,vh:window.innerHeight}});}})()",
        id = id,
        text = text
    )
}

// ---------------------------------------------------------------------------
// 驱动器
// ---------------------------------------------------------------------------

/// 一次驱动帧需要的窗口/WebView 能力;由 `ClientWindow::poll` 组装。
pub(crate) struct FrameContext<'a> {
    /// 已挂载的菜单 WebView(尚未挂载时为 `None`,脚本会等待)。
    pub webview: Option<&'a MenuWebview>,
    /// NSWindow `windowNumber`(合成事件的归属窗口)。
    pub window_number: i64,
    /// content 尺寸(逻辑点)。
    pub content_width: f64,
    /// content 高度(逻辑点)。
    pub content_height: f64,
    /// 输入计数快照(累计值)。
    pub taps: InputTaps,
}

/// 单条操作的一次执行结果。
enum Flow {
    /// 立即推进到下一条。
    Advance,
    /// 等到指定时刻后**推进到下一条**(`Sleep` 的语义:等待本身即完成)。
    SleepUntil(Instant),
    /// 等到指定时刻后**重新执行本条**(`ClickButton` 的回执轮询节流)。
    RetryAt(Instant),
    /// 反复探测直到条件满足;`Instant` 是超时点,超时按卡点落盘退出。
    Await(Instant),
}

/// 在途定时等待的语义:到点后是推进还是重跑当前操作。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum WaitKind {
    Advance,
    Retry,
}

/// 在途的按钮点击:已注入脚本,等待页面回执。
struct PendingClick {
    /// 本次注入对应的请求号。
    request: u32,
    /// 注入时刻;重试共用同一超时点。
    started: Instant,
}

/// 脚本驱动器:操作游标、断言基准、合成光标位置与已收集的测量结果。
struct Driver {
    /// 脚本起点(总时长兜底)。
    started: Instant,
    ops: Vec<Op>,
    pc: usize,
    /// 当前操作的定时等待(语义 + 到点时刻);`None` 表示立即可执行。
    wait: Option<(WaitKind, Instant)>,
    /// 当前 Await 的超时点;离开当前操作时清空。
    await_deadline: Option<Instant>,
    /// 当前 Await 超时是否只记录不退出(非门禁观察)。
    await_soft: bool,
    /// 断言基准:输入计数。
    base_taps: InputTaps,
    /// 断言基准:观察值。
    base_obs: Observations,
    /// 合成光标的窗口坐标(point,原点在 content 左下角)。
    cursor_x: f64,
    cursor_y: f64,
    /// 是否已把合成光标放到窗口中心。
    cursor_centered: bool,
    /// 左键当前是否按住(决定位移事件的类型)。
    left_down: bool,
    /// 在途按钮点击。
    pending_click: Option<PendingClick>,
    /// 下一个回执请求号。
    next_request_id: u32,
    /// 已执行的 S1 断言记录。
    s1_records: Vec<StepRecord>,
    /// S2 各组测量结果;`None` 表示该组尚未完成。
    s2: [Option<WindowStats>; 2],
    /// 是否出现过失败断言。
    any_failed: bool,
}

impl Driver {
    fn new() -> Self {
        let ops = plan_script();
        eprintln!(
            "mornlea spike auto: 自驱动档开启 arm={} 共 {} 条操作;窗口即将出现并接收合成输入,期间请勿触碰键鼠",
            arm_tag(),
            ops.len()
        );
        Self {
            started: Instant::now(),
            ops,
            pc: 0,
            wait: None,
            await_deadline: None,
            await_soft: false,
            base_taps: InputTaps::default(),
            base_obs: Observations::default(),
            cursor_x: 0.0,
            cursor_y: 0.0,
            cursor_centered: false,
            left_down: false,
            pending_click: None,
            next_request_id: 1,
            s1_records: Vec::new(),
            s2: [None, None],
            any_failed: false,
        }
    }

    /// 记录一条门禁断言结果。
    fn record(&mut self, label: &str, passed: bool, detail: String) {
        if !passed {
            self.any_failed = true;
        }
        self.push_record(label, passed, true, detail);
    }

    /// 记录一条非门禁观察:照实展示,不参与判据与退出码。
    fn record_note(&mut self, label: &str, passed: bool, detail: String) {
        self.push_record(label, passed, false, detail);
    }

    fn push_record(&mut self, label: &str, passed: bool, gating: bool, detail: String) {
        eprintln!(
            "mornlea spike auto: {}{} {} — {detail}",
            if passed { "PASS" } else { "FAIL" },
            if gating { "" } else { "(非门禁)" },
            label
        );
        self.s1_records.push(StepRecord {
            label: label.to_string(),
            passed,
            gating,
            detail,
        });
    }

    /// 驱动一帧。
    fn on_frame(&mut self, ctx: &FrameContext<'_>) {
        let now = Instant::now();
        // 合成光标从窗口中心起算:窗口边缘的点击落在 AppKit 的 resize/
        // chrome 区域,不会进入内容视图,不能作为 hitTest 证据。
        if !self.cursor_centered && ctx.content_width > 1.0 {
            self.cursor_x = ctx.content_width / 2.0;
            self.cursor_y = ctx.content_height / 2.0;
            self.cursor_centered = true;
        }
        // 静默相位观察:穿透断言的分母(观察了多少帧游戏相位)。
        with_observations(note_quiet_frame);
        // 总时长兜底。
        if now.duration_since(self.started) > Duration::from_millis(TOTAL_TIMEOUT_MS) {
            self.finish(ctx, "总时长超限");
        }
        // 当前 Await 超时:按卡点落盘部分结果并退出。
        if let Some(deadline) = self.await_deadline
            && now >= deadline
        {
            let label = self.ops.get(self.pc).map_or("<unknown>", op_label);
            let detail = format!("等待超时,当前操作:{label}");
            if self.await_soft {
                self.record_note(label, false, detail);
                self.pc += 1;
                self.await_deadline = None;
                self.await_soft = false;
                return;
            }
            self.record(label, false, detail);
            self.finish(ctx, label);
        }
        // 定时等待:未到点本帧不做任何事;到点后按语义推进或重跑当前操作。
        if let Some((kind, at)) = self.wait {
            if now < at {
                return;
            }
            self.wait = None;
            if kind == WaitKind::Advance {
                // 等待本身就是这条操作的完成,推进到下一条;下一帧继续。
                self.pc += 1;
                self.await_deadline = None;
                return;
            }
        }
        for _ in 0..MAX_OPS_PER_FRAME {
            let Some(op) = self.ops.get(self.pc).cloned() else {
                self.finish(ctx, "脚本到达末尾");
            };
            match self.execute(&op, ctx, now) {
                Flow::Advance => {
                    self.pc += 1;
                    self.await_deadline = None;
                }
                Flow::SleepUntil(at) => {
                    self.wait = Some((WaitKind::Advance, at));
                    return;
                }
                Flow::RetryAt(at) => {
                    self.wait = Some((WaitKind::Retry, at));
                    return;
                }
                Flow::Await(deadline) => {
                    // 超时点在操作开始时确立,不得被后续帧顺延,否则条件
                    // 永不满足时脚本会无限等待。
                    if self.await_deadline.is_none() {
                        self.await_deadline = Some(deadline);
                    }
                    return;
                }
            }
        }
    }

    /// 执行单条操作。
    fn execute(&mut self, op: &Op, ctx: &FrameContext<'_>, now: Instant) -> Flow {
        match op {
            Op::Mark => {
                self.base_taps = ctx.taps;
                self.base_obs = observations_snapshot();
                Flow::Advance
            }
            Op::Sleep { ms } => Flow::SleepUntil(now + Duration::from_millis(*ms)),
            Op::Key { vk, text, down } => {
                // SAFETY: 主线程(FFI 窗口出口约束);事件立即投递。
                let posted = unsafe { post_key_event(*vk, text, *down, ctx.window_number) };
                if !posted {
                    self.record("post:key", false, "NSEvent 构造或投递失败".to_string());
                }
                Flow::Advance
            }
            Op::Mouse { primary, down } => {
                if *primary {
                    self.left_down = *down;
                }
                let event_type = match (*primary, *down) {
                    (true, true) => EVENT_LEFT_MOUSE_DOWN,
                    (true, false) => EVENT_LEFT_MOUSE_UP,
                    (false, true) => EVENT_RIGHT_MOUSE_DOWN,
                    (false, false) => EVENT_RIGHT_MOUSE_UP,
                };
                // SAFETY: 主线程;事件立即投递。
                let posted = unsafe {
                    post_mouse_event(
                        event_type,
                        NSPoint::new(self.cursor_x, self.cursor_y),
                        1,
                        ctx.window_number,
                    )
                };
                if !posted {
                    self.record("post:mouse", false, "NSEvent 构造或投递失败".to_string());
                }
                Flow::Advance
            }
            Op::MouseDelta { dx, dy } => {
                self.cursor_x += dx;
                self.cursor_y += dy;
                // 按住左键期间真实硬件产生的是 LeftMouseDragged;其余走
                // MouseMoved,覆盖 winit 的同一条 device-event 分发分支。
                let event_type = if self.left_down {
                    EVENT_LEFT_MOUSE_DRAGGED
                } else {
                    EVENT_MOUSE_MOVED
                };
                // SAFETY: 主线程;事件立即投递。
                let posted = unsafe {
                    post_mouse_event(
                        event_type,
                        NSPoint::new(self.cursor_x, self.cursor_y),
                        0,
                        ctx.window_number,
                    )
                };
                if !posted {
                    self.record(
                        "post:mouse-delta",
                        false,
                        "NSEvent 构造或投递失败".to_string(),
                    );
                }
                Flow::Advance
            }
            Op::AwaitPhase {
                visible,
                timeout_ms,
                label,
                soft,
            } => {
                let obs = observations_snapshot();
                if obs.phase_wants_visible == Some(*visible) {
                    self.record(label, true, format!("相位已达 wants_visible={visible}"));
                    return Flow::Advance;
                }
                self.await_soft = *soft;
                Flow::Await(now + Duration::from_millis(*timeout_ms))
            }
            Op::ClickButton { text, label } => self.click_button(ctx, text, label, now),
            Op::ExpectKeys { mask, label } => {
                let delta = ctx.taps.delta_since(self.base_taps);
                // 掩码取累计值(重复按键在第二轮的增量掩码恒为 0),事件数取
                // 增量:前者证明「按的是这些键」,后者证明「本步骤内确实发生」。
                let taps = &ctx.taps;
                let missing_down = mask & !taps.key_down_mask;
                let missing_up = mask & !taps.key_up_mask;
                let passed = missing_down == 0 && missing_up == 0;
                self.record(
                    label,
                    passed,
                    format!(
                        "expect={mask:#x} down_mask={:#x} up_mask={:#x} events(down=+{},up=+{})",
                        taps.key_down_mask,
                        taps.key_up_mask,
                        delta.key_down_events,
                        delta.key_up_events
                    ),
                );
                Flow::Advance
            }
            Op::ExpectChars { at_least, label } => {
                let delta = ctx.taps.delta_since(self.base_taps);
                self.record(
                    label,
                    delta.chars >= *at_least,
                    format!("chars={} expect>={at_least}", delta.chars),
                );
                Flow::Advance
            }
            Op::ExpectMouse {
                primary,
                secondary,
                label,
            } => {
                let delta = ctx.taps.delta_since(self.base_taps);
                let mut detail = Vec::new();
                let mut passed = true;
                for (name, want, press, release, total) in [
                    (
                        "primary",
                        *primary,
                        delta.primary_press,
                        delta.primary_release,
                        ctx.taps.primary_press,
                    ),
                    (
                        "secondary",
                        *secondary,
                        delta.secondary_press,
                        delta.secondary_release,
                        ctx.taps.secondary_press,
                    ),
                ] {
                    let Some(want) = want else { continue };
                    detail.push(format!(
                        "{name} press=+{press} release=+{release} (total={total})"
                    ));
                    if want && (press == 0 || release == 0) {
                        passed = false;
                    }
                }
                self.record(label, passed, detail.join(", "));
                Flow::Advance
            }
            Op::ExpectMouseDelta { min_abs, label } => {
                let delta = ctx.taps.delta_since(self.base_taps);
                let abs = delta.mouse_delta_x.abs() + delta.mouse_delta_y.abs();
                self.record_note(
                    label,
                    delta.mouse_delta_events > 0 && abs >= *min_abs,
                    format!(
                        "delta_events={} |dx|+|dy|={abs:.1} expect_events>0 expect_abs>={min_abs} \
                         (非门禁:device-event 在 NSApplication 级分发,不经过 WebView hitTest)",
                        delta.mouse_delta_events
                    ),
                );
                Flow::Advance
            }
            Op::ExpectHudChange { min_changes, label } => {
                let obs = observations_snapshot();
                let changes = obs.hud_changes.saturating_sub(self.base_obs.hud_changes);
                self.record(
                    label,
                    changes >= *min_changes,
                    format!("hud_changes={changes} hud_bytes={}", obs.hud_bytes),
                );
                Flow::Advance
            }
            Op::ObserveOutline { label } => {
                let obs = observations_snapshot();
                let flips = obs
                    .outline_flips
                    .saturating_sub(self.base_obs.outline_flips);
                self.record_note(
                    label,
                    flips > 0,
                    format!(
                        "outline_flips=+{flips} present={} (非门禁:合成位移无法驱动视角时仅作佐证)",
                        obs.outline_present
                    ),
                );
                Flow::Advance
            }
            Op::ObserveSecondary { label } => {
                let delta = ctx.taps.delta_since(self.base_taps);
                self.record_note(
                    label,
                    delta.secondary_press > 0 && delta.secondary_release > 0,
                    format!(
                        "secondary press=+{} release=+{} (total={}) (非门禁:合成右键未进入 responder 链,三臂一致)",
                        delta.secondary_press,
                        delta.secondary_release,
                        ctx.taps.secondary_press
                    ),
                );
                Flow::Advance
            }
            Op::ObserveHud { label } => {
                let obs = observations_snapshot();
                let changes = obs.hud_changes.saturating_sub(self.base_obs.hud_changes);
                self.record_note(
                    label,
                    changes > 0,
                    format!(
                        "hud_changes=+{changes} hud_bytes={} (非门禁:采掘条与弹条入流受装配进度影响)",
                        obs.hud_bytes
                    ),
                );
                Flow::Advance
            }
            Op::ExpectUplinkQuiet => {
                let obs = observations_snapshot();
                let leaked = obs
                    .uplink_in_quiet_phase
                    .saturating_sub(self.base_obs.uplink_in_quiet_phase);
                self.record(
                    "uplink:静默相位内桥上行事件为 0",
                    leaked == 0,
                    format!(
                        "leaked={leaked} quiet_frames={} uplink_total={}",
                        obs.quiet_frames.saturating_sub(self.base_obs.quiet_frames),
                        obs.uplink_total
                    ),
                );
                Flow::Advance
            }
            Op::S2Begin { group } => match crate::overlay_spike::frame_probe() {
                Some(probe) => {
                    probe.begin_steady_window();
                    Flow::Advance
                }
                None => {
                    self.record(
                        group.label(),
                        false,
                        "帧探针未开启(MORNLEA_SPIKE_FPS 未设置),该组无 S2 数据".to_string(),
                    );
                    Flow::Advance
                }
            },
            Op::S2End { group } => {
                let stats =
                    crate::overlay_spike::frame_probe().and_then(|probe| probe.window_stats());
                match stats {
                    Some(stats) => {
                        eprintln!(
                            "mornlea spike auto: S2[{}] frames={} frame_us(mean={} p95={}) \
                             interval_us(mean={} p95={})",
                            group.label(),
                            stats.samples,
                            stats.frame_us.mean,
                            stats.frame_us.p95,
                            stats.interval_us.mean,
                            stats.interval_us.p95
                        );
                        self.s2[group.index()] = Some(stats);
                    }
                    None => self.record(
                        group.label(),
                        false,
                        "S2 稳态窗口样本不足(帧探针未开启或采样过少)".to_string(),
                    ),
                }
                Flow::Advance
            }
            Op::Finish => {
                self.finish(ctx, "脚本完成");
            }
        }
    }

    /// 注入「按文本查找启用按钮并 `.click()`」的脚本并等待页面回执。
    ///
    /// 页面可能尚未渲染(挂载后首帧),`{ok:false}` 与无回执都按 250ms 重试;
    /// 超时按卡点退出。
    fn click_button(
        &mut self,
        ctx: &FrameContext<'_>,
        text: &'static str,
        label: &'static str,
        now: Instant,
    ) -> Flow {
        let Some(webview) = ctx.webview else {
            // WebView 尚未挂载:等它出现,超时按卡点退出。
            self.pending_click = None;
            return Flow::Await(now + Duration::from_millis(STEP_TIMEOUT_MS));
        };
        // 已在途:查回执。
        if let Some(pending) = &self.pending_click {
            let request = pending.request;
            let started = pending.started;
            match take_js_reply() {
                None => return Flow::RetryAt(now + Duration::from_millis(250)),
                Some(reply) => {
                    self.pending_click = None;
                    if !reply.ok {
                        // 求值失败(页面未就绪等):重试,共用同一超时点。
                        if now >= started + Duration::from_millis(STEP_TIMEOUT_MS) {
                            self.record(label, false, format!("页面求值失败: {}", reply.body));
                            self.finish(ctx, label);
                        }
                        eprintln!("mornlea spike auto: 求值失败,250ms 后重试({})", reply.body);
                        return Flow::RetryAt(now + Duration::from_millis(250));
                    }
                    // 回执归属校验:正文里的请求号不匹配说明是过期回执,
                    // 丢弃后继续等本次请求。
                    let echoed = serde_json::from_str::<serde_json::Value>(&reply.body)
                        .ok()
                        .and_then(|value| value.get("id").and_then(|id| id.as_u64()))
                        .map(|id| id as u32);
                    if echoed != Some(request) {
                        if now >= started + Duration::from_millis(STEP_TIMEOUT_MS) {
                            self.record(
                                label,
                                false,
                                format!(
                                    "回执请求号不符(期望 {request},收到 {echoed:?}): {}",
                                    reply.body
                                ),
                            );
                            self.finish(ctx, label);
                        }
                        eprintln!(
                            "mornlea spike auto: 回执请求号不符(期望 {request},收到 {echoed:?}),丢弃"
                        );
                        return Flow::RetryAt(now + Duration::from_millis(250));
                    }
                    match serde_json::from_str::<serde_json::Value>(&reply.body) {
                        Ok(value) if value.get("ok") == Some(&serde_json::json!(true)) => {
                            self.record(
                                label,
                                true,
                                format!(
                                    "selector={} text={} buttons={} viewport={}x{}",
                                    value
                                        .get("selector")
                                        .and_then(|v| v.as_str())
                                        .unwrap_or("?"),
                                    value.get("text").and_then(|v| v.as_str()).unwrap_or("?"),
                                    value.get("count").and_then(|v| v.as_u64()).unwrap_or(0),
                                    value.get("vw").and_then(|v| v.as_u64()).unwrap_or(0),
                                    value.get("vh").and_then(|v| v.as_u64()).unwrap_or(0)
                                ),
                            );
                            return Flow::Advance;
                        }
                        Ok(value) => {
                            if now >= started + Duration::from_millis(STEP_TIMEOUT_MS) {
                                self.record(
                                    label,
                                    false,
                                    format!("按钮未命中(自动进入游戏失败): {value}"),
                                );
                                self.finish(ctx, label);
                            }
                            eprintln!("mornlea spike auto: 按钮未命中,250ms 后重试 page={value}");
                            return Flow::RetryAt(now + Duration::from_millis(250));
                        }
                        Err(error) => {
                            self.record(
                                label,
                                false,
                                format!("回执不是 JSON: {error} body={}", reply.body),
                            );
                            return Flow::Advance;
                        }
                    }
                }
            }
        }
        // 首次注入。
        let request = self.next_request_id;
        self.next_request_id += 1;
        let script = click_button_script(request, text);
        eprintln!("mornlea spike auto: 注入按钮点击脚本 request={request} text={text}");
        webview.spike_evaluate(&script);
        self.pending_click = Some(PendingClick {
            request,
            started: now,
        });
        Flow::RetryAt(now + Duration::from_millis(250))
    }

    /// 汇总判据、写结果文件并退出进程。
    fn finish(&mut self, ctx: &FrameContext<'_>, reason: &str) -> ! {
        let summary = build_summary(self, ctx, reason);
        eprintln!(
            "mornlea spike auto: 结束 arm={} reason={} s1 通过 {}/{} S2 完成 {}/2 上行泄漏 {}",
            summary.arm,
            reason,
            summary.s1_passed,
            summary.s1_total,
            summary.s2_done,
            summary.uplink_in_quiet_phase
        );
        write_results(&summary);
        let code = if summary.any_failed { 3 } else { 0 };
        // 先落盘再退出;spike 进程的退出路径不需要优雅收尾。
        std::process::exit(code);
    }
}

/// 当前操作的标签(超时报告用)。
fn op_label(op: &Op) -> &'static str {
    match op {
        Op::Mark => "mark",
        Op::Sleep { .. } => "sleep",
        Op::Key { .. } => "key",
        Op::Mouse { .. } => "mouse",
        Op::MouseDelta { .. } => "mouse-delta",
        Op::AwaitPhase { label, .. } | Op::ClickButton { label, .. } => label,
        Op::ExpectKeys { label, .. }
        | Op::ExpectChars { label, .. }
        | Op::ExpectMouse { label, .. }
        | Op::ExpectMouseDelta { label, .. }
        | Op::ExpectHudChange { label, .. } => label,
        Op::ExpectUplinkQuiet => "uplink:静默相位内桥上行事件为 0",
        Op::ObserveOutline { label }
        | Op::ObserveHud { label }
        | Op::ObserveSecondary { label } => label,
        Op::S2Begin { group } | Op::S2End { group } => group.label(),
        Op::Finish => "finish",
    }
}

/// 驱动器单例;只在主线程访问,互斥量承担借用约束。
static DRIVER: Mutex<Option<Driver>> = Mutex::new(None);

/// 驱动一帧;由 `ClientWindow::poll` 在事件泵与快照编码之后调用。
pub(crate) fn on_frame(ctx: &FrameContext<'_>) {
    if !enabled() {
        return;
    }
    let mut guard = DRIVER.lock().expect("spike 自驱动锁中毒");
    if guard.is_none() {
        *guard = Some(Driver::new());
    }
    if let Some(driver) = guard.as_mut() {
        driver.on_frame(ctx);
    }
}

// ---------------------------------------------------------------------------
// 汇总与落盘
// ---------------------------------------------------------------------------

/// 一臂的验证结果汇总(落盘前在内存里成形)。
struct Summary {
    arm: &'static str,
    generated_at: String,
    host: HostInfo,
    /// content 尺寸(逻辑点),S2 口径的一部分。
    content_width: f64,
    content_height: f64,
    s1_records: Vec<StepRecord>,
    s1_passed: usize,
    s1_total: usize,
    s2_done: usize,
    s2_idle: Option<WindowStats>,
    s2_mining: Option<WindowStats>,
    uplink_in_quiet_phase: u64,
    uplink_total: u64,
    quiet_frames: u64,
    phase_flips: u64,
    outline_flips: u64,
    reason: String,
    any_failed: bool,
}

/// 机型/系统信息;命令不可用时如实标注 unknown。
#[derive(Debug, Clone)]
struct HostInfo {
    /// `sysctl -n machdep.cpu.brand_string`。
    cpu: String,
    /// `sw_vers -productVersion`。
    macos: String,
}

impl HostInfo {
    fn probe() -> Self {
        Self {
            cpu: command_output("sysctl", &["-n", "machdep.cpu.brand_string"]),
            macos: command_output("sw_vers", &["-productVersion"]),
        }
    }
}

/// 运行一条命令并取首行输出;失败返回 unknown。
fn command_output(program: &str, args: &[&str]) -> String {
    std::process::Command::new(program)
        .args(args)
        .output()
        .ok()
        .map(|out| String::from_utf8_lossy(&out.stdout).trim().to_string())
        .filter(|text| !text.is_empty())
        .unwrap_or_else(|| "unknown".to_string())
}

/// 从驱动器状态构造汇总。
fn build_summary(driver: &Driver, ctx: &FrameContext<'_>, reason: &str) -> Summary {
    let obs = observations_snapshot();
    // 门禁断言决定判据;非门禁观察(装配时长、device-event 分支、HUD 入流
    // 口径)照实展示但不推翻结论。
    let gating: Vec<_> = driver
        .s1_records
        .iter()
        .filter(|record| record.gating)
        .cloned()
        .collect();
    // S1 总判据:全部门禁断言通过,且游戏静默相位内桥上行事件始终为 0。
    let s1_clean = obs.uplink_in_quiet_phase == 0;
    let s1_passed = gating.iter().filter(|record| record.passed).count() + usize::from(s1_clean);
    let any_failed = driver.any_failed || !s1_clean;
    Summary {
        arm: arm_tag(),
        generated_at: iso_timestamp(),
        host: HostInfo::probe(),
        content_width: 0.0,
        content_height: 0.0,
        s1_records: driver.s1_records.clone(),
        s1_passed,
        s1_total: gating.len() + 1,
        s2_done: driver.s2.iter().filter(|stats| stats.is_some()).count(),
        s2_idle: driver.s2[0],
        s2_mining: driver.s2[1],
        uplink_in_quiet_phase: obs.uplink_in_quiet_phase,
        uplink_total: obs.uplink_total,
        quiet_frames: obs.quiet_frames,
        phase_flips: obs.phase_flips,
        outline_flips: obs.outline_flips,
        reason: reason.to_string(),
        any_failed,
    }
    .with_context(ctx)
}

/// 把窗口尺寸并入汇总;真实窗口的 content 预设是 S2 口径的一部分。
impl Summary {
    fn with_context(mut self, ctx: &FrameContext<'_>) -> Self {
        self.content_width = ctx.content_width;
        self.content_height = ctx.content_height;
        self
    }
}

/// ISO 8601 本地时间戳(秒精度);时钟倒退等异常时退化为空串。
fn iso_timestamp() -> String {
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_secs())
        .unwrap_or(0);
    // 只做 UTC 的固定偏移换算,足够标注执行时间;不引入时间库。
    let days = now / 86_400;
    let rest = now % 86_400;
    let (year, month, day) = civil_from_days(days as i64);
    format!(
        "{year:04}-{month:02}-{day:02}T{:02}:{:02}:{:02}Z",
        rest / 3_600,
        (rest % 3_600) / 60,
        rest % 60
    )
}

/// 天数 → 公历年月日(Howard Hinnant 的 civil_from_days 算法,公有领域)。
fn civil_from_days(days: i64) -> (i64, u32, u32) {
    let z = days + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = z - era * 146_097;
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let year = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let day = (doy - (153 * mp + 2) / 5 + 1) as u32;
    let month = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
    (if month <= 2 { year + 1 } else { year }, month, day)
}

/// 写 `spike-result.json`(跨臂合并)与 `spike-report.md`(按臂追加)。
fn write_results(summary: &Summary) {
    let dir = out_dir();
    if let Err(error) = write_result_json(&dir, summary) {
        eprintln!("mornlea spike auto: 结果 JSON 写入失败: {error}");
    }
    if let Err(error) = write_report_md(&dir, summary) {
        eprintln!("mornlea spike auto: 报告写入失败: {error}");
    }
}

/// 读旧 JSON、按臂 upsert、写回;旧内容不是合法 JSON 时整份重建并留痕。
fn write_result_json(dir: &Path, summary: &Summary) -> std::io::Result<()> {
    let path = dir.join(RESULT_JSON);
    let mut root: serde_json::Map<String, serde_json::Value> = match std::fs::read(&path) {
        Ok(bytes) => match serde_json::from_slice::<serde_json::Value>(&bytes) {
            Ok(serde_json::Value::Object(map)) => map,
            _ => {
                eprintln!("mornlea spike auto: 既有结果文件不可解析,重建 {path:?}");
                serde_json::Map::new()
            }
        },
        Err(_) => serde_json::Map::new(),
    };
    let arms = root
        .entry("arms")
        .or_insert_with(|| serde_json::Value::Object(serde_json::Map::new()));
    if let Some(arms) = arms.as_object_mut() {
        arms.insert(summary.arm.to_string(), arm_record_json(summary));
    }
    std::fs::write(&path, serde_json::to_vec_pretty(&root).unwrap_or_default())
}

/// 把一臂汇总转成结构化 JSON:臂位配置、S1 逐条 pass/fail、S2 两组四列统计。
///
/// serde 未入本 crate 依赖面(只有 serde_json),手工组装也便于把判据语义
/// (如 `s1Verdict`)与字段一一对应。
fn arm_record_json(summary: &Summary) -> serde_json::Value {
    let stats_json = |stats: &Option<WindowStats>| match stats {
        Some(stats) => serde_json::json!({
            "samples": stats.samples,
            "interval_us": {
                "mean": stats.interval_us.mean,
                "p50": stats.interval_us.p50,
                "p95": stats.interval_us.p95,
                "max": stats.interval_us.max,
            },
            "frame_us": {
                "mean": stats.frame_us.mean,
                "p50": stats.frame_us.p50,
                "p95": stats.frame_us.p95,
                "max": stats.frame_us.max,
            },
        }),
        None => serde_json::Value::Null,
    };
    serde_json::json!({
        "arm": summary.arm,
        "overlay": summary.arm,
        "fps_probe": crate::overlay_spike::frame_probe().is_some(),
        "generated_at": summary.generated_at,
        "host": { "cpu": summary.host.cpu, "macos": summary.host.macos },
        "content_size_pt": [summary.content_width, summary.content_height],
        "exit_reason": summary.reason,
        "s1": {
            "verdict": if summary.any_failed { "fail" } else { "pass" },
            "passed": summary.s1_passed,
            "total": summary.s1_total,
            "uplink_events_in_quiet_phase": summary.uplink_in_quiet_phase,
            "quiet_phase_frames": summary.quiet_frames,
            "phase_flips": summary.phase_flips,
            "outline_flips": summary.outline_flips,
            "steps": summary.s1_records.iter().map(|record| serde_json::json!({
                "label": record.label,
                "pass": record.passed,
                "detail": record.detail,
            })).collect::<Vec<_>>(),
        },
        "s2": {
            "idle": stats_json(&summary.s2_idle),
            "mining": stats_json(&summary.s2_mining),
        },
    })
}

/// 追加一节人读报告;文件不存在时先写表头。
fn write_report_md(dir: &Path, summary: &Summary) -> std::io::Result<()> {
    let path = dir.join(REPORT_MD);
    let mut out = match std::fs::read_to_string(&path) {
        Ok(text) => text,
        Err(_) => String::from("# GameOverlay spike 自驱动执行报告\n"),
    };
    out.push_str(&render_report_section(summary));
    std::fs::write(&path, out)
}

/// 生成单臂的报告小节(Markdown)。
fn render_report_section(summary: &Summary) -> String {
    let mut text = String::new();
    text.push_str(&format!(
        "\n## 臂位 `{}`(执行 {})\n\n- 机型/CPU:`{}`;macOS `{}`\n- 退出原因:`{}`\n\
         - content 尺寸:{:.0}×{:.0} pt;相位翻转 {} 次;静默相位观察 {} 帧\n\
         - 桥上行:总 {} 条;**静默相位内 {} 条**(GameOverlay 穿透断言要求 0)\n\
         - 目标方块轮廓翻转 {} 次(准星是否对上地形的间接证据)\n\n",
        summary.arm,
        summary.generated_at,
        summary.host.cpu,
        summary.host.macos,
        summary.reason,
        summary.content_width,
        summary.content_height,
        summary.phase_flips,
        summary.quiet_frames,
        summary.uplink_total,
        summary.uplink_in_quiet_phase,
        summary.outline_flips,
    ));

    text.push_str(&format!(
        "### S1 输入序列(通过 {}/{}{})\n\n| # | 断言 | 结果 | 依据 |\n| --- | --- | --- | --- |\n",
        summary.s1_passed,
        summary.s1_total,
        if summary.any_failed {
            "、判据不达成"
        } else {
            ""
        }
    ));
    for (index, record) in summary.s1_records.iter().enumerate() {
        text.push_str(&format!(
            "| {} | {} | {} | {} |\n",
            index + 1,
            record.label.replace('|', "\\|"),
            if record.passed { "PASS" } else { "FAIL" },
            record.detail.replace('|', "\\|")
        ));
    }
    if summary.uplink_in_quiet_phase == 0 {
        text.push_str("\n静默相位内桥上行事件为 0:`GameOverlay` 不产生任何上行,穿透断言成立。\n");
    } else {
        text.push_str(&format!(
            "\n**静默相位内出现 {} 条桥上行事件**:WebView 参与了响应链,hitTest 分级不成立。\n",
            summary.uplink_in_quiet_phase
        ));
    }

    text.push_str("\n### S2 帧开销\n\n| 组 | samples | interval_us mean | interval_us p50 | interval_us p95 | interval_us max | frame_us mean | frame_us p50 | frame_us p95 | frame_us max |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n");
    for (label, stats) in [("idle", summary.s2_idle), ("mining", summary.s2_mining)] {
        match stats {
            Some(stats) => text.push_str(&format!(
                "| {label} | {} | {} | {} | {} | {} | {} | {} | {} | {} |\n",
                stats.samples,
                stats.interval_us.mean,
                stats.interval_us.p50,
                stats.interval_us.p95,
                stats.interval_us.max,
                stats.frame_us.mean,
                stats.frame_us.p50,
                stats.frame_us.p95,
                stats.frame_us.max
            )),
            None => text.push_str(&format!(
                "| {label} | — 无数据(帧探针未开启或样本不足) | — | — | — | — | — | — | — | — |\n"
            )),
        }
    }
    text.push('\n');
    text
}

#[cfg(test)]
mod tests {
    use super::{
        CHAT_TEXT, DIGIT_KEYS, Op, S2Group, arm_tag_for, ascii_vk, click_button_script, digit_text,
        go_bit, key_mask, plan_script,
    };
    use winit::keyboard::KeyCode;

    use crate::input::key_bit;
    use crate::overlay::OverlayMode;

    /// 臂位标注是结果文件与跨臂比较的主键:不强制 = `baseline`(生产路径,
    /// 参与模式由相位推导,游戏相位即 GameOverlay),强制 Menu = 对照臂
    /// (**复测取基线用这一臂**),强制 GameOverlay = 验证臂。标注名沿用
    /// A 组口径,语义迁移记录在 checklist 与 design D7;改名会让既有结果
    /// 文件与报告失去可比性,必须同步文档。
    #[test]
    fn arm_tag_follows_forced_mode() {
        assert_eq!(arm_tag_for(None), "baseline");
        assert_eq!(arm_tag_for(Some(OverlayMode::Menu)), "menu");
        assert_eq!(arm_tag_for(Some(OverlayMode::GameOverlay)), "game");
    }

    /// 合成事件用的虚拟键码必须与 winit 的扫描码→`PhysicalKey` 映射以及
    /// Go `client.Key` 位序三方一致,否则断言的位掩码与真实键位脱节。
    #[test]
    fn virtual_keycodes_match_go_key_bits() {
        let pairs = [
            (super::VK_W, KeyCode::KeyW),
            (super::VK_A, KeyCode::KeyA),
            (super::VK_S, KeyCode::KeyS),
            (super::VK_D, KeyCode::KeyD),
            (super::VK_E, KeyCode::KeyE),
            (super::VK_RETURN, KeyCode::Enter),
            (super::VK_ESCAPE, KeyCode::Escape),
        ];
        for (vk, code) in pairs {
            assert_eq!(go_bit(vk), key_bit(code), "vk={vk:#x} code={code:?}");
        }
        for (index, vk) in DIGIT_KEYS.iter().enumerate() {
            let code = match index {
                0 => KeyCode::Digit1,
                1 => KeyCode::Digit2,
                2 => KeyCode::Digit3,
                3 => KeyCode::Digit4,
                4 => KeyCode::Digit5,
                5 => KeyCode::Digit6,
                6 => KeyCode::Digit7,
                7 => KeyCode::Digit8,
                _ => KeyCode::Digit9,
            };
            assert_eq!(go_bit(*vk), key_bit(code), "vk={vk:#x} code={code:?}");
        }
    }

    /// 脚本必须完整覆盖 S1 序列、两组 S2 测量与菜单两态回环,且以 Finish 收尾。
    #[test]
    fn script_covers_s1_s2_and_menu_round_trip() {
        let ops = plan_script();
        assert!(
            matches!(ops.last(), Some(Op::Finish)),
            "脚本必须以 Finish 收尾"
        );
        let count = |predicate: &dyn Fn(&Op) -> bool| ops.iter().filter(|op| predicate(op)).count();
        // 两组 S2 测量各自成对出现。
        assert_eq!(
            count(&|op| matches!(
                op,
                Op::S2Begin {
                    group: S2Group::Idle
                }
            )),
            1
        );
        assert_eq!(
            count(&|op| matches!(
                op,
                Op::S2End {
                    group: S2Group::Idle
                }
            )),
            1
        );
        assert_eq!(
            count(&|op| matches!(
                op,
                Op::S2Begin {
                    group: S2Group::Mining
                }
            )),
            1
        );
        assert_eq!(
            count(&|op| matches!(
                op,
                Op::S2End {
                    group: S2Group::Mining
                }
            )),
            1
        );
        // 菜单按钮点击:进入游戏 + 退回主菜单,且每条 ClickButton 后都跟装配等待。
        assert!(count(&|op| matches!(op, Op::ClickButton { .. })) >= 3);
        assert!(count(&|op| matches!(op, Op::AwaitPhase { .. })) >= 6);
        // 穿透断言在两轮 S1 与测量结束后各一次。
        assert!(count(&|op| matches!(op, Op::ExpectUplinkQuiet)) >= 3);
        // 数字键 1..9 在两轮 S1 里都被按下。
        let digit_taps = ops
            .iter()
            .filter(|op| matches!(op, Op::Key { vk, down: true, .. } if DIGIT_KEYS.contains(vk)))
            .count();
        assert!(digit_taps >= 18, "两轮各 9 个数字键,实际 {digit_taps}");
    }

    /// 断言用的位掩码必须覆盖全部数字键与 WASD。
    #[test]
    fn key_masks_cover_all_targets() {
        assert_eq!(
            key_mask(&DIGIT_KEYS),
            (8..=16).fold(0u64, |acc, bit| acc | (1 << bit))
        );
        assert_eq!(
            key_mask(&[super::VK_W, super::VK_A, super::VK_S, super::VK_D]),
            (1 << 0) | (1 << 1) | (1 << 2) | (1 << 3)
        );
    }

    /// 页面脚本按文本查找启用按钮,带回退路径,并把请求号与命中信息写进回执。
    #[test]
    fn click_script_targets_enabled_button_with_fallback() {
        let script = click_button_script(7, "进入游戏");
        assert!(script.contains("var id=7;"), "回执必须携带请求号:{script}");
        assert!(
            script.contains(r#"var want="进入游戏";"#),
            "目标文本必须内插:{script}"
        );
        // 选择器只锚定菜单结构的稳定类名,不触碰组件内部实现。
        assert!(script.contains("querySelectorAll('.menu-button')"));
        assert!(script.contains("querySelector('.menu-buttons')"));
        // 命中路径 click() 走真实 React onClick;未命中回执携带 ok:false。
        assert!(script.contains("hit.click()"));
        assert!(script.contains("ok:false"));
    }

    /// 数字键与 ASCII 字符的映射必须有对应字符,否则聊天步会注入空文本。
    #[test]
    fn key_text_mappings_are_total_for_script_use() {
        for vk in DIGIT_KEYS {
            assert_eq!(digit_text(vk).len(), 1, "vk={vk:#x}");
        }
        for ch in CHAT_TEXT.chars() {
            assert!(ascii_vk(ch).is_some(), "字符 {ch} 必须有虚拟键码");
        }
        assert_eq!(ascii_vk('1'), None, "非字母字符不走字母映射");
    }
}

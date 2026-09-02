//! 菜单层进程内 WKWebView 集成(darwin 专属)。
//!
//! 按 design D2 的裁决:用 `objc2-web-kit` 手写集成,把一个透明 WKWebView
//! 以 subview 形式挂在 winit NSWindow 的 contentView 之上,不引入 wry/tao、
//! 不建第二窗口栈。关键设计:
//!
//! - **透明**:`setDrawsBackground: NO` 让 wgpu 画面从 WebView 下层透出
//!   (该属性为半文档特性,由首日 spike 实证)。
//! - **资产离线供给**:`WKURLSchemeHandler` 从二进制内嵌字节供给
//!   `mornlea://` 资产——零磁盘写入、零网络、零 CDN。
//! - **下行**:Go 推送状态 JSON,Rust 浅校验后缓存,并经
//!   `evaluateJavaScript` 调 `window.mornlea.onState(<json>)`(JSON 即合法
//!   JS 表达式;Go 侧 encoding/json 恒转义 U+2028/U+2029,注入安全)。
//!   仅在状态文本变化时求值——同一份状态重复推送是幂等空操作。
//! - **上行**:`WKScriptMessageHandler` 收页面信封,经 `NSJSONSerialization`
//!   还原为 JSON 字节交 [`crate::bridge`] 校验入队。
//! - **相位路由(两态参与模型)**:下行状态的相位字段是参与模式的唯一驱动
//!   源(`crate::overlay`)。菜单相位(以及叠加可见调试面板的游戏相位)为
//!   `Menu` 态:视图可见并夺取 firstResponder,键盘/指针交给页面,winit 不
//!   再收到按键,游戏输入天然静默。无 chrome 的游戏相位为 `GameOverlay`
//!   态:视图**保持可见**(透明合成于 wgpu 画面之上,承载常显 HUD),
//!   `hitTest:` 返回 nil 让事件穿透,firstResponder 归还 winit 内容视图
//!   (键盘不双投、不丢)。视图被隐藏时(Spike 对照臂强制 Menu 态的游戏
//!   相位)不做任何求值,页面 DOM 状态在隐藏期间保持,重新显示无需重置。
//! - **页面就绪重推**:WebView 是异步加载的,加载完成前的求值会丢失;
//!   `didFinishNavigation` 后把缓存的最近状态重推给新文档,保证首屏有状态,
//!   并按当前参与模式重放 GameOverlay 态的页面透明。同一回调也是渲染进程
//!   崩溃(`webViewWebContentProcessDidTerminate`)后的自愈通道:崩溃即重载
//!   页面,重载完成走既有的就绪重推,菜单呈现自行恢复。
//!
//! 线程约束:全部方法必须在创建窗口的 OS 主线程调用(与 `ClientWindow`
//! 的 FFI 线程约束一致);WebKit 对象都要求主线程。

use std::sync::{Arc, Mutex};

use objc2::rc::Retained;
use objc2::runtime::{AnyObject, NSObject, NSObjectProtocol, ProtocolObject};
use objc2::{
    AnyThread, DefinedClass, MainThreadMarker, MainThreadOnly, define_class, extern_conformance,
    msg_send,
};
use objc2_foundation::{
    NSData, NSDictionary, NSHTTPURLResponse, NSJSONSerialization, NSJSONWritingOptions, NSPoint,
    NSRect, NSSize, NSString, NSURL, NSURLRequest,
};
use objc2_web_kit::{
    WKNavigation, WKNavigationDelegate, WKScriptMessage, WKScriptMessageHandler,
    WKURLSchemeHandler, WKURLSchemeTask, WKUserContentController, WKWebView,
    WKWebViewConfiguration,
};

use crate::bridge::SharedUiEventQueue;
use crate::overlay::{OverlayMode, OverlayRuntime, OverlayWebView, apply_page_phase};

/// 内嵌菜单前端资产(vite build 产物,dist 入库)。字体是唯一白名单二进制
/// 资产(OFL-1.1,见 frontend/src/ui/fonts/),经本表与 html/js/css 一同
/// 内嵌供给。
static EMBEDDED_INDEX_HTML: &[u8] = include_bytes!("../frontend/dist/index.html");
static EMBEDDED_INDEX_JS: &[u8] = include_bytes!("../frontend/dist/assets/index.js");
static EMBEDDED_INDEX_CSS: &[u8] = include_bytes!("../frontend/dist/assets/index.css");
static EMBEDDED_PIXEL_FONT: &[u8] =
    include_bytes!("../frontend/dist/assets/fusion-pixel-12px-proportional-zh_hans.ttf.woff2");

/// 页面入口 URL;host 仅为命名空间,path 才是资产寻址依据。
const ENTRY_URL: &str = "mornlea://app/index.html";

/// 按 URL path 解析内嵌资产,返回 (字节, MIME)。
///
/// 页面里是相对引用(`./assets/index.js`),因此 path 与入库 dist 相对路径
/// 一一对应;未来新增字体等资产时在此表扩展。未命中返回 None(404)。
fn embedded_asset(path: &str) -> Option<(&'static [u8], &'static str)> {
    match path {
        "/" | "/index.html" => Some((EMBEDDED_INDEX_HTML, "text/html; charset=utf-8")),
        "/assets/index.js" => Some((EMBEDDED_INDEX_JS, "text/javascript; charset=utf-8")),
        "/assets/index.css" => Some((EMBEDDED_INDEX_CSS, "text/css; charset=utf-8")),
        "/assets/fusion-pixel-12px-proportional-zh_hans.ttf.woff2" => {
            Some((EMBEDDED_PIXEL_FONT, "font/woff2"))
        }
        _ => None,
    }
}

/// 跨回调对象与宿主共享的桥状态:上行事件队列、最近下行状态缓存与幂等
/// 求值基准。回调对象持有本结构的 `Arc`;全部字段只存可跨线程类型,导航
/// 完成回调所需的宿主 WebView 由 WKNavigationDelegate 的消息参数直接给出,
/// 不经字段中转(避免「delegate 持有 WebView → WebView 持有 delegate」的环)。
///
/// 事件队列用 [`SharedUiEventQueue`](crate::bridge::SharedUiEventQueue) 克隆自
/// 进程级单例:上行事件的排空出口在渲染器(`drain_ui_events`)上,而 WebView
/// 挂在窗口侧,两侧经同一单例交汇。类型整体 Send+Sync,但按线程模型只在
/// 主线程访问。
struct HostShared {
    queue: SharedUiEventQueue,
    /// 最近一次 `push_state` 收到的状态 JSON 原文(浅校验过)。
    last_state: Mutex<Option<String>>,
    /// 最近一次实际求值到**当前文档**的状态文本;幂等转发的判定基准。
    last_evaluated: Mutex<Option<String>>,
    /// 两态参与模式运行态:与子类 ivar 同源。导航完成回调据此重放
    /// GameOverlay 态的页面透明——页面重载会丢失注入的相位样式,重放必须
    /// 与相位切换读到同一份模式。
    overlay: Arc<OverlayRuntime>,
}

// 桥回调对象:同时实现 `WKScriptMessageHandler`(上行信封)与
// `WKNavigationDelegate`(页面就绪重推)。单对象双协议,避免两份共享状态。
define_class!(
    // 两个协议都要求主线程实现,类本身声明为 MainThreadOnly。
    #[unsafe(super(NSObject))]
    #[thread_kind = MainThreadOnly]
    #[ivars = Arc<HostShared>]
    #[name = "MornleaBridgeDelegate"]
    struct BridgeDelegate;

    unsafe impl WKScriptMessageHandler for BridgeDelegate {
        #[unsafe(method(userContentController:didReceiveScriptMessage:))]
        unsafe fn user_content_controller_did_receive_script_message(
            &self,
            _user_content_controller: &WKUserContentController,
            message: &WKScriptMessage,
        ) {
            self.receive_script_message(message);
        }
    }

    unsafe impl WKNavigationDelegate for BridgeDelegate {
        #[unsafe(method(webView:didFinishNavigation:))]
        unsafe fn web_view_did_finish_navigation(
            &self,
            web_view: &WKWebView,
            _navigation: Option<&WKNavigation>,
        ) {
            self.did_finish_navigation(web_view);
        }

        #[unsafe(method(webView:didFailProvisionalNavigation:withError:))]
        unsafe fn web_view_did_fail_provisional_navigation_with_error(
            &self,
            _web_view: &WKWebView,
            _navigation: Option<&WKNavigation>,
            error: &objc2_foundation::NSError,
        ) {
            // 内嵌资产供给失败属部署级故障(缺资产/注册错误),宁可显式
            // 打到 stderr 也不能让菜单无声消失。
            let detail = error.localizedDescription();
            eprintln!("mornlea webview: 页面加载失败: {detail}");
        }

        #[unsafe(method(webView:didFailNavigation:withError:))]
        unsafe fn web_view_did_fail_navigation_with_error(
            &self,
            _web_view: &WKWebView,
            _navigation: Option<&WKNavigation>,
            error: &objc2_foundation::NSError,
        ) {
            let detail = error.localizedDescription();
            eprintln!("mornlea webview: 导航失败: {detail}");
        }

        #[unsafe(method(webViewWebContentProcessDidTerminate:))]
        unsafe fn web_view_web_content_process_did_terminate(&self, web_view: &WKWebView) {
            self.web_content_process_did_terminate(web_view);
        }
    }
);

extern_conformance!(
    unsafe impl NSObjectProtocol for BridgeDelegate {}
);

impl BridgeDelegate {
    fn new(shared: Arc<HostShared>, mtm: MainThreadMarker) -> Retained<Self> {
        // SAFETY: 仅在主线程创建(mtm 证明);ivars 随 MainThreadOnly 的回调
        // 对象只在主线程访问,不承担任何跨线程语义。
        let this = mtm.alloc().set_ivars(shared);
        // SAFETY: 调用 NSObject 的指定初始化器;ivars 已在 alloc 后、init 前写入。
        unsafe { msg_send![super(this), init] }
    }

    /// 上行信封入口:页面 postMessage 的对象被 WebKit 装箱为
    /// NSDictionary/NSArray 等容器,这里统一经 `NSJSONSerialization` 还原为
    /// JSON 字节再交桥队列浅校验入队。序列化失败(非容器)的载荷被静默
    /// 丢弃——协议的显式拒绝语义由 [`UiEventQueue::enqueue_envelope`] 的
    /// 校验结果与 Go 消费侧承担,回调路径没有可上报的错误通道。
    fn receive_script_message(&self, message: &WKScriptMessage) {
        // SAFETY: body 消息只读,WebKit 保证回调期间对象有效。
        let body = unsafe { message.body() };
        // SAFETY: body 是任意 JS 装箱值;非容器时返回 Err,不触碰队列。
        let data = match unsafe {
            NSJSONSerialization::dataWithJSONObject_options_error(
                &body,
                NSJSONWritingOptions::empty(),
            )
        } {
            Ok(data) => data,
            Err(_) => return,
        };
        let bytes = data.to_vec();
        // spike 自驱动档把上行事件数作为 GameOverlay 穿透断言的输入;关闭时
        // 零解析。信封是否被接受由 enqueue 的返回值决定,这里只统计到达数。
        crate::spike_auto::note_uplink_envelope(&bytes);
        let shared = self.ivars();
        // 容量护栏兜底:页面事件洪泛时整信封拒绝,不阻塞主线程。
        let _ = shared.queue.enqueue_envelope(&bytes);
    }

    /// 页面导航完成:新文档没有任何状态,把缓存的最近状态重推一次。
    /// 重推同时刷新幂等基准(`last_evaluated`),保证缓存真正落到新文档。
    /// GameOverlay 态下还要重放页面透明注入(新文档不携带任何样式),否则
    /// 菜单页的不透明背景会盖住 wgpu 画面。宿主 WebView 由 delegate 消息
    /// 参数给出,不经共享状态中转。
    fn did_finish_navigation(&self, webview: &WKWebView) {
        let shared = self.ivars();
        if shared.overlay.mode() == OverlayMode::GameOverlay {
            // SAFETY: 主线程回调;webview 由 WebKit 保证在回调期间存活。
            unsafe { apply_page_phase(webview, true) };
        }
        let state = shared.last_state.lock().expect("桥状态缓存锁中毒").clone();
        let Some(state) = state else { return };
        evaluate_state(webview, &state);
        *shared.last_evaluated.lock().expect("桥幂等基准锁中毒") = Some(state);
    }

    /// 渲染进程(WebContent)崩溃兜底:进程终止时页面失去全部脚本状态,菜单
    /// 会无声消失。这里立即重载页面;重载完成后的 [`Self::did_finish_navigation`]
    /// 把缓存的最近状态重推给新文档,Go 侧无需感知崩溃——下次状态变化前菜单
    /// 就已按崩溃前内容恢复。
    fn web_content_process_did_terminate(&self, webview: &WKWebView) {
        eprintln!("mornlea webview: WebContent 进程终止,重载菜单页面自愈");
        // SAFETY: 主线程回调;返回的导航对象无需保留(完成回调经
        // didFinishNavigation 送达同一 delegate)。
        let _ = unsafe { webview.reload() };
    }
}

// 资产 scheme handler:每个 `mornlea://` 请求从内嵌字节取资产回包;
// 未命中按 404 回应(空体),页面侧表现为加载失败而非挂死。
define_class!(
    #[unsafe(super(NSObject))]
    #[thread_kind = MainThreadOnly]
    #[ivars = ()]
    #[name = "MornleaSchemeHandler"]
    struct SchemeHandler;

    unsafe impl WKURLSchemeHandler for SchemeHandler {
        #[unsafe(method(webView:startURLSchemeTask:))]
        unsafe fn web_view_start_url_scheme_task(
            &self,
            _web_view: &WKWebView,
            task: &ProtocolObject<dyn WKURLSchemeTask>,
        ) {
            serve_scheme_task(task);
        }

        #[unsafe(method(webView:stopURLSchemeTask:))]
        unsafe fn web_view_stop_url_scheme_task(
            &self,
            _web_view: &WKWebView,
            _task: &ProtocolObject<dyn WKURLSchemeTask>,
        ) {
            // 页面取消加载(如导航离开):内嵌供给是同步完成的,无在途
            // 异步工作需要撤销。
        }
    }
);

extern_conformance!(
    unsafe impl NSObjectProtocol for SchemeHandler {}
);

impl SchemeHandler {
    fn new(mtm: MainThreadMarker) -> Retained<Self> {
        // SAFETY: 仅在主线程创建(mtm 证明);无状态类以空 ivars 走标准
        // alloc + init 指定初始化器序列。
        let this = mtm.alloc::<Self>().set_ivars(());
        // SAFETY: 调用 NSObject 的指定初始化器;ivars 已在 alloc 后、init 前写入。
        unsafe { msg_send![super(this), init] }
    }
}

/// 回应单个 scheme 任务:同步写响应 + 数据 + finish,零磁盘零网络。
fn serve_scheme_task(task: &ProtocolObject<dyn WKURLSchemeTask>) {
    // SAFETY: request 消息只读,WebKit 保证回调期间对象有效。
    let url = unsafe { task.request().URL() };
    let path = url
        .as_ref()
        .and_then(|url| url.path())
        .map_or(String::new(), |path| path.to_string());
    let Some((bytes, mime)) = embedded_asset(&path) else {
        respond_not_found(task, path);
        return;
    };
    let Some(url) = url else { return };
    // SAFETY: task 由 WebKit 传入且当前仍有效;容器类型均为 foundation 值。
    unsafe {
        let header = NSString::from_str("Content-Type");
        let value = NSString::from_str(mime);
        let headers = NSDictionary::from_slices(&[&*header], &[&*value]);
        let response = NSHTTPURLResponse::initWithURL_statusCode_HTTPVersion_headerFields(
            NSHTTPURLResponse::alloc(),
            &url,
            200,
            Some(&NSString::from_str("HTTP/1.1")),
            Some(&headers),
        );
        if let Some(response) = response {
            task.didReceiveResponse(&response);
            task.didReceiveData(&NSData::with_bytes(bytes));
            task.didFinish();
        }
    }
}

/// 对未知 path 回 404,不产出任何资产字节。
fn respond_not_found(task: &ProtocolObject<dyn WKURLSchemeTask>, path: String) {
    // SAFETY: request 消息只读,WebKit 保证回调期间对象有效。
    let url = unsafe { task.request().URL() };
    let Some(url) = url else { return };
    // SAFETY: 同 serve_scheme_task;404 只是状态码差异。
    unsafe {
        let response = NSHTTPURLResponse::initWithURL_statusCode_HTTPVersion_headerFields(
            NSHTTPURLResponse::alloc(),
            &url,
            404,
            Some(&NSString::from_str("HTTP/1.1")),
            None,
        );
        let Some(response) = response else { return };
        task.didReceiveResponse(&response);
        task.didReceiveData(&NSData::with_bytes(
            format!("mornlea: 未知资产 {path}").as_bytes(),
        ));
        task.didFinish();
    }
}

/// 构造状态求值脚本(纯函数,便于测试钉值);形态见 [`evaluate_state`]。
fn evaluate_state_script(state: &str) -> String {
    format!(
        "(function(){{var s={state};function d(){{if(window.mornlea&&window.mornlea.onState){{window.mornlea.onState(s);return true;}}return false;}}if(!d()){{var t=setInterval(function(){{if(d())clearInterval(t);}},16);setTimeout(function(){{clearInterval(t);}},10000);}}}})()"
    )
}

/// 把状态 JSON 以 `window.mornlea.onState(<json>)` 表达式求值。
///
/// JSON 是合法 JS 表达式(现代 JS 字符串允许 U+2028/U+2029,且 Go
/// encoding/json 恒转义它们),无需再转义;求值是异步 fire-and-forget,
/// 失败路径由下一次状态推送或导航完成重推自愈。
fn evaluate_state(webview: &WKWebView, state: &str) {
    // 自投递脚本:导航完成回调先于 ES 模块执行(探针实证 readyState=interactive
    // 时 `window.mornlea` 尚未定义),立即求值会静默丢失。脚本因此自带重试:
    // 桥全局就绪前每 16ms 重投一次,10 秒后放弃——WebContent 不响应时不给
    // 页面留下常驻定时器。投递是幂等的:同一份状态重复 onState 只会让 React
    // 收敛到同一渲染。
    let script = NSString::from_str(&evaluate_state_script(state));
    // SAFETY: 主线程;完成回调为 None(投递重试在脚本内自愈)。
    unsafe { webview.evaluateJavaScript_completionHandler(&script, None) };
}

/// 把 WebView 背景设为透明:`setDrawsBackground:` 选择器存在(macOS 26 起
/// 公开)时直接调用;更早系统仅当 `drawsBackground` getter 存在(属性可经
/// KVC 写)时走 KVC 半文档路径,否则保持不透明——功能降级为菜单自绘
/// 背景,绝不因未知选择器或缺失键崩溃( spike 假设 (a) 的守卫实现)。
///
/// SAFETY: receiver 是主线程上存续的 WKWebView;所有消息都先经
/// respondsToSelector 守卫。
unsafe fn apply_transparent_background(webview: &WKWebView) {
    let setter = objc2::sel!(setDrawsBackground:);
    // SAFETY: respondsToSelector 只读消息,任意对象安全。
    let has_public_setter: bool = webview.respondsToSelector(setter);
    if has_public_setter {
        let _: () = unsafe { msg_send![webview, setDrawsBackground: false] };
        return;
    }
    // 无公开选择器的系统走 wry 同款 KVC 半文档路径(WebKit 内部长期实现
    // `drawsBackground` 键,只是未入公开头文件);键缺失时 NSUndefinedKeyException
    // 被 objc2::exception::catch 折叠为「不透明」降级——绝不让 ObjC 异常穿透
    // FFI(FFI 层的 catch_unwind 拦不住 foreign exception)。
    let key = NSString::from_str("drawsBackground");
    let value = objc2_foundation::NSNumber::new_bool(false);
    // SAFETY: KVC 消息只触达主线程上的存活 WebView;消息参数均为本次调用
    // 栈内存续的局部对象。
    // SAFETY: WebView 引用经 AssertUnwindSafe 跨 catch 边界——消息本身
    // 不会重入 Rust 状态,异常路径不产生部分写入。
    let outcome = unsafe {
        objc2::exception::catch(std::panic::AssertUnwindSafe(|| {
            let _: () = msg_send![webview, setValue: &*value, forKey: &*key];
        }))
    };
    if outcome.is_err() {
        eprintln!("mornlea webview: KVC 透明背景不可用,菜单退化为不透明自绘背景");
    }
}

/// 回读 `drawsBackground` 生效值;仅公开 getter 存在时可信,否则返回 None
/// (调用方按未知记录,不伪造证据)。
///
/// SAFETY: 消息先经 respondsToSelector 守卫,receiver 在主线程存活。
unsafe fn draws_background(webview: &WKWebView) -> Option<bool> {
    let getter = objc2::sel!(drawsBackground);
    // SAFETY: 同上。
    let has_getter: bool = webview.respondsToSelector(getter);
    if !has_getter {
        return None;
    }
    // SAFETY: getter 存在,BOOL 返回按 objc2 ABI 转换。
    let value: bool = unsafe { msg_send![webview, drawsBackground] };
    Some(value)
}

/// 下行状态被拒绝的原因。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StateError {
    /// 不是合法 UTF-8、不是 JSON 对象,或缺 `phase` 字符串字段。
    Malformed,
}

/// 浅校验一份状态并报告它是否要求 WebView 以菜单形态全参与:非 game 相位恒
/// 要求;game 相位叠加可见的 debug 面板(`debug.visible`)同样要求——面板
/// 需要键盘与指针。返回 `false` 即游戏相位,参与模式进入 GameOverlay。
/// 校验失败(非 UTF-8/非对象/缺 phase)返回 [`StateError::Malformed`]。
pub(crate) fn state_wants_visible(json: &[u8]) -> Result<bool, StateError> {
    let text = std::str::from_utf8(json).map_err(|_| StateError::Malformed)?;
    let value: serde_json::Value = serde_json::from_str(text).map_err(|_| StateError::Malformed)?;
    let phase = value
        .get("phase")
        .and_then(|phase| phase.as_str())
        .ok_or(StateError::Malformed)?;
    Ok(state_wants_visible_parsed(phase, &value))
}

/// 对已解析状态计算菜单参与意图;与 [`MenuWebview::push_state`] 共用判定,
/// 也是参与模式推导([`crate::overlay::mode_for_phase`])的输入。
fn state_wants_visible_parsed(phase: &str, value: &serde_json::Value) -> bool {
    // debug.visible 可叠加在任意相位(含 game):面板可见时 WebView 全参与。
    let debug_visible = value.pointer("/debug/visible").and_then(|v| v.as_bool()) == Some(true);
    phase != "game" || debug_visible
}

/// 一个挂载在 winit 窗口上的菜单 WebView。生命周期由持有它的渲染器管理:
/// drop 时解除脚本 handler 注册并把自身移出视图树,不触碰窗口指针。
pub struct MenuWebview {
    /// 子类实例(`MornleaOverlayWebView`,见 [`crate::overlay`]):两态参与
    /// 模型要求 `hitTest:` 可被 GameOverlay 态改写,裸 WKWebView 分支已退役。
    /// 存储为父类句柄,既有生命周期逻辑(挂载、显隐、求值)不需要分支。
    webview: Retained<WKWebView>,
    /// 建立脚本 handler 注册的控制器;teardown 时据此解除注册。
    content_controller: Retained<WKUserContentController>,
    /// 共享桥状态(与回调对象同源);其中的 `overlay` 字段就是两态参与模式
    /// 运行态,与子类 ivar 同源(见 [`crate::overlay`]),不再单独持有一份,
    /// 相位切换与导航回调只读同一份 `Arc`。
    shared: Arc<HostShared>,
    /// spike 遗留强制档位(`MORNLEA_SPIKE_OVERLAY`,spike 验收后移除):
    /// `Some(mode)` 时参与模式被钉死,不再跟随下行相位;`None`(生产)时
    /// 模式完全由相位推导。只改「模式来源」,不新增动作语义。
    spike_forced: Option<OverlayMode>,
    /// winit 内容视图指针(游戏相位归还 firstResponder 的目标)。
    /// 与窗口同生命周期;只在窗口存续的正常运行路径上解引用。
    game_view: *mut AnyObject,
    /// 菜单参与意图:下行相位是否要求 WebView 全参与。GameOverlay 态下视图
    /// 仍可见,本字段只表示「以菜单形态参与」,真实显隐由动作表的
    /// `hide_view` 决定,两者只在强制 Menu 档的游戏相位出现分歧。
    ///
    /// 不变式(跨文件,依赖 [`crate::window::should_mount`]):初值 `false`
    /// 成立的前提是 `attach` 只在 `wants_visible == true` 的推送里发生——
    /// 因此挂载后对同一份状态的 [`Self::push_state`] 必然触发一次
    /// `false -> true` 翻转,首套 AppKit 动作(显示 + 夺取焦点 + 页面相位)
    /// 由此落地。若挂载门被绕开(例如挂载发生在游戏相位推送),首个同值推送
    /// 不触发任何切换:视图停留在挂载时的隐藏态,参与模式停留在缺省
    /// `Menu`(穿透关闭),GameOverlay 的可见合成、穿透与页面
    /// 相位在首个相位周期内静默不生效(视图隐藏时输入仍由 winit 独占,缺省
    /// 不穿透保证不会吞输入;但 HUD 不呈现,直到下一次相位翻转自愈)。
    menu_participating: bool,
}

// 线程模型:全部方法都在创建窗口的 OS 主线程调用(FFI 层 thread-local
// 表约束),`game_view` 指向的 winit 视图与窗口同生命周期,解引用点(相位
// 切换)都在窗口存续期间;持有方 `ClientWindow` 存于 thread-local 表,
// 本类型天然不跨线程。

impl MenuWebview {
    /// 创建 WebView 并挂载到 `ns_window` 的 contentView 之上。
    ///
    /// `game_view` 是 winit 渲染视图(游戏相位归还 firstResponder 的目标)。
    ///
    /// 非主线程或挂载失败返回 None,上层按「无 WebView」降级运行,零参与
    /// 语义不破。
    ///
    /// # Safety
    ///
    /// `ns_window`/`game_view` 必须是当前主线程上存续的有效 NSWindow/NSView
    /// 指针,由调用方从活动窗口句柄取得;指针在窗口销毁后不得再传入。
    pub unsafe fn attach(ns_window: *mut AnyObject, game_view: *mut AnyObject) -> Option<Self> {
        let mtm = MainThreadMarker::new()?;
        if ns_window.is_null() || game_view.is_null() {
            return None;
        }
        // 参与模式运行态在共享桥状态建立前构造:回调对象与子类 ivar 持有
        // 同一份 `Arc`,相位切换与页面重放因此读到同一份模式。
        let overlay_runtime = OverlayRuntime::new();
        // spike 遗留强制档位只在挂载时读取一次;生产路径(未设环境变量)为
        // None,参与模式由下行相位逐次推导。
        let spike_forced = crate::overlay_spike::forced_mode_from_env();
        let shared = Arc::new(HostShared {
            queue: crate::bridge::shared_queue().clone(),
            last_state: Mutex::new(None),
            last_evaluated: Mutex::new(None),
            overlay: overlay_runtime.clone(),
        });
        // mtm 同时约束本函数只在主线程调用;类与回调对象均为主线程构造。
        let delegate = BridgeDelegate::new(shared.clone(), mtm);
        let scheme_handler = SchemeHandler::new(mtm);

        // SAFETY: WKWebViewConfiguration::new 要求主线程,mtm 已证明。
        let config = unsafe { WKWebViewConfiguration::new(mtm) };
        // 资产 scheme:页面内全部请求都从内嵌字节供给。
        unsafe {
            config.setURLSchemeHandler_forURLScheme(
                Some(ProtocolObject::from_ref(&*scheme_handler)),
                &NSString::from_str("mornlea"),
            );
        }
        // SAFETY: getter 只读且要求主线程。
        let controller = unsafe { config.userContentController() };
        unsafe {
            controller.addScriptMessageHandler_name(
                ProtocolObject::from_ref(&*delegate),
                &NSString::from_str("mornlea"),
            );
        }

        let frame = NSRect::new(NSPoint::new(0.0, 0.0), NSSize::new(0.0, 0.0));
        // 子类是唯一构造路径:两态参与模型要求 `hitTest:` 在 GameOverlay 态
        // 返回 nil,裸 WKWebView 无法承担;`Menu` 态的命中行为与父类实现
        // 逐项一致(穿透标志关闭即走 super),子类化因此对菜单相位零副作用。
        // SAFETY: 主线程(mtm 证明);frame/config 均为本函数栈上或本线程
        // 存续的有效值。
        let webview = unsafe { OverlayWebView::new(frame, &config, overlay_runtime.clone(), mtm) }
            // 上转到父类句柄:后续既有生命周期逻辑(挂载、显隐、求值)对
            // 子类实例同样成立,不需要分支。
            .into_super();
        // 透明背景:setDrawsBackground 为半文档属性(首日 spike 实证)。
        // macOS 26 起 WKWebView 公开 drawsBackground 属性;更早系统上经
        // KVC `setValue:forKey:` 写同名的半文档属性。两条路都先经
        // respondsToSelector/验证器守卫,不支持的系统上保持不透明——
        // 功能降级为菜单自绘背景,绝不崩溃。
        unsafe { apply_transparent_background(&webview) };
        // 回读生效值并留痕:透明失败时菜单背景退化为页面自绘,这里给出
        // 一眼可判的部署证据(每次进程至多一行)。
        let effective = unsafe { draws_background(&webview) };
        eprintln!("mornlea webview: 挂载完成 drawsBackground={effective:?}");

        // 挂到 winit contentView 之上,frame 跟随窗口(autoresizing 双轴)。
        let content_view: *mut AnyObject = unsafe { msg_send![ns_window, contentView] };
        if content_view.is_null() {
            return None;
        }
        // SAFETY: contentView 判空后取 bounds;NSRect 按值返回不借用 ObjC 内存。
        let bounds: NSRect = unsafe { msg_send![content_view, bounds] };
        let _: () = unsafe { msg_send![&webview, setFrame: bounds] };
        // NSViewWidthSizable(2) | NSViewHeightSizable(16):窗口 resize 时
        // WebView 跟随,无需手工同步。
        let _: () = unsafe { msg_send![&webview, setAutoresizingMask: 18usize] };
        let _: () = unsafe { msg_send![content_view, addSubview: &*webview] };

        // 初始隐藏:游戏相位/离屏构造不参与,首个菜单状态推送时才显示。
        let _: () = unsafe { msg_send![&webview, setHidden: true] };

        unsafe {
            webview.setNavigationDelegate(Some(ProtocolObject::from_ref(&*delegate)));
        }

        // 加载页面;资产由 scheme handler 供给。
        let entry = NSURL::URLWithString(&NSString::from_str(ENTRY_URL))?;
        let request = NSURLRequest::requestWithURL(&entry);
        unsafe { webview.loadRequest(&request) };

        // spike 挂载留痕放在这里:视图已入树、页面已开始加载,与 Drop 侧的
        // 卸载留痕严格成对;生产路径(未设环境变量)静默。
        crate::overlay_spike::log_mount(spike_forced);
        Some(Self {
            webview,
            content_controller: controller,
            shared,
            spike_forced,
            game_view,
            menu_participating: false,
        })
    }

    /// 下行状态推送:浅校验 → 相位推导参与模式 → 缓存 → 相位路由 → 幂等求值。
    ///
    /// 浅校验只做「可解析 + phase 字段存在」(相位决定参与模式,必须认识);
    /// schema 深校验(枚举、上界、未知字段)由 Go 组装侧与前端解析侧承担。
    /// 参与模式由相位推导(`crate::overlay::mode_for_phase`),spike 遗留
    /// 强制档位存在时被钉死。求值只在视图实际可见时发起:GameOverlay 态视图
    /// 常驻可见,游戏相位的 HUD 状态因此持续下行;视图真被隐藏时只更新缓存
    /// (零参与:无求值、无消息)。
    pub fn push_state(&mut self, json: &[u8]) -> Result<(), StateError> {
        let text = std::str::from_utf8(json).map_err(|_| StateError::Malformed)?;
        let value: serde_json::Value =
            serde_json::from_str(text).map_err(|_| StateError::Malformed)?;
        let phase = value
            .get("phase")
            .and_then(|phase| phase.as_str())
            .ok_or(StateError::Malformed)?;
        let wants_visible = state_wants_visible_parsed(phase, &value);
        let mode = self
            .spike_forced
            .unwrap_or_else(|| crate::overlay::mode_for_phase(wants_visible));
        let transition = crate::overlay::plan_transition(mode, wants_visible);
        *self.shared.last_state.lock().expect("桥状态缓存锁中毒") = Some(text.to_owned());
        if wants_visible != self.menu_participating {
            self.menu_participating = wants_visible;
            self.apply_transition(mode, transition);
        }
        if !transition.hide_view {
            self.evaluate_if_changed();
        }
        Ok(())
    }

    /// 幂等求值:仅当缓存状态与本文档已求值文本不同时才发起。
    fn evaluate_if_changed(&mut self) {
        let state = self
            .shared
            .last_state
            .lock()
            .expect("桥状态缓存锁中毒")
            .clone();
        let Some(state) = state else { return };
        if self
            .shared
            .last_evaluated
            .lock()
            .expect("桥幂等基准锁中毒")
            .as_deref()
            == Some(state.as_str())
        {
            return;
        }
        evaluate_state(&self.webview, &state);
        *self.shared.last_evaluated.lock().expect("桥幂等基准锁中毒") = Some(state);
    }

    /// 向页面求值一段脚本,结果经 `spike_auto` 的完成回调写回收件箱。
    ///
    /// 只被 spike 自驱动档调用(自动进入游戏的按钮查找与 `.click()`);
    /// 生产路径的下行状态推送仍走 [`evaluate_state`],不携带完成回调。
    ///
    /// # Safety
    ///
    /// 主线程方法(`MenuWebview` 的既有线程约束);完成回调由 WebKit 在主
    /// 线程投递,`RcBlock` 在调用返回后仍被 WebKit 持有,闭包只触碰进程级
    /// 单例,不借用本对象。
    pub(crate) fn spike_evaluate(&self, script: &str) {
        let js = NSString::from_str(script);
        let handler = crate::spike_auto::js_completion_block();
        // SAFETY: 主线程;handler 由 RcBlock 保活到调用结束,WebKit 侧按
        // Block 语义自行复制持有。
        unsafe {
            self.webview
                .evaluateJavaScript_completionHandler(&js, Some(&*handler))
        };
    }

    /// 应用一次参与模式切换:动作表落到 AppKit(视图显隐、`hitTest:` 穿透、
    /// firstResponder 归属)并把页面切到对应相位呈现。
    ///
    /// 动作表由 [`crate::overlay::plan_transition`] 推导,是参与语义的唯一
    /// 真相:`Menu` 态复现既有行为——隐藏即归还焦点、显示即夺取焦点;
    /// `GameOverlay` 态在游戏相位改为「保持可见 + `hitTest:` 穿透」,焦点仍
    /// 归还 winit,合成与响应链参与就此解耦。GameOverlay 态同时注入页面
    /// 相位透明(菜单页的不透明背景会盖住 wgpu 画面),菜单相位撤除注入。
    /// 调用点只在相位翻转处,重复推送不重放 AppKit 动作。
    fn apply_transition(
        &mut self,
        mode: OverlayMode,
        transition: crate::overlay::OverlayTransition,
    ) {
        self.shared.overlay.set_mode(mode);
        self.shared.overlay.apply(transition);
        crate::overlay_spike::log_phase_transition(mode, self.menu_participating);
        // SAFETY: 主线程(与全部 FFI 窗口出口一致的线程约束);webview 存续。
        unsafe { apply_page_phase(&self.webview, mode == OverlayMode::GameOverlay) };
        let _: () = unsafe { msg_send![&self.webview, setHidden: transition.hide_view] };
        // NSView.window:从自身视图取回宿主窗口,避免跨调用持有窗口指针。
        let window: *mut AnyObject = unsafe { msg_send![&self.webview, window] };
        if transition.focus_game_view {
            // 游戏相位:焦点归还 winit 视图,键盘恢复 winit 独占。
            let _: () = unsafe { msg_send![window, makeFirstResponder: self.game_view] };
        } else {
            // 菜单相位:WebView 夺取焦点消费键盘/指针,winit 侧静默。
            let _: () = unsafe { msg_send![window, makeFirstResponder: &*self.webview] };
        }
    }
}

impl Drop for MenuWebview {
    fn drop(&mut self) {
        // spike 卸载留痕:与挂载留痕成对,验证臂多记一行,生产路径静默。
        crate::overlay_spike::log_unmount(self.spike_forced);
        // 解除脚本 handler 注册(userContentController 强持有回调对象,
        // 必须先断开,避免 WebKit 卸载顺序上向失效对象投递)。
        unsafe {
            self.content_controller
                .removeScriptMessageHandlerForName(&NSString::from_str("mornlea"));
        }
        // 只操作自身视图,不触碰窗口指针:窗口可能已被先行销毁。
        let _: () = unsafe { msg_send![&self.webview, removeFromSuperview] };
    }
}

#[cfg(test)]
mod tests {
    use super::{
        StateError, evaluate_state_script, state_wants_visible, state_wants_visible_parsed,
    };
    use crate::overlay::{OverlayMode, mode_for_phase, plan_transition};

    /// 相位枚举全值覆盖:菜单族(menu/starting/loading/settings/paused)与
    /// 叠加可见调试面板的游戏相位都要求 WebView 全参与,只有无 chrome 的
    /// 游戏相位进入 GameOverlay。这是参与模式推导的唯一输入,漏一个相位值
    /// 就会让该相位既没有 chrome 又吞输入。
    #[test]
    fn phase_visibility_covers_the_schema_phase_enum() {
        for phase in ["menu", "starting", "loading", "settings", "paused"] {
            assert!(
                state_wants_visible_parsed(phase, &serde_json::json!({"phase": phase})),
                "phase={phase} 必须全参与"
            );
        }
        assert!(!state_wants_visible_parsed(
            "game",
            &serde_json::json!({"phase": "game"})
        ));
        // debug.visible 叠加在游戏相位:调试面板需要键盘与指针,回到全参与。
        assert!(state_wants_visible_parsed(
            "game",
            &serde_json::json!({"phase": "game", "debug": {"visible": true}})
        ));
        // debug.visible 为 false 的游戏相位仍是 GameOverlay。
        assert!(!state_wants_visible_parsed(
            "game",
            &serde_json::json!({"phase": "game", "debug": {"visible": false}})
        ));
    }

    /// 浅校验的拒绝面:非 UTF-8、非 JSON、非对象、缺 phase、phase 非字符串
    /// 一律 `Malformed`,绝不触碰运行态——这是「未知状态类型/非法 UTF-8
    /// 被拒绝且不触碰呈现状态」契约在 Rust 侧的落点。
    #[test]
    fn malformed_state_is_rejected_without_touching_runtime() {
        assert_eq!(state_wants_visible(b"\xff\xfe"), Err(StateError::Malformed));
        assert_eq!(
            state_wants_visible(b"not json at all"),
            Err(StateError::Malformed)
        );
        assert_eq!(state_wants_visible(b"[1,2,3]"), Err(StateError::Malformed));
        assert_eq!(state_wants_visible(b"{}"), Err(StateError::Malformed));
        assert_eq!(
            state_wants_visible(br#"{"phase":7}"#),
            Err(StateError::Malformed)
        );
    }

    /// 未知相位字符串兜底为全参与(Menu):Rust 浅校验只认识「非 game 即
    /// 菜单族」,schema 相位枚举新增值或大小写漂移时会先落到「WebView 全
    /// 参与」这一侧——宁可让页面消费键盘,也不让未知相位进入 GameOverlay
    /// 后既无 chrome 又吞输入。枚举值的合法性由 Go 组装侧与 TS 守卫的深
    /// 校验承担,非法值在下行前就被拒绝。
    #[test]
    fn unknown_phase_falls_back_to_full_participation() {
        for phase in ["lobby", "GAME", "Game", "", "游戏", "game "] {
            let state = serde_json::json!({ "phase": phase });
            assert!(
                state_wants_visible_parsed(phase, &state),
                "phase={phase:?} 必须兜底为全参与"
            );
            assert_eq!(
                mode_for_phase(state_wants_visible_parsed(phase, &state)),
                OverlayMode::Menu,
                "phase={phase:?} 参与模式必须是 Menu"
            );
        }
    }

    /// hud 状态族的「Rust 半部」是**显式不校验**:下行状态在 Rust 侧只做
    /// 相位浅校验(`phase` 存在与否 + `debug.visible`),不把 `uiState` 反
    /// 序列化成类型,因此桥 schema 单源(`frontend/src/bridge/schema.json`)
    /// 的 hudState 族钉值由 Go 组装侧与 TS 守卫承接,Rust 不另立类型钉值
    /// ——上行信封那半部(`bridge` 的协议常量对 `$defs.uplinkEnvelope` 钉值)
    /// 与本条合起来才是三端钉值中 Rust 侧的完整边界。本测试钉住该边界:
    /// hud 族的字段值不参与参与模式推导,越界/未知 hud 字段也不改变相位判定。
    #[test]
    fn hud_family_stays_outside_the_rust_validation_surface() {
        // 相位判定只由 phase 字段决定,hud 内容(哪怕越界)不改变它。
        assert!(!state_wants_visible_parsed(
            "game",
            &serde_json::json!({"phase": "game", "hud": {"hotbar": {"selected": 99}}})
        ));
        assert!(state_wants_visible_parsed(
            "menu",
            &serde_json::json!({"phase": "menu", "hud": {"health": -1, "unknown": true}})
        ));
        // hud 族缺席与存在等价:字段缺席不构成 Malformed,浅校验面不扩大。
        assert_eq!(
            state_wants_visible(br#"{"phase":"game"}"#),
            state_wants_visible(br#"{"phase":"game","hud":{}}"#),
            "hud 族存在与否不得改变浅校验结果"
        );
    }

    /// 生产路径(无 spike 强制)下,参与模式完全由相位推导:菜单相位 →
    /// Menu(隐藏动作表与既有行为一致),游戏相位 → GameOverlay(可见 +
    /// 穿透 + 焦点归还 winit)。GameOverlay 态下状态求值必须继续,游戏相
    /// 位的 HUD 下行才进得了页面。
    #[test]
    fn production_mode_follows_phase_and_keeps_game_phase_wired() {
        // 游戏相位:GameOverlay,视图不隐藏,状态持续下行(渲染接线的前提)。
        let mode = mode_for_phase(false);
        let transition = plan_transition(mode, false);
        assert_eq!(mode, OverlayMode::GameOverlay);
        assert!(!transition.hide_view, "GameOverlay 态必须持续合成");
        assert!(transition.hit_test_passthrough);
        assert!(transition.focus_game_view);

        // 菜单相位:Menu,既有行为逐项一致。
        let mode = mode_for_phase(true);
        let transition = plan_transition(mode, true);
        assert_eq!(mode, OverlayMode::Menu);
        assert!(!transition.hide_view);
        assert!(!transition.hit_test_passthrough);
        assert!(!transition.focus_game_view);
    }

    /// spike 遗留强制档位的语义收敛:强制档位只改「模式来源」,不新增动作
    /// 语义——菜单相位下两态动作逐项一致,菜单 chrome 可点与模式无关。
    /// 强制 Menu 的游戏相位即旧隐藏语义,已由
    /// `crate::overlay::tests::menu_mode_reproduces_legacy_semantics` 钉住,
    /// 这里不重复;强制 GameOverlay 与生产 GameOverlay 是同一个枚举值,共用
    /// 同一张动作表,不构成独立断言。
    #[test]
    fn spike_forced_modes_converge_on_the_production_action_table() {
        for mode in [OverlayMode::Menu, OverlayMode::GameOverlay] {
            assert_eq!(
                plan_transition(mode, true),
                plan_transition(OverlayMode::Menu, true),
                "mode={mode:?}: 菜单相位动作必须与 Menu 态逐项一致"
            );
        }
    }

    /// 状态求值脚本必须把状态原文作为 JS 表达式内插(`window.mornlea.onState`
    /// 的单一下行通道),并自带桥全局就绪重试;脚本形态变化会破坏页面桥约定。
    #[test]
    fn state_script_forwards_state_expression() {
        let script = evaluate_state_script(r#"{"phase":"menu"}"#);
        assert!(script.contains(r#"var s={"phase":"menu"};"#));
        assert!(script.contains("window.mornlea.onState(s)"));
        assert!(script.contains("setInterval"));
        assert!(script.ends_with("}})()"));
    }

    /// `evaluate_state` 的脚本是纯函数 [`evaluate_state_script`] 的产物;
    /// 这里钉住脚本的自我重试边界:10 秒后放弃,不留给页面常驻定时器。
    #[test]
    fn state_script_retry_window_is_bounded() {
        let script = evaluate_state_script("{}");
        assert!(script.contains("10000"));
        assert!(script.contains("16"));
    }
}

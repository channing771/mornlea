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
//! - **相位路由**:菜单相位显示并夺取 firstResponder(键盘/指针交给页面,
//!   winit 不再收到按键,游戏输入天然静默);游戏相位隐藏并把
//!   firstResponder 归还 winit 内容视图(键盘不双投、不丢)。WebView 隐藏
//!   时不做任何求值,页面 DOM 状态在隐藏期间保持,重新显示无需重置。
//! - **页面就绪重推**:WebView 是异步加载的,加载完成前的求值会丢失;
//!   `didFinishNavigation` 后把缓存的最近状态重推给新文档,保证首屏有状态。
//!   同一回调也是渲染进程崩溃(`webViewWebContentProcessDidTerminate`)后的
//!   自愈通道:崩溃即重载页面,重载完成走既有的就绪重推,菜单呈现自行恢复。
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

/// 内嵌菜单前端资产(vite build 产物,dist 入库)。
static EMBEDDED_INDEX_HTML: &[u8] = include_bytes!("../frontend/dist/index.html");
static EMBEDDED_INDEX_JS: &[u8] = include_bytes!("../frontend/dist/assets/index.js");
static EMBEDDED_INDEX_CSS: &[u8] = include_bytes!("../frontend/dist/assets/index.css");

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
/// 挂在窗口侧,两侧经同一单例交汇——与旧 egui 路径「全局事件队列 + 渲染器
/// 逐帧 take」的拓扑一致。类型整体 Send+Sync,但按线程模型只在主线程访问。
struct HostShared {
    queue: SharedUiEventQueue,
    /// 最近一次 `push_state` 收到的状态 JSON 原文(浅校验过)。
    last_state: Mutex<Option<String>>,
    /// 最近一次实际求值到**当前文档**的状态文本;幂等转发的判定基准。
    last_evaluated: Mutex<Option<String>>,
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
        let shared = self.ivars();
        // 容量护栏兜底:页面事件洪泛时整信封拒绝,不阻塞主线程。
        let _ = shared.queue.enqueue_envelope(&data.to_vec());
    }

    /// 页面导航完成:新文档没有任何状态,把缓存的最近状态重推一次。
    /// 重推同时刷新幂等基准(`last_evaluated`),保证缓存真正落到新文档。
    /// 宿主 WebView 由 delegate 消息参数给出,不经共享状态中转。
    fn did_finish_navigation(&self, webview: &WKWebView) {
        let shared = self.ivars();
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
    let script = NSString::from_str(&format!(
        "(function(){{var s={state};function d(){{if(window.mornlea&&window.mornlea.onState){{window.mornlea.onState(s);return true;}}return false;}}if(!d()){{var t=setInterval(function(){{if(d())clearInterval(t);}},16);setTimeout(function(){{clearInterval(t);}},10000);}}}})()"
    ));
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

/// 浅校验一份状态并报告它是否要求 WebView 可见:非 game 相位恒可见;
/// game 相位叠加可见的 debug 面板(`debug.visible`)同样要求可见。
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

/// 对已解析状态计算可见性意图;与 [`MenuWebview::push_state`] 共用判定。
fn state_wants_visible_parsed(phase: &str, value: &serde_json::Value) -> bool {
    // debug.visible 可叠加在任意相位(含 game):面板可见时 WebView 显示。
    let debug_visible = value.pointer("/debug/visible").and_then(|v| v.as_bool()) == Some(true);
    phase != "game" || debug_visible
}

/// 一个挂载在 winit 窗口上的菜单 WebView。生命周期由持有它的渲染器管理:
/// drop 时解除脚本 handler 注册并把自身移出视图树,不触碰窗口指针。
pub struct MenuWebview {
    webview: Retained<WKWebView>,
    /// 建立脚本 handler 注册的控制器;teardown 时据此解除注册。
    content_controller: Retained<WKUserContentController>,
    /// 共享桥状态(与回调对象同源)。
    shared: Arc<HostShared>,
    /// winit 内容视图指针(游戏相位归还 firstResponder 的目标)。
    /// 与窗口同生命周期;只在窗口存续的正常运行路径上解引用。
    game_view: *mut AnyObject,
    visible: bool,
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
        let shared = Arc::new(HostShared {
            queue: crate::bridge::shared_queue().clone(),
            last_state: Mutex::new(None),
            last_evaluated: Mutex::new(None),
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
        let webview = unsafe {
            WKWebView::initWithFrame_configuration(WKWebView::alloc(mtm), frame, &config)
        };
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

        Some(Self {
            webview,
            content_controller: controller,
            shared,
            game_view,
            visible: false,
        })
    }

    /// 下行状态推送:浅校验 → 缓存 → 相位路由 → 幂等求值。
    ///
    /// 浅校验只做「可解析 + phase 字段存在」(相位决定可见性,必须认识);
    /// schema 深校验(枚举、上界、未知字段)由 Go 组装侧与前端解析侧承担。
    /// 求值仅在显示相位发生;隐藏相位只更新缓存(零参与:无求值、无消息)。
    pub fn push_state(&mut self, json: &[u8]) -> Result<(), StateError> {
        let text = std::str::from_utf8(json).map_err(|_| StateError::Malformed)?;
        let value: serde_json::Value =
            serde_json::from_str(text).map_err(|_| StateError::Malformed)?;
        let phase = value
            .get("phase")
            .and_then(|phase| phase.as_str())
            .ok_or(StateError::Malformed)?;
        let wants_visible = state_wants_visible_parsed(phase, &value);
        *self.shared.last_state.lock().expect("桥状态缓存锁中毒") = Some(text.to_owned());
        if wants_visible != self.visible {
            self.set_visible(wants_visible);
        }
        if self.visible {
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

    /// 切换显示/隐藏与 firstResponder 归属。
    fn set_visible(&mut self, visible: bool) {
        if self.visible == visible {
            return;
        }
        self.visible = visible;
        let _: () = unsafe { msg_send![&self.webview, setHidden: !visible] };
        // NSView.window:从自身视图取回宿主窗口,避免跨调用持有窗口指针。
        let window: *mut AnyObject = unsafe { msg_send![&self.webview, window] };
        if visible {
            // 菜单相位:WebView 夺取焦点消费键盘/指针,winit 侧静默。
            let _: () = unsafe { msg_send![window, makeFirstResponder: &*self.webview] };
        } else {
            // 游戏相位:焦点归还 winit 视图,键盘恢复 winit 独占。
            let _: () = unsafe { msg_send![window, makeFirstResponder: self.game_view] };
        }
    }
}

impl Drop for MenuWebview {
    fn drop(&mut self) {
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

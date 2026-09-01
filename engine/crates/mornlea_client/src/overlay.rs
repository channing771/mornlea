//! GameOverlay 参与模型(生产实现)。
//!
//! WebView 的响应链参与恰有两态——`Menu`(菜单相位,全参与)与 `GameOverlay`
//! (游戏相位,仅合成),枚举见 [`OverlayMode`]。模式由下行状态的相位字段
//! 推导(`crate::webview::MenuWebview::push_state` 是唯一切换点,与既有
//! 「装配完成隐藏 WebView / 退回主菜单显示」生命周期同源),切换动作经
//! [`plan_transition`] 落到 AppKit:
//!
//! - **Menu**:视图可见、`hitTest:` 走父类实现、firstResponder 归 WebView
//!   ——菜单 chrome 可点,键盘由页面路由。
//! - **GameOverlay**:视图**保持可见**(透明合成于 wgpu 画面之上,承载常显
//!   HUD)、`hitTest:` 返回 `nil`(命中测试跳过本视图子树,事件落到下层
//!   winit 内容视图)、firstResponder 归还 winit——可见合成与响应链参与
//!   解耦,输入行为与无 WebView 基线逐项一致(前置 spike S1 实证)。
//!
//! benchmark/capture/`-connect` 无头路径零参与不因本模型改变:WebView 只在
//! 「需要呈现菜单」的状态推送时挂载,纯游戏相位推送与挂载失败后的推送都不
//! 触发挂载(挂载门谓词 [`crate::window`] 侧钉住)。
//!
//! 线程约束:子类构造与模式切换必须在创建窗口的 OS 主线程调用(与
//! [`crate::webview`] 的既有约束一致);`hitTest:` 被 AppKit 在主线程高频
//! 调用,穿透标志因此用原子量,免锁;纯函数与页面相位脚本无线程要求。

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU8, Ordering};

use objc2::rc::Retained;
use objc2::runtime::AnyObject;
use objc2::{DefinedClass, MainThreadMarker, MainThreadOnly, define_class, msg_send};
use objc2_foundation::{NSPoint, NSRect, NSString};
use objc2_web_kit::{WKWebView, WKWebViewConfiguration};

/// WebView 参与模式(生产枚举)。
///
/// 两态互斥且完备:菜单相位需要 chrome 可交互,游戏相位需要 HUD 常驻合成
/// 又必须把输入全部让给 winit。没有第三态——「既不可见也不参与」的旧隐藏
/// 语义已被 GameOverlay 取代(见 [`plan_transition`] 的动作表)。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum OverlayMode {
    /// 菜单相位:WebView 全参与(既有行为),菜单 chrome 可点、键盘归页面。
    #[default]
    Menu,
    /// 游戏相位:WebView 仅合成(可见 + `hitTest:` 穿透),输入归 winit。
    GameOverlay,
}

/// 由「下行状态是否要求 WebView 以菜单形态全参与」推导参与模式。
///
/// 下行相位的菜单族(menu/starting/settings/paused,以及叠加可见调试面板的
/// game 相位——面板需要键盘与指针)都要求全参与;只有无 chrome 的游戏相位
/// 进入 GameOverlay。可见性与参与度在同一字段上编码,`crate::webview` 侧的
/// 相位解析是唯一驱动源。
pub(crate) fn mode_for_phase(phase_wants_visible: bool) -> OverlayMode {
    if phase_wants_visible {
        OverlayMode::Menu
    } else {
        OverlayMode::GameOverlay
    }
}

/// 一次相位切换要落到 AppKit 的动作集合。
///
/// 三个动作覆盖两态的全部差异:`hide_view` 决定视图是否还在窗口里(GameOverlay
/// 恒为 false——合成不能停),`hit_test_passthrough` 决定 `hitTest:` 是否
/// 返回 `nil`(只有子类实例消费它),`focus_game_view` 决定 firstResponder
/// 归属(键盘不双投、不丢)。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct OverlayTransition {
    /// 是否把 WebView 视图置为隐藏。
    pub hide_view: bool,
    /// `hitTest:` 是否返回 nil(事件穿透到 winit 内容视图)。
    pub hit_test_passthrough: bool,
    /// 是否把 firstResponder 归还 winit 渲染视图。
    pub focus_game_view: bool,
}

/// 由参与模式与相位推导切换动作。
///
/// `Menu` 态的表项就是既有生产语义(游戏相位隐藏 + 归还焦点、菜单相位显示
/// + WebView 夺取焦点),由测试钉住;`GameOverlay` 态在游戏相位改为「不隐藏
/// + 穿透」,焦点仍归还 winit。
///
/// 菜单相位下两态动作一致:菜单 chrome 必须可点,这一点与模式无关。
pub(crate) fn plan_transition(mode: OverlayMode, phase_wants_visible: bool) -> OverlayTransition {
    match (mode, phase_wants_visible) {
        // 菜单相位:两态一致,WebView 夺取焦点消费键盘/指针。
        (_, true) => OverlayTransition {
            hide_view: false,
            hit_test_passthrough: false,
            focus_game_view: false,
        },
        // 游戏相位 + Menu 态:既有语义——隐藏 + 归还焦点(键盘回到 winit)。
        (OverlayMode::Menu, false) => OverlayTransition {
            hide_view: true,
            hit_test_passthrough: false,
            focus_game_view: true,
        },
        // 游戏相位 + GameOverlay 态:保持可见以持续合成 HUD,但退出响应链。
        (OverlayMode::GameOverlay, false) => OverlayTransition {
            hide_view: false,
            hit_test_passthrough: true,
            focus_game_view: true,
        },
    }
}

/// [`OverlayMode`] 的原子存储编码;模式只有两个值,`u8` 足够且免于互斥量。
fn encode_mode(mode: OverlayMode) -> u8 {
    match mode {
        OverlayMode::Menu => 0,
        OverlayMode::GameOverlay => 1,
    }
}

/// Rust 侧与子类 ivar 共享的参与模式运行态。
///
/// 两个标志都是原子量:`hitTest:` 在主线程被 AppKit 高频调用,读取路径不能
/// 加锁;写入点(`set_mode`/`apply`)也只在主线程的相位切换处发生,原子量
/// 只是让 ivar 类型免于互斥量与借用约束。`MenuWebview` 持有本结构的 `Arc`,
/// 导航回调侧经共享宿主读取当前模式,保证「页面重载后重放相位样式」与
/// 「相位切换」看到同一份真相。
#[derive(Debug, Default)]
pub(crate) struct OverlayRuntime {
    /// 当前参与模式(见 [`encode_mode`])。
    mode: AtomicU8,
    /// `hitTest:` 子类实现读取的穿透标志。
    passthrough: AtomicBool,
}

impl OverlayRuntime {
    /// 建立运行态;默认 `Menu`(穿透关闭)——保守缺省,绝不让
    /// 尚未收到任何下行状态的视图吞掉输入。
    pub(crate) fn new() -> Arc<Self> {
        Arc::new(Self::default())
    }

    /// 当前参与模式。
    pub(crate) fn mode(&self) -> OverlayMode {
        match self.mode.load(Ordering::Relaxed) {
            1 => OverlayMode::GameOverlay,
            _ => OverlayMode::Menu,
        }
    }

    /// 写入当前参与模式;只有相位切换点调用,重写同值是幂等空操作。
    pub(crate) fn set_mode(&self, mode: OverlayMode) {
        self.mode.store(encode_mode(mode), Ordering::Relaxed);
    }

    /// 应用一次切换动作:只有穿透标志需要落到子类 ivar,视图隐藏与焦点由
    /// 调用方在 AppKit 侧执行。
    pub(crate) fn apply(&self, transition: OverlayTransition) {
        self.passthrough
            .store(transition.hit_test_passthrough, Ordering::Relaxed);
    }

    /// `hitTest:` 子类实现读取的穿透标志。
    pub(crate) fn hit_test_passthrough(&self) -> bool {
        self.passthrough.load(Ordering::Relaxed)
    }
}

define_class!(
    // 只声明直接父类:本模块只需要 `Retained<OverlayWebView> -> Retained<WKWebView>`
    // 的一层上转,不需要跨多层祖先的转换;更深的继承链由 objc2-web-kit 自己声明。
    #[unsafe(super(WKWebView))]
    #[thread_kind = MainThreadOnly]
    #[ivars = Arc<OverlayRuntime>]
    #[name = "MornleaOverlayWebView"]
    /// WKWebView 子类:唯一目的是 override `hitTest:` 以实现 GameOverlay 态的
    /// 响应链穿透。其余行为全部继承父类,不触碰资产、桥与生命周期逻辑。
    ///
    /// 只覆盖 [`WKWebView`] 的指定初始化器 `initWithFrame:configuration:`,
    /// ivars 在 `alloc` 后、`super` 实现执行前写入;其余初始化器不被使用
    /// (视图由本 crate 自行构造,不参与归档或 nib 装载)。
    pub(crate) struct OverlayWebView;

    impl OverlayWebView {
        /// `hitTest:` 分级:穿透态返回 `nil`,让 AppKit 的命中测试跳过本视图
        /// 整个子树,事件落到下层 winit 内容视图;非穿透态原样走父类实现,
        /// 与裸 WKWebView 的命中行为一致(菜单相位全参与的前提)。
        ///
        /// 返回类型用 `method_id` 声明:`hitTest:` 返回 autoreleased 的视图
        /// 对象,`Option<Retained<AnyObject>>` 经 objc2 的 autorelease 语义
        /// 归还所有权,与父类 ABI 一致(`nil` 即穿透)。方法体必须是表达式
        /// 形式——`define_class!` 会把整个 body 包进结果转换,早退 `return`
        /// 会绕过该转换。
        #[unsafe(method_id(hitTest:))]
        unsafe fn hit_test(&self, point: NSPoint) -> Option<Retained<AnyObject>> {
            if self.ivars().hit_test_passthrough() {
                None
            } else {
                // SAFETY: super(self) 命中 WKWebView 的实现;point 按值传入,
                // 返回的对象指针经 Retained 接管与父类 ABI 一致。
                unsafe { msg_send![super(self), hitTest: point] }
            }
        }
    }
);

impl OverlayWebView {
    /// 构造子类实例:与 [`WKWebView::initWithFrame_configuration`] 等价的
    /// 入参,`runtime` 作为 ivar 与 [`crate::webview`] 侧共享。
    ///
    /// # Safety
    ///
    /// 只能在主线程调用(`mtm` 证明);`config` 必须是主线程上存续的有效
    /// 配置对象,`frame` 为窗口坐标系内的有限矩形。
    pub(crate) unsafe fn new(
        frame: NSRect,
        config: &WKWebViewConfiguration,
        runtime: Arc<OverlayRuntime>,
        mtm: MainThreadMarker,
    ) -> Retained<Self> {
        let this = mtm.alloc::<Self>().set_ivars(runtime);
        // SAFETY: `initWithFrame:configuration:` 是 WKWebView 的指定初始化器,
        // 走 super 实现完成父类侧初始化;ivar 已在 alloc 后写入,WebKit 要求
        // 的主线程与配置有效性由安全前提承担。
        unsafe { msg_send![super(this), initWithFrame: frame, configuration: config] }
    }
}

/// GameOverlay 态页面相位脚本的样式模板;`{on}` 由 [`apply_page_phase`]
/// 替换为 `true`/`false`。
///
/// 菜单页面自带不透明背景,GameOverlay 常驻时会把 wgpu 画面整窗盖住。脚本
/// 在 `<html>` 上翻转 `mornlea-game-phase` 类并按需注入一条只改 `body`
/// 背景的样式,游戏相位由此透明合成于既有 wgpu 呈现之上;菜单相位撤除类,
/// 页面回到自绘背景。选择器只锚定 `index.html` 的 `<html>`/`<body>` 稳定
/// 节点,不触碰任何组件类名,前端重构不会破坏注入。
const PAGE_PHASE_SCRIPT: &str = "(function(){var on={on};function d(){var e=document.documentElement;if(!e)return false;e.classList.toggle('mornlea-game-phase',on);if(on&&!document.getElementById('mornlea-game-phase-style')){var s=document.createElement('style');s.id='mornlea-game-phase-style';s.textContent='html.mornlea-game-phase body{background:transparent !important}';(document.head||e).appendChild(s);}return true;}if(!d()){var t=setInterval(function(){if(d())clearInterval(t);},16);setTimeout(function(){clearInterval(t);},10000);}})()";

/// 生成页面相位脚本;`game_overlay` 表示是否处于 GameOverlay 态。
fn page_phase_script(game_overlay: bool) -> String {
    PAGE_PHASE_SCRIPT.replace("{on}", if game_overlay { "true" } else { "false" })
}

/// 把页面切到 GameOverlay 态呈现:游戏相位启用透明合成,菜单相位撤除。
///
/// 注入自带重试(文档尚未建立根元素时 16ms 重投一次,10 秒后放弃),与状态
/// 求值的自愈策略一致;脚本是幂等的——重复注入只让 DOM 收敛到同一份样式,
/// 不会叠加样式元素。页面重载(首次加载与 WebContent 崩溃自愈)会丢失注入,
/// 调用方在导航完成回调里按当前参与模式重放。
///
/// 这是 C 组任务落地 HUD 组件前的过渡接线:HUD 页面自带透明样式后,本注入
/// 与样式常量一并移除,相位透明改由前端承担。
///
/// # Safety
///
/// 调用方必须在主线程(`WKWebView` 的线程要求),`webview` 为存续对象。
pub(crate) unsafe fn apply_page_phase(webview: &WKWebView, game_overlay: bool) {
    let script = NSString::from_str(&page_phase_script(game_overlay));
    // SAFETY: 主线程调用;完成回调为 None(重试在脚本内自愈)。
    unsafe { webview.evaluateJavaScript_completionHandler(&script, None) };
}

#[cfg(test)]
mod tests {
    use objc2::{ClassType, sel};
    use objc2_web_kit::WKWebView;

    use super::{
        OverlayMode, OverlayRuntime, OverlayTransition, OverlayWebView, encode_mode,
        mode_for_phase, page_phase_script, plan_transition,
    };

    /// Menu 态动作表就是既有生产语义:游戏相位隐藏 + 归还焦点,菜单相位
    /// 显示 + WebView 夺取焦点,且从不穿透。这是菜单 chrome 可点的前提。
    #[test]
    fn menu_mode_reproduces_legacy_semantics() {
        assert_eq!(
            plan_transition(OverlayMode::Menu, false),
            OverlayTransition {
                hide_view: true,
                hit_test_passthrough: false,
                focus_game_view: true,
            }
        );
        assert_eq!(
            plan_transition(OverlayMode::Menu, true),
            OverlayTransition {
                hide_view: false,
                hit_test_passthrough: false,
                focus_game_view: false,
            }
        );
    }

    /// GameOverlay 态的核心契约:游戏相位保持可见合成但不参与响应链,焦点
    /// 仍归还 winit;菜单相位回到全参与,菜单 chrome 可点。
    #[test]
    fn game_overlay_keeps_visible_but_stops_hit_testing_in_game_phase() {
        assert_eq!(
            plan_transition(OverlayMode::GameOverlay, false),
            OverlayTransition {
                hide_view: false,
                hit_test_passthrough: true,
                focus_game_view: true,
            }
        );
        assert_eq!(
            plan_transition(OverlayMode::GameOverlay, true),
            OverlayTransition {
                hide_view: false,
                hit_test_passthrough: false,
                focus_game_view: false,
            }
        );
    }

    /// 模式推导:菜单相位(以及叠加可见调试面板的游戏相位)→ Menu,
    /// 无 chrome 的游戏相位 → GameOverlay;两态枚举恰好被覆盖。
    #[test]
    fn mode_for_phase_covers_both_states() {
        assert_eq!(mode_for_phase(true), OverlayMode::Menu);
        assert_eq!(mode_for_phase(false), OverlayMode::GameOverlay);
        assert_eq!(OverlayMode::default(), OverlayMode::Menu);
    }

    /// 全组合不变式:穿透当且仅当「GameOverlay 态的游戏相位」;Menu 态在
    /// 任何相位都不穿透,GameOverlay 态在菜单相位不穿透。视图隐藏与 GameOverlay
    /// 也互斥——合成不能停,否则 HUD 消失。
    #[test]
    fn passthrough_holds_exactly_for_game_overlay_in_game_phase() {
        for (mode, wants_visible) in [
            (OverlayMode::Menu, false),
            (OverlayMode::Menu, true),
            (OverlayMode::GameOverlay, false),
            (OverlayMode::GameOverlay, true),
        ] {
            let transition = plan_transition(mode, wants_visible);
            assert_eq!(
                transition.hit_test_passthrough,
                mode == OverlayMode::GameOverlay && !wants_visible,
                "mode={mode:?} wants_visible={wants_visible}"
            );
            assert_eq!(
                transition.hide_view,
                mode == OverlayMode::Menu && !wants_visible,
                "mode={mode:?} wants_visible={wants_visible}"
            );
            // 焦点只在菜单相位交给 WebView:输入独占边界不因模式改变,
            // GameOverlay 态的游戏相位同样把 firstResponder 还给 winit。
            assert_eq!(
                transition.focus_game_view, !wants_visible,
                "mode={mode:?} wants_visible={wants_visible}"
            );
        }
    }

    /// 两态切换序列:菜单 → 游戏 → 暂停 → 菜单 → 游戏,模式由相位逐次推导,
    /// 穿透标志必须随相位来回恢复,不能在第二次进入游戏相位时丢失。
    #[test]
    fn phase_driven_round_trip_does_not_lose_state() {
        let runtime = OverlayRuntime::new();
        for (round, wants_visible) in [true, false, true, false, true].iter().enumerate() {
            let mode = mode_for_phase(*wants_visible);
            let transition = plan_transition(mode, *wants_visible);
            runtime.set_mode(mode);
            runtime.apply(transition);
            assert_eq!(runtime.mode(), mode, "round {round}: 模式应跟随相位");
            assert_eq!(
                runtime.hit_test_passthrough(),
                !*wants_visible,
                "round {round}: 穿透标志应与相位相反"
            );
        }
    }

    /// 运行态缺省:Menu 且不穿透——尚未收到任何下行状态的视图绝不吞输入。
    #[test]
    fn runtime_defaults_to_menu_without_passthrough() {
        let runtime = OverlayRuntime::new();
        assert_eq!(runtime.mode(), OverlayMode::Menu);
        assert!(!runtime.hit_test_passthrough());
        // 显式切到 GameOverlay 再切回:模式与穿透都能双向恢复。
        runtime.set_mode(OverlayMode::GameOverlay);
        runtime.apply(plan_transition(OverlayMode::GameOverlay, false));
        assert_eq!(runtime.mode(), OverlayMode::GameOverlay);
        assert!(runtime.hit_test_passthrough());
        runtime.set_mode(OverlayMode::Menu);
        runtime.apply(plan_transition(OverlayMode::Menu, true));
        assert_eq!(runtime.mode(), OverlayMode::Menu);
        assert!(!runtime.hit_test_passthrough());
    }

    /// 模式编码必须可逆:原子存储里只有两个合法值,未知值回落 Menu(保守
    /// 缺省),不允许出现第三态。
    #[test]
    fn mode_encoding_round_trips() {
        assert_eq!(encode_mode(OverlayMode::Menu), 0);
        assert_eq!(encode_mode(OverlayMode::GameOverlay), 1);
        assert_ne!(
            encode_mode(OverlayMode::Menu),
            encode_mode(OverlayMode::GameOverlay)
        );
    }

    /// 子类必须真正 override `hitTest:`:本类的方法实现与 WKWebView 解析到
    /// 的实现不同,穿透分支才可能在 GameOverlay 态生效。类对象查询不实例化
    /// 视图,也就不需要主线程;真实的命中行为由 spike 自驱动档在真实窗口
    /// 上验证(spike-checklist 第 6 节)。
    #[test]
    fn subclass_overrides_hit_test_on_top_of_web_kit() {
        let sel = sel!(hitTest:);
        let own = <OverlayWebView as ClassType>::class()
            .instance_method(sel)
            .expect("子类必须实现 hitTest:");
        let parent = <WKWebView as ClassType>::class()
            .instance_method(sel)
            .expect("WKWebView 必须解析到 hitTest: 实现");
        // 函数指针地址不保证唯一,clippy 禁止直接比较;`fn_addr_eq` 是官方
        // 认可的地址等价判定,这里只需回答「两个 IMP 是否同一份实现」。
        assert!(
            !std::ptr::fn_addr_eq(own.implementation(), parent.implementation()),
            "子类必须 override 父类的 hitTest: 实现"
        );
    }

    /// 页面相位脚本随模式翻转且只锚定稳定节点:`{on}` 必须被替换成字面量
    /// 布尔,样式选择器只改 `body` 背景,不得引用任何组件类名(前端重构
    /// 不应破坏注入);样式元素带固定 id,重复注入不会叠加。
    #[test]
    fn page_phase_script_flips_and_only_anchors_stable_nodes() {
        let on = page_phase_script(true);
        let off = page_phase_script(false);
        assert!(on.contains("var on=true;"));
        assert!(off.contains("var on=false;"));
        assert!(!on.contains("{on}"));
        for script in [&on, &off] {
            assert!(script.contains("document.documentElement"));
            assert!(script.contains("classList.toggle('mornlea-game-phase'"));
            assert!(script.contains("getElementById('mornlea-game-phase-style')"));
            assert!(
                script.contains("html.mornlea-game-phase body{background:transparent"),
                "GameOverlay 态必须让 wgpu 画面透出:{script}"
            );
        }
        // 样式注入被脚本内的 `on` 变量门控:关闭态走 `classList.toggle(...,on)`
        // 的 false 分支只撤类,不会新建样式元素——菜单相位回到页面自绘背景。
        assert!(on.contains("classList.toggle('mornlea-game-phase',on)"));
        assert!(on.contains("if(on&&!document.getElementById('mornlea-game-phase-style')"));
        // 与 spike 脚手架的区别:生产注入不淡化页面内容,调试面板保持可见。
        assert!(!on.contains("opacity"));
    }

    /// 页面相位脚本是合法的可求值表达式形态:以 IIFE 开头、平衡的花括号,
    /// 交由 WebView 求值前就先在 Rust 侧抓住结构性笔误。
    #[test]
    fn page_phase_script_is_balanced_iife() {
        for game_overlay in [true, false] {
            let script = page_phase_script(game_overlay);
            assert!(script.starts_with("(function(){"));
            assert!(script.ends_with("}})()") || script.ends_with("})()"));
            let opens = script.matches('{').count();
            let closes = script.matches('}').count();
            assert_eq!(opens, closes, "花括号必须平衡");
        }
    }

    /// `apply_page_phase` 的返回值语义:本测试只确认脚本能独立成串(生成
    /// 自纯函数),真实求值路径在主线程,由 spike 自驱动档覆盖。
    #[test]
    fn page_phase_script_is_self_contained() {
        let script = page_phase_script(true);
        assert!(script.contains("return true;"));
        assert!(script.contains("setInterval"));
        // 幂等:样式元素只在缺失时创建,重复求值不会叠加。
        assert_eq!(script.matches("createElement('style')").count(), 1);
    }
}

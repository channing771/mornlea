//! egui 菜单界面的唯一样式令牌来源:面板语言(深色半透明表面 + 1 逻辑点
//! 亮边 + 单一琥珀强调色)与文字层级全部集中在这里,主菜单/暂停/设置/调试
//! 面板一律先取既有令牌,再考虑新增。
//!
//! 与 Go 侧 `internal/render/hud/style.go` 的 HUD 令牌表并排存在、互不引用:
//! 两侧没有 ABI 通道,数值各自精修。本表同时承担「egui 全局 [`Style`] 批量
//! 接入」的构造职责——按钮、输入框、滑杆等控件状态色经 [`ui_style`] 一次
//! 写入,draw 侧只在 egui 样式表达达不到的局部(手绘文本/标记)用显式颜色
//! 参数消费同一批常量。
//!
//! 值域约定:令牌以 0..1 分量记录,落定为 egui [`Color32`] 的 sRGB u8 时按
//! 「分量 × 255 四舍五入」换算,保持与设计文档的数值可并排对照。
//!
//! 强调色纪律:[`ACCENT_AMBER`] 只允许出现在「选中、进度、焦点/编辑态」
//! 三类语义;错误行用 [`DANGER`];不得引入第二种强调色相。

use egui::{Color32, CornerRadius, Shadow, Stroke, Style};

/// 面板表面:半透明深色,透出世界画面维持空间感;设置页与调试面板的底、
/// 菜单/暂停按钮的填充共用同一表面语言。
pub(crate) const PANEL_FILL: Color32 = Color32::from_rgba_unmultiplied_const(11, 13, 16, 235);

/// 面板 1 逻辑点亮边:把面板从暗背景里「切」出来的描边语言;也作为非交互
/// 控件(分隔线)与静默态输入框的描边,保证整族形状一致。
pub(crate) const PANEL_STROKE: Stroke = Stroke {
    width: 1.0,
    color: Color32::from_rgba_unmultiplied_const(255, 255, 255, 31),
};

/// 主文字:标题、按钮正文、行值。
pub(crate) const TEXT_PRIMARY: Color32 = Color32::from_rgb(240, 242, 245);

/// 次级文字:行标签、版本行、远程注明行、材质路径提示。
pub(crate) const TEXT_SECONDARY: Color32 = Color32::from_rgb(158, 166, 179);

/// 唯一强调色:焦点/悬停描边、选中行左缘标记、编辑态、脏草稿提示。
pub(crate) const ACCENT_AMBER: Color32 = Color32::from_rgb(255, 184, 61);

/// 强调色的 1 逻辑点描边形态:悬停/焦点控件边框与编辑态输入框共用。
pub(crate) const ACCENT_STROKE: Stroke = Stroke {
    width: 1.0,
    color: ACCENT_AMBER,
};

/// 强调色的 25% 透明变体:文本选区高亮、滑杆进度填充与选中按钮的底衬。
pub(crate) const ACCENT_WASH: Color32 = Color32::from_rgba_unmultiplied_const(255, 184, 61, 64);

/// 错误行:与琥珀、正文白拉开色相的告警红。
pub(crate) const DANGER: Color32 = Color32::from_rgb(240, 89, 89);

/// 主菜单整屏不透明底色(标题画面基调,比旧深灰更深)。
pub(crate) const MENU_BACKGROUND: Color32 = Color32::from_rgb(14, 16, 19);

/// 暂停遮罩:面板族同色的 0.82 透明变体;背后世界保持隐约可见是暂停层
/// 语义,故透明度低于 [`PANEL_FILL`] 而色相与之对齐。
pub(crate) const PAUSE_OVERLAY: Color32 = Color32::from_rgba_unmultiplied_const(11, 13, 16, 209);

/// 控件凹槽底:输入框与滑杆轨道的内凹表面,比面板表面更暗;对应 Go 侧
/// `slotWell` 的「凹陷」语义,不另设第二种深色相。
pub(crate) const CONTROL_WELL: Color32 = Color32::from_rgb(5, 6, 8);

/// 控件圆角:小圆角语义在按钮/输入框一族统一为 2 逻辑点。
pub(crate) const BUTTON_CORNER_RADIUS: CornerRadius = CornerRadius::same(2);

/// 把令牌批量写进 egui 全局 [`Style`]:控件状态色、选区/焦点描边、输入
/// 凹槽与无阴影保证一次接入。悬停与焦点态在 egui 里分别落在 `hovered` 与
/// `active`(带焦点/按下的控件),两者都用琥珀描边表达「可按下/已聚焦」。
pub(crate) fn ui_style() -> Style {
    let mut style = Style::default();
    let visuals = &mut style.visuals;

    // 普通文本(标签/标题/滑杆说明)取主文字;非交互描边让分隔线也归入
    // 亮边语言。
    visuals.widgets.noninteractive.fg_stroke = Stroke::new(1.0, TEXT_PRIMARY);
    visuals.widgets.noninteractive.bg_stroke = PANEL_STROKE;

    // 静默/悬停/按下三态的填充与文字:按钮用 weak_bg_fill(半透明面板),
    // 必须有底的控件(滑杆轨道等)用凹槽底。
    for state in [
        &mut visuals.widgets.inactive,
        &mut visuals.widgets.hovered,
        &mut visuals.widgets.active,
    ] {
        state.weak_bg_fill = PANEL_FILL;
        state.bg_fill = CONTROL_WELL;
        state.fg_stroke = Stroke::new(1.0, TEXT_PRIMARY);
        state.corner_radius = BUTTON_CORNER_RADIUS;
    }
    // 静默态描边 = 1 逻辑点亮边;悬停/焦点态描边换琥珀。
    visuals.widgets.inactive.bg_stroke = PANEL_STROKE;
    visuals.widgets.hovered.bg_stroke = ACCENT_STROKE;
    visuals.widgets.active.bg_stroke = ACCENT_STROKE;

    // 选区与焦点:TextEdit 聚焦描边、选中按钮的文字/填充与文本高亮都从
    // `selection` 取色,是「编辑态琥珀」的统一来源。
    visuals.selection.bg_fill = ACCENT_WASH;
    visuals.selection.stroke = ACCENT_STROKE;

    // 输入凹槽与滚动/轨道底色。
    visuals.extreme_bg_color = CONTROL_WELL;
    visuals.text_edit_bg_color = Some(CONTROL_WELL);

    // 无阴影:菜单是确定性即时模式 UI,投影不属于面板语言。
    visuals.window_shadow = Shadow::NONE;
    visuals.popup_shadow = Shadow::NONE;

    style
}

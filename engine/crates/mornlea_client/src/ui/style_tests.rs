//! 令牌钉值与全局 [`egui::Style`] 接线测试:面板语言任何精修只改 `style.rs`,
//! 并在这里逐值复核;`UiState` 必须在构造期把令牌批量接入 egui 全局 Style,
//! 保证四个界面共享同一份面板语言。

use super::style;
use super::*;

#[test]
fn token_values_are_pinned() {
    assert_eq!(
        style::PANEL_FILL,
        egui::Color32::from_rgba_unmultiplied(11, 13, 16, 235)
    );
    assert_eq!(
        style::PAUSE_OVERLAY,
        egui::Color32::from_rgba_unmultiplied(11, 13, 16, 209)
    );
    assert_eq!(
        style::PANEL_STROKE,
        egui::Stroke::new(
            1.0,
            egui::Color32::from_rgba_unmultiplied(255, 255, 255, 31)
        )
    );
    assert_eq!(style::TEXT_PRIMARY, egui::Color32::from_rgb(240, 242, 245));
    assert_eq!(
        style::TEXT_SECONDARY,
        egui::Color32::from_rgb(158, 166, 179)
    );
    assert_eq!(style::ACCENT_AMBER, egui::Color32::from_rgb(255, 184, 61));
    assert_eq!(
        style::ACCENT_STROKE,
        egui::Stroke::new(1.0, style::ACCENT_AMBER)
    );
    assert_eq!(
        style::ACCENT_WASH,
        egui::Color32::from_rgba_unmultiplied(255, 184, 61, 64)
    );
    assert_eq!(style::DANGER, egui::Color32::from_rgb(240, 89, 89));
    assert_eq!(style::MENU_BACKGROUND, egui::Color32::from_rgb(14, 16, 19));
    assert_eq!(style::CONTROL_WELL, egui::Color32::from_rgb(5, 6, 8));
    assert_eq!(style::BUTTON_CORNER_RADIUS, egui::CornerRadius::same(2));
}

#[test]
fn ui_style_batches_tokens_into_global_visuals() {
    let style = style::ui_style();
    let widgets = &style.visuals.widgets;

    // 普通文本与非交互描边。
    assert_eq!(widgets.noninteractive.fg_stroke.color, style::TEXT_PRIMARY);
    assert_eq!(widgets.noninteractive.bg_stroke, style::PANEL_STROKE);

    // 静默/悬停/按下三态:半透明面板填充 + 凹槽底 + 主文字 + 2 逻辑点圆角。
    for state in [&widgets.inactive, &widgets.hovered, &widgets.active] {
        assert_eq!(state.weak_bg_fill, style::PANEL_FILL);
        assert_eq!(state.bg_fill, style::CONTROL_WELL);
        assert_eq!(state.fg_stroke.color, style::TEXT_PRIMARY);
        assert_eq!(state.corner_radius, style::BUTTON_CORNER_RADIUS);
    }
    assert_eq!(widgets.inactive.bg_stroke, style::PANEL_STROKE);
    assert_eq!(
        widgets.hovered.bg_stroke,
        style::ACCENT_STROKE,
        "悬停态应为琥珀描边"
    );
    assert_eq!(
        widgets.active.bg_stroke,
        style::ACCENT_STROKE,
        "焦点/按压态应为琥珀描边"
    );

    // 选区与焦点:编辑态琥珀的统一来源。
    assert_eq!(style.visuals.selection.bg_fill, style::ACCENT_WASH);
    assert_eq!(style.visuals.selection.stroke, style::ACCENT_STROKE);

    // 输入凹槽。
    assert_eq!(style.visuals.text_edit_bg_color, Some(style::CONTROL_WELL));
    assert_eq!(style.visuals.extreme_bg_color, style::CONTROL_WELL);

    // 无阴影。
    assert_eq!(style.visuals.window_shadow, egui::Shadow::NONE);
    assert_eq!(style.visuals.popup_shadow, egui::Shadow::NONE);
}

#[test]
fn ui_state_applies_token_style_on_construction() {
    let state = UiState::new();
    let visuals = &state.ctx.style_of(state.ctx.theme()).visuals;
    assert_eq!(
        visuals.widgets.inactive.weak_bg_fill,
        style::PANEL_FILL,
        "UiState 构造期必须把令牌接入全局 Style"
    );
    assert_eq!(visuals.widgets.hovered.bg_stroke.color, style::ACCENT_AMBER);
    assert_eq!(visuals.selection.stroke.color, style::ACCENT_AMBER);
    assert_eq!(visuals.text_edit_bg_color, Some(style::CONTROL_WELL));
}

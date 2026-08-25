//! UI 主题测试共享夹具：layout v1 编码、主菜单解包与固定屏幕。
//!
//! 仅放被 menu ABI、menu render、raw input 或 settings ABI 中至少两个主题
//! 共同消费的 helper；单主题 helper 留在对应测试文件。

use super::*;

/// 编码允许注入原始 `enabled` 值的 layout v1 主菜单帧。
pub(super) fn encode_frame_raw(
    layout: u32,
    flags: u32,
    buttons: &[(u32, &str, u32)],
    title: &str,
    version: &str,
    error: &str,
) -> Vec<u8> {
    let mut bytes = Vec::new();
    bytes.extend_from_slice(&layout.to_le_bytes());
    bytes.extend_from_slice(&flags.to_le_bytes());
    bytes.extend_from_slice(&(buttons.len() as u32).to_le_bytes());
    for (id, label, enabled) in buttons {
        bytes.extend_from_slice(&id.to_le_bytes());
        bytes.extend_from_slice(&(label.len() as u32).to_le_bytes());
        bytes.extend_from_slice(label.as_bytes());
        bytes.extend_from_slice(&enabled.to_le_bytes());
    }
    bytes.extend_from_slice(&(title.len() as u32).to_le_bytes());
    bytes.extend_from_slice(title.as_bytes());
    bytes.extend_from_slice(&(version.len() as u32).to_le_bytes());
    bytes.extend_from_slice(version.as_bytes());
    bytes.extend_from_slice(&(error.len() as u32).to_le_bytes());
    bytes.extend_from_slice(error.as_bytes());
    bytes
}

/// 编码把按钮 `enabled` 布尔值映射为 0/1 的合法 layout v1 主菜单帧。
pub(super) fn encode_frame(
    layout: u32,
    flags: u32,
    buttons: &[(u32, &str, bool)],
    title: &str,
    version: &str,
    error: &str,
) -> Vec<u8> {
    let raw = buttons
        .iter()
        .map(|(id, label, enabled)| (*id, *label, u32::from(*enabled)))
        .collect::<Vec<_>>();
    encode_frame_raw(layout, flags, &raw, title, version, error)
}

/// 返回多人/设置禁用、进入/退出启用的四按钮主菜单夹具。
pub(super) fn four_button_frame() -> Vec<u8> {
    encode_frame(
        UI_LAYOUT_VERSION,
        UI_FLAG_VISIBLE,
        &[
            (1, "进入游戏", true),
            (2, "多人游戏", false),
            (3, "设置", false),
            (4, "退出游戏", true),
        ],
        "Mornlea",
        "dev",
        "",
    )
}

/// 把通用 [`UiFrame`] 夹具收窄为主菜单帧。
pub(super) fn menu_frame(frame: &UiFrame) -> &UiMenuFrame {
    match frame {
        UiFrame::Menu(menu) => menu,
        UiFrame::Settings(_) => panic!("测试夹具应为主菜单"),
    }
}

/// 返回 UI 无头测试共用的 1280×720 逻辑屏幕。
pub(super) fn screen_rect() -> Rect {
    Rect::from_min_size(pos2(0.0, 0.0), vec2(1280.0, 720.0))
}

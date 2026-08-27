//! 暂停页 ABI 测试：跨语言契约数字钉值、layout v4 字段解码与非法输入拒绝。

use super::*;

/// 编码一帧 layout v4 暂停页段：版本号后各一个 u32 布尔(visible/remote)。
fn encode_pause_frame(layout: u32, visible: u32, remote: u32) -> Vec<u8> {
    let mut out = Vec::new();
    out.extend_from_slice(&layout.to_le_bytes());
    out.extend_from_slice(&visible.to_le_bytes());
    out.extend_from_slice(&remote.to_le_bytes());
    out
}

/// 钉住暂停页契约的十进制数字：布局版本 4、「返回游戏」(≡Escape) 动作 8、
/// 「退回主菜单」动作 9。数字是 Go/Rust 两侧共同遵守的约定，任何一侧都
/// 不得单方面改动。
#[test]
fn pause_constants_pin_cross_language_numbers() {
    assert_eq!(UI_PAUSE_LAYOUT_VERSION, 4);
    assert_eq!(UI_ACTION_PAUSE_BACK, 8);
    assert_eq!(UI_ACTION_PAUSE_QUIT_TO_MENU, 9);
}

#[test]
fn pause_layout_v4_decodes_visible_and_remote_flags() {
    let frame = decode_ui_frame(&encode_pause_frame(UI_PAUSE_LAYOUT_VERSION, 1, 0)).unwrap();
    assert_eq!(
        frame,
        UiFrame::Pause(UiPauseFrame {
            visible: true,
            remote: false,
        })
    );
    let frame = decode_ui_frame(&encode_pause_frame(UI_PAUSE_LAYOUT_VERSION, 0, 1)).unwrap();
    assert_eq!(
        frame,
        UiFrame::Pause(UiPauseFrame {
            visible: false,
            remote: true,
        })
    );
}

#[test]
fn pause_layout_v4_rejects_invalid_matrix_and_tail() {
    let valid = encode_pause_frame(UI_PAUSE_LAYOUT_VERSION, 1, 1);
    assert!(
        matches!(decode_ui_frame(&valid), Ok(UiFrame::Pause(_))),
        "合法帧应落入暂停页分支"
    );

    // 未知布局版本不得被暂停页分支吞下。
    let mut unknown_layout = valid.clone();
    unknown_layout[0..4].copy_from_slice(&99u32.to_le_bytes());
    assert!(decode_ui_frame(&unknown_layout).is_err());

    // visible/remote 与其余页一致只接受 0/1 布尔。
    assert!(decode_ui_frame(&encode_pause_frame(UI_PAUSE_LAYOUT_VERSION, 2, 1)).is_err());
    assert!(decode_ui_frame(&encode_pause_frame(UI_PAUSE_LAYOUT_VERSION, 1, 2)).is_err());

    // 无变长字段,任一字段边界截断与任何尾随字节都严格拒绝。
    assert!(decode_ui_frame(&valid[..4]).is_err(), "缺 visible");
    assert!(decode_ui_frame(&valid[..8]).is_err(), "缺 remote");
    let mut tail = valid;
    tail.push(0);
    assert!(decode_ui_frame(&tail).is_err());
}

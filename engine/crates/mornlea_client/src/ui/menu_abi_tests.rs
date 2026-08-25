//! 主菜单 layout v1 ABI 解码测试：字段边界、UTF-8、截断与精确成功值。

use super::test_support::{encode_frame, encode_frame_raw, four_button_frame, menu_frame};
use super::*;

#[test]
fn decode_rejects_layout_version_mismatch() {
    assert!(decode_ui_frame(&encode_frame(2, UI_FLAG_VISIBLE, &[], "", "", "")).is_err());
}

#[test]
fn decode_rejects_truncated_bytes() {
    let bytes = four_button_frame();
    for cut in [0usize, 3, 7, 11, 12] {
        assert!(decode_ui_frame(&bytes[..cut]).is_err(), "cut={cut}");
    }
}

#[test]
fn decode_rejects_too_many_buttons() {
    let many = (1..=9u32).map(|i| (i, "x", true)).collect::<Vec<_>>();
    assert!(
        decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &many,
            "",
            "",
            ""
        ))
        .is_err()
    );
}

#[test]
fn decode_rejects_field_overflow() {
    let long_label: String = "长".repeat(33); // 每字 3 字节 => 99 字节。
    assert!(
        decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[(1, &long_label, true)],
            "",
            "",
            ""
        ))
        .is_err()
    );

    let long_title = "长".repeat(50); // 150 字节。
    assert!(
        decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[],
            &long_title,
            "",
            ""
        ))
        .is_err()
    );

    let long_version = "长".repeat(25); // 75 字节。
    assert!(
        decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[],
            "",
            &long_version,
            ""
        ))
        .is_err()
    );

    let long_error = "长".repeat(90); // 270 字节。
    assert!(
        decode_ui_frame(&encode_frame(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[],
            "",
            "",
            &long_error
        ))
        .is_err()
    );
}

#[test]
fn decode_rejects_non_utf8() {
    let mut bytes = encode_frame(UI_LAYOUT_VERSION, UI_FLAG_VISIBLE, &[], "", "", "");
    // 在 title 长度字段(偏移 12)后追加 1 字节非法 UTF-8。
    bytes[12..16].copy_from_slice(&1u32.to_le_bytes());
    bytes.push(0xff);
    assert!(decode_ui_frame(&bytes).is_err());
}

#[test]
fn decode_rejects_segment_too_large() {
    let mut bytes = four_button_frame();
    bytes.resize(MAX_UI_SEGMENT_BYTES + 1, 0);
    assert!(decode_ui_frame(&bytes).is_err());
}

#[test]
fn decode_minimal_success_fields_exact() {
    let frame = decode_ui_frame(&encode_frame(
        UI_LAYOUT_VERSION,
        UI_FLAG_VISIBLE,
        &[],
        "",
        "",
        "",
    ))
    .unwrap();
    let frame = menu_frame(&frame);
    assert!(frame.visible);
    assert_eq!(frame.title, "");
    assert_eq!(frame.version, "");
    assert_eq!(frame.error, "");
    assert!(frame.buttons.is_empty());
}

#[test]
fn decode_maximal_success_fields_exact() {
    let labels = (0..8u32).map(|i| (i, "button", true)).collect::<Vec<_>>();
    let title = "天".repeat(40); // 120 字节,<=128。
    let version = "v".repeat(64);
    let error = "错".repeat(40); // 120 字节,<=256。
    let frame = decode_ui_frame(&encode_frame(
        UI_LAYOUT_VERSION,
        UI_FLAG_VISIBLE,
        &labels,
        &title,
        &version,
        &error,
    ))
    .unwrap();
    let frame = menu_frame(&frame);
    assert!(frame.visible);
    assert_eq!(frame.buttons.len(), 8);
    assert_eq!(frame.buttons[7].id, 7);
    assert_eq!(frame.buttons[7].label, "button");
    assert!(frame.buttons.iter().all(|button| button.enabled));
    assert_eq!(frame.title, title);
    assert_eq!(frame.version, version);
    assert_eq!(frame.error, error);
}

#[test]
fn decode_four_button_with_enabled_fields_exact() {
    // 四按钮 + 错误行的夹具:逐字段(含 enabled)精确断言。
    let frame = decode_ui_frame(&encode_frame(
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
        "存档无法打开",
    ))
    .unwrap();
    let frame = menu_frame(&frame);
    assert!(frame.visible);
    assert_eq!(frame.title, "Mornlea");
    assert_eq!(frame.version, "dev");
    assert_eq!(frame.error, "存档无法打开");
    assert_eq!(frame.buttons.len(), 4);
    assert_eq!(
        frame.buttons[0],
        UiButton {
            id: 1,
            label: "进入游戏".into(),
            enabled: true
        }
    );
    assert_eq!(
        frame.buttons[1],
        UiButton {
            id: 2,
            label: "多人游戏".into(),
            enabled: false
        }
    );
    assert_eq!(
        frame.buttons[2],
        UiButton {
            id: 3,
            label: "设置".into(),
            enabled: false
        }
    );
    assert_eq!(
        frame.buttons[3],
        UiButton {
            id: 4,
            label: "退出游戏".into(),
            enabled: true
        }
    );
}

#[test]
fn decode_rejects_enabled_out_of_range() {
    // enabled 只接受 0/1,其余值(如 2)视为非法。
    assert!(
        decode_ui_frame(&encode_frame_raw(
            UI_LAYOUT_VERSION,
            UI_FLAG_VISIBLE,
            &[(1, "A", 2)],
            "",
            "",
            ""
        ))
        .is_err()
    );
}

#[test]
fn decode_rejects_enabled_truncated() {
    // 单按钮 label "A"(1 字节)时,其 enabled u32 落在偏移 21..25;截断到
    // 中间(22)或其后(21)都读不到完整字段 => Err。
    let bytes = encode_frame(
        UI_LAYOUT_VERSION,
        UI_FLAG_VISIBLE,
        &[(1, "A", true)],
        "",
        "",
        "",
    );
    assert!(decode_ui_frame(&bytes[..21]).is_err());
    assert!(decode_ui_frame(&bytes[..22]).is_err());
}

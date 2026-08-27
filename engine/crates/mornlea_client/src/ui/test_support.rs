//! UI 主题测试共享夹具：帧编码、固定屏幕、输出排空与三帧点击。
//!
//! 仅放被 menu ABI、menu render、raw input 或 settings ABI 中至少两个主题
//! 共同消费的 helper；单主题 helper 留在对应测试文件。

use super::*;

/// 返回 UI 无头呈现测试共用的小型测试字体。
pub(super) fn test_font() -> &'static [u8] {
    include_bytes!("testdata/demo.ttf")
}

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
        UiFrame::Settings(_) | UiFrame::Debug(_) => panic!("测试夹具应为主菜单"),
    }
}

/// 返回 UI 无头测试共用的 1280×720 逻辑屏幕。
pub(super) fn screen_rect() -> Rect {
    Rect::from_min_size(pos2(0.0, 0.0), vec2(1280.0, 720.0))
}

/// 返回并排空当前结构化输出事件。
pub(super) fn take_output_events(state: &mut UiState) -> Vec<UiOutputEvent> {
    let events = state.pending_events.events();
    let max_batch_bytes = 8 + UI_OUTPUT_QUEUE_CAPACITY * (8 + 12 + MAX_UI_SETTINGS_PATH_BYTES);
    let mut out = vec![0u8; max_batch_bytes];
    state.drain_events(&mut out).unwrap();
    events
}

/// 在 `center` 完成移动、按下、释放三帧点击。
///
/// egui 需要移动帧先建立悬浮，后续按下/释放才能稳定登记 click；所有 UI
/// 命中测试共用这一条生产输入路径，避免各主题复制时序细节。
pub(super) fn click_ui(state: &mut UiState, frame: &UiFrame, screen: Rect, center: egui::Pos2) {
    for events in [
        vec![UiEvent::CursorMoved(center.x as f64, center.y as f64)],
        vec![
            UiEvent::CursorMoved(center.x as f64, center.y as f64),
            UiEvent::MouseButton(true, true),
        ],
        vec![
            UiEvent::CursorMoved(center.x as f64, center.y as f64),
            UiEvent::MouseButton(true, false),
        ],
    ] {
        state
            .run_frame(raw_input(&events, screen, 1.0, None), frame, 1.0)
            .unwrap();
    }
}

/// 扁平收集 FullOutput 里所有文本段(设置页与调试面板主题共用)。
pub(super) fn shape_text(shape: &egui::Shape, out: &mut String) {
    match shape {
        egui::Shape::Text(text) => {
            out.push_str(&text.galley.job.text);
            out.push('\n');
        }
        egui::Shape::Vec(shapes) => {
            for shape in shapes {
                shape_text(shape, out);
            }
        }
        _ => {}
    }
}

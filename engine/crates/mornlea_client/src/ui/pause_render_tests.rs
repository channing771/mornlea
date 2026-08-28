//! 暂停页无头呈现与交互测试：两按钮、远程注明行两分支与 Escape 键事件免疫。

use super::test_support::{
    click_ui, collect_rects_and_texts, screen_rect, shape_text, take_output_events, test_font,
    text_color,
};
use super::*;

/// 远程注明行的可断言核心文案(整行文案以它开头,供两分支断言复用)。
const REMOTE_NOTE_SNIPPET: &str = "远程世界不会暂停";

fn pause_frame(remote: bool) -> UiFrame {
    UiFrame::Pause(UiPauseFrame {
        visible: true,
        remote,
    })
}

fn prepared_state() -> UiState {
    let mut state = UiState::new();
    assert!(state.install_font(test_font()));
    state
}

/// 运行一帧并扁平收集全部绘制文本。
fn rendered_text(state: &mut UiState, frame: &UiFrame) -> String {
    let output = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), frame, 1.0)
        .expect("事件队列应有容量")
        .expect("可见暂停页应产出布局");
    let mut text = String::new();
    for clipped in &output.shapes {
        shape_text(&clipped.shape, &mut text);
    }
    text
}

#[test]
fn pause_renders_two_buttons_without_remote_note_for_local_world() {
    let mut state = prepared_state();
    let text = rendered_text(&mut state, &pause_frame(false));
    // 标题为 Rust 固定文案,须与两按钮同帧可判定。
    assert!(text.contains("已暂停"), "暂停页应绘制标题：{text}");
    // 两个固定按钮各恰好绘制一次;本地单机不呈现注明行。
    assert_eq!(text.matches("返回游戏").count(), 1);
    assert_eq!(text.matches("退回主菜单").count(), 1);
    assert!(
        !text.contains(REMOTE_NOTE_SNIPPET),
        "单机世界不应出现远程注明行：{text}"
    );
}

#[test]
fn pause_remote_flag_presents_note_line() {
    let mut state = prepared_state();
    let text = rendered_text(&mut state, &pause_frame(true));
    assert!(
        text.contains(REMOTE_NOTE_SNIPPET),
        "远程标志置位应呈现注明行：{text}"
    );
}

#[test]
fn pause_frame_ignores_escape_key_events() {
    let mut state = prepared_state();
    let frame = pause_frame(false);

    // 暂停帧收到 Escape 键事件不得合成任何动作:宿主 winit 泵同一帧既更新
    // 键位快照又把按键入队为 UI 键事件,Go 侧 Esc 栈据快照边沿裁决开合;
    // 若 Rust 再合成返回动作,开层当帧的回声会把覆盖层立即关掉(开层即闭)。
    state
        .run_frame(
            raw_input(
                &[UiEvent::Key {
                    key: Key::Escape,
                    pressed: true,
                    modifiers: egui::Modifiers::default(),
                }],
                screen_rect(),
                1.0,
                None,
            ),
            &frame,
            1.0,
        )
        .unwrap()
        .unwrap();
    assert!(
        take_output_events(&mut state).is_empty(),
        "暂停帧不得把 Escape 键事件合成为动作"
    );

    // 「返回游戏」按钮是 back 动作的唯一来源,点击仍产生该 typed action。
    let rects = menu_button_layout(screen_rect(), 2);
    click_ui(&mut state, &frame, screen_rect(), rects[0].center());
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(UI_ACTION_PAUSE_BACK)]
    );
}

#[test]
fn pause_quit_to_menu_button_emits_quit_action() {
    let mut state = prepared_state();
    let frame = pause_frame(true);
    let rects = menu_button_layout(screen_rect(), 2);
    click_ui(&mut state, &frame, screen_rect(), rects[1].center());
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(UI_ACTION_PAUSE_QUIT_TO_MENU)]
    );
}

#[test]
fn pause_visuals_use_panel_tokens() {
    let mut state = prepared_state();
    let text = rendered_text(&mut state, &pause_frame(true));
    assert!(text.contains(REMOTE_NOTE_SNIPPET), "注明行应绘制：{text}");

    // 重新取一帧做形状级断言:遮罩、标题与注明行都落在令牌上。
    let frame = pause_frame(true);
    let full = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .expect("事件队列应有容量")
        .expect("可见暂停页应产出布局");
    let (rects, texts) = collect_rects_and_texts(&full);

    // 遮罩 = 面板族同色的 0.82 透明变体。
    assert!(
        rects
            .iter()
            .any(|rect| rect.rect == screen_rect() && rect.fill == style::PAUSE_OVERLAY),
        "暂停遮罩应为令牌 PAUSE_OVERLAY"
    );

    // 标题 = 主文字;远程注明行 = 次级文字(不再是琥珀提示色)。
    let title = texts
        .iter()
        .find(|text| text.galley.job.text.contains("已暂停"))
        .expect("暂停标题应绘制");
    assert_eq!(text_color(title), style::TEXT_PRIMARY);
    let note = texts
        .iter()
        .find(|text| text.galley.job.text.contains(REMOTE_NOTE_SNIPPET))
        .expect("远程注明行应绘制");
    assert_eq!(text_color(note), style::TEXT_SECONDARY);

    // 按钮与主菜单同令牌:半透明面板填充。
    assert!(
        rects.iter().any(|rect| rect.fill == style::PANEL_FILL),
        "暂停按钮应为半透明面板填充"
    );
}

#[test]
fn pause_hidden_frame_produces_no_layout() {
    let mut state = prepared_state();
    let frame = UiFrame::Pause(UiPauseFrame {
        visible: false,
        remote: false,
    });
    assert!(
        state
            .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
            .expect("事件队列应有容量")
            .is_none()
    );
}

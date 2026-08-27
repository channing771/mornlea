//! 主菜单无头呈现与交互测试：命中、禁用态、布局、字体和确定性输出。

use super::test_support::{
    click_ui, encode_frame, four_button_frame, menu_frame, screen_rect, take_output_events,
    test_font,
};
use super::*;

/// 返回同主菜单夹具但全部按钮启用的 layout v1 帧。
fn four_button_frame_all_enabled() -> Vec<u8> {
    encode_frame(
        UI_LAYOUT_VERSION,
        UI_FLAG_VISIBLE,
        &[
            (1, "进入游戏", true),
            (2, "多人游戏", true),
            (3, "设置", true),
            (4, "退出游戏", true),
        ],
        "Mornlea",
        "dev",
        "",
    )
}

#[test]
fn menu_hit_click_emits_event_once() {
    let mut state = UiState::new();
    assert!(state.install_font(test_font()));
    let frame = decode_ui_frame(&four_button_frame()).unwrap();

    let rects = menu_button_layout(screen_rect(), 4);
    // 点击启用的「退出游戏」(id=4)中心,应恰好回传该 id 一次。
    let target = rects[3].center();
    click_ui(&mut state, &frame, screen_rect(), target);
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(4)]
    );
    assert!(take_output_events(&mut state).is_empty());
}

#[test]
fn menu_disabled_button_click_no_event() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let rects = menu_button_layout(screen_rect(), 4);

    // 经 wire 的禁用路径:由 enabled=0 的字节解码出「多人游戏」禁用帧,点击其中心不产生事件。
    let frame = decode_ui_frame(&four_button_frame()).unwrap();
    assert!(!menu_frame(&frame).buttons[1].enabled);
    click_ui(&mut state, &frame, screen_rect(), rects[1].center());
    assert!(take_output_events(&mut state).is_empty());

    // 同布局但 enabled=1:点击同样位置返回该按钮 id。
    let frame2 = decode_ui_frame(&four_button_frame_all_enabled()).unwrap();
    assert!(menu_frame(&frame2).buttons[1].enabled);
    click_ui(&mut state, &frame2, screen_rect(), rects[1].center());
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(2)]
    );
}

#[test]
fn menu_settings_button_obeys_enabled_field() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let rects = menu_button_layout(screen_rect(), 4);

    let disabled = decode_ui_frame(&four_button_frame()).unwrap();
    click_ui(&mut state, &disabled, screen_rect(), rects[2].center());
    assert!(take_output_events(&mut state).is_empty());

    let enabled = decode_ui_frame(&encode_frame(
        UI_LAYOUT_VERSION,
        UI_FLAG_VISIBLE,
        &[
            (1, "进入游戏", true),
            (2, "多人游戏", false),
            (3, "设置", true),
            (4, "退出游戏", true),
        ],
        "Mornlea",
        "dev",
        "",
    ))
    .unwrap();
    click_ui(&mut state, &enabled, screen_rect(), rects[2].center());
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(3)]
    );
}

#[test]
fn menu_capacity_preflight_reserves_worst_case_before_running_egui() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = decode_ui_frame(&four_button_frame_all_enabled()).unwrap();
    for id in 0..57 {
        state
            .pending_events
            .enqueue(UiOutputEvent::Action(id))
            .unwrap();
    }
    let before = state.pending_events.events();

    // 主菜单一帧保守预留最多八个按钮事件，因此只余七格时即使本帧没有
    // 实际点击也要在运行 egui 前拒绝；排空后同一空输入必须正常呈现。
    assert!(matches!(
        state.run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0,),
        Err(UiOutputError::Capacity)
    ));
    assert_eq!(state.pending_events.events(), before);
    let _ = take_output_events(&mut state);
    assert!(
        state
            .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0,)
            .unwrap()
            .is_some()
    );
}

#[test]
fn menu_same_point_hits_at_most_one() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = decode_ui_frame(&four_button_frame()).unwrap();
    let rects = menu_button_layout(screen_rect(), 4);

    // 指针落在首按钮中心:恰好只命中按钮 1,且只产生一次事件。
    let center = rects[0].center();
    click_ui(&mut state, &frame, screen_rect(), center);
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(1)]
    );

    // 指针落在距所有按钮都远的点(屏幕上方标题区):不命中任何按钮。
    click_ui(&mut state, &frame, screen_rect(), egui::pos2(640.0, 40.0));
    assert!(take_output_events(&mut state).is_empty());
}

#[test]
fn menu_buttons_do_not_overlap() {
    let rects = menu_button_layout(screen_rect(), 4);
    assert_eq!(rects.len(), 4);
    for pair in rects.windows(2) {
        assert!(pair[1].min.y >= pair[0].max.y, "按钮不得重叠: {pair:?}");
    }
    for rect in &rects {
        assert_eq!(rect.width(), MENU_BUTTON_WIDTH);
        assert_eq!(rect.height(), MENU_BUTTON_HEIGHT);
    }
}

#[test]
fn menu_run_frame_is_deterministic_and_no_texture_churn() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = decode_ui_frame(&four_button_frame()).unwrap();
    let raw = raw_input(&[], screen_rect(), 1.0, None);

    let first = state
        .run_frame(raw.clone(), &frame, 1.0)
        .expect("事件队列应有容量")
        .expect("应产出布局");
    let second = state
        .run_frame(raw, &frame, 1.0)
        .expect("事件队列应有容量")
        .expect("应产出布局");
    assert_eq!(first.shapes.len(), second.shapes.len());
    assert!(
        second.textures_delta.is_empty(),
        "第二次同输入不应再上传纹理"
    );
    assert!(!first.shapes.is_empty());
}

#[test]
fn menu_title_drawn_with_zero_buttons() {
    // 标题绘制不被按钮数 gating:按钮表为空(rects.first() 为 None)时标题仍绘制,
    // 锚在屏幕上半部固定偏移,避免 0 按钮帧丢标题。
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = decode_ui_frame(&encode_frame(
        UI_LAYOUT_VERSION,
        UI_FLAG_VISIBLE,
        &[],
        "Mornlea",
        "dev",
        "",
    ))
    .expect("0 按钮帧应可解码");
    let full = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .expect("事件队列应有容量")
        .expect("应产出布局");
    let has_title = full.shapes.iter().any(|clipped| {
        matches!(&clipped.shape, egui::Shape::Text(t) if t.galley.job.text.contains("Mornlea"))
    });
    assert!(has_title, "按钮数为 0 时标题仍应绘制");
}

#[test]
fn menu_hidden_or_no_font_returns_none() {
    let mut state = UiState::new();
    let frame = decode_ui_frame(&four_button_frame()).unwrap();
    assert!(
        state
            .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
            .expect("事件队列应有容量")
            .is_none()
    );
    state.install_font(test_font());
    let mut hidden = frame.clone();
    match &mut hidden {
        UiFrame::Menu(menu) => menu.visible = false,
        UiFrame::Settings(_) | UiFrame::Debug(_) => panic!("测试夹具应为主菜单"),
    }
    assert!(
        state
            .run_frame(raw_input(&[], screen_rect(), 1.0, None), &hidden, 1.0)
            .expect("事件队列应有容量")
            .is_none()
    );
}

#[test]
fn install_font_rejects_empty_and_is_idempotent() {
    let mut state = UiState::new();
    assert!(!state.install_font(&[]));
    assert!(!state.has_font());
    assert!(state.install_font(test_font()));
    assert!(state.has_font());
    assert!(state.install_font(test_font()));
    assert!(state.has_font());
}

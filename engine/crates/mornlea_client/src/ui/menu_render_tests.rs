//! 主菜单无头呈现与交互测试：命中、禁用态、布局、字体和确定性输出。

use super::test_support::{encode_frame, four_button_frame, menu_frame, screen_rect};
use super::*;

fn test_font() -> &'static [u8] {
    include_bytes!("testdata/demo.ttf")
}

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

fn take_output_events(state: &mut UiState) -> Vec<UiOutputEvent> {
    let events = state.pending_events.events();
    let mut out = vec![0u8; 4096];
    state.drain_events(&mut out).unwrap();
    events
}

/// 在 `center` 处完成一次完整点击:移动帧、按下帧、释放帧,共三帧。
///
/// egui 的指针悬浮要在指针首次移动到的**下一帧**才被认定(debug 证实
/// 同帧 Move+Press 时 hovered=false),故需先一帧建立悬浮,随后按下帧
/// 置 down,最后释放帧登记 click。禁用按钮在所有帧都不产生事件。
fn click_button(state: &mut UiState, frame: &UiFrame, center: egui::Pos2) {
    let move_only = raw_input(
        &[UiEvent::CursorMoved(center.x as f64, center.y as f64)],
        screen_rect(),
        1.0,
        None,
    );
    state.run_frame(move_only, frame, 1.0).unwrap();
    let press = raw_input(
        &[
            UiEvent::CursorMoved(center.x as f64, center.y as f64),
            UiEvent::MouseButton(true, true),
        ],
        screen_rect(),
        1.0,
        None,
    );
    state.run_frame(press, frame, 1.0).unwrap();
    let release = raw_input(
        &[
            UiEvent::CursorMoved(center.x as f64, center.y as f64),
            UiEvent::MouseButton(true, false),
        ],
        screen_rect(),
        1.0,
        None,
    );
    state.run_frame(release, frame, 1.0).unwrap();
}

#[test]
fn menu_hit_click_emits_event_once() {
    let mut state = UiState::new();
    assert!(state.install_font(test_font()));
    let frame = decode_ui_frame(&four_button_frame()).unwrap();

    let rects = menu_button_layout(screen_rect(), 4);
    // 点击启用的「退出游戏」(id=4)中心,应恰好回传该 id 一次。
    let target = rects[3].center();
    click_button(&mut state, &frame, target);
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
    click_button(&mut state, &frame, rects[1].center());
    assert!(take_output_events(&mut state).is_empty());

    // 同布局但 enabled=1:点击同样位置返回该按钮 id。
    let frame2 = decode_ui_frame(&four_button_frame_all_enabled()).unwrap();
    assert!(menu_frame(&frame2).buttons[1].enabled);
    click_button(&mut state, &frame2, rects[1].center());
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(2)]
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
    click_button(&mut state, &frame, center);
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(1)]
    );

    // 指针落在距所有按钮都远的点(屏幕上方标题区):不命中任何按钮。
    click_button(&mut state, &frame, egui::pos2(640.0, 40.0));
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
        UiFrame::Settings(_) => panic!("测试夹具应为主菜单"),
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

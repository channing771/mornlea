//! 设置页无头呈现与交互测试：控件、滚动布局、有界输入和事件顺序。

use super::test_support::{screen_rect, small_screen_rect, test_font};
use super::*;

fn settings_frame(
    audio_volume: f32,
    window: UiSettingsWindow,
    texture_pack_path: &str,
    dirty: bool,
    status: &str,
    error: &str,
) -> UiFrame {
    UiFrame::Settings(UiSettingsFrame {
        visible: true,
        audio_volume,
        window,
        texture_pack_path: texture_pack_path.to_owned(),
        dirty,
        status: status.to_owned(),
        error: error.to_owned(),
    })
}

fn render(state: &mut UiState, frame: &UiFrame, screen: Rect, events: &[UiEvent]) {
    state
        .run_frame(raw_input(events, screen, 1.0, None), frame, 1.0)
        .expect("设置页事件队列应有容量")
        .expect("设置页应产出布局");
}

fn take_output_events(state: &mut UiState) -> Vec<UiOutputEvent> {
    let events = state.pending_events.events();
    let mut out = vec![0u8; 70_000];
    state.drain_events(&mut out).unwrap();
    events
}

fn rect(state: &UiState, element: SettingsElement) -> Rect {
    state
        .settings_layout
        .iter()
        .find_map(|(got, rect, _)| (*got == element).then_some(*rect))
        .unwrap_or_else(|| panic!("设置页缺少 {element:?} 几何"))
}

fn response_id(state: &UiState, element: SettingsElement) -> egui::Id {
    state
        .settings_layout
        .iter()
        .find_map(|(got, _, id)| (*got == element).then_some(*id).flatten())
        .unwrap_or_else(|| panic!("设置页缺少 {element:?} response id"))
}

fn click(state: &mut UiState, frame: &UiFrame, screen: Rect, center: egui::Pos2) {
    render(
        state,
        frame,
        screen,
        &[UiEvent::CursorMoved(center.x as f64, center.y as f64)],
    );
    render(
        state,
        frame,
        screen,
        &[
            UiEvent::CursorMoved(center.x as f64, center.y as f64),
            UiEvent::MouseButton(true, true),
        ],
    );
    render(
        state,
        frame,
        screen,
        &[
            UiEvent::CursorMoved(center.x as f64, center.y as f64),
            UiEvent::MouseButton(true, false),
        ],
    );
}

fn shape_text(shape: &egui::Shape, out: &mut String) {
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

#[test]
fn settings_renders_three_controls_actions_and_feedback() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = settings_frame(
        0.25,
        UiSettingsWindow::Size960x540,
        "packs/local",
        true,
        "设置已保存",
        "保存失败：无权限",
    );
    let full = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .unwrap()
        .unwrap();

    let mut text = String::new();
    for clipped in &full.shapes {
        shape_text(&clipped.shape, &mut text);
    }
    for expected in [
        "设置",
        "总音量",
        "25",
        "%",
        "材质包目录",
        "packs/local",
        "下次启动生效",
        "窗口大小",
        "640 × 360",
        "960 × 540",
        "1280 × 720",
        "有未保存的更改",
        "设置已保存",
        "保存失败：无权限",
        "保存",
        "取消更改",
        "返回",
    ] {
        assert!(
            text.contains(expected),
            "缺少文本 {expected:?}，全部文本：{text}"
        );
    }
    for element in [
        SettingsElement::Audio,
        SettingsElement::TexturePath,
        SettingsElement::Window640x360,
        SettingsElement::Window960x540,
        SettingsElement::Window1280x720,
        SettingsElement::Save,
        SettingsElement::Cancel,
        SettingsElement::Back,
    ] {
        let _ = rect(&state, element);
    }
}

#[test]
fn settings_small_layout_is_bounded_non_overlapping_and_scroll_accessible() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = settings_frame(
        0.5,
        UiSettingsWindow::Size1280x720,
        "packs/local",
        true,
        "材质包将在下次启动时生效",
        "这是一条用于验证小窗口滚动访问的错误",
    );
    let screen = small_screen_rect();
    render(&mut state, &frame, screen, &[]);

    let panel = rect(&state, SettingsElement::Panel);
    let viewport = rect(&state, SettingsElement::ScrollViewport);
    let content = rect(&state, SettingsElement::ScrollContent);
    assert!(screen.contains_rect(panel));
    assert!(panel.contains_rect(viewport));
    assert!(
        content.height() > viewport.height(),
        "小窗口必须出现纵向滚动"
    );

    let controls = [
        SettingsElement::Audio,
        SettingsElement::TexturePath,
        SettingsElement::Window640x360,
        SettingsElement::Window960x540,
        SettingsElement::Window1280x720,
        SettingsElement::Save,
        SettingsElement::Cancel,
        SettingsElement::Back,
    ];
    let initial_visible = controls
        .iter()
        .copied()
        .filter(|element| viewport.contains_rect(rect(&state, *element)))
        .collect::<Vec<_>>();
    assert!(initial_visible.contains(&SettingsElement::Audio));
    assert!(initial_visible.contains(&SettingsElement::TexturePath));

    render(
        &mut state,
        &frame,
        screen,
        &[
            UiEvent::CursorMoved(viewport.center().x as f64, viewport.center().y as f64),
            UiEvent::Scroll(0.0, -1_000.0),
        ],
    );
    // ScrollArea 在收到滚轮输入后请求下一次重排；再跑一帧读取最终位置。
    render(&mut state, &frame, screen, &[]);
    let viewport = rect(&state, SettingsElement::ScrollViewport);
    let after_visible = controls
        .iter()
        .copied()
        .filter(|element| viewport.contains_rect(rect(&state, *element)))
        .collect::<Vec<_>>();
    assert!(after_visible.contains(&SettingsElement::Save));
    assert!(after_visible.contains(&SettingsElement::Cancel));
    assert!(after_visible.contains(&SettingsElement::Back));

    let records = state
        .settings_layout
        .iter()
        .filter(|(element, _, _)| controls.contains(element))
        .collect::<Vec<_>>();
    for (index, (_, left, _)) in records.iter().enumerate() {
        for (_, right, _) in records.iter().skip(index + 1) {
            assert!(
                !left.intersects(*right),
                "关键交互矩形不得重叠：{left:?} / {right:?}"
            );
        }
    }
}

#[test]
fn settings_audio_and_window_emit_complete_snapshots() {
    let mut audio_state = UiState::new();
    audio_state.install_font(test_font());
    let frame = settings_frame(
        0.25,
        UiSettingsWindow::Size960x540,
        "packs/local",
        false,
        "",
        "",
    );
    render(&mut audio_state, &frame, screen_rect(), &[]);
    let audio = rect(&audio_state, SettingsElement::Audio);
    let target = pos2(audio.min.x + audio.width() * 0.55, audio.center().y);
    render(
        &mut audio_state,
        &frame,
        screen_rect(),
        &[UiEvent::CursorMoved(target.x as f64, target.y as f64)],
    );
    render(
        &mut audio_state,
        &frame,
        screen_rect(),
        &[
            UiEvent::CursorMoved(target.x as f64, target.y as f64),
            UiEvent::MouseButton(true, true),
        ],
    );
    let events = take_output_events(&mut audio_state);
    assert_eq!(events.len(), 1);
    let UiOutputEvent::SettingsChanged(audio_changed) = &events[0] else {
        panic!("音量编辑应产生 settings-changed：{events:?}");
    };
    assert!(audio_changed.audio_volume > 0.25);
    assert_eq!(audio_changed.window, UiSettingsWindow::Size960x540);
    assert_eq!(audio_changed.texture_pack_path, "packs/local");

    let mut window_state = UiState::new();
    window_state.install_font(test_font());
    render(&mut window_state, &frame, screen_rect(), &[]);
    let target = rect(&window_state, SettingsElement::Window640x360).center();
    click(&mut window_state, &frame, screen_rect(), target);
    assert_eq!(
        take_output_events(&mut window_state),
        vec![UiOutputEvent::SettingsChanged(UiSettingsValues {
            audio_volume: 0.25,
            window: UiSettingsWindow::Size640x360,
            texture_pack_path: "packs/local".to_owned(),
        })]
    );
}

#[test]
fn settings_path_enforces_utf8_byte_bound_and_filters_crlf() {
    let mut accepted = UiState::new();
    accepted.install_font(test_font());
    let frame = settings_frame(
        0.5,
        UiSettingsWindow::Size960x540,
        &"a".repeat(1021),
        false,
        "",
        "",
    );
    render(&mut accepted, &frame, screen_rect(), &[]);
    let path_id = response_id(&accepted, SettingsElement::TexturePath);
    accepted
        .ctx
        .memory_mut(|memory| memory.request_focus(path_id));
    render(&mut accepted, &frame, screen_rect(), &[UiEvent::Text('界')]);
    let events = take_output_events(&mut accepted);
    let UiOutputEvent::SettingsChanged(settings) = &events[0] else {
        panic!("1024-byte 多字节路径应产生变化：{events:?}");
    };
    assert_eq!(settings.texture_pack_path.len(), 1024);
    assert!(settings.texture_pack_path.is_char_boundary(1024));

    let mut rejected = UiState::new();
    rejected.install_font(test_font());
    let frame = settings_frame(
        0.5,
        UiSettingsWindow::Size960x540,
        &"a".repeat(1022),
        false,
        "",
        "",
    );
    render(&mut rejected, &frame, screen_rect(), &[]);
    let path_id = response_id(&rejected, SettingsElement::TexturePath);
    rejected
        .ctx
        .memory_mut(|memory| memory.request_focus(path_id));
    render(
        &mut rejected,
        &frame,
        screen_rect(),
        &[
            UiEvent::Text('界'),
            UiEvent::Text('\r'),
            UiEvent::Text('\n'),
        ],
    );
    assert!(take_output_events(&mut rejected).is_empty());
}

#[test]
fn settings_same_frame_change_precedes_save_and_capacity_is_atomic() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = settings_frame(0.5, UiSettingsWindow::Size960x540, "pack", false, "", "");
    render(&mut state, &frame, screen_rect(), &[]);
    let path_id = response_id(&state, SettingsElement::TexturePath);
    let save = rect(&state, SettingsElement::Save).center();
    state.ctx.memory_mut(|memory| memory.request_focus(path_id));
    render(
        &mut state,
        &frame,
        screen_rect(),
        &[UiEvent::CursorMoved(save.x as f64, save.y as f64)],
    );
    render(
        &mut state,
        &frame,
        screen_rect(),
        &[
            UiEvent::Text('x'),
            UiEvent::CursorMoved(save.x as f64, save.y as f64),
            UiEvent::MouseButton(true, true),
            UiEvent::MouseButton(true, false),
        ],
    );
    let events = take_output_events(&mut state);
    assert!(matches!(
        events.first(),
        Some(UiOutputEvent::SettingsChanged(_))
    ));
    assert_eq!(
        events.get(1),
        Some(&UiOutputEvent::Action(UI_ACTION_SETTINGS_SAVE))
    );

    let mut full = UiState::new();
    full.install_font(test_font());
    for id in 0..63 {
        full.pending_events
            .enqueue(UiOutputEvent::Action(id))
            .unwrap();
    }
    render(&mut full, &frame, screen_rect(), &[]);
    let path_id = response_id(&full, SettingsElement::TexturePath);
    let save = rect(&full, SettingsElement::Save).center();
    full.ctx.memory_mut(|memory| memory.request_focus(path_id));
    render(
        &mut full,
        &frame,
        screen_rect(),
        &[UiEvent::CursorMoved(save.x as f64, save.y as f64)],
    );
    let before = full.pending_events.events();
    let result = full.run_frame(
        raw_input(
            &[
                UiEvent::Text('x'),
                UiEvent::CursorMoved(save.x as f64, save.y as f64),
                UiEvent::MouseButton(true, true),
                UiEvent::MouseButton(true, false),
            ],
            screen_rect(),
            1.0,
            None,
        ),
        &frame,
        1.0,
    );
    assert!(matches!(result, Err(UiOutputError::Capacity)));
    assert_eq!(full.pending_events.events(), before);
}

#[test]
fn settings_escape_and_back_emit_back_action() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = settings_frame(0.5, UiSettingsWindow::Size1280x720, "", false, "", "");
    render(
        &mut state,
        &frame,
        screen_rect(),
        &[UiEvent::Key {
            key: Key::Escape,
            pressed: true,
            modifiers: egui::Modifiers::default(),
        }],
    );
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(UI_ACTION_SETTINGS_BACK)]
    );

    render(&mut state, &frame, screen_rect(), &[]);
    let target = rect(&state, SettingsElement::Back).center();
    click(&mut state, &frame, screen_rect(), target);
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(UI_ACTION_SETTINGS_BACK)]
    );
}

#[test]
fn settings_cancel_emits_cancel_action_and_dependency_stays_manual() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = settings_frame(0.5, UiSettingsWindow::Size1280x720, "", true, "", "");
    render(&mut state, &frame, screen_rect(), &[]);
    let target = rect(&state, SettingsElement::Cancel).center();
    click(&mut state, &frame, screen_rect(), target);
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::Action(UI_ACTION_SETTINGS_CANCEL)]
    );

    let manifest = include_str!("../../Cargo.toml");
    assert!(manifest.lines().all(|line| {
        let line = line.trim_start();
        !line.starts_with("egui-winit") && !line.starts_with("egui_winit")
    }));
}

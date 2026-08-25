//! 设置页无头呈现与交互测试：控件、滚动布局、有界输入和事件顺序。

use super::test_support::{click_ui, screen_rect, take_output_events, test_font};
use super::*;

fn small_screen_rect() -> Rect {
    Rect::from_min_size(pos2(0.0, 0.0), vec2(640.0, 360.0))
}

fn text_edit_font() -> &'static [u8] {
    // 400-byte demo 字体没有真实字符 advance，egui 会把所有字符光标钳到 0；
    // 中部光标测试复用生产已授权的 Noto CJK 字体，验证真实 UTF-8 位置。
    include_bytes!("../../../../../internal/render/assets/NotoSansCJKsc-Regular.otf")
}

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

fn request_path_focus(state: &mut UiState, frame: &UiFrame, screen: Rect) -> egui::Id {
    let id = response_id(state, SettingsElement::TexturePath);
    state.ctx.memory_mut(|memory| memory.request_focus(id));
    render(state, frame, screen, &[]);
    assert!(state.ctx.memory(|memory| memory.has_focus(id)));
    assert_eq!(id, Id::new(SETTINGS_TEXTURE_PATH_ID_SOURCE));
    id
}

fn focus_path_at(
    state: &mut UiState,
    frame: &UiFrame,
    screen: Rect,
    char_index: usize,
) -> egui::Id {
    let id = request_path_focus(state, frame, screen);
    // 在生产 `run_frame` 的同一 egui pass 内设置真实 TextEditState，规避
    // egui 对 pass 外状态写入的 begin-pass 重置；该字段不进入非测试构建。
    state.settings_cursor_override = Some(char_index);
    render(state, frame, screen, &[]);
    assert_eq!(
        egui::TextEdit::load_state(&state.ctx, id)
            .unwrap()
            .cursor
            .char_range()
            .unwrap()
            .primary
            .index,
        char_index.into()
    );
    id
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
    click_ui(&mut window_state, &frame, screen_rect(), target);
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
fn settings_path_filters_crlf_without_hiding_regular_input() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let frame = settings_frame(0.5, UiSettingsWindow::Size960x540, "pack", false, "", "");
    render(&mut state, &frame, screen_rect(), &[]);
    focus_path_at(&mut state, &frame, screen_rect(), 4);
    render(
        &mut state,
        &frame,
        screen_rect(),
        &[
            UiEvent::Text('x'),
            UiEvent::Text('\r'),
            UiEvent::Text('\n'),
            UiEvent::Text('y'),
        ],
    );
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::SettingsChanged(UiSettingsValues {
            audio_volume: 0.5,
            window: UiSettingsWindow::Size960x540,
            texture_pack_path: "packxy".to_owned(),
        })]
    );
}

#[test]
fn settings_path_middle_multibyte_insert_preserves_both_sides() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let frame = settings_frame(
        0.5,
        UiSettingsWindow::Size960x540,
        "ab世界cd",
        false,
        "",
        "",
    );
    render(&mut state, &frame, screen_rect(), &[]);
    let path_id = focus_path_at(&mut state, &frame, screen_rect(), 3);
    render(&mut state, &frame, screen_rect(), &[UiEvent::Text('好')]);
    assert_eq!(
        response_id(&state, SettingsElement::TexturePath),
        path_id,
        "TextEdit id 必须跨帧稳定"
    );
    let events = take_output_events(&mut state);
    let UiOutputEvent::SettingsChanged(settings) = &events[0] else {
        panic!("中部多字节插入应产生变化：{events:?}");
    };
    assert_eq!(settings.texture_pack_path, "ab世好界cd");
    assert!(
        settings
            .texture_pack_path
            .is_char_boundary(settings.texture_pack_path.len())
    );
}

#[test]
fn settings_path_accepts_exact_utf8_byte_limit() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let original = "a".repeat(1021);
    let frame = settings_frame(0.5, UiSettingsWindow::Size960x540, &original, false, "", "");
    render(&mut state, &frame, screen_rect(), &[]);
    request_path_focus(&mut state, &frame, screen_rect());
    render(&mut state, &frame, screen_rect(), &[UiEvent::Text('界')]);
    let events = take_output_events(&mut state);
    let UiOutputEvent::SettingsChanged(settings) = &events[0] else {
        panic!("1024-byte 多字节路径应产生变化：{events:?}");
    };
    assert_eq!(settings.texture_pack_path.len(), 1024);
    assert!(settings.texture_pack_path.is_char_boundary(1024));
}

#[test]
fn settings_path_over_limit_rolls_back_whole_edit() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let original = "a".repeat(1022);
    let frame = settings_frame(0.5, UiSettingsWindow::Size960x540, &original, false, "", "");
    render(&mut state, &frame, screen_rect(), &[]);
    request_path_focus(&mut state, &frame, screen_rect());
    render(&mut state, &frame, screen_rect(), &[UiEvent::Text('界')]);
    assert!(take_output_events(&mut state).is_empty());
    let text_state = egui::TextEdit::load_state(
        &state.ctx,
        response_id(&state, SettingsElement::TexturePath),
    )
    .unwrap();
    assert!(text_state.cursor.char_range().is_some());
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
}

fn prepared_scrolled_text_state(frame: &UiFrame) -> (UiState, egui::Id, egui::Pos2) {
    let mut state = UiState::new();
    state.install_font(test_font());
    let screen = small_screen_rect();
    render(&mut state, frame, screen, &[]);
    let viewport = rect(&state, SettingsElement::ScrollViewport);
    render(
        &mut state,
        frame,
        screen,
        &[
            UiEvent::CursorMoved(viewport.center().x as f64, viewport.center().y as f64),
            UiEvent::Scroll(0.0, -1_000.0),
        ],
    );
    render(&mut state, frame, screen, &[]);
    let save = rect(&state, SettingsElement::Save).center();
    render(
        &mut state,
        frame,
        screen,
        &[UiEvent::CursorMoved(save.x as f64, save.y as f64)],
    );
    let path_id = request_path_focus(&mut state, frame, screen);
    (state, path_id, save)
}

fn text_and_save_input(save: egui::Pos2) -> RawInput {
    raw_input(
        &[
            UiEvent::Text('x'),
            UiEvent::CursorMoved(save.x as f64, save.y as f64),
            UiEvent::MouseButton(true, true),
            UiEvent::MouseButton(true, false),
        ],
        small_screen_rect(),
        1.0,
        None,
    )
}

#[test]
fn settings_capacity_preflight_preserves_transient_state_and_replay() {
    let frame = settings_frame(
        0.5,
        UiSettingsWindow::Size960x540,
        "pack",
        true,
        "材质将在下次启动生效",
        "用于形成滚动内容的错误提示",
    );

    let (mut clean, _, clean_save) = prepared_scrolled_text_state(&frame);
    clean
        .run_frame(text_and_save_input(clean_save), &frame, 1.0)
        .unwrap();
    let clean_events = take_output_events(&mut clean);

    let (mut blocked, path_id, save) = prepared_scrolled_text_state(&frame);
    for id in 0..61 {
        blocked
            .pending_events
            .enqueue(UiOutputEvent::Action(id))
            .unwrap();
    }
    let pending_before = blocked.pending_events.events();
    let layout_before = blocked.settings_layout.clone();
    let cursor_before = egui::TextEdit::load_state(&blocked.ctx, path_id)
        .unwrap()
        .cursor
        .char_range();
    assert!(blocked.ctx.memory(|memory| memory.has_focus(path_id)));

    assert!(matches!(
        blocked.run_frame(text_and_save_input(save), &frame, 1.0),
        Err(UiOutputError::Capacity)
    ));
    assert_eq!(blocked.pending_events.events(), pending_before);
    assert_eq!(blocked.settings_layout, layout_before);
    assert!(blocked.ctx.memory(|memory| memory.has_focus(path_id)));
    assert_eq!(
        egui::TextEdit::load_state(&blocked.ctx, path_id)
            .unwrap()
            .cursor
            .char_range(),
        cursor_before
    );

    let _ = take_output_events(&mut blocked);
    blocked
        .run_frame(text_and_save_input(save), &frame, 1.0)
        .unwrap();
    assert_eq!(take_output_events(&mut blocked), clean_events);
}

#[test]
fn settings_escape_and_back_emit_back_action() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let frame = settings_frame(0.5, UiSettingsWindow::Size1280x720, "pack", false, "", "");
    render(&mut state, &frame, screen_rect(), &[]);
    let path_id = focus_path_at(&mut state, &frame, screen_rect(), 2);
    let cursor_before = egui::TextEdit::load_state(&state.ctx, path_id)
        .unwrap()
        .cursor
        .char_range();
    assert!(state.ctx.memory(|memory| memory.has_focus(path_id)));
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
    // Escape 产生返回 action，同时按 egui 单行编辑惯例释放文本焦点；Go 若因
    // dirty 阻止离页，草稿和字符光标仍保留，用户可再次点击继续编辑。
    assert!(!state.ctx.memory(|memory| memory.has_focus(path_id)));
    assert_eq!(
        egui::TextEdit::load_state(&state.ctx, path_id)
            .unwrap()
            .cursor
            .char_range(),
        cursor_before
    );

    render(&mut state, &frame, screen_rect(), &[]);
    let target = rect(&state, SettingsElement::Back).center();
    click_ui(&mut state, &frame, screen_rect(), target);
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
    click_ui(&mut state, &frame, screen_rect(), target);
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

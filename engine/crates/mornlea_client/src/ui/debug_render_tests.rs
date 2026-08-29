//! 调试面板无头呈现与交互测试：读数区、行呈现、键盘动作与编辑态事件。

use super::test_support::{
    collect_rects_and_texts, screen_rect, shape_text, take_output_events, test_font, text_color,
};
use super::*;

fn panel_action(action: u32) -> UiOutputEvent {
    UiOutputEvent::DebugPanel(UiDebugPanelEvent {
        action,
        value: String::new(),
    })
}

fn key(key: Key) -> UiEvent {
    UiEvent::Key {
        key,
        pressed: true,
        modifiers: egui::Modifiers::default(),
    }
}

/// 编辑态测试使用生产 Noto CJK 字体:demo 字体无真实字符 advance,egui 会把
/// 字符光标钳到 0,无法验证键入位置与确认值(设置页测试同因)。
fn text_edit_font() -> &'static [u8] {
    include_bytes!("../../../../../internal/render/assets/NotoSansCJKsc-Regular.otf")
}

fn row(
    label: &str,
    value: &str,
    readonly: bool,
    selected: bool,
    editable: bool,
    editing: bool,
) -> UiDebugRow {
    let editing = editing && editable;
    UiDebugRow {
        label: label.to_owned(),
        value: value.to_owned(),
        readonly,
        selected,
        editable,
        editing,
        edit_value: if editing {
            value.to_owned()
        } else {
            String::new()
        },
        edit_cursor: if editing { value.len() } else { 0 },
    }
}

fn debug_frame(rows: Vec<UiDebugRow>) -> UiFrame {
    UiFrame::Debug(UiDebugFrame {
        visible: true,
        frame_millis: 12.5,
        position: [10.0, 64.0, -3.0],
        yaw: 45.0,
        pitch: -12.0,
        tick: 1234,
        world_time: 42,
        loaded_chunks: 137,
        mode: "单机".to_owned(),
        rows,
    })
}

fn sample_frame() -> UiFrame {
    debug_frame(vec![
        row("── physics ──", "", true, false, false, false),
        row("gravity", "9.8", false, true, true, false),
        row("viewDistance", "16", true, false, false, false),
        row("fovDegrees", "70", false, false, true, false),
    ])
}

fn render(state: &mut UiState, frame: &UiFrame, events: &[UiEvent]) {
    state
        .run_frame(raw_input(events, screen_rect(), 1.0, None), frame, 1.0)
        .expect("调试面板事件队列应有容量")
        .expect("调试面板应产出布局");
}

fn collect_text(output: &egui::FullOutput) -> String {
    let mut text = String::new();
    for clipped in &output.shapes {
        shape_text(&clipped.shape, &mut text);
    }
    text
}

#[test]
fn debug_panel_renders_readout_and_rows() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = sample_frame();
    let full = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .unwrap()
        .unwrap();
    let text = collect_text(&full);
    for expected in [
        "帧时",
        "12.50 ms",
        "坐标",
        "10.0, 64.0, -3.0",
        "朝向",
        "yaw 45.0 pitch -12.0",
        "Tick",
        "1234",
        "时刻",
        "42",
        "区块数",
        "137",
        "模式",
        "单机",
        "── physics ──",
        "gravity",
        "9.8",
        "viewDistance",
        "16",
        "fovDegrees",
        "70",
    ] {
        assert!(
            text.contains(expected),
            "缺少文本 {expected:?}，全部文本：{text}"
        );
    }
}

#[test]
fn debug_panel_visuals_use_panel_tokens() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = sample_frame();
    let full = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .unwrap()
        .unwrap();
    let (rects, texts) = collect_rects_and_texts(&full);

    // 面板 = 半透明表面 + 1 逻辑点亮边。
    assert!(
        rects.iter().any(|r| r.fill == style::PANEL_FILL),
        "调试面板应为令牌 PANEL_FILL"
    );
    assert!(
        rects
            .iter()
            .any(|r| r.stroke.color == style::PANEL_STROKE.color),
        "调试面板应带 1 逻辑点亮边描边"
    );

    // 文字层级:标签一律次级文字;读数与普通行的值为主文字,只读行的值
    // 也降为次级以表达「不可编辑」。
    for (needle, expected) in [
        ("帧时", style::TEXT_SECONDARY),
        ("12.50 ms", style::TEXT_PRIMARY),
        ("── physics ──", style::TEXT_SECONDARY),
        ("gravity", style::TEXT_SECONDARY),
        ("9.8", style::TEXT_PRIMARY),
        ("viewDistance", style::TEXT_SECONDARY),
        ("16", style::TEXT_SECONDARY),
    ] {
        let hit = texts
            .iter()
            .find(|text| text.galley.job.text == needle)
            .unwrap_or_else(|| panic!("缺少文本 {needle:?}"));
        assert_eq!(text_color(hit), expected, "文本 {needle:?} 颜色");
    }

    // 选中行 = 琥珀左缘标记(整行高、固定窄条),替代旧的整行高亮背景。
    let mark = rects
        .iter()
        .find(|r| r.fill == style::ACCENT_AMBER)
        .expect("选中行应有琥珀左缘标记");
    assert_eq!(mark.rect.width(), DEBUG_PANEL_SELECTED_MARK_WIDTH);
    assert_eq!(mark.rect.height(), DEBUG_PANEL_ROW_HEIGHT);
}

#[test]
fn debug_panel_editing_frame_is_amber_stroked() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let frame = debug_frame(vec![row("fovDegrees", "70", false, true, true, true)]);
    render(&mut state, &frame, &[]);
    let full = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .unwrap()
        .unwrap();
    let (rects, _) = collect_rects_and_texts(&full);
    assert!(
        rects.iter().any(|r| r.stroke.color == style::ACCENT_AMBER),
        "编辑态输入框应有琥珀描边"
    );
}

#[test]
fn debug_panel_hidden_frame_does_zero_work() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let mut frame = sample_frame();
    let UiFrame::Debug(debug) = &mut frame else {
        unreachable!();
    };
    debug.visible = false;
    let output = state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .expect("事件队列应有容量");
    assert!(output.is_none(), "隐藏面板不得产出 UI 布局");
    assert!(state.debug_edit_buffers.is_empty());
}

#[test]
fn debug_panel_arrow_keys_emit_select_events_in_key_order() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = sample_frame();
    render(
        &mut state,
        &frame,
        &[key(Key::ArrowDown), key(Key::ArrowUp)],
    );
    let events = take_output_events(&mut state);
    assert_eq!(
        events,
        vec![
            panel_action(DEBUG_PANEL_ACTION_SELECT_NEXT),
            panel_action(DEBUG_PANEL_ACTION_SELECT_PREV),
        ]
    );
}

#[test]
fn debug_panel_enter_enters_editing_only_on_selected_editable() {
    let mut state = UiState::new();
    state.install_font(test_font());
    render(&mut state, &sample_frame(), &[key(Key::Enter)]);
    assert_eq!(
        take_output_events(&mut state),
        vec![panel_action(DEBUG_PANEL_ACTION_ENTER_EDIT)]
    );

    // 无选中行时 Enter 不产生动作。
    let mut no_selection = sample_frame();
    let UiFrame::Debug(frame) = &mut no_selection else {
        unreachable!();
    };
    for r in &mut frame.rows {
        r.selected = false;
    }
    let mut state = UiState::new();
    state.install_font(test_font());
    render(&mut state, &no_selection, &[key(Key::Enter)]);
    assert!(take_output_events(&mut state).is_empty());
}

#[test]
fn debug_panel_escape_closes_when_not_editing() {
    let mut state = UiState::new();
    state.install_font(test_font());
    render(&mut state, &sample_frame(), &[key(Key::Escape)]);
    assert_eq!(
        take_output_events(&mut state),
        vec![panel_action(DEBUG_PANEL_ACTION_CLOSE)]
    );
}

#[test]
fn debug_panel_editing_types_value_and_confirms_on_enter() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let frame = debug_frame(vec![row("fovDegrees", "70", false, true, true, true)]);
    render(&mut state, &frame, &[]);

    // TextEdit 需要焦点才接收文本；焦点+光标由首个编辑帧自动播种。
    let id = Id::new(("mornlea-debug-edit", 0));
    assert!(state.ctx.memory(|memory| memory.has_focus(id)));

    render(&mut state, &frame, &[UiEvent::Text('5')]);
    let events = take_output_events(&mut state);
    assert_eq!(
        events,
        vec![UiOutputEvent::DebugPanel(UiDebugPanelEvent {
            action: DEBUG_PANEL_ACTION_EDIT_VALUE,
            value: "705".to_owned(),
        })]
    );

    // egui 单行编辑在 Enter 时失焦并交给动作采样。
    render(&mut state, &frame, &[key(Key::Enter)]);
    let events = take_output_events(&mut state);
    assert!(
        events.iter().any(|event| matches!(
            event,
            UiOutputEvent::DebugPanel(UiDebugPanelEvent {
                action: DEBUG_PANEL_ACTION_CONFIRM,
                value,
            }) if value == "705"
        )),
        "缺少确认事件：{events:?}"
    );
}

#[test]
fn debug_panel_editing_seeds_cursor_from_byte_offset_at_char_boundary() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let frame = debug_frame(vec![UiDebugRow {
        label: "name".to_owned(),
        value: "世界".to_owned(),
        readonly: false,
        selected: true,
        editable: true,
        editing: true,
        edit_value: "世界".to_owned(),
        edit_cursor: 3,
    }]);
    render(&mut state, &frame, &[]);

    // edit_cursor=3 是「世」后「界」前的字节偏移，对应字符索引 1：
    // 键入必须插入到正确位置，而不是被字节偏移误当成字符索引钳到末尾。
    render(&mut state, &frame, &[UiEvent::Text('5')]);
    let events = take_output_events(&mut state);
    assert_eq!(
        events,
        vec![UiOutputEvent::DebugPanel(UiDebugPanelEvent {
            action: DEBUG_PANEL_ACTION_EDIT_VALUE,
            value: "世5界".to_owned(),
        })]
    );
}

#[test]
fn debug_panel_hide_clears_edit_buffers() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let mut frame = debug_frame(vec![row("fovDegrees", "70", false, true, true, true)]);
    render(&mut state, &frame, &[]);
    assert_eq!(state.debug_edit_buffers.len(), 1);

    let UiFrame::Debug(debug) = &mut frame else {
        unreachable!();
    };
    debug.visible = false;
    state
        .run_frame(raw_input(&[], screen_rect(), 1.0, None), &frame, 1.0)
        .expect("隐藏面板事件队列也应有余量");
    assert!(
        state.debug_edit_buffers.is_empty(),
        "面板隐藏时必须清空编辑草稿，避免 reopen 后播种陈旧会话文本"
    );
}

#[test]
fn debug_panel_editing_escape_cancels_and_blocks_navigation() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let frame = debug_frame(vec![row("fovDegrees", "70", false, true, true, true)]);
    render(&mut state, &frame, &[]);

    // 编辑期间方向键只移动文本光标,不产生行选中动作…
    render(
        &mut state,
        &frame,
        &[key(Key::ArrowDown), key(Key::ArrowUp)],
    );
    assert!(take_output_events(&mut state).is_empty());

    // …Esc 产生取消(而非关闭)。
    render(&mut state, &frame, &[key(Key::Escape)]);
    assert_eq!(
        take_output_events(&mut state),
        vec![panel_action(DEBUG_PANEL_ACTION_CANCEL)]
    );
}

#[test]
fn debug_panel_edit_value_caps_at_byte_limit() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let base = "x".repeat(MAX_DEBUG_PANEL_EDIT_VALUE_BYTES);
    let frame = debug_frame(vec![row("fovDegrees", &base, false, true, true, true)]);
    render(&mut state, &frame, &[]);
    render(
        &mut state,
        &frame,
        &[UiEvent::Text('y'), UiEvent::Text('z')],
    );
    let events = take_output_events(&mut state);
    assert_eq!(
        events,
        vec![UiOutputEvent::DebugPanel(UiDebugPanelEvent {
            action: DEBUG_PANEL_ACTION_EDIT_VALUE,
            value: base.clone(),
        })]
    );
}

#[test]
fn debug_panel_capacity_preflight_rejects_before_consuming_input() {
    let mut state = UiState::new();
    state.install_font(test_font());
    let frame = sample_frame();
    render(&mut state, &frame, &[]);
    for id in 0..(UI_OUTPUT_QUEUE_CAPACITY - MAX_DEBUG_OUTPUT_EVENTS_PER_FRAME + 1) {
        state
            .pending_events
            .enqueue(UiOutputEvent::Action(id as u32))
            .unwrap();
    }
    let pending_before = state.pending_events.events();
    assert!(matches!(
        state.run_frame(
            raw_input(&[key(Key::ArrowDown)], screen_rect(), 1.0, None),
            &frame,
            1.0
        ),
        Err(UiOutputError::Capacity)
    ));
    assert_eq!(state.pending_events.events(), pending_before);
}

#[test]
fn debug_panel_edit_state_survives_repeat_frames_and_resets_when_done() {
    let mut state = UiState::new();
    state.install_font(text_edit_font());
    let mut frame = debug_frame(vec![row("fovDegrees", "70", false, true, true, true)]);
    render(&mut state, &frame, &[]);
    assert_eq!(state.debug_edit_buffers[&0], "70");

    // 键入后草稿前进,Go 回显新值不重置已输入的草稿。
    render(&mut state, &frame, &[UiEvent::Text('5')]);
    assert_eq!(state.debug_edit_buffers[&0], "705");
    assert_eq!(
        take_output_events(&mut state),
        vec![UiOutputEvent::DebugPanel(UiDebugPanelEvent {
            action: DEBUG_PANEL_ACTION_EDIT_VALUE,
            value: "705".to_owned(),
        })]
    );
    if let UiFrame::Debug(debug) = &mut frame {
        debug.rows[0].edit_value = "705".to_owned();
    }
    render(&mut state, &frame, &[]);
    assert_eq!(state.debug_edit_buffers[&0], "705");

    // Go 确认后下一帧翻转 editing：草稿随之清空。
    if let UiFrame::Debug(debug) = &mut frame {
        debug.rows[0].editing = false;
    }
    render(&mut state, &frame, &[]);
    assert!(state.debug_edit_buffers.is_empty());
}

//! 调试面板 layout v3 ABI 测试：段头读数区、定宽行记录、编辑态字段与拒绝矩阵。

use super::*;

/// 段头模式名的字节上界。
const MODE_BYTES: usize = 64;
/// 编辑值原文的字节上界。
const EDIT_BYTES: usize = 64;

/// 编码 24 字节定宽字段：前置 UTF-8 字节 + 后续零填充。
fn fixed_field(value: &[u8]) -> Vec<u8> {
    assert!(value.len() <= 24, "测试夹具不得超过定宽 24 字节");
    let mut out = [0u8; 24];
    out[..value.len()].copy_from_slice(value);
    out.to_vec()
}

/// 编码一行定宽记录；`edit` 为 Some((值, 光标字节偏移)) 时追加编辑态字段。
fn row(label: &str, value: &str, flags: u32, edit: Option<(&str, u32)>) -> Vec<u8> {
    let mut out = fixed_field(label.as_bytes());
    out.extend(fixed_field(value.as_bytes()));
    out.extend_from_slice(&flags.to_le_bytes());
    if let Some((value, cursor)) = edit {
        out.extend_from_slice(&(value.len() as u32).to_le_bytes());
        out.extend_from_slice(value.as_bytes());
        out.extend_from_slice(&cursor.to_le_bytes());
    }
    out
}

/// 原始定宽记录:标签/值槽位必须是恰好 [`MAX_DEBUG_PANEL_RUNES_PER_SIDE`]
/// 字节的原样内容,供构造非法 UTF-8/坏填充的测试夹具。
fn row_raw(label24: &[u8], value24: &[u8], flags: u32) -> Vec<u8> {
    assert_eq!(label24.len(), 24);
    assert_eq!(value24.len(), 24);
    let mut out = label24.to_vec();
    out.extend_from_slice(value24);
    out.extend_from_slice(&flags.to_le_bytes());
    out
}

/// 编码完整 layout v3 段。
#[allow(clippy::too_many_arguments)]
fn debug_frame_raw(
    flags: u32,
    frame_millis: f64,
    position: [f32; 3],
    yaw: f32,
    pitch: f32,
    tick: u64,
    world_time: u64,
    loaded_chunks: u32,
    mode: &[u8],
    rows: &[Vec<u8>],
) -> Vec<u8> {
    let mut out = Vec::new();
    out.extend_from_slice(&UI_DEBUG_LAYOUT_VERSION.to_le_bytes());
    out.extend_from_slice(&flags.to_le_bytes());
    out.extend_from_slice(&frame_millis.to_bits().to_le_bytes());
    for value in position {
        out.extend_from_slice(&value.to_bits().to_le_bytes());
    }
    out.extend_from_slice(&yaw.to_bits().to_le_bytes());
    out.extend_from_slice(&pitch.to_bits().to_le_bytes());
    out.extend_from_slice(&tick.to_le_bytes());
    out.extend_from_slice(&world_time.to_le_bytes());
    out.extend_from_slice(&loaded_chunks.to_le_bytes());
    out.extend_from_slice(&(mode.len() as u32).to_le_bytes());
    out.extend_from_slice(mode);
    out.extend_from_slice(&(rows.len() as u32).to_le_bytes());
    for each in rows {
        out.extend_from_slice(each);
    }
    out
}

/// 三段行的合法夹具：只读段头行 + 可编辑选中行 + 编辑态行。
fn sample_rows() -> Vec<Vec<u8>> {
    vec![
        row("── physics ──", "", DEBUG_PANEL_ROW_FLAG_READONLY, None),
        row(
            "gravity",
            "9.8",
            DEBUG_PANEL_ROW_FLAG_SELECTED | DEBUG_PANEL_ROW_FLAG_EDITABLE,
            None,
        ),
        row(
            "fovDegrees",
            "70",
            DEBUG_PANEL_ROW_FLAG_SELECTED
                | DEBUG_PANEL_ROW_FLAG_EDITABLE
                | DEBUG_PANEL_ROW_FLAG_EDITING,
            Some(("70.5", 4)),
        ),
    ]
}

/// 合法完整段（无尾随字节）。
fn valid_frame() -> Vec<u8> {
    debug_frame_raw(
        1,
        12.5,
        [10.0, 64.0, -3.0],
        45.0,
        -12.0,
        1234,
        42,
        137,
        "单机".as_bytes(),
        &sample_rows(),
    )
}

#[test]
fn debug_panel_layout_v3_decodes_exact_fields() {
    let frame = decode_ui_frame(&valid_frame()).unwrap();
    let UiFrame::Debug(frame) = frame else {
        panic!("v3 段应解码为调试面板帧");
    };
    assert!(frame.visible);
    assert_eq!(frame.frame_millis, 12.5);
    assert_eq!(frame.position, [10.0, 64.0, -3.0]);
    assert_eq!(frame.yaw, 45.0);
    assert_eq!(frame.pitch, -12.0);
    assert_eq!(frame.tick, 1234);
    assert_eq!(frame.world_time, 42);
    assert_eq!(frame.loaded_chunks, 137);
    assert_eq!(frame.mode, "单机");
    assert_eq!(frame.rows.len(), 3);
    let section = &frame.rows[0];
    assert_eq!(section.label, "── physics ──");
    assert_eq!(section.value, "");
    assert!(section.readonly);
    assert!(!section.selected);
    assert!(!section.editable);
    assert!(!section.editing);
    let selected = &frame.rows[1];
    assert_eq!(selected.label, "gravity");
    assert_eq!(selected.value, "9.8");
    assert!(selected.selected);
    assert!(selected.editable);
    assert!(!selected.editing);
    let editing = &frame.rows[2];
    assert!(editing.editing);
    assert!(editing.selected);
    assert!(editing.editable);
    assert_eq!(editing.edit_value, "70.5");
    assert_eq!(editing.edit_cursor, 4);
}

#[test]
fn debug_panel_layout_v3_minimal_and_max_row_decodes() {
    let minimal = debug_frame_raw(0, 0.0, [0.0; 3], 0.0, 0.0, 0, 0, 0, "联机".as_bytes(), &[]);
    let UiFrame::Debug(minimal) = decode_ui_frame(&minimal).unwrap() else {
        panic!("最小段应解码为调试面板帧");
    };
    assert!(!minimal.visible);
    assert!(minimal.rows.is_empty());
    assert_eq!(minimal.mode, "联机");

    let labels = (0..MAX_DEBUG_PANEL_ROWS)
        .map(|i| row(&i.to_string(), "x", DEBUG_PANEL_ROW_FLAG_READONLY, None))
        .collect::<Vec<_>>();
    let max = debug_frame_raw(
        1,
        16.6,
        [1.0, 2.0, 3.0],
        1.0,
        2.0,
        1,
        2,
        3,
        "benchmark".as_bytes(),
        &labels,
    );
    let UiFrame::Debug(max) = decode_ui_frame(&max).unwrap() else {
        panic!("64 行段应解码成功");
    };
    assert_eq!(max.rows.len(), MAX_DEBUG_PANEL_ROWS);
    assert_eq!(max.rows.last().unwrap().label, "63");
}

#[test]
fn debug_panel_layout_v3_exact_24_byte_fields_success() {
    let full: String = "x".repeat(24);
    let rows = vec![row(&full, &full, DEBUG_PANEL_ROW_FLAG_READONLY, None)];
    let bytes = debug_frame_raw(1, 0.0, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"mode", &rows);
    let UiFrame::Debug(frame) = decode_ui_frame(&bytes).unwrap() else {
        panic!("满 24 字节标签/值应解码成功");
    };
    assert_eq!(frame.rows[0].label, full);
    assert_eq!(frame.rows[0].value, full);
}

#[test]
fn debug_panel_layout_v3_rejects_unknown_layout_flags_and_mode() {
    let valid = valid_frame();
    let mode_long = vec![b'm'; MODE_BYTES + 1];
    let mut cases: Vec<(&str, Vec<u8>)> = Vec::new();

    let mut unknown_layout = valid.clone();
    unknown_layout[0..4].copy_from_slice(&99u32.to_le_bytes());
    cases.push(("layout", unknown_layout));
    cases.push((
        "header flags",
        debug_frame_raw(2, 12.5, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"mode", &[]),
    ));
    cases.push((
        "empty mode",
        debug_frame_raw(1, 12.5, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"", &[]),
    ));
    cases.push((
        "mode over bound",
        debug_frame_raw(1, 12.5, [0.0; 3], 0.0, 0.0, 0, 0, 0, &mode_long, &[]),
    ));
    cases.push((
        "mode newline",
        debug_frame_raw(1, 12.5, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"a\nb", &[]),
    ));
    cases.push((
        "mode utf8",
        debug_frame_raw(1, 12.5, [0.0; 3], 0.0, 0.0, 0, 0, 0, &[0xff], &[]),
    ));

    for (name, bytes) in cases {
        assert!(decode_ui_frame(&bytes).is_err(), "case={name}");
    }
}

#[test]
fn debug_panel_layout_v3_rejects_invalid_readout_values() {
    let mut cases: Vec<(&str, Vec<u8>)> = Vec::new();
    cases.push((
        "nan millis",
        debug_frame_raw(1, f64::NAN, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"m", &[]),
    ));
    cases.push((
        "negative millis",
        debug_frame_raw(1, -1.0, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"m", &[]),
    ));
    cases.push((
        "inf position",
        debug_frame_raw(1, 0.0, [f32::INFINITY, 0.0, 0.0], 0.0, 0.0, 0, 0, 0, b"m", &[]),
    ));
    cases.push((
        "nan yaw",
        debug_frame_raw(1, 0.0, [0.0; 3], f32::NAN, 0.0, 0, 0, 0, b"m", &[]),
    ));
    cases.push((
        "inf pitch",
        debug_frame_raw(1, 0.0, [0.0; 3], 0.0, f32::INFINITY, 0, 0, 0, b"m", &[]),
    ));

    for (name, bytes) in cases {
        assert!(decode_ui_frame(&bytes).is_err(), "case={name}");
    }
}

#[test]
fn debug_panel_layout_v3_rejects_row_count_overflow() {
    let rows = (0..=MAX_DEBUG_PANEL_ROWS)
        .map(|i| row(&i.to_string(), "x", DEBUG_PANEL_ROW_FLAG_READONLY, None))
        .collect::<Vec<_>>();
    let bytes = debug_frame_raw(1, 0.0, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"mode", &rows);
    assert!(decode_ui_frame(&bytes).is_err());
}

#[test]
fn debug_panel_layout_v3_rejects_invalid_row_flags() {
    let mut cases: Vec<(&str, u32)> = Vec::new();
    cases.push(("unknown bit", DEBUG_PANEL_ROW_FLAG_EDITING << 1));
    cases.push((
        "selected readonly",
        DEBUG_PANEL_ROW_FLAG_SELECTED | DEBUG_PANEL_ROW_FLAG_READONLY,
    ));
    cases.push(("editing no editable", DEBUG_PANEL_ROW_FLAG_EDITING));
    for (name, flags) in cases {
        let bytes = debug_frame_raw(
            1,
            0.0,
            [0.0; 3],
            0.0,
            0.0,
            0,
            0,
            0,
            b"mode",
            &[row("label", "value", flags, None)],
        );
        assert!(decode_ui_frame(&bytes).is_err(), "case={name}");
    }
}

#[test]
fn debug_panel_layout_v3_rejects_fixed_field_padding_and_utf8() {
    // 标签/值字段是 24 字节零填充:首个 NUL 之后必须全为零,且首段必须合法 UTF-8;
    // 超界内容在定宽帧中无法编码(结构上不可能),由 Go 编码器截断保障。
    let mut label_tail = fixed_field("a".as_bytes());
    label_tail[6] = 0x01;
    let bad_utf8 = fixed_field(&[0xff]);
    let mut value_tail = fixed_field(b"value");
    value_tail[6] = 0x01;
    let bad_value = fixed_field(&[0xc3, 0xff]);
    let cases: Vec<(&str, Vec<Vec<u8>>)> = vec![
        (
            "label nonzero tail",
            vec![row_raw(&label_tail, &fixed_field(b"value"), DEBUG_PANEL_ROW_FLAG_READONLY)],
        ),
        (
            "label utf8",
            vec![
                row_raw(&bad_utf8, &fixed_field(b"value"), DEBUG_PANEL_ROW_FLAG_READONLY),
            ],
        ),
        (
            "value nonzero tail",
            vec![row_raw(&fixed_field(b"label"), &value_tail, DEBUG_PANEL_ROW_FLAG_READONLY)],
        ),
        (
            "value utf8",
            vec![row_raw(&fixed_field(b"label"), &bad_value, DEBUG_PANEL_ROW_FLAG_READONLY)],
        ),
    ];
    for (name, rows) in cases {
        let bytes = debug_frame_raw(1, 0.0, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"mode", &rows);
        assert!(decode_ui_frame(&bytes).is_err(), "case={name}");
    }
}

#[test]
fn debug_panel_layout_v3_rejects_invalid_edit_fields() {
    let edit_flags = DEBUG_PANEL_ROW_FLAG_EDITABLE | DEBUG_PANEL_ROW_FLAG_EDITING;
    let over_bound: String = "x".repeat(EDIT_BYTES + 1);
    let cases: Vec<(&str, Option<(&str, u32)>)> = vec![
        ("edit over bound", Some((&over_bound, 0))),
        ("cursor after end", Some(("value", 6))),
        ("cursor mid char", Some(("世界", 1))),
    ];
    for (name, edit) in cases {
        let bytes = debug_frame_raw(
            1,
            0.0,
            [0.0; 3],
            0.0,
            0.0,
            0,
            0,
            0,
            b"mode",
            &[row("label", "value", edit_flags, edit)],
        );
        assert!(decode_ui_frame(&bytes).is_err(), "case={name}");
    }

    // 非法 UTF-8 编辑值:&str 无法携带坏字节,手工在定宽记录后追加编辑字段。
    let mut raw_row = fixed_field(b"label");
    raw_row.extend(fixed_field(b"value"));
    raw_row.extend_from_slice(&edit_flags.to_le_bytes());
    raw_row.extend_from_slice(&1u32.to_le_bytes());
    raw_row.push(0xff);
    raw_row.extend_from_slice(&0u32.to_le_bytes());
    let bytes = debug_frame_raw(1, 0.0, [0.0; 3], 0.0, 0.0, 0, 0, 0, b"mode", &[raw_row]);
    assert!(decode_ui_frame(&bytes).is_err(), "case=edit utf8");
}

#[test]
fn debug_panel_layout_v3_accepts_up_to_three_zero_pad_bytes() {
    let mut bytes = valid_frame();
    bytes.extend_from_slice(&[0, 0]);
    let UiFrame::Debug(frame) = decode_ui_frame(&bytes).unwrap() else {
        panic!("≤3 零填充应解码成功");
    };
    assert_eq!(frame.rows.len(), 3);

    let mut three = valid_frame();
    three.extend_from_slice(&[0, 0, 0]);
    assert!(decode_ui_frame(&three).is_ok());
}

#[test]
fn debug_panel_layout_v3_rejects_trailing_and_truncated_bytes() {
    let mut cases: Vec<(&str, Vec<u8>)> = Vec::new();

    let mut nonzero = valid_frame();
    nonzero.push(0x01);
    cases.push(("tail nonzero", nonzero));

    let mut zero_overflow = valid_frame();
    zero_overflow.extend_from_slice(&[0, 0, 0, 0]);
    cases.push(("tail four zeros", zero_overflow));

    let valid = valid_frame();
    for cut in [0usize, 1, 100, valid.len() - 1] {
        cases.push(("truncated", valid[..cut].to_vec()));
    }

    for (name, bytes) in cases {
        assert!(decode_ui_frame(&bytes).is_err(), "case={name}");
    }
}

#[test]
fn debug_panel_layout_v3_rejects_segment_over_budget() {
    let mut bytes = valid_frame();
    bytes.resize(MAX_UI_SEGMENT_BYTES + 1, 0);
    assert!(decode_ui_frame(&bytes).is_err());
}

#[test]
fn debug_panel_event_batch_encodes_kind_three_golden() {
    let mut queue = UiOutputQueue::new();
    queue
        .enqueue(UiOutputEvent::DebugPanel(UiDebugPanelEvent {
            action: DEBUG_PANEL_ACTION_CONFIRM,
            value: "70.5".to_owned(),
        }))
        .unwrap();

    let mut out = vec![0u8; 2048];
    let written = queue.drain_into(&mut out).unwrap();
    out.truncate(written);

    let mut want = Vec::new();
    want.extend_from_slice(&1u32.to_le_bytes());
    want.extend_from_slice(&1u32.to_le_bytes());
    want.extend_from_slice(&UI_EVENT_KIND_DEBUG_ACTION.to_le_bytes());
    want.extend_from_slice(&12u32.to_le_bytes());
    want.extend_from_slice(&DEBUG_PANEL_ACTION_CONFIRM.to_le_bytes());
    want.extend_from_slice(&4u32.to_le_bytes());
    want.extend_from_slice(b"70.5");
    assert_eq!(out, want);
    assert!(queue.is_empty());
}

#[test]
fn debug_panel_event_queue_rejects_unknown_action_and_over_bound_value() {
    let mut queue = UiOutputQueue::new();
    assert_eq!(
        queue.enqueue(UiOutputEvent::DebugPanel(UiDebugPanelEvent {
            action: DEBUG_PANEL_ACTION_CLOSE + 1,
            value: String::new(),
        })),
        Err(UiOutputError::Invalid)
    );
    assert_eq!(
        queue.enqueue(UiOutputEvent::DebugPanel(UiDebugPanelEvent {
            action: DEBUG_PANEL_ACTION_CONFIRM,
            value: "x".repeat(MAX_DEBUG_PANEL_EDIT_VALUE_BYTES + 1),
        })),
        Err(UiOutputError::Invalid)
    );
    assert!(queue.is_empty());
}

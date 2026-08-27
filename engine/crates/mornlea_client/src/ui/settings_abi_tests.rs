//! 设置 ABI 测试：只关注 layout v2、结构化事件批与 layout v1 兼容门禁。

use super::test_support::four_button_frame;
use super::*;

fn string_field(out: &mut Vec<u8>, value: &[u8]) {
    out.extend_from_slice(&(value.len() as u32).to_le_bytes());
    out.extend_from_slice(value);
}

fn settings_frame_raw(
    flags: u32,
    audio: f32,
    window: u32,
    path: &[u8],
    dirty: u32,
    status: &[u8],
    error: &[u8],
) -> Vec<u8> {
    let mut out = Vec::new();
    out.extend_from_slice(&UI_SETTINGS_LAYOUT_VERSION.to_le_bytes());
    out.extend_from_slice(&flags.to_le_bytes());
    out.extend_from_slice(&audio.to_bits().to_le_bytes());
    out.extend_from_slice(&window.to_le_bytes());
    string_field(&mut out, path);
    out.extend_from_slice(&dirty.to_le_bytes());
    string_field(&mut out, status);
    string_field(&mut out, error);
    out
}

fn settings_values(path: &str) -> UiSettingsValues {
    UiSettingsValues {
        audio_volume: 0.25,
        window: UiSettingsWindow::Size960x540,
        texture_pack_path: path.to_owned(),
    }
}

#[test]
fn settings_layout_v2_decodes_exact_fields() {
    let bytes = settings_frame_raw(
        1,
        0.25,
        2,
        b"packs/local",
        1,
        "材质包将在下次启动时生效".as_bytes(),
        "保存失败".as_bytes(),
    );
    let frame = decode_ui_frame(&bytes).unwrap();
    assert_eq!(
        frame,
        UiFrame::Settings(UiSettingsFrame {
            visible: true,
            audio_volume: 0.25,
            window: UiSettingsWindow::Size960x540,
            texture_pack_path: "packs/local".to_owned(),
            dirty: true,
            status: "材质包将在下次启动时生效".to_owned(),
            error: "保存失败".to_owned(),
        })
    );
}

#[test]
fn settings_layout_v2_rejects_invalid_matrix_and_tail() {
    let valid = settings_frame_raw(1, 0.5, 3, b"pack", 0, b"", b"");
    let mut cases = Vec::new();

    let mut unknown_layout = valid.clone();
    unknown_layout[0..4].copy_from_slice(&99u32.to_le_bytes());
    cases.push(("layout", unknown_layout));
    cases.push(("flags", settings_frame_raw(2, 0.5, 3, b"", 0, b"", b"")));
    cases.push(("nan", settings_frame_raw(1, f32::NAN, 3, b"", 0, b"", b"")));
    cases.push((
        "inf",
        settings_frame_raw(1, f32::INFINITY, 3, b"", 0, b"", b""),
    ));
    cases.push(("low", settings_frame_raw(1, -0.1, 3, b"", 0, b"", b"")));
    cases.push(("high", settings_frame_raw(1, 1.1, 3, b"", 0, b"", b"")));
    cases.push(("window", settings_frame_raw(1, 0.5, 99, b"", 0, b"", b"")));
    cases.push(("dirty", settings_frame_raw(1, 0.5, 3, b"", 2, b"", b"")));
    cases.push(("utf8", settings_frame_raw(1, 0.5, 3, &[0xff], 0, b"", b"")));
    cases.push((
        "newline",
        settings_frame_raw(1, 0.5, 3, b"a\nb", 0, b"", b""),
    ));
    cases.push((
        "path bound",
        settings_frame_raw(1, 0.5, 3, &vec![b'a'; 1025], 0, b"", b""),
    ));
    cases.push((
        "status bound",
        settings_frame_raw(1, 0.5, 3, b"", 0, &vec![b'a'; 257], b""),
    ));
    cases.push((
        "error bound",
        settings_frame_raw(1, 0.5, 3, b"", 0, b"", &vec![b'a'; 257]),
    ));
    let mut tail = valid.clone();
    tail.push(0);
    cases.push(("tail", tail));
    cases.push(("truncated", valid[..valid.len() - 1].to_vec()));

    for (name, bytes) in cases {
        assert!(decode_ui_frame(&bytes).is_err(), "case={name}");
    }
}

#[test]
fn menu_layout_v1_rejects_unknown_flags_and_tail_without_wire_drift() {
    let valid = four_button_frame();
    assert!(matches!(decode_ui_frame(&valid), Ok(UiFrame::Menu(_))));

    let mut flags = valid.clone();
    flags[4..8].copy_from_slice(&2u32.to_le_bytes());
    assert!(decode_ui_frame(&flags).is_err());
    let mut tail = valid;
    tail.push(0);
    assert!(decode_ui_frame(&tail).is_err());
}

#[test]
fn structured_batch_cross_language_golden_preserves_change_before_action() {
    let changed = UiOutputEvent::SettingsChanged(settings_values("packs/local"));
    let save = UiOutputEvent::Action(UI_ACTION_SETTINGS_SAVE);
    let mut queue = UiOutputQueue::new();
    queue
        .enqueue_frame(&[changed.clone(), save.clone()])
        .unwrap();

    let mut out = vec![0u8; 2048];
    let written = queue.drain_into(&mut out).unwrap();
    out.truncate(written);

    let mut want = Vec::new();
    want.extend_from_slice(&1u32.to_le_bytes());
    want.extend_from_slice(&2u32.to_le_bytes());
    want.extend_from_slice(&2u32.to_le_bytes());
    want.extend_from_slice(&23u32.to_le_bytes());
    want.extend_from_slice(&0.25f32.to_bits().to_le_bytes());
    want.extend_from_slice(&2u32.to_le_bytes());
    string_field(&mut want, b"packs/local");
    want.extend_from_slice(&1u32.to_le_bytes());
    want.extend_from_slice(&4u32.to_le_bytes());
    want.extend_from_slice(&UI_ACTION_SETTINGS_SAVE.to_le_bytes());
    assert_eq!(out, want);
    assert!(queue.is_empty());
}

#[test]
fn output_queue_enforces_64_and_atomic_65_boundary() {
    let mut queue = UiOutputQueue::new();
    for id in 0..64 {
        queue.enqueue(UiOutputEvent::Action(id)).unwrap();
    }
    assert_eq!(queue.len(), 64);
    assert_eq!(
        queue.enqueue(UiOutputEvent::Action(64)),
        Err(UiOutputError::Capacity)
    );
    assert_eq!(queue.len(), 64);
    assert_eq!(queue.events()[0], UiOutputEvent::Action(0));
    assert_eq!(queue.events()[63], UiOutputEvent::Action(63));

    let mut atomic = UiOutputQueue::new();
    for id in 0..63 {
        atomic.enqueue(UiOutputEvent::Action(id)).unwrap();
    }
    assert_eq!(
        atomic.enqueue_frame(&[
            UiOutputEvent::SettingsChanged(settings_values("changed")),
            UiOutputEvent::Action(7),
        ]),
        Err(UiOutputError::Capacity)
    );
    assert_eq!(atomic.len(), 63);
}

#[test]
fn output_queue_capacity_failure_does_not_write_or_consume() {
    let mut queue = UiOutputQueue::new();
    queue
        .enqueue(UiOutputEvent::SettingsChanged(settings_values(
            "packs/local",
        )))
        .unwrap();
    let before = queue.events();
    let mut small = [0xaau8; 8];
    assert_eq!(queue.drain_into(&mut small), Err(UiOutputError::Capacity));
    assert_eq!(small, [0xaa; 8]);
    assert_eq!(queue.events(), before);
}

#[test]
fn output_queue_empty_is_legal_batch() {
    let mut queue = UiOutputQueue::new();
    let mut out = [0xaau8; 8];
    assert_eq!(queue.drain_into(&mut out), Ok(8));
    assert_eq!(&out[0..4], &1u32.to_le_bytes());
    assert_eq!(&out[4..8], &0u32.to_le_bytes());
}

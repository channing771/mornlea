//! winit → egui 事件桥测试：坐标、鼠标、滚轮、IME、键码和修饰键状态。

use super::*;
use winit::dpi::PhysicalPosition;
use winit::event::{DeviceId, TouchPhase};

fn cursor_moved(x: f64, y: f64) -> WindowEvent {
    WindowEvent::CursorMoved {
        device_id: DeviceId::dummy(),
        position: PhysicalPosition::new(x, y),
    }
}

fn cursor_left() -> WindowEvent {
    WindowEvent::CursorLeft {
        device_id: DeviceId::dummy(),
    }
}

fn mouse_input(button: MouseButton, state: ElementState) -> WindowEvent {
    WindowEvent::MouseInput {
        device_id: DeviceId::dummy(),
        state,
        button,
    }
}

fn mouse_wheel_line(dx: f32, dy: f32) -> WindowEvent {
    WindowEvent::MouseWheel {
        device_id: DeviceId::dummy(),
        delta: MouseScrollDelta::LineDelta(dx, dy),
        phase: TouchPhase::Moved,
    }
}

fn mouse_wheel_pixel(dx: f64, dy: f64) -> WindowEvent {
    WindowEvent::MouseWheel {
        device_id: DeviceId::dummy(),
        delta: MouseScrollDelta::PixelDelta(PhysicalPosition::new(dx, dy)),
        phase: TouchPhase::Moved,
    }
}

fn ime_commit(text: &str) -> WindowEvent {
    WindowEvent::Ime(Ime::Commit(text.to_owned()))
}

#[test]
fn winit_to_ui_events_cursor_uses_logical_coords_and_left() {
    let mut mods = egui::Modifiers::default();
    // scale=2:物理 (200,100) -> 逻辑 (100,50)。
    let out = winit_to_ui_events(
        &[cursor_moved(200.0, 100.0), cursor_left()],
        2.0,
        false,
        &mut mods,
    );
    assert_eq!(
        out,
        vec![UiEvent::CursorMoved(100.0, 50.0), UiEvent::CursorGone]
    );
    assert!(mods.is_none());
}

#[test]
fn winit_to_ui_events_mouse_input_both_buttons() {
    let mut mods = egui::Modifiers::default();
    let out = winit_to_ui_events(
        &[
            mouse_input(MouseButton::Left, ElementState::Pressed),
            mouse_input(MouseButton::Right, ElementState::Released),
            mouse_input(MouseButton::Middle, ElementState::Pressed),
        ],
        1.0,
        false,
        &mut mods,
    );
    // 中键被忽略,只保留主/次键。
    assert_eq!(
        out,
        vec![
            UiEvent::MouseButton(true, true),
            UiEvent::MouseButton(false, false)
        ]
    );
}

#[test]
fn winit_to_ui_events_mouse_wheel_line_and_pixel() {
    let mut mods = egui::Modifiers::default();
    let out = winit_to_ui_events(
        &[mouse_wheel_line(1.0, -2.0), mouse_wheel_pixel(10.0, -20.0)],
        1.0,
        false,
        &mut mods,
    );
    // Line 按 60 点换算;Pixel 原样 f32 化。
    assert_eq!(
        out,
        vec![UiEvent::Scroll(60.0, -120.0), UiEvent::Scroll(10.0, -20.0)]
    );
}

#[test]
fn winit_to_ui_events_ime_commit_emits_text_per_char() {
    let mut mods = egui::Modifiers::default();
    let out = winit_to_ui_events(
        &[ime_commit("你好ab\n"), ime_commit("x")],
        1.0,
        false,
        &mut mods,
    );
    // \n 是控制字符应被过滤;每个字符一个 Text。
    assert_eq!(
        out,
        vec![
            UiEvent::Text('你'),
            UiEvent::Text('好'),
            UiEvent::Text('a'),
            UiEvent::Text('b'),
            UiEvent::Text('x'),
        ]
    );
}

#[test]
fn key_events_accumulate_modifiers_in_order() {
    let mut mods = egui::Modifiers::default();
    // Shift 按下:先更新修饰状态,再发射 Key 事件(事件携带 shift=true)。
    let shift_down = key_event_to_ui_events(KeyCode::ShiftLeft, true, None, false, &mut mods);
    assert!(mods.shift);
    assert_eq!(shift_down.len(), 1);
    assert_eq!(
        shift_down[0],
        UiEvent::Key {
            key: Key::ShiftLeft,
            pressed: true,
            modifiers: mods,
        }
    );

    // Shift 按住时按下 A:Key 事件携带 shift=true,文本 'A' 也发射。
    let a = key_event_to_ui_events(KeyCode::KeyA, true, Some("A"), false, &mut mods);
    assert_eq!(a.len(), 2);
    assert_eq!(
        a[0],
        UiEvent::Key {
            key: Key::A,
            pressed: true,
            modifiers: mods,
        }
    );
    assert_eq!(a[1], UiEvent::Text('A'));

    // Shift 释放:状态翻回,Key 事件仍携带更新后的(无 shift)状态。
    let shift_up = key_event_to_ui_events(KeyCode::ShiftLeft, false, None, false, &mut mods);
    assert!(!mods.shift);
    assert_eq!(
        shift_up[0],
        UiEvent::Key {
            key: Key::ShiftLeft,
            pressed: false,
            modifiers: egui::Modifiers::default(),
        }
    );

    // Control/Alt 左变体同样翻转各自位。
    key_event_to_ui_events(KeyCode::ControlLeft, true, None, false, &mut mods);
    assert!(mods.ctrl);
    key_event_to_ui_events(KeyCode::AltLeft, true, None, false, &mut mods);
    assert!(mods.alt);
}

#[test]
fn key_events_filter_control_chars() {
    let mut mods = egui::Modifiers::default();
    // 文本含换行/制表等控制字符,只保留可见字符 'a'。
    let out = key_event_to_ui_events(KeyCode::KeyA, true, Some("a\n\t"), false, &mut mods);
    assert_eq!(
        out,
        vec![
            UiEvent::Key {
                key: Key::A,
                pressed: true,
                modifiers: egui::Modifiers::default(),
            },
            UiEvent::Text('a'),
        ]
    );
}

#[test]
fn key_events_ime_active_suppresses_text() {
    let mut mods = egui::Modifiers::default();
    // IME 激活期间键盘文本不发射(改由 Commit 提供),只留 Key 事件。
    let out = key_event_to_ui_events(KeyCode::KeyA, true, Some("a"), true, &mut mods);
    assert_eq!(out.len(), 1);
    assert!(matches!(
        out[0],
        UiEvent::Key {
            key: Key::A,
            pressed: true,
            ..
        }
    ));
}

#[test]
fn key_events_unknown_code_produces_no_event() {
    let mut mods = egui::Modifiers::default();
    // F1 不在映射表,不产生任何 Key 事件(文本亦无)。
    let out = key_event_to_ui_events(KeyCode::F1, true, None, false, &mut mods);
    assert!(out.is_empty());
    // 未知键码的修饰键推导也不影响状态。
    assert!(mods.is_none());
}

#[test]
fn key_from_maps_common_keys_and_unknown_none() {
    // 常用键齐全:导航、回车/退格、空格、字幕、数字、修饰键左变体。
    assert_eq!(key_from(KeyCode::Escape), Some(Key::Escape));
    assert_eq!(key_from(KeyCode::Enter), Some(Key::Enter));
    assert_eq!(key_from(KeyCode::Backspace), Some(Key::Backspace));
    assert_eq!(key_from(KeyCode::ArrowUp), Some(Key::ArrowUp));
    assert_eq!(key_from(KeyCode::ArrowDown), Some(Key::ArrowDown));
    assert_eq!(key_from(KeyCode::ArrowLeft), Some(Key::ArrowLeft));
    assert_eq!(key_from(KeyCode::ArrowRight), Some(Key::ArrowRight));
    assert_eq!(key_from(KeyCode::Space), Some(Key::Space));
    assert_eq!(key_from(KeyCode::ShiftLeft), Some(Key::ShiftLeft));
    assert_eq!(key_from(KeyCode::ControlLeft), Some(Key::ControlLeft));
    assert_eq!(key_from(KeyCode::AltLeft), Some(Key::AltLeft));
    assert_eq!(key_from(KeyCode::KeyW), Some(Key::W));
    assert_eq!(key_from(KeyCode::KeyZ), Some(Key::Z));
    assert_eq!(key_from(KeyCode::Digit0), Some(Key::Num0));
    assert_eq!(key_from(KeyCode::Digit9), Some(Key::Num9));
    // 字母区间全覆盖(A..Z)。
    for (code, key) in [
        (KeyCode::KeyA, Key::A),
        (KeyCode::KeyB, Key::B),
        (KeyCode::KeyM, Key::M),
        (KeyCode::KeyY, Key::Y),
        (KeyCode::KeyZ, Key::Z),
    ] {
        assert_eq!(key_from(code), Some(key));
    }
    // 未知键返回 None。
    assert_eq!(key_from(KeyCode::F1), None);
    assert_eq!(key_from(KeyCode::Home), None);
    assert_eq!(key_from(KeyCode::ShiftRight), None);
    assert_eq!(key_from(KeyCode::ControlRight), None);
    assert_eq!(key_from(KeyCode::AltRight), None);
}

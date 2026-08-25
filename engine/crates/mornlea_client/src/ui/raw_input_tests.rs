//! egui `RawInput` 组装测试：事件字段、修饰键、缩放与默认指针位置。

use super::test_support::screen_rect;
use super::*;

#[test]
fn raw_input_translates_events_and_fields() {
    let mods = egui::Modifiers {
        shift: true,
        ..Default::default()
    };
    let raw = raw_input(
        &[
            UiEvent::CursorMoved(100.0, 200.0),
            UiEvent::CursorGone,
            UiEvent::MouseButton(true, true),
            UiEvent::Key {
                key: egui::Key::A,
                pressed: true,
                modifiers: mods,
            },
            UiEvent::Text('好'),
            UiEvent::Scroll(1.0, -2.0),
        ],
        screen_rect(),
        2.0,
        None,
    );
    assert_eq!(raw.screen_rect, Some(screen_rect()));
    assert!(raw.focused);
    assert_eq!(raw.time, None);
    assert_eq!(raw.viewport().native_pixels_per_point, Some(2.0));
    assert!(raw.modifiers.shift);

    assert_eq!(raw.events.len(), 6);
    assert!(matches!(raw.events[0], egui::Event::PointerMoved(p) if p == pos2(100.0, 200.0)));
    assert!(matches!(raw.events[1], egui::Event::PointerGone));
    assert!(
        matches!(raw.events[2], egui::Event::PointerButton { button: egui::PointerButton::Primary, pressed: true, pos, .. } if pos == pos2(100.0, 200.0))
    );
    assert!(matches!(
        raw.events[3],
        egui::Event::Key {
            key: egui::Key::A,
            pressed: true,
            repeat: false,
            ..
        }
    ));
    assert!(matches!(&raw.events[4], egui::Event::Text(s) if s == "好"));
    assert!(
        matches!(raw.events[5], egui::Event::MouseWheel { delta, phase: egui::TouchPhase::Move, .. } if delta == vec2(1.0, -2.0))
    );
}

#[test]
fn raw_input_defaults_pointer_pos_to_screen_center() {
    let raw = raw_input(
        &[UiEvent::MouseButton(true, true)],
        screen_rect(),
        1.0,
        None,
    );
    assert!(
        matches!(raw.events[0], egui::Event::PointerButton { pos, .. } if pos == screen_rect().center())
    );
}

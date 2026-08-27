//! 窗口输入桥队列测试：顺序排空、1024 条边界和显式清空。

use super::*;

#[test]
fn ui_queue_push_take_drains_in_order() {
    clear_ui_events();
    push_ui_event(UiEvent::CursorMoved(1.0, 2.0));
    push_ui_event(UiEvent::CursorGone);
    push_ui_event(UiEvent::Text('a'));
    let taken = take_ui_events();
    assert_eq!(
        taken,
        vec![
            UiEvent::CursorMoved(1.0, 2.0),
            UiEvent::CursorGone,
            UiEvent::Text('a')
        ]
    );
    // take 是排空语义:再次 take 为空。
    assert!(take_ui_events().is_empty());
}

#[test]
fn ui_queue_capacity_keeps_latest_drops_oldest() {
    clear_ui_events();
    for i in 0..1000 {
        push_ui_event(UiEvent::Text(char::from_u32('a' as u32 + i).unwrap_or('a')));
    }
    assert_eq!(take_ui_events().len(), 1000);

    clear_ui_events();
    // 塞满 1025 条:保留最近 1024 条,最旧的 1 条被丢弃。
    for i in 0..(UI_EVENT_QUEUE_CAPACITY as u32 + 1) {
        push_ui_event(UiEvent::CursorMoved(i as f64, 0.0));
    }
    let taken = take_ui_events();
    assert_eq!(taken.len(), UI_EVENT_QUEUE_CAPACITY);
    // 首元素是最新保留的最旧一条:i=1;末元素为最新 i=1024。
    assert_eq!(taken[0], UiEvent::CursorMoved(1.0, 0.0));
    assert_eq!(
        taken[UI_EVENT_QUEUE_CAPACITY - 1],
        UiEvent::CursorMoved(1024.0, 0.0)
    );
}

#[test]
fn ui_queue_clear_empties() {
    clear_ui_events();
    push_ui_event(UiEvent::Text('x'));
    clear_ui_events();
    assert!(take_ui_events().is_empty());
}

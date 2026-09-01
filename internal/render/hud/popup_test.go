package hud

import "testing"

// popup_test.go 只锁窗口计时：物品名弹条的字形呈现已迁 WebView HUD 组件，
// 留在 Go 侧的是 40 权威 tick 窗口与「未确认变化不触发」的裁决输入。

// TestPopupExpiresAfter40Ticks 锁定 40 权威 tick 窗口的硬切边界：窗口内最后一
// tick 仍可见，恰好在第 40 tick 时不再可见。
func TestPopupExpiresAfter40Ticks(t *testing.T) {
	overlay := PopupOverlay{Text: "石头", ShownAtTick: 100, WorldTick: 139, Valid: true}
	if !overlay.Visible() {
		t.Fatal("窗口内最后一 tick 应可见")
	}
	overlay.WorldTick = 140
	if overlay.Visible() {
		t.Fatal("40 tick 后应不可见")
	}
}

// TestPopupVisibleRejectsUnconfirmedAndReversedTicks 见证退化路径：未确认变化
// 与「WorldTick 先于 ShownAtTick」的调用缺陷都按不可见处理。
func TestPopupVisibleRejectsUnconfirmedAndReversedTicks(t *testing.T) {
	if overlay := (PopupOverlay{Text: "石头", Valid: false}); overlay.Visible() {
		t.Fatal("未确认变化不得可见")
	}
	if overlay := (PopupOverlay{Text: "石头", ShownAtTick: 41, WorldTick: 1, Valid: true}); overlay.Visible() {
		t.Fatal("tick 倒退按不可见处理")
	}
}

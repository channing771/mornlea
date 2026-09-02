//go:build darwin

package app

import (
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/client"
)

func TestCombatFeedback(t *testing.T) {
	var feedback combatFeedback
	if feedback.MarkerVisible() {
		t.Fatalf("初始 marker 可见，预期不可见")
	}
	if !feedback.Observe(1) {
		t.Fatalf("tick 1 未被接受")
	}
	if !feedback.MarkerVisible() {
		t.Fatalf("tick 1 后 marker 不可见")
	}
	if feedback.lastServerTick != 1 || feedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("接受后状态=%+v，想要 tick=1 frames=%d", feedback, combatMarkerFrameCount)
	}
	if feedback.Observe(1) {
		t.Fatalf("重复 tick 1 被接受")
	}
	if feedback.Observe(0) {
		t.Fatalf("陈旧 tick 0 被接受")
	}
	if feedback.lastServerTick != 1 || feedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("忽略重复/陈旧后状态改变=%+v", feedback)
	}
	if !feedback.Observe(2) {
		t.Fatalf("tick 2 未被接受")
	}
	if feedback.lastServerTick != 2 || feedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("tick 2 后状态=%+v", feedback)
	}
	if feedback.remainingFrames != 6 {
		t.Fatalf("剩余帧=%d，想要 6", feedback.remainingFrames)
	}
}

func TestCombatFeedbackAfterRenderOnlyDecrementsOnRendered(t *testing.T) {
	var feedback combatFeedback
	if !feedback.Observe(1) {
		t.Fatal("Observe 1 失败")
	}
	feedback.AfterRender(false)
	if feedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("AfterRender(false) 扣帧：剩余=%d", feedback.remainingFrames)
	}
	if !feedback.MarkerVisible() {
		t.Fatalf("AfterRender(false) 后不可见")
	}
	for i := 0; i < 5; i++ {
		feedback.AfterRender(true)
		if !feedback.MarkerVisible() {
			t.Fatalf("第 %d 次 true 后提前不可见", i+1)
		}
	}
	feedback.AfterRender(true)
	if feedback.MarkerVisible() {
		t.Fatalf("六次 true 后仍可见，剩余=%d", feedback.remainingFrames)
	}
	if feedback.remainingFrames != 0 {
		t.Fatalf("六次后剩余=%d，想要 0", feedback.remainingFrames)
	}
	feedback.AfterRender(true)
	if feedback.remainingFrames != 0 {
		t.Fatalf("零帧后继续递减")
	}
}

func TestCombatFeedbackReset(t *testing.T) {
	var feedback combatFeedback
	if !feedback.Observe(5) {
		t.Fatal("Observe 5 失败")
	}
	feedback.AfterRender(true)
	feedback.Reset()
	if feedback != (combatFeedback{}) {
		t.Fatalf("Reset 后=%+v，想要零值", feedback)
	}
	if feedback.MarkerVisible() {
		t.Fatalf("Reset 后仍可见")
	}
}

func TestCombatFeedbackArmMarker(t *testing.T) {
	var feedback combatFeedback
	feedback.ArmMarker()
	if !feedback.MarkerVisible() || feedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("ArmMarker 后=%+v", feedback)
	}
	feedback.AfterRender(true)
	feedback.AfterRender(true)
	feedback.ArmMarker()
	if feedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("ArmMarker 未重置为 6，剩余=%d", feedback.remainingFrames)
	}
}

func TestCombatFeedbackInputHealthInventoryDoNotTrigger(t *testing.T) {
	var feedback combatFeedback
	if feedback.MarkerVisible() {
		t.Fatalf("初始不应可见")
	}
	// 模拟健康/物品/输入变化不应自行武装 marker。
	// 这里仅验证状态机未被意外通过其他路径调用：初始零值保持不可见，
	// 且唯一的武装路径是 Observe/ArmMarker。
	if feedback.Observe(0) {
		t.Fatalf("零 tick 不应武装")
	}
	if feedback.MarkerVisible() {
		t.Fatalf("零 tick 后可见，说明非法武装")
	}
}

// TestCombatMarkerChangePointsDriveSingleHudPush 钉住 marker 计时与 HUD 推送
// 纪律层的衔接：计时状态留在 `combatFeedback`，只有武装与到期两个状态变化点
// 驱动下行；呈现帧计数、失败不消耗与同窗重武装都不产生额外下行。
func TestCombatMarkerChangePointsDriveSingleHudPush(t *testing.T) {
	var feedback combatFeedback
	var window fakeInteractiveWindow
	var assembleCalls int
	scheduler := client.NewUIHudPushScheduler(&window, func() client.UIHudState {
		assembleCalls++
		return client.UIHudState{
			Viewport: client.NewUIHudViewport(1280, 720),
			Marker:   feedback.MarkerVisible(),
		}
	})

	// 武装：唯一的变化点之一，恰一次下行且携带可见位。
	feedback.Observe(1)
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("武装后的冲刷应下行")
	}
	if got := string(window.pushedUIStates[0]); !strings.Contains(got, `"marker":true`) {
		t.Fatalf("武装下行应携带可见 marker: %s", got)
	}

	// 窗口内的成功呈现逐帧消耗，可见位不变：无脏标记的冲刷零推送、零求值。
	for i := 0; i < int(combatMarkerFrameCount)-1; i++ {
		feedback.AfterRender(true)
		if !feedback.MarkerVisible() {
			t.Fatalf("第 %d 次呈现后提前到期", i+1)
		}
		if scheduler.Flush() {
			t.Fatal("可见位不变时不应下行")
		}
	}
	if got := len(window.pushedUIStates); got != 1 {
		t.Fatalf("窗口内推送次数 = %d, want 1", got)
	}
	if assembleCalls != 1 {
		t.Fatalf("无脏标记的冲刷不应求值组装, 实际 %d 次", assembleCalls)
	}

	// 失败呈现不消耗：状态不变，即使被标记为脏也保持零推送。
	feedback.AfterRender(false)
	scheduler.Mark()
	if scheduler.Flush() {
		t.Fatal("呈现失败后状态不变，不得下行")
	}

	// 窗口内重武装只重置帧计数，可见位保持可见：呈现状态不变即零推送。
	feedback.ArmMarker()
	scheduler.Mark()
	if scheduler.Flush() {
		t.Fatal("可见位不变的重武装不应下行")
	}
	if feedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("重武装未重置帧计数: %d", feedback.remainingFrames)
	}

	// 重置后的窗口再次逐帧耗尽，最后一次成功呈现触发到期：第二个变化点，
	// 恰一次下行。不可见位按 schema 的可选布尔缺席表达，与武装载荷形成差异。
	for i := 0; i < int(combatMarkerFrameCount); i++ {
		feedback.AfterRender(true)
	}
	if feedback.MarkerVisible() {
		t.Fatal("重置后的六次成功呈现应到期")
	}
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("到期后的冲刷应下行")
	}
	if got := string(window.pushedUIStates[1]); strings.Contains(got, `"marker":true`) {
		t.Fatalf("到期下行不得再携带可见 marker: %s", got)
	} else if got == string(window.pushedUIStates[0]) {
		t.Fatal("到期载荷应与武装载荷不同")
	}

	// 到期后的重武装让 marker 重新可见：第三个变化点，恰一次下行携带可见位。
	feedback.ArmMarker()
	scheduler.Mark()
	if !scheduler.Flush() {
		t.Fatal("到期后重武装的冲刷应下行")
	}
	if got := string(window.pushedUIStates[2]); !strings.Contains(got, `"marker":true`) {
		t.Fatalf("重武装下行应携带可见 marker: %s", got)
	}
	if got := len(window.pushedUIStates); got != 3 {
		t.Fatalf("三个变化点应恰产生三次下行, 实际 %d", got)
	}
	// 求值只发生在有脏标记的冲刷（含终态未变化的两次），三次下行之外无多余求值。
	if assembleCalls != 5 {
		t.Fatalf("组装求值次数 = %d, want 5", assembleCalls)
	}
}

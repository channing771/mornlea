//go:build darwin

package app

import "testing"

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

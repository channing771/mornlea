//go:build darwin

package app

const combatMarkerFrameCount uint8 = 6

// combatFeedback 跟踪严格递增的战斗确认与 6 帧击中标记的剩余呈现。
type combatFeedback struct {
	lastServerTick  uint64
	remainingFrames uint8
}

func (feedback *combatFeedback) Observe(serverTick uint64) bool {
	if serverTick <= feedback.lastServerTick {
		return false
	}
	feedback.lastServerTick = serverTick
	feedback.remainingFrames = combatMarkerFrameCount
	return true
}

func (feedback *combatFeedback) ArmMarker() { feedback.remainingFrames = combatMarkerFrameCount }

func (feedback *combatFeedback) MarkerVisible() bool { return feedback.remainingFrames > 0 }

// AfterRender 消耗一个成功呈现帧，返回 marker 是否在本帧到期。计时语义不变：
// 只有 renderer 实际成功呈现才消耗，失败不消耗。到期是 hud 分节的变化源（marker
// 显隐由 WebView 组件按下行驱动），调用方据此置脏。
func (feedback *combatFeedback) AfterRender(rendered bool) (expired bool) {
	if !rendered || feedback.remainingFrames == 0 {
		return false
	}
	feedback.remainingFrames--
	return feedback.remainingFrames == 0
}

func (feedback *combatFeedback) Reset() { *feedback = combatFeedback{} }

func (a *Application) ArmCombatMarker() { a.combatFeedback.ArmMarker() }

func (a *Application) ResetCombatFeedback() { a.combatFeedback.Reset() }

func (a *Application) CombatMarkerVisible() bool { return a.combatFeedback.MarkerVisible() }

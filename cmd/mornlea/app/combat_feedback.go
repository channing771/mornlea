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

func (feedback *combatFeedback) AfterRender(rendered bool) {
	if rendered && feedback.remainingFrames > 0 {
		feedback.remainingFrames--
	}
}

func (feedback *combatFeedback) Reset() { *feedback = combatFeedback{} }

func (a *Application) ArmCombatMarker() { a.combatFeedback.ArmMarker() }

func (a *Application) ResetCombatFeedback() { a.combatFeedback.Reset() }

func (a *Application) CombatMarkerVisible() bool { return a.combatFeedback.MarkerVisible() }

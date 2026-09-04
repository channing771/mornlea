//go:build darwin

package app

import "time"

const damageFeedbackDuration = 180 * time.Millisecond

// DamageFeedback 只根据确认生命值维护本地呈现计时，不预测任何伤害。
type DamageFeedback struct {
	HasHealth bool
	Health    uint8
	Remaining time.Duration
}

// update 接收本帧确认生命值并返回 0..1 的遮罩强度。
func (feedback *DamageFeedback) update(
	health uint8,
	ready bool,
	elapsed time.Duration,
) float32 {
	if !ready {
		feedback.Reset()
		return 0
	}
	if !feedback.HasHealth {
		feedback.HasHealth = true
		feedback.Health = health
		return 0
	}
	damaged := health < feedback.Health
	feedback.Health = health
	if damaged {
		feedback.Remaining = damageFeedbackDuration
		return 1
	}
	if elapsed > 0 {
		if elapsed >= feedback.Remaining {
			feedback.Remaining = 0
		} else {
			feedback.Remaining -= elapsed
		}
	}
	return float32(feedback.Remaining) / float32(damageFeedbackDuration)
}

// Reset 清除当前会话的确认基线与呈现计时。
func (feedback *DamageFeedback) Reset() {
	*feedback = DamageFeedback{}
}

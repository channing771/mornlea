package render

import "time"

// ViewmodelMotion 是主键的本地呈现时钟；只消费显式 elapsed，不读取墙钟或权威 tick。
// 按住时用取模推进，长暂停也不补跑历史动作；松开后完成当前一挥。
type ViewmodelMotion struct {
	age      time.Duration
	duration time.Duration
	active   bool
	held     bool
}

// Advance 推进上一段动作，再观测当前有效主键；动作中再次点击不截断完整轨迹。
func (m *ViewmodelMotion) Advance(elapsed time.Duration, primary bool, tier ViewmodelTier) {
	if elapsed < 0 {
		elapsed = 0
	}
	if m.active {
		if elapsed >= m.duration-m.age {
			if primary && m.held {
				m.age = (elapsed - (m.duration - m.age)) % m.duration
			} else {
				m.active = false
				m.age = 0
			}
		} else {
			m.age += elapsed
		}
	}
	if primary && !m.active {
		_, ticks := ViewmodelSwingParams(tier)
		m.duration = time.Duration(ticks) * 50 * time.Millisecond
		m.age = 0
		m.active = true
	}
	m.held = primary
}

// Phase 返回完整动作的归一化时间；起挥首帧保留可见预备姿势。
func (m *ViewmodelMotion) Phase() (bool, float32) {
	if !m.active {
		return false, 0
	}
	return true, float32(float64(m.age) / float64(m.duration))
}

// ViewmodelClickAngle 的负段预备、正段下挥、末段回收均在同一握持根上完成。
func ViewmodelClickAngle(active bool, phase float32, tier ViewmodelTier) float32 {
	if !active || phase >= 1 {
		return 0
	}
	amplitude, _ := ViewmodelSwingParams(tier)
	if phase < .2 {
		return -amplitude * (.12 + .48*max(phase, 0)/.2)
	}
	if phase < .48 {
		return amplitude * (-.6 + 1.6*(phase-.2)/.28)
	}
	p := (phase - .48) / .52
	return amplitude * (1 - p*p*(3-2*p))
}

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

// Phase 返回完整动作的归一化时间；起挥首帧为中立，后续显式时间连续推进预备姿势。
func (m *ViewmodelMotion) Phase() (bool, float32) {
	if !m.active {
		return false, 0
	}
	return true, float32(float64(m.age) / float64(m.duration))
}

// `viewmodelPose` 将短预备、快速工作段和较慢回收写成共同握持根的相机空间姿态。
// 位移的 XY 使用归一化投影跨度；深度独立推进，避免只增加 Z 却原地摆动。
type viewmodelPose struct{ x, y, forward, pitch, yaw, roll float32 }

func viewmodelPhasePose(phase float32, tier ViewmodelTier) viewmodelPose {
	if phase <= 0 || phase >= 1 {
		return viewmodelPose{}
	}
	wind := viewmodelPose{x: -.01, y: .02, forward: .015, roll: .025}
	hit := viewmodelPose{x: -.22, y: .20, forward: .14, pitch: -.35, yaw: -.10, roll: .30}
	switch tier {
	case ViewmodelTierSword:
		hit = viewmodelPose{x: -.20, y: .32, forward: .10, pitch: -.22, yaw: -.18, roll: .80}
	case ViewmodelTierPick, ViewmodelTierAxe:
		hit = viewmodelPose{x: -.12, y: .22, forward: .15, pitch: -.95, yaw: .08, roll: .32}
	case ViewmodelTierHoe:
		hit = viewmodelPose{x: -.11, y: .20, forward: .14, pitch: -.80, yaw: -.12, roll: .38}
	case ViewmodelTierBlock:
		hit = viewmodelPose{x: -.20, y: .18, forward: .10, pitch: -.30, yaw: -.08, roll: .25}
	}
	var a, b viewmodelPose
	var t float32
	switch {
	case phase < .16:
		b = wind
		t = phase / .16
	case phase < .40:
		a = wind
		b = hit
		t = (phase - .16) / .24
	default:
		a = hit
		t = (phase - .40) / .60
	}
	t = t * t * (3 - 2*t)
	return viewmodelPose{x: a.x + (b.x-a.x)*t, y: a.y + (b.y-a.y)*t, forward: a.forward + (b.forward-a.forward)*t, pitch: a.pitch + (b.pitch-a.pitch)*t, yaw: a.yaw + (b.yaw-a.yaw)*t, roll: a.roll + (b.roll-a.roll)*t}
}

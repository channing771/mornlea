package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// TestPassiveGrazeMuzzleReachesGrass 锁定吻部贴草几何：放牧低头后牛头包围盒
// 最低点距站立平面必须 ≤0.5 格（常量由牛头包围盒与转轴反推，替代固定经验角）。
func TestPassiveGrazeMuzzleReachesGrass(t *testing.T) {
	position := mgl32.Vec3{4, 5, 6}
	grazing := buildAvatarParts(nil, []Avatar{{
		Key: PassiveEntityKey(7), Position: position, Pitch: PassiveGrazeHeadPitch(true),
	}})
	bounds := transformedUnitCubeBounds(grazing[0].transform)
	if distance := bounds.min[1] - position.Y(); distance > 0.5 {
		t.Fatalf("吻部距站立平面=%v，想要 ≤0.5 格", distance)
	}
	// 常态吻部在上方：低头必须带来可辨下压。
	standing := buildAvatarParts(nil, []Avatar{{Key: PassiveEntityKey(7), Position: position}})
	standingBounds := transformedUnitCubeBounds(standing[0].transform)
	if bounds.min[1] >= standingBounds.min[1] {
		t.Fatalf("放牧吻部高度=%v，想要低于常态 %v", bounds.min[1], standingBounds.min[1])
	}
}

// TestPassiveIdleNodIsTickDrivenAndAuthorityFree 锁定闲时点头：非放牧非死亡的
// 牛头俯仰是权威 tick 的慢速小幅正弦纯函数（禁用墙钟），不碰位置/朝向/生命。
func TestPassiveIdleNodIsTickDrivenAndAuthorityFree(t *testing.T) {
	first := PassiveIdleNodPitch(100, 7)
	repeat := PassiveIdleNodPitch(100, 7)
	if first != repeat {
		t.Fatal("同 tick 闲时点头不一致，想要纯函数")
	}
	// 40 tick 内有肉眼可辨的起伏。
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for tick := uint64(100); tick < 140; tick++ {
		pitch := PassiveIdleNodPitch(tick, 7)
		lo, hi = min(lo, pitch), max(hi, pitch)
		if pitch < -0.2 || pitch > 0.2 {
			t.Fatalf("tick=%d 点头=%v，想要小幅（±0.2 内）", tick, pitch)
		}
	}
	if hi-lo < 0.05 {
		t.Fatalf("40 tick 起伏=%v，想要肉眼可辨（≥0.05）", hi-lo)
	}
	// 不同个体错相，避免整齐划一。
	same := true
	for tick := uint64(100); tick < 140; tick++ {
		if PassiveIdleNodPitch(tick, 7) != PassiveIdleNodPitch(tick, 9) {
			same = false
			break
		}
	}
	if same {
		t.Fatal("两头牛点头恒等，想要 ID 参与派生")
	}
}

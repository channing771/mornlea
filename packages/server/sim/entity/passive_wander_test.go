package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// 本文件锁定漫游的分段稳定朝向契约：目标朝向按固定长度 tick 段组织，段内由
// 世界种子、段序号与牛 ID 确定性派生且保持不变；跨段经既有有界转向角过渡，
// 单 tick 转角不超上限、绝不瞬跳。用例逐 tick 递进权威时钟——冻结 tick 时
// 新旧实现都给出恒定朝向，正是原缺陷漏网之处。

// wanderSegmentTicks 用测试侧字面量锁定段长契约（40 tick = 2 秒）：刻意不复
// 用生产常量，段长漂移必须在此报红而非静默跟随。
const wanderSegmentTicks = 40

// wanderConvergeTicks 是段首收敛到段目标所需的最少 tick 数：段间朝向差最大
// 为 π，每 tick 有界转 `passiveIdleLookMaxTurn`，ceil(π/0.2)=16。
const wanderConvergeTicks = 16

// wanderSegmentWantYaw 纯算出指定段的漫游目标朝向，与生产派生同式：以世界种
// 子、段序号与牛 ID 哈希后取低 24 位映射到 [-π, π)。
func wanderSegmentWantYaw(seed int64, segment, id uint64) float32 {
	base := splitmix64(uint64(seed) ^ segment ^ id)
	return normalizeYaw(float32(base&0xFFFFFF) * (2 * math.Pi / 0x1000000))
}

func TestPassiveWanderHeadingHoldsWithinSegment(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	// 引擎无会话：闲时看人与引诱均不生效，牛处于纯漫游态。
	restoreGrazeCow(t, engine, 41, mgl32.Vec3{2.5, 1, 2.5})
	for segment := uint64(0); segment < 5; segment++ {
		want := wanderSegmentWantYaw(engine.seed, segment, 41)
		for offset := uint64(0); offset < wanderSegmentTicks; offset++ {
			engine.tick.Store(segment*wanderSegmentTicks + offset)
			engine.advancePassiveMovement()
			if offset < wanderConvergeTicks {
				// 段首允许有界转向逐步逼近目标，不要求立即落位。
				continue
			}
			if got := engine.passives.entries[0].yaw; got != want {
				t.Fatalf("段 %d 内 tick %d 朝向=%v，想要收敛后保持段目标 %v",
					segment, segment*wanderSegmentTicks+offset, got, want)
			}
		}
	}
}

func TestPassiveWanderTurnsBoundedAcrossSegments(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 42, mgl32.Vec3{2.5, 1, 2.5})
	previous := engine.passives.entries[0].yaw
	for tick := uint64(0); tick < 5*wanderSegmentTicks; tick++ {
		engine.tick.Store(tick)
		engine.advancePassiveMovement()
		current := engine.passives.entries[0].yaw
		if step := math.Abs(float64(normalizeYaw(current - previous))); step > 0.2001 {
			t.Fatalf("tick %d 单步转向=%v，想要有界（≤0.2），跨段也不瞬跳", tick, step)
		}
		previous = current
	}
}

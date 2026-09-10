package physics

import (
	"math"
	"testing"
)

// sneakWantEnvelope 用 movementTargetFromYaw 同函数推导潜行前移的水平包络期望，
// 不手算浮点：零初速站立起步时潜行目标速度单步可达，界即目标速度的凸包。
func sneakWantEnvelope(t *testing.T, tunables Tunables, yawSin, yawCos float32) (wantMin, wantMax [3]float32) {
	t.Helper()
	want := movementTargetFromYaw(0, 1, tunables.WalkSpeed*tunables.SneakSpeedMultiplier, yawSin, yawCos)
	if want.Len() > tunables.GroundAcceleration*FixedDeltaSeconds {
		t.Fatalf("潜行目标速度 %v 单步不可达，包络期望不再是目标凸包", want)
	}
	dt := FixedDeltaSeconds
	wantMin = [3]float32{min(0, want.X()) * dt, 0, min(0, want.Z()) * dt}
	wantMax = [3]float32{max(0, want.X()) * dt, 0, max(0, want.Z()) * dt}
	return wantMin, wantMax
}

func checkHorizontalEnvelope(t *testing.T, sweepMin, sweepMax [3]float32, wantMin, wantMax [3]float32) {
	t.Helper()
	for _, axis := range [...]int{0, 2} {
		if sweepMin[axis] != wantMin[axis] || sweepMax[axis] != wantMax[axis] {
			t.Fatalf("水平轴 %d 包络=[%v,%v]，想要 [%v,%v]", axis, sweepMin[axis], sweepMax[axis], wantMin[axis], wantMax[axis])
		}
	}
}

func TestSneakSlowsSweepBounds(t *testing.T) {
	tunables := DefaultTunables()
	state := State{OnGround: true}
	input := Input{MoveZ: 1, Sneaking: true}
	yawSin := float32(math.Sin(float64(input.Yaw)))
	yawCos := float32(math.Cos(float64(input.Yaw)))
	sweepMin, sweepMax := stepSweepBounds(state, input, tunables, yawSin, yawCos)
	wantMin, wantMax := sneakWantEnvelope(t, tunables, yawSin, yawCos)
	checkHorizontalEnvelope(t, [3]float32(sweepMin), [3]float32(sweepMax), wantMin, wantMax)
}

func TestSneakTakesPriorityOverSprint(t *testing.T) {
	tunables := DefaultTunables()
	state := State{OnGround: true}
	yawSin := float32(math.Sin(0))
	yawCos := float32(math.Cos(0))
	sneakMin, sneakMax := stepSweepBounds(state, Input{MoveZ: 1, Sneaking: true}, tunables, yawSin, yawCos)
	bothMin, bothMax := stepSweepBounds(state, Input{MoveZ: 1, Sneaking: true, Sprinting: true}, tunables, yawSin, yawCos)
	if bothMin != sneakMin || bothMax != sneakMax {
		t.Fatalf("潜行+疾跑包络=[%v,%v]，想要潜行包络 [%v,%v]", bothMin, bothMax, sneakMin, sneakMax)
	}
	wantMin, wantMax := sneakWantEnvelope(t, tunables, yawSin, yawCos)
	checkHorizontalEnvelope(t, [3]float32(bothMin), [3]float32(bothMax), wantMin, wantMax)
}

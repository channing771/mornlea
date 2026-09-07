//go:build darwin

package app

// app_audio_snow_test.go：踩雪音效与踢雪尘驱动输入的本地派生性质。spec
// 「踩雪音效与踢雪粒子本地呈现」两个 Scenario 的客户端侧锚点：步频限频
// （水平位移每累计 ≥0.8 格至多一次、静止/非雪面/离开雪面静默）与踢雪事件
// 门控（疾跑或落地边沿且落足格为雪层才有事件）。断言只消费 tracker 与
// 派生函数的字面量返回值，不依赖窗口与音频设备。

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// snowWalkObservations 从 start 起以 step 步长推进 count 次并统计触发次数：
// 调用方先经 `snowSeedStrideBaseline` 建立位置基线（基线建立的那次恒不触发），
// 各分段保持连续坐标以避免把分段间隙误计入位移。
func snowWalkObservations(
	feedback *snowStepFeedback,
	start float32,
	step float32,
	count int,
	onGround bool,
	foot core.BlockID,
) int {
	plays := 0
	position := mgl32.Vec3{start, 1, 0.5}
	for range count {
		position[0] += step
		if feedback.ObserveStride(position, onGround, foot, true) {
			plays++
		}
	}
	return plays
}

// snowSeedStrideBaseline 在 start 处建立（或重锚）位置基线：基线建立的那次
// 观测恒不触发，后续位移从零起算。
func snowSeedStrideBaseline(feedback *snowStepFeedback, start float32) {
	feedback.ObserveStride(mgl32.Vec3{start, 1, 0.5}, true, core.GrassID, true)
}

// TestSnowStepPlaysBoundedFrequencyOnSnow 证雪上行走出声且限频：水平移动
// 2 格恰好 2 次触发（0.8 门控下 2.0/0.8 = 2.5 → 2），门控把播放频率钉在
// 有界步频上。
func TestSnowStepPlaysBoundedFrequencyOnSnow(t *testing.T) {
	if snowStepStride != 0.8 {
		t.Fatalf("snowStepStride = %v，想要 0.8（design 总表）", snowStepStride)
	}
	feedback := &snowStepFeedback{}
	snowSeedStrideBaseline(feedback, 0)
	plays := snowWalkObservations(feedback, 0, 0.05, 40, true, core.SnowLayer3BlockID)
	if plays < 2 || plays > 3 {
		t.Fatalf("移动 2 格触发 %d 次，想要 2..3 次（0.8 门控）", plays)
	}
	if plays != 2 {
		t.Fatalf("0.05 步长下移动 2 格触发 %d 次，想要恰 2 次", plays)
	}
}

// TestSnowStepStationaryAndOffSnowSilent 证静止与非雪面零播放：位移不足阈值
// 的原地抖动、以及干地上的整段行进都不触发。
func TestSnowStepStationaryAndOffSnowSilent(t *testing.T) {
	feedback := &snowStepFeedback{}
	snowSeedStrideBaseline(feedback, 0)
	if plays := snowWalkObservations(feedback, 0, 0.01, 50, true, core.SnowLayer2BlockID); plays != 0 {
		t.Fatalf("原地抖动 0.5 格触发 %d 次，想要 0", plays)
	}
	if plays := snowWalkObservations(feedback, 0.5, 0.1, 30, true, core.GrassID); plays != 0 {
		t.Fatalf("干地行进 3 格触发 %d 次，想要 0", plays)
	}
}

// TestSnowStepStopsAfterLeavingSnow 证离开雪面停止：雪上首步触发后，干地
// 位移不触发；回到雪面后按新阈值重新触发。
func TestSnowStepStopsAfterLeavingSnow(t *testing.T) {
	feedback := &snowStepFeedback{}
	snowSeedStrideBaseline(feedback, 0)
	if plays := snowWalkObservations(feedback, 0, 0.4, 2, true, core.SnowLayer4BlockID); plays != 1 {
		t.Fatalf("雪上位移 0.8 格触发 %d 次，想要 1", plays)
	}
	if plays := snowWalkObservations(feedback, 0.8, 0.4, 4, true, core.GrassID); plays != 0 {
		t.Fatalf("离开雪面后的干地位移触发 %d 次，想要 0", plays)
	}
	if plays := snowWalkObservations(feedback, 2.4, 0.4, 2, true, core.SnowLayer1BlockID); plays != 1 {
		t.Fatalf("回到雪面后的位移触发 %d 次，想要 1", plays)
	}
}

// TestSnowStepAirborneDistanceNotAccumulated 证空中位移不是落足移动：不累计
// 也不清零，落地后的落足位移从头起算。
func TestSnowStepAirborneDistanceNotAccumulated(t *testing.T) {
	feedback := &snowStepFeedback{}
	snowSeedStrideBaseline(feedback, 0)
	if plays := snowWalkObservations(feedback, 0, 0.6, 1, false, core.SnowLayer3BlockID); plays != 0 {
		t.Fatalf("空中位移触发了踩雪音 %d 次", plays)
	}
	if plays := snowWalkObservations(feedback, 0.6, 0.3, 1, true, core.SnowLayer3BlockID); plays != 0 {
		t.Fatalf("落地后 0.3 格触发 %d 次，想要 0（空中 0.6 未累计）", plays)
	}
	if plays := snowWalkObservations(feedback, 0.9, 0.6, 1, true, core.SnowLayer3BlockID); plays != 1 {
		t.Fatalf("落足累计 0.9 格触发 %d 次，想要 1", plays)
	}
}

// TestSnowStepTeleportReanchorsBaseline 证和解/传送跳变重锚基线：大幅瞬移
// 不触发也不累计，其后的正常行进从零起算。
func TestSnowStepTeleportReanchorsBaseline(t *testing.T) {
	feedback := &snowStepFeedback{}
	snowSeedStrideBaseline(feedback, 0)
	if feedback.ObserveStride(mgl32.Vec3{30, 1, 0.5}, true, core.SnowLayer3BlockID, true) {
		t.Fatal("30 格瞬移触发了踩雪音")
	}
	if plays := snowWalkObservations(feedback, 30, 0.4, 1, true, core.SnowLayer3BlockID); plays != 0 {
		t.Fatalf("瞬移后 0.4 格触发 %d 次，想要 0（基线已重锚）", plays)
	}
	// 步长取 0.5（二进制精确），避免大坐标下 float32 累计误差干扰门控判定。
	if plays := snowWalkObservations(feedback, 30.4, 0.5, 2, true, core.SnowLayer3BlockID); plays != 1 {
		t.Fatalf("重锚后累计 1.0 格触发 %d 次，想要 1", plays)
	}
}

// TestSnowStepFootViewUnavailableSilent 证落足格视图未就绪时静默：缺块宁可
// 漏响也不凭空出声（与 `MirrorCollisionSource.BlockIDAt` 的回退契约同向）。
func TestSnowStepFootViewUnavailableSilent(t *testing.T) {
	feedback := &snowStepFeedback{}
	feedback.ObserveStride(mgl32.Vec3{0, 1, 0.5}, true, 0, true)
	plays := 0
	for step := 1; step <= 5; step++ {
		if feedback.ObserveStride(mgl32.Vec3{float32(step) * 0.4, 1, 0.5}, true, 0, false) {
			plays++
		}
	}
	if plays != 0 {
		t.Fatalf("落足格未就绪时触发 %d 次，想要 0", plays)
	}
}

// snowKickStates 构造一对前后物理状态：before 空中、after 落地时可显式控制。
func snowKickStates(beforeGround, afterGround bool) (physics.State, physics.State) {
	before := physics.State{
		Position: mgl32.Vec3{0.5, 5, 0.5}, OnGround: beforeGround,
	}
	after := physics.State{
		Position: mgl32.Vec3{0.9, 1, 0.5}, OnGround: afterGround,
	}
	return before, after
}

// TestDeriveSnowKickGatesOnSnowAndEventEdges 证踢雪事件门控：只有疾跑（意图
// +前移+落足）或落地边沿、且落足格为雪层才有事件；晴雪后疾跑与落地都在此
// 门内，非雪面（含未就绪视图）恒零事件。
func TestDeriveSnowKickGatesOnSnowAndEventEdges(t *testing.T) {
	snow := core.SnowLayer3BlockID
	cases := []struct {
		name     string
		before   physics.State
		after    physics.State
		sprint   bool
		moveZ    int8
		foot     core.BlockID
		loaded   bool
		wantKick bool
	}{
		{
			name: "雪面疾跑中", before: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			after:  physics.State{Position: mgl32.Vec3{0.9, 1, 0.5}, OnGround: true},
			sprint: true, moveZ: 1, foot: snow, loaded: true, wantKick: true,
		},
		{
			name: "干地疾跑中", before: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			after:  physics.State{Position: mgl32.Vec3{0.9, 1, 0.5}, OnGround: true},
			sprint: true, moveZ: 1, foot: core.GrassID, loaded: true,
		},
		{
			name: "雪面疾跑但无前移", before: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			after:  physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			sprint: true, moveZ: 0, foot: snow, loaded: true,
		},
		{
			name: "雪面普通行走", before: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			after:  physics.State{Position: mgl32.Vec3{0.9, 1, 0.5}, OnGround: true},
			sprint: false, moveZ: 1, foot: snow, loaded: true,
		},
		{
			name: "雪面落地边沿", before: physics.State{Position: mgl32.Vec3{0.5, 5, 0.5}, OnGround: false},
			after: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			foot:  snow, loaded: true, wantKick: true,
		},
		{
			name: "干地落地边沿", before: physics.State{Position: mgl32.Vec3{0.5, 5, 0.5}, OnGround: false},
			after: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			foot:  core.GrassID, loaded: true,
		},
		{
			name: "雪面持续着地无事件", before: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			after: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			foot:  snow, loaded: true,
		},
		{
			name: "落足格视图未就绪", before: physics.State{Position: mgl32.Vec3{0.5, 5, 0.5}, OnGround: false},
			after: physics.State{Position: mgl32.Vec3{0.5, 1, 0.5}, OnGround: true},
			foot:  0, loaded: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := deriveSnowKick(tc.before, tc.after, tc.sprint, tc.moveZ, tc.foot, tc.loaded)
			if input.Kick != tc.wantKick {
				t.Fatalf("Kick = %v，想要 %v", input.Kick, tc.wantKick)
			}
			if input.Kick {
				dx := input.Feet.X() - tc.after.Position.X()
				dy := input.Feet.Y() - tc.after.Position.Y()
				dz := input.Feet.Z() - tc.after.Position.Z()
				if math.Abs(float64(dx)) > 1e-6 || math.Abs(float64(dy)) > 1e-6 || math.Abs(float64(dz)) > 1e-6 {
					t.Fatalf("事件脚位 %v 与落足位置 %v 不一致", input.Feet, tc.after.Position)
				}
			}
		})
	}
}

// TestDeriveSnowKickZeroInputOnNoEvent 证无事件帧的驱动输入为零值：装配侧
// 对零值输入不重锚，上一事件的雪尘按寿命自然老化。
func TestDeriveSnowKickZeroInputOnNoEvent(t *testing.T) {
	before, after := snowKickStates(true, true)
	if input := deriveSnowKick(before, after, false, 1, core.SnowLayer3BlockID, true); input != (render.SnowKickInput{}) {
		t.Fatalf("无事件帧输入 = %+v，想要零值", input)
	}
}

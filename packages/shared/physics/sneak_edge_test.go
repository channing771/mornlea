package physics

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// sneakCliffSource 是单格悬崖桩：`x<0` 的 `y=-1` 格是满方块（顶面 `y=0`），
// 其余格均为空气（已加载）；`unloaded` 置位时所有查询都报未加载。
type sneakCliffSource struct {
	queries  int
	unloaded bool
}

func (s *sneakCliffSource) CollisionBoxes(position core.BlockPos) CollisionBoxSet {
	s.queries++
	if s.unloaded {
		return CollisionBoxSet{}
	}
	if position.Y == -1 && position.X < 0 {
		return CollisionBoxSet{Loaded: true, Count: 1, Boxes: [8]core.AABB{{Max: mgl32.Vec3{1, 1, 1}}}}
	}
	return CollisionBoxSet{Loaded: true}
}

// sneakYawForForward 用 `movementTargetFromYaw` 反推 yaw：扫描一周取前向
// 意图最接近目标水平方向的那个值，不手算角度。
func sneakYawForForward(t *testing.T, wantX, wantZ float32) float32 {
	t.Helper()
	bestYaw := 0.0
	bestDist := math.MaxFloat64
	for deg := 0; deg < 3600; deg++ {
		yaw := float64(deg) * math.Pi / 1800
		target := movementTargetFromYaw(0, 1, 1, float32(math.Sin(yaw)), float32(math.Cos(yaw)))
		dx := float64(target.X() - wantX)
		dz := float64(target.Z() - wantZ)
		if dist := dx*dx + dz*dz; dist < bestDist {
			bestDist, bestYaw = dist, yaw
		}
	}
	yaw := float32(bestYaw)
	target := movementTargetFromYaw(0, 1, 1, float32(math.Sin(bestYaw)), float32(math.Cos(bestYaw)))
	if target.Y() != 0 || math.Abs(float64(target.X()-wantX)) > 1e-3 || math.Abs(float64(target.Z()-wantZ)) > 1e-3 {
		t.Fatalf("反推的前向 %v 不指向 (%v, 0, %v)", target, wantX, wantZ)
	}
	return yaw
}

func TestSneakEdgeCliffLosesSupport(t *testing.T) {
	yaw := sneakYawForForward(t, 1, 0)
	state := State{Position: mgl32.Vec3{-0.2, 0, 0}, OnGround: true}
	source := &sneakCliffSource{}
	if SneakEdgeHolds(state, 0, 1, yaw, source) {
		t.Fatalf("悬崖边潜行前移应判失支撑")
	}
	if source.queries > 2 {
		t.Fatalf("边缘探针查询 %d 次，超过每步 2 次上界", source.queries)
	}
}

func TestSneakEdgeZeroIntentHolds(t *testing.T) {
	yaw := sneakYawForForward(t, 1, 0)
	state := State{Position: mgl32.Vec3{-0.2, 0, 0}, OnGround: true}
	source := &sneakCliffSource{}
	if !SneakEdgeHolds(state, 0, 0, yaw, source) {
		t.Fatalf("零意图不得钳制")
	}
	if source.queries != 0 {
		t.Fatalf("零意图查询 %d 次，想要 0 次", source.queries)
	}
}

func TestSneakEdgeUnloadedHolds(t *testing.T) {
	yaw := sneakYawForForward(t, 1, 0)
	state := State{Position: mgl32.Vec3{-0.2, 0, 0}, OnGround: true}
	source := &sneakCliffSource{unloaded: true}
	if !SneakEdgeHolds(state, 0, 1, yaw, source) {
		t.Fatalf("未加载格应按有支撑处理")
	}
}

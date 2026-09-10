package physics

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// SneakEdgeHolds 报告按本步水平意图走一步后脚底仍有支撑，是潜行边缘保护的唯一判据。
//
// 调用方必须已确认 OnGround && 潜行减速有效 && !Jump 且水平意图非零；`Jump` 主动
// 跳落、击退与水推走非输入路径，一律不经本函数（本函数不收 `Jump` 参数）。
// 未加载格按有支撑处理（不误钳制）。每步至多 2 次方块查询（两足印前缘正下方），有界。
func SneakEdgeHolds(state State, moveX, moveZ int8, yaw float32, source CollisionSource) bool {
	if moveX == 0 && moveZ == 0 {
		return true
	}
	yawSin := float32(math.Sin(float64(yaw)))
	yawCos := float32(math.Cos(float64(yaw)))
	// 注：用单位速度求方向，不含 WalkSpeed，方向与速度档无关。
	direction := movementTargetFromYaw(moveX, moveZ, 1, yawSin, yawCos)
	if direction.Len() == 0 {
		return true
	}
	// 前探半宽加一小段余量，落在两足印前缘；侧向张开整半宽，对齐身体前角。
	probe := state.Position.Add(direction.Mul(PlayerWidth/2 + 0.05))
	side := mgl32.Vec3{-direction.Z(), 0, direction.X()}.Mul(PlayerWidth / 2)
	// 脚底格取脚底平面下一格：`GroundProbe` 把恰好落在整数高度的脚底归到下格，
	// 与 `playerSupport` 同式；非整数顶面（耕地/床）同样归到承载格。
	footY := collisionCheckedFloor(state.Position.Y() - GroundProbe)
	for _, foot := range [...]mgl32.Vec3{probe.Add(side), probe.Sub(side)} {
		boxes := source.CollisionBoxes(core.BlockPos{
			X: collisionCheckedFloor(foot.X()),
			Y: footY,
			Z: collisionCheckedFloor(foot.Z()),
		})
		if !boxes.Loaded || boxes.Count > 0 {
			return true
		}
	}
	return false
}

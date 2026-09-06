package client

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// ThirdPersonCameraDistance 是第三人称相机在开阔地的固定后拉距离（格）：
// 背面位于眼睛沿视线反方向该距离处，正面位于视线正方向同等距离处。
// 取值承接 spec“默认约 4 格”，调用方不得各自复制字面量。
const ThirdPersonCameraDistance float32 = 4

// thirdPersonCameraInset 是防穿墙收缩保留的最小贴脸距离（格）：相机停在
// 最近阻挡点之前该距离处；阻挡比它更近时收至眼睛处，不得进入墙内。
// 取值保证大于近裁剪面（`Near` 为 0.1），墙面不糊住近裁剪面。
const thirdPersonCameraInset float32 = 0.25

// ResolveThirdPersonCamera 由眼睛位姿推导本帧渲染相机位姿，是第三人称
// 后拉与防穿墙的唯一落点。`base.Pos` 必须是眼睛位置（调用方经与服务端
// 交互射线同源的眼高得出），瞄准、挖掘与攻击射线继续用眼睛位姿，本函数
// 只决定渲染用位姿，不改权威与瞄准语义。
//
//   - 第一人称（或非法模式）原样直通，不做阻挡查询（查询函数可为 nil）。
//   - 背面：沿视线反方向拉出固定距离，朝向与眼睛一致。
//   - 正面：沿视线正方向拉出同等距离，朝向回望玩家（yaw+Pi、pitch 取反，
//     使前向恰为眼睛前向的反方向），模型保持眼睛朝向即面向相机（可观察到
//     脸部），移动仍用眼睛朝向、与第一人称一致。
//   - 防穿墙：以眼睛为原点沿拉出方向发射体素射线（复用 `core.RaycastBlocks`
//     只读查询，不另起遍历），命中完整不透明方块时按最近阻挡点收缩并保留
//     最小贴脸距离；完全遮挡（阻挡比贴脸距离更近）收至眼睛处；查询报错
//     （未加载等）按开阔地处理，不凭未知收缩。
//
// 纯函数：同输入同输出，可重放；单次射线 traverses 至多固定距离内的格子，
// 有界工作，可进渲染热路径。
func ResolveThirdPersonCamera(base Camera, mode CameraMode, solid func(core.BlockPos) (bool, error)) Camera {
	switch mode {
	case CameraThirdPersonBack, CameraThirdPersonFront:
	default:
		return base
	}
	forward := base.Forward()
	var rayDir mgl32.Vec3
	out := base
	if mode == CameraThirdPersonBack {
		rayDir = forward.Mul(-1)
	} else {
		rayDir = forward
		out.Yaw = base.Yaw + float32(math.Pi)
		out.Pitch = -base.Pitch
	}
	if solid == nil {
		out.Pos = base.Pos.Add(rayDir.Mul(ThirdPersonCameraDistance))
		return out
	}
	hit, found, err := core.RaycastBlocks(base.Pos, rayDir, ThirdPersonCameraDistance, solid)
	if err != nil || !found {
		out.Pos = base.Pos.Add(rayDir.Mul(ThirdPersonCameraDistance))
		return out
	}
	pull := hit.Distance - thirdPersonCameraInset
	if pull <= 0 {
		out.Pos = base.Pos
		return out
	}
	out.Pos = base.Pos.Add(rayDir.Mul(pull))
	return out
}

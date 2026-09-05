package capture

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
)

// captureMiningCrackTarget 是裂纹场景采掘目标的固定坐标：target-block-feedback
// 夹具的那块砖——空气邻域中唯一的实体方块。裂纹必须贴在真实方块的表面上，
// 不允许摆拍在空气坐标上。
var captureMiningCrackTarget = core.BlockPos{X: 0, Y: 3, Z: -3}

// captureCrackCameraPos 是裂纹场景相机的固定位置：与 target-block-feedback
// 同一姿势（正视 -Z、平视），砖块 +Z 面恰好投影在屏幕中心，距离 4.5 格、
// 在选框 6 格射程之内。
var captureCrackCameraPos = mgl32.Vec3{0.5, 3.5, 2.5}

// applyMiningCrackCaptureState 装入两个裂纹场景共用的固定环境：世界由
// Prepare（复用 target-block-feedback 夹具）重装，这里显式钉死正午、相机、
// 中心与全部共享呈现状态。场景共用同一个 application，因此既不依赖前序
// 场景的清理，也不把本场景的残留泄给后续：mining 夹具经 HUD 夹具的 defer
// 语义恢复，紧随其后的 main-menu 另有一遍 resetCapturePresentation 兜底。
func applyMiningCrackCaptureState(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	*app.Camera() = client.Camera{
		Pos: captureCrackCameraPos, Yaw: 0, Pitch: 0,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetInventoryOpen(false)
	app.SetBlockTargetReset(false)
	if app.Panel() != nil {
		app.Panel().SetVisible(false)
	}
	return nil
}

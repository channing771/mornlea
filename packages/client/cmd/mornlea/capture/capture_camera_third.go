package capture

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// captureCameraThirdServerTick 是双机位场景钉死的权威 tick：自身体静止，
// 步态恒回中；晴天无降水粒子，相位只由它决定，连跑零漂移。
const captureCameraThirdServerTick = uint64(1)

// applyCameraThirdBackCaptureState 钉死第三人称背面场景的全部呈现状态。
func applyCameraThirdBackCaptureState(app SceneApplication) error {
	return applyCameraThirdCaptureState(app, client.CameraThirdPersonBack)
}

// applyCameraThirdFrontCaptureState 钉死第三人称正面场景的全部呈现状态。
func applyCameraThirdFrontCaptureState(app SceneApplication) error {
	return applyCameraThirdCaptureState(app, client.CameraThirdPersonFront)
}

// applyCameraThirdCaptureState 是双机位的唯一装配：同一眼睛位姿（人物舞台
// 草顶上一个眼高、朝 -Z），仅机位模式不同。后拉、防穿墙、自身身体追加与
// 双手互斥全部复用已有帧装配，不摆拍：`RenderFrame` 经 `resolveRenderCamera`
// 推导渲染位姿，经 `appendSelfAvatar` 追加自身体，第三人称下
// `deriveViewmodelInput` 恒为 nil（无双手像素）。
func applyCameraThirdCaptureState(app SceneApplication, mode client.CameraMode) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	app.SetServerTick(captureCameraThirdServerTick)
	*app.Camera() = client.Camera{
		Pos:    mgl32.Vec3{0.5, 1 + physics.ActiveTunables().EyeHeight, 4.5},
		Yaw:    0,
		Pitch:  -0.05,
		FovY:   mgl32.DegToRad(70),
		Aspect: float32(captureWidth) / captureHeight,
		Near:   0.1,
		Far:    2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetCameraMode(mode)
	return nil
}

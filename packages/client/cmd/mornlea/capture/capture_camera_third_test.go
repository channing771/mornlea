//go:build darwin

package capture

// capture_camera_third_test.go：第三人称双机位场景的钉死回归。
// `applyCameraThirdBackCaptureState` 与 `applyCameraThirdFrontCaptureState`
// 共用同一眼睛位姿（同世界位置、同朝向），仅 `CameraMode` 不同；呈现复用
// 已有装配（`resolveRenderCamera` 后拉 + `appendSelfAvatar` 自身身体 +
// 第三人称 viewmodel 恒 nil），不另起摆拍路径。场景暂不进 `captureScenes`，
// 由清单扩展任务统一追加；天气不碰（新鲜装配默认晴天）。

import (
	"image"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// thirdPersonFixtureEye 是双机位共用的眼睛位姿：脚底落在人物舞台草顶
// （y=1）正上方一个眼高处，朝 -Z，与人眼站立一致。
func thirdPersonFixtureEye() mgl32.Vec3 {
	return mgl32.Vec3{0.5, 1 + physics.ActiveTunables().EyeHeight, 4.5}
}

// polluteThirdPersonApplication 种植前序场景可能留下的共享呈现状态：双机位
// 的 `Apply` 必须显式清场，不依赖场景表顺序。
func polluteThirdPersonApplication(t *testing.T, app *application.Application) {
	t.Helper()
	if err := app.RemotePlayers().Apply(network.RemotePlayerSpawn{
		PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: 9}, DisplayName: "路人",
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0, 80, 0},
	}); err != nil {
		t.Fatal(err)
	}
	app.SetInventoryOpen(true)
	app.Panel().SetVisible(true)
	app.SetMiningOverlay(hud.MiningOverlay{
		Active: true, HasTarget: true, Target: captureMiningCrackTarget,
		ProgressTicks: 1, RequiredTicks: 30,
	})
	app.ChatInput().Open()
}

// TestCameraThirdScenesShareEyeAndDifferOnlyInMode 钉住双机位的夹具契约：
// 同一眼睛位置、同一朝向，仅机位模式不同；前序共享状态显式清空。
func TestCameraThirdScenesShareEyeAndDifferOnlyInMode(t *testing.T) {
	back := application.NewPresentationApplicationForTest()
	front := application.NewPresentationApplicationForTest()
	polluteThirdPersonApplication(t, back)
	polluteThirdPersonApplication(t, front)

	if err := applyCameraThirdBackCaptureState(back); err != nil {
		t.Fatalf("应用 camera-third-back: %v", err)
	}
	if err := applyCameraThirdFrontCaptureState(front); err != nil {
		t.Fatalf("应用 camera-third-front: %v", err)
	}

	wantEye := thirdPersonFixtureEye()
	for name, app := range map[string]*application.Application{"back": back, "front": front} {
		camera := app.Camera()
		if camera.Pos != wantEye || camera.Yaw != 0 || camera.Pitch != -0.05 {
			t.Fatalf("%s 眼睛 = %+v yaw=%v pitch=%v，想要 %v/0/-0.05（同位置同朝向）",
				name, camera.Pos, camera.Yaw, camera.Pitch, wantEye)
		}
		if app.WorldTimeTicks() != 6000 {
			t.Fatalf("%s world time = %d，想要 6000（固定正午）", name, app.WorldTimeTicks())
		}
		if app.ServerTick() != captureCameraThirdServerTick {
			t.Fatalf("%s server tick = %d，想要钉死的 %d", name, app.ServerTick(), captureCameraThirdServerTick)
		}
		if got := app.RemotePlayers().Presentations(); len(got) != 0 {
			t.Fatalf("%s 远端玩家未清空: %+v", name, got)
		}
		if app.InventoryOpen() || app.Panel().Visible() || app.ChatInput().IsOpen() {
			t.Fatalf("%s 共享界面状态未清空: inventoryOpen=%v panelVisible=%v chatOpen=%v",
				name, app.InventoryOpen(), app.Panel().Visible(), app.ChatInput().IsOpen())
		}
		if overlay := app.MiningOverlay(); overlay != (hud.MiningOverlay{}) {
			t.Fatalf("%s 采掘镜像未清空: %+v", name, overlay)
		}
	}
	if *back.Camera() != *front.Camera() {
		t.Fatalf("双机位眼睛不一致：back=%+v front=%+v（必须同位置同朝向）",
			*back.Camera(), *front.Camera())
	}
	if mode := back.CameraMode(); mode != client.CameraThirdPersonBack {
		t.Fatalf("back 模式 = %d，想要背面 %d", mode, client.CameraThirdPersonBack)
	}
	if mode := front.CameraMode(); mode != client.CameraThirdPersonFront {
		t.Fatalf("front 模式 = %d，想要正面 %d", mode, client.CameraThirdPersonFront)
	}
}

// thirdPersonHeadRect 以自头心为锚的屏幕矩形：头心取脚底（眼睛减眼高）
// 上 1.6 格（与 `render` 侧头部件 `1.4+0.2` 同式），渲染相机经既有后拉
// 装配推导（开阔地，无阻挡），不另写投影路径。
func thirdPersonHeadRect(t *testing.T, app *application.Application, mode client.CameraMode) image.Rectangle {
	t.Helper()
	eye := *app.Camera()
	rendered := client.ResolveThirdPersonCamera(eye, mode, nil)
	feet := eye.Pos.Sub(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0})
	center := captureSceneProject(t, rendered, feet.Add(mgl32.Vec3{0, 1.6, 0}))
	const half = 30
	return image.Rect(center.X-half, center.Y-half, center.X+half, center.Y+half)
}

// countFaceSkin 统计脸部肤色签名像素：正午光照下人脸皮肤渲染约
// (188,165,146)（r-b≈42、r-g≈23），眼白奶油色亦落入该带；草地偏绿
// （g>r）、天空偏蓝（b≥r）、石头近中性，都落不进来。
func countFaceSkin(img *image.NRGBA, rect image.Rectangle) int {
	count := 0
	rect = rect.Intersect(img.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := img.PixOffset(x, y)
			r, g, b := int(img.Pix[i]), int(img.Pix[i+1]), int(img.Pix[i+2])
			if r >= 175 && r-b >= 25 && r-b <= 60 && r-g >= 8 && r-g <= 35 {
				count++
			}
		}
	}
	return count
}

// countHairPixels 统计头发暗部像素：头发本色约 (70,50,40)，正午渲染后仍
// 是暗暖色；草地、天空、石头与皮肤都不落入该带。
func countHairPixels(img *image.NRGBA, rect image.Rectangle) int {
	count := 0
	rect = rect.Intersect(img.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			i := img.PixOffset(x, y)
			r, g, b := int(img.Pix[i]), int(img.Pix[i+1]), int(img.Pix[i+2])
			if r >= 60 && r <= 140 && r-g >= 10 && r-g <= 40 && r-b >= 20 && r-b <= 60 {
				count++
			}
		}
	}
	return count
}

// TestCameraThirdScenesShowSelfBodyAndFacesThroughFullChain 是双机位的完整
// 链路像素断言：经无窗口离屏链路抓帧，两图都显示自身身体且都不含 viewmodel
// 像素；正面可辨认脸部（眼白），背面可辨认背后（头发暗头）。渲染器是离屏
// 设备，不创建也不聚焦任何前台窗口；无 GPU 适配器时跳过。
func TestCameraThirdScenesShowSelfBodyAndFacesThroughFullChain(t *testing.T) {
	backApp := newCaptureSceneRenderApplication(t)
	if backApp.Window() != nil {
		t.Fatal("双机位像素断言必须走无窗口离屏链路，当前存在交互窗口")
	}
	backScene := captureScene{
		Name: "camera-third-back", WarmupFrames: 8,
		Prepare: prepareAvatarStage, Apply: applyCameraThirdBackCaptureState,
	}
	backImg, err := captureSceneImage(backApp, backScene)
	if err != nil {
		t.Fatalf("抓取 camera-third-back: %v", err)
	}
	frontApp := newCaptureSceneRenderApplication(t)
	frontScene := captureScene{
		Name: "camera-third-front", WarmupFrames: 8,
		Prepare: prepareAvatarStage, Apply: applyCameraThirdFrontCaptureState,
	}
	frontImg, err := captureSceneImage(frontApp, frontScene)
	if err != nil {
		t.Fatalf("抓取 camera-third-front: %v", err)
	}

	backRect := thirdPersonHeadRect(t, backApp, client.CameraThirdPersonBack)
	frontRect := thirdPersonHeadRect(t, frontApp, client.CameraThirdPersonFront)
	for name, rect := range map[string]image.Rectangle{"back": backRect, "front": frontRect} {
		if !rect.In(image.Rect(0, 0, captureWidth, captureHeight)) {
			t.Fatalf("%s 头部矩形 %v 越出画面：自身体不在画面内", name, rect)
		}
	}
	backCream := countFaceSkin(backImg, backRect)
	frontCream := countFaceSkin(frontImg, frontRect)
	t.Logf("脸部肤色像素：back=%d front=%d", backCream, frontCream)
	if frontCream < 400 {
		t.Fatalf("正面脸部肤色像素=%d：脸部不可辨认", frontCream)
	}
	if backCream > 40 {
		t.Fatalf("背面脸部肤色像素=%d：背后应为头发覆盖", backCream)
	}
	backHair := countHairPixels(backImg, backRect)
	t.Logf("背面头发像素=%d", backHair)
	if backHair < 500 {
		t.Fatalf("背面头发像素=%d：背后不可辨认（自身体缺席）", backHair)
	}

	// 双手与脸部共用肤色签名：双手若进入像素，必落在底部两角；两图该处
	// 必须零肤色签名（地面、天空与石头 marker 都无此签名）。
	corners := func() []image.Rectangle {
		return []image.Rectangle{
			image.Rect(0, captureHeight-48, 96, captureHeight),
			image.Rect(captureWidth-96, captureHeight-48, captureWidth, captureHeight),
		}
	}
	for _, corner := range corners() {
		if n := countFaceSkin(backImg, corner); n != 0 {
			t.Fatalf("背面底部角落 %v 肤色像素=%d：混入 viewmodel", corner, n)
		}
		if n := countFaceSkin(frontImg, corner); n != 0 {
			t.Fatalf("正面底部角落 %v 肤色像素=%d：混入 viewmodel", corner, n)
		}
	}

	// 确定性：同一场景连抓两次必须零漂移（固定 tick/位姿）。
	again, err := captureSceneImage(backApp, backScene)
	if err != nil {
		t.Fatalf("重复抓取 camera-third-back: %v", err)
	}
	if rediff, _, err := compareImages(backImg, again); err != nil {
		t.Fatal(err)
	} else if rediff.DiffPixels != 0 {
		t.Fatalf("同机连跑漂移：%s，想要零漂移", rediff)
	}
}

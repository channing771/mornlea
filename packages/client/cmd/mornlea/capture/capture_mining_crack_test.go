//go:build darwin

package capture

// capture_mining_crack_test.go 钉住采掘裂纹场景的夹具与像素证据：权威采掘
// 镜像必须 active 且携带固定实体目标，进度分别映射为浅阶段与重阶段；真实
// 渲染链路回读的帧里，裂纹像素必须成片出现在目标方块的投影区域内，关闭
// 目标镜像后同区域不得再有差异——两者共同充当 spec
// mining-crack-presentation 验收场景的像素蓝本。

import (
	"image"
	"image/draw"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// captureCrackTargetRegion 是目标方块 +Z 面投影中心附近的断言窗口：相机在
// (0.5,3.5,2.5) 正视 -Z，砖面距离 4.5，640×360、竖直 70° FOV 下约
// 57px/格，砖面覆盖屏幕中心约 57×57 的方形。窗口取中心 ±20px，完全落在
// 面内，避开面缘的选框线、上方的目标名称与下方的 HUD。
var captureCrackTargetRegion = image.Rect(300, 160, 340, 200)

// captureCrackMinDiffPixels 是「目标区域出现裂纹」的差异像素数下限：浅阶段
// （阶段 2）在 16×16 纹素里约两成是裂纹纹素，每纹素约 3.6px，估计两百余
// 像素；下限留出数倍余量，同时要求最大通道差超过 golden 噪声门（2），
// 排除亚噪声漂移凑数的可能。
const captureCrackMinDiffPixels = 60

// TestMiningCrackCaptureScenesPinAuthoritativeTarget 钉住两个裂纹场景的
// 权威夹具：进度 6/30 与 29/30 按 BlockCrackStage 公式分别落在浅阶段 2 与
// 最重的阶段 9（29 只到 required-1，不呈现已破坏方块的裂纹）；目标必须是
// 夹具世界里的实体砖块，且恰好是选框射线的命中点——裂纹与选框同源门控，
// 射线不命中就没有附着面。
func TestMiningCrackCaptureScenesPinAuthoritativeTarget(t *testing.T) {
	for _, tc := range []struct {
		name         string
		wantStage    int
		wantProgress uint16
	}{
		{name: "mining-crack-early", wantStage: 2, wantProgress: 6},
		{name: "mining-crack-heavy", wantStage: 9, wantProgress: 29},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scene := captureSceneByName(t, tc.name)
			if scene.WarmupFrames != 8 || scene.Prepare == nil || scene.Apply == nil {
				t.Fatalf("%s 场景不完整: %+v", tc.name, scene)
			}
			// 采掘镜像由场景 Apply 经 SetMiningOverlay 直装（HUD 夹具随
			// 常显层退役），这里用旁路夹具记录期望值并在 Apply 后核对；
			// 可采标志已随屏幕采掘条退役，镜像只携带裂纹所需字段。
			want := hud.MiningOverlay{
				Active: true, HasTarget: true, Target: captureMiningCrackTarget,
				ProgressTicks: tc.wantProgress, RequiredTicks: 30,
			}
			if got := render.BlockCrackStage(want.ProgressTicks, want.RequiredTicks); got != tc.wantStage {
				t.Fatalf("%s stage=%d，想要 %d", tc.name, got, tc.wantStage)
			}

			mesher := client.NewMesher(assets.NewRegistry(), 1)
			t.Cleanup(mesher.Close)
			app := newCaptureAICompanionState()
			app.SetMirror(client.NewMirror())
			app.SetMesher(mesher)
			predictor := client.NewPredictor()
			if err := predictor.Begin(network.PlayerState{
				Dimension: core.Overworld, Position: captureCrackCameraPos,
				Yaw: 0, Pitch: 0, Ready: true,
				Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
			}); err != nil {
				t.Fatal(err)
			}
			app.SetPredictor(predictor)
			if err := scene.Prepare(app); err != nil {
				t.Fatalf("准备 %s: %v", tc.name, err)
			}
			if err := scene.Apply(app); err != nil {
				t.Fatalf("应用 %s: %v", tc.name, err)
			}
			if got, loaded := app.Mirror().BlockAt(core.Overworld, captureMiningCrackTarget); !loaded || got != core.BrickID {
				t.Fatalf("裂纹目标 BlockAt=%d/%v，想要 BrickID/true", got, loaded)
			}
			wantCamera := client.Camera{
				Pos: captureCrackCameraPos, Yaw: 0, Pitch: 0,
				FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
				Near: 0.1, Far: 2000,
			}
			if *app.Camera() != wantCamera {
				t.Fatalf("camera=%+v，想要 %+v", *app.Camera(), wantCamera)
			}
			target, ok := app.CurrentBlockTarget()
			if !ok || target.Position != captureMiningCrackTarget {
				t.Fatalf("CurrentBlockTarget=%+v/%v，想要命中 %v", target, ok, captureMiningCrackTarget)
			}
			if mining := app.MiningOverlay(); mining != want {
				t.Fatalf("%s mining=%+v，想要 %+v", tc.name, mining, want)
			}
		})
	}
}

// TestMiningCrackCaptureScenePixelEvidence 用真实离屏渲染链路回读两帧比较
// 目标区域：有目标帧必须比关闭 HasTarget 的帧多出成片裂纹像素（差异像素
// 数与最大通道差双下限）——关闭目标帧同时充当「无目标不呈现裂纹」的抑制
// 基线（菜单相位门控由 app 侧 deriveBlockCrack 测试钉住，本装配不带菜单
// 全景管线，故选 HasTarget=false 这把剪刀差）。无目标帧重复抓帧必须逐
// 像素一致：既证明上述差异来自裂纹而非抓帧噪声，也钉住场景输出的确定性。
func TestMiningCrackCaptureScenePixelEvidence(t *testing.T) {
	app := application.NewOffscreenRenderApplicationForTest(
		t, &application.IntegrationGlyphSource{}, captureWidth, captureHeight,
		config.Defaults().Render)
	// 选框（进而裂纹）以 predictor 就绪为前提；眼睛位置随相机，落在空气中，
	// 不触发水下视觉路径。
	predictor := client.NewPredictor()
	if err := predictor.Begin(network.PlayerState{
		Dimension: core.Overworld, Position: captureCrackCameraPos,
		Yaw: 0, Pitch: 0, Ready: true,
		Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
	}); err != nil {
		t.Fatal(err)
	}
	app.SetPredictor(predictor)

	// suppressed 在原夹具 Apply 之后把采掘镜像的 HasTarget 关掉：选框与
	// 世界内容保持不变，目标区域的差异因此只可能来自裂纹本身（HUD 进度条
	// 已迁 WebView，不参与本对照）。
	suppressed := func(scene captureScene) captureScene {
		copied := scene
		inner := copied.Apply
		copied.Apply = func(app SceneApplication) error {
			if err := inner(app); err != nil {
				return err
			}
			overlay := app.MiningOverlay()
			overlay.HasTarget = false
			app.SetMiningOverlay(overlay)
			return nil
		}
		return copied
	}
	// cropRegion 抽出零起点坐标的区域副本：compareImages 的 PixOffset 调用
	// 假定图像 Min 在原点，直接传 SubImage 的裁剪视图会算出负偏移。
	cropRegion := func(img *image.NRGBA) *image.NRGBA {
		cropped := image.NewNRGBA(image.Rect(0, 0, captureCrackTargetRegion.Dx(), captureCrackTargetRegion.Dy()))
		draw.Draw(cropped, cropped.Bounds(), img, captureCrackTargetRegion.Min, draw.Src)
		return cropped
	}
	regionDiff := func(got, want *image.NRGBA) imageDiff {
		t.Helper()
		diff, _, err := compareImages(cropRegion(got), cropRegion(want))
		if err != nil {
			t.Fatal(err)
		}
		return diff
	}

	for _, tc := range []struct{ name string }{
		{name: "mining-crack-early"},
		{name: "mining-crack-heavy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scene := captureSceneByName(t, tc.name)
			cracked, err := captureSceneImage(app, scene)
			if err != nil {
				t.Fatalf("抓取裂纹帧: %v", err)
			}
			plain, err := captureSceneImage(app, suppressed(scene))
			if err != nil {
				t.Fatalf("抓取无目标帧: %v", err)
			}
			diff := regionDiff(cracked, plain)
			t.Logf("目标区域裂纹差异：%s", diff)
			if diff.DiffPixels < captureCrackMinDiffPixels || diff.MaxChannelDelta <= 2 {
				t.Fatalf("目标区域未见裂纹像素：%s（下限 %d 像素且最大通道差 > 2）",
					diff, captureCrackMinDiffPixels)
			}
		})
	}

	heavy := captureSceneByName(t, "mining-crack-heavy")
	plain, err := captureSceneImage(app, suppressed(heavy))
	if err != nil {
		t.Fatalf("抓取无目标帧: %v", err)
	}
	plainAgain, err := captureSceneImage(app, suppressed(heavy))
	if err != nil {
		t.Fatalf("重复抓取无目标帧: %v", err)
	}
	if diff := regionDiff(plainAgain, plain); diff.DiffPixels != 0 {
		t.Fatalf("无目标帧重复抓帧在目标区域漂移：%s，区域=%v", diff, captureCrackTargetRegion)
	}
}

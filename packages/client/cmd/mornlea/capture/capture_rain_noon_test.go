//go:build darwin

package capture

// capture_rain_noon_test.go：雨天正午场景的钉死回归。`prepareRainNoon` 装入
// 种子 42 的固定地形（与橡树林同一批 3×3 生成区块），
// `applyRainNoonCaptureState` 钉死正午、雪线下机位、雨天权威天气与固定 tick。
// 本文件只断言夹具数据面与 CPU 侧呈现输入（降水实例、灰化、压暗），
// golden 基线由后续任务承接；场景已进 `captureScenes`（紧随 mining-crack-heavy）。

import (
	"bytes"
	"encoding/binary"
	"math"
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
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// rainNoonCameraPos 是雨天场景的固定机位：与橡树林同一雪线下机位，测试侧
// 只读，不重新推导选点。
var rainNoonCameraPos = mgl32.Vec3{-3.5, 75.5, 12.5}

// newRainNoonTestApplication 构造带空镜像与已就绪预测器的最小呈现应用：
// 预测器从晴天开始，`Apply` 的雨天注入必须经和解口径把它推成雨天。
func newRainNoonTestApplication(t *testing.T) *application.Application {
	t.Helper()
	app := application.NewPresentationApplicationForTest()
	app.SetMirror(client.NewMirror())
	predictor := client.NewPredictor()
	if err := predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: rainNoonCameraPos, Yaw: 0, Pitch: -0.38, Ready: true,
		Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
		WorldTimeTicks: 6000, WeatherKind: core.WeatherClear,
	}); err != nil {
		t.Fatalf("预置预测器: %v", err)
	}
	app.SetPredictor(predictor)
	return app
}

// TestRainNoonApplyPinsNoonRainAndFixedTick 钉住雨天场景的全部呈现状态：
// 固定正午、固定雪线下机位、呈现侧与预测侧同为雨天、粒子相位 tick 钉死，
// 前序场景留下的共享呈现状态显式清空（无 HUD 像素的落点之一）。
func TestRainNoonApplyPinsNoonRainAndFixedTick(t *testing.T) {
	app := newRainNoonTestApplication(t)
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

	if err := applyRainNoonCaptureState(app); err != nil {
		t.Fatalf("应用 rain-noon: %v", err)
	}
	if app.WorldTimeTicks() != 6000 {
		t.Fatalf("world time = %d，想要 6000（固定正午）", app.WorldTimeTicks())
	}
	if app.Camera().Pos != rainNoonCameraPos || app.Camera().Yaw != 0 || app.Camera().Pitch != -0.38 {
		t.Fatalf("camera = %+v yaw=%v pitch=%v，想要 %v/0/-0.38（固定雪线下机位）",
			app.Camera().Pos, app.Camera().Yaw, app.Camera().Pitch, rainNoonCameraPos)
	}
	if app.ServerTick() != captureRainNoonServerTick {
		t.Fatalf("server tick = %d，想要钉死的 %d（雨粒子相位只由固定 tick 决定）",
			app.ServerTick(), captureRainNoonServerTick)
	}
	if app.Weather() != core.WeatherRain {
		t.Fatalf("呈现天气 = %d，想要雨天 %d", app.Weather(), core.WeatherRain)
	}
	if weather, ready := app.Predictor().Weather(); !ready || weather != core.WeatherRain {
		t.Fatalf("预测器天气 = %d/ready=%v，想要雨天 %d/true（注入必须经预测器接受口径）",
			weather, ready, core.WeatherRain)
	}
	// 场景钉分点正午：注入的 PlayerState 未携带季节字段（零值即春始），
	// 呈现侧 yearPhase 必须为 0——昼弧 12000 warp 恒等、冷色 tint 权重 0。
	if got := app.YearPhase(); got != 0 {
		t.Fatalf("rain-noon 的 yearPhase = %v，想要 0（分点正午）", got)
	}
	if got := app.RemotePlayers().Presentations(); len(got) != 0 {
		t.Fatalf("远端玩家未清空: %+v", got)
	}
	if app.InventoryOpen() || app.Panel().Visible() || app.ChatInput().IsOpen() {
		t.Fatalf("共享界面状态未清空: inventoryOpen=%v panelVisible=%v chatOpen=%v",
			app.InventoryOpen(), app.Panel().Visible(), app.ChatInput().IsOpen())
	}
	if overlay := app.MiningOverlay(); overlay != (hud.MiningOverlay{}) {
		t.Fatalf("采掘镜像未清空: %+v", overlay)
	}
}

// TestRainNoonPrepareLoadsFixedSeedTerrain 钉住雨天场景的地形夹具：与橡树林
// 同一种子、同一批 3×3 生成区块，相机格是空气（机位不在地形内部）。
func TestRainNoonPrepareLoadsFixedSeedTerrain(t *testing.T) {
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := &application.Application{}
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	if err := prepareRainNoon(app); err != nil {
		t.Fatalf("准备 rain-noon: %v", err)
	}
	generator := worldgen.New(captureOakGroveSeed, config.Defaults().FluidEnabled)
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			position := core.ChunkPos{X: x, Z: z}
			want := generator.GenerateChunk(position)
			gotHash, gotRevision, loaded := app.Mirror().Hash(core.Overworld, position)
			if !loaded || gotRevision != 1 || gotHash != want.Hash() {
				t.Fatalf("chunk (%d,%d) hash/revision/loaded=(%x,%d,%v)，想要 (%x,1,true)",
					x, z, gotHash, gotRevision, loaded, want.Hash())
			}
		}
	}
	cameraCell := core.BlockPos{
		X: int32(math.Floor(float64(rainNoonCameraPos[0]))),
		Y: int32(math.Floor(float64(rainNoonCameraPos[1]))),
		Z: int32(math.Floor(float64(rainNoonCameraPos[2]))),
	}
	if block, loaded := app.Mirror().BlockAt(core.Overworld, cameraCell); !loaded || block != core.AirID {
		t.Fatalf("rain-noon 相机格 %+v loaded/block=%v/%d，想要 true/%d",
			cameraCell, loaded, block, core.AirID)
	}
}

// TestRainNoonWeatherChainFormsFollowLocalTemperature 钉住雨天三要素的 CPU 侧
// 输入：降水实例非空且晴天为空、同 tick 重放逐字节一致；场景钉分点正午
// （yearPhase=0、effPhase=6000），逐粒形态必须等于共享温度公式在该粒子世界
// 高度的判定——分点正午雨的局部温度为 11−4−1.25·(y−64)，雪形边界在
// y=64+7/1.25=69.6；机位 75.5、降水柱 (65.5,85.5] 跨越边界，低处雨丝与高处
// 雪点并存（温度边界由共享公式唯一决定，客户端逐粒子本地求值）。天空灰化
// 为正、露天亮度被压暗。
func TestRainNoonWeatherChainFormsFollowLocalTemperature(t *testing.T) {
	var encoder render.InstanceEncoder
	rainy := encoder.EncodeWeatherInstances(nil, rainNoonCameraPos, 0, captureRainNoonServerTick, core.WeatherRain, 0, 6000)
	if len(rainy) == 0 {
		t.Fatal("雨天降水实例为空：降水粒子没有进入呈现链路")
	}
	var clearEncoder render.InstanceEncoder
	if clear := clearEncoder.EncodeWeatherInstances(nil, rainNoonCameraPos, 0, captureRainNoonServerTick, core.WeatherClear, 0, 6000); len(clear) != 0 {
		t.Fatalf("晴天降水实例 = %d 字节，想要 0", len(clear))
	}
	var replayEncoder render.InstanceEncoder
	if again := replayEncoder.EncodeWeatherInstances(nil, rainNoonCameraPos, 0, captureRainNoonServerTick, core.WeatherRain, 0, 6000); !bytes.Equal(rainy, again) {
		t.Fatal("同 tick 降水重放不一致：粒子相位没有绑定固定 tick")
	}
	// 逐粒断言形态恰等于温度判定：实例布局与 render 侧 avatar 编码同源（96
	// 字节定长：0..64 列主序 mat4、64..80 四通道色），步长直接引用天气侧常量
	//（与 avatar 通道同源同值，不在别处复制该字面量）；纵向尺度取 Y 基向量
	// 长度（雨丝纵向 0.6 的细丝、雪点 0.09 立方体，判形口径与 render 侧单测
	// 同式）。
	const rainFilamentHeight = float32(0.6)
	stride := render.WeatherInstanceBytes
	if len(rainy)%stride != 0 {
		t.Fatalf("雨天降水流 = %d 字节，不是 %d 字节实例的整数倍", len(rainy), stride)
	}
	var snow, rain int
	for index := 0; index < len(rainy)/stride; index++ {
		base := index * stride
		f32 := func(offset int) float32 {
			return math.Float32frombits(binary.LittleEndian.Uint32(rainy[base+offset:]))
		}
		centerY := f32(52)
		wantSnow := core.PrecipitationIsSnow(0, 6000, core.WeatherRain, centerY)
		yScale := float32(math.Sqrt(float64(f32(16)*f32(16) + f32(20)*f32(20) + f32(24)*f32(24))))
		isSnow := math.Abs(float64(yScale-rainFilamentHeight)) >= 1e-6
		if isSnow != wantSnow {
			t.Fatalf("粒子 %d 高度 %v 的形态与温度判定不一致（wantSnow=%v）", index, centerY, wantSnow)
		}
		if wantSnow {
			snow++
		} else {
			rain++
		}
	}
	// 降水柱跨越分点正午的温度边界（69.6）：两种形态必须并存。
	if snow == 0 || rain == 0 {
		t.Fatalf("分点正午降水形态 snow=%d rain=%d，想要跨温度边界两种形态并存", snow, rain)
	}
	if gray := render.WeatherSkyGray(core.WeatherRain); gray <= 0 {
		t.Fatalf("雨天灰度 = %v，想要正值（灰化天空）", gray)
	}
	if got := render.ApplyWeatherDaylight(1, core.WeatherRain, captureRainNoonServerTick); got >= 1 {
		t.Fatalf("雨天正午亮度 = %v，想要被压暗（< 1）", got)
	}
}

// rainNoonMinWeatherDiffPixels 是雨天足迹的差分下限：同一几何的晴天对照里，
// 天空灰化与亮度压暗是整片天空像素的变化，远高于同机重复抓帧的个位数 LSB
// 漂移；下限取整千量级，误报只能来自天气本身。
const rainNoonMinWeatherDiffPixels = 1000

// TestRainNoonScenePixelsShowRainThroughFullChain 是雨天场景的完整链路像素
// 断言：经无窗口离屏链路（预热、装夹具、收敛、回读）抓帧，同一几何的晴天
// 对照（橡树林同一地形机位）与雨天帧必须有整片量级的差异；同一场景连抓
// 两次必须零漂移（固定 tick 确定性的落点）。渲染器是离屏设备，不创建也不
// 聚焦任何前台窗口；无 GPU 适配器时跳过。
func TestRainNoonScenePixelsShowRainThroughFullChain(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	if app.Window() != nil {
		t.Fatal("雨天像素断言必须走无窗口离屏链路，当前存在交互窗口")
	}
	rainScene := captureScene{
		Name: "rain-noon", WarmupFrames: 8,
		Prepare: prepareRainNoon, Apply: applyRainNoonCaptureState,
	}
	rainy, err := captureSceneImage(app, rainScene)
	if err != nil {
		t.Fatalf("抓取 rain-noon: %v", err)
	}
	clearApp := newCaptureSceneRenderApplication(t)
	clearScene := captureScene{
		Name: "oak-grove-clear-control", WarmupFrames: 8,
		Prepare: prepareOakGrove, Apply: applyOakGroveCaptureState,
	}
	clear, err := captureSceneImage(clearApp, clearScene)
	if err != nil {
		t.Fatalf("抓取晴天对照: %v", err)
	}
	diff, _, err := compareImages(rainy, clear)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("雨天足迹差异：%s", diff)
	if diff.DiffPixels < rainNoonMinWeatherDiffPixels || diff.MaxChannelDelta <= 2 {
		t.Fatalf("雨天足迹不足：%s（下限 %d 像素且最大通道差 > 2）", diff, rainNoonMinWeatherDiffPixels)
	}
	again, err := captureSceneImage(app, rainScene)
	if err != nil {
		t.Fatalf("重复抓取 rain-noon: %v", err)
	}
	if rediff, _, err := compareImages(rainy, again); err != nil {
		t.Fatal(err)
	} else if rediff.DiffPixels != 0 {
		t.Fatalf("同机连跑漂移：%s，想要零漂移（雨粒子相位必须绑定固定 tick）", rediff)
	}
}

//go:build darwin

package capture

// capture_snow_cover_test.go：冬季雪景场景的钉死回归。`prepareSnowCover` 复用
// 橡树林种子 42 的固定地形，在镜头前按 z 分区预铺 1..4 档雪层；
// `applySnowCoverCaptureState` 钉住冬中正午（SeasonWinter/128 + 昼弧 9454 的相位补偿，季节化相位
// 仍恰 6000）并注入雨天——冬季低地局部温度 ≤ 雪点，降水形态按共享温度公式
// 自验为全雪。本文件断言夹具数据面与 CPU 侧呈现输入（雪层档位、降水形态、
// 冷色 tint），golden 基线由 visual-check 承接；场景在 `captureScenes` 中紧随
// camera-third-front、先于 main-menu。

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
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestSnowCoverSceneEntryComplete 锁住场景条目的完整性与形态：与 rain-noon 同为
// 「Prepare 装夹具 + Apply 定状态 + 8 帧预热」的世界场景，不携带菜单/设置相位
// 与易变钉住。
func TestSnowCoverSceneEntryComplete(t *testing.T) {
	scene := captureSceneByName(t, "snow-cover")
	if scene.Prepare == nil || scene.Apply == nil {
		t.Fatalf("场景=%+v，想要完整 snow-cover", scene)
	}
	if scene.WarmupFrames != 8 {
		t.Fatalf("snow-cover WarmupFrames=%d，想要 8", scene.WarmupFrames)
	}
	if scene.Menu || scene.Settings != nil || scene.PinVolatile != nil {
		t.Fatalf("snow-cover 不应携带菜单/设置/易变钉住夹具: %+v", scene)
	}
}

// TestSnowCoverPrepareOverlaysFourTierSnowBands 钉住雪景夹具的数据面：
//  1. 每档（1..4）分区各有一块足量的雪层带（CPU 侧断言方块存在与档位；
//     视觉可辨由 golden 首录承担）；
//  2. 每格雪层都铺在白名单地表（草/土/石/沙/砾石/整块雪）正上方的空气位，
//     与积雪机制的判定序一致，不是悬空摆拍；
//  3. 夹具逐字节确定：两次装配的 3×3 区块内容哈希与 revision 逐项一致。
func TestSnowCoverPrepareOverlaysFourTierSnowBands(t *testing.T) {
	newSnowCoverMirror := func() *application.Application {
		t.Helper()
		mesher := client.NewMesher(assets.NewRegistry(), 1)
		t.Cleanup(mesher.Close)
		app := &application.Application{}
		app.SetMirror(client.NewMirror())
		app.SetMesher(mesher)
		if err := prepareSnowCover(app); err != nil {
			t.Fatalf("准备 snow-cover: %v", err)
		}
		return app
	}
	app := newSnowCoverMirror()

	// 逐分区断言：档位正确、数量足量（一条 4×27 的带扣掉树冠/短草/水面后仍
	// 剩大半）、支撑格全部是白名单地表。
	for _, band := range captureSnowCoverBands {
		layers := 0
		for z := band.minZ; z <= band.maxZ; z++ {
			for x := captureSnowCoverBandMinX; x <= captureSnowCoverBandMaxX; x++ {
				for y := int32(32); y <= 96; y++ {
					position := core.BlockPos{X: x, Y: y, Z: z}
					tier, isLayer := core.SnowLayerTier(mirrorBlockOrPanic(t, app, position))
					if !isLayer {
						continue
					}
					if tier != band.tier {
						t.Fatalf("分区 z∈[%d,%d] 内 (%d,%d,%d) 是 %d 档雪层，想要 %d 档",
							band.minZ, band.maxZ, x, y, z, tier, band.tier)
					}
					below := mirrorBlockOrPanic(t, app, core.BlockPos{X: x, Y: y - 1, Z: z})
					if !isSnowCoverSurface(below) {
						t.Fatalf("雪层 (%d,%d,%d) 下方是 %d，想要白名单地表", x, y, z, below)
					}
					layers++
				}
			}
		}
		if layers < 24 {
			t.Fatalf("分区 z∈[%d,%d] 的 %d 档雪层只有 %d 格，不足一块可辨区域（下限 24）",
				band.minZ, band.maxZ, band.tier, layers)
		}
		t.Logf("分区 z∈[%d,%d]：%d 档雪层 %d 格", band.minZ, band.maxZ, band.tier, layers)
	}

	// 四条带之外不得出现任何雪层：夹具的可见变化只有预铺雪层。
	for z := int32(-16); z <= 16; z++ {
		for x := int32(-16); x <= 16; x++ {
			if snowCoverBandOf(x, z) != 0 {
				continue
			}
			for y := int32(32); y <= 96; y++ {
				position := core.BlockPos{X: x, Y: y, Z: z}
				if tier, isLayer := core.SnowLayerTier(mirrorBlockOrPanic(t, app, position)); isLayer {
					t.Fatalf("分区外 (%d,%d,%d) 出现 %d 档雪层", x, y, z, tier)
				}
			}
		}
	}

	// 确定性：两次装配的 3×3 区块内容（哈希+revision）逐项一致。
	again := newSnowCoverMirror()
	for z := int32(-1); z <= 1; z++ {
		for x := int32(-1); x <= 1; x++ {
			position := core.ChunkPos{X: x, Z: z}
			gotHash, gotRevision, loaded := app.Mirror().Hash(core.Overworld, position)
			wantHash, wantRevision, wantLoaded := again.Mirror().Hash(core.Overworld, position)
			if !loaded || !wantLoaded || gotHash != wantHash || gotRevision != wantRevision {
				t.Fatalf("chunk (%d,%d) 两次装配 hash/revision=(%x,%d)/(%x,%d) loaded=%v/%v，想要逐字节一致",
					x, z, gotHash, gotRevision, wantHash, wantRevision, loaded, wantLoaded)
			}
		}
	}
	if got := app.Mesher().Stats().DirtySections; got == 0 {
		t.Fatal("雪景夹具装入后 mesher 没有 dirty section")
	}
}

// snowCoverBandOf 返回坐标 (x,z) 所属雪层带的档位（不在任何带内时为 0）：
// 按 `captureSnowCoverBands` 与 x 边界反查，与生产夹具共用同一份分区表。
func snowCoverBandOf(x, z int32) uint8 {
	if x < captureSnowCoverBandMinX || x > captureSnowCoverBandMaxX {
		return 0
	}
	for _, band := range captureSnowCoverBands {
		if z >= band.minZ && z <= band.maxZ {
			return band.tier
		}
	}
	return 0
}

// mirrorBlockOrPanic 读取镜像方块，未加载直接失败——夹具范围只覆盖已装配的
// 3×3 窗口，越窗即是夹具 bug。
func mirrorBlockOrPanic(t *testing.T, app *application.Application, position core.BlockPos) core.BlockID {
	t.Helper()
	block, loaded := app.Mirror().BlockAt(core.Overworld, position)
	if !loaded {
		t.Fatalf("方块 %+v 未加载：夹具越出了 3×3 窗口", position)
	}
	return block
}

// TestSnowCoverApplyPinsWinterNoonSnowFormRain 钉住 Apply 的全部呈现状态：
// 冬中正午钉（yearPhase 0.875、相位补偿 22727 后季节化相位恰 6000、昼夜状态
// 与分点正午同值）、冬季冷色 tint 生效、注入的雨天经预测器接受口径成为呈现
// 天气、海平面（画面中最暖的点）冬季雨温 ≤ 雪点即全画面雪形，前序场景留下的
// 共享呈现状态显式清空。
func TestSnowCoverApplyPinsWinterNoonSnowFormRain(t *testing.T) {
	app := newSnowCoverTestApplication(t)
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

	if err := applySnowCoverCaptureState(app); err != nil {
		t.Fatalf("应用 snow-cover: %v", err)
	}
	if app.WorldTimeTicks() != 6000 {
		t.Fatalf("world time = %d，想要 6000（冻结正午）", app.WorldTimeTicks())
	}
	if got := app.Camera().Pos; got != rainNoonCameraPos || app.Camera().Yaw != 0 || app.Camera().Pitch != -0.38 {
		t.Fatalf("camera = %+v yaw=%v pitch=%v，想要 %v/0/-0.38（与 rain-noon 同机位风格）",
			got, app.Camera().Yaw, app.Camera().Pitch, rainNoonCameraPos)
	}
	// 冬中钉：SeasonWinter/128 重建 yearPhase=0.875（冬季 [0.75,1) 的中点）。
	if got := app.YearPhase(); got != 0.875 {
		t.Fatalf("snow-cover 的 yearPhase = %v，想要 0.875（冬中钉）", got)
	}
	// 相位补偿后季节化相位仍恰 6000（正午）：昼弧 9454 下 p=4727 → 6000。
	if got := app.DayPhaseOffset(); got != captureWinterNoonDayPhaseOffset {
		t.Fatalf("显示相位偏移 = %d，想要钉死的 %d", got, captureWinterNoonDayPhaseOffset)
	}
	eff := core.EffectiveDayPhase(6000, captureWinterNoonDayPhaseOffset, core.DayArcTicks(0.875))
	if eff != 6000 {
		t.Fatalf("相位补偿后的季节化相位 = %d，想要 6000（正午）", eff)
	}
	// 正午的太阳与亮度与分点基线同值（昼夜曲线只消费 effPhase）；天空 clear
	// 色相对分点正午偏冷（红端降、蓝端升）——冬季冷色 tint 的 CPU 侧证据。
	winterNoon := render.DayNightAt(6000, captureWinterNoonDayPhaseOffset, 0.875)
	equinoxNoon := render.DayNightAt(6000, 0)
	if winterNoon.Sun != equinoxNoon.Sun || winterNoon.Daylight != equinoxNoon.Daylight {
		t.Fatalf("冬中正午 Sun/Daylight = %v/%v，想要与分点正午一致 %v/%v",
			winterNoon.Sun, winterNoon.Daylight, equinoxNoon.Sun, equinoxNoon.Daylight)
	}
	if winterNoon.ClearColor[0] >= equinoxNoon.ClearColor[0] ||
		winterNoon.ClearColor[2] <= equinoxNoon.ClearColor[2] {
		t.Fatalf("冬中天空 clear 色 = %v，想要相对分点 %v 红端降蓝端升（冷色 tint）",
			winterNoon.ClearColor, equinoxNoon.ClearColor)
	}
	// 形态自验的核心数值：冬中雨在海平面（画面中最暖的高度，更高只会更冷）
	// 的局部温度必须 ≤ 雪点 0℃——整条降水柱因此全为雪形。
	seaLevel := core.TemperatureAt(0.875, 6000, core.WeatherRain, core.TemperatureSeaLevelY)
	if seaLevel > core.TemperatureSnowPoint {
		t.Fatalf("冬中海平面雨温 = %v℃，想要 ≤ %v（雪形）", seaLevel, core.TemperatureSnowPoint)
	}
	if !core.PrecipitationIsSnow(0.875, 6000, core.WeatherRain, core.TemperatureSeaLevelY) {
		t.Fatal("冬中海平面降水形态应为雪（温度判定与谓词不一致）")
	}
	if app.ServerTick() != captureSnowCoverServerTick {
		t.Fatalf("server tick = %d，想要钉死的 %d（雪粒子相位只由固定 tick 决定）",
			app.ServerTick(), captureSnowCoverServerTick)
	}
	if app.Weather() != core.WeatherRain {
		t.Fatalf("呈现天气 = %d，想要雨天 %d（形态由温度派生为雪）", app.Weather(), core.WeatherRain)
	}
	if weather, ready := app.Predictor().Weather(); !ready || weather != core.WeatherRain {
		t.Fatalf("预测器天气 = %d/ready=%v，想要雨天 %d/true（注入必须经预测器接受口径）",
			weather, ready, core.WeatherRain)
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

// newSnowCoverTestApplication 构造带空镜像与已就绪预测器的最小呈现应用：预测器
// 预置 rain-noon 注入过的高位 tick（同一次抓帧运行内 snow-cover 排在其后，注入
// tick 必须仍单调前进才被接受），晴天开始，Apply 的雨天注入必须经和解口径推成
// 雨天。
func newSnowCoverTestApplication(t *testing.T) *application.Application {
	t.Helper()
	app := application.NewPresentationApplicationForTest()
	app.SetMirror(client.NewMirror())
	predictor := client.NewPredictor()
	if err := predictor.Begin(network.PlayerState{
		ServerTick: captureRainNoonServerTick, Dimension: core.Overworld,
		Position: rainNoonCameraPos, Yaw: 0, Pitch: -0.38, Ready: true,
		Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks, Hunger: core.MaxHunger,
		WorldTimeTicks: 6000, WeatherKind: core.WeatherClear,
	}); err != nil {
		t.Fatalf("预置预测器: %v", err)
	}
	app.SetPredictor(predictor)
	return app
}

// TestSnowCoverWeatherChainFormsSnowAtAllHeights 钉住雪景降水的 CPU 侧输入：
// 冬中正午雨天在机位处的降水实例非空且同 tick 重放逐字节一致；逐粒形态必须
// 全为雪（冬中雨温在整条降水柱高度上都 ≤ 雪点——没有任何 0.6 长的雨丝细线）；
// 天空灰化为正、露天亮度被压暗（与 rain-noon 同一降水链路）。
func TestSnowCoverWeatherChainFormsSnowAtAllHeights(t *testing.T) {
	var encoder render.InstanceEncoder
	snowy := encoder.EncodeWeatherInstances(nil, rainNoonCameraPos, 0, captureSnowCoverServerTick, core.WeatherRain, 0.875, 6000)
	if len(snowy) == 0 {
		t.Fatal("雪天降水实例为空：降水粒子没有进入呈现链路")
	}
	var replayEncoder render.InstanceEncoder
	if again := replayEncoder.EncodeWeatherInstances(nil, rainNoonCameraPos, 0, captureSnowCoverServerTick, core.WeatherRain, 0.875, 6000); !bytes.Equal(snowy, again) {
		t.Fatal("同 tick 降水重放不一致：粒子相位没有绑定固定 tick")
	}
	const rainFilamentHeight = float32(0.6)
	stride := render.WeatherInstanceBytes
	if len(snowy)%stride != 0 {
		t.Fatalf("雪天降水流 = %d 字节，不是 %d 字节实例的整数倍", len(snowy), stride)
	}
	for index := 0; index < len(snowy)/stride; index++ {
		base := index * stride
		f32 := func(offset int) float32 {
			return math.Float32frombits(binary.LittleEndian.Uint32(snowy[base+offset:]))
		}
		centerY := f32(52)
		yScale := float32(math.Sqrt(float64(f32(16)*f32(16) + f32(20)*f32(20) + f32(24)*f32(24))))
		if math.Abs(float64(yScale-rainFilamentHeight)) < 1e-6 {
			t.Fatalf("粒子 %d 高度 %v 仍是雨丝细线：冬中降水柱必须全为雪形", index, centerY)
		}
		if !core.PrecipitationIsSnow(0.875, 6000, core.WeatherRain, centerY) {
			t.Fatalf("粒子 %d 高度 %v 的温度判定不是雪：形态与温度公式不一致", index, centerY)
		}
	}
	if gray := render.WeatherSkyGray(core.WeatherRain); gray <= 0 {
		t.Fatalf("雨天灰度 = %v，想要正值（灰化天空）", gray)
	}
	if got := render.ApplyWeatherDaylight(1, core.WeatherRain, captureSnowCoverServerTick); got >= 1 {
		t.Fatalf("雪天正午亮度 = %v，想要被压暗（< 1）", got)
	}
}

// snowCoverMinSceneDiffPixels 是雪景足迹的差分下限：与 rain-noon 的雨天足迹同
// 量级（预铺雪层 + 全雪降水 + 冷色天空都是整片像素的变化），下限取整千量级，
// 误报只能来自场景本身。
const snowCoverMinSceneDiffPixels = 1000

// TestSnowCoverScenePixelsShowSnowThroughFullChain 是雪景的完整链路像素断言：
// 经无窗口离屏链路抓帧，同一地形的晴天分点对照（橡树林）与雪景帧必须有整片
// 量级的差异；同一场景连抓两次必须零漂移（固定 tick 与确定性夹具的落点）。
// 渲染器是离屏设备，不创建也不聚焦任何前台窗口；无 GPU 适配器时跳过。
func TestSnowCoverScenePixelsShowSnowThroughFullChain(t *testing.T) {
	app := newCaptureSceneRenderApplication(t)
	if app.Window() != nil {
		t.Fatal("雪景像素断言必须走无窗口离屏链路，当前存在交互窗口")
	}
	snowScene := captureSceneByName(t, "snow-cover")
	snowy, err := captureSceneImage(app, snowScene)
	if err != nil {
		t.Fatalf("抓取 snow-cover: %v", err)
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
	diff, _, err := compareImages(snowy, clear)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("雪景足迹差异：%s", diff)
	if diff.DiffPixels < snowCoverMinSceneDiffPixels || diff.MaxChannelDelta <= 2 {
		t.Fatalf("雪景足迹不足：%s（下限 %d 像素且最大通道差 > 2）", diff, snowCoverMinSceneDiffPixels)
	}
	again, err := captureSceneImage(app, snowScene)
	if err != nil {
		t.Fatalf("重复抓取 snow-cover: %v", err)
	}
	if rediff, _, err := compareImages(snowy, again); err != nil {
		t.Fatal(err)
	} else if rediff.DiffPixels != 0 {
		t.Fatalf("同机连跑漂移：%s，想要零漂移（雪粒子相位必须绑定固定 tick）", rediff)
	}
}

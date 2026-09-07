//go:build darwin

package capture

// motion_weather_cycle_test.go：晴雨雷暴全过程 motion 演示的钉死回归。
// 时间线（96 帧、延迟 13cs 即约 8fps）：F0–23 晴、F24–47 雨、F48–71 雷暴、
// F72–95 回晴；合成 tick 逐帧 +8（压缩推进，不按真实分钟时长），雷暴段内
// 必含闪光帧。链路正确性（降水编码、天空灰化、压暗）由雨天场景与 render
// 包单测承接，这里只钉时间线落点、注入语义与产物约定。

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestWeatherCycleMotionCaptures96FramesInOrder 钉住演示抓帧的帧约定：96 帧、
// 帧号 0→95 顺序递进、同一固定序列两次抓取逐帧字节一致。
func TestWeatherCycleMotionCaptures96FramesInOrder(t *testing.T) {
	capture := func(record *[]int) func(int) (*image.NRGBA, error) {
		return func(frame int) (*image.NRGBA, error) {
			*record = append(*record, frame)
			img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			img.Pix[0] = uint8(frame)
			return img, nil
		}
	}
	var firstFrames []int
	first, err := captureWeatherCycleMotionFrames(capture(&firstFrames))
	if err != nil {
		t.Fatalf("抓取 motion 帧序列: %v", err)
	}
	if len(first) != weatherCycleMotionFrameCount {
		t.Fatalf("motion 帧数=%d，想要 %d", len(first), weatherCycleMotionFrameCount)
	}
	if weatherCycleMotionFrameCount != 96 {
		t.Fatalf("motion 帧数常量=%d，想要 96（8fps 乘 12 秒）", weatherCycleMotionFrameCount)
	}
	for index, frame := range firstFrames {
		if frame != index {
			t.Fatalf("第 %d 次抓取帧号=%d，想要 %d", index, frame, index)
		}
	}
	var secondFrames []int
	second, err := captureWeatherCycleMotionFrames(capture(&secondFrames))
	if err != nil {
		t.Fatalf("重抓 motion 帧序列: %v", err)
	}
	for index := range first {
		if !bytes.Equal(first[index].Pix, second[index].Pix) {
			t.Fatalf("第 %d 帧两次抓取字节不一致", index)
		}
	}
}

// TestWeatherCycleMotionFrameBudget 钉住帧预算：96 帧恰为四段均分、落在录制
// 循环预算内、延迟约 8fps（12 秒循环约 12.5 秒）。
func TestWeatherCycleMotionFrameBudget(t *testing.T) {
	if weatherCycleMotionFrameCount <= 0 || weatherCycleMotionFrameCount > motionMaxFrames {
		t.Fatalf("天气演示帧数=%d，想要落在录制循环预算内（1..%d）",
			weatherCycleMotionFrameCount, motionMaxFrames)
	}
	if weatherCycleMotionFrameCount%4 != 0 {
		t.Fatalf("帧数=%d，想要被四天气段整除", weatherCycleMotionFrameCount)
	}
	if weatherCycleMotionFrameDelay != 13 {
		t.Fatalf("帧延迟=%dcs，想要 13cs（约 8fps，与采掘演示同口径）", weatherCycleMotionFrameDelay)
	}
}

// TestWeatherCycleMotionTickIsCompressed 钉住压缩 tick：逐帧固定步长递增、
// 基点复用雨天场景的单调安全大 tick、尾帧仍排在水下场景注入 tick 之前。
func TestWeatherCycleMotionTickIsCompressed(t *testing.T) {
	if weatherCycleMotionTickBase != captureRainNoonServerTick {
		t.Fatalf("tick 基点=%d，想要复用雨天场景的 %d（大于收敛期任何真实 tick）",
			weatherCycleMotionTickBase, captureRainNoonServerTick)
	}
	for _, frame := range []int{0, 1, 47, 95} {
		want := weatherCycleMotionTickBase + uint64(frame)*weatherCycleMotionTickStride
		if got := weatherCycleMotionTick(frame); got != want {
			t.Fatalf("第 %d 帧 tick=%d，想要 %d（逐帧固定步长）", frame, got, want)
		}
	}
	if last := weatherCycleMotionTick(weatherCycleMotionFrameCount - 1); last >= uint64(1)<<20 {
		t.Fatalf("尾帧 tick=%d，已触及水下场景的注入 tick（场景间单调顺序被破坏）", last)
	}
}

// TestWeatherCycleMotionCoversFullCycle 钉住全过程覆盖：晴→雨→雷暴→回晴四段
// 边界、灰度递增（晴零、雷暴最灰）、雷暴段内必含闪光帧。
func TestWeatherCycleMotionCoversFullCycle(t *testing.T) {
	boundaries := []struct {
		frame int
		want  core.WeatherKind
	}{
		{0, core.WeatherClear}, {23, core.WeatherClear},
		{24, core.WeatherRain}, {47, core.WeatherRain},
		{48, core.WeatherThunder}, {71, core.WeatherThunder},
		{72, core.WeatherClear}, {95, core.WeatherClear},
	}
	for _, point := range boundaries {
		if got := weatherCycleMotionWeather(point.frame); got != point.want {
			t.Fatalf("第 %d 帧天气=%d，想要 %d", point.frame, got, point.want)
		}
	}
	clear, rain, thunder := render.WeatherSkyGray(core.WeatherClear),
		render.WeatherSkyGray(core.WeatherRain), render.WeatherSkyGray(core.WeatherThunder)
	if clear != 0 || !(rain > 0) || !(thunder > rain) {
		t.Fatalf("天空灰度 晴=%v 雨=%v 雷暴=%v，想要 0 < 雨 < 雷暴", clear, rain, thunder)
	}
	flashes := 0
	for frame := 0; frame < weatherCycleMotionFrameCount; frame++ {
		if weatherCycleMotionWeather(frame) == core.WeatherThunder &&
			render.ThunderFlashLift(weatherCycleMotionTick(frame)) > 0 {
			flashes++
		}
	}
	if flashes == 0 {
		t.Fatal("雷暴段内零闪光帧：tick 基点与步长的模 80 配合被破坏")
	}
}

// TestWeatherCycleMotionFrameAppliesWeatherThroughPredictor 钉住逐帧注入语义：
// 每帧天气经预测器接受口径（值域与单调 tick 校验），呈现侧与预测侧一致、
// 粒子相位 tick 同步钉死；乱序或回退的 tick 会被预测器静默忽略，本函数按
// 接受失败报错，不产出天气与 tick 脱节的假帧。
func TestWeatherCycleMotionFrameAppliesWeatherThroughPredictor(t *testing.T) {
	app := newRainNoonTestApplication(t)
	for _, frame := range []int{0, 24, 48, 72, 95} {
		if err := applyWeatherCycleFrame(app, frame); err != nil {
			t.Fatalf("推进第 %d 帧: %v", frame, err)
		}
		want := weatherCycleMotionWeather(frame)
		if app.Weather() != want {
			t.Fatalf("第 %d 帧呈现天气=%d，想要 %d", frame, app.Weather(), want)
		}
		if weather, ready := app.Predictor().Weather(); !ready || weather != want {
			t.Fatalf("第 %d 帧预测器天气=%d/ready=%v，想要 %d/true", frame, weather, ready, want)
		}
		if app.ServerTick() != weatherCycleMotionTick(frame) {
			t.Fatalf("第 %d 帧 tick=%d，想要 %d", frame, app.ServerTick(), weatherCycleMotionTick(frame))
		}
	}
}

// weatherCycleVariedFrames 构造确定性夹具：96 帧逐帧变色（多色相跨帧），
// 让共享自适应调色板走多盒切分路径——单色输入会坍缩成单盒子，盖不住调色板
// 路径的非确定性。
func weatherCycleVariedFrames() []*image.NRGBA {
	frames := make([]*image.NRGBA, 0, weatherCycleMotionFrameCount)
	for index := range weatherCycleMotionFrameCount {
		img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
		fill := color.NRGBA{R: uint8(index * 2), G: 120, B: 60, A: 255}
		for y := 0; y < 3; y++ {
			for x := 0; x < 4; x++ {
				img.SetNRGBA(x, y, fill)
			}
		}
		frames = append(frames, img)
	}
	return frames
}

// TestWeatherCycleMotionGIFDecodesTo96Frames 钉住编码产物可解码且帧数符合约定。
func TestWeatherCycleMotionGIFDecodesTo96Frames(t *testing.T) {
	frames := weatherCycleVariedFrames()
	data, err := encodeWeatherCycleMotionGIF(frames)
	if err != nil {
		t.Fatalf("编码 motion GIF: %v", err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码 motion GIF: %v", err)
	}
	if len(decoded.Image) != weatherCycleMotionFrameCount {
		t.Fatalf("解码帧数=%d，想要 %d", len(decoded.Image), weatherCycleMotionFrameCount)
	}
}

// TestWeatherCycleMotionGIFEncodingIsDeterministic 钉住固定输入→固定字节：
// 同一份多色相 96 帧两次编码逐字节一致（连跑两次验证的落点）。
func TestWeatherCycleMotionGIFEncodingIsDeterministic(t *testing.T) {
	frames := weatherCycleVariedFrames()
	first, err := encodeWeatherCycleMotionGIF(frames)
	if err != nil {
		t.Fatalf("首次编码 motion GIF: %v", err)
	}
	second, err := encodeWeatherCycleMotionGIF(frames)
	if err != nil {
		t.Fatalf("重编码 motion GIF: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("同一 96 帧两次编码字节不一致（%d vs %d 字节）", len(first), len(second))
	}
}

// TestWeatherCycleMotionGIFRefusesEmptyFrames 钉住空输入直接失败：
// 坏帧留在产物里只会制造假证据。
func TestWeatherCycleMotionGIFRefusesEmptyFrames(t *testing.T) {
	if _, err := encodeWeatherCycleMotionGIF(nil); err == nil {
		t.Fatal("空帧编码 motion GIF 想要报错，实际通过")
	}
}

// TestWeatherCycleMotionSceneStaysOutOfCaptureScenes 钉住演示场景不进正式表：
// 世界 PNG 纪律与 `visual-check` 比对都不感知 motion 演示。
func TestWeatherCycleMotionSceneStaysOutOfCaptureScenes(t *testing.T) {
	for _, scene := range captureScenes {
		lowered := strings.ToLower(scene.Name)
		if strings.Contains(lowered, "weather-cycle") {
			t.Fatalf("正式场景表混入演示场景 %q", scene.Name)
		}
	}
	if weatherCycleMotionScene.Name != "weather-cycle-motion" {
		t.Fatalf("演示收敛场景名=%q，想要 weather-cycle-motion", weatherCycleMotionScene.Name)
	}
}

// TestWeatherCycleMotionScenePinsSummerNoonWithCompensation 钉住演示机位的
// 季节钉（与 rain-noon 场景同一钉法，裁决同语义）：夏至钉 + 相位补偿后
// yearPhase=0.25、季节化相位仍恰 6000——昼夜曲线与无季节基线逐值一致，入库
// GIF 逐字节不变；夏至正午的温度边界 y=84.8 只把降水柱顶 84.8..85.5 的少量
// 粒子判为雪尘，它们位于该俯视机位视野之外（rain-noon 同机位基线恒等实证），
// 入画降水段保持纯雨形。
func TestWeatherCycleMotionScenePinsSummerNoonWithCompensation(t *testing.T) {
	app := newRainNoonTestApplication(t)
	if err := applyWeatherCycleMotionCaptureState(app); err != nil {
		t.Fatalf("应用演示收敛场景: %v", err)
	}
	if got := app.YearPhase(); got != 0.25 {
		t.Fatalf("演示机位 yearPhase = %v，想要 0.25（夏至钉）", got)
	}
	offset := app.DayPhaseOffset()
	if offset != captureSummerNoonDayPhaseOffset {
		t.Fatalf("演示机位显示相位偏移 = %d，想要补偿值 %d", offset, captureSummerNoonDayPhaseOffset)
	}
	if got := core.EffectiveDayPhase(6000, offset, core.DayArcTicks(0.25)); got != 6000 {
		t.Fatalf("补偿后的季节化相位 = %d，想要 6000（正午）", got)
	}
	if got, want := render.DayNightAt(6000, offset, 0.25), render.DayNightAt(6000, 0); got != want {
		t.Fatalf("演示机位昼夜状态 = %+v，想要与无季节基线一致 %+v", got, want)
	}
}

// TestRunWeatherCycleMotionValidatesInputs 钉住演示入口的前置校验：空路径与
// 空应用在收敛世界之前失败，不产出半截文件。
func TestRunWeatherCycleMotionValidatesInputs(t *testing.T) {
	if err := RunWeatherCycleMotion(nil, ""); err == nil {
		t.Fatal("空路径想要报错，实际通过")
	}
	if err := RunWeatherCycleMotion(nil, "weather-cycle.gif"); err == nil {
		t.Fatal("空应用想要报错，实际通过")
	}
}

// TestRunMotionRoutesWeatherCycle 钉住主 dispatch 接到新入口：未知剧本仍拒绝。
func TestRunMotionRoutesWeatherCycle(t *testing.T) {
	if err := RunMotion(nil, "", "weather-cycle"); err == nil {
		t.Fatal("weather-cycle 空路径想要报错（已路由到入口校验），实际通过")
	}
	if err := RunMotion(nil, "", "unknown-scene"); err == nil {
		t.Fatal("未知 motion 剧本想要报错，实际通过")
	}
}

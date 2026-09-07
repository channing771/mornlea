package capture

// motion_weather_cycle.go：晴雨雷暴全过程的 motion 演示入口。产物是 96 帧
// GIF，只验呈现、不进任何比对门禁：收敛场景值不追加进 `captureScenes`
// （世界 PNG 纪律独立），`RunCapture`/`visual-check`/`visual-update` 都不
// 感知它。
//
// 时间线（帧号定天气段、延迟 13cs 即约 8fps，96 帧循环约 12.5 秒）：
// F0–23 晴、F24–47 雨、F48–71 雷暴、F72–95 回晴。天气段按压缩 tick 推进
// （逐帧固定步长，不按真实分钟时长录制）；粒子位置是（序号，权威 tick）
// 的纯函数，步长越大帧间位移越明显，雷暴段内必含闪光帧（基点与步长的模
// 80 配合由回归测试钉住）。
//
// 演示驱动的是真实 `RenderFrame` 天气链路（降水实例 + 天空灰化 + 昼夜压暗
// 与雷暴闪光），只是天气与 tick 来源是合成注入而非真实无头消息：真实权威
// 天气取决于服务端播了什么，随录制时机漂移（见 `captureScene` 的注释），
// 演示必须逐帧确定才钉得住四段约定。注入复用雨天场景的 headless 固定天气
// 口径（预测器值域与单调 tick 校验），链路本身已有雨天场景的逐帧编码测试
// 与完整链路像素测试覆盖，这里不管链路、只管呈现。

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// weatherCycleMotionFrameCount 是演示 GIF 的固定帧数：晴雨雷暴回晴四段
	// 各 24 帧，不超过 8fps 乘 12 秒的帧预算。
	weatherCycleMotionFrameCount = 96
	// weatherCycleMotionFrameDelay 是 GIF 单帧延迟（百分之一秒）：约 8fps，
	// 与采掘演示同口径，96 帧循环约 12.5 秒。
	weatherCycleMotionFrameDelay = 13
	// weatherCycleMotionTickBase 是合成 tick 序列的起点：复用雨天场景钉死的
	// 大 tick（大于收敛期见过的任何真实 tick，预测器单调校验不静默忽略注入），
	// 且模闪光周期余 48——配合下述步长，雷暴段恰好落进点亮窗。
	weatherCycleMotionTickBase = captureRainNoonServerTick
	// weatherCycleMotionTickStride 是合成 tick 每帧的压缩步长：降水下落速度
	// 约 0.55 格/tick，步长 8 即帧间约 4.4 格位移，粒子起落肉眼可辨；同时
	// 步长 8 与闪光周期 80 的配合让 24 帧雷暴段必含闪光帧。
	weatherCycleMotionTickStride = uint64(8)
	// weatherCycleMotionRainStart / ThunderStart / ClearAgainStart 是三段
	// 切换帧：之前一段的末帧仍属旧天气，切换帧起属新天气。
	weatherCycleMotionRainStart       = 24
	weatherCycleMotionThunderStart    = 48
	weatherCycleMotionClearAgainStart = 72
)

// weatherCycleMotionScene 是演示的收敛场景值：仅本文件内部使用，绝不追加进
// `captureScenes`。世界夹具与机位复用雨天场景（橡树林种子 42 的固定地形与
// 雪线下机位，整列降水只能选雨形），呈现状态钉死沿用橡树林（固定正午 +
// 同机位 + 晴天基线），动态只来自逐帧的天气与 tick 注入。
var weatherCycleMotionScene = captureScene{
	Name:         "weather-cycle-motion",
	WarmupFrames: 8,
	Prepare:      prepareRainNoon,
	Apply:        applyOakGroveCaptureState,
}

// weatherCycleMotionTick 把帧号映射为合成 tick：逐帧固定步长压缩推进。
func weatherCycleMotionTick(frame int) uint64 {
	return weatherCycleMotionTickBase + uint64(frame)*weatherCycleMotionTickStride
}

// weatherCycleMotionWeather 给出指定帧的天气段：晴→雨→雷暴→回晴。
func weatherCycleMotionWeather(frame int) core.WeatherKind {
	switch {
	case frame < weatherCycleMotionRainStart:
		return core.WeatherClear
	case frame < weatherCycleMotionThunderStart:
		return core.WeatherRain
	case frame < weatherCycleMotionClearAgainStart:
		return core.WeatherThunder
	default:
		return core.WeatherClear
	}
}

// applyWeatherCycleFrame 推进一帧时间线状态：该帧的天气经抓帧路径注入固定
// 天气（预测器接受口径 + 呈现侧直写），粒子相位 tick 同步钉死。调用方随后
// 走真实 `RenderFrame` 抓帧。
func applyWeatherCycleFrame(app SceneApplication, frame int) error {
	return injectFixedWeather(app, weatherCycleMotionTick(frame), weatherCycleMotionWeather(frame))
}

// captureWeatherCycleFrame 是演示时间线单帧的生产抓帧缝：天气与 tick 注入 →
// 真实 `RenderFrame` + 回读。`RunWeatherCycleMotion` 与回归测试共用它，测试
// 抓到的即产物抓到的。
func captureWeatherCycleFrame(app SceneApplication, frame int) (*image.NRGBA, error) {
	if err := applyWeatherCycleFrame(app, frame); err != nil {
		return nil, err
	}
	if _, err := app.RenderFrame(captureDrainMax); err != nil {
		return nil, err
	}
	return bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight), nil
}

// `captureWeatherCycleMotionFrames` 以帧号 0→95 连抓固定帧数：抓帧回调由调用方
// 注入（生产走 `captureWeatherCycleFrame`，测试走合成帧），
// 本函数只钉住帧数与帧序，是演示确定性的落点。超预算请求在首帧捕获前拒绝。
func captureWeatherCycleMotionFrames(
	capture func(frame int) (*image.NRGBA, error),
) ([]*image.NRGBA, error) {
	return captureBoundedMotionFrames(weatherCycleMotionFrameCount, capture)
}

// `encodeWeatherCycleMotionGIF` 保留天气演示的原始延迟，复用全片确定性
// 自适应调色板与无抖色编码；空输入明确失败。
func encodeWeatherCycleMotionGIF(frames []*image.NRGBA) ([]byte, error) {
	return encodeMotionGIF(frames, weatherCycleMotionFrameDelay)
}

// RunWeatherCycleMotion 是 motion 演示的独立入口：收敛世界 → 96 帧时间线连抓
// （合成 tick + 逐帧天气注入）→ 标准库编码写盘。只写传入的输出路径那一个
// 文件，不碰 `captureScenes` 与任何 PNG 基线。
func RunWeatherCycleMotion(app SceneApplication, outPath string) error {
	if outPath == "" {
		return fmt.Errorf("motion 演示输出路径为空")
	}
	if app == nil {
		return fmt.Errorf("motion 演示缺少应用实例")
	}
	if err := prepareCaptureApplication(app); err != nil {
		return err
	}
	// 昼夜冻结与正式抓帧同一理由：收敛帧期间到达的权威时间会改写钉死的正午，
	// 最终 96 帧的天空光因此随进程启动漂移。
	app.SetWorldTimeFrozen(true)
	defer app.SetWorldTimeFrozen(false)
	// 收敛帧先行（产出帧丢弃）：网格化与上传收敛后，96 帧循环里的画面只随
	// 合成 tick 与注入天气变化，不混入渐进加载像素。
	if _, err := captureSceneImage(app, weatherCycleMotionScene); err != nil {
		return fmt.Errorf("收敛 motion 场景: %w", err)
	}
	frames, err := captureWeatherCycleMotionFrames(func(frame int) (*image.NRGBA, error) {
		return captureWeatherCycleFrame(app, frame)
	})
	if err != nil {
		return err
	}
	data, err := encodeWeatherCycleMotionGIF(frames)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("创建 motion 输出目录 %s: %w", filepath.Dir(outPath), err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return fmt.Errorf("写出 motion GIF %s: %w", outPath, err)
	}
	fmt.Printf("已生成 motion 演示 %s（%d 帧，%d 字节）\n", outPath, len(frames), len(data))
	return nil
}

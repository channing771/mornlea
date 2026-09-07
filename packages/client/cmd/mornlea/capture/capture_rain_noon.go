package capture

import (
	"fmt"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// captureRainNoonServerTick 是雨天场景钉死的权威 tick：降水位置是（序号，
// tick）的纯函数，相位只由它决定。它必须大于加载期间见过的任何真实 tick
// （否则预测器的单调校验会静默忽略注入，画面回到晴天），同时小于
// water-underwater 的注入 tick（`1 << 20`），排在后面的末场景仍能单调前进。
const captureRainNoonServerTick = uint64(1) << 19

// captureSummerNoonDayPhaseOffset 是「夏至正午钉」的显示相位补偿，rain-noon
// 场景与 weather-cycle motion 演示共用：钉夏至（yearPhase=0.25、昼弧 15600）
// 后，worldTime%24000=6000 时的线性相位为 (6000+1800)%24000=7800，warp 后
// effPhase = 7800·12000/15600 = 6000——正午天空与日照和分点基线逐字节一致
// （spec weather-camera-showcase）。
const captureSummerNoonDayPhaseOffset = 1800

// pinCaptureSummerNoon 把呈现侧季节输入钉在「夏至正午 + 相位补偿」：镜像季
// 节钉 SeasonSummer/0（温度边界抬到 y=84.8，低地降水主体保持雨形），显示相
// 位按当季昼弧补偿，使 worldTime%24000=6000 下 warp 后的季节化相位仍恰
// 6000。调用方必须已把世界时间钉在 6000（`applyOakGroveCaptureState` 的钉
// 值）；恒等自验按同一对钉值复算，补偿失效（昼弧或世界时间被改动）时当场
// 失败，不产出错位天空。rain-noon 场景与 weather-cycle motion 演示共用本
// 钉法，补偿值只对调用场景生效——下一场景由抓帧管线的
// `pinCaptureSeasonBaseline` 复位。
func pinCaptureSummerNoon(app SceneApplication) error {
	if err := app.SetCaptureSeason(core.SeasonSummer, 0); err != nil {
		return err
	}
	if err := app.SetCaptureDayPhaseOffset(captureSummerNoonDayPhaseOffset); err != nil {
		return err
	}
	effPhase := core.EffectiveDayPhase(6000, captureSummerNoonDayPhaseOffset, core.DayArcTicks(0.25))
	if effPhase != 6000 {
		return fmt.Errorf("夏至相位补偿后季节化相位 = %d，想要 6000（正午）", effPhase)
	}
	return nil
}

// prepareRainNoon 装入雨天场景的固定地形：复用橡树林种子 42 的 3×3 生成
// 区块。选址理由见 `applyRainNoonCaptureState`：该机位下降水柱主体落在夏至
// 正午的温度边界（y=84.8）之下、柱顶少量高出，画面主体为雨、柱顶带温度
// 梯度真实表现的少量雪尘，且地形已验证有树。
func prepareRainNoon(app SceneApplication) error {
	return prepareOakGrove(app)
}

// applyRainNoonCaptureState 钉死雨天场景的全部呈现状态：夏至正午（季节钉
// Summer/0 + 显示相位补偿 1800，季节化相位仍恰 6000）、固定机位与共享清场
// 复用橡树林（同一地形同一机位，天空与日照和既有基线逐字节一致，画面差异
// 只来自天气与温度化的降水形态），再经抓帧路径注入固定雨天。场景已进
// `captureScenes`（紧随 mining-crack-heavy），注入的雨天由后继场景的公共
// 清场复位为晴天，季节钉由抓帧管线的分点默认锚复位。
func applyRainNoonCaptureState(app SceneApplication) error {
	if err := applyOakGroveCaptureState(app); err != nil {
		return err
	}
	// 夏至正午钉 + 相位补偿：抓帧管线已在 Apply 前把季节钉回分点，这里改钉
	// 夏至并按昼弧补偿显示相位（与 weather-cycle motion 演示共用
	// `pinCaptureSummerNoon`，含恒等自验）。
	if err := pinCaptureSummerNoon(app); err != nil {
		return fmt.Errorf("钉住 rain-noon 夏至正午: %w", err)
	}
	return injectRainNoonWeather(app)
}

// injectFixedWeather 经抓帧路径注入固定天气：权威天气夹具走预测器已有的
// 接受口径（值域与单调 tick 校验），呈现侧天气取预测器接受后的值，粒子相
// 位 tick 同步钉死。不碰游戏内权威——调用点只在场景 `Apply` 与 motion 演示
// 的逐帧推进（收敛帧与演示循环都不 drain 真实消息，夹具不会被覆盖）。
// tick 必须大于预测器已见过的任何真实 tick，否则单调校验静默忽略注入。
func injectFixedWeather(app SceneApplication, tick uint64, kind core.WeatherKind) error {
	predictor := app.Predictor()
	if predictor == nil {
		return fmt.Errorf("固定天气注入需要预测器，当前为 nil")
	}
	if app.Mirror() == nil {
		return fmt.Errorf("固定天气注入需要世界镜像，当前为 nil")
	}
	camera := *app.Camera()
	state := network.PlayerState{
		ServerTick:     tick,
		Dimension:      core.Overworld,
		Position:       camera.Pos,
		Yaw:            camera.Yaw,
		Pitch:          camera.Pitch,
		Ready:          true,
		Health:         core.MaxHealth,
		Oxygen:         core.MaxOxygenTicks,
		Hunger:         core.MaxHunger,
		WorldTimeTicks: 6000,
		WeatherKind:    kind,
	}
	if _, err := predictor.ApplyPlayerState(state, client.MirrorCollisionSource{
		Mirror:    app.Mirror(),
		Dimension: core.Overworld,
	}); err != nil {
		return fmt.Errorf("注入固定权威天气: %w", err)
	}
	weather, ready := predictor.Weather()
	if !ready || weather != kind {
		return fmt.Errorf("固定天气注入后预测器天气=%d/ready=%v，想要 %d/true",
			weather, ready, kind)
	}
	app.SetServerTick(tick)
	return app.SetCaptureWeather(weather)
}

// injectRainNoonWeather 经抓帧路径注入固定雨天：调用点只在场景 `Apply`
// （收敛帧不再 drain，夹具不会被真实消息覆盖）。
func injectRainNoonWeather(app SceneApplication) error {
	return injectFixedWeather(app, captureRainNoonServerTick, core.WeatherRain)
}

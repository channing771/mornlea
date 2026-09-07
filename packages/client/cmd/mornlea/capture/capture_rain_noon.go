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

// prepareRainNoon 装入雨天场景的固定地形：复用橡树林种子 42 的 3×3 生成
// 区块。选址理由见 `applyRainNoonCaptureState`：该机位在雪线以下且地形已
// 验证有树，整列降水只能选雨形。
func prepareRainNoon(app SceneApplication) error {
	return prepareOakGrove(app)
}

// applyRainNoonCaptureState 钉死雨天场景的全部呈现状态：正午、雪线下机位与
// 共享清场复用橡树林（同一地形同一机位，画面差异只来自天气），再经抓帧
// 路径注入固定雨天。场景已进 `captureScenes`（紧随 mining-crack-heavy），
// 注入的雨天由后继场景的公共清场复位为晴天。
func applyRainNoonCaptureState(app SceneApplication) error {
	if err := applyOakGroveCaptureState(app); err != nil {
		return err
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

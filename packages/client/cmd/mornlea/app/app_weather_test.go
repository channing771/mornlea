//go:build darwin

package app

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestApplicationWeatherFollowsNewestAuthoritativeState 锁定天气呈现输入的
// 客户端消费链：天气随权威 `PlayerState` 一起被接受（与世界时间同样的「只认
// 更新 tick」纪律），呈现侧降水/天空/亮度只消费该字段，不读本地随机或墙钟。
func TestApplicationWeatherFollowsNewestAuthoritativeState(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	newest := network.PlayerState{
		ServerTick: 2, WorldTimeTicks: 6000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		WeatherKind: core.WeatherRain,
	}
	sendInteractiveServerMessage(t, serverEndpoint, newest)
	app.DrainServerMessages(1)
	if app.weather != core.WeatherRain {
		t.Fatalf("接受新状态后的天气 = %d，想要 %d", app.weather, core.WeatherRain)
	}

	for _, stale := range []network.PlayerState{
		{ServerTick: 1, WorldTimeTicks: 6000, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true, WeatherKind: core.WeatherThunder},
		{ServerTick: 2, WorldTimeTicks: 6000, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true, WeatherKind: core.WeatherClear},
	} {
		sendInteractiveServerMessage(t, serverEndpoint, stale)
		app.DrainServerMessages(1)
		if app.weather != core.WeatherRain {
			t.Fatalf("旧或重复状态将天气改为 %d，想要 %d", app.weather, core.WeatherRain)
		}
	}
}

// TestApplicationWeatherFrozenWithWorldTime 天气与世界时间同一冻结开关：
// capture 钉住天空状态时，后续权威状态不得覆盖已钉住的天气呈现输入。
func TestApplicationWeatherFrozenWithWorldTime(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	base := network.PlayerState{
		ServerTick: 2, WorldTimeTicks: 6000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		WeatherKind: core.WeatherRain,
	}
	sendInteractiveServerMessage(t, serverEndpoint, base)
	app.DrainServerMessages(1)
	app.SetWorldTimeFrozen(true)
	newer := network.PlayerState{
		ServerTick: 3, WorldTimeTicks: 6100, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		WeatherKind: core.WeatherThunder,
	}
	sendInteractiveServerMessage(t, serverEndpoint, newer)
	app.DrainServerMessages(1)
	if app.weather != core.WeatherRain {
		t.Fatalf("冻结期间天气 = %d，想要钉住的 %d", app.weather, core.WeatherRain)
	}
	if app.serverTick != newer.ServerTick {
		t.Fatalf("冻结期间权威 tick = %d，想要 %d（冻结只拦呈现量）", app.serverTick, newer.ServerTick)
	}
}

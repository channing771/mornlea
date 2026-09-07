//go:build darwin

package app

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestApplicationSeasonMirrorFollowsNewestAuthoritativeState 锁定季节三字段
// （协议 v37 起随玩家状态同步）在 app 侧的持有与接受链：与世界时间、天气
// 同一「只认更新 tick」纪律，旧或重复状态不得回退已确认的镜像值。
func TestApplicationSeasonMirrorFollowsNewestAuthoritativeState(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	newest := network.PlayerState{
		ServerTick: 2, WorldTimeTicks: 6000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Season: core.SeasonWinter, SeasonProgress: 200, Temperature: -7,
	}
	sendInteractiveServerMessage(t, serverEndpoint, newest)
	app.DrainServerMessages(1)
	if app.season != core.SeasonWinter || app.seasonProgress != 200 || app.temperature != -7 {
		t.Fatalf("接受新状态后的季节镜像 = (%d,%d,%d)，想要 (%d,%d,%d)",
			app.season, app.seasonProgress, app.temperature,
			core.SeasonWinter, 200, -7)
	}

	for _, stale := range []network.PlayerState{
		{ServerTick: 1, WorldTimeTicks: 6000, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true, Season: core.SeasonSummer, SeasonProgress: 10, Temperature: 30},
		{ServerTick: 2, WorldTimeTicks: 6000, Dimension: core.Overworld, Position: newest.Position, OnGround: true, Ready: true, Season: core.SeasonSpring, SeasonProgress: 0, Temperature: 0},
	} {
		sendInteractiveServerMessage(t, serverEndpoint, stale)
		app.DrainServerMessages(1)
		if app.season != core.SeasonWinter || app.seasonProgress != 200 || app.temperature != -7 {
			t.Fatalf("旧或重复状态将季节镜像改为 (%d,%d,%d)，想要 (%d,%d,%d)",
				app.season, app.seasonProgress, app.temperature,
				core.SeasonWinter, 200, -7)
		}
	}
}

// TestApplicationSeasonMirrorFrozenWithWorldTime 季节三字段与世界时间、天气
// 同一冻结开关：capture 钉住天空状态时，后续权威状态不得覆盖已钉住的季节
// 呈现输入（权威 tick 照常前进，冻结只拦呈现量）。
func TestApplicationSeasonMirrorFrozenWithWorldTime(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	base := network.PlayerState{
		ServerTick: 2, WorldTimeTicks: 6000, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Season: core.SeasonWinter, SeasonProgress: 200, Temperature: -7,
	}
	sendInteractiveServerMessage(t, serverEndpoint, base)
	app.DrainServerMessages(1)
	app.SetWorldTimeFrozen(true)
	newer := network.PlayerState{
		ServerTick: 3, WorldTimeTicks: 6100, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		Season: core.SeasonSummer, SeasonProgress: 80, Temperature: 28,
	}
	sendInteractiveServerMessage(t, serverEndpoint, newer)
	app.DrainServerMessages(1)
	if app.season != core.SeasonWinter || app.seasonProgress != 200 || app.temperature != -7 {
		t.Fatalf("冻结期间季节镜像 = (%d,%d,%d)，想要钉住的 (%d,%d,%d)",
			app.season, app.seasonProgress, app.temperature,
			core.SeasonWinter, 200, -7)
	}
	if app.serverTick != newer.ServerTick {
		t.Fatalf("冻结期间权威 tick = %d，想要 %d（冻结只拦呈现量）", app.serverTick, newer.ServerTick)
	}
}

package server

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestPlayerStatePublishesWeatherKind 锁定天气的下发链路：sim 在
// `PlayerUpdate` 中给出的天气必须原样进入发给玩家本人的 `PlayerState`。
// 天气取非零的雨——零值与「字段根本没搬运」不可分辨。天气是世界单值，
// 所有会话收到的值一致。
func TestPlayerStatePublishesWeatherKind(t *testing.T) {
	h := newRemotePublicationHarness(t, 1, 2)
	h.markSnapshotSent(1, core.ChunkPos{})
	h.markSnapshotSent(2, core.ChunkPos{})
	first := h.playerUpdate(1, true, core.Overworld, mgl32.Vec3{0.5, 2, 0.5})
	first.WeatherKind = core.WeatherRain
	second := h.playerUpdate(2, true, core.Overworld, mgl32.Vec3{0.75, 2, 0.5})
	second.WeatherKind = core.WeatherRain

	h.publish(contract.TickResult{Tick: 5, Players: []contract.PlayerUpdate{first, second}})

	for _, id := range []contract.SessionID{1, 2} {
		states := 0
		for _, message := range h.drain(id) {
			state, ok := message.(network.PlayerState)
			if !ok {
				continue
			}
			states++
			if state.WeatherKind != core.WeatherRain {
				t.Fatalf("会话 %d 的 PlayerState.WeatherKind = %d，想要雨 %d",
					id, state.WeatherKind, core.WeatherRain)
			}
		}
		if states != 1 {
			t.Fatalf("会话 %d 收到 %d 条 PlayerState，想要 1", id, states)
		}
	}
}

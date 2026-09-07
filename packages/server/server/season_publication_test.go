package server

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestPlayerStatePublishesSeasonFields 锁定季节三字段的下发链路：sim 在
// `PlayerUpdate` 中给出的季节、季内进度与温度必须原样进入发给玩家本人的
// `PlayerState`。取值全部避开零值——零值与「字段根本没搬运」不可分辨；
// 进度取 200、温度取负值 −12（int8 符号位路径），季节取冬（3）。
func TestPlayerStatePublishesSeasonFields(t *testing.T) {
	h := newRemotePublicationHarness(t, 1, 2)
	h.markSnapshotSent(1, core.ChunkPos{})
	h.markSnapshotSent(2, core.ChunkPos{})
	first := h.playerUpdate(1, true, core.Overworld, mgl32.Vec3{0.5, 2, 0.5})
	first.Season = core.SeasonWinter
	first.SeasonProgress = 200
	first.Temperature = -12
	second := h.playerUpdate(2, true, core.Overworld, mgl32.Vec3{0.75, 2, 0.5})
	second.Season = core.SeasonWinter
	second.SeasonProgress = 200
	second.Temperature = -12

	h.publish(contract.TickResult{Tick: 5, Players: []contract.PlayerUpdate{first, second}})

	for _, id := range []contract.SessionID{1, 2} {
		states := 0
		for _, message := range h.drain(id) {
			state, ok := message.(network.PlayerState)
			if !ok {
				continue
			}
			states++
			if state.Season != core.SeasonWinter {
				t.Fatalf("会话 %d 的 PlayerState.Season = %d，想要冬 %d",
					id, state.Season, core.SeasonWinter)
			}
			if state.SeasonProgress != 200 {
				t.Fatalf("会话 %d 的 PlayerState.SeasonProgress = %d，想要 200",
					id, state.SeasonProgress)
			}
			if state.Temperature != -12 {
				t.Fatalf("会话 %d 的 PlayerState.Temperature = %d，想要 -12",
					id, state.Temperature)
			}
		}
		if states != 1 {
			t.Fatalf("会话 %d 收到 %d 条 PlayerState，想要 1", id, states)
		}
	}
}

package server

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// TestPlayerStatePublishesDayPhaseOffset 锁定显示相位偏移的下发链路：sim 在
// `PlayerUpdate` 中给出的偏移必须原样进入发给玩家本人的 `PlayerState`。偏移取
// 非零非上界的 12399——零值与「字段根本没搬运」不可分辨，上界又容易与越界
// 拒绝路径巧合重合。偏移是世界单值，所有会话收到的值一致。
func TestPlayerStatePublishesDayPhaseOffset(t *testing.T) {
	h := newRemotePublicationHarness(t, 1, 2)
	h.markSnapshotSent(1, core.ChunkPos{})
	h.markSnapshotSent(2, core.ChunkPos{})
	first := h.playerUpdate(1, true, core.Overworld, mgl32.Vec3{0.5, 2, 0.5})
	first.DayPhaseOffset = 12399
	second := h.playerUpdate(2, true, core.Overworld, mgl32.Vec3{0.75, 2, 0.5})
	second.DayPhaseOffset = 12399

	h.publish(contract.TickResult{Tick: 5, Players: []contract.PlayerUpdate{first, second}})

	for _, id := range []contract.SessionID{1, 2} {
		states := 0
		for _, message := range h.drain(id) {
			state, ok := message.(network.PlayerState)
			if !ok {
				continue
			}
			states++
			if state.DayPhaseOffset != 12399 {
				t.Fatalf("会话 %d 的 PlayerState.DayPhaseOffset = %d，想要 12399", id, state.DayPhaseOffset)
			}
		}
		if states != 1 {
			t.Fatalf("会话 %d 收到 %d 条 PlayerState，想要 1", id, states)
		}
	}
}

package sim

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件是 internal/sim 的共享测试 helper 中心（见 AGENTS.md「测试文件编排」）：
// 只装被多个测试文件引用的纯 helper，不得含任何测试函数。

func readyMeleePlayers(t *testing.T, count int) (*Engine, []SessionID) {
	t.Helper()
	engine, _ := readyMovementPlayer(t)
	for id := count; id >= 2; id-- {
		engine.RegisterSession(SessionID(id), core.Overworld, core.ChunkPos{})
	}
	if count > 1 {
		engine.Step()
	}
	sessions := make([]SessionID, count)
	for index := range count {
		sessions[index] = SessionID(index + 1)
	}
	return engine, sessions
}

func setMeleePlayer(engine *Engine, id SessionID, position mgl32.Vec3, yaw float32) {
	player := engine.sessions[id].player
	player.state.Position = position
	player.yaw = yaw
	player.pitch = 0
	player.health = core.MaxHealth
	player.reset = false
}

// farmlandMoistureCandidateWatch 记录湿度阶段开始前是否观察到指定候选。
type farmlandMoistureCandidateWatch struct {
	phaseSeen     bool
	candidateSeen bool
}

// watchFarmlandMoistureCandidateAtPhase 在 `advanceFarmlandMoisture` 消费队列前，
// 记录 `key` 是否曾经位于去重集合中。
func watchFarmlandMoistureCandidateAtPhase(
	engine *Engine,
	key farmlandMoistureKey,
) *farmlandMoistureCandidateWatch {
	watch := &farmlandMoistureCandidateWatch{}
	engine.stepPhaseObserver = func(phase stepPhase) {
		if phase != phaseFarmlandMoistureAdvance {
			return
		}
		watch.phaseSeen = true
		if _, queued := engine.farmlandMoisture.queued[key]; queued {
			watch.candidateSeen = true
		}
	}
	return watch
}

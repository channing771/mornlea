package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

func readyFarmlandMoistureState(t *testing.T, positions ...core.ChunkPos) *State {
	t.Helper()
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	for _, position := range positions {
		chunk := world.NewChunk(position)
		chunk.Compact()
		if !dimension.BeginGeneration(position) {
			t.Fatalf("区块 %+v 未开始生成", position)
		}
		if err := dimension.ApplyGenerated(position, chunk); err != nil {
			t.Fatal(err)
		}
	}
	state.environment.scope = make(map[core.ChunkKey]struct{}, len(positions))
	state.environment.scopeNext = make(map[core.ChunkKey]struct{}, len(positions))
	for _, position := range positions {
		state.environment.scope[core.ChunkKey{Dimension: core.Overworld, Pos: position}] = struct{}{}
	}
	return state
}

func setFarmlandMoistureTestBlock(t *testing.T, state *State, position core.BlockPos, block core.BlockID) {
	t.Helper()
	old, changed, err := state.Dimension(core.Overworld).SetBlock(position, block)
	if err != nil || !changed {
		t.Fatalf("设置方块 old=%d changed=%v err=%v，想要成功", old, changed, err)
	}
}

func advanceFarmlandMoistureTest(state *State, active []core.ChunkKey, tick uint64) *Mutation {
	mutation := state.NewMutation()
	state.AdvanceFarmlandMoisture(
		active,
		state.NewEnvironmentMutation(mutation, tick, EnvironmentConfig{}),
	)
	return mutation
}

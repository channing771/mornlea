package realm

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

func TestFarmlandMoistureFluidMembershipChanges(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	farmland := core.BlockPos{X: 8, Y: 1, Z: 8}
	water := core.BlockPos{X: 12, Y: 1, Z: 8}
	x, _, z := farmland.Local()
	chunk.SetBlock(x, farmland.Y, z, core.FarmlandDryID)
	chunk.Compact()
	if !dimension.BeginGeneration(chunk.Pos) {
		t.Fatal("中心区块未开始生成")
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		t.Fatal(err)
	}

	config := EnvironmentConfig{FluidFlowDelayTicks: 1}
	mutation := state.NewMutation()
	environment := state.NewEnvironmentMutation(mutation, 0, config)
	if _, changed, err := environment.SetBlock(core.Overworld, water, core.WaterSourceID); err != nil || !changed {
		t.Fatalf("放水 changed=%v err=%v，想要成功", changed, err)
	}
	state.AdvanceFarmlandMoisture([]core.ChunkKey{{Dimension: core.Overworld}}, environment)
	if got, _ := dimension.BlockAt(farmland); got != core.FarmlandWetID {
		t.Fatalf("同一 Mutation 放水后耕地=%d，想要湿耕地 %d", got, core.FarmlandWetID)
	}
	if mutation.Len() != 1 {
		t.Fatalf("环境写入登记了 %d 个区块，想要共享一个 Mutation", mutation.Len())
	}

	mutation = state.NewMutation()
	environment = state.NewEnvironmentMutation(mutation, 1, config)
	if _, changed, err := environment.SetBlock(core.Overworld, water, core.AirID); err != nil || !changed {
		t.Fatalf("移除水 changed=%v err=%v，想要成功", changed, err)
	}
	state.AdvanceFarmlandMoisture([]core.ChunkKey{{Dimension: core.Overworld}}, environment)
	if got, _ := dimension.BlockAt(farmland); got != core.FarmlandDryID {
		t.Fatalf("同一 Mutation 失水后耕地=%d，想要干耕地 %d", got, core.FarmlandDryID)
	}
}

func TestFarmlandMoistureRescansNewlyActiveChunk(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	farmland := core.BlockPos{X: 8, Y: 1, Z: 8}
	water := core.BlockPos{X: 12, Y: 1, Z: 8}
	for _, entry := range []struct {
		position core.BlockPos
		block    core.BlockID
	}{
		{farmland, core.FarmlandDryID},
		{water, core.WaterSourceID},
	} {
		x, _, z := entry.position.Local()
		chunk.SetBlock(x, entry.position.Y, z, entry.block)
	}
	chunk.Compact()
	if !dimension.BeginGeneration(chunk.Pos) {
		t.Fatal("中心区块未开始生成")
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		t.Fatal(err)
	}

	active := []core.ChunkKey{{Dimension: core.Overworld}}
	for tick := uint64(0); tick < 5; tick++ {
		mutation := state.NewMutation()
		environment := state.NewEnvironmentMutation(mutation, tick, EnvironmentConfig{})
		state.AdvanceFarmlandMoisture(active, environment)
	}
	if got, _ := dimension.BlockAt(farmland); got != core.FarmlandWetID {
		t.Fatalf("重扫后耕地=%d，想要湿耕地 %d", got, core.FarmlandWetID)
	}
}

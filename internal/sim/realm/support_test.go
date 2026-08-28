package realm

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

func TestSupportCandidatesFollowChangedBlocks(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	chunk := world.NewChunk(core.ChunkPos{})
	torchSupport := core.BlockPos{X: 2, Y: 1, Z: 2}
	bedSupport := core.BlockPos{X: 6, Y: 1, Z: 2}
	for _, entry := range []struct {
		position core.BlockPos
		block    core.BlockID
	}{
		{position: torchSupport, block: core.StoneID},
		{position: core.BlockPos{X: 2, Y: 2, Z: 2}, block: core.TorchStandingID},
		{position: bedSupport, block: core.StoneID},
		{position: core.BlockPos{X: 6, Y: 2, Z: 2}, block: core.BedFootID(0)},
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

	mutation := state.NewMutation()
	for _, position := range []core.BlockPos{torchSupport, bedSupport} {
		if _, changed, err := dimension.SetBlock(position, core.AirID); err != nil || !changed {
			t.Fatalf("移除支撑 changed=%v err=%v，想要成功", changed, err)
		}
		mutation.Record(core.Overworld, position, core.AirID)
	}

	if got := state.TorchSupportCandidates(mutation); len(got) != 1 || got[0].Support != torchSupport {
		t.Fatalf("火把候选=%+v，想要支撑 %+v", got, torchSupport)
	}
	if got := state.BedSupportCandidates(mutation); len(got) != 1 || got[0].Support != bedSupport {
		t.Fatalf("床候选=%+v，想要支撑 %+v", got, bedSupport)
	}
}

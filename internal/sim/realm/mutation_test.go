package realm

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

func TestMutationCommitOrdersChunksAndBlocks(t *testing.T) {
	state := NewState(core.Overworld, 1)
	overworld := state.Dimension(core.Overworld)
	other := state.Dimension(1)
	for _, dimension := range []*Dimension{overworld, other} {
		for _, pos := range []core.ChunkPos{{}, {X: 1}} {
			if !dimension.BeginGeneration(pos) {
				t.Fatalf("BeginGeneration(%+v) = false", pos)
			}
			if err := dimension.ApplyGenerated(pos, world.NewChunk(pos)); err != nil {
				t.Fatal(err)
			}
		}
	}

	low := core.BlockPos{X: 1, Y: 2, Z: 3}
	high := core.BlockPos{X: 2, Y: 2, Z: 3}
	nextChunk := core.BlockPos{X: core.SectionSize, Y: 2, Z: 3}
	otherPos := core.BlockPos{X: 1, Y: 2, Z: 3}
	for _, change := range []struct {
		dimension *Dimension
		position  core.BlockPos
		block     core.BlockID
	}{
		{overworld, high, core.StoneID},
		{other, otherPos, core.DirtID},
		{overworld, nextChunk, core.GrassID},
		{overworld, low, core.DirtID},
	} {
		if _, changed, err := change.dimension.SetBlock(change.position, change.block); err != nil || !changed {
			t.Fatalf("SetBlock(%+v, %d) = (_, %v, %v)", change.position, change.block, changed, err)
		}
	}

	mutation := state.NewMutation()
	mutation.Record(core.Overworld, high, core.StoneID)
	mutation.Record(1, otherPos, core.DirtID)
	mutation.Record(core.Overworld, nextChunk, core.GrassID)
	mutation.Record(core.Overworld, low, core.DirtID)
	batches := mutation.Commit()

	if len(batches) != 3 {
		t.Fatalf("batch count = %d, want 3", len(batches))
	}
	if got := batches[0]; got.Dimension != core.Overworld || got.Chunk != (core.ChunkPos{}) || got.BaseRevision != 1 || got.NewRevision != 2 || len(got.Changes) != 2 || got.Changes[0].Position != low || got.Changes[1].Position != high {
		t.Fatalf("first batch = %+v", got)
	}
	if got := batches[1]; got.Dimension != core.Overworld || got.Chunk != (core.ChunkPos{X: 1}) || got.BaseRevision != 1 || got.NewRevision != 2 || len(got.Changes) != 1 || got.Changes[0].Position != nextChunk {
		t.Fatalf("second batch = %+v", got)
	}
	if got := batches[2]; got.Dimension != 1 || got.Chunk != (core.ChunkPos{}) || got.BaseRevision != 1 || got.NewRevision != 2 || len(got.Changes) != 1 || got.Changes[0].Position != otherPos {
		t.Fatalf("third batch = %+v", got)
	}
}

func TestMutationCommitOnlyOnce(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	position := core.BlockPos{Y: 2}
	if !dimension.BeginGeneration(position.Chunk()) {
		t.Fatal("BeginGeneration() = false")
	}
	if err := dimension.ApplyGenerated(position.Chunk(), world.NewChunk(position.Chunk())); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := dimension.SetBlock(position, core.StoneID); err != nil || !changed {
		t.Fatalf("SetBlock() = (_, %v, %v)", changed, err)
	}

	mutation := state.NewMutation()
	mutation.Record(core.Overworld, position, core.StoneID)
	if batches := mutation.Commit(); len(batches) != 1 {
		t.Fatalf("first Commit() batch count = %d, want 1", len(batches))
	}
	if batches := mutation.Commit(); len(batches) != 0 {
		t.Fatalf("second Commit() batch count = %d, want 0", len(batches))
	}
	info, ok := dimension.Info(position.Chunk())
	if !ok || info.Revision != 2 {
		t.Fatalf("revision after second Commit() = (%d, %v), want (2, true)", info.Revision, ok)
	}
}

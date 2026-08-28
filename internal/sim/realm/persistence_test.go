package realm

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

func TestInFlightCleanChunkIsRetainedOnUnload(t *testing.T) {
	state := NewState(core.Overworld)
	dimension := state.Dimension(core.Overworld)
	pos := core.ChunkPos{Z: -9}
	chunk := world.NewChunk(pos)
	if !dimension.BeginLoading(pos) {
		t.Fatal("load not started")
	}
	if err := dimension.ApplyLoaded(pos, chunk, 7, 7, false, false); err != nil {
		t.Fatal(err)
	}
	dimension.records[pos].SaveInFlightRevision = 7
	if dimension.RequestUnload(pos) {
		t.Fatal("in-flight clean chunk was discarded")
	}
	record := dimension.records[pos]
	if record.State != ChunkUnloading || record.Chunk != chunk || !record.UnloadRequested {
		t.Fatalf("in-flight clean chunk was discarded: %+v", record)
	}
}

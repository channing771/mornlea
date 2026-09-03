package persistence

import (
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestPersistenceBackpressureHysteresisBoundary(t *testing.T) {
	if !nextPersistenceBackpressure(false, 100, 100) {
		t.Fatal("estimated bytes at cap did not enter backpressure")
	}
	if !nextPersistenceBackpressure(true, 90, 100) {
		t.Fatal("estimated bytes at 90 percent cleared backpressure")
	}
	if nextPersistenceBackpressure(true, 89, 100) {
		t.Fatal("estimated bytes below 90 percent did not clear backpressure")
	}
	if nextPersistenceBackpressure(true, 90, 101) {
		t.Fatal("integer bytes below an exact 90 percent fraction did not clear backpressure")
	}
	if !nextPersistenceBackpressure(true, 91, 101) {
		t.Fatal("integer bytes above an exact 90 percent fraction cleared backpressure")
	}
}

func TestPersistenceStatusReturnsCopiedCurrentState(t *testing.T) {
	running := newPersistenceServer(t, newPersistenceTestStore())
	running.engine = dirtyReadyEngine(t, []core.ChunkKey{chunkKey(0, 0)})
	running.world.mu.Lock()
	running.world.engine = running.engine
	running.world.lastSaveError = "original failure"
	running.world.lastSaveErrorAt = time.Unix(123, 0)
	running.world.backpressured = true
	running.world.mu.Unlock()

	status := running.PersistenceStatus()
	if status.DirtyChunks != 1 || status.EstimatedBytes != emptyChunkEstimateBytes ||
		status.InFlightChunks != 0 || !status.Backpressured ||
		status.LastError != "original failure" ||
		!status.LastErrorAt.Equal(time.Unix(123, 0)) || status.AutosaveDrained {
		t.Fatalf("persistence status=%+v", status)
	}
	status.LastError = "caller mutation"
	status.LastErrorAt = time.Time{}
	got := running.PersistenceStatus()
	if got.LastError != "original failure" || !got.LastErrorAt.Equal(time.Unix(123, 0)) {
		t.Fatalf("caller mutated server status: %+v", got)
	}
}

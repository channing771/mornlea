package server

import (
	"reflect"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
)

func TestPersistenceBackpressureQueuesAcquireUntilMemoryRecovers(t *testing.T) {
	store := &blockingLoadStore{
		metadata: storage.Metadata{FormatVersion: 3, Seed: 42},
		started:  make(chan core.ChunkKey, 1),
	}
	_, endpoint := network.NewMemoryPair(64)
	config := DefaultConfig(42)
	config.ViewRadius = 0
	config.Workers = 1
	config.SaveWorkers = 1
	config.TrustedObserver = true
	config.UnsavedBytes = 512
	running := newAttachedWorldForTest(config, endpoint, &countingGenerator{}, store)
	t.Cleanup(func() { shutdownServerForTest(t, running) })
	heldKey := chunkKey(0, 0)
	engine := dirtyReadyEngine(t, []core.ChunkKey{heldKey})
	running.config.AutosaveTicks = engine.TickCount() + 100
	setPersistenceEngineForTest(t, running, engine)
	running.engine.RegisterObserverSession(trustedObserverSessionID)
	running.engine.Enqueue(contract.Command{
		Session: trustedObserverSessionID, Sequence: 1,
		Kind:      contract.CommandTrustedObserverCenter,
		Dimension: heldKey.Dimension, Center: heldKey.Pos,
	})
	running.engine.Step()
	running.trustedObserverSequence = 1
	target := core.ChunkPos{X: 20, Z: -20}
	if err := running.SetTrustedObserverCenter(core.Overworld, target); err != nil {
		t.Fatal(err)
	}

	result := running.StepForTest()
	wantAcquire := core.ChunkKey{Dimension: core.Overworld, Pos: target}
	if !reflect.DeepEqual(result.Acquire, []core.ChunkKey{wantAcquire}) {
		t.Fatalf("Acquire=%+v, want queued %+v", result.Acquire, wantAcquire)
	}
	select {
	case started := <-store.started:
		t.Fatalf("backpressured acquisition dispatched %+v", started)
	default:
	}
	info, exists := running.engine.ChunkInfo(wantAcquire)
	if !exists || info.State != contract.ChunkLoading || len(running.pending) != 1 {
		t.Fatalf("queued unknown chunk state=%+v exists=%v pending=%+v", info, exists, running.pending)
	}
	status := running.PersistenceStatus()
	if !status.Backpressured || status.DirtyChunks != 1 || status.EstimatedBytes != emptyChunkEstimateBytes ||
		status.InFlightChunks != 0 || status.AutosaveDrained {
		t.Fatalf("backpressure status=%+v", status)
	}

	selected := running.engine.PersistenceSnapshots(1, 1<<20, contract.SaveAll)
	if len(selected) != 1 || selected[0].Key != heldKey {
		t.Fatalf("cleanup snapshot=%+v, want held key", selected)
	}
	running.engine.ApplyPersisted([]contract.PersistedChunk{{Key: heldKey, Revision: selected[0].Revision}})
	running.StepForTest()
	select {
	case started := <-store.started:
		if started != wantAcquire {
			t.Fatalf("resumed load=%+v, want %+v", started, wantAcquire)
		}
	case <-time.After(waitDeadline):
		t.Fatal("acquisition did not resume below hysteresis threshold")
	}
	if resumed := running.PersistenceStatus(); resumed.Backpressured || !resumed.AutosaveDrained {
		t.Fatalf("resumed status=%+v", resumed)
	}
}

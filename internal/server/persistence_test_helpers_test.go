package server

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/server/persistence"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/storage"
	"github.com/channing771/mornlea/internal/world"
)

const emptyChunkEstimateBytes = 512 + core.DropsPerChunk*world.DropSlotBytes +
	core.FurnacesPerChunk*world.FurnaceSlotBytes + core.ChestsPerChunk*world.ChestSlotBytes

func dirtyReadyEngine(t *testing.T, keys []core.ChunkKey) *sim.Engine {
	t.Helper()
	engine := sim.NewEngine(0, 0, 0)
	for index, key := range keys {
		session := sim.SessionID(index + 1)
		engine.RegisterObserverSession(session)
		engine.Enqueue(sim.Command{
			Session: session, Sequence: 1,
			Kind: sim.CommandTrustedObserverCenter, Dimension: key.Dimension, Center: key.Pos,
		})
		requested := engine.Step()
		if !reflect.DeepEqual(requested.Acquire, []core.ChunkKey{key}) {
			t.Fatalf("Acquire=%+v, want %+v", requested.Acquire, []core.ChunkKey{key})
		}
		engine.SubmitAcquired(sim.AcquiredChunk{Key: key, Missing: true})
		generated := engine.Step()
		if !reflect.DeepEqual(generated.Generate, []core.ChunkKey{key}) {
			t.Fatalf("Generate=%+v, want %+v", generated.Generate, []core.ChunkKey{key})
		}
		engine.SubmitGenerated(sim.GeneratedChunk{
			Dimension: key.Dimension,
			Pos:       key.Pos,
			Chunk:     world.NewChunk(key.Pos),
		})
		ready := engine.Step()
		if !reflect.DeepEqual(ready.Ready, []core.ChunkKey{key}) {
			t.Fatalf("Ready=%+v, want %+v", ready.Ready, []core.ChunkKey{key})
		}
	}
	return engine
}

func dirtyPlayerEngine(t *testing.T, key core.ChunkKey) *sim.Engine {
	t.Helper()
	engine := sim.NewEngine(0, 0, 0)
	engine.RegisterSession(testSessionID, key.Dimension, key.Pos)
	requested := engine.Step()
	if !reflect.DeepEqual(requested.Acquire, []core.ChunkKey{key}) {
		t.Fatalf("Acquire=%+v, want %+v", requested.Acquire, []core.ChunkKey{key})
	}
	engine.SubmitAcquired(sim.AcquiredChunk{Key: key, Missing: true})
	generated := engine.Step()
	if !reflect.DeepEqual(generated.Generate, []core.ChunkKey{key}) {
		t.Fatalf("Generate=%+v, want %+v", generated.Generate, []core.ChunkKey{key})
	}
	engine.SubmitGenerated(sim.GeneratedChunk{
		Dimension: key.Dimension,
		Pos:       key.Pos,
		Chunk:     (&gatedGenerator{flat: true}).chunk(key.Pos),
	})
	ready := engine.Step()
	if len(ready.Ready) != 1 || len(ready.Players) != 1 || !ready.Players[0].Ready {
		t.Fatalf("ready tick=%+v, want one ready chunk and player", ready)
	}
	return engine
}

func committedResult(saves []storage.ChunkSave) storage.SaveResult {
	committed := make(map[core.ChunkKey]uint64, len(saves))
	for _, save := range saves {
		committed[save.Key] = save.Revision
	}
	return storage.SaveResult{Committed: committed}
}

func chunkKey(x, z int32) core.ChunkKey {
	return core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: x, Z: z}}
}

func containsChunkKey(keys []core.ChunkKey, want core.ChunkKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func resetWorldPersistenceForTest(t *testing.T, running *Server) {
	t.Helper()
	running.world.Close()
	running.world = persistence.NewWorld(
		running.store,
		running.engine,
		persistenceOptions(running.config, &running.stepMu),
	)
}

func setPersistenceEngineForTest(t *testing.T, running *Server, engine *sim.Engine) {
	t.Helper()
	running.engine = engine
	resetWorldPersistenceForTest(t, running)
}

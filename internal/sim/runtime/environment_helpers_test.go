package runtime

import (
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

const farmlandWetRadius = 4

func blockLabel(id core.BlockID) string {
	name, _ := core.BlockDisplayName(id)
	return name
}

func readyMovementPlayer(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	engine := NewEngine(0, 0, 0)
	session := SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	requested := engine.Step()
	wantKey := core.ChunkKey{Dimension: core.Overworld}
	if !reflect.DeepEqual(requested.Acquire, []core.ChunkKey{wantKey}) {
		t.Fatalf("Acquire=%+v，想要 %+v", requested.Acquire, wantKey)
	}
	engine.SubmitAcquired(AcquiredChunk{Key: wantKey, Missing: true})
	generated := engine.Step()
	if !reflect.DeepEqual(generated.Generate, []core.ChunkKey{wantKey}) {
		t.Fatalf("Generate=%+v，想要 %+v", generated.Generate, wantKey)
	}
	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld,
		Chunk:     movementFlatChunk(core.ChunkPos{}),
	})
	spawned := engine.Step()
	if len(spawned.Players) != 1 || !spawned.Players[0].Ready {
		t.Fatalf("玩家没有在 flat world 激活: %+v", spawned.Players)
	}
	return engine, session
}

func movementFlatChunk(position core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(position)
	for x := 0; x < core.SectionSize; x++ {
		for z := 0; z < core.SectionSize; z++ {
			chunk.SetBlock(x, 0, z, core.GrassID)
		}
	}
	return chunk
}

func readyMiningPlayers(
	t *testing.T,
	count int,
) (*Engine, []SessionID, []core.BlockPos) {
	t.Helper()
	engine, _ := readyMovementPlayer(t)
	for id := count; id >= 2; id-- {
		engine.RegisterSession(SessionID(id), core.Overworld, core.ChunkPos{})
	}
	if count > 1 {
		engine.Step()
	}
	sessions := make([]SessionID, count)
	targets := make([]core.BlockPos, count)
	for index := range count {
		id := SessionID(index + 1)
		sessions[index] = id
		target := core.BlockPos{X: int32(index * 2), Y: 1, Z: 5}
		targets[index] = target
		engine.SetBlockForTest(target, core.StoneID)
		engine.SetPlayerPositionForTest(id, mgl32.Vec3{float32(index*2) + 0.5, 1, 8.5})
		engine.Enqueue(Command{
			Session: id, Sequence: uint64(10 + index), Kind: CommandPlayerInput,
			Pitch: -0.4, Mining: true,
		})
	}
	return engine, sessions, targets
}

type farmlandMoistureCandidateWatch struct {
	phaseSeen     bool
	candidateSeen bool
}

func watchFarmlandMoistureCandidateAtPhase(
	engine *Engine,
	dimension core.DimensionID,
	position core.BlockPos,
) *farmlandMoistureCandidateWatch {
	watch := &farmlandMoistureCandidateWatch{}
	engine.stepPhaseObserver = func(phase stepPhase) {
		if phase != phaseFarmlandMoistureAdvance {
			return
		}
		watch.phaseSeen = true
		watch.candidateSeen = engine.realm.FarmlandQueued(dimension, position)
	}
	return watch
}

func loadFlatChunks(t *testing.T, dimension *Dimension, minX, maxX, minZ, maxZ int32) {
	t.Helper()
	for x := minX; x <= maxX; x++ {
		for z := minZ; z <= maxZ; z++ {
			position := core.ChunkPos{X: x, Z: z}
			if info, ok := dimension.Info(position); ok && info.State == realm.ChunkReady {
				continue
			}
			chunk := movementFlatChunk(position)
			if !dimension.BeginGeneration(position) {
				t.Fatalf("区块 %+v 未开始生成", position)
			}
			if err := dimension.ApplyGenerated(position, chunk); err != nil {
				t.Fatal(err)
			}
		}
	}
}

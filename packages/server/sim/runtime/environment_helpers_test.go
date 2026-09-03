package runtime

import (
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/fluid"
	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

const farmlandWetRadius = 4

func (engine *Engine) fluidQueue(dimension core.DimensionID) *fluid.Queue {
	return engine.realm.FluidQueue(dimension)
}

func (engine *Engine) enqueueFluidUpdate(
	dimension core.DimensionID,
	position core.BlockPos,
) {
	engine.realm.EnqueueFluidUpdate(dimension, position)
}

func fluidNeighbors(position core.BlockPos) [6]core.BlockPos {
	return [6]core.BlockPos{
		{X: position.X, Y: position.Y + 1, Z: position.Z},
		{X: position.X, Y: position.Y - 1, Z: position.Z},
		{X: position.X + 1, Y: position.Y, Z: position.Z},
		{X: position.X - 1, Y: position.Y, Z: position.Z},
		{X: position.X, Y: position.Y, Z: position.Z + 1},
		{X: position.X, Y: position.Y, Z: position.Z - 1},
	}
}

func fluidScopeSnapshot(engine *Engine) map[core.ChunkKey]struct{} {
	keys := engine.realm.AppendFluidScopeKeys(nil)
	result := make(map[core.ChunkKey]struct{}, len(keys))
	for _, key := range keys {
		result[key] = struct{}{}
	}
	return result
}

// fluidWorld 只给旧的流体队列白盒测试提供 realm-backed 适配器；生产 tick 直接
// 调用 realm 的环境阶段，不经过该类型。
type fluidWorld struct {
	engine    *Engine
	id        core.DimensionID
	dimension *Dimension
	scope     map[core.ChunkKey]struct{}
	pending   *realm.Mutation
}

func (adapter *fluidWorld) chunk(position core.BlockPos) *world.Chunk {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return nil
	}
	key := core.ChunkKey{Dimension: adapter.id, Pos: position.Chunk()}
	if _, ok := adapter.scope[key]; !ok {
		return nil
	}
	chunk, _ := adapter.dimension.ReadyChunk(key.Pos)
	return chunk
}

func (adapter *fluidWorld) BlockAt(position core.BlockPos) core.BlockID {
	chunk := adapter.chunk(position)
	if chunk == nil {
		return core.BarrierID
	}
	x, _, z := position.Local()
	return chunk.BlockAt(x, position.Y, z)
}

func (adapter *fluidWorld) SetBlock(position core.BlockPos, block core.BlockID) {
	chunk := adapter.chunk(position)
	if chunk == nil {
		return
	}
	x, _, z := position.Local()
	if chunk.BlockAt(x, position.Y, z) == block {
		return
	}
	chunk.SetBlock(x, position.Y, z, block)
	adapter.pending.Record(adapter.id, position, block)
	adapter.engine.realm.EnqueueFluidUpdate(adapter.id, position)
}

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

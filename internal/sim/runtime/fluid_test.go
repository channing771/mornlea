package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

func fluidFlatChunk(position core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(position)
	for x := range core.SectionSize {
		for z := range core.SectionSize {
			chunk.SetBlock(x, 0, z, core.GrassID)
		}
	}
	chunk.Compact()
	return chunk
}

func fluidSeedChunk(position core.ChunkPos, seed map[core.BlockPos]core.BlockID) *world.Chunk {
	chunk := fluidFlatChunk(position)
	for blockPosition, block := range seed {
		if blockPosition.Chunk() != position {
			continue
		}
		x, _, z := blockPosition.Local()
		chunk.SetBlock(x, blockPosition.Y, z, block)
	}
	chunk.Compact()
	return chunk
}

func readyFluidPlayer(
	t *testing.T,
	seed map[core.BlockPos]core.BlockID,
	withhold func(core.ChunkPos) bool,
) (*Engine, SessionID, []core.ChunkKey) {
	t.Helper()
	engine := NewEngine(DropInterestRadius, 0, 0)
	const session = SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	var withheld []core.ChunkKey
	for range 12 {
		result := engine.Step()
		for _, key := range result.Acquire {
			engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			if withhold != nil && withhold(key.Pos) {
				withheld = append(withheld, key)
				continue
			}
			engine.SubmitGenerated(GeneratedChunk{
				Dimension: key.Dimension, Pos: key.Pos,
				Chunk: fluidSeedChunk(key.Pos, seed),
			})
		}
	}
	if player, ok := engine.Player(session); !ok || !player.Ready {
		t.Fatalf("玩家未 Ready: %+v", player)
	}
	return engine, session, withheld
}

func fluidBlockAt(t *testing.T, engine *Engine, position core.BlockPos) core.BlockID {
	t.Helper()
	block, ready := engine.dimension(core.Overworld).BlockAt(position)
	if !ready {
		t.Fatalf("读取 %+v 时区块未就绪", position)
	}
	return block
}

func TestFluidQueuesArePerDimension(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	overworld := engine.realm.FluidQueue(core.Overworld)
	if engine.realm.FluidQueue(core.Overworld) != overworld {
		t.Fatal("同一维度两次取到了不同队列")
	}
	other := engine.realm.FluidQueue(core.Overworld + 1)
	if other == overworld {
		t.Fatal("两个维度共用流体队列")
	}
	overworld.Enqueue(core.BlockPos{X: 1, Y: 2, Z: 3}, 0, 0)
	if other.Len() != 0 {
		t.Fatalf("另一维度队列被污染，Len=%d", other.Len())
	}
}

func TestFluidRescanWakesFluidAcrossChunkBoundary(t *testing.T) {
	source := core.BlockPos{X: 12, Y: 1, Z: 8}
	seed := map[core.BlockPos]core.BlockID{source: core.WaterSourceID}
	late := core.ChunkPos{X: 1}
	engine, _, withheld := readyFluidPlayer(t, seed, func(position core.ChunkPos) bool {
		return position == late
	})
	if len(withheld) != 1 || withheld[0].Pos != late {
		t.Fatalf("延后区块=%+v，想要 %+v", withheld, late)
	}
	for range 200 {
		engine.Step()
	}
	if got := fluidBlockAt(t, engine, core.BlockPos{X: 15, Y: 1, Z: 8}); got != core.WaterLevel3ID {
		t.Fatalf("接缝内侧=%d，想要 %d", got, core.WaterLevel3ID)
	}
	if got := engine.realm.FluidQueue(core.Overworld).Len(); got != 0 {
		t.Fatalf("平衡后队列仍有 %d 项", got)
	}
	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld, Pos: late, Chunk: fluidSeedChunk(late, seed),
	})
	for range 200 {
		engine.Step()
	}
	for _, item := range []struct {
		position core.BlockPos
		want     core.BlockID
	}{
		{core.BlockPos{X: 16, Y: 1, Z: 8}, core.WaterLevel4ID},
		{core.BlockPos{X: 17, Y: 1, Z: 8}, core.WaterLevel5ID},
		{core.BlockPos{X: 18, Y: 1, Z: 8}, core.WaterLevel6ID},
		{core.BlockPos{X: 19, Y: 1, Z: 8}, core.WaterLevel7ID},
	} {
		if got := fluidBlockAt(t, engine, item.position); got != item.want {
			t.Fatalf("接缝外侧 %+v=%d，想要 %d", item.position, got, item.want)
		}
	}
}

func TestFluidOutsideInterestRangeHoldsAndResumes(t *testing.T) {
	inside := core.BlockPos{X: 3<<core.SectionShift - 1, Y: 5, Z: 8}
	outside := core.BlockPos{X: 3 << core.SectionShift, Y: 5, Z: 8}
	outsideChunk := core.ChunkPos{X: 3}
	seed := map[core.BlockPos]core.BlockID{inside: core.WaterLevel1ID, outside: core.WaterLevel1ID}
	engine := NewEngine(DropInterestRadius, 0, 0)
	dimension := engine.dimension(core.Overworld)
	if !dimension.BeginGeneration(outsideChunk) {
		t.Fatal("范围外区块未开始生成")
	}
	if err := dimension.ApplyGenerated(outsideChunk, fluidSeedChunk(outsideChunk, seed)); err != nil {
		t.Fatal(err)
	}
	const session = SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	for range 12 {
		result := engine.Step()
		for _, key := range result.Acquire {
			engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			engine.SubmitGenerated(GeneratedChunk{
				Dimension: key.Dimension, Pos: key.Pos, Chunk: fluidSeedChunk(key.Pos, seed),
			})
		}
	}
	for range 60 {
		engine.Step()
	}
	if got := fluidBlockAt(t, engine, inside); got != core.AirID {
		t.Fatalf("范围内对照格=%d，想要空气", got)
	}
	if got := fluidBlockAt(t, engine, outside); got != core.WaterLevel1ID {
		t.Fatalf("范围外流体=%d，想要保持 %d", got, core.WaterLevel1ID)
	}
	if info, ok := dimension.Info(outsideChunk); !ok || info.State != realm.ChunkReady {
		t.Fatalf("范围外区块状态=%+v", info)
	}
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{
		float32(3<<core.SectionShift) + 8.5, 1, 8.5,
	})
	for range 60 {
		engine.Step()
	}
	if got := fluidBlockAt(t, engine, outside); got != core.AirID {
		t.Fatalf("重新进入范围后流体=%d，想要空气", got)
	}
}

func TestBlockRemovalEnqueuesNeighbouringFluid(t *testing.T) {
	engine, _, targets := readyMiningPlayers(t, 1)
	target := targets[0]
	source := core.BlockPos{X: target.X, Y: target.Y, Z: target.Z - 1}
	behind := core.BlockPos{X: target.X, Y: target.Y, Z: target.Z - 2}
	engine.SetBlockForTest(source, core.WaterSourceID)
	for range 10 {
		engine.Step()
	}
	if got := fluidBlockAt(t, engine, target); got != core.StoneID {
		t.Fatalf("采掘完成前目标=%d", got)
	}
	if got := fluidBlockAt(t, engine, behind); got != core.AirID {
		t.Fatalf("采掘完成前水已扩散到背面=%d", got)
	}
	for range 120 {
		engine.Step()
	}
	if got := fluidBlockAt(t, engine, target); got != core.WaterLevel1ID {
		t.Fatalf("采掘空位=%d，想要 %d", got, core.WaterLevel1ID)
	}
	if got := fluidBlockAt(t, engine, behind); got != core.WaterLevel1ID {
		t.Fatalf("水源背面=%d，想要 %d", got, core.WaterLevel1ID)
	}
}

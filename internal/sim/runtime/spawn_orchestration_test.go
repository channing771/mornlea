package runtime

import (
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

func TestSpawnWaitsForEarlierUnknownCandidate(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	requested := engine.Step()
	for _, key := range requested.Acquire {
		engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
	}
	engine.Step()

	laterChunk := world.NewChunk(core.ChunkPos{X: -1})
	laterChunk.SetBlock(15, 0, 0, core.GrassID)
	loadSpawnTestChunk(t, engine.dimension(core.Overworld), laterChunk)
	if player := onlyInternalPlayer(t, engine.Step()); player.Ready {
		t.Fatalf("较早候选仍 unknown 时跳到了较晚 surface: %+v", player)
	}

	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld,
		Pos:       core.ChunkPos{},
		Chunk:     spawnTestChunk(core.ChunkPos{}, core.BlockPos{}),
	})
	player := onlyInternalPlayer(t, engine.Step())
	if !player.Ready || player.State.Position != (mgl32.Vec3{0.5, 1, 0.5}) {
		t.Fatalf("较早候选 Ready 后 spawn=%+v", player)
	}
}

func TestPendingSpawnGenerateRetainActivateAndForget(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	const sessionID = SessionID(1)
	anchor := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	target := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -1}}
	engine.RegisterSession(sessionID, core.Overworld, anchor.Pos)

	acquiredAnchor := engine.Step()
	if player := onlyInternalPlayer(t, acquiredAnchor); player.Ready {
		t.Fatalf("生成 anchor 前 player=%+v，想要 PendingSpawn", player)
	}
	if !reflect.DeepEqual(acquiredAnchor.Acquire, []core.ChunkKey{anchor}) {
		t.Fatalf("首次 Acquire=%+v，想要 [%+v]", acquiredAnchor.Acquire, anchor)
	}
	engine.SubmitAcquired(AcquiredChunk{Key: anchor, Missing: true})
	generatedAnchor := engine.Step()
	if !reflect.DeepEqual(generatedAnchor.Generate, []core.ChunkKey{anchor}) {
		t.Fatalf("首次 Generate=%+v，想要 [%+v]", generatedAnchor.Generate, anchor)
	}

	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld,
		Pos:       anchor.Pos,
		Chunk:     world.NewChunk(anchor.Pos),
	})
	retained := engine.Step()
	if player := onlyInternalPlayer(t, retained); player.Ready {
		t.Fatalf("空 anchor 后 player=%+v，想要继续 PendingSpawn", player)
	}
	if !reflect.DeepEqual(retained.Ready, []core.ChunkKey{anchor}) ||
		!reflect.DeepEqual(retained.Acquire, []core.ChunkKey{target}) {
		t.Fatalf("retain tick Ready=%+v Acquire=%+v", retained.Ready, retained.Acquire)
	}
	wanted := engine.subscriptions[sessionID].wanted
	if _, ok := wanted[anchor]; !ok {
		t.Fatalf("PendingSpawn 未保留 anchor: wanted=%+v", wanted)
	}
	if _, ok := wanted[target]; !ok {
		t.Fatalf("PendingSpawn 未保留 target: wanted=%+v", wanted)
	}
	engine.SubmitAcquired(AcquiredChunk{Key: target, Missing: true})
	generatedTarget := engine.Step()
	if !reflect.DeepEqual(generatedTarget.Generate, []core.ChunkKey{target}) {
		t.Fatalf("target Generate=%+v", generatedTarget.Generate)
	}
	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld,
		Pos:       target.Pos,
		Chunk: spawnTestChunk(target.Pos, core.BlockPos{
			X: -1,
			Y: 0,
			Z: 0,
		}),
	})
	activated := engine.Step()
	player := onlyInternalPlayer(t, activated)
	if !player.Ready || !player.Reset ||
		player.State.Position != (mgl32.Vec3{-0.5, 1, 0.5}) {
		t.Fatalf("target Ready 后 player=%+v，想要 Active reset", player)
	}
	if !reflect.DeepEqual(activated.Ready, []core.ChunkKey{target}) ||
		!reflect.DeepEqual(activated.Forget[sessionID], []core.ChunkKey{anchor}) {
		t.Fatalf("activate tick Ready=%+v Forget=%+v", activated.Ready, activated.Forget)
	}
	wanted = engine.subscriptions[sessionID].wanted
	if !reflect.DeepEqual(wanted, map[core.ChunkKey]struct{}{target: {}}) {
		t.Fatalf("Active subscription wanted=%+v，想要仅 target", wanted)
	}
	if !reflect.DeepEqual(engine.wanted, map[core.ChunkKey]struct{}{target: {}}) {
		t.Fatalf("Active union wanted=%+v，想要仅 target", engine.wanted)
	}
	info := requireChunkInfo(t, engine.dimension(core.Overworld), anchor.Pos)
	if info.State != realm.ChunkUnloading || info.Chunk == nil ||
		!info.UnloadRequested || !info.Dirty {
		t.Fatalf("activate forget 后未保留待持久 anchor: %+v", info)
	}
}

func onlyInternalPlayer(t *testing.T, result TickResult) PlayerUpdate {
	t.Helper()
	if len(result.Players) != 1 {
		t.Fatalf("Players=%+v，想要恰好一个", result.Players)
	}
	return result.Players[0]
}

func loadSpawnTestChunk(t *testing.T, dimension *Dimension, chunk *world.Chunk) {
	t.Helper()
	if !dimension.BeginGeneration(chunk.Pos) {
		t.Fatalf("区块 %+v 未开始生成", chunk.Pos)
	}
	if err := dimension.ApplyGenerated(chunk.Pos, chunk); err != nil {
		t.Fatal(err)
	}
}

func spawnTestChunk(pos core.ChunkPos, support core.BlockPos) *world.Chunk {
	chunk := world.NewChunk(pos)
	x, _, z := support.Local()
	chunk.SetBlock(x, support.Y, z, core.GrassID)
	return chunk
}

func TestExhaustedSpawnRetriesOnlyAfterRevisionChange(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	dimension := engine.dimension(core.Overworld)
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			loadSpawnTestChunk(t, dimension, world.NewChunk(core.ChunkPos{X: x, Z: z}))
		}
	}

	if player := onlyInternalPlayer(t, engine.Step()); player.Ready {
		t.Fatalf("全空气候选不应 Ready: %+v", player)
	}
	dimension.UpdateReadyChunk(core.ChunkPos{}, func(chunk *world.Chunk) {
		chunk.SetBlock(0, 0, 0, core.GrassID)
	})
	if player := onlyInternalPlayer(t, engine.Step()); player.Ready {
		t.Fatalf("revision 未变却重新扫描: %+v", player)
	}

	dimension.Touch(core.ChunkPos{})
	player := onlyInternalPlayer(t, engine.Step())
	if !player.Ready || player.State.Position != (mgl32.Vec3{0.5, 1, 0.5}) {
		t.Fatalf("revision 改变后未从首候选重试: %+v", player)
	}
}

func TestSpawnFallbackSurvivesChunkReadinessGap(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	dimension := engine.dimension(core.Overworld)
	gap := core.ChunkPos{X: -1, Z: -1}
	pillar := core.BlockPos{X: 0, Z: 0}

	var withheld *world.Chunk
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			pos := core.ChunkPos{X: x, Z: z}
			chunk := runtimeSpawnLadderChunk(pos, 4)
			if pos == pillar.Chunk() {
				runtimeSpawnLadderPillar(chunk, pillar, 3)
			}
			if pos == gap {
				withheld = chunk
				continue
			}
			loadSpawnTestChunk(t, dimension, chunk)
		}
	}
	if withheld == nil {
		t.Fatalf("夹具失效：候选区块里没有 %+v", gap)
	}

	first := engine.Step()
	if player := onlyInternalPlayer(t, first); player.Ready {
		t.Fatalf("缺口区块未就绪时不应出生: %+v", player)
	}
	gapKey := core.ChunkKey{Dimension: core.Overworld, Pos: gap}
	if !containsChunkKey(first.Acquire, gapKey) {
		t.Fatalf("断点缺口未进入 runtime acquire：%+v", first.Acquire)
	}

	var player PlayerUpdate
	result := first
	for range 8 {
		for _, key := range result.Acquire {
			if key != gapKey {
				t.Fatalf("夹具失效：意外 acquire %+v", key)
			}
			engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			if key != gapKey {
				t.Fatalf("夹具失效：意外生成 %+v", key)
			}
			engine.SubmitGenerated(GeneratedChunk{
				Dimension: key.Dimension, Pos: key.Pos, Chunk: withheld,
			})
		}
		result = engine.Step()
		player = onlyInternalPlayer(t, result)
		if player.Ready {
			break
		}
	}
	if !player.Ready {
		t.Fatalf("补齐缺口区块后仍未出生: %+v", player)
	}
	if got := player.State.Position; got != (mgl32.Vec3{0.5, 4, 0.5}) {
		t.Fatalf("出生点=%v，想要断点之前记下的第 2 档柱顶 (0.5,4,0.5)", got)
	}

	x, _, z := (core.BlockPos{X: -1, Z: -1}).Local()
	if got := withheld.BlockAt(x, 0, z); got != core.GrassID {
		t.Fatalf("夹具失效：断点列列底=%d，想要草方块 %d", got, core.GrassID)
	}
	for y := int32(1); y <= 2; y++ {
		if got := withheld.BlockAt(x, y, z); !core.IsFluid(got) {
			t.Fatalf("夹具失效：断点列 y=%d 是 %d，不是流体", y, got)
		}
	}
}

func runtimeSpawnLadderChunk(pos core.ChunkPos, depth int32) *world.Chunk {
	chunk := world.NewChunk(pos)
	for x := range core.SectionSize {
		for z := range core.SectionSize {
			chunk.SetBlock(x, 0, z, core.GrassID)
			for y := int32(1); y <= depth; y++ {
				chunk.SetBlock(x, y, z, core.WaterSourceID)
			}
		}
	}
	chunk.Compact()
	return chunk
}

func runtimeSpawnLadderPillar(chunk *world.Chunk, column core.BlockPos, top int32) {
	x, _, z := column.Local()
	for y := int32(1); y <= top; y++ {
		chunk.SetBlock(x, y, z, core.GrassID)
	}
	chunk.Compact()
}

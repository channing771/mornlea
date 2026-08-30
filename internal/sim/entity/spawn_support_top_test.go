package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

func TestSpawnFarmlandSupportTop(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	dimension := engine.dimension(core.Overworld)
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			chunk := world.NewChunk(core.ChunkPos{X: x, Z: z})
			for localX := 0; localX < core.SectionSize; localX++ {
				for localZ := 0; localZ < core.SectionSize; localZ++ {
					chunk.SetBlock(localX, 0, localZ, core.GrassID)
				}
			}
			if x == 0 && z == 0 {
				chunk.SetBlock(0, 0, 0, core.FarmlandDryID)
			}
			loadSpawnTestChunk(t, dimension, chunk)
		}
	}
	update := onlyInternalPlayer(t, engine.Step())
	if !update.Ready || update.State.Position != (mgl32.Vec3{0.5, 0.9375, 0.5}) {
		t.Fatalf("spawn=%+v, want farmland top", update)
	}
}

func TestPlayerRestoreFarmlandSupportTop(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := SessionID(70)
	current := PlayerLocation{Dimension: core.Overworld, Position: mgl32.Vec3{2.5, 0.9375, 0.5}}
	safe := PlayerLocation{Dimension: core.Overworld, Position: mgl32.Vec3{16.5, 1, 0.5}}
	engine.RegisterPlayer(id, PlayerRestore{Current: &current, Safe: &safe, SpawnDimension: core.Overworld})
	makeRestoreWorldReady(t, engine, current, safe)
	setRestoreBlock(t, engine, core.BlockPos{X: 2, Y: 0, Z: 0}, core.FarmlandDryID)
	update := onlyPlayerUpdate(t, engine.Step(), id)
	if !update.Ready || update.State.Position != current.Position || !update.State.OnGround {
		t.Fatalf("restore=%+v, want exact farmland top", update)
	}
}

func TestSafeFarmlandSupportTop(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := SessionID(71)
	current := PlayerLocation{Dimension: core.Overworld, Position: mgl32.Vec3{2.5, 1, 0.5}}
	safe := PlayerLocation{Dimension: core.Overworld, Position: mgl32.Vec3{16.5, 1, 0.5}}
	engine.RegisterPlayer(id, PlayerRestore{Current: &current, Safe: &safe, SpawnDimension: core.Overworld})
	makeRestoreWorldReady(t, engine, current, safe)
	setRestoreBlock(t, engine, core.BlockPos{X: 2, Y: 0, Z: 0}, core.FarmlandDryID)
	if update := onlyPlayerUpdate(t, engine.Step(), id); !update.Ready {
		t.Fatalf("initial restore=%+v", update)
	}
	engine.sessions[id].player.state.Position = mgl32.Vec3{2.5, 0.9375, 0.5}
	engine.sessions[id].player.state.OnGround = true
	engine.Step()
	if engine.sessions[id].player.state.Position != (mgl32.Vec3{2.5, 0.9375, 0.5}) {
		t.Fatalf("player position=%v, want farmland top", engine.sessions[id].player.state.Position)
	}
	if got := engine.sessions[id].player.safe; got == nil || got.Position != (mgl32.Vec3{2.5, 0.9375, 0.5}) {
		t.Fatalf("safe=%+v, want farmland top", got)
	}
}

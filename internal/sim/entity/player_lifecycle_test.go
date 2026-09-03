package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

func TestSafeLocationUpdatesOnlyAfterCompleteReadyGroundContact(t *testing.T) {
	tests := []struct {
		name       string
		position   mgl32.Vec3
		onGround   bool
		prepare    func(*testing.T, *Engine)
		wantUpdate bool
	}{
		{
			name:       "complete grounded support",
			position:   mgl32.Vec3{3.5, 1, 0.5},
			onGround:   true,
			wantUpdate: true,
		},
		{
			name:     "airborne",
			position: mgl32.Vec3{3.5, 5, 0.5},
		},
		{
			name:     "unknown adjacent chunk",
			position: mgl32.Vec3{63.9, 1, 0.5},
			onGround: true,
		},
		{
			name:     "partial support",
			position: mgl32.Vec3{3.95, 1, 0.5},
			onGround: true,
			prepare: func(t *testing.T, engine *Engine) {
				setRestoreBlock(t, engine, core.BlockPos{X: 4, Y: 0, Z: 0}, core.AirID)
			},
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := NewEngine(0, 0, 0)
			id := SessionID(20 + index)
			current := PlayerLocation{
				Dimension: core.Overworld,
				Position:  mgl32.Vec3{2.5, 1, 0.5},
			}
			originalSafe := PlayerLocation{
				Dimension: core.Overworld,
				Position:  mgl32.Vec3{16.5, 1, 0.5},
			}
			engine.RegisterPlayer(id, PlayerRestore{
				Current:        &current,
				Safe:           &originalSafe,
				SpawnDimension: core.Overworld,
			})
			makeRestoreWorldReady(t, engine, current, originalSafe)
			if update := onlyPlayerUpdate(t, advanceActorsTick(engine), id); !update.Ready {
				t.Fatalf("initial restore=%+v", update)
			}
			if test.prepare != nil {
				test.prepare(t, engine)
			}
			engine.sessions[id].player.state = physics.State{
				Position: test.position,
				OnGround: test.onGround,
			}

			advanceActorsTick(engine)
			safe := engine.sessions[id].player.safe
			want := originalSafe.Position
			if test.wantUpdate {
				want = test.position
			}
			if safe == nil || safe.Dimension != core.Overworld || safe.Position != want {
				t.Fatalf("safe=%+v, want position %v", safe, want)
			}
			if test.wantUpdate {
				snapshot, ok := engine.PlayerSnapshot(id)
				if !ok || snapshot.Safe == nil || snapshot.Safe.Position != want {
					t.Fatalf("PlayerSnapshot=(%+v,%v), want safe %v", snapshot, ok, want)
				}
			}
		})
	}
}

func TestPlayerRestoreAndSnapshotDeepCopySafeLocation(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := SessionID(42)
	current := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{2.5, 1, 0.5},
	}
	persistedSafe := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{16.5, 1, 0.5},
	}
	suppliedSafe := persistedSafe
	engine.RegisterPlayer(id, PlayerRestore{
		Current:        &current,
		Safe:           &suppliedSafe,
		SpawnDimension: core.Overworld,
	})
	makeRestoreWorldReady(t, engine, current, persistedSafe)
	suppliedSafe.Position = mgl32.Vec3{99.5, 99, 99.5}
	if update := onlyPlayerUpdate(t, advanceActorsTick(engine), id); !update.Ready {
		t.Fatalf("restore=%+v", update)
	}

	first, ok := engine.PlayerSnapshot(id)
	if !ok || first.Safe == nil || *first.Safe != persistedSafe {
		t.Fatalf("first snapshot=(%+v,%v), want safe %+v", first, ok, persistedSafe)
	}
	first.Safe.Position = mgl32.Vec3{77.5, 77, 77.5}
	second, ok := engine.PlayerSnapshot(id)
	if !ok || second.Safe == nil || *second.Safe != persistedSafe {
		t.Fatalf("snapshot alias polluted authority: (%+v,%v)", second, ok)
	}
}

func TestUpdateSafeLocationDoesNotAllocateAfterInitialization(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := SessionID(43)
	current := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{2.5, 1, 0.5},
	}
	safe := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{16.5, 1, 0.5},
	}
	engine.RegisterPlayer(id, PlayerRestore{
		Current:        &current,
		Safe:           &safe,
		SpawnDimension: core.Overworld,
	})
	makeRestoreWorldReady(t, engine, current, safe)
	if update := onlyPlayerUpdate(t, advanceActorsTick(engine), id); !update.Ready {
		t.Fatalf("restore=%+v", update)
	}
	session := engine.sessions[id]
	allocations := testing.AllocsPerRun(1000, func() {
		engine.updateSafeLocation(session)
	})
	if allocations != 0 {
		t.Fatalf("updateSafeLocation allocs/op=%f, want 0", allocations)
	}
}

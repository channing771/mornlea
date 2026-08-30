package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
)

func TestEntityAuthorityUsesExplicitPhysicsTunables(t *testing.T) {
	t.Cleanup(func() { physics.SetTunables(physics.DefaultTunables()) })

	explicit := physics.DefaultTunables()
	explicit.EyeHeight = 1.62
	explicit.GroundAcceleration = 2
	conflicting := explicit
	conflicting.EyeHeight = 0.2
	conflicting.GroundAcceleration = 40
	physics.SetTunables(conflicting)

	t.Run("player", func(t *testing.T) {
		engine, session := readyMovementPlayer(t)
		engine.physicsTunables = explicit
		player := engine.sessions[session].player
		player.input = physics.Input{MoveX: 1}
		before := player.state
		source := dimensionCollisionSource{dimension: engine.dimension(core.Overworld)}
		want := physics.StepWithTunables(before, player.input, source, explicit).State
		engine.advanceActivePlayers()
		if player.state != want {
			t.Fatalf("玩家未服从显式 physics tunables：got=%+v want=%+v", player.state, want)
		}
	})

	t.Run("companion", func(t *testing.T) {
		engine, _ := readyMovementPlayer(t)
		id := companionTestID(17)
		activateCompanionAt(t, engine, id, mgl32.Vec3{2.5, 1, 2.5})
		engine.physicsTunables = explicit
		entry := engine.companions[id]
		entry.input = physics.Input{MoveX: 1}
		before := entry.state
		source := dimensionCollisionSource{dimension: engine.dimension(core.Overworld)}
		want := physics.StepWithTunables(before, entry.input, source, explicit).State
		engine.advanceActiveCompanions()
		if entry.state != want {
			t.Fatalf("伙伴未服从显式 physics tunables：got=%+v want=%+v", entry.state, want)
		}
	})

	t.Run("hostile", func(t *testing.T) {
		engine, _ := readyMovementPlayer(t)
		mob := validTestHostile(170)
		if err := engine.RestoreHostile(mob); err != nil {
			t.Fatal(err)
		}
		engine.physicsTunables = explicit
		entry := &engine.hostiles.entries[0]
		entry.input = physics.Input{MoveX: 1}
		before := entry.state
		source := dimensionCollisionSource{dimension: engine.dimension(core.Overworld)}
		want := physics.StepWithTunables(before, entry.input, source, explicit).State
		engine.advanceHostileMovement()
		if entry.state != want {
			t.Fatalf("夜行者未服从显式 physics tunables：got=%+v want=%+v", entry.state, want)
		}
	})

	t.Run("spawn tier", func(t *testing.T) {
		engine, _ := readyMovementPlayer(t)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 0}, core.WaterSourceID)
		source := dimensionCollisionSource{dimension: engine.dimension(core.Overworld)}
		if tier := spawnTierOf(mgl32.Vec3{0.5, 1, 0.5}, source, explicit); tier != spawnTierSubmerged {
			t.Fatalf("出生档位未服从显式 EyeHeight：got=%v want=%v", tier, spawnTierSubmerged)
		}
		if tier := spawnTierOf(mgl32.Vec3{0.5, 1, 0.5}, source, conflicting); tier != spawnTierEyeDry {
			t.Fatalf("冲突 EyeHeight 对照夹具失效：got=%v want=%v", tier, spawnTierEyeDry)
		}
	})
}

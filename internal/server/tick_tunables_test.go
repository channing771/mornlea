package server

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

func TestCompanionManagerUsesExplicitTickTunables(t *testing.T) {
	previousPhysics := physics.ActiveTunables()
	previousSimulation := tuning.ActiveTunables()
	t.Cleanup(func() {
		physics.SetTunables(previousPhysics)
		tuning.SetTunables(previousSimulation)
	})

	explicit := runtime.ActiveTickTunables()
	explicit.Physics.EyeHeight = 1.62
	explicit.Simulation.InteractionReach = 3
	conflicting := explicit
	conflicting.Physics.EyeHeight = 10
	conflicting.Simulation.InteractionReach = 0.25
	physics.SetTunables(conflicting.Physics)
	tuning.SetTunables(conflicting.Simulation)

	t.Run("within interaction reach", func(t *testing.T) {
		body := companion.Body{Position: [3]float32{0.5, 0, 0.5}}
		target := core.BlockPos{X: 1, Y: 2, Z: 0}
		if !withinInteractionReach(body, target, explicit) {
			t.Fatal("伙伴交互距离未服从显式参数束")
		}
		if withinInteractionReach(body, target, conflicting) {
			t.Fatal("冲突参数束对照夹具未生效")
		}
	})

	t.Run("issuer look hit", func(t *testing.T) {
		definitions := []companion.Definition{{ID: chatTestCompanionID(1), Name: "阿木"}}
		host, _, _ := companionManagerHostReady(t, definitions, nil)
		active := activeLoginForPlayer(t, host, integrationIdentity(0x71, "发令者").PlayerID)
		player, ok := host.world.engine.Player(active.Session)
		if !ok || !player.Ready {
			t.Fatalf("发令者未 ready：%+v", player)
		}
		player.Pitch = -float32(math.Pi / 2)
		hit, ok := host.world.companionManager.issuerLookHit(player, explicit)
		if !ok || hit.Y != 0 {
			t.Fatalf("显式参数束未命中脚下地面：hit=%+v ok=%v", hit, ok)
		}
		if _, ok := host.world.companionManager.issuerLookHit(player, conflicting); ok {
			t.Fatal("冲突参数束对照夹具意外命中")
		}
	})
}

package runtime

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

func TestCombatKnockbackAddsNormalizedHorizontalVelocity(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	attackerPosition := mgl32.Vec3{0, 1, 0}
	targetPosition := mgl32.Vec3{3, 1, 4}
	setMeleePlayer(engine, sessions[0], attackerPosition, 0)
	setMeleePlayer(engine, sessions[1], targetPosition, 0)
	target := engine.sessions[sessions[1]].player
	target.state.Velocity = mgl32.Vec3{1, 2, 3}
	intent := combatIntent{
		attacker:         combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[0])},
		target:           combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[1])},
		damage:           2,
		attackerPosition: attackerPosition,
		targetPosition:   targetPosition,
	}

	if !engine.settleCombatIntent(&TickResult{}, intent) {
		t.Fatal("合法 intent 未结算")
	}
	want := mgl32.Vec3{1.21, 2, 3.28}
	for axis := range 3 {
		if math.Abs(float64(target.state.Velocity[axis]-want[axis])) > 1e-6 {
			t.Fatalf("velocity=%v，想要 %v", target.state.Velocity, want)
		}
	}
}

func TestCombatKnockbackOverlapUsesFiniteAttackerYaw(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	position := mgl32.Vec3{2, 1, 2}
	setMeleePlayer(engine, sessions[0], position, math.Pi/2)
	setMeleePlayer(engine, sessions[1], position, 0)
	target := engine.sessions[sessions[1]].player
	target.state.Velocity = mgl32.Vec3{0.25, 4, 0.75}
	intent := combatIntent{
		attacker:         combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[0])},
		target:           combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[1])},
		damage:           2,
		attackerPosition: position,
		targetPosition:   position,
		attackerYaw:      math.Pi / 2,
	}

	if !engine.settleCombatIntent(&TickResult{}, intent) {
		t.Fatal("水平重合 intent 未结算")
	}
	want := mgl32.Vec3{-0.10, 4, 0.75}
	for axis := range 3 {
		got := float64(target.state.Velocity[axis])
		if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-float64(want[axis])) > 1e-6 {
			t.Fatalf("水平重合 velocity=%v，想要有限值 %v", target.state.Velocity, want)
		}
	}
}

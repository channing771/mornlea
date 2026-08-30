package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件锁定 hostile-first victim reservation、loser 无副作用和不同 victim 互击。

func TestCombatReservationHostileFirst(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{2.0, 1, 2.5})
	engine.sessions[sessions[0]].player.miningHeld = true
	hostile := &engine.hostiles.entries[0]
	hostile.attackIntent = true
	hostile.attackTargetSession = sessions[1]

	engine.advanceCombat(&TickResult{})
	if got := engine.sessions[sessions[1]].player.health; got != 17 {
		t.Fatalf("同 victim 竞争后 health=%d，想要只受 hostile 3 点后的 17", got)
	}
	if hostile.attackCooldown != 20 {
		t.Fatalf("hostile winner attack cooldown=%d，想要 20", hostile.attackCooldown)
	}
	if got := engine.sessions[sessions[0]].player.attackCooldownTicks; got != 0 {
		t.Fatalf("player loser attack cooldown=%d，想要 0", got)
	}
}

func TestCombatReservationSameKindLowestID(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 7, mgl32.Vec3{0.5, 1, 2.0})
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{2.0, 1, 0.5})
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	for index := range engine.hostiles.entries {
		hostile := &engine.hostiles.entries[index]
		hostile.attackIntent = true
		hostile.attackTargetSession = session
	}

	engine.advanceCombat(&TickResult{})
	if got := engine.sessions[session].player.health; got != 17 {
		t.Fatalf("同 kind 竞争后 health=%d，想要 17", got)
	}
	if got := engine.hostiles.entries[0].attackCooldown; got != 20 {
		t.Fatalf("较小 hostile ID cooldown=%d，想要 20", got)
	}
	if got := engine.hostiles.entries[1].attackCooldown; got != 0 {
		t.Fatalf("较大 hostile ID cooldown=%d，想要 0", got)
	}
}

func TestCombatReservationLoserHasNoSideEffects(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{2.0, 1, 2.5})
	loser := engine.sessions[sessions[0]].player
	loser.miningHeld = true
	loser.inventory.Hotbar.Selected = 0
	loser.inventory.Hotbar.Slots[0] = core.ItemStack{
		Item: core.ItemIronSword, Count: 1, Durability: 10,
	}
	loser.inventoryDirty = false
	beforeInventory := loser.inventory
	hostile := &engine.hostiles.entries[0]
	hostile.attackIntent = true
	hostile.attackTargetSession = sessions[1]
	result := TickResult{}

	engine.advanceCombat(&result)
	if loser.attackCooldownTicks != 0 || loser.exhaustionMilli != 0 ||
		loser.meleeSuppressedMining || loser.inventoryDirty || loser.inventory != beforeInventory ||
		len(result.CombatHits) != 0 {
		t.Fatalf("reservation loser 留下副作用: attack=%d fatigue=%d suppressed=%v dirty=%v inventory=%+v hits=%+v",
			loser.attackCooldownTicks, loser.exhaustionMilli, loser.meleeSuppressedMining,
			loser.inventoryDirty, loser.inventory, result.CombatHits)
	}
}

func TestCombatMutualPlayersSettleBeforeDeaths(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 3.1415927)
	for _, id := range sessions {
		player := engine.sessions[id].player
		player.health = 2
		player.miningHeld = true
	}

	engine.advanceCombat(&TickResult{})
	for _, id := range sessions {
		player := engine.sessions[id].player
		if player.health != 0 || player.attackCooldownTicks != 10 || player.hurtCooldownTicks != 10 {
			t.Fatalf("session %d mutual 结果 health/attack/hurt=(%d,%d,%d)，想要 (0,10,10)",
				id, player.health, player.attackCooldownTicks, player.hurtCooldownTicks)
		}
	}
}

func TestCombatMutualPlayerHostileSettleBeforeDeaths(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.0}, 0)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{0.5, 1, 2.5})
	player := engine.sessions[session].player
	player.health = 3
	player.miningHeld = true
	hostile := &engine.hostiles.entries[0]
	hostile.health = 2
	hostile.attackIntent = true
	hostile.attackTargetSession = session

	engine.advanceCombat(&TickResult{})
	if player.health != 0 || hostile.health != 0 {
		t.Fatalf("跨 kind mutual health=(player %d, hostile %d)，想要双方 0",
			player.health, hostile.health)
	}
	if player.attackCooldownTicks != 10 || player.hurtCooldownTicks != 20 ||
		hostile.attackCooldown != 20 || hostile.hurtCooldown != 10 {
		t.Fatalf("跨 kind mutual cooldown=(%d,%d,%d,%d)，想要 (10,20,20,10)",
			player.attackCooldownTicks, player.hurtCooldownTicks,
			hostile.attackCooldown, hostile.hurtCooldown)
	}
}

func TestCombatInvalidHostileTargetHasNoSideEffects(t *testing.T) {
	engine, _ := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{0.5, 1, 2.5})
	restoreCombatHostile(t, engine, 5, mgl32.Vec3{0.5, 1, 1.5})
	attacker := &engine.hostiles.entries[0]
	target := &engine.hostiles.entries[1]
	beforeState := target.state
	result := TickResult{}
	intent := combatIntent{
		attacker:  combatActor{kind: core.CombatTargetHostile, id: attacker.id},
		target:    combatActor{kind: core.CombatTargetHostile, id: target.id},
		dimension: core.Overworld, damage: 3,
		attackerPosition: attacker.state.Position, targetPosition: target.state.Position,
	}

	if engine.settleCombatIntent(&result, intent) {
		t.Fatal("hostile→hostile intent 被接受")
	}
	if target.health != core.MaxHealth || target.state != beforeState ||
		attacker.attackCooldown != 0 || target.hurtCooldown != 0 || len(result.CombatHits) != 0 {
		t.Fatalf("非法 hostile target 留下副作用: health=%d state=%+v attack/hurt=(%d,%d) hits=%+v",
			target.health, target.state, attacker.attackCooldown, target.hurtCooldown, result.CombatHits)
	}
}

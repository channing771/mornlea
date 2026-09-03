package entity

import (
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定统一战斗阶段的固定容量、整阶段溢出和来源独立冷却边界。

type combatPlayerStateForTest struct {
	health                       uint8
	velocity                     mgl32.Vec3
	attackCooldown, hurtCooldown uint8
	inventory                    core.Inventory
	inventoryDirty               bool
	exhaustion                   uint16
	meleeSuppressedMining        bool
}

type combatHostileStateForTest struct {
	health                       uint8
	velocity                     mgl32.Vec3
	attackCooldown, hurtCooldown uint8
}

type combatStateForTest struct {
	players  []combatPlayerStateForTest
	hostiles []combatHostileStateForTest
	hits     []CombatHit
}

func captureCombatStateForTest(engine *Engine, result *TickResult) combatStateForTest {
	state := combatStateForTest{hits: append([]CombatHit(nil), result.CombatHits...)}
	for _, id := range engine.sortedActiveSessions() {
		player := engine.sessions[id].player
		state.players = append(state.players, combatPlayerStateForTest{
			health: player.health, velocity: player.state.Velocity,
			attackCooldown: player.attackCooldownTicks, hurtCooldown: player.hurtCooldownTicks,
			inventory: player.inventory, inventoryDirty: player.inventoryDirty,
			exhaustion:            player.exhaustionMilli,
			meleeSuppressedMining: player.meleeSuppressedMining,
		})
	}
	for index := range engine.hostiles.entries {
		hostile := &engine.hostiles.entries[index]
		state.hostiles = append(state.hostiles, combatHostileStateForTest{
			health: hostile.health, velocity: hostile.state.Velocity,
			attackCooldown: hostile.attackCooldown, hurtCooldown: hostile.hurtCooldown,
		})
	}
	return state
}

func TestCombatSnapshotCapacity(t *testing.T) {
	engine, _ := readyMeleePlayers(t, 8)
	for id := uint64(1); id <= 64; id++ {
		restoreCombatHostile(t, engine, id, mgl32.Vec3{2.5, 1, 2.5})
	}
	if ok := engine.advanceCombatWithLimits(&TickResult{}, 72, 72); !ok {
		t.Fatal("72 个 actor 被错误判为溢出")
	}

	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 1, mgl32.Vec3{2.0, 1, 0.5})
	player := engine.sessions[session].player
	player.attackCooldownTicks, player.hurtCooldownTicks = 4, 5
	player.meleeSuppressedMining = true
	player.state.Velocity = mgl32.Vec3{1, 2, 3}
	hostile := &engine.hostiles.entries[0]
	hostile.attackCooldown, hostile.hurtCooldown = 6, 7
	hostile.state.Velocity = mgl32.Vec3{4, 5, 6}
	result := TickResult{CombatHits: []CombatHit{{Session: 99, Damage: 2, TargetKind: core.CombatTargetPlayer}}}
	want := captureCombatStateForTest(engine, &result)
	want.players[0].meleeSuppressedMining = false
	if ok := engine.advanceCombatWithLimits(&result, 1, 72); ok {
		t.Fatal("第二个 actor 被错误接受")
	}
	if got := captureCombatStateForTest(engine, &result); !reflect.DeepEqual(got, want) {
		t.Fatalf("actor 溢出留下部分副作用\n got=%+v\nwant=%+v", got, want)
	}
}

func TestCombatIntentCapacity(t *testing.T) {
	engine, sessions := combatFullIntentEngine(t)
	if ok := engine.advanceCombatWithLimits(&TickResult{}, 72, 72); !ok {
		t.Fatal("72 条 raw intent 被错误判为溢出")
	}

	engine, sessions = readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 3.1415927)
	for _, id := range sessions {
		player := engine.sessions[id].player
		player.miningHeld = true
		player.meleeSuppressedMining = true
	}
	result := TickResult{CombatHits: []CombatHit{{Session: 99, Damage: 2, TargetKind: core.CombatTargetPlayer}}}
	want := captureCombatStateForTest(engine, &result)
	for index := range want.players {
		want.players[index].meleeSuppressedMining = false
	}
	if ok := engine.advanceCombatWithLimits(&result, 72, 1); ok {
		t.Fatal("第二条 raw intent 被错误接受")
	}
	if got := captureCombatStateForTest(engine, &result); !reflect.DeepEqual(got, want) {
		t.Fatalf("intent 溢出留下部分副作用\n got=%+v\nwant=%+v", got, want)
	}
}

func combatFullIntentEngine(t *testing.T) (*Engine, []SessionID) {
	t.Helper()
	engine, sessions := readyMeleePlayers(t, 8)
	for pair := range 4 {
		x := float32(pair * 4)
		setMeleePlayer(engine, sessions[pair*2], mgl32.Vec3{x + 0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[pair*2+1], mgl32.Vec3{x + 0.5, 1, 2.5}, 3.1415927)
		engine.sessions[sessions[pair*2]].player.miningHeld = true
		engine.sessions[sessions[pair*2+1]].player.miningHeld = true
	}
	for id := uint64(1); id <= 64; id++ {
		restoreCombatHostile(t, engine, id, mgl32.Vec3{2.0, 1, 4.5})
		hostile := &engine.hostiles.entries[engine.hostiles.findIndex(id)]
		hostile.attackIntent = true
		hostile.attackTargetSession = sessions[0]
	}
	return engine, sessions
}

func TestCombatPlayerCooldown(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	attacker := engine.sessions[sessions[0]].player
	target := engine.sessions[sessions[1]].player
	attacker.miningHeld = true

	engine.advanceCombat(&TickResult{})
	if target.health != 18 || attacker.attackCooldownTicks != 10 || target.hurtCooldownTicks != 10 {
		t.Fatalf("首击后 health/attack/hurt=(%d,%d,%d)，想要 (18,10,10)",
			target.health, attacker.attackCooldownTicks, target.hurtCooldownTicks)
	}
	for range 9 {
		engine.advanceCombat(&TickResult{})
	}
	if target.health != 18 {
		t.Fatalf("第 2..10 tick 内重复命中，health=%d", target.health)
	}
	engine.advanceCombat(&TickResult{})
	if target.health != 16 {
		t.Fatalf("第 11 tick health=%d，想要 16", target.health)
	}
}

func TestCombatHostileCooldown(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 1, mgl32.Vec3{2.0, 1, 0.5})
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	hostile := &engine.hostiles.entries[0]
	hostile.attackIntent = true
	hostile.attackTargetSession = session
	target := engine.sessions[session].player

	engine.advanceCombat(&TickResult{})
	if target.health != 17 || hostile.attackCooldown != 20 || target.hurtCooldownTicks != 20 {
		t.Fatalf("首击后 health/attack/hurt=(%d,%d,%d)，想要 (17,20,20)",
			target.health, hostile.attackCooldown, target.hurtCooldownTicks)
	}
	for range 19 {
		engine.advanceCombat(&TickResult{})
	}
	if target.health != 17 {
		t.Fatalf("20 tick 冷却内重复命中，health=%d", target.health)
	}
	engine.advanceCombat(&TickResult{})
	if target.health != 14 {
		t.Fatalf("冷却归零 tick health=%d，想要 14", target.health)
	}
}

func TestCombatCooldownsDecrementTogether(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 1, mgl32.Vec3{2.0, 1, 0.5})
	player := engine.sessions[session].player
	player.attackCooldownTicks, player.hurtCooldownTicks = 2, 2
	hostile := &engine.hostiles.entries[0]
	hostile.attackCooldown, hostile.hurtCooldown = 2, 2
	if ok := engine.advanceCombatWithLimits(&TickResult{}, 72, 72); !ok {
		t.Fatal("合法冷却推进失败")
	}
	if player.attackCooldownTicks != 1 || player.hurtCooldownTicks != 1 ||
		hostile.attackCooldown != 1 || hostile.hurtCooldown != 1 {
		t.Fatalf("四类冷却=(%d,%d,%d,%d)，想要全为 1",
			player.attackCooldownTicks, player.hurtCooldownTicks,
			hostile.attackCooldown, hostile.hurtCooldown)
	}
}

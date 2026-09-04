package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定玩家近战对被动牛的接线：被动牛作为新增受害者种类进入与玩家/
// 夜行者同一套候选冻结与统一结算，经 `DamagePassive` 扣血并触发逃跑，致死
// 走同 tick 移除加 1 生牛肉的既有死亡路径。目标种类值按追加顺序排在既有
// 种类之后，等距仲裁时保既有行为不变。

// restoreCombatPassive 以指定 ID 把一头地面被动牛恢复进集合。
func restoreCombatPassive(t *testing.T, engine *Engine, id uint64, position mgl32.Vec3) {
	t.Helper()
	mob := validTestPassive(id)
	mob.State.Position = position
	if err := engine.RestorePassive(mob); err != nil {
		t.Fatalf("恢复被动牛 %d：%v", id, err)
	}
}

func TestPlayerMeleeHitsPassiveDealsDamageAndFlees(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{10.5, 1, 4.5}, 0)
	restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 2.5})
	attacker := engine.sessions[sessions[0]].player
	attacker.miningHeld = true
	result := TickResult{}

	engine.advanceCombat(&result)

	entry := &engine.passives.entries[engine.passives.findIndex(5)]
	if entry.health != core.MaxHealth-2 {
		t.Fatalf("被动牛 health=%d，想要空手 2 点后的 %d", entry.health, core.MaxHealth-2)
	}
	if entry.fleeTicks != passiveFleeDurationTicks {
		t.Fatalf("被动牛 fleeTicks=%d，想要受击逃跑的 %d", entry.fleeTicks, passiveFleeDurationTicks)
	}
	if entry.fleeFrom != attacker.state.Position {
		t.Fatalf("被动牛 fleeFrom=%v，想要攻击者位置 %v", entry.fleeFrom, attacker.state.Position)
	}
	if attacker.attackCooldownTicks != playerMeleeCooldownTicks {
		t.Fatalf("攻击者 attack cooldown=%d，想要 %d", attacker.attackCooldownTicks, playerMeleeCooldownTicks)
	}
	if attacker.exhaustionMilli != 100 || !attacker.meleeSuppressedMining {
		t.Fatalf("命中 side effects=(fatigue %d, suppressed %v)，想要 (100,true)",
			attacker.exhaustionMilli, attacker.meleeSuppressedMining)
	}
	if entry.state.Velocity.Len() < 0.34 || entry.state.Velocity.Y() != 0 {
		t.Fatalf("被动牛击退 velocity=%v，想要水平约 0.35", entry.state.Velocity)
	}
	if len(result.CombatHits) != 1 || result.CombatHits[0] != (CombatHit{
		Session: sessions[0], Damage: 2, TargetKind: core.CombatTargetPassive,
	}) {
		t.Fatalf("CombatHits=%+v，想要一次被动牛命中", result.CombatHits)
	}
}

func TestPlayerMeleePassiveReusesWeaponDamageTable(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.0}, 0)
	restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 2.5})
	attacker := engine.sessions[session].player
	attacker.inventory.Hotbar.Slots[0] = core.ItemStack{
		Item: core.ItemIronSword, Count: 1, Durability: 2,
	}
	attacker.inventoryDirty = false
	attacker.miningHeld = true
	result := TickResult{}

	engine.advanceCombat(&result)

	entry := &engine.passives.entries[engine.passives.findIndex(5)]
	if entry.health != core.MaxHealth-6 {
		t.Fatalf("被动牛 health=%d，想要铁剑 6 点后的 %d", entry.health, core.MaxHealth-6)
	}
	if got := attacker.inventory.Hotbar.Slots[0].Durability; got != 1 {
		t.Fatalf("铁剑 durability=%d，想要 1", got)
	}
	if len(result.CombatHits) != 1 || result.CombatHits[0].TargetKind != core.CombatTargetPassive {
		t.Fatalf("被动牛 CombatHits=%+v", result.CombatHits)
	}
}

func TestPlayerMeleePassiveRanksAfterExistingKinds(t *testing.T) {
	t.Run("等距玩家优先于被动牛", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 2.5})
		engine.sessions[sessions[0]].player.miningHeld = true

		engine.advanceCombat(&TickResult{})

		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("等距玩家 health=%d，想要 18", got)
		}
		entry := &engine.passives.entries[engine.passives.findIndex(5)]
		if entry.health != core.MaxHealth {
			t.Fatalf("等距被动牛 health=%d，想要保持 %d", entry.health, core.MaxHealth)
		}
	})

	t.Run("等距夜行者优先于被动牛", func(t *testing.T) {
		engine, session := hostileCombatEngine(t)
		setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.5}, 0)
		restoreCombatHostile(t, engine, 9, mgl32.Vec3{0.5, 1, 2.5})
		restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 2.5})
		engine.sessions[session].player.miningHeld = true

		engine.advanceCombat(&TickResult{})

		if got := engine.hostiles.entries[0].health; got != 18 {
			t.Fatalf("等距夜行者 health=%d，想要 18", got)
		}
		entry := &engine.passives.entries[engine.passives.findIndex(5)]
		if entry.health != core.MaxHealth {
			t.Fatalf("等距被动牛 health=%d，想要保持 %d", entry.health, core.MaxHealth)
		}
	})

	t.Run("更近夜行者优先于更远被动牛", func(t *testing.T) {
		engine, session := hostileCombatEngine(t)
		setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.5}, 0)
		restoreCombatHostile(t, engine, 9, mgl32.Vec3{0.5, 1, 2.5})
		restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 1.5})
		engine.sessions[session].player.miningHeld = true

		engine.advanceCombat(&TickResult{})

		if got := engine.hostiles.entries[0].health; got != 18 {
			t.Fatalf("更近夜行者 health=%d，想要 18", got)
		}
		entry := &engine.passives.entries[engine.passives.findIndex(5)]
		if entry.health != core.MaxHealth {
			t.Fatalf("更远被动牛 health=%d，想要保持 %d", entry.health, core.MaxHealth)
		}
	})

	t.Run("更近被动牛优先于更远夜行者", func(t *testing.T) {
		engine, session := hostileCombatEngine(t)
		setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.5}, 0)
		restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 2.5})
		restoreCombatHostile(t, engine, 9, mgl32.Vec3{0.5, 1, 1.5})
		engine.sessions[session].player.miningHeld = true

		engine.advanceCombat(&TickResult{})

		entry := &engine.passives.entries[engine.passives.findIndex(5)]
		if entry.health != core.MaxHealth-2 {
			t.Fatalf("更近被动牛 health=%d，想要 %d", entry.health, core.MaxHealth-2)
		}
		if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
			t.Fatalf("更远夜行者 health=%d，想要保持 %d", got, core.MaxHealth)
		}
	})
}

func TestPlayerMeleeSharedPassiveVictimReservedOnce(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 0.5}, math.Pi)
	restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 2.5})
	winner := engine.sessions[sessions[0]].player
	loser := engine.sessions[sessions[1]].player
	winner.miningHeld = true
	loser.miningHeld = true
	loser.inventoryDirty = false
	result := TickResult{}

	engine.advanceCombat(&result)

	entry := &engine.passives.entries[engine.passives.findIndex(5)]
	if entry.health != core.MaxHealth-2 {
		t.Fatalf("被动牛 health=%d，想要只受一次 2 点后的 %d", entry.health, core.MaxHealth-2)
	}
	if winner.attackCooldownTicks != playerMeleeCooldownTicks {
		t.Fatalf("先结算者 attack cooldown=%d，想要 %d",
			winner.attackCooldownTicks, playerMeleeCooldownTicks)
	}
	if loser.attackCooldownTicks != 0 || loser.exhaustionMilli != 0 ||
		loser.meleeSuppressedMining || loser.inventoryDirty || len(result.CombatHits) != 1 {
		t.Fatalf("reservation loser 留下副作用: attack=%d fatigue=%d suppressed=%v dirty=%v hits=%+v",
			loser.attackCooldownTicks, loser.exhaustionMilli, loser.meleeSuppressedMining,
			loser.inventoryDirty, result.CombatHits)
	}
}

func TestPlayerMeleeLethalHitSettlesPassiveDeathSameTick(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.0}, 0)
	restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 2.5})
	// 预置到空手一击致死：与直接恢复满血牛再连击多轮等价，但只消耗一个生产 tick。
	if !engine.DamagePassive(5, int32(core.MaxHealth-2), mgl32.Vec3{0.5, 1, 4.0}) {
		t.Fatal("预置伤害被拒绝")
	}
	engine.sessions[session].player.miningHeld = true

	tick := engine.beginTick()
	tick.context.AdvanceHostiles(nil, &tick.result)
	if index := tick.context.engine.passives.findIndex(5); index < 0 ||
		tick.context.engine.passives.entries[index].health != 0 {
		t.Fatal("近战阶段后被动牛未进入死亡待结算（生命未归零）")
	}
	tick.context.AdvancePassives()

	if len(tick.context.engine.passives.entries) != 0 {
		t.Fatal("同 tick 死亡结算后被动牛仍在集合中")
	}
	if got := countLoadedDrops(t, engine, core.ItemRawBeef); got != 1 {
		t.Fatalf("死亡掉落生牛肉=%d，想要恰好 1", got)
	}
}

func TestSettlePassiveVictimRejectsUnknownIDWithoutSideEffects(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	attacker := engine.sessions[sessions[0]].player
	target := engine.sessions[sessions[1]].player
	attacker.miningHeld = true
	result := TickResult{}
	intent := combatIntent{
		attacker:  combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[0])},
		target:    combatActor{kind: core.CombatTargetPassive, id: 999},
		dimension: core.Overworld, damage: 2,
		attackerPosition: attacker.state.Position, targetPosition: target.state.Position,
	}

	if engine.settleCombatIntent(&result, intent) {
		t.Fatal("未知被动牛 ID 的 intent 被接受")
	}
	if target.health != core.MaxHealth || attacker.attackCooldownTicks != 0 ||
		attacker.exhaustionMilli != 0 || attacker.meleeSuppressedMining || len(result.CombatHits) != 0 {
		t.Fatalf("非法被动牛目标留下副作用: target=%d attack=%d fatigue=%d suppressed=%v hits=%+v",
			target.health, attacker.attackCooldownTicks, attacker.exhaustionMilli,
			attacker.meleeSuppressedMining, result.CombatHits)
	}
}

func TestSettlePassiveVictimRejectsHostileAttacker(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.5}, 0)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{0.5, 1, 2.5})
	restoreCombatPassive(t, engine, 5, mgl32.Vec3{0.5, 1, 1.5})
	attacker := &engine.hostiles.entries[0]
	before := engine.passives.entries[engine.passives.findIndex(5)]
	result := TickResult{}
	intent := combatIntent{
		attacker:  combatActor{kind: core.CombatTargetHostile, id: attacker.id},
		target:    combatActor{kind: core.CombatTargetPassive, id: 5},
		dimension: core.Overworld, damage: hostileMeleeDamage,
		attackerPosition: attacker.state.Position, targetPosition: before.state.Position,
	}

	if engine.settleCombatIntent(&result, intent) {
		t.Fatal("夜行者攻击被动牛的 intent 被接受")
	}
	after := engine.passives.entries[engine.passives.findIndex(5)]
	if after != before || attacker.attackCooldown != 0 || len(result.CombatHits) != 0 {
		t.Fatalf("非法夜行者攻击留下副作用: passive=%+v attack=%d hits=%+v",
			after, attacker.attackCooldown, result.CombatHits)
	}
}

func TestCombatActorCapacityCoversFullHouseWithPassives(t *testing.T) {
	engine, _ := readyMeleePlayers(t, 8)
	for id := uint64(1); id <= 64; id++ {
		restoreCombatHostile(t, engine, id, mgl32.Vec3{2.5, 1, 2.5})
	}
	for id := uint64(1); id <= 32; id++ {
		restoreCombatPassive(t, engine, id, mgl32.Vec3{2.5, 1, 2.5})
	}
	// 8 玩家加 64 夜行者加 32 被动牛共 104 个参战者：满编仍须一次走完战斗阶段。
	if !engine.advanceCombatWithLimits(&TickResult{}, maxCombatActors, maxCombatIntents) {
		t.Fatal("满编 104 个参战者被判为溢出，想要一次结算")
	}
	// 被动牛占用参战者快照预算：旧 72 上限下满编溢出失败，证明新增快照真实进入冻结管线。
	if engine.advanceCombatWithLimits(&TickResult{}, 72, 72) {
		t.Fatal("104 个参战者在 72 上限下被接受，想要溢出失败")
	}
}

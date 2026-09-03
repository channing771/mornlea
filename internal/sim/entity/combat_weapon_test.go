package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

func TestPlayerCombatWeaponDamageAndDurability(t *testing.T) {
	for _, test := range []struct {
		name       string
		stack      core.ItemStack
		wantStack  core.ItemStack
		wantDamage uint8
		wantDirty  bool
	}{
		{name: "空手", wantDamage: 2},
		{
			name: "普通物品", stack: core.ItemStack{Item: core.ItemStone, Count: 3},
			wantStack: core.ItemStack{Item: core.ItemStone, Count: 3}, wantDamage: 2,
		},
		{
			name: "损坏剑", stack: core.ItemStack{Item: core.ItemBrokenIronSword, Count: 1},
			wantStack: core.ItemStack{Item: core.ItemBrokenIronSword, Count: 1}, wantDamage: 2,
		},
		{
			name: "木剑", stack: core.ItemStack{Item: core.ItemWoodenSword, Count: 1, Durability: 10},
			wantStack:  core.ItemStack{Item: core.ItemWoodenSword, Count: 1, Durability: 9},
			wantDamage: 4, wantDirty: true,
		},
		{
			name: "石剑", stack: core.ItemStack{Item: core.ItemStoneSword, Count: 1, Durability: 10},
			wantStack:  core.ItemStack{Item: core.ItemStoneSword, Count: 1, Durability: 9},
			wantDamage: 5, wantDirty: true,
		},
		{
			name: "铁剑", stack: core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 10},
			wantStack:  core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 9},
			wantDamage: 6, wantDirty: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions := readyMeleePlayers(t, 2)
			setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
			setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
			attacker := engine.sessions[sessions[0]].player
			target := engine.sessions[sessions[1]].player
			attacker.inventory.Hotbar.Selected = 0
			attacker.inventory.Hotbar.Slots[0] = test.stack
			attacker.inventoryDirty = false
			attacker.miningHeld = true
			result := TickResult{}

			engine.advanceCombat(&result)
			if got := target.health; got != core.MaxHealth-test.wantDamage {
				t.Fatalf("目标 health=%d，想要 %d", got, core.MaxHealth-test.wantDamage)
			}
			if got := attacker.inventory.Hotbar.Slots[0]; got != test.wantStack {
				t.Fatalf("命中后 stack=%+v，想要 %+v", got, test.wantStack)
			}
			if attacker.inventoryDirty != test.wantDirty {
				t.Fatalf("inventoryDirty=%v，想要 %v", attacker.inventoryDirty, test.wantDirty)
			}
			if attacker.exhaustionMilli != 100 || !attacker.meleeSuppressedMining {
				t.Fatalf("命中 side effects=(fatigue %d, suppressed %v)，想要 (100,true)",
					attacker.exhaustionMilli, attacker.meleeSuppressedMining)
			}
			if len(result.CombatHits) != 1 || result.CombatHits[0] != (CombatHit{
				Session: sessions[0], Damage: test.wantDamage, TargetKind: core.CombatTargetPlayer,
			}) {
				t.Fatalf("CombatHits=%+v，想要一次 player hit", result.CombatHits)
			}
		})
	}
}

func TestPlayerCombatWeaponLastDurabilityBreaksAfterDamage(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	attacker := engine.sessions[sessions[0]].player
	target := engine.sessions[sessions[1]].player
	attacker.inventory.Hotbar.Slots[0] = core.ItemStack{
		Item: core.ItemIronSword, Count: 1, Durability: 1,
	}
	attacker.inventoryDirty = false
	attacker.miningHeld = true

	engine.advanceCombat(&TickResult{})
	if target.health != 14 {
		t.Fatalf("最后一点铁剑造成 damage=%d，想要 6", core.MaxHealth-target.health)
	}
	want := core.ItemStack{Item: core.ItemBrokenIronSword, Count: 1}
	if got := attacker.inventory.Hotbar.Slots[0]; got != want || !attacker.inventoryDirty {
		t.Fatalf("最后一点命中后 stack/dirty=(%+v,%v)，想要 (%+v,true)",
			got, attacker.inventoryDirty, want)
	}
}

func TestPlayerCombatWeaponDamagesHostile(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.0}, 0)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{0.5, 1, 2.5})
	attacker := engine.sessions[session].player
	attacker.inventory.Hotbar.Slots[0] = core.ItemStack{
		Item: core.ItemIronSword, Count: 1, Durability: 2,
	}
	attacker.inventoryDirty = false
	attacker.miningHeld = true
	result := TickResult{}

	engine.advanceCombat(&result)
	if got := engine.hostiles.entries[0].health; got != 14 {
		t.Fatalf("hostile health=%d，想要 14", got)
	}
	if got := attacker.inventory.Hotbar.Slots[0].Durability; got != 1 {
		t.Fatalf("铁剑 durability=%d，想要 1", got)
	}
	if len(result.CombatHits) != 1 || result.CombatHits[0].TargetKind != core.CombatTargetHostile {
		t.Fatalf("hostile CombatHits=%+v", result.CombatHits)
	}
}

func TestPlayerCombatWeaponRejectedPathsHaveNoSideEffects(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Engine, *playerState, *playerState)
	}{
		{
			name: "挥空",
			mutate: func(_ *Engine, _ *playerState, target *playerState) {
				target.state.Position = mgl32.Vec3{0.5, 1, 6.5}
			},
		},
		{
			name: "遮挡",
			mutate: func(engine *Engine, _ *playerState, _ *playerState) {
				engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.StoneID)
			},
		},
		{
			name: "超距",
			mutate: func(_ *Engine, _ *playerState, target *playerState) {
				target.state.Position = mgl32.Vec3{0.5, 1, 0.5}
			},
		},
		{
			name: "攻击冷却",
			mutate: func(_ *Engine, attacker, _ *playerState) {
				attacker.attackCooldownTicks = 2
			},
		},
		{
			name: "目标保护",
			mutate: func(_ *Engine, _ *playerState, target *playerState) {
				target.hurtCooldownTicks = 2
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, sessions := readyMeleePlayers(t, 2)
			setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
			setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
			attacker := engine.sessions[sessions[0]].player
			target := engine.sessions[sessions[1]].player
			attacker.inventory.Hotbar.Slots[0] = core.ItemStack{
				Item: core.ItemIronSword, Count: 1, Durability: 10,
			}
			attacker.inventoryDirty = false
			attacker.miningHeld = true
			test.mutate(engine, attacker, target)
			beforeStack := attacker.inventory.Hotbar.Slots[0]
			result := TickResult{}

			engine.advanceCombat(&result)
			if target.health != core.MaxHealth || attacker.inventory.Hotbar.Slots[0] != beforeStack ||
				attacker.inventoryDirty || attacker.exhaustionMilli != 0 ||
				attacker.meleeSuppressedMining || len(result.CombatHits) != 0 {
				t.Fatalf("拒绝路径留下副作用: health=%d stack=%+v dirty=%v fatigue=%d suppressed=%v hits=%+v",
					target.health, attacker.inventory.Hotbar.Slots[0], attacker.inventoryDirty,
					attacker.exhaustionMilli, attacker.meleeSuppressedMining, result.CombatHits)
			}
		})
	}
}

func TestPlayerCombatWeaponFrozenSlotMismatchFailsClosed(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	attacker := engine.sessions[sessions[0]].player
	target := engine.sessions[sessions[1]].player
	attacker.inventory.Hotbar.Slots[0] = core.ItemStack{
		Item: core.ItemIronSword, Count: 1, Durability: 10,
	}
	attacker.inventory.Hotbar.Selected = 1
	attacker.inventoryDirty = false
	intent := combatIntent{
		attacker: combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[0])},
		target:   combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[1])},
		damage:   6, selectedSlot: 0, selectedItem: core.ItemIronSword,
	}
	beforeStack := attacker.inventory.Hotbar.Slots[0]
	result := TickResult{}

	if engine.settleCombatIntent(&result, intent) {
		t.Fatal("冻结栏位已变化的 intent 被接受")
	}
	if target.health != core.MaxHealth || attacker.attackCooldownTicks != 0 || target.hurtCooldownTicks != 0 ||
		attacker.inventory.Hotbar.Slots[0] != beforeStack || attacker.inventoryDirty ||
		attacker.exhaustionMilli != 0 || attacker.meleeSuppressedMining || len(result.CombatHits) != 0 {
		t.Fatalf("冻结栏位失配留下副作用: target=%d attack/hurt=(%d,%d) stack=%+v dirty=%v fatigue=%d suppressed=%v hits=%+v",
			target.health, attacker.attackCooldownTicks, target.hurtCooldownTicks,
			attacker.inventory.Hotbar.Slots[0], attacker.inventoryDirty,
			attacker.exhaustionMilli, attacker.meleeSuppressedMining, result.CombatHits)
	}
}

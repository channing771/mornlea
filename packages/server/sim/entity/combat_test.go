package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestRayAABBDistance 覆盖近战射线进入玩家包围盒的边界：回退方向、平行轴和
// 起点已在盒内都不能被 slab 计算误判成一次前向命中。
func TestRayAABBDistance(t *testing.T) {
	bounds := core.AABB{Min: mgl32.Vec3{1, -1, -1}, Max: mgl32.Vec3{2, 1, 1}}
	for _, test := range []struct {
		name              string
		origin, direction mgl32.Vec3
		want              float32
		hit               bool
	}{
		{"正向命中", mgl32.Vec3{}, mgl32.Vec3{1, 0, 0}, 1, true},
		{"反向不命中", mgl32.Vec3{}, mgl32.Vec3{-1, 0, 0}, 0, false},
		{"超出三格", mgl32.Vec3{-3, 0, 0}, mgl32.Vec3{1, 0, 0}, 0, false},
		{"平行轴在盒外", mgl32.Vec3{0, 2, 0}, mgl32.Vec3{1, 0, 0}, 0, false},
		{"起点在盒内", mgl32.Vec3{1.5, 0, 0}, mgl32.Vec3{1, 0, 0}, 0, true},
		{"边界恰为三格", mgl32.Vec3{-2, 0, 0}, mgl32.Vec3{1, 0, 0}, 3, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, hit := rayAABBDistance(test.origin, test.direction, bounds)
			if hit != test.hit || hit && math.Abs(float64(got-test.want)) > 1e-6 {
				t.Fatalf("rayAABBDistance() = (%v, %v)，想要 (%v, %v)", got, hit, test.want, test.hit)
			}
		})
	}
}

// TestPlayerMeleeHeldResolvesDamageCooldownAndRelease 覆盖持续 primary action 的
// 首次命中、目标冷却与松手三个可观察边界。
func TestPlayerMeleeHeldResolvesDamageCooldownAndRelease(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	attacker := engine.sessions[sessions[0]].player
	target := engine.sessions[sessions[1]].player
	attacker.miningHeld = true

	advanceHostilesTick(engine, nil)
	if got := target.health; got != 18 {
		t.Fatalf("首 tick health=%d，想要 18", got)
	}
	for range 9 {
		advanceHostilesTick(engine, nil)
	}
	if got := target.health; got != 18 {
		t.Fatalf("冷却内 health=%d，想要 18", got)
	}
	advanceHostilesTick(engine, nil)
	if got := target.health; got != 16 {
		t.Fatalf("第十个间隔 tick health=%d，想要 16", got)
	}
	attacker.miningHeld = false
	advanceHostilesTick(engine, nil)
	if got := target.health; got != 16 {
		t.Fatalf("松手后 health=%d，想要保持 16", got)
	}
}

// TestPlayerMeleeSimultaneousLethalIntents 覆盖同 tick 的所有意图必须在死亡结算
// 之前收集：双方都只有两点生命时，相向 primary action 必须让双方都进入重生。
func TestPlayerMeleeSimultaneousLethalIntents(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, math.Pi)
	for _, id := range sessions {
		player := engine.sessions[id].player
		player.health = 2
		player.miningHeld = true
	}

	result := advanceHostilesTick(engine, nil)
	for _, id := range sessions {
		player := engine.sessions[id].player
		if player.health != core.MaxHealth || player.lifecycle != PlayerPendingSpawn {
			t.Fatalf("session %d 死亡结算后 (health, lifecycle)=(%d, %d)，想要 (%d, %d)",
				id, player.health, player.lifecycle, core.MaxHealth, PlayerPendingSpawn)
		}
	}
	for _, update := range result.Players {
		if update.Health == 0 {
			t.Fatalf("发布了 health 0: %+v", update)
		}
	}
}

// TestPlayerMeleeCooldownBelongsToTarget 覆盖冷却不因换一名攻击者而失效。
func TestPlayerMeleeCooldownBelongsToTarget(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 3)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	setMeleePlayer(engine, sessions[2], mgl32.Vec3{10.5, 1, 4.5}, 0)
	engine.sessions[sessions[0]].player.miningHeld = true

	advanceHostilesTick(engine, nil)
	target := engine.sessions[sessions[1]].player
	if target.health != 18 {
		t.Fatalf("首次命中 health=%d，想要 18", target.health)
	}
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{10.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 4.5}, 0)
	engine.sessions[sessions[0]].player.miningHeld = false
	engine.sessions[sessions[2]].player.miningHeld = true

	advanceHostilesTick(engine, nil)
	if target.health != 18 {
		t.Fatalf("换攻击者穿透目标冷却 health=%d，想要 18", target.health)
	}
}

// TestPlayerMeleeEightPlayersResolveOneIntentEach 覆盖满员时意图使用 SessionID
// 稳定顺序收集，且每名攻击者在同一 tick 至多写入一条意图。
func TestPlayerMeleeEightPlayersResolveOneIntentEach(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 8)
	for pair := range 4 {
		x := float32(pair * 4)
		setMeleePlayer(engine, sessions[pair*2], mgl32.Vec3{x + 0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[pair*2+1], mgl32.Vec3{x + 0.5, 1, 2.5}, math.Pi)
		engine.sessions[sessions[pair*2]].player.miningHeld = true
		engine.sessions[sessions[pair*2+1]].player.miningHeld = true
	}

	advanceHostilesTick(engine, nil)
	for _, id := range sessions {
		if got := engine.sessions[id].player.health; got != 18 {
			t.Fatalf("session %d health=%d，想要每人只受一次 2 点伤害后的 18", id, got)
		}
	}
}

func TestPlayerCombatTargetOrdering(t *testing.T) {
	t.Run("距离优先", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 1.5}, 0)
		restoreCombatHostile(t, engine, 9, mgl32.Vec3{0.5, 1, 2.5})
		engine.sessions[sessions[0]].player.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.hostiles.entries[0].health; got != 18 {
			t.Fatalf("更近 hostile health=%d，想要 18", got)
		}
		if got := engine.sessions[sessions[1]].player.health; got != 20 {
			t.Fatalf("更远 player health=%d，想要 20", got)
		}
	})

	t.Run("等距 kind 优先", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		restoreCombatHostile(t, engine, 1, mgl32.Vec3{0.5, 1, 2.5})
		engine.sessions[sessions[0]].player.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("等距 player health=%d，想要 18", got)
		}
		if got := engine.hostiles.entries[0].health; got != 20 {
			t.Fatalf("等距 hostile health=%d，想要 20", got)
		}
	})

	t.Run("player 同 kind 最小 ID", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 3)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.sessions[sessions[0]].player.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("较小 SessionID health=%d，想要 18", got)
		}
		if got := engine.sessions[sessions[2]].player.health; got != 20 {
			t.Fatalf("较大 SessionID health=%d，想要 20", got)
		}
	})

	t.Run("hostile 同 kind 最小 ID", func(t *testing.T) {
		engine, session := readyMovementPlayer(t)
		setMeleePlayer(engine, session, mgl32.Vec3{0.5, 1, 4.5}, 0)
		restoreCombatHostile(t, engine, 7, mgl32.Vec3{0.5, 1, 2.5})
		restoreCombatHostile(t, engine, 3, mgl32.Vec3{0.5, 1, 2.5})
		engine.sessions[session].player.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.hostiles.entries[0].health; got != 18 {
			t.Fatalf("较小 hostile ID health=%d，想要 18", got)
		}
		if got := engine.hostiles.entries[1].health; got != 20 {
			t.Fatalf("较大 hostile ID health=%d，想要 20", got)
		}
	})
}

func TestPlayerCombatRaySemantics(t *testing.T) {
	t.Run("非零 pitch", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 3, 2.5}, 0)
		attacker := engine.sessions[sessions[0]].player
		attacker.pitch = 0.35
		attacker.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("抬头射线未命中，health=%d", got)
		}
	})

	t.Run("固体严格在前阻挡", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.StoneID)
		engine.sessions[sessions[0]].player.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 20 {
			t.Fatalf("固体前遮挡后 health=%d，想要 20", got)
		}
	})

	t.Run("表面等距不阻挡", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.7}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 2}, core.StoneID)
		engine.sessions[sessions[0]].player.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("表面等距被错误阻挡，health=%d", got)
		}
	})

	t.Run("流体透明", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.WaterSourceID)
		engine.sessions[sessions[0]].player.miningHeld = true

		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("流体错误阻挡，health=%d", got)
		}
	})
}

// TestPlayerMeleeTarget 保留玩家近战目标选择的稳定入口。六个既有标签逐项
// 驱动现行统一战斗结算，既覆盖玩家候选，也证明伙伴不会成为战斗目标。
func TestPlayerMeleeTarget(t *testing.T) {
	t.Run("选择最近且按会话平局", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 3)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 1.5}, 0)
		engine.sessions[sessions[0]].player.miningHeld = true
		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("最近目标 health=%d，想要 18", got)
		}
		if got := engine.sessions[sessions[2]].player.health; got != 20 {
			t.Fatalf("较远目标 health=%d，想要 20", got)
		}

		engine, sessions = readyMeleePlayers(t, 3)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.sessions[sessions[0]].player.miningHeld = true
		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("等距较小 SessionID health=%d，想要 18", got)
		}
		if got := engine.sessions[sessions[2]].player.health; got != 20 {
			t.Fatalf("等距较大 SessionID health=%d，想要 20", got)
		}
	})

	t.Run("排除非候选与伙伴", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 3)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.sessions[sessions[1]].player.lifecycle = PlayerPendingSpawn
		engine.sessions[sessions[2]].dimension = core.DimensionID(1)
		companionID := companionTestID(1)
		engine.RegisterCompanion(CompanionRestore{
			ID: companionID,
			Body: &companion.Body{
				ID: companionID, Dimension: core.Overworld,
				Position: [3]float32{0.5, 1, 3.5},
			},
			SpawnDimension: core.Overworld,
		})
		engine.sessions[sessions[0]].player.miningHeld = true
		result := TickResult{}
		engine.advanceCombat(&result)
		if len(result.CombatHits) != 0 {
			t.Fatalf("非候选玩家或伙伴产生战斗命中：%+v", result.CombatHits)
		}
	})

	t.Run("排除死亡与超距", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 3)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, -1}, 0)
		engine.sessions[sessions[1]].player.health = 0
		engine.sessions[sessions[0]].player.miningHeld = true
		result := TickResult{}
		engine.advanceCombat(&result)
		if len(result.CombatHits) != 0 {
			t.Fatalf("死亡或超距玩家产生战斗命中：%+v", result.CombatHits)
		}
	})

	t.Run("固体阻挡而同距方块不阻挡", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.StoneID)
		engine.sessions[sessions[0]].player.miningHeld = true
		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 20 {
			t.Fatalf("被石块遮挡的目标 health=%d，想要 20", got)
		}

		engine, sessions = readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.7}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 2}, core.StoneID)
		engine.sessions[sessions[0]].player.miningHeld = true
		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("同距方块错误阻挡，health=%d，想要 18", got)
		}
	})

	t.Run("流体穿透", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.WaterSourceID)
		engine.sessions[sessions[0]].player.miningHeld = true
		engine.advanceCombat(&TickResult{})
		if got := engine.sessions[sessions[1]].player.health; got != 18 {
			t.Fatalf("流体错误阻挡，health=%d，想要 18", got)
		}
	})

	t.Run("未就绪方块拒绝本 tick", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{15.5, 1, 0.5}, -math.Pi/2)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{17.5, 1, 0.5}, 0)
		engine.sessions[sessions[0]].player.miningHeld = true
		result := TickResult{}
		engine.advanceCombat(&result)
		if len(result.CombatHits) != 0 || engine.sessions[sessions[1]].player.health != 20 {
			t.Fatalf("未就绪区块仍产生战斗命中：hits=%+v health=%d",
				result.CombatHits, engine.sessions[sessions[1]].player.health)
		}
	})
}

func TestPlayerCombatProtectedNearestTarget(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 1.5}, 0)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{0.5, 1, 2.5})
	engine.hostiles.entries[0].hurtCooldown = 2
	engine.sessions[sessions[0]].player.miningHeld = true

	engine.advanceCombat(&TickResult{})
	if got := engine.hostiles.entries[0].health; got != 20 {
		t.Fatalf("受保护最近 hostile health=%d，想要 20", got)
	}
	if got := engine.sessions[sessions[1]].player.health; got != 20 {
		t.Fatalf("攻击穿透到后方 player，health=%d", got)
	}
}

func TestPlayerCombatFrozenIntent(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 1.5}, 0)
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{0.5, 1, 2.5})
	attacker := engine.sessions[sessions[0]].player
	attacker.inventory.Hotbar.Selected = 0
	attacker.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	snapshots := []combatActorSnapshot{
		{
			actor:     combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[0])},
			dimension: core.Overworld, position: mgl32.Vec3{0.5, 1, 4.5},
			health: 20, attacking: true, selectedSlot: 0, selectedItem: core.ItemStone,
		},
		{
			actor:     combatActor{kind: core.CombatTargetPlayer, id: uint64(sessions[1])},
			dimension: core.Overworld, position: mgl32.Vec3{0.5, 1, 1.5}, health: 20,
		},
		{
			actor:     combatActor{kind: core.CombatTargetHostile, id: 3},
			dimension: core.Overworld, position: mgl32.Vec3{0.5, 1, 2.5}, health: 20,
		},
	}

	attacker.state.Position = mgl32.Vec3{10, 10, 10}
	attacker.yaw, attacker.pitch, attacker.health = 1, 1, 1
	attacker.inventory.Hotbar.Selected = 1
	attacker.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 250}
	engine.sessions[sessions[1]].player.state.Position = mgl32.Vec3{12, 12, 12}
	engine.sessions[sessions[1]].player.health = 1
	engine.hostiles.entries[0].state.Position = mgl32.Vec3{14, 14, 14}
	engine.hostiles.entries[0].health = 1

	intent, ok := engine.playerCombatIntent(&snapshots[0], snapshots)
	if !ok {
		t.Fatal("冻结 snapshot 未形成 intent")
	}
	if intent.target != (combatActor{kind: core.CombatTargetHostile, id: 3}) {
		t.Fatalf("冻结目标=%+v，想要 hostile 3", intent.target)
	}
	if math.Abs(float64(intent.distance-1.7)) > 1e-6 {
		t.Fatalf("冻结距离=%v，想要 1.7", intent.distance)
	}
	if intent.damage != 2 || intent.selectedSlot != 0 || intent.selectedItem != core.ItemStone {
		t.Fatalf("冻结伤害/栏位身份=(%d,%d,%d)，想要 (2,0,%d)",
			intent.damage, intent.selectedSlot, intent.selectedItem, core.ItemStone)
	}
}

func TestCombatHitsAreSortedAndLimitedPerSession(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 4)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{4.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 2.5}, 0)
	setMeleePlayer(engine, sessions[3], mgl32.Vec3{4.5, 1, 2.5}, 0)
	engine.sessions[sessions[0]].player.miningHeld = true
	engine.sessions[sessions[1]].player.miningHeld = true

	result := advanceHostilesTick(engine, nil)
	want := []CombatHit{
		{Session: sessions[0], Damage: 2, TargetKind: core.CombatTargetPlayer},
		{Session: sessions[1], Damage: 2, TargetKind: core.CombatTargetPlayer},
	}
	if len(result.CombatHits) != len(want) {
		t.Fatalf("CombatHits=%+v，想要 %+v", result.CombatHits, want)
	}
	for index := range want {
		if result.CombatHits[index] != want[index] {
			t.Fatalf("CombatHits[%d]=%+v，想要 %+v", index, result.CombatHits[index], want[index])
		}
	}
}

func TestCombatHitsExcludeHostileAttacks(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{2.0, 1, 0.5})
	action := HostileAction{
		ID: 3, AttackTarget: true, TargetSession: session,
	}
	if !validHostileAction(action) {
		t.Fatal("hostile attack intent 不是合法 owner 输入")
	}

	result := advanceHostilesTick(engine, []HostileAction{action})
	if len(result.CombatHits) != 0 {
		t.Fatalf("hostile attack 产生 CombatHits=%+v", result.CombatHits)
	}
	if got := engine.sessions[session].player.health; got != core.MaxHealth-3 {
		t.Fatalf("hostile attack health=%d，想要 %d", got, core.MaxHealth-3)
	}
}

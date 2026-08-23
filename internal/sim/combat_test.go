package sim

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
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

// TestPlayerMeleeTarget 覆盖候选过滤、距离/SessionID 裁决与真实方块射线阻挡。
func TestPlayerMeleeTarget(t *testing.T) {
	t.Run("选择最近且按会话平局", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 8)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 1.5}, 0)

		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); !ok || got != sessions[1] {
			t.Fatalf("最近目标 = (%d, %v)，想要 (%d, true)", got, ok, sessions[1])
		}

		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 2.5}, 0)
		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); !ok || got != sessions[1] {
			t.Fatalf("等距目标 = (%d, %v)，想要较小 SessionID (%d, true)", got, ok, sessions[1])
		}
	})

	t.Run("排除非候选与伙伴", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 3)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.sessions[sessions[1]].player.lifecycle = PlayerPendingSpawn
		engine.sessions[sessions[2]].dimension = core.DimensionID(1)
		engine.dimensions[core.DimensionID(1)] = NewDimension(core.DimensionID(1))

		companionID := companion.ID{1}
		engine.companions[companionID] = &companionState{
			id:         companionID,
			active:     true,
			dimension:  core.Overworld,
			actorState: actorState{state: physics.State{Position: mgl32.Vec3{0.5, 1, 3.5}}},
		}

		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); ok {
			t.Fatalf("非候选/伙伴被选为目标: %d", got)
		}
	})

	t.Run("排除死亡与超距", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 3)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, -1}, 0)
		engine.sessions[sessions[1]].player.health = 0

		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); ok {
			t.Fatalf("死亡或超距玩家被选为目标: %d", got)
		}
	})

	t.Run("固体阻挡而同距方块不阻挡", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.StoneID)

		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); ok {
			t.Fatalf("被石块遮挡仍选中目标: %d", got)
		}

		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.AirID)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.7}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 2}, core.StoneID)
		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); !ok || got != sessions[1] {
			t.Fatalf("同距方块错误阻挡: (%d, %v)", got, ok)
		}
	})

	t.Run("流体穿透", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.WaterSourceID)

		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); !ok || got != sessions[1] {
			t.Fatalf("流体错误阻挡: (%d, %v)", got, ok)
		}
	})

	t.Run("未就绪方块拒绝本 tick", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{15.5, 1, 0.5}, -math.Pi/2)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{17.5, 1, 0.5}, 0)

		if got, ok := engine.playerMeleeTarget(sessions[0], engine.sortedActiveSessions()); ok {
			t.Fatalf("未就绪区块仍选中目标: %d", got)
		}
	})
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

	engine.Step()
	if got := target.health; got != 18 {
		t.Fatalf("首 tick health=%d，想要 18", got)
	}
	for range 9 {
		engine.Step()
	}
	if got := target.health; got != 18 {
		t.Fatalf("冷却内 health=%d，想要 18", got)
	}
	engine.Step()
	if got := target.health; got != 16 {
		t.Fatalf("第十个间隔 tick health=%d，想要 16", got)
	}
	attacker.miningHeld = false
	engine.Step()
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

	result := engine.Step()
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

	engine.Step()
	target := engine.sessions[sessions[1]].player
	if target.health != 18 {
		t.Fatalf("首次命中 health=%d，想要 18", target.health)
	}
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{10.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[2], mgl32.Vec3{0.5, 1, 4.5}, 0)
	engine.sessions[sessions[0]].player.miningHeld = false
	engine.sessions[sessions[2]].player.miningHeld = true

	engine.Step()
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

	engine.Step()
	for _, id := range sessions {
		if got := engine.sessions[id].player.health; got != 18 {
			t.Fatalf("session %d health=%d，想要每人只受一次 2 点伤害后的 18", id, got)
		}
	}
}

func readyMeleePlayers(t *testing.T, count int) (*Engine, []SessionID) {
	t.Helper()
	engine, _ := readyMovementPlayer(t)
	for id := count; id >= 2; id-- {
		engine.RegisterSession(SessionID(id), core.Overworld, core.ChunkPos{})
	}
	if count > 1 {
		engine.Step()
	}
	sessions := make([]SessionID, count)
	for index := range count {
		sessions[index] = SessionID(index + 1)
	}
	return engine, sessions
}

func setMeleePlayer(engine *Engine, id SessionID, position mgl32.Vec3, yaw float32) {
	player := engine.sessions[id].player
	player.state.Position = position
	player.yaw = yaw
	player.pitch = 0
	player.health = core.MaxHealth
	player.reset = false
}

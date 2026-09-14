package server

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 本文件锁定掷骨者的管理器侧 ranged advisor：距离带驱动移动决策（>14 格
// A* 接近、6..14 格保持、<6 格直线后退意图，不做绕障承诺）、射击决策
// （40 tick 冷却就绪 + `core.InteractionTarget` 同源 LOS 才提交
// `RangedAttack`，基准方向 = 归一化(目标眼位 − 掷骨者眼位)，同 tick 射击
// 优先于移动）、以及掷骨者绝不冻结近战攻击意图。

// newHostileRangedWorld 与 `newHostileChaseWorld` 同构，但世界难度取
// peaceful：ranged advisor 断言只考察被恢复的掷骨者，夜间随机生成会引入
// 不可控的额外个体与投射物；和平档经既有生成入口短路（共享代码路径），
// 恰好把这一噪声源关掉。
func newHostileRangedWorld(
	t *testing.T,
	chunkFactory func(core.ChunkPos) *world.Chunk,
) (*runtime.Engine, *hostileManager, func([]hostileTargetPlayer)) {
	t.Helper()
	if chunkFactory == nil {
		chunkFactory = chaseFlatChunk
	}
	engine := runtime.NewEngine(2, chaseNightTicks, 0, core.DifficultyPeaceful)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	for range 40 {
		result := engine.Step()
		for _, key := range result.Acquire {
			engine.SubmitAcquired(contract.AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			engine.SubmitGenerated(contract.GeneratedChunk{
				Dimension: core.Overworld,
				Pos:       key.Pos,
				Chunk:     chunkFactory(key.Pos),
			})
		}
		if len(result.Acquire) == 0 && len(result.Generate) == 0 {
			break
		}
	}
	for dx := int32(-1); dx <= 1; dx++ {
		for dz := int32(-1); dz <= 1; dz++ {
			info, ok := engine.ChunkInfo(core.ChunkKey{
				Dimension: core.Overworld,
				Pos:       core.ChunkPos{X: dx, Z: dz},
			})
			if !ok || info.State != contract.ChunkReady {
				t.Fatalf("夹具世界区块 (%d,%d) 未就绪", dx, dz)
			}
		}
	}
	// 真实会话玩家经出生扫描落在原点附近，恰好与被恢复掷骨者的眼位射线同格
	// ——把玩家挪出飞行路径，投射物的实体扫描就不会被这个夹具副产物截胡
	//（ranged advisor 的目标全部来自注入的 targets，与真实玩家位置无关）。
	engine.SetPlayerPositionForTest(1, mgl32.Vec3{0.5, 1, 20.5})
	manager := newHostileManager(engine)
	var targets []hostileTargetPlayer
	manager.onlinePlayers = func() []hostileTargetPlayer { return targets }
	t.Cleanup(manager.close)
	return engine, manager, func(list []hostileTargetPlayer) { targets = list }
}

// rangedMob 构造一只可通过恢复校验的掷骨者记录。
func rangedMob(id uint64, position mgl32.Vec3) contract.HostileMob {
	mob := chaseMob(id, position)
	mob.Kind = contract.HostileKindBoneThrower
	return mob
}

func TestHurlerApproachesViaPathfindingWhenFar(t *testing.T) {
	engine, manager, setTargets := newHostileRangedWorld(t, nil)
	restoreChaseHostile(t, engine, rangedMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{16.5, 1, 0.5})})
	advanceHostileManager(manager)
	if !chaseSlot(t, manager, 11).pathInFlight {
		t.Fatal("16 格外的掷骨者未派发 A* 接近快照")
	}
	waitForChaseResults(t, manager, 1)
	advanceHostileManager(manager)
	if got := engine.HostileMobs()[0].Kind; got != contract.HostileKindBoneThrower {
		t.Fatalf("投影 kind=%d，想要掷骨者", got)
	}
	// 应用路径后再次推进：接近带的掷骨者沿 waypoint 朝目标移动（首拍被
	// 射击意图占用时，冷却期内的后续 tick 照常前进）。
	for range 3 {
		engine.Step()
		before := engine.HostileMobs()[0].State.Position
		advanceHostileManager(manager)
		engine.Step()
		after := engine.HostileMobs()[0].State.Position
		if after.X() > before.X() {
			return
		}
	}
	t.Fatal("接近带掷骨者未沿 waypoint 朝目标移动")
}

func TestHurlerHoldsPositionInMidBand(t *testing.T) {
	engine, manager, setTargets := newHostileRangedWorld(t, nil)
	restoreChaseHostile(t, engine, rangedMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	advanceHostileManager(manager)
	if chaseSlot(t, manager, 11).pathInFlight {
		t.Fatal("保持带（10 格）的掷骨者被派发寻路，想要保持位置")
	}
	before := engine.HostileMobs()[0].State.Position
	advanceHostileManager(manager)
	engine.Step()
	after := engine.HostileMobs()[0].State.Position
	if after != before {
		t.Fatalf("保持带掷骨者发生了位移：%v -> %v", before, after)
	}
}

func TestHurlerHoldsAtBandEdges(t *testing.T) {
	// 带边界语义：恰好 14 格归保持带（不寻路）、恰好 6 格同样保持（不后退）。
	engine, manager, setTargets := newHostileRangedWorld(t, nil)
	restoreChaseHostile(t, engine, rangedMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{14.5, 1, 0.5})})
	advanceHostileManager(manager)
	if chaseSlot(t, manager, 11).pathInFlight {
		t.Fatal("恰好 14 格的掷骨者被派发寻路，想要保持带")
	}
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{6.5, 1, 0.5})})
	advanceHostileManager(manager)
	engine.Step()
	before := engine.HostileMobs()[0].State.Position
	advanceHostileManager(manager)
	engine.Step()
	if after := engine.HostileMobs()[0].State.Position; after != before {
		t.Fatalf("恰好 6 格的掷骨者发生了位移：%v -> %v", before, after)
	}
}

func TestHurlerRetreatsStraightLineWhenClose(t *testing.T) {
	engine, manager, setTargets := newHostileRangedWorld(t, nil)
	restoreChaseHostile(t, engine, rangedMob(11, mgl32.Vec3{3.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{0.5, 1, 0.5})})
	// 直线后退意图：世界轴归一化向量背向目标（+X），可能被墙挡住但方向必须
	// 远离目标。首拍可能被射击意图占用（射击优先），两拍内必须后退。
	for range 3 {
		before := engine.HostileMobs()[0].State.Position
		advanceHostileManager(manager)
		engine.Step()
		if after := engine.HostileMobs()[0].State.Position; after.X() > before.X() {
			return
		}
	}
	t.Fatal("<6 格掷骨者未背向目标直线后退")
}

func TestHurlerNeverSubmitsMeleeIntentWhenWithinReach(t *testing.T) {
	// 近战触及距离内（<6 → 后退带）的掷骨者只提交后退移动意图，绝不冻结
	// AttackTarget——注入目标没有可伤害的真实玩家，这里以「唯一可能进入
	// inbox 的意图来自后退/射击」的位移方向作为近战意图缺席的可观察形态。
	engine, manager, setTargets := newHostileRangedWorld(t, nil)
	restoreChaseHostile(t, engine, rangedMob(11, mgl32.Vec3{1.2, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{0.5, 1, 0.5})})
	for range 3 {
		before := engine.HostileMobs()[0].State.Position
		advanceHostileManager(manager)
		engine.Step()
		after := engine.HostileMobs()[0].State.Position
		if after.X() < before.X() {
			t.Fatalf("触及距离内的掷骨者朝目标移动了：%v -> %v", before, after)
		}
	}
}

func TestHurlerShootsWhenCooldownReadyAndLOSVisible(t *testing.T) {
	engine, manager, setTargets := newHostileRangedWorld(t, nil)
	restoreChaseHostile(t, engine, rangedMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	advanceHostileManager(manager)
	engine.Step()
	projectiles := engine.ProjectilesForTest()
	if len(projectiles) != 1 {
		t.Fatalf("视线通畅且冷却就绪的掷骨者未发射：投射物=%d，想要 1", len(projectiles))
	}
	if got := projectiles[0].Kind; got != 0 /* 骨刺 */ {
		t.Fatalf("弹种=%d，想要骨刺 0", got)
	}
	if got := engine.HostileMobs()[0].ShootCooldown; got != 40 {
		t.Fatalf("射击后冷却=%d，想要 40", got)
	}
	// 冷却期内不再射击：投射物数量保持 1。
	advanceHostileManager(manager)
	engine.Step()
	if got := len(engine.ProjectilesForTest()); got != 1 {
		t.Fatalf("冷却期内再次发射：投射物=%d，想要仍为 1", got)
	}
}

func TestHurlerDoesNotShootWhenLOSBlocked(t *testing.T) {
	// 场景「视线被遮挡时不射击」：掷骨者与目标之间隔一堵实体墙——不发射，
	// 冷却保持就绪；墙体拆除后同姿态恢复射击。
	engine, manager, setTargets := newHostileRangedWorld(t, nil)
	restoreChaseHostile(t, engine, rangedMob(11, mgl32.Vec3{0.5, 1, 0.5}))
	setTargets([]hostileTargetPlayer{chaseTarget(0x02, 1, [3]float32{10.5, 1, 0.5})})
	// 在掷骨者（x≈0.5）与目标（x≈10.5）之间立起 y=2..3 的实体墙：眼位射线
	// （约 y=2.62）穿过 x=5 的墙体。
	for y := int32(2); y <= 3; y++ {
		engine.SetBlockForTest(core.BlockPos{X: 5, Y: y, Z: 0}, core.StoneID)
	}
	advanceHostileManager(manager)
	engine.Step()
	if got := len(engine.ProjectilesForTest()); got != 0 {
		t.Fatalf("视线被遮挡仍发射：投射物=%d，想要 0", got)
	}
	if got := engine.HostileMobs()[0].ShootCooldown; got != 0 {
		t.Fatalf("被遮挡时冷却=%d，想要保持就绪 0", got)
	}
	// 拆墙后视线恢复：同姿态下一拍即可发射。
	engine.SetBlockForTest(core.BlockPos{X: 5, Y: 2, Z: 0}, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: 5, Y: 3, Z: 0}, core.AirID)
	advanceHostileManager(manager)
	engine.Step()
	if got := len(engine.ProjectilesForTest()); got != 1 {
		t.Fatalf("视线恢复后未发射：投射物=%d，想要 1", got)
	}
}

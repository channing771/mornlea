package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定敌怪 kind 分支的生成契约：候选通过全部生成校验后按候选哈希
// 2:1 分派（hash%3==0 → 掷骨者，其余 → 夜行者）、近玩家 48 格上限按 kind
// 分别计数（夜行者 8、掷骨者 4，互不挤占）、恢复校验的 kind 值域、以及
// 掷骨者与夜行者共享同一份灼烧/远离消失/近战意图豁免的生命周期代码路径。

// collectSpawnedKinds 自 start 起逐 tick 扫描生成判定，把每次生成的
// (ID, kind) 记录后清空集合继续，至多收集 count 只或扫描 limit 个 tick。
// 清空集合保证个体 ID 恒为候选哈希本体（无冲突重散列），kind 与 ID 的
// 派生关系因此可精确断言。
func collectSpawnedKinds(
	t *testing.T, engine *Engine, start uint64, count, limit int,
) []HostileMob {
	t.Helper()
	collected := make([]HostileMob, 0, count)
	for offset := range limit {
		engine.worldTime.Store(start + uint64(offset))
		before := len(engine.hostiles.entries)
		engine.advanceHostileSpawn()
		if len(engine.hostiles.entries) > before {
			mobs := engine.HostileMobs()
			collected = append(collected, mobs[len(mobs)-1])
			if len(collected) == count {
				return collected
			}
			clearHostilesForTest(engine)
		}
	}
	t.Fatalf("扫描 %d 个 tick 内只收集到 %d/%d 只敌怪", limit, len(collected), count)
	return nil
}

func TestHostileSpawnKindFollowsCandidateHashRule(t *testing.T) {
	// kind 分派的精确形态：清空集合的探针下个体 ID 即候选哈希（未重散列），
	// 因此「掷骨者 ⇔ ID%3==0」是 2:1 分派规则（hash%3==0 → 掷骨者）的直接
	// 可观察断言，逐只核对不留统计灰区。
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	mobs := collectSpawnedKinds(t, engine, 13000, 40, 4000)
	hurlers := 0
	for _, mob := range mobs {
		want := HostileKindNightwalker
		if mob.ID%3 == 0 {
			want = HostileKindBoneThrower
		}
		if mob.Kind != want {
			t.Fatalf("敌怪 ID %d kind=%d，想要 %d（hash%%3 分派规则）", mob.ID, mob.Kind, want)
		}
		if mob.Kind == HostileKindBoneThrower {
			hurlers++
		}
	}
	if hurlers == 0 || hurlers == len(mobs) {
		t.Fatalf("40 只探针里掷骨者=%d，两类 kind 未同时出现，分派未生效", hurlers)
	}
}

func TestHostileSpawnKindRatioStaysNearTwoToOne(t *testing.T) {
	// 比例契约的统计形态：hash%3 的均匀性使掷骨者占约 1/3。探针窗口给足
	// 样本并以宽松边界（0.2..0.47）拒绝系统性偏离（如恒 0 或恒 1 的分派）。
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	mobs := collectSpawnedKinds(t, engine, 13000, 90, 9000)
	hurlers := 0
	for _, mob := range mobs {
		if mob.Kind == HostileKindBoneThrower {
			hurlers++
		}
	}
	ratio := float64(hurlers) / float64(len(mobs))
	if ratio < 0.2 || ratio > 0.47 {
		t.Fatalf("掷骨者比例=%v（%d/%d），越出 2:1 分派的宽松边界 [0.2, 0.47]",
			ratio, hurlers, len(mobs))
	}
}

func TestHostileSpawnNearLimitCountsPerKind(t *testing.T) {
	// 场景「近玩家上限按 kind 分别计数」：先探得两类候选各自的生成 tick，
	// 再分别预置满额的另一类个体——一类到上限必须只拒同 kind 候选，
	// 另一类候选照常生成，两类上限互不挤占。
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	probe := func(want uint8) uint64 {
		for offset := range 9000 {
			tick := uint64(13000 + offset)
			clearHostilesForTest(engine)
			engine.worldTime.Store(tick)
			engine.advanceHostileSpawn()
			if len(engine.hostiles.entries) == 1 && engine.hostiles.entries[0].kind == want {
				return tick
			}
		}
		t.Fatal("探针窗口内没有找到目标 kind 的生成 tick")
		return 0
	}
	nightwalkerTick := probe(HostileKindNightwalker)
	hurlerTick := probe(HostileKindBoneThrower)
	if nightwalkerTick == hurlerTick {
		t.Fatal("夹具失效：两类候选探到同一 tick")
	}

	// 预置 8 只近处夜行者（满夜行者近限）+ 4 只近处掷骨者（满掷骨者近限），
	// 之后每个探针 tick 只考察一个新候选。
	preset := func() {
		clearHostilesForTest(engine)
		for id := uint64(1); id <= 8; id++ {
			mob := validTestHostile(id)
			mob.Kind = HostileKindNightwalker
			mob.State.Position = mgl32.Vec3{float32(id) + 0.5, 1, 0.5}
			if err := engine.RestoreHostile(mob); err != nil {
				t.Fatalf("预置近处夜行者 %d：%v", id, err)
			}
		}
		for id := uint64(11); id <= 14; id++ {
			mob := validTestHostile(id)
			mob.Kind = HostileKindBoneThrower
			mob.State.Position = mgl32.Vec3{float32(id) + 20.5, 1, 0.5}
			if err := engine.RestoreHostile(mob); err != nil {
				t.Fatalf("预置近处掷骨者 %d：%v", id, err)
			}
		}
	}

	// 夜行者候选（ID%3!=0 的候选哈希 tick）：两类都已满额 → 拒绝。
	preset()
	engine.worldTime.Store(nightwalkerTick)
	engine.advanceHostileSpawn()
	if got := len(engine.hostiles.entries); got != 12 {
		t.Fatalf("夜行者近限满额时夜行者候选被接受：数量=%d，想要 12", got)
	}

	// 掷骨者候选（ID%3==0 的候选哈希 tick）：同样两类都已满额 → 拒绝。
	preset()
	engine.worldTime.Store(hurlerTick)
	engine.advanceHostileSpawn()
	if got := len(engine.hostiles.entries); got != 12 {
		t.Fatalf("掷骨者近限满额时掷骨者候选被接受：数量=%d，想要 12", got)
	}

	// 只清空掷骨者（夜行者仍满 8）：掷骨者候选必须被接受——夜行者的 8 只
	// 不挤占掷骨者的 4 个名额。
	preset()
	for id := uint64(11); id <= 14; id++ {
		if index := engine.hostiles.findIndex(id); index >= 0 {
			engine.hostiles.removeAt(index)
		}
	}
	engine.worldTime.Store(hurlerTick)
	engine.advanceHostileSpawn()
	if got := len(engine.hostiles.entries); got != 9 {
		t.Fatalf("夜行者满额时掷骨者候选被拒：数量=%d，想要 9（8 夜行者 + 1 掷骨者）", got)
	}

	// 只清空夜行者（掷骨者仍满 4）：夜行者候选必须被接受——掷骨者的 4 只
	// 不挤占夜行者的 8 个名额。
	preset()
	for id := uint64(1); id <= 8; id++ {
		if index := engine.hostiles.findIndex(id); index >= 0 {
			engine.hostiles.removeAt(index)
		}
	}
	engine.worldTime.Store(nightwalkerTick)
	engine.advanceHostileSpawn()
	if got := len(engine.hostiles.entries); got != 5 {
		t.Fatalf("掷骨者满额时夜行者候选被拒：数量=%d，想要 5（1 夜行者 + 4 掷骨者）", got)
	}
}

func TestHostileMobProjectionCarriesKind(t *testing.T) {
	// 恢复与投影：掷骨者记录经 RestoreHostile 进集合后，HostileMobs 投影必须
	// 原样携带 Kind；射击冷却是瞬态，恢复入口恒从就绪态开始。
	engine := NewEngine(0, 0, 0)
	mob := validTestHostile(42)
	mob.Kind = HostileKindBoneThrower
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复掷骨者：%v", err)
	}
	projected := engine.HostileMobs()[0]
	if projected.Kind != HostileKindBoneThrower {
		t.Fatalf("投影 kind=%d，想要 %d", projected.Kind, HostileKindBoneThrower)
	}
	if projected.ShootCooldown != 0 {
		t.Fatalf("恢复后射击冷却=%d，想要就绪态 0", projected.ShootCooldown)
	}
	// 恢复入口忽略输入里的瞬态冷却：即便快照带着残余冷却，恢复仍就绪。
	mob.ShootCooldown = 17
	engine2 := NewEngine(0, 0, 0)
	if err := engine2.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复带瞬态冷却的记录：%v", err)
	}
	if got := engine2.hostiles.entries[0].shootCooldown; got != 0 {
		t.Fatalf("恢复后的瞬态射击冷却=%d，想要 0（不入存档）", got)
	}
}

func TestHurlerBurnsLikeNightwalker(t *testing.T) {
	// 共享生命周期（灼烧）：掷骨者白昼露天同样每 20 tick 扣 1，代码路径与
	// 夜行者共用（同一段 advanceHostileBurn，无 kind 分叉）。
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(21)
	mob.Kind = HostileKindBoneThrower
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复掷骨者：%v", err)
	}
	for range 19 {
		engine.advanceHostileBurn(testDayTick)
	}
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("掷骨者前 19 tick 生命=%d，想要仍为 %d", got, core.MaxHealth)
	}
	engine.advanceHostileBurn(testDayTick)
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth-1 {
		t.Fatalf("掷骨者第 20 tick 生命=%d，想要 %d", got, core.MaxHealth-1)
	}
}

func TestHurlerDistantDespawnLikeNightwalker(t *testing.T) {
	// 共享生命周期（远离消失）：掷骨者远离全部玩家累计 600 tick 后同样无掉落
	// 移除。
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(24)
	mob.Kind = HostileKindBoneThrower
	mob.State.Position = mgl32.Vec3{80.5, 1, 0.5}
	mob.DistantTicks = 599
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复掷骨者：%v", err)
	}
	engine.advanceHostileDistant()
	if len(engine.hostiles.entries) != 0 {
		t.Fatalf("远离 600 tick 后掷骨者仍在（数量=%d）", len(engine.hostiles.entries))
	}
	if got := countLoadedDrops(t, engine, core.ItemBone); got != 0 {
		t.Fatalf("远离消失产生了 %d 个骨头掉落，想要 0", got)
	}
}

func TestHurlerProducesNoMeleeIntent(t *testing.T) {
	// 场景「无近战意图」：掷骨者与玩家相邻且攻击意图被冻结时，战斗结算的
	// kind 门禁必须把它挡下——玩家不掉血、掷骨者不进攻击冷却。
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 21, mgl32.Vec3{2.0, 1, 0.5})
	engine.hostiles.entries[0].kind = HostileKindBoneThrower
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	advanceHostilesTick(engine, []HostileAction{{
		ID: 21, AttackTarget: true, TargetSession: session,
	}})
	if got := engine.sessions[session].player.health; got != core.MaxHealth {
		t.Fatalf("掷骨者近战意图扣血到 %d，想要保持 %d", got, core.MaxHealth)
	}
	if got := engine.hostiles.entries[0].attackCooldown; got != 0 {
		t.Fatalf("掷骨者近战意图进入冷却 %d，想要 0", got)
	}
}

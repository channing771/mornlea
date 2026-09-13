package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/updates"
)

// 本文件锁定掷骨者的远程射击结算：`RangedAttack` 意图经冷却/存活/kind 校验后
// 在眼位生成骨刺（伤害 3、初速 22 格/秒）、确定性散布只依赖
// (worldSeed, 权威 tick, 敌怪 ID) 且水平/竖直偏移都 ≤ 0.06 rad（固定数值
// 契约）、射击冷却 40 tick 瞬态不入存档。

// hurlerShotEngine 构造夜间相位、一名已激活玩家与一只地面掷骨者的夹具。
func hurlerShotEngine(t *testing.T, id uint64) (*Engine, SessionID) {
	t.Helper()
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, id, mgl32.Vec3{2.5, 1, 2.5})
	engine.hostiles.entries[0].kind = HostileKindBoneThrower
	return engine, session
}

// hurlerShotAction 返回一条指向 +X 的合法射击意图（基准方向按归一化向量提交，
// 与管理器的生产形态一致）。
func hurlerShotAction(id uint64) HostileAction {
	return HostileAction{ID: id, RangedAttack: true, AimX: 1}
}

func TestRangedAttackSpawnsShardFromEyeTowardAim(t *testing.T) {
	engine, _ := hurlerShotEngine(t, 21)
	mob := &engine.hostiles.entries[0]
	base := mgl32.Vec3{1, 0, 0}
	// 直接结算意图阶段：出生事实在投射物推进前断言，位置/速度不受同 tick
	// 弹道积分与重力影响。
	engine.applyHostileActions([]HostileAction{hurlerShotAction(21)})
	if got := len(engine.projectiles.entries); got != 1 {
		t.Fatalf("射击后投射物数量=%d，想要恰好 1", got)
	}
	shard := engine.projectiles.entries[0]
	if shard.kind != projectileKindShard {
		t.Fatalf("弹种=%d，想要骨刺 %d", shard.kind, projectileKindShard)
	}
	if shard.owner != 21 {
		t.Fatalf("发射者=%d，想要敌怪 ID 21", shard.owner)
	}
	if shard.damage != projectileShardDamage {
		t.Fatalf("伤害=%d，想要 %d", shard.damage, projectileShardDamage)
	}
	// 出生点是掷骨者眼位（脚底 + 眼高）。
	eye := mob.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	if shard.position != eye {
		t.Fatalf("出生点=%v，想要眼位 %v", shard.position, eye)
	}
	// 初速恒为 22 格/秒；方向与基准方向的偏移是散布，两轴分量都不得越界。
	speed := shard.velocity.Len()
	if math.Abs(float64(speed-projectileShardSpeed)) > 1e-3 {
		t.Fatalf("初速=%v，想要 %v", speed, projectileShardSpeed)
	}
	assertShardOffsetWithinContract(t, base, shard.velocity)
	// 射击结算置满冷却周期，且投影可观察。
	if got := mob.shootCooldown; got != hostileShootCooldownTicks {
		t.Fatalf("射击后冷却=%d，想要 %d", got, hostileShootCooldownTicks)
	}
	if got := engine.HostileMobs()[0].ShootCooldown; got != hostileShootCooldownTicks {
		t.Fatalf("投影射击冷却=%d，想要 %d", got, hostileShootCooldownTicks)
	}
}

// assertShardOffsetWithinContract 断言弹速方向相对基准方向的散布偏移：水平
// （yaw）与竖直（pitch）两轴各自 ≤ HostileShotSpreadMaxRadians（0.06 rad）。
func assertShardOffsetWithinContract(t *testing.T, base, velocity mgl32.Vec3) {
	t.Helper()
	unit := velocity.Normalize()
	baseYaw := math.Atan2(float64(-base.X()), float64(-base.Z()))
	basePitch := math.Asin(math.Min(1, math.Max(-1, float64(base.Y()))))
	yaw := math.Atan2(float64(-unit.X()), float64(-unit.Z()))
	pitch := math.Asin(math.Min(1, math.Max(-1, float64(unit.Y()))))
	limit := float64(updates.HostileShotSpreadMaxRadians) + 1e-6
	if dyaw := math.Abs(yaw - baseYaw); dyaw > limit {
		t.Fatalf("水平散布偏移=%v rad，越出 ±%v", dyaw, limit)
	}
	if dpitch := math.Abs(pitch - basePitch); dpitch > limit {
		t.Fatalf("竖直散布偏移=%v rad，越出 ±%v", dpitch, limit)
	}
}

func TestRangedAttackCooldownBlocksReshotsForExactlyFortyTicks(t *testing.T) {
	// 冷却周期契约：射击当刻置满 40，随后逐 tick 递减；冷却期内重冻结的
	// 射击意图确定性丢弃，恰好 40 个 tick 后恢复结算。
	engine, _ := hurlerShotEngine(t, 21)
	action := hurlerShotAction(21)
	advanceHostilesTick(engine, []HostileAction{action})
	if got := engine.hostiles.entries[0].shootCooldown; got != hostileShootCooldownTicks {
		t.Fatalf("首次射击后冷却=%d，想要 %d", got, hostileShootCooldownTicks)
	}
	for tick := uint8(1); tick < hostileShootCooldownTicks; tick++ {
		advanceHostilesTick(engine, []HostileAction{action})
		want := hostileShootCooldownTicks - tick
		if got := engine.hostiles.entries[0].shootCooldown; got != want {
			t.Fatalf("冷却第 %d tick 剩余=%d，想要 %d", tick, got, want)
		}
	}
	// 第 40 tick：冷却归零，同 tick 的射击意图再次结算并重新进入冷却。
	advanceHostilesTick(engine, []HostileAction{action})
	if got := engine.hostiles.entries[0].shootCooldown; got != hostileShootCooldownTicks {
		t.Fatalf("冷却走完后未恢复射击（冷却=%d，想要重新置满 %d）", got, hostileShootCooldownTicks)
	}
}

func TestRangedAttackDropsInvalidIntentsDeterministically(t *testing.T) {
	cases := map[string]func(*Engine, *HostileAction){
		"未知敌怪 ID": func(engine *Engine, action *HostileAction) {
			action.ID = 999
		},
		"夜行者携带射击意图": func(engine *Engine, action *HostileAction) {
			engine.hostiles.entries[0].kind = HostileKindNightwalker
		},
		"零方向": func(engine *Engine, action *HostileAction) {
			action.AimX, action.AimY, action.AimZ = 0, 0, 0
		},
		"方向非有限": func(engine *Engine, action *HostileAction) {
			action.AimY = float32(math.Inf(1))
		},
	}
	for name, mutate := range cases {
		engine, _ := hurlerShotEngine(t, 21)
		action := hurlerShotAction(21)
		mutate(engine, &action)
		engine.applyHostileActions([]HostileAction{action})
		if got := len(engine.projectiles.entries); got != 0 {
			t.Fatalf("%s：非法射击意图生成了 %d 条投射物，想要 0", name, got)
		}
		if got := engine.hostiles.entries[0].shootCooldown; got != 0 {
			t.Fatalf("%s：被拒意图仍消耗冷却（%d），想要保持就绪", name, got)
		}
	}
}

func TestRangedAttackSkipsDeadHostile(t *testing.T) {
	// 死亡个体（生命 0、待结算移除）不再接受射击意图：死亡结算统一在 tick
	// 稍后完成，期间不得产生新的世界副作用。
	engine, _ := hurlerShotEngine(t, 21)
	engine.hostiles.entries[0].health = 0
	engine.applyHostileActions([]HostileAction{hurlerShotAction(21)})
	if got := len(engine.projectiles.entries); got != 0 {
		t.Fatalf("死亡掷骨者仍发射（投射物=%d），想要 0", got)
	}
}

func TestHostileShotSpreadIsBitIdenticalOnReplay(t *testing.T) {
	// 场景「散布确定性」：相同 (seed, tick, ID, 基准方向) 的散布弹速逐位一致；
	// 不同 (tick, ID) 组合覆盖多个不同偏移——散布不是常量，也不依赖进程随机源。
	// 结算直接经 applyHostileActions 驱动，权威 tick 用 Store 固定到探针值。
	run := func(tick uint64, id uint64) mgl32.Vec3 {
		engine, _ := hurlerShotEngine(t, id)
		engine.tick.Store(tick)
		engine.applyHostileActions([]HostileAction{hurlerShotAction(id)})
		if len(engine.projectiles.entries) != 1 {
			t.Fatalf("tick %d / 敌怪 %d：射击未生成投射物", tick, id)
		}
		return engine.projectiles.entries[0].velocity
	}
	first := run(7, 21)
	second := run(7, 21)
	if first != second {
		t.Fatalf("相同输入的散布弹速不一致：%v vs %v", first, second)
	}
	distinct := map[[3]float32]struct{}{first: {}}
	for _, combo := range [][2]uint64{{8, 21}, {9, 21}, {7, 34}, {8, 34}, {9, 34}, {10, 21}, {11, 21}, {12, 34}} {
		velocity := run(combo[0], combo[1])
		distinct[[3]float32{velocity.X(), velocity.Y(), velocity.Z()}] = struct{}{}
	}
	if len(distinct) < 4 {
		t.Fatalf("不同 (tick, ID) 组合只覆盖 %d 种散布弹速，想要 ≥4", len(distinct))
	}
}

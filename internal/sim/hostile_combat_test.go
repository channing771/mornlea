package sim

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件锁定夜行者的移动意图消费与近战伤害结算：`HostileAction` 是有界追逐
// worker 在 tick 边界提交的唯一轴量入口（世界轴方向 + 跳跃 + 一次攻击意图），
// 近战结算在同一 tick 内先冻结全部意图、再按 ID 升序进入统一战斗结算，造成
// 3 点伤害并进入 20 tick 冷却。

// hostileCombatEngine 构造一名已激活玩家与夜间相位的 flat 世界夹具：夜相位
// 使灼烧与夜间生成在后续推进中都不干扰伤害断言（生成候选落在未加载区块，
// 全部被拒）。
func hostileCombatEngine(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	engine, session := readyMovementPlayer(t)
	engine.worldTime.Store(testNightTick)
	return engine, session
}

// restoreCombatHostile 以指定 ID 把一只地面夜行者恢复进集合。
func restoreCombatHostile(t *testing.T, engine *Engine, id uint64, position mgl32.Vec3) {
	t.Helper()
	mob := validTestHostile(id)
	mob.State.Position = position
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者 %d：%v", id, err)
	}
}

func TestHostileActionSteersMovementAlongWorldAxis(t *testing.T) {
	engine, _ := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 11, mgl32.Vec3{5.5, 1, 5.5})

	// 世界轴意图 (-1, 0)（朝 -X）必须转化为朝向前进而非平移：夜行者 yaw 旋到
	// 该方向并以既有 per-actor 物理前进。
	if !engine.EnqueueHostileAction(HostileAction{ID: 11, MoveX: -1}) {
		t.Fatal("移动意图被有界 inbox 拒绝")
	}
	engine.Step()
	mob := engine.hostiles.entries[0]
	if mob.state.Position.X() >= 5.5 {
		t.Fatalf("朝 -X 的意图未产生 -X 位移：位置=%v", mob.state.Position)
	}
	const westYaw = float32(math.Pi / 2)
	if diff := mob.yaw - westYaw; diff > 1e-4 || diff < -1e-4 {
		t.Fatalf("朝 -X 的 yaw=%v，想要 atan2(1,0)=π/2", mob.yaw)
	}
	// 下一 tick 无 action：输入回中性（保 yaw、无前进），夜行者停住。
	before := mob.state.Position
	engine.Step()
	if engine.hostiles.entries[0].state.Position != before {
		t.Fatalf("无意图 tick 夜行者仍位移：%v -> %v",
			before, engine.hostiles.entries[0].state.Position)
	}
}

func TestHostileActionInboxIsBoundedAndDropsOverflow(t *testing.T) {
	engine, _ := hostileCombatEngine(t)
	for id := uint64(1); id <= maxHostiles; id++ {
		restoreCombatHostile(t, engine, id, mgl32.Vec3{2.5, 1, 2.5})
	}
	for id := uint64(1); id <= maxHostiles; id++ {
		if !engine.EnqueueHostileAction(HostileAction{ID: id}) {
			t.Fatalf("第 %d 条意图被拒绝，想要 inbox 容量覆盖全集合", id)
		}
	}
	if engine.EnqueueHostileAction(HostileAction{ID: maxHostiles}) {
		t.Fatal("超容意图被接受，想要非阻塞丢弃")
	}
}

func TestHostileActionDropsInvalidPayloadDeterministically(t *testing.T) {
	engine, _ := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 11, mgl32.Vec3{5.5, 1, 5.5})
	cases := map[string]HostileAction{
		"移动分量越界":  {ID: 11, MoveX: 1.5},
		"移动分量非有限": {ID: 11, MoveZ: float32(math.Inf(1))},
		"攻击意图缺会话": {ID: 11, AttackTarget: true},
	}
	for name, action := range cases {
		engine.EnqueueHostileAction(action)
		engine.Step()
		if got := engine.hostiles.entries[0].state.Position.X(); got != 5.5 {
			t.Fatalf("%s：非法载荷产生了位移（X=%v）", name, got)
		}
	}
}

func TestHostileMeleeDealsExactlyThreeDamageAndEntersCooldown(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	// 水平距离 1.5（≤1.8 攻击距离）。
	restoreCombatHostile(t, engine, 21, mgl32.Vec3{2.0, 1, 0.5})
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})

	if !engine.EnqueueHostileAction(HostileAction{
		ID: 21, AttackTarget: true, TargetSession: session,
	}) {
		t.Fatal("攻击意图被拒绝")
	}
	engine.Step()

	if got := engine.sessions[session].player.health; got != core.MaxHealth-3 {
		t.Fatalf("玩家生命=%d，想要 %d（恰好 3 点伤害）", got, core.MaxHealth-3)
	}
	if got := engine.hostiles.entries[0].attackCooldown; got != hostileCooldownPeriodTicks {
		t.Fatalf("攻击冷却=%d，想要 %d", got, hostileCooldownPeriodTicks)
	}
	if engine.sessions[session].player.hurtCooldownTicks == 0 {
		t.Fatal("受击保护期未建立")
	}
}

func TestHostileMeleeCooldownBlocksRepeatedDamage(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 21, mgl32.Vec3{2.0, 1, 0.5})
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	engine.EnqueueHostileAction(HostileAction{ID: 21, AttackTarget: true, TargetSession: session})
	engine.Step()
	if got := engine.sessions[session].player.health; got != core.MaxHealth-3 {
		t.Fatalf("首次攻击生命=%d，想要 %d", got, core.MaxHealth-3)
	}
	// 冷却期内逐 tick 重冻结意图：19 tick 内绝不重复扣血。
	for tick := 1; tick < 20; tick++ {
		engine.EnqueueHostileAction(HostileAction{ID: 21, AttackTarget: true, TargetSession: session})
		engine.Step()
		if got := engine.sessions[session].player.health; got != core.MaxHealth-3 {
			t.Fatalf("冷却期第 %d tick 生命=%d，想要保持 %d", tick, got, core.MaxHealth-3)
		}
	}
	// 第 20 tick 冷却走完：同位再冻结一次意图可再次命中。
	engine.EnqueueHostileAction(HostileAction{ID: 21, AttackTarget: true, TargetSession: session})
	engine.Step()
	if got := engine.sessions[session].player.health; got != core.MaxHealth-6 {
		t.Fatalf("冷却结束后生命=%d，想要 %d", got, core.MaxHealth-6)
	}
}

func TestHostileMeleeOutOfRangeDoesNoDamage(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 21, mgl32.Vec3{3.0, 1, 0.5}) // 水平 2.5 > 1.8
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	engine.EnqueueHostileAction(HostileAction{ID: 21, AttackTarget: true, TargetSession: session})
	engine.Step()
	if got := engine.sessions[session].player.health; got != core.MaxHealth {
		t.Fatalf("超距攻击扣血到 %d，想要保持 %d", got, core.MaxHealth)
	}
	if got := engine.hostiles.entries[0].attackCooldown; got != 0 {
		t.Fatalf("超距攻击进入冷却 %d，想要 0", got)
	}
}

func TestHostileMeleeSettlesFrozenIntentsByIDWithProtection(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	// 两只夜行者同 tick 各冻结一次攻击意图；ID 升序结算下先攻者建立受击保护，
	// 后攻者被合并——玩家生命恰好只下降一次（3 点）。
	restoreCombatHostile(t, engine, 3, mgl32.Vec3{2.0, 1, 0.5})
	restoreCombatHostile(t, engine, 7, mgl32.Vec3{0.5, 1, 2.0})
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	engine.EnqueueHostileAction(HostileAction{ID: 7, AttackTarget: true, TargetSession: session})
	engine.EnqueueHostileAction(HostileAction{ID: 3, AttackTarget: true, TargetSession: session})
	engine.Step()
	if got := engine.sessions[session].player.health; got != core.MaxHealth-3 {
		t.Fatalf("同 tick 双重意图生命=%d，想要按保护期合并为 %d", got, core.MaxHealth-3)
	}
	if got := engine.hostiles.entries[0].attackCooldown; got != hostileCooldownPeriodTicks {
		t.Fatalf("先结算者（ID 3）冷却=%d，想要 %d", got, hostileCooldownPeriodTicks)
	}
	if got := engine.hostiles.entries[1].attackCooldown; got != 0 {
		t.Fatalf("被保护合并者（ID 7）冷却=%d，想要 0", got)
	}
}

func TestHostileMeleeIgnoresStaleOrForeignTargets(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 21, mgl32.Vec3{2.0, 1, 0.5})
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	// 未知会话的攻击意图必须确定性丢弃。
	engine.EnqueueHostileAction(HostileAction{ID: 21, AttackTarget: true, TargetSession: 99})
	engine.Step()
	if got := engine.sessions[session].player.health; got != core.MaxHealth {
		t.Fatalf("未知会话意图扣血到 %d，想要保持 %d", got, core.MaxHealth)
	}
}

func TestHostileMeleeDeathUsesExistingSettlement(t *testing.T) {
	engine, session := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 21, mgl32.Vec3{2.0, 1, 0.5})
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{0.5, 1, 0.5})
	player := engine.sessions[session].player
	player.health = 2
	engine.EnqueueHostileAction(HostileAction{ID: 21, AttackTarget: true, TargetSession: session})
	engine.Step()
	// 既有死亡结算在同一 tick 完成：满血复活并转入待重生的位置跳变路径，
	// 外部观察不到生命值为 0 的中间状态。
	if got := player.health; got != core.MaxHealth {
		t.Fatalf("敌怪致死后的生命=%d，想要既有结算的满血 %d", got, core.MaxHealth)
	}
	if player.lifecycle != PlayerPendingSpawn {
		t.Fatal("敌怪致死未走既有的位置跳变（待重生）路径")
	}
}

func TestPlanHostileChaseWritesFactsAndValidatesPairing(t *testing.T) {
	engine, _ := hostileCombatEngine(t)
	restoreCombatHostile(t, engine, 21, mgl32.Vec3{2.5, 1, 2.5})
	target := testTargetPlayerID()
	if !engine.PlanHostileChase(21, true, target, 4096) {
		t.Fatal("合法追逐事实被拒绝")
	}
	mobs := engine.HostileMobs()
	if !mobs[0].HasTarget || mobs[0].PlayerID != target || mobs[0].NextRepathTicks != 4096 {
		t.Fatalf("追逐事实未落盘：%+v", mobs[0])
	}
	if engine.PlanHostileChase(21, false, target, 0) {
		t.Fatal("无目标却携带玩家 ID 的事实被接受")
	}
	if engine.PlanHostileChase(21, true, core.PlayerID{}, 0) {
		t.Fatal("有目标但玩家 ID 为零的事实被接受")
	}
	if engine.PlanHostileChase(999, true, target, 0) {
		t.Fatal("未知夜行者的追逐事实被接受")
	}
	// 合法清除：无目标时玩家 ID 必须归零。
	if !engine.PlanHostileChase(21, false, core.PlayerID{}, 7) {
		t.Fatal("合法清除被拒绝")
	}
	mobs = engine.HostileMobs()
	if mobs[0].HasTarget || mobs[0].PlayerID != (core.PlayerID{}) || mobs[0].NextRepathTicks != 7 {
		t.Fatalf("清除后的追逐事实不正确：%+v", mobs[0])
	}
}

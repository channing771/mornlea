package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是踩踏雪层留脚印（snow-cover-accumulation）的行为主题测试：落足实体
// （玩家与被动牛共用同一机制）水平位移累计跨过 0.6 格阈值时，把脚部所在格的
// 雪层削低一档（1 档踩碎为空气）；静止站立不削减；每实体每 tick 至多写 1 格；
// 未加载区块静默跳过且变更仍经 mutation 广播。
//
// 夹具全部复用既有 helper：readyMovementPlayer 构造 flat 世界玩家（y=0 全草，
// 脚底 Y 恰为整数格顶 1.0，脚部所在格即 y=1 的雪层格），loadFlatChunks 补齐行
// 进方向区块，SetBlockForTest 预铺雪层。行走用 CommandPlayerInput 直驱权威物
// 理；被动牛用 DamagePassive 的逃跑输入驱赶出确定性的直线位移。

// advanceSnowFootprintTick 按生产阶段顺序推进玩家命令、actor 物理与脚印结算，
// 并提交该阶段写入。
func advanceSnowFootprintTick(engine *Engine, commands []Command) TickResult {
	tick := engine.beginTick()
	tick.context.ApplyPlayerCommands(commands, &tick.result)
	tick.context.AdvanceActors()
	tick.context.SettleSnowFootprints()
	commitMutation(tick.mutation, &tick.result)
	return publishFixture(engine, &tick)
}

// advanceSnowPassiveTick 按生产顺序推进被动牛阶段（含移动）与脚印结算并提交
// 写入：形如 advanceGrazeTick，但补上脚印结算出口。
func advanceSnowPassiveTick(engine *Engine, tick uint64) TickResult {
	engine.tick.Store(tick)
	result := TickResult{}
	pending := engine.newMutation()
	engine.advancePassives(pending)
	engine.settleSnowFootprints(pending)
	engine.finishChanges(pending, &result)
	engine.advanceFixtureClock()
	return result
}

// snowFootprintPlayerPosition 读取玩家的权威位置并断言落足状态：脚印判定只在
// OnGround 上发生，夹具一旦让玩家离地即判失败而不是静默走弱。
func snowFootprintPlayerPosition(t *testing.T, engine *Engine, session SessionID) mgl32.Vec3 {
	t.Helper()
	player := engine.sessions[session].player
	if player == nil || player.lifecycle != PlayerActive {
		t.Fatal("玩家不在 Active 状态，夹具失效")
	}
	if !player.state.OnGround {
		t.Fatalf("玩家意外离地: %+v", player.state)
	}
	return player.state.Position
}

// walkSnowFootprintTowardNegativeZ 以 Yaw=0（前方恰为 -Z）直线驱赶玩家至少
// distance 格。每 tick 下发一条 MoveZ=1 输入，位移完全由权威物理决定。
func walkSnowFootprintTowardNegativeZ(
	t *testing.T,
	engine *Engine,
	session SessionID,
	distance float32,
) {
	t.Helper()
	start := snowFootprintPlayerPosition(t, engine, session)
	for step := uint64(0); step < 200; step++ {
		advanceSnowFootprintTick(engine, []Command{{
			Session:  session,
			Sequence: step + 2,
			Kind:     CommandPlayerInput,
			MoveZ:    1,
		}})
		position := snowFootprintPlayerPosition(t, engine, session)
		if start.Z()-position.Z() >= distance {
			return
		}
	}
	t.Fatalf("200 tick 内未走满 %v 格", distance)
}

// laySnowFootprintStrip 在 y=1 铺一条沿 Z 轴的雪带（含两端）。
func laySnowFootprintStrip(
	t *testing.T,
	engine *Engine,
	x, minZ, maxZ int32,
	block core.BlockID,
) {
	t.Helper()
	for z := minZ; z <= maxZ; z++ {
		engine.SetBlockForTest(core.BlockPos{X: x, Y: 1, Z: z}, block)
	}
}

// requireSnowStripUntouched 断言雪带上没有任何格被削穿到 1 档或空气：同格
// 重复削减（来回踱步/采样抖动）会让这里的某格低于 2 档。
func requireSnowStripNotOvertrimmed(
	t *testing.T,
	engine *Engine,
	x, minZ, maxZ int32,
) {
	t.Helper()
	for z := minZ; z <= maxZ; z++ {
		if got := fluidBlockAt(t, engine, core.BlockPos{X: x, Y: 1, Z: z}); got == core.AirID ||
			got == core.SnowLayer1BlockID {
			t.Fatalf("雪带格 (x=%d,z=%d) 被削穿到 %d，想要每格至多降一档", x, z, got)
		}
	}
}

// TestSnowFootprintWalkLeavesTrimmedTrail 覆盖 Scenario「行走留下脚印链」：
// 玩家站 3 档雪层沿直线走 3 格，行进路径格降为 2 档。采样点落在每跨 0.6 格
// 的累计阈值处：起点格从未被跨过阈值采样，保持 3 档；路径中段两格各被采样
// 恰一次，降为 2 档。
func TestSnowFootprintWalkLeavesTrimmedTrail(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 0, -1, 0)
	laySnowFootprintStrip(t, engine, 0, -3, 0, core.SnowLayer3BlockID)

	walkSnowFootprintTowardNegativeZ(t, engine, session, 3)

	for _, z := range []int32{-1, -2} {
		if got := fluidBlockAt(t, engine, core.BlockPos{X: 0, Y: 1, Z: z}); got != core.SnowLayer2BlockID {
			t.Fatalf("路径格 (z=%d) = %d，想要降为 2 档雪层", z, got)
		}
	}
	if got := fluidBlockAt(t, engine, core.BlockPos{X: 0, Y: 1, Z: 0}); got != core.SnowLayer3BlockID {
		t.Fatalf("起点格 = %d，想要未被采样保持 3 档", got)
	}
	requireSnowStripNotOvertrimmed(t, engine, 0, -3, 0)
}

// TestSnowFootprintShattersThinnestTier 覆盖 Scenario「最薄档踩碎」：落足移动
// 经过 1 档雪层格，该格 MUST 变为空气。
func TestSnowFootprintShattersThinnestTier(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 0, -1, 0)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 1, Z: -1}, core.SnowLayer1BlockID)

	walkSnowFootprintTowardNegativeZ(t, engine, session, 1.2)

	if got := fluidBlockAt(t, engine, core.BlockPos{X: 0, Y: 1, Z: -1}); got != core.AirID {
		t.Fatalf("被踩的 1 档雪层 = %d，想要空气", got)
	}
}

// TestSnowFootprintStandingStillKeepsTier 覆盖 Scenario「静止不反复削减」：
// 玩家静止站在 2 档雪层上，任意多权威 tick 后档位不变。累计器预置到阈值
// 边缘之下（0.55 < 0.6），静止的零位移永不跨过阈值——「位移不足不采样」
// 与「同格记忆」两道防线都不必依赖，这里钉死最前置的那道。
func TestSnowFootprintStandingStillKeepsTier(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 1, Z: 0}, core.SnowLayer2BlockID)
	engine.sessions[session].player.snowFootprint.travel = 0.55

	for range 40 {
		advanceSnowFootprintTick(engine, nil)
	}

	if got := fluidBlockAt(t, engine, core.BlockPos{X: 0, Y: 1, Z: 0}); got != core.SnowLayer2BlockID {
		t.Fatalf("静止站立 40 tick 后雪层 = %d，想要保持 2 档", got)
	}
}

// TestSnowFootprintPassiveCowTramplesSnow 锚定「玩家与被动牛同机制」：受击逃跑
// 输入驱使牛沿 -Z 直线穿过 3 档雪带，逃跑路径格与玩家行走同形地降为 2 档，
// 起点格保持、无格被削穿。
func TestSnowFootprintPassiveCowTramplesSnow(t *testing.T) {
	engine := newGrazeEngine(t, 0)
	restoreGrazeCow(t, engine, 7, mgl32.Vec3{2.5, 1, 2.5})
	laySnowFootprintStrip(t, engine, 2, -4, 2, core.SnowLayer3BlockID)
	// 伤害来自 +Z 远处：逃跑朝向恰为 -Z，位移沿 x=2.5 直线。
	if !engine.DamagePassive(7, 1, mgl32.Vec3{2.5, 1, 12.5}) {
		t.Fatal("已知个体的有效伤害被拒绝")
	}

	for tick := uint64(0); tick < 25; tick++ {
		advanceSnowPassiveTick(engine, tick)
	}

	for _, z := range []int32{1, 0, -1, -2} {
		if got := fluidBlockAt(t, engine, core.BlockPos{X: 2, Y: 1, Z: z}); got != core.SnowLayer2BlockID {
			t.Fatalf("逃跑路径格 (z=%d) = %d，想要降为 2 档雪层", z, got)
		}
	}
	if got := fluidBlockAt(t, engine, core.BlockPos{X: 2, Y: 1, Z: 2}); got != core.SnowLayer3BlockID {
		t.Fatalf("牛的起点格 = %d，想要未被采样保持 3 档", got)
	}
	requireSnowStripNotOvertrimmed(t, engine, 2, -4, 2)
}

// TestSnowFootprintWritesAtMostOneCellPerEntityPerTick 钉死「每实体每 tick 至多
// 写 1 格」与广播契约：单次 noteSnowFootprint 调用代表单实体的一个权威 tick，
// 即使该步水平位移远超阈值（夹具一步跨 3 格），也只采样终点落足格一次；途经
// 格不被顺带削掉；采样后累计清零，同 tick 的静止续调用不再追加候选。
func TestSnowFootprintWritesAtMostOneCellPerEntityPerTick(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	foot := core.BlockPos{X: 3, Y: 1, Z: 0}
	passed := core.BlockPos{X: 1, Y: 1, Z: 0}
	engine.SetBlockForTest(foot, core.SnowLayer3BlockID)
	engine.SetBlockForTest(passed, core.SnowLayer3BlockID)
	tracker := &engine.sessions[session].player.snowFootprint

	engine.noteSnowFootprint(
		tracker, core.Overworld,
		mgl32.Vec3{0.5, 1, 0.5}, mgl32.Vec3{3.5, 1, 0.5}, true,
	)
	if len(engine.snowFootprintPending) != 1 {
		t.Fatalf("单步跨 3 格产生了 %d 个候选，想要至多 1 个", len(engine.snowFootprintPending))
	}
	engine.noteSnowFootprint(
		tracker, core.Overworld,
		mgl32.Vec3{3.5, 1, 0.5}, mgl32.Vec3{3.5, 1, 0.5}, true,
	)
	if len(engine.snowFootprintPending) != 1 {
		t.Fatalf("采样后的静止续调用又追加了候选: %d", len(engine.snowFootprintPending))
	}

	result := TickResult{}
	pending := engine.newMutation()
	engine.settleSnowFootprints(pending)
	engine.finishChanges(pending, &result)

	if got := fluidBlockAt(t, engine, foot); got != core.SnowLayer2BlockID {
		t.Fatalf("落足格 = %d，想要降为 2 档", got)
	}
	if got := fluidBlockAt(t, engine, passed); got != core.SnowLayer3BlockID {
		t.Fatalf("途经格 = %d，想要不被顺带削掉", got)
	}
	if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 ||
		result.Changes[0].Changes[0] != (BlockChange{Position: foot, Block: core.SnowLayer2BlockID}) {
		t.Fatalf("脚印广播形状 = %+v，想要恰好一条 (落足格, 2 档)", result.Changes)
	}
	if len(engine.snowFootprintPending) != 0 {
		t.Fatalf("结算后暂存未清空: %d", len(engine.snowFootprintPending))
	}
}

// TestSnowFootprintSkipsUnloadedChunk 锚定「未加载区块不写、不同步加载」：候
// 选格落在未加载区块时结算静默跳过，不产生任何变更也不清掉既有内容。
func TestSnowFootprintSkipsUnloadedChunk(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	engine.snowFootprintPending = append(engine.snowFootprintPending, snowFootprintCell{
		dimension: core.Overworld,
		position:  core.BlockPos{X: 4096, Y: 1, Z: 4096},
	})

	result := TickResult{}
	pending := engine.newMutation()
	engine.settleSnowFootprints(pending)
	engine.finishChanges(pending, &result)

	if len(result.Changes) != 0 {
		t.Fatalf("未加载区块的候选产生了变更: %+v", result.Changes)
	}
	if len(engine.snowFootprintPending) != 0 {
		t.Fatalf("结算后暂存未清空: %d", len(engine.snowFootprintPending))
	}
}

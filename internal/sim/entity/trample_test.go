package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件是踩踏（farmland-trample）的行为主题测试：落在耕地上把耕地踩回泥土、
// 正上方作物按采掘同形规则连带掉落、掉落容量不足时整格放弃、落地边沿语义
// （持续站立不重复触发、跳起再落重新触发）、双玩家同格落地的幂等结算、跨格
// 覆盖全部被覆盖耕地。掉落数量与采掘逐件相同的性质由
// property_trample_yield_parity_test.go 单独锁定，这里只断言形状与区间。
//
// 夹具全部复用既有 helper：readyMovementPlayer 构造 flat 世界玩家（y=0 全草），
// SetBlockForTest 写方块，fillMiningDropsLeavingOneSlot 制造容量不足，
// miningDropTotals / tillBlockAt / miningTargetRecord 读结果。
//
// 需要保持原样的用例一律用干耕地 + 成熟小麦：干耕地在无水邻域下被随机 tick
// 抽中也不会变（farmlandIsWet 恒假、双向转换无事发生），成熟作物被抽中同样
// 不再生长，两者是随机 tick 的不动点，「多个 tick 后仍原样」因此是确定性断言。

// trampleFoot 是踩踏用例的落脚格（readyMovementPlayer 的 flat 世界里 y=0 全是
// 草，用例把它替换成耕地），trampleCrop 是它正上方的作物格。
var (
	trampleFoot = core.BlockPos{X: 0, Y: 0, Z: 0}
	trampleCrop = core.BlockPos{X: 0, Y: 1, Z: 0}
)

// landPlayerFromAbove 把玩家瞬移到 foot 列中心上方 3 格处并推进 tick 直到落地，
// 返回落地那一步的 TickResult。高度 3 保证摔落伤害为零（安全高度内），测试只
// 观察踩踏，不与死亡/伤害路径耦合。
func landPlayerFromAbove(
	t *testing.T,
	engine *Engine,
	session SessionID,
	foot core.BlockPos,
) TickResult {
	t.Helper()
	return landPlayerAt(t, engine, session, mgl32.Vec3{
		float32(foot.X) + 0.5, float32(foot.Y) + 4, float32(foot.Z) + 0.5,
	})
}

// landPlayerAt 把玩家瞬移到指定位置并推进 tick 直到落地，返回落地那一步的
// TickResult。跨格用例需要精确控制落点（如格心交界处），不能只按列中心落。
func landPlayerAt(
	t *testing.T,
	engine *Engine,
	session SessionID,
	position mgl32.Vec3,
) TickResult {
	t.Helper()
	engine.SetPlayerPositionForTest(session, position)
	for range 400 {
		result := engine.Step()
		if onlyMovementPlayer(t, result).State.OnGround {
			return result
		}
	}
	t.Fatalf("玩家在 400 tick 内未落地: 起点 %+v", position)
	return TickResult{}
}

// landBothPlayersFromAbove 把两名玩家同时瞬移到 foot 列中心上方同一高度并推进
// tick，直到两人在**同一个** Step 内落地，返回落地那一步的 TickResult。任一
// 玩家先行落地直接判夹具失败——那不是「同 tick 落地边沿」的双玩家场景。
//
// 落地观察直读 `engine.sessions` 的权威 `OnGround` 而不能复用 `landPlayerAt`：
// 后者依赖的 `onlyMovementPlayer` 硬性断言 TickResult 里恰好一名玩家。
func landBothPlayersFromAbove(
	t *testing.T,
	engine *Engine,
	first, second SessionID,
	foot core.BlockPos,
) TickResult {
	t.Helper()
	start := mgl32.Vec3{
		float32(foot.X) + 0.5, float32(foot.Y) + 4, float32(foot.Z) + 0.5,
	}
	engine.SetPlayerPositionForTest(first, start)
	engine.SetPlayerPositionForTest(second, start)
	for range 400 {
		result := engine.Step()
		firstGrounded := engine.sessions[first].player.state.OnGround
		secondGrounded := engine.sessions[second].player.state.OnGround
		if !firstGrounded && !secondGrounded {
			continue
		}
		if !firstGrounded || !secondGrounded {
			t.Fatalf("两名玩家未在同一 tick 落地: first=%v second=%v",
				firstGrounded, secondGrounded)
		}
		return result
	}
	t.Fatalf("两名玩家在 400 tick 内未落地: 起点 %+v", start)
	return TickResult{}
}

// assertTrampleBroadcasted 钉死「踩踏变更必须经既有 recordChange 汇入本 tick 的
// 广播批次」：只改内存不广播同样能让方块断言全绿（与翻地/种植用例同风格）。
// 落地 tick 里除踩踏外没有其他方块写者，因此形状可以取精确值。
func assertTrampleBroadcasted(t *testing.T, result TickResult, farmland bool) {
	t.Helper()
	changes := []BlockChange{
		{Position: trampleFoot, Block: core.DirtID},
		{Position: trampleCrop, Block: core.AirID},
	}
	if !farmland {
		changes = changes[:1]
	}
	if len(result.Changes) != 1 || len(result.Changes[0].Changes) != len(changes) {
		t.Fatalf("踩踏广播形状 = %+v，想要 %d 条变更", result.Changes, len(changes))
	}
	for index, want := range changes {
		if result.Changes[0].Changes[index] != want {
			t.Fatalf("广播[%d] = %+v，想要 %+v", index, result.Changes[0].Changes[index], want)
		}
	}
}

// TestTrampleLandingOnMatureWheatDestroysField 覆盖 Scenario「落在成熟麦田中央」：
// 湿耕地变泥土、成熟作物变空气、掉落含 1..3 小麦与 1..3 种子（数量由确定性哈希
// 决定，精确重放由性质测试锁定），且两条变更都进了同一批广播。
func TestTrampleLandingOnMatureWheatDestroysField(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetBlockForTest(trampleFoot, core.FarmlandWetID)
	engine.SetBlockForTest(trampleCrop, core.WheatStage7ID)

	result := landPlayerFromAbove(t, engine, session, trampleFoot)

	if got := tillBlockAt(t, engine, trampleFoot); got != core.DirtID {
		t.Fatalf("被踩耕地 = %d，想要泥土 %d", got, core.DirtID)
	}
	if got := tillBlockAt(t, engine, trampleCrop); got != core.AirID {
		t.Fatalf("被踩耕地上的作物 = %d，想要空气", got)
	}
	got := miningDropTotals(miningTargetRecord(t, engine, trampleCrop).Chunk)
	if len(got) != 2 ||
		got[core.ItemWheat] < 1 || got[core.ItemWheat] > 3 ||
		got[core.ItemWheatSeeds] < 1 || got[core.ItemWheatSeeds] > 3 {
		t.Fatalf("踩踏掉落 = %+v，想要 1..3 小麦 + 1..3 种子", got)
	}
	assertTrampleBroadcasted(t, result, true)
}

// TestTrampleLandingOnBareFarmlandDropsNothing 覆盖 Scenario「落在空耕地上」：
// 干耕地变泥土且零掉落——耕地转泥土是方块转换而非破坏，本身 MUST NOT 掉落。
func TestTrampleLandingOnBareFarmlandDropsNothing(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetBlockForTest(trampleFoot, core.FarmlandDryID)

	result := landPlayerFromAbove(t, engine, session, trampleFoot)

	if got := tillBlockAt(t, engine, trampleFoot); got != core.DirtID {
		t.Fatalf("被踩空耕地 = %d，想要泥土 %d", got, core.DirtID)
	}
	if got := miningDropTotals(miningTargetRecord(t, engine, trampleFoot).Chunk); len(got) != 0 {
		t.Fatalf("空耕地踩踏产生了掉落: %+v", got)
	}
	assertTrampleBroadcasted(t, result, false)
}

// TestTrampleCapacityFailureKeepsCellIntact 覆盖 Scenario「掉落容量不足时不破坏」：
// 掉落槽只剩一个空位时（小麦放得下、种子放不下），整格 MUST 保持原样——绝不
// 出现「耕地已变泥土、作物已消失但没有对应掉落物」的部分结算。区块字节与
// revision 逐项不变；掉落侧比较**物品计数**而不是 `DropsHash`——预填的掉落物在
// 下落等待的多个 tick 里正常老化，年龄字段本来就该变，而部分结算只会体现为
// 计数里多出小麦或种子。
func TestTrampleCapacityFailureKeepsCellIntact(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetBlockForTest(trampleFoot, core.FarmlandDryID)
	engine.SetBlockForTest(trampleCrop, core.WheatStage7ID)
	fillMiningDropsLeavingOneSlot(engine, trampleCrop)
	record := miningTargetRecord(t, engine, trampleCrop)
	beforeHash := record.Chunk.Hash()
	beforeRevision := record.Revision
	beforeDrops := miningDropTotals(record.Chunk)

	landPlayerFromAbove(t, engine, session, trampleFoot)

	if got := tillBlockAt(t, engine, trampleFoot); got != core.FarmlandDryID {
		t.Fatalf("容量不足时耕地被破坏: %d", got)
	}
	if got := tillBlockAt(t, engine, trampleCrop); got != core.WheatStage7ID {
		t.Fatalf("容量不足时作物消失: %d", got)
	}
	if got := record.Chunk.Hash(); got != beforeHash || record.Revision != beforeRevision {
		t.Fatalf("容量不足修改了区块或 revision: hash=%x/%x revision=%d/%d",
			got, beforeHash, record.Revision, beforeRevision)
	}
	if got := miningDropTotals(record.Chunk); !equalMiningDropTotals(got, beforeDrops) {
		t.Fatalf("容量不足产生了掉落: %+v，想要保持 %+v", got, beforeDrops)
	}
}

// TestTrampleEdgeSemantics 覆盖 Scenario「持续站立不重复触发」与「跳起再落重新
// 触发」。两条语义共享一个夹具才能互相印证：
//
//  1. 容量不足的落地被整格放弃后，清空两个掉落槽腾出容量，继续原地站立多个
//     tick——若踩踏被错误地挂在「在地面」而不是「落地边沿」，此时容量已可用，
//     耕地会在站立期间被踩掉，用例在这里就红。
//  2. 随后跳起再落产生新边沿，容量可用下结算成功：耕地变泥土、作物掉落。
func TestTrampleEdgeSemantics(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetBlockForTest(trampleFoot, core.FarmlandDryID)
	engine.SetBlockForTest(trampleCrop, core.WheatStage7ID)
	fillMiningDropsLeavingOneSlot(engine, trampleCrop)
	landPlayerFromAbove(t, engine, session, trampleFoot)
	if got := tillBlockAt(t, engine, trampleFoot); got != core.FarmlandDryID {
		t.Fatalf("前置失败：容量不足的落地破坏了耕地: %d", got)
	}

	// 清空全部掉落槽：容量已足，且让最终断言可以取「恰好两类产物」的精确形状。
	key := core.ChunkKey{Dimension: core.Overworld, Pos: trampleCrop.Chunk()}
	for slot := range core.DropsPerChunk {
		engine.SetChunkDropForTest(key, slot, world.DropSlot{Generation: 1})
	}

	for range 20 {
		engine.Step()
	}
	if got := tillBlockAt(t, engine, trampleFoot); got != core.FarmlandDryID {
		t.Fatalf("持续站立期间踩踏再次触发: %d，想要耕地保持", got)
	}
	if got := tillBlockAt(t, engine, trampleCrop); got != core.WheatStage7ID {
		t.Fatalf("持续站立期间作物被移除: %d", got)
	}

	// 跳起再落：新边沿必须重新结算（容量已可用，本笔必然成功）。
	engine.Enqueue(Command{Session: session, Sequence: 1, Kind: CommandPlayerInput, Jump: true})
	landed := false
	for range 100 {
		result := engine.Step()
		if onlyMovementPlayer(t, result).State.OnGround {
			landed = true
			break
		}
	}
	if !landed {
		t.Fatal("跳跃后 100 tick 内未观察到再次落地")
	}
	if got := tillBlockAt(t, engine, trampleFoot); got != core.DirtID {
		t.Fatalf("跳起再落后耕地 = %d，想要重新结算为泥土", got)
	}
	if got := tillBlockAt(t, engine, trampleCrop); got != core.AirID {
		t.Fatalf("跳起再落后作物 = %d，想要空气", got)
	}
	got := miningDropTotals(miningTargetRecord(t, engine, trampleCrop).Chunk)
	if len(got) != 2 ||
		got[core.ItemWheat] < 1 || got[core.ItemWheat] > 3 ||
		got[core.ItemWheatSeeds] < 1 || got[core.ItemWheatSeeds] > 3 {
		t.Fatalf("重新触发的掉落 = %+v，想要 1..3 小麦 + 1..3 种子", got)
	}
}

// TestTrampleDualPlayerLandingSameCellIsIdempotent 锚定 spec 的 MUST 幂等条款：
// 「同格被多名玩家的落地同时覆盖时结算 MUST 幂等且结果与结算次序无关」。
//
// 夹具让两名玩家站在完全相同的位置上同 tick 落地：他们的覆盖格集合逐格相同，
// `tramplePending` 里因此出现两条坐标完全一致的候选记录——这正是「结算次序
// 无关」最锋利的输入形态，因为无论哪条先结算，另一条读到的都已经是泥土。
// 断言三件事：
//
//  1. 恰好一次结算——耕地变泥土、作物变空气，且落地 tick 的广播恰好只有
//     [(耕地, 泥土), (作物, 空气)] 两条变更（任何二次结算都会留下重复条目）；
//  2. 掉落恰好一批——两类产物数量都在 [1,3] 内；
//  3. 结果与单玩家落地同一格完全一致——对照引擎用同一世界种子、在同一个
//     权威 tick 上结算（激活第二名玩家消耗的步数在对照侧空转补齐，下落物理
//     逐 tick 相同，落地 tick 相等断言钉死对齐），`cropYieldRolls` 的哈希输入
//     逐项相同，掉落计数必须逐件相等；任何双重结算都会以计数翻倍在这里暴露。
func TestTrampleDualPlayerLandingSameCellIsIdempotent(t *testing.T) {
	engine, first := readyMovementPlayer(t)
	const second = SessionID(2)
	current := PlayerLocation{Dimension: core.Overworld, Position: mgl32.Vec3{2.5, 1, 0.5}}
	engine.RegisterPlayer(second, PlayerRestore{Current: &current, SpawnDimension: core.Overworld})
	// 等第二名玩家激活并站稳：这一段里世界上还没有耕地，任何落地边沿都踩在
	// 草上，不产生踩踏结算；消耗的步数记下来给对照引擎空转补齐 tick 对齐。
	warmup := 0
	for engine.sessions[second].player.lifecycle != PlayerActive ||
		!engine.sessions[second].player.state.OnGround {
		engine.Step()
		warmup++
		if warmup > 32 {
			t.Fatal("第二名玩家在 32 tick 内未激活站稳")
		}
	}

	controlEngine, controlSession := readyMovementPlayer(t)
	for range warmup {
		controlEngine.Step()
	}

	engine.SetBlockForTest(trampleFoot, core.FarmlandWetID)
	engine.SetBlockForTest(trampleCrop, core.WheatStage7ID)
	controlEngine.SetBlockForTest(trampleFoot, core.FarmlandWetID)
	controlEngine.SetBlockForTest(trampleCrop, core.WheatStage7ID)

	landing := landBothPlayersFromAbove(t, engine, first, second, trampleFoot)
	controlLanding := landPlayerFromAbove(t, controlEngine, controlSession, trampleFoot)

	if got, want := landing.Tick, controlLanding.Tick; got != want {
		t.Fatalf("双玩家与单玩家落地 tick 不对齐: %d vs %d（掉落哈希输入将分岔）", got, want)
	}
	if got := tillBlockAt(t, engine, trampleFoot); got != core.DirtID {
		t.Fatalf("双玩家落地后耕地 = %d，想要泥土 %d", got, core.DirtID)
	}
	if got := tillBlockAt(t, engine, trampleCrop); got != core.AirID {
		t.Fatalf("双玩家落地后作物 = %d，想要空气", got)
	}
	assertTrampleBroadcasted(t, landing, true)

	got := miningDropTotals(miningTargetRecord(t, engine, trampleCrop).Chunk)
	if len(got) != 2 ||
		got[core.ItemWheat] < 1 || got[core.ItemWheat] > 3 ||
		got[core.ItemWheatSeeds] < 1 || got[core.ItemWheatSeeds] > 3 {
		t.Fatalf("双玩家掉落 = %+v，想要恰好一批 1..3 小麦 + 1..3 种子", got)
	}
	if farmland := tillBlockAt(t, controlEngine, trampleFoot); farmland != core.DirtID {
		t.Fatalf("对照引擎耕地 = %d，想要泥土 %d", farmland, core.DirtID)
	}
	if crop := tillBlockAt(t, controlEngine, trampleCrop); crop != core.AirID {
		t.Fatalf("对照引擎作物 = %d，想要空气", crop)
	}
	control := miningDropTotals(miningTargetRecord(t, controlEngine, trampleCrop).Chunk)
	if !equalMiningDropTotals(got, control) {
		t.Fatalf("双玩家掉落与单玩家不一致: %+v vs %+v", got, control)
	}
}

// TestTrampleDestroysNewCrops 覆盖 IsCrop 新区间：落在马铃薯/胡萝卜上的踩踏与小麦同形。
func TestTrampleDestroysNewCrops(t *testing.T) {
	for _, tc := range []struct {
		name string
		crop core.BlockID
		item core.ItemID
	}{
		{"PotatoStage3", core.PotatoStage3ID, core.ItemPotato},
		{"CarrotStage3", core.CarrotStage3ID, core.ItemCarrot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := readyMovementPlayer(t)
			engine.SetBlockForTest(trampleFoot, core.FarmlandDryID)
			engine.SetBlockForTest(trampleCrop, tc.crop)
			result := landPlayerFromAbove(t, engine, session, trampleFoot)
			if got := tillBlockAt(t, engine, trampleFoot); got != core.DirtID {
				t.Fatalf("被踩耕地 = %d，想要泥土 %d", got, core.DirtID)
			}
			if got := tillBlockAt(t, engine, trampleCrop); got != core.AirID {
				t.Fatalf("被踩作物 = %d，想要空气", got)
			}
			got := miningDropTotals(miningTargetRecord(t, engine, trampleCrop).Chunk)
			if got[tc.item] != 1 || len(got) != 1 {
				t.Fatalf("踩踏掉落 = %+v，想要 1 %d", got, tc.item)
			}
			assertTrampleBroadcasted(t, result, true)
		})
	}
}

// TestTrampleCrossCellCoverageSettlesAllCoveredFarmland 覆盖 Scenario「跨格站立
// 踩踏全部覆盖格」：玩家落在四列交界处，碰撞盒（半宽 0.3）水平覆盖 2×2 列，
// 其中对角两格是耕地，两格 MUST 都被结算为泥土——只判玩家中心柱会漏掉半边。
func TestTrampleCrossCellCoverageSettlesAllCoveredFarmland(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	near := core.BlockPos{X: 0, Y: 0, Z: 0}
	far := core.BlockPos{X: 1, Y: 0, Z: 1}
	engine.SetBlockForTest(near, core.FarmlandDryID)
	engine.SetBlockForTest(far, core.FarmlandDryID)

	landPlayerAt(t, engine, session, mgl32.Vec3{1, 4, 1})

	if got := tillBlockAt(t, engine, near); got != core.DirtID {
		t.Fatalf("近侧耕地 = %d，想要泥土 %d", got, core.DirtID)
	}
	if got := tillBlockAt(t, engine, far); got != core.DirtID {
		t.Fatalf("远侧耕地 = %d，想要泥土 %d", got, core.DirtID)
	}
}

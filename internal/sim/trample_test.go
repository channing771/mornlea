package sim

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件是踩踏（farmland-trample）的行为主题测试：落在耕地上把耕地踩回泥土、
// 正上方作物按采掘同形规则连带掉落、掉落容量不足时整格放弃、落地边沿语义
// （持续站立不重复触发、跳起再落重新触发）。掉落数量与采掘逐件相同的性质由
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

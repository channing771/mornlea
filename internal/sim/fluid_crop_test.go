package sim

import (
	"sort"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件覆盖变更 flood-destroys-crops 在权威模拟侧的可观察行为：作物格被流动水
// 替换时按采掘同表产出掉落物（spec Scenario「冲毁按采掘同表产出掉落物」）、单
// tick 内原子成立、「掉落槽满时拒绝破坏并稍后重试」、耕地保持原编号且湿判定联
// 动成立。实现挂在 `fluidWorld.SetBlock` 这一唯一写入汇聚点上（见 fluid_crop.go）。
//
// 夹具刻意把作物放在 z=8 一列：玩家出生在区块 (0,0) 原点附近，冲毁产生的掉落物
// 距其远超拾取半径，断言窗口内不会被拾取路径干扰。

// fluidCropFarmland 是夹具中作物格正下方的耕地。
var fluidCropFarmland = core.BlockPos{X: 0, Y: 0, Z: 8}

// fluidCropCell 是夹具中的作物格。
var fluidCropCell = core.BlockPos{X: 0, Y: 1, Z: 8}

// readyFluidCropWorld 构造一名 active 玩家与一片已就绪的平坦世界，并把耕地与
// 指定阶段的作物种进区块 (0,0) 的 z=8 列。
//
// 夹具**刻意不含任何流体**：水源由各用例在自己的前置准备（例如填满掉落槽）完成
// 之后再放置，使「先拒绝、后释放槽位」的时序完全受测者控制——若种子阶段就带水，
// 构造期的 12 个 Step 可能已经把冲毁做完，容量用例的前提就不成立了。
func readyFluidCropWorld(
	t *testing.T,
	crop core.BlockID,
) (*Engine, SessionID) {
	t.Helper()
	seed := map[core.BlockPos]core.BlockID{
		fluidCropFarmland: core.FarmlandDryID,
		fluidCropCell:     crop,
	}
	engine, session, _ := readyFluidPlayer(t, seed, nil)
	return engine, session
}

// floodFluidCropFrom 把 source 写成水源并显式唤醒它。
//
// `engine.SetBlockForTest` 绕过 recordChange，因此写入本身不会触发流体入队；与
// buildFluidChannel 的既有实践一致，这里显式调用 `engine.enqueueFluidUpdate`，
// 让水源从下一个延迟窗口开始按正常规则流动。
func floodFluidCropFrom(engine *Engine, source core.BlockPos) {
	engine.SetBlockForTest(source, core.WaterSourceID)
	engine.enqueueFluidUpdate(core.Overworld, source)
}

// fluidCropFloodSourceAbove 返回垂直冲毁用的水源位置：作物格正上方一格。
func fluidCropFloodSourceAbove() core.BlockPos {
	return core.BlockPos{X: fluidCropCell.X, Y: fluidCropCell.Y + 1, Z: fluidCropCell.Z}
}

// stepUntilFluidCropFlooded 推进权威 tick 直到 position 变成 want；返回发生该次
// 写入的权威 tick 值（结算 tick）。Step 返回时 `Engine.tick.Load()` 已指向下一
// tick，而步内的一切读取——含 `settleFloodedCrop` 调 `cropYieldRolls` 的取值点
// ——都发生在自增之前，因此返回值恰为「观察到翻转的那一步的 Load() − 1」，供
// 用例按夹具已知输入现算期望产量。超过上界仍未变化直接失败，避免在不收敛的
// 场景里静默通过。
func stepUntilFluidCropFlooded(
	t *testing.T,
	engine *Engine,
	position core.BlockPos,
	want core.BlockID,
) uint64 {
	t.Helper()
	for range 200 {
		engine.Step()
		if got := fluidBlockAt(t, engine, position); got == want {
			return engine.tick.Load() - 1
		}
	}
	t.Fatalf("推进 200 tick 后 %+v 仍未变成 %d", position, want)
	return 0
}

// fluidCropStack 是掉落物集合比较用的投影：只保留「哪一堆、什么物品、多少个」，
// 忽略 `DropID.Generation` 与槽位号这类随分配次序变化的字段。
type fluidCropStack struct {
	BlockIndex uint32
	Item       core.ItemID
	Count      uint8
}

// fluidCropDropsOf 返回该会话兴趣范围内的全部掉落物投影，按稳定顺序排序。
func fluidCropDropsOf(engine *Engine, session SessionID) []fluidCropStack {
	snapshots := engine.AppendSessionDrops(session, nil)
	stacks := make([]fluidCropStack, 0, len(snapshots))
	for _, snapshot := range snapshots {
		stacks = append(stacks, fluidCropStack{
			BlockIndex: snapshot.BlockIndex,
			Item:       snapshot.Item,
			Count:      snapshot.Count,
		})
	}
	sort.Slice(stacks, func(i, j int) bool {
		if stacks[i].BlockIndex != stacks[j].BlockIndex {
			return stacks[i].BlockIndex < stacks[j].BlockIndex
		}
		if stacks[i].Item != stacks[j].Item {
			return stacks[i].Item < stacks[j].Item
		}
		return stacks[i].Count < stacks[j].Count
	})
	return stacks
}

// expectFluidCropDrops 断言当前掉落物集合恰好等于想要的一组堆。
func expectFluidCropDrops(
	t *testing.T,
	engine *Engine,
	session SessionID,
	want ...fluidCropStack,
) {
	t.Helper()
	got := fluidCropDropsOf(engine, session)
	cropIndex, indexed := world.ChunkBlockIndex(fluidCropCell)
	if !indexed {
		t.Fatal("作物格没有区块索引")
	}
	for index := range want {
		want[index].BlockIndex = cropIndex
	}
	sort.Slice(want, func(i, j int) bool {
		if want[i].Item != want[j].Item {
			return want[i].Item < want[j].Item
		}
		return want[i].Count < want[j].Count
	})
	if len(got) != len(want) {
		t.Fatalf("掉落物集合=%+v，想要 %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("掉落物[%d]=%+v，想要 %+v（全集=%+v）", index, got[index], want[index], got)
		}
	}
}

// TestFluidCropVerticalFloodYieldsMatureHarvest 覆盖 spec Scenario
// 「冲毁按采掘同表产出掉落物」的成熟分支：水源悬在成熟小麦正上方，垂直传播把
// 作物格写成最强流动水。期望数量不硬编码：按夹具已知的 (seed, 结算 tick, 维度,
// 坐标) 调包内共享的 `cropYieldRolls` 现算（与 trample_test 同做法）——本用例
// 锁的是「冲毁读的是与采掘同一条产量哈希流」，与 mining 路径的数值对齐由
// property 级 parity 测试锁定。
func TestFluidCropVerticalFloodYieldsMatureHarvest(t *testing.T) {
	engine, session := readyFluidCropWorld(t, core.WheatStage7ID)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())

	settleTick := stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	wheatCount, seedCount := cropYieldRolls(
		engine.seed, settleTick, core.Overworld, fluidCropCell,
	)
	expectFluidCropDrops(t, engine, session,
		fluidCropStack{Item: core.ItemWheat, Count: wheatCount},
		fluidCropStack{Item: core.ItemWheatSeeds, Count: seedCount},
	)
}

// TestFluidCropHorizontalFloodYieldsImmatureSeed 覆盖同一 Scenario 的未成熟分支：
// 地面水源沿地水平传播进入相邻未成熟作物格。evalCell 里源的流体等级读作 0，
// 其水平邻居因此得到等级 1（与落到空气上的编号一致），产出 1 颗种子。
func TestFluidCropHorizontalFloodYieldsImmatureSeed(t *testing.T) {
	engine, session := readyFluidCropWorld(t, core.WheatStage3ID)
	source := core.BlockPos{
		X: fluidCropCell.X + 1, Y: fluidCropCell.Y, Z: fluidCropCell.Z,
	}
	floodFluidCropFrom(engine, source)

	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	expectFluidCropDrops(t, engine, session,
		fluidCropStack{Item: core.ItemWheatSeeds, Count: 1},
	)
}

// fluidCropDualSourceFloor 是双源夹具里水平候选 F 的承重底：泥土保证 F 下方
// 不可替换，`evalCell` 因此走水平传播分支而不是垂直分支。
var fluidCropDualSourceFloor = core.BlockPos{X: 1, Y: 0, Z: 8}

// fluidCropFeeder 是双源夹具的水平候选源：持有等级 1 流动水的 F 格，向西侧的
// 作物格写入等级 2 候选。
var fluidCropFeeder = core.BlockPos{X: 1, Y: 1, Z: 8}

// fluidCropFeederSupport 是 F 正上方的隐藏水源：只为 `flowingSurvives` 提供存活
// 支撑，刻意不直接入队——它一旦求值就会向西铺开等级 1 的水，破坏「两个候选
// 同一批到达」的前提。fluidWorld 只读方块不看队列，支撑判定照常成立。
var fluidCropFeederSupport = core.BlockPos{X: 1, Y: 2, Z: 8}

// TestFluidCropSameTickDualSourceMergesToStrongestAndSettlesOnce 锁定 spec
// Scenario「同 tick 冲突写入取最强者」的 AND 子句：写往同一作物格的多个候选按
// 同一规则合并，冲毁结算恰好发生一次。
//
// 夹具让两个**不同等级**的候选在同一批 Advance 里争抢同一成熟作物格：
//
//   - 垂直候选 A（fluidCropFloodSourceAbove）：作物正上方的水源，垂直优先
//     写入等级 1；
//   - 水平候选 F（fluidCropFeeder）：东侧相邻的等级 1 流动水（上方藏一格
//     不入队的水源作支撑、下方泥土挡住垂直分支），水平递减写入等级 2。
//
// 两格在同一个准备步内入队（同一 now、同一 delay），因此两个候选落在同一次
// 合并批次里。断言三件事：
//
//  1. 作物格在整个收敛窗口内恰好广播一笔变更——合并发生在提交之前；若实现
//     退化成「逐候选直接落笔」，弱候选先冲毁一次、强候选再改写一次，两笔变更
//     在这里必红；
//  2. 最终生效值是最强者等级 1——合并语义本身；
//  3. 掉落物恰好一批且与 `cropYieldRolls` 在结算 tick 上的现算值逐件相等
//     ——冲毁恰好结算一次，且读的仍是采掘同一条产量哈希流。
func TestFluidCropSameTickDualSourceMergesToStrongestAndSettlesOnce(t *testing.T) {
	engine, session := readyFluidCropWorld(t, core.WheatStage7ID)
	engine.SetBlockForTest(fluidCropDualSourceFloor, core.DirtID)
	engine.SetBlockForTest(fluidCropFeederSupport, core.WaterSourceID)
	engine.SetBlockForTest(fluidCropFeeder, core.WaterLevel1ID)
	engine.SetBlockForTest(fluidCropFloodSourceAbove(), core.WaterSourceID)

	// 同一时刻入队两个候选源。enqueueFluidUpdate 的六邻扩散会把夹具周边一并
	// 入队，但那些格要么非流体（空写入）、要么不指向作物格，不影响断言。
	engine.enqueueFluidUpdate(core.Overworld, fluidCropFloodSourceAbove())
	engine.enqueueFluidUpdate(core.Overworld, fluidCropFeeder)

	const settleTicks = 200
	const stabilizeTicks = 16
	cropWrites := 0
	settleTick := uint64(0)
	flooded := false
	for range settleTicks + stabilizeTicks {
		result := engine.Step()
		for _, batch := range result.Changes {
			for _, change := range batch.Changes {
				if change.Position == fluidCropCell {
					cropWrites++
				}
			}
		}
		if !flooded && core.IsFluid(fluidBlockAt(t, engine, fluidCropCell)) {
			flooded = true
			settleTick = engine.tick.Load() - 1
		}
	}
	if !flooded {
		t.Fatalf("收敛窗口内作物格未被冲毁，仍是 %d", fluidBlockAt(t, engine, fluidCropCell))
	}
	if cropWrites != 1 {
		t.Fatalf("作物格被广播 %d 笔变更，想要恰好 1（合并先于提交）", cropWrites)
	}
	if got := fluidBlockAt(t, engine, fluidCropCell); got != core.WaterLevel1ID {
		t.Fatalf("作物格最终为 %d，想要最强候选 %d", got, core.WaterLevel1ID)
	}
	wheatCount, seedCount := cropYieldRolls(
		engine.seed, settleTick, core.Overworld, fluidCropCell,
	)
	expectFluidCropDrops(t, engine, session,
		fluidCropStack{Item: core.ItemWheat, Count: wheatCount},
		fluidCropStack{Item: core.ItemWheatSeeds, Count: seedCount},
	)
}

// fluidCropSlotView 是掉落槽的语义投影：剔除 `AgeTicks` 与 `PickupDelayTicks`
// 这两个每 tick 必然推进的字段。容量用例要证明的是「没有任何新的结算副作用」，
// 而不是槽位逐字节冻结——老化计数照常前进是既有掉落物通道的正常行为。
type fluidCropSlotView struct {
	Active     bool
	Item       core.ItemID
	Count      uint8
	Durability uint16
	BlockIndex uint32
}

// fluidCropChunkSlots 读取区块 (0,0) 全部掉落槽的语义投影。
func fluidCropChunkSlots(engine *Engine) [core.DropsPerChunk]fluidCropSlotView {
	chunk := engine.dimensions[core.Overworld].records[core.ChunkPos{}].Chunk
	var views [core.DropsPerChunk]fluidCropSlotView
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		views[slot] = fluidCropSlotView{
			Active:     drop.Active,
			Item:       drop.Stack.Item,
			Count:      drop.Stack.Count,
			Durability: drop.Stack.Durability,
			BlockIndex: drop.BlockIndex,
		}
	}
	return views
}

// fillFluidCropDropSlots 用 32 个占位石头堆填满区块 (0,0) 的全部掉落槽。
// 占位堆放在 (15,0,15)，远离作物列，避免与被测掉落物混淆。
func fillFluidCropDropSlots(t *testing.T, engine *Engine) {
	t.Helper()
	blockIndex, indexed := world.ChunkBlockIndex(core.BlockPos{X: 15, Y: 0, Z: 15})
	if !indexed {
		t.Fatal("占位方块没有区块索引")
	}
	filler := world.DropSlot{
		Generation:       1,
		Active:           true,
		Stack:            core.ItemStack{Item: core.ItemStone, Count: 1},
		BlockIndex:       blockIndex,
		PickupDelayTicks: 10,
	}
	key := core.ChunkKey{Dimension: core.Overworld}
	for slot := range core.DropsPerChunk {
		engine.SetChunkDropForTest(key, slot, filler)
	}
}

// TestFluidCropCapacityFullRejectsAndRetriesUntilSlotFreed 覆盖 spec Scenario
// 「掉落槽满时拒绝破坏并稍后重试」：
//
//  1. 槽满期间冲毁 MUST NOT 发生：作物保持存在、格未被改写、槽位没有任何结算
//     副作用，且目标格仍被排程（队列不排空）；
//  2. 释放一个槽位后重试自然完成：作物被冲毁、种子落入腾出的槽位。
//
// 用未成熟作物：它的掉落恰好只有一堆（1 种子），「释放一个槽位」才精确对应
// 「刚好够这次结算」的最紧边界——成熟作物需要两堆，一个空槽必然再次拒绝，
// 测不出「释放即完成」这条性质。
func TestFluidCropCapacityFullRejectsAndRetriesUntilSlotFreed(t *testing.T) {
	engine, session := readyFluidCropWorld(t, core.WheatStage3ID)
	fillFluidCropDropSlots(t, engine)
	engine.farmlandMoisture = farmlandMoistureState{}
	watch := watchFarmlandMoistureCandidateAtPhase(engine, farmlandMoistureKey{
		dimension: core.Overworld,
		position:  fluidCropFarmland,
	})
	full := fluidCropChunkSlots(engine)
	active := 0
	for _, view := range full {
		if view.Active {
			active++
		}
	}
	if active != core.DropsPerChunk {
		t.Fatalf("夹具失效：仅 %d 个活跃槽，想要 %d", active, core.DropsPerChunk)
	}

	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())
	for range 40 {
		engine.Step()
	}
	engine.stepPhaseObserver = nil
	if got := fluidBlockAt(t, engine, fluidCropCell); got != core.WheatStage3ID {
		t.Fatalf("槽满期间作物格被改写为 %d，作物必须保持存在", got)
	}
	after := fluidCropChunkSlots(engine)
	if after != full {
		t.Fatalf("槽满期间出现结算副作用：before=%+v after=%+v", full, after)
	}
	if got := overworldFluidQueue(t, engine).Len(); got == 0 {
		t.Fatal("拒绝之后目标格没有被重新排程，释放槽位后将永远无法完成冲毁")
	}
	if !watch.phaseSeen {
		t.Fatal("容量拒绝窗口未经过湿度阶段观察点")
	}
	if watch.candidateSeen {
		t.Fatal("容量拒绝的作物写入在湿度阶段消费前产生了耕地候选")
	}

	// 释放一个槽位：重试到期后冲毁应当完成，种子占据腾出的槽位。
	engine.dimensions[core.Overworld].records[core.ChunkPos{}].Chunk.ClearDrop(
		core.DropsPerChunk - 1,
	)
	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)

	// 兴趣范围内此时同时存在占位石头与本次冲毁的产物，不能做全集相等断言；
	// 改为逐一分类：除占位堆外，必须恰好有一堆「作物格上的 1 颗小麦种子」。
	cropIndex, cropIndexed := world.ChunkBlockIndex(fluidCropCell)
	fillerIndex, fillerIndexed := world.ChunkBlockIndex(core.BlockPos{X: 15, Y: 0, Z: 15})
	if !cropIndexed || !fillerIndexed {
		t.Fatal("夹具方块没有区块索引")
	}
	seedStacks, fillers := 0, 0
	for _, drop := range fluidCropDropsOf(engine, session) {
		switch {
		case drop.BlockIndex == cropIndex &&
			drop.Item == core.ItemWheatSeeds && drop.Count == 1:
			seedStacks++
		case drop.BlockIndex == fillerIndex &&
			drop.Item == core.ItemStone && drop.Count == 1:
			fillers++
		default:
			t.Fatalf("意外掉落物 %+v", drop)
		}
	}
	if seedStacks != 1 {
		t.Fatalf("作物格上的种子堆=%d，想要恰好 1", seedStacks)
	}
	if fillers != core.DropsPerChunk-1 {
		t.Fatalf("占位堆=%d，想要保留 %d", fillers, core.DropsPerChunk-1)
	}
	final := fluidCropChunkSlots(engine)
	active = 0
	for _, view := range final {
		if view.Active {
			active++
		}
	}
	if active != core.DropsPerChunk {
		t.Fatalf("完成后活跃槽=%d，想要恰好回到 %d", active, core.DropsPerChunk)
	}
}

// TestFluidCropSettlesAtomicallyInSingleTick 锁定 spec Scenario「掉落槽满时拒绝
// 破坏并稍后重试」里的硬约束「任何时刻 MUST NOT 出现方块已被替换而掉落物未产出
// 的状态」：逐 tick 推进，在方块翻转的那个 tick 立即检查，冲毁与掉落必须同时可
// 观察；翻转之前也 MUST NOT 提前出现任何掉落物。
func TestFluidCropSettlesAtomicallyInSingleTick(t *testing.T) {
	engine, session := readyFluidCropWorld(t, core.WheatStage7ID)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())

	before := fluidBlockAt(t, engine, fluidCropCell)
	for range 200 {
		engine.Step()
		after := fluidBlockAt(t, engine, fluidCropCell)
		if after != before && !core.IsCrop(after) {
			// 方块在本 tick 翻转：这是唯一允许冲毁生效的时刻，
			// 掉落物必须在同一个 tick 内已经就位；期望数量按夹具已知输入调
			// `cropYieldRolls` 现算（取值点说明见 stepUntilFluidCropFlooded）。
			wheatCount, seedCount := cropYieldRolls(
				engine.seed, engine.tick.Load()-1, core.Overworld, fluidCropCell,
			)
			expectFluidCropDrops(t, engine, session,
				fluidCropStack{Item: core.ItemWheat, Count: wheatCount},
				fluidCropStack{Item: core.ItemWheatSeeds, Count: seedCount},
			)
			return
		}
		before = after
		if drops := fluidCropDropsOf(engine, session); len(drops) != 0 {
			t.Fatalf("冲毁发生前出现了掉落物：%+v", drops)
		}
	}
	t.Fatal("200 tick 内作物格未被冲毁，无法验证单 tick 原子性")
}

// TestFluidCropFloodPreservesFarmland 覆盖 spec Scenario「作物格可被流动水替换
// 并消失」的 AND 子句：作物消失后，其下方的耕地不被破坏，仍留在原格。
func TestFluidCropFloodPreservesFarmland(t *testing.T) {
	engine, _ := readyFluidCropWorld(t, core.WheatStage7ID)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())

	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	if got := fluidBlockAt(t, engine, fluidCropFarmland); !core.IsFarmland(got) {
		t.Fatalf("耕地格=%d，想要保持为耕地", got)
	}
}

// TestFluidCropFloodedFarmlandTurnsWet 覆盖湿判定联动：冲毁写入产生流体 membership
// 变化后，留在原格的耕地必须在同一 tick 转为湿耕地。
func TestFluidCropFloodedFarmlandTurnsWet(t *testing.T) {
	engine, _ := readyFluidCropWorld(t, core.WheatStage3ID)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())
	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)

	if got := fluidBlockAt(t, engine, fluidCropFarmland); got != core.FarmlandWetID {
		t.Fatalf("冲毁后的耕地未在同 tick 转湿，仍是 %d", got)
	}
}

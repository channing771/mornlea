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

// stepUntilFluidCropFlooded 推进权威 tick 直到 position 变成 want；超过上界仍未
// 变化直接失败，避免在不收敛的场景里静默通过。
func stepUntilFluidCropFlooded(
	t *testing.T,
	engine *Engine,
	position core.BlockPos,
	want core.BlockID,
) {
	t.Helper()
	for range 200 {
		engine.Step()
		if got := fluidBlockAt(t, engine, position); got == want {
			return
		}
	}
	t.Fatalf("推进 200 tick 后 %+v 仍未变成 %d", position, want)
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
// 作物格写成最强流动水，产出必须与玩家采掘完全相同——1 小麦 + 2 种子。
func TestFluidCropVerticalFloodYieldsMatureHarvest(t *testing.T) {
	engine, session := readyFluidCropWorld(t, core.WheatStage7ID)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())

	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	expectFluidCropDrops(t, engine, session,
		fluidCropStack{Item: core.ItemWheat, Count: 1},
		fluidCropStack{Item: core.ItemWheatSeeds, Count: 2},
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
			// 掉落物必须在同一个 tick 内已经就位。
			expectFluidCropDrops(t, engine, session,
				fluidCropStack{Item: core.ItemWheat, Count: 1},
				fluidCropStack{Item: core.ItemWheatSeeds, Count: 2},
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
	if got := fluidBlockAt(t, engine, fluidCropFarmland); got != core.FarmlandDryID {
		t.Fatalf("耕地格=%d，想要保持 %d", got, core.FarmlandDryID)
	}
}

// TestFluidCropFloodedFarmlandTurnsWet 覆盖湿判定联动：冲毁完成后，留在原格的
// 耕地在随机 tick 抽中时按既有规则转为湿耕地。
//
// 推进方式复用既有农业测试（readyCropWorld / TestFarmlandWetnessRangeBoundary）
// 的做法：调高 `RandomTicksPerSection` 让抽样快速命中耕地格，再用
// stepUntilBlock 等待转换完成——干湿转换挂在随机 tick 上，固定几步内命不中是
// 抽样机制的固有性质而非缺陷。
func TestFluidCropFloodedFarmlandTurnsWet(t *testing.T) {
	engine, _ := readyFluidCropWorld(t, core.WheatStage3ID)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())
	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)

	t.Cleanup(func() { SetTunables(DefaultTunables()) })
	wetSampling := ActiveTunables()
	wetSampling.RandomTicksPerSection = 64
	SetTunables(wetSampling)

	if _, ok := stepUntilBlock(engine, fluidCropFarmland, core.FarmlandWetID); !ok {
		t.Fatalf("冲毁后的耕地未被抽中转湿，仍是 %d",
			fluidBlockAt(t, engine, fluidCropFarmland))
	}
}

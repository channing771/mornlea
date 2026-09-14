package realm

// sapling_flood_test.go：流动水冲毁树苗的掉落与容量重试语义（spec 流体 delta
// 的「树苗格被流体写入时 MUST 视为树苗被冲毁并掉落 1 个树苗」与其容量拒绝
// Scenario）。夹具与作物/短草冲毁共用 `fluidCropEngine`：同一 z=8 列、同一
// 水源与唤醒方式，只是目标格换成 `SaplingID`，因此三者的流动规则与结算差异
// 可以逐字对照。

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// readyFluidSaplingWorld 构造 z=8 列上种着树苗的夹具世界。树苗下方是耕地，
// 因此随机 tick 的生长分支在冲毁前不会把它替换掉（支撑不符）。
func readyFluidSaplingWorld(t *testing.T) (*fluidCropEngine, fluidCropSession) {
	t.Helper()
	return newFluidCropEngine(t, core.SaplingID), fluidCropSession{}
}

// TestFluidSaplingVerticalFloodDropsOneSapling 覆盖 spec Scenario「树苗格可被
// 流动水替换并掉落一个树苗」的垂直分支：水源悬在树苗正上方，垂直传播把树苗格
// 写成最强流动水，同时恰好产生一个树苗掉落物。
func TestFluidSaplingVerticalFloodDropsOneSapling(t *testing.T) {
	engine, session := readyFluidSaplingWorld(t)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())

	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	expectFluidCropDrops(t, engine, session, fluidCropStack{Item: core.ItemSapling, Count: 1})
}

// TestFluidSaplingHorizontalFloodDropsOneSapling 覆盖同一 Scenario 的水平分支：
// 地面水源沿地水平传播进入相邻树苗格（源的水平邻居得到等级 1），同样掉落一个
// 树苗。
func TestFluidSaplingHorizontalFloodDropsOneSapling(t *testing.T) {
	engine, session := readyFluidSaplingWorld(t)
	source := core.BlockPos{
		X: fluidCropCell.X + 1, Y: fluidCropCell.Y, Z: fluidCropCell.Z,
	}
	floodFluidCropFrom(engine, source)

	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	expectFluidCropDrops(t, engine, session, fluidCropStack{Item: core.ItemSapling, Count: 1})
}

// TestFluidSaplingSameTickDualSourceMergesToStrongestOnceOneDrop 覆盖 spec
// Scenario「同 tick 冲突写入取最强者」的树苗子句：写往同一树苗格的多个候选按
// 同一规则合并，冲毁结算恰好发生一次、最多掉落 1 个树苗。夹具与作物/短草的双源
// 用例同构——垂直候选 A（等级 1）与水平候选 F（等级 2）同批争抢树苗格，断言
// 恰好一笔变更广播、最终值为最强等级 1、掉落恰好一堆树苗。
func TestFluidSaplingSameTickDualSourceMergesToStrongestOnceOneDrop(t *testing.T) {
	engine, session := readyFluidSaplingWorld(t)
	engine.SetBlockForTest(fluidCropDualSourceFloor, core.DirtID)
	engine.SetBlockForTest(fluidCropFeederSupport, core.WaterSourceID)
	engine.SetBlockForTest(fluidCropFeeder, core.WaterLevel1ID)
	engine.SetBlockForTest(fluidCropFloodSourceAbove(), core.WaterSourceID)

	engine.enqueueFluidUpdate(core.Overworld, fluidCropFloodSourceAbove())
	engine.enqueueFluidUpdate(core.Overworld, fluidCropFeeder)

	const settleTicks = 200
	const stabilizeTicks = 16
	saplingWrites := 0
	flooded := false
	for range settleTicks + stabilizeTicks {
		result := engine.Step()
		for _, batch := range result.Changes {
			for _, change := range batch.Changes {
				if change.Position == fluidCropCell {
					saplingWrites++
				}
			}
		}
		if !flooded && core.IsFluid(fluidBlockAt(t, engine, fluidCropCell)) {
			flooded = true
		}
	}
	if !flooded {
		t.Fatalf("收敛窗口内树苗格未被冲毁，仍是 %d", fluidBlockAt(t, engine, fluidCropCell))
	}
	if saplingWrites != 1 {
		t.Fatalf("树苗格被广播 %d 笔变更，想要恰好 1（合并先于提交）", saplingWrites)
	}
	if got := fluidBlockAt(t, engine, fluidCropCell); got != core.WaterLevel1ID {
		t.Fatalf("树苗格最终为 %d，想要最强候选 %d", got, core.WaterLevel1ID)
	}
	expectFluidCropDrops(t, engine, session, fluidCropStack{Item: core.ItemSapling, Count: 1})
}

// TestFluidSaplingCapacityFullRejectsAndRetriesUntilSlotFreed 覆盖 spec Scenario
// 「树苗冲毁容量不足时保留待更新」：
//
//  1. 槽满期间冲毁 MUST NOT 发生：树苗保持存在、格未被改写、槽位没有任何结算
//     副作用，且目标格仍被排程（队列不排空）；
//  2. 释放一个槽位后重试自然完成：树苗被冲毁、树苗掉落物落入腾出的槽位。
func TestFluidSaplingCapacityFullRejectsAndRetriesUntilSlotFreed(t *testing.T) {
	engine, session := readyFluidSaplingWorld(t)
	fillFluidCropDropSlots(t, engine)
	engine.realm.ResetFarmlandMoisture()
	full := fluidCropChunkSlots(engine)

	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())
	for range 40 {
		engine.Step()
	}
	if got := fluidBlockAt(t, engine, fluidCropCell); got != core.SaplingID {
		t.Fatalf("槽满期间树苗格被改写为 %d，树苗必须保持存在", got)
	}
	if after := fluidCropChunkSlots(engine); after != full {
		t.Fatalf("槽满期间出现结算副作用：before=%+v after=%+v", full, after)
	}
	if got := overworldFluidQueue(t, engine).Len(); got == 0 {
		t.Fatal("拒绝之后目标格没有被重新排程，释放槽位后将永远无法完成冲毁")
	}

	// 释放一个槽位：重试到期后冲毁应当完成，树苗落入腾出的槽位。
	if !engine.dimension(core.Overworld).UpdateReadyChunk(core.ChunkPos{}, func(chunk *world.Chunk) {
		chunk.ClearDrop(core.DropsPerChunk - 1)
	}) {
		t.Fatal("夹具区块未就绪")
	}
	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)

	saplingIndex, saplingIndexed := world.ChunkBlockIndex(fluidCropCell)
	fillerIndex, fillerIndexed := world.ChunkBlockIndex(core.BlockPos{X: 15, Y: 0, Z: 15})
	if !saplingIndexed || !fillerIndexed {
		t.Fatal("夹具方块没有区块索引")
	}
	saplings, fillers := 0, 0
	for _, drop := range fluidCropDropsOf(engine, session) {
		switch {
		case drop.BlockIndex == saplingIndex &&
			drop.Item == core.ItemSapling && drop.Count == 1:
			saplings++
		case drop.BlockIndex == fillerIndex &&
			drop.Item == core.ItemStone && drop.Count == 1:
			fillers++
		default:
			t.Fatalf("意外掉落物 %+v", drop)
		}
	}
	if saplings != 1 {
		t.Fatalf("树苗格上的树苗堆=%d，想要恰好 1", saplings)
	}
	if fillers != core.DropsPerChunk-1 {
		t.Fatalf("占位堆=%d，想要保留 %d", fillers, core.DropsPerChunk-1)
	}
}

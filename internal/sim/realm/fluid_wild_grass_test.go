package realm

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// fluid_wild_grass_test.go：natural-grass-seeds 在权威流体侧的短草行为——
// 流动水覆盖短草零掉落，且掉落槽已满时替换照常完成（spec「短草替换不受掉落
// 容量限制」）。与 fluid_crop_test.go 的关系是植物冲毁语义的另一半：作物走
// `fluidWorld.SetBlock` 的 `IsCrop` 特殊掉落分支并保留容量重试；短草走普通
// 写入路径，不进入掉落结算。夹具复用 fluidCropEngine（同一 z=8 列远离玩家
// 拾取半径的约定），只是把目标格换成 `ShortGrassID`。

// readyFluidWildGrassWorld 构造 z=8 列上种着短草的夹具世界。
func readyFluidWildGrassWorld(t *testing.T) (*fluidCropEngine, fluidCropSession) {
	t.Helper()
	return newFluidCropEngine(t, core.ShortGrassID), fluidCropSession{}
}

// TestFluidWildGrassVerticalFloodReplacesWithoutDrops 覆盖 spec Scenario
// 「短草格可被流动水替换且零掉落」的垂直分支：水源悬在短草正上方，垂直传播
// 把短草格写成最强流动水；世界掉落物必须保持为空——短草没有物品身份，
// 覆盖不产生小麦种子、短草物品或任何其他物品。
func TestFluidWildGrassVerticalFloodReplacesWithoutDrops(t *testing.T) {
	engine, session := readyFluidWildGrassWorld(t)
	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())

	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	expectFluidCropDrops(t, engine, session)
}

// TestFluidWildGrassHorizontalFloodReplacesWithoutDrops 覆盖同一 Scenario 的
// 水平分支：地面水源沿地水平传播进入相邻短草格（源的水平邻居得到等级 1），
// 同样零掉落。
func TestFluidWildGrassHorizontalFloodReplacesWithoutDrops(t *testing.T) {
	engine, session := readyFluidWildGrassWorld(t)
	source := core.BlockPos{
		X: fluidCropCell.X + 1, Y: fluidCropCell.Y, Z: fluidCropCell.Z,
	}
	floodFluidCropFrom(engine, source)

	stepUntilFluidCropFlooded(t, engine, fluidCropCell, core.WaterLevel1ID)
	expectFluidCropDrops(t, engine, session)
}

// TestFluidWildGrassFloodWithFullDropSlotsStillReplaces 覆盖 spec Scenario
// 「短草替换不受掉落容量限制」：全部掉落槽被占位堆填满后，短草格仍必须按
// 正常流动规则被改写，掉落槽保持占位原样——短草替换不做容量预演，也没有
// 容量重试待办（与作物冲毁的拒绝-重试语义相反）。
func TestFluidWildGrassFloodWithFullDropSlotsStillReplaces(t *testing.T) {
	engine, session := readyFluidWildGrassWorld(t)
	fillFluidCropDropSlots(t, engine)
	engine.realm.ResetFarmlandMoisture()
	full := fluidCropChunkSlots(engine)

	floodFluidCropFrom(engine, fluidCropFloodSourceAbove())
	for range 40 {
		engine.Step()
	}
	if got := fluidBlockAt(t, engine, fluidCropCell); got != core.WaterLevel1ID {
		t.Fatalf("掉落槽已满时短草格=%d，想要仍被改写为等级 1 流动水（零掉落不受容量限制）", got)
	}
	if after := fluidCropChunkSlots(engine); after != full {
		t.Fatalf("短草替换出现了掉落槽副作用：before=%+v after=%+v", full, after)
	}
	expectFluidCropDropsClassification(t, engine, session)
}

// expectFluidCropDropsClassification 断言容量用例里除占位石头外没有任何其他
// 掉落物：占位堆恰好保留 `DropsPerChunk` 个，不出现种子或任何结构件。
func expectFluidCropDropsClassification(t *testing.T, engine *fluidCropEngine, session fluidCropSession) {
	t.Helper()
	fillerIndex := fluidCropFillerBlockIndex(t)
	fillers := 0
	for _, drop := range fluidCropDropsOf(engine, session) {
		if drop.BlockIndex == fillerIndex && drop.Item == core.ItemStone && drop.Count == 1 {
			fillers++
			continue
		}
		t.Fatalf("短草覆盖产生了意外掉落物 %+v，想要零掉落", drop)
	}
	if fillers != core.DropsPerChunk {
		t.Fatalf("占位堆=%d，想要保留 %d", fillers, core.DropsPerChunk)
	}
}

// fluidCropFillerBlockIndex 返回占位堆的区块内索引（fillFluidCropDropSlots
// 的落点 (15,0,15)）。
func fluidCropFillerBlockIndex(t *testing.T) uint32 {
	t.Helper()
	index, indexed := world.ChunkBlockIndex(core.BlockPos{X: 15, Y: 0, Z: 15})
	if !indexed {
		t.Fatal("占位方块没有区块索引")
	}
	return index
}

// TestFluidWildGrassSameTickDualSourceMergesToStrongestOnceZeroDrop 覆盖 spec
// Scenario「同 tick 冲突写入取最强者」的短草子句：写往同一短草格的多个候选
// 只结算一次且保持零掉落。夹具与作物的双源用例同构——垂直候选 A（等级 1）
// 与水平候选 F（等级 2）同批争抢短草格，断言恰好一笔变更广播、最终值为最强
// 等级 1、窗口内零掉落。
func TestFluidWildGrassSameTickDualSourceMergesToStrongestOnceZeroDrop(t *testing.T) {
	engine, session := readyFluidWildGrassWorld(t)
	engine.SetBlockForTest(fluidCropDualSourceFloor, core.DirtID)
	engine.SetBlockForTest(fluidCropFeederSupport, core.WaterSourceID)
	engine.SetBlockForTest(fluidCropFeeder, core.WaterLevel1ID)
	engine.SetBlockForTest(fluidCropFloodSourceAbove(), core.WaterSourceID)

	engine.enqueueFluidUpdate(core.Overworld, fluidCropFloodSourceAbove())
	engine.enqueueFluidUpdate(core.Overworld, fluidCropFeeder)

	const settleTicks = 200
	const stabilizeTicks = 16
	grassWrites := 0
	flooded := false
	for range settleTicks + stabilizeTicks {
		result := engine.Step()
		for _, batch := range result.Changes {
			for _, change := range batch.Changes {
				if change.Position == fluidCropCell {
					grassWrites++
				}
			}
		}
		if !flooded && core.IsFluid(fluidBlockAt(t, engine, fluidCropCell)) {
			flooded = true
		}
	}
	if !flooded {
		t.Fatalf("收敛窗口内短草格未被覆盖，仍是 %d", fluidBlockAt(t, engine, fluidCropCell))
	}
	if grassWrites != 1 {
		t.Fatalf("短草格被广播 %d 笔变更，想要恰好 1（合并先于提交）", grassWrites)
	}
	if got := fluidBlockAt(t, engine, fluidCropCell); got != core.WaterLevel1ID {
		t.Fatalf("短草格最终为 %d，想要最强候选 %d", got, core.WaterLevel1ID)
	}
	expectFluidCropDrops(t, engine, session)
}

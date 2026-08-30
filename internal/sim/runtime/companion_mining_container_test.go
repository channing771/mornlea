package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件锁定伙伴采掘容器（箱子/熔炉）的批量全或无结算（change
// companion-mine-containers 方案 A）：产物是「容器本体 + 全部内容物堆」，
// 结算前在伙伴背包副本上按固定序逐堆预演，任一堆放不下即该 tick 整体不结算
// ——方块、容器内容物、工具耐久、背包与采掘进度满格状态全部保持。

// companionContainerTicks 是铁镐对箱子/熔炉的权威采掘计时（miningRule），
// 完成分叉恰好发生在第 companionContainerTicks 次 advanceMiningOnce。
const companionContainerTicks = 8

// companionContainerBareHandTicks 是空手（错误工具档）对箱子/熔炉的权威采掘
// 计时（miningRule 的 default 分支），harvestable 为假——错误工具用例的完成
// 分叉恰好发生在第 companionContainerBareHandTicks 次 advanceMiningOnce。
const companionContainerBareHandTicks = 30

// readyCompanionChestMining 在公共采掘场景（readyCompanionMining）的目标格上
// 激活一个箱子槽并装入指定内容物，返回场景与写入的完整箱子槽（供前后比对）。
func readyCompanionChestMining(
	t *testing.T,
	tool core.ItemID,
	items [core.ChestSlots]core.ItemStack,
) (companionMiningFixture, world.ChestSlot) {
	t.Helper()
	fixture := readyCompanionMining(t, core.ChestID, tool)
	index, ok := world.ChunkBlockIndex(fixture.target)
	if !ok {
		t.Fatalf("箱子目标 %+v 没有区块索引", fixture.target)
	}
	slot := world.ChestSlot{
		Generation: 3, Active: true, BlockIndex: index, Items: items,
	}
	fixture.engine.SetChunkChestForTest(
		core.ChunkKey{Dimension: core.Overworld, Pos: fixture.target.Chunk()}, 0, slot,
	)
	return fixture, slot
}

// readyCompanionFurnaceMining 在公共采掘场景的目标格上激活一个熔炉槽并装入
// 输入/燃料/输出三格内容物，返回场景与写入的完整熔炉槽（供前后比对）。
func readyCompanionFurnaceMining(
	t *testing.T,
	tool core.ItemID,
	input, fuel, output core.ItemStack,
) (companionMiningFixture, world.FurnaceSlot) {
	t.Helper()
	fixture := readyCompanionMining(t, core.FurnaceID, tool)
	index, ok := world.ChunkBlockIndex(fixture.target)
	if !ok {
		t.Fatalf("熔炉目标 %+v 没有区块索引", fixture.target)
	}
	slot := world.FurnaceSlot{
		Generation: 2, Active: true, BlockIndex: index,
		Input: input, Fuel: fuel, Output: output,
	}
	fixture.engine.SetChunkFurnaceForTest(
		core.ChunkKey{Dimension: core.Overworld, Pos: fixture.target.Chunk()}, 0, slot,
	)
	return fixture, slot
}

// companionChestAt 读取目标格所在区块的箱子槽 0，区块未就绪时直接失败。
func companionChestAt(t *testing.T, fixture companionMiningFixture) world.ChestSlot {
	t.Helper()
	return miningTargetRecord(t, fixture.engine, fixture.target).Chunk.Chest(0)
}

// companionFurnaceAt 读取目标格所在区块的熔炉槽 0，区块未就绪时直接失败。
func companionFurnaceAt(t *testing.T, fixture companionMiningFixture) world.FurnaceSlot {
	t.Helper()
	return miningTargetRecord(t, fixture.engine, fixture.target).Chunk.Furnace(0)
}

// TestCompanionMineableBlockContainerTargets 锁定防御清单的容器放开：箱子与
// 熔炉是合法的伙伴采掘目标（其批量结算的容量安全由完成分叉的全或无预演保证，
// 不再由目标清单承担）；农业十编号的显式拒绝保持不变（农业语义尚未裁决）。
func TestCompanionMineableBlockContainerTargets(t *testing.T) {
	if !companionMineableBlock(core.ChestID) {
		t.Fatal("companionMineableBlock(ChestID) = false，箱子必须是合法的伙伴采掘目标")
	}
	if !companionMineableBlock(core.FurnaceID) {
		t.Fatal("companionMineableBlock(FurnaceID) = false，熔炉必须是合法的伙伴采掘目标")
	}
	for _, block := range []core.BlockID{
		core.WheatStage0ID, core.WheatStage1ID, core.WheatStage2ID, core.WheatStage3ID,
		core.WheatStage4ID, core.WheatStage5ID, core.WheatStage6ID, core.WheatStage7ID,
		core.FarmlandDryID, core.FarmlandWetID,
	} {
		if companionMineableBlock(block) {
			t.Fatalf("companionMineableBlock(%d) = true，农业方块必须被显式拒绝", block)
		}
	}
}

// TestCompanionMiningChestBatchIsAtomic 锁定箱子批量结算的原子性与固定序：
// 完成前每 tick 四方（方块、容器槽、耐久、背包）不变；完成 tick 内方块变空气、
// 容器槽停用、本体与内容物直入背包、耐久扣减同时成立；内容物按「本体在前、
// 槽位序」入包；背包余量不足时整体不结算且进度保持满格（稳定饱和）。
func TestCompanionMiningChestBatchIsAtomic(t *testing.T) {
	t.Run("空箱子同tick原子回收", func(t *testing.T) {
		fixture, chest := readyCompanionChestMining(t, core.ItemIronPickaxe, [core.ChestSlots]core.ItemStack{})
		entry := fixture.entry
		full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)

		for tick := 1; tick < companionContainerTicks; tick++ {
			advanceMiningOnce(fixture.engine)
			if got := companionMiningBlockAt(t, fixture); got != core.ChestID {
				t.Fatalf("tick %d 箱子提前破坏=%d", tick, got)
			}
			if got := companionChestAt(t, fixture); got != chest || !got.Active {
				t.Fatalf("tick %d 完成前箱子槽被动过: %+v", tick, got)
			}
			if got := entry.inventory.Hotbar.Slots[0].Durability; got != full {
				t.Fatalf("tick %d 耐久提前扣减=%d", tick, got)
			}
			if got := companionItemCount(entry, core.ItemChest); got != 0 {
				t.Fatalf("tick %d 本体提前入包=%d", tick, got)
			}
		}

		result := advanceMiningOnce(fixture.engine)
		if got := companionMiningBlockAt(t, fixture); got != core.AirID {
			t.Fatalf("完成 tick 方块=%d，想要空气", got)
		}
		if got := companionChestAt(t, fixture); got != (world.ChestSlot{Generation: 3}) {
			t.Fatalf("完成 tick 箱子槽未停用且保留 generation: %+v", got)
		}
		if got := companionItemCount(entry, core.ItemChest); got != 1 {
			t.Fatalf("完成 tick 本体未入包: chest=%d", got)
		}
		if got := entry.inventory.Hotbar.Slots[0].Durability; got != full-1 {
			t.Fatalf("完成 tick 耐久=%d，想要 %d", got, full-1)
		}
		if len(result.Changes) != 1 || len(result.Changes[0].Changes) != 1 ||
			result.Changes[0].Changes[0].Position != fixture.target ||
			result.Changes[0].Changes[0].Block != core.AirID {
			t.Fatalf("完成 tick 区块变更=%+v，想要单一空气变更", result.Changes)
		}
		if !entry.inventoryDirty {
			t.Fatal("完成 tick 没有标记 inventoryDirty")
		}
		if entry.mining != (miningState{}) {
			t.Fatalf("完成后进度未清零: %+v", entry.mining)
		}
	})

	t.Run("内容物按本体在前的槽位序入包", func(t *testing.T) {
		var items [core.ChestSlots]core.ItemStack
		// 槽位刻意稀疏（0/2/5）：证明空槽被跳过、产物严格按容器槽位序展开。
		items[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
		items[2] = core.ItemStack{Item: core.ItemGlass, Count: 2}
		items[5] = core.ItemStack{Item: core.ItemOakPlanks, Count: 7}
		fixture, _ := readyCompanionChestMining(t, core.ItemIronPickaxe, items)
		entry := fixture.entry

		for range companionContainerTicks {
			advanceMiningOnce(fixture.engine)
		}

		if got := companionMiningBlockAt(t, fixture); got != core.AirID {
			t.Fatalf("完成 tick 方块=%d，想要空气", got)
		}
		if got := companionChestAt(t, fixture); got != (world.ChestSlot{Generation: 3}) {
			t.Fatalf("完成 tick 箱子槽未停用且保留 generation: %+v", got)
		}
		// 产物全部互异且初始背包除工具外全空：AddStack 的空格阶段按提交序占位，
		// 落位顺序即产物固定序——本体最前，内容物按槽位序紧随其后。
		if got := entry.inventory.Hotbar.Slots[1]; got != (core.ItemStack{Item: core.ItemChest, Count: 1}) {
			t.Fatalf("槽 1=%+v，想要箱子本体（本体在前）", got)
		}
		if got := entry.inventory.Hotbar.Slots[2]; got != items[0] {
			t.Fatalf("槽 2=%+v，想要槽位序首堆 %+v", got, items[0])
		}
		if got := entry.inventory.Hotbar.Slots[3]; got != items[2] {
			t.Fatalf("槽 3=%+v，想要槽位序次堆 %+v", got, items[2])
		}
		if got := entry.inventory.Hotbar.Slots[4]; got != items[5] {
			t.Fatalf("槽 4=%+v，想要槽位序末堆 %+v", got, items[5])
		}
		if got := companionItemCount(entry, core.ItemStone); got != 3 {
			t.Fatalf("stone=%d，想要 3", got)
		}
		if got := companionItemCount(entry, core.ItemGlass); got != 2 {
			t.Fatalf("glass=%d，想要 2", got)
		}
		if got := companionItemCount(entry, core.ItemOakPlanks); got != 7 {
			t.Fatalf("planks=%d，想要 7", got)
		}
	})

	t.Run("内容物超出背包余量时全或无", func(t *testing.T) {
		var items [core.ChestSlots]core.ItemStack
		items[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
		items[1] = core.ItemStack{Item: core.ItemGlass, Count: 2}
		items[3] = core.ItemStack{Item: core.ItemSand, Count: 4}
		fixture, chest := readyCompanionChestMining(t, core.ItemIronPickaxe, items)
		entry := fixture.entry
		full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
		// 背包只留 hotbar 1、2 两个空格：预演中本体与首堆内容物可以入位、第二堆
		// 失败——证明预演部分成功的堆绝不上账，结算必须整体放弃。
		fillCompanionInventory(entry, core.ItemDirt)
		entry.inventory.Hotbar.Slots[1] = core.ItemStack{}
		entry.inventory.Hotbar.Slots[2] = core.ItemStack{}
		before := entry.inventory

		for tick := 0; tick < 3*companionContainerTicks; tick++ {
			advanceMiningOnce(fixture.engine)
		}

		if got := companionMiningBlockAt(t, fixture); got != core.ChestID {
			t.Fatalf("无容量却破坏了箱子=%d", got)
		}
		if got := companionChestAt(t, fixture); got != chest || !got.Active {
			t.Fatalf("无容量期间容器内容物被动过: %+v", got)
		}
		if entry.inventory != before {
			t.Fatal("无容量期间背包被修改")
		}
		if got := entry.inventory.Hotbar.Slots[0].Durability; got != full {
			t.Fatalf("无容量期间耐久被扣减=%d", got)
		}
		if entry.inventoryDirty {
			t.Fatal("无容量期间标记了 inventoryDirty")
		}
		if entry.mining.requiredTicks != companionContainerTicks ||
			entry.mining.progressTicks != entry.mining.requiredTicks {
			t.Fatalf("无容量时进度没有保持满格: %+v", entry.mining)
		}
	})
}

// TestCompanionMiningFurnaceBatchIsAtomic 锁定熔炉批量结算：本体与输入/燃料/
// 输出三格内容物一并入包；不可容纳时全或无（四方不变、进度满格）。
func TestCompanionMiningFurnaceBatchIsAtomic(t *testing.T) {
	input := core.ItemStack{Item: core.ItemRawIron, Count: 2}
	fuel := core.ItemStack{Item: core.ItemCoal, Count: 3}
	output := core.ItemStack{Item: core.ItemIronIngot, Count: 1}

	t.Run("本体与三格内容物一并入包", func(t *testing.T) {
		fixture, furnace := readyCompanionFurnaceMining(t, core.ItemIronPickaxe, input, fuel, output)
		entry := fixture.entry
		full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)

		for tick := 1; tick < companionContainerTicks; tick++ {
			advanceMiningOnce(fixture.engine)
			if got := companionMiningBlockAt(t, fixture); got != core.FurnaceID {
				t.Fatalf("tick %d 熔炉提前破坏=%d", tick, got)
			}
			if got := companionFurnaceAt(t, fixture); got != furnace || !got.Active {
				t.Fatalf("tick %d 完成前熔炉槽被动过: %+v", tick, got)
			}
		}

		advanceMiningOnce(fixture.engine)
		if got := companionMiningBlockAt(t, fixture); got != core.AirID {
			t.Fatalf("完成 tick 方块=%d，想要空气", got)
		}
		if got := companionFurnaceAt(t, fixture); got != (world.FurnaceSlot{Generation: 2}) {
			t.Fatalf("完成 tick 熔炉槽未停用且保留 generation: %+v", got)
		}
		// 产物固定序：本体 → 输入 → 燃料 → 输出（互异物品 + 空背包下落位即顺序）。
		if got := entry.inventory.Hotbar.Slots[1]; got != (core.ItemStack{Item: core.ItemFurnace, Count: 1}) {
			t.Fatalf("槽 1=%+v，想要熔炉本体", got)
		}
		if got := entry.inventory.Hotbar.Slots[2]; got != input {
			t.Fatalf("槽 2=%+v，想要输入格 %+v", got, input)
		}
		if got := entry.inventory.Hotbar.Slots[3]; got != fuel {
			t.Fatalf("槽 3=%+v，想要燃料格 %+v", got, fuel)
		}
		if got := entry.inventory.Hotbar.Slots[4]; got != output {
			t.Fatalf("槽 4=%+v，想要输出格 %+v", got, output)
		}
		if got := entry.inventory.Hotbar.Slots[0].Durability; got != full-1 {
			t.Fatalf("完成 tick 耐久=%d，想要 %d", got, full-1)
		}
		if entry.mining != (miningState{}) {
			t.Fatalf("完成后进度未清零: %+v", entry.mining)
		}
	})

	t.Run("不可容纳时全或无", func(t *testing.T) {
		fixture, furnace := readyCompanionFurnaceMining(t, core.ItemIronPickaxe, input, fuel, output)
		entry := fixture.entry
		full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
		// 只留一个空格：预演中本体入位、输入格失败——部分预演绝不上账。
		fillCompanionInventory(entry, core.ItemDirt)
		entry.inventory.Hotbar.Slots[1] = core.ItemStack{}
		before := entry.inventory

		for tick := 0; tick < 3*companionContainerTicks; tick++ {
			advanceMiningOnce(fixture.engine)
		}

		if got := companionMiningBlockAt(t, fixture); got != core.FurnaceID {
			t.Fatalf("无容量却破坏了熔炉=%d", got)
		}
		if got := companionFurnaceAt(t, fixture); got != furnace || !got.Active {
			t.Fatalf("无容量期间容器内容物被动过: %+v", got)
		}
		if entry.inventory != before {
			t.Fatal("无容量期间背包被修改")
		}
		if got := entry.inventory.Hotbar.Slots[0].Durability; got != full {
			t.Fatalf("无容量期间耐久被扣减=%d", got)
		}
		if entry.inventoryDirty {
			t.Fatal("无容量期间标记了 inventoryDirty")
		}
		if entry.mining.requiredTicks != companionContainerTicks ||
			entry.mining.progressTicks != entry.mining.requiredTicks {
			t.Fatalf("无容量时进度没有保持满格: %+v", entry.mining)
		}
	})
}

// TestCompanionMiningContainerWrongToolSkipsBody 锁定错误工具（harvestable 为
// 假）的容器完成语义，对齐玩家路径 `completeMining` 的可收获判定：空手挖箱子
// 30 tick 且不可收获，完成 tick 方块仍被破坏、容器槽停用、全部内容物入包，
// 但容器本体不入包——`CompanionMineContainerStaging` 的产物集合在 harvestable
// 为假时不计本体（错误工具挖掉的是「箱子方块加内容物」，不是「一件箱子物品
// 加内容物」）。
func TestCompanionMiningContainerWrongToolSkipsBody(t *testing.T) {
	var items [core.ChestSlots]core.ItemStack
	items[0] = core.ItemStack{Item: core.ItemGlass, Count: 2}
	items[4] = core.ItemStack{Item: core.ItemOakPlanks, Count: 3}
	fixture, _ := readyCompanionChestMining(t, core.ItemNone, items)
	entry := fixture.entry

	for tick := 1; tick < companionContainerBareHandTicks; tick++ {
		advanceMiningOnce(fixture.engine)
		if got := companionMiningBlockAt(t, fixture); got != core.ChestID {
			t.Fatalf("tick %d 箱子提前破坏=%d", tick, got)
		}
	}
	advanceMiningOnce(fixture.engine)

	if got := companionMiningBlockAt(t, fixture); got != core.AirID {
		t.Fatalf("完成 tick 方块=%d，想要空气（错误工具仍破坏方块）", got)
	}
	if got := companionChestAt(t, fixture); got != (world.ChestSlot{Generation: 3}) {
		t.Fatalf("完成 tick 箱子槽未停用且保留 generation: %+v", got)
	}
	if got := companionItemCount(entry, core.ItemChest); got != 0 {
		t.Fatalf("错误工具完成时本体入包=%d，想要 0（harvestable 为假不计本体）", got)
	}
	if got := companionItemCount(entry, core.ItemGlass); got != 2 {
		t.Fatalf("内容物玻璃=%d，想要 2", got)
	}
	if got := companionItemCount(entry, core.ItemOakPlanks); got != 3 {
		t.Fatalf("内容物木板=%d，想要 3", got)
	}
	if entry.mining != (miningState{}) {
		t.Fatalf("完成后进度未清零: %+v", entry.mining)
	}
}

// TestCompanionMiningContainerEdgeClearsProgressWithoutSettlement 锁定完成 tick 的
// 容器边缘：目标格的容器槽缺失（方块编号是箱子却没有活动箱子槽的不一致世界）
// 或完成前方块已被移除时，对齐玩家路径的 RejectNoTarget/无效目标语义——清零
// 进度、不结算、方块与既有容器槽都不被触碰（无容器槽泄漏）。
func TestCompanionMiningContainerEdgeClearsProgressWithoutSettlement(t *testing.T) {
	t.Run("完成tick容器槽缺失清零进度不结算", func(t *testing.T) {
		// 箱子方块存在但目标格没有活动箱子槽；另在别的方块索引上放一个带内容物
		// 的活动槽作哨兵——失败路径绝不能停用或改写它。
		fixture := readyCompanionMining(t, core.ChestID, core.ItemIronPickaxe)
		entry := fixture.entry
		decoyIndex, ok := world.ChunkBlockIndex(core.BlockPos{
			X: fixture.target.X + 2, Y: fixture.target.Y, Z: fixture.target.Z,
		})
		if !ok {
			t.Fatalf("哨兵位置 %+v 没有区块索引", fixture.target)
		}
		var decoyItems [core.ChestSlots]core.ItemStack
		decoyItems[0] = core.ItemStack{Item: core.ItemGlass, Count: 2}
		decoy := world.ChestSlot{
			Generation: 4, Active: true, BlockIndex: decoyIndex, Items: decoyItems,
		}
		fixture.engine.SetChunkChestForTest(
			core.ChunkKey{Dimension: core.Overworld, Pos: fixture.target.Chunk()}, 0, decoy)

		for tick := 1; tick <= companionContainerTicks; tick++ {
			advanceMiningOnce(fixture.engine)
		}

		if got := companionMiningBlockAt(t, fixture); got != core.ChestID {
			t.Fatalf("容器槽缺失却破坏了箱子=%d", got)
		}
		if entry.mining != (miningState{}) {
			t.Fatalf("完成 tick 进度未清零: %+v", entry.mining)
		}
		if got := companionChestAt(t, fixture); got != decoy {
			t.Fatalf("哨兵容器槽被改动: %+v，想要原样保留 %+v", got, decoy)
		}
		if got := companionItemCount(entry, core.ItemChest); got != 0 {
			t.Fatalf("容器槽缺失却把本体入包=%d", got)
		}
	})

	t.Run("完成前方块被移除清零进度不结算", func(t *testing.T) {
		var items [core.ChestSlots]core.ItemStack
		items[0] = core.ItemStack{Item: core.ItemGlass, Count: 2}
		fixture, chest := readyCompanionChestMining(t, core.ItemIronPickaxe, items)
		entry := fixture.entry
		for tick := 1; tick < companionContainerTicks; tick++ {
			advanceMiningOnce(fixture.engine)
		}
		if got := entry.mining.progressTicks; got != companionContainerTicks-1 {
			t.Fatalf("前置进度=%d，想要 %d", got, companionContainerTicks-1)
		}

		fixture.engine.SetBlockForTest(fixture.target, core.AirID)
		advanceMiningOnce(fixture.engine)

		if entry.mining != (miningState{}) {
			t.Fatalf("方块移除后进度未清零: %+v", entry.mining)
		}
		if got := companionMiningBlockAt(t, fixture); got != core.AirID {
			t.Fatalf("移除后方块=%d", got)
		}
		if got := companionChestAt(t, fixture); got != chest || !got.Active {
			t.Fatalf("方块移除后容器槽被改动: %+v", got)
		}
		if got := companionItemCount(entry, core.ItemGlass); got != 0 {
			t.Fatalf("内容物提前入包=%d", got)
		}
	})
}

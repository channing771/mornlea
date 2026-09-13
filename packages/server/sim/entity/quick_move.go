package entity

import (
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 本文件实现快捷搬运（`CommandQuickMoveStack`）的固定目标序结算：把来源格
// 整堆移动到对侧区域的首个可容纳位置。目标序是确定性契约——容器区来源按
// 拾取四相位序（`Inventory.AddStack`）、箱子/网格按统一索引升序、熔炉按
// 「熔炼输入优先于燃料」、纯背包面板按区域受限两相位序对侧互移；全部扫描
// 都是固定顺序，无 map 迭代。数量为整堆：部分可容纳时按既有合并语义执行
//（余量留源，非全有或全无），对侧零吸收整单拒绝。

// quickMoveInsertRegion 按区域受限的两相位序把 source 并入 inventory 的
// first..last 区段（含端点）：先按统一索引升序并入同类未满格，再升序占用
// 空格——与 `core.Inventory.AddStack` 的四相位纪律同构，只是把扫描域收敛
// 到单一区域（快捷栏 ↔ 背包互移绝不跨区）。空格承接继承来源的耐久字段，
// 余量原样返回；source 非法或区段内零吸收时返回原状态与原栈。
func quickMoveInsertRegion(
	inventory core.Inventory,
	source core.ItemStack,
	first, last uint8,
) (core.Inventory, core.ItemStack) {
	original := inventory
	if source.Item == core.ItemNone || !source.Valid() {
		return original, source
	}
	limit, _ := core.ItemStackLimit(source.Item)
	for _, merge := range [2]bool{true, false} {
		for slot := first; slot <= last; slot++ {
			if source.Count == 0 {
				return inventory, core.ItemStack{}
			}
			current, _ := inventory.Slot(slot)
			if merge {
				if current.Item != source.Item || current.Count >= limit {
					continue
				}
			} else if current.Item != core.ItemNone {
				continue
			}
			if !merge {
				current = source
				current.Count = 0
			}
			moved := min(limit-current.Count, source.Count)
			current.Count += moved
			source.Count -= moved
			next, ok := inventory.SetSlot(slot, current)
			if !ok {
				// 不可达防御：读出的合法栏位必然可写回；保持失败零改动。
				return original, source
			}
			inventory = next
		}
	}
	return inventory, source
}

// quickMoveMergeInto 报告 target 能否容纳 source（快捷搬运的可容纳格 = 空
// 或同类未满）：可容纳时返回目标新值与来源余量——空格整堆迁移（继承耐久
// 字段）、同类按目标剩余容量合并，余量留源；异类非空与同类满格不可容纳。
func quickMoveMergeInto(
	source, target core.ItemStack,
) (nextTarget, leftover core.ItemStack, fits bool) {
	limit, hasLimit := core.ItemStackLimit(source.Item)
	if !hasLimit {
		return core.ItemStack{}, core.ItemStack{}, false
	}
	if target.Item != core.ItemNone &&
		(target.Item != source.Item || target.Count >= limit) {
		return core.ItemStack{}, core.ItemStack{}, false
	}
	nextTarget = target
	if target.Item == core.ItemNone {
		nextTarget = source
		nextTarget.Count = 0
	}
	moved := min(source.Count, limit-nextTarget.Count)
	nextTarget.Count += moved
	if source.Count > moved {
		leftover = core.ItemStack{Item: source.Item, Count: source.Count - moved}
	}
	return nextTarget, leftover, true
}

// applyQuickMoveInventory 在纯背包面板（统一索引 0..35）内执行一次快捷
// 搬运：快捷栏（0..8）与背包（9..35）互为对侧区域，来源整堆按区域受限
// 两相位序并入对侧，余量留源；对侧零吸收（无空位且无同类余量）或源空
// 返回 false。区域内部是纯迁移，不改变背包物品总量，回收不变量沿既有
// 背包内部移动的同一条论证保持，不另做预演。
func (player *playerState) applyQuickMoveInventory(from uint8) bool {
	inventory := player.inventory
	source, ok := inventory.Slot(from)
	if !ok || source.Item == core.ItemNone {
		return false
	}
	var first, last uint8
	if from < core.HotbarSlots {
		first, last = core.HotbarSlots, core.InventorySlots-1
	} else {
		first, last = 0, core.HotbarSlots-1
	}
	next, leftover := quickMoveInsertRegion(inventory, source, first, last)
	if leftover.Count == source.Count {
		return false
	}
	// 来源格在对侧扫描区段之外，余量写回不影响已并入的量。
	next, ok = next.SetSlot(from, leftover)
	if !ok {
		return false
	}
	player.inventory = next
	player.inventoryDirty = true
	return true
}

// applyQuickMoveCrafting 在合成统一视图（网格 0..8、背包 9..44）内执行一次
// 快捷搬运：网格来源整堆按拾取四相位序并入背包（余量留源）；背包来源在
// 有效网格区（0..size²-1）内升序进首个「空或同类未满」格（余量留源）。
// 两个方向都在局部副本上预演回收不变量（`canRepackCrafting`）——玩家主动
// 移动不得制造「网格无法完整装回背包」的状态；破坏不变量、对侧零吸收或
// 源空返回 false 且逐格不变。
func (player *playerState) applyQuickMoveCrafting(from uint8) bool {
	inventory, grid := player.inventory, player.crafting
	source := craftingViewSlot(inventory, grid, from)
	if source.Item == core.ItemNone {
		return false
	}
	if from < core.CraftingGridSlots {
		// 网格 → 背包：拾取四相位序整堆并入，余量留在网格来源格。
		next, leftover := inventory.AddStack(source)
		if leftover.Count == source.Count {
			return false
		}
		next, nextGrid, ok := setCraftingViewSlot(next, grid, from, leftover)
		if !ok || !next.Valid() || !canRepackCrafting(next, nextGrid) {
			return false
		}
		player.inventory, player.crafting = next, nextGrid
		player.inventoryDirty = true
		player.craftingDirty = true
		return true
	}
	// 背包 → 网格：有效网格区内升序找首个可容纳格，只落位一格、余量留源。
	extent := grid.Size * grid.Size
	for slot := uint8(0); slot < extent; slot++ {
		nextTarget, leftover, fits := quickMoveMergeInto(source, grid.Slots[slot])
		if !fits {
			continue
		}
		nextInventory, nextGrid, ok := setCraftingViewSlot(inventory, grid, from, leftover)
		if !ok {
			return false
		}
		if nextInventory, nextGrid, ok = setCraftingViewSlot(
			nextInventory, nextGrid, slot, nextTarget,
		); !ok {
			return false
		}
		if !nextInventory.Valid() || !canRepackCrafting(nextInventory, nextGrid) {
			return false
		}
		player.inventory, player.crafting = nextInventory, nextGrid
		player.inventoryDirty = true
		player.craftingDirty = true
		return true
	}
	return false
}

// quickMoveChestStack 在玩家物品与箱子的值副本上计算一次快捷搬运：来源在
// 箱子区（36..62）→ 整堆按拾取四相位序并入玩家背包（余量留源）；来源在
// 背包区（0..35）→ 统一索引 36..62 升序进首个「空或同类未满」格（余量
// 留源）。值域非法、源空或对侧零吸收都返回原值和 false；回收不变量预演
// 由调用方（`applyContainerMove`）统一执行。
func quickMoveChestStack(
	inventory core.Inventory,
	chest world.ChestSlot,
	from uint8,
) (core.Inventory, world.ChestSlot, bool) {
	if from >= core.ChestViewSlots || !chest.Active || !chest.Valid() || !inventory.Valid() {
		return inventory, chest, false
	}
	if from >= core.ChestFirstSlot {
		source, ok := chestViewSlot(inventory, chest, from)
		if !ok || source.Item == core.ItemNone {
			return inventory, chest, false
		}
		next, leftover := inventory.AddStack(source)
		if leftover.Count == source.Count {
			return inventory, chest, false
		}
		nextChest := chest
		if next, nextChest, ok = setChestViewSlot(next, nextChest, from, leftover); !ok {
			return inventory, chest, false
		}
		if !nextChest.Valid() || !next.Valid() {
			return inventory, chest, false
		}
		return next, nextChest, true
	}
	source, ok := inventory.Slot(from)
	if !ok || source.Item == core.ItemNone {
		return inventory, chest, false
	}
	for slot := uint8(core.ChestFirstSlot); slot < core.ChestViewSlots; slot++ {
		target, _ := chestViewSlot(inventory, chest, slot)
		nextTarget, leftover, fits := quickMoveMergeInto(source, target)
		if !fits {
			continue
		}
		nextInventory, nextChest := inventory, chest
		if nextInventory, nextChest, ok = setChestViewSlot(
			nextInventory, nextChest, from, leftover,
		); !ok {
			return inventory, chest, false
		}
		if nextInventory, nextChest, ok = setChestViewSlot(
			nextInventory, nextChest, slot, nextTarget,
		); !ok {
			return inventory, chest, false
		}
		if !nextChest.Valid() || !nextInventory.Valid() {
			return inventory, chest, false
		}
		return nextInventory, nextChest, true
	}
	return inventory, chest, false
}

// quickMoveFurnaceStack 在玩家物品与熔炉的值副本上计算一次快捷搬运：来源
// 在容器区（输入 36/燃料 37/输出 38，输出只可为来源）→ 整堆按拾取四相位
// 序并入玩家背包（余量留源）；来源在背包区（0..35）→ 按「熔炼输入优先于
// 燃料」的槽位优先级落位——物品是熔炼表内物品时先试输入槽、是煤时再试
// 燃料槽，两类皆可时输入槽先行，前序候选无可容纳容量时顺延，均无可容纳
// 整单拒绝。全部熔炉格写入（目标与来源余量）都经 `setFurnaceViewSlot`
// 逐字复用既有槽位约束（输入仅熔炼表内物品且换物品重置进度、燃料仅煤、
// 输出白名单）：余量是来源栈的子堆，同类余量不重置进度、清空即换物品
// 重置，与部分移动的写回路径同形。值域非法、源空或对侧零吸收都返回原值
// 和 false；回收不变量预演由调用方统一执行。
func quickMoveFurnaceStack(
	inventory core.Inventory,
	furnace world.FurnaceSlot,
	from uint8,
) (core.Inventory, world.FurnaceSlot, bool) {
	if from >= core.FurnaceViewSlots || !furnace.Active || !furnace.Valid() || !inventory.Valid() {
		return inventory, furnace, false
	}
	if from >= core.InventorySlots {
		source, ok := furnaceViewSlot(inventory, furnace, from)
		if !ok || source.Item == core.ItemNone {
			return inventory, furnace, false
		}
		next, leftover := inventory.AddStack(source)
		if leftover.Count == source.Count {
			return inventory, furnace, false
		}
		nextFurnace := furnace
		if next, nextFurnace, ok = setFurnaceViewSlot(next, nextFurnace, from, leftover); !ok {
			return inventory, furnace, false
		}
		if !nextFurnace.Valid() || !next.Valid() {
			return inventory, furnace, false
		}
		return next, nextFurnace, true
	}
	source, ok := inventory.Slot(from)
	if !ok || source.Item == core.ItemNone {
		return inventory, furnace, false
	}
	_, smeltable := core.SmeltingOutput(source.Item)
	fuel := source.Item == core.ItemCoal
	if !smeltable && !fuel {
		return inventory, furnace, false
	}
	for step := 0; step < 2; step++ {
		slot := uint8(core.FurnaceFuelSlot)
		if step == 0 {
			if !smeltable {
				continue
			}
			slot = core.FurnaceInputSlot
		} else if !fuel {
			break
		}
		target, _ := furnaceViewSlot(inventory, furnace, slot)
		nextTarget, leftover, fits := quickMoveMergeInto(source, target)
		if !fits {
			continue
		}
		nextInventory, nextFurnace := inventory, furnace
		if nextInventory, nextFurnace, ok = setFurnaceViewSlot(
			nextInventory, nextFurnace, from, leftover,
		); !ok {
			return inventory, furnace, false
		}
		if nextInventory, nextFurnace, ok = setFurnaceViewSlot(
			nextInventory, nextFurnace, slot, nextTarget,
		); !ok {
			return inventory, furnace, false
		}
		if !nextFurnace.Valid() || !nextInventory.Valid() {
			return inventory, furnace, false
		}
		return nextInventory, nextFurnace, true
	}
	return inventory, furnace, false
}

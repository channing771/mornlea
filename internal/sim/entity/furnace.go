package entity

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

// advanceFurnaces 在单写者 tick 中推进活动范围内的熔炉。
func (engine *Engine) advanceFurnaces(mutation *realm.Mutation) {
	keys := engine.activeInterestKeys()
	burnTicks := engine.tunables.FurnaceBurnTicks
	smeltTicks := engine.tunables.FurnaceSmeltTicks
	for _, key := range keys {
		dimension := engine.dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		chunk, ok := dimension.ReadyChunk(key.Pos)
		if !ok {
			continue
		}
		if advanceChunkFurnaces(chunk, burnTicks, smeltTicks) {
			engine.touchChunk(key, mutation)
		}
	}
}

func advanceChunkFurnaces(chunk *world.Chunk, burnTicks uint16, smeltTicks uint8) bool {
	changed := false
	for slot := range core.FurnacesPerChunk {
		furnace := chunk.Furnace(slot)
		if !furnace.Active {
			continue
		}
		next, updated := advanceFurnace(furnace, burnTicks, smeltTicks)
		if !updated {
			continue
		}
		chunk.SetFurnace(slot, next)
		changed = true
	}
	return changed
}

func advanceFurnace(furnace world.FurnaceSlot, burnTicks uint16, smeltTicks uint8) (world.FurnaceSlot, bool) {
	output, ok := canSmelt(furnace)
	if !ok {
		return furnace, false
	}
	if furnace.BurnTicks == 0 {
		if furnace.Fuel.Item != core.ItemCoal || furnace.Fuel.Count == 0 {
			return furnace, false
		}
		furnace.Fuel.Count--
		if furnace.Fuel.Count == 0 {
			furnace.Fuel = core.ItemStack{}
		}
		furnace.BurnTicks = burnTicks
	}
	furnace.BurnTicks--
	furnace.ProgressTicks++
	if furnace.ProgressTicks < smeltTicks {
		return furnace, true
	}
	furnace.ProgressTicks = 0
	furnace.Input.Count--
	if furnace.Input.Count == 0 {
		furnace.Input = core.ItemStack{}
	}
	if furnace.Output.Item == core.ItemNone {
		furnace.Output = core.ItemStack{Item: output, Count: 1}
	} else {
		furnace.Output.Count++
	}
	return furnace, true
}

func canSmelt(furnace world.FurnaceSlot) (core.ItemID, bool) {
	output, ok := core.SmeltingOutput(furnace.Input.Item)
	if !ok || furnace.Input.Count == 0 {
		return core.ItemNone, false
	}
	return output, furnace.Output.Item == core.ItemNone ||
		(furnace.Output.Item == output && furnace.Output.Count < core.MaxStackCount)
}

// SetChunkFurnaceForTest 直接写入一个已 Ready 区块的熔炉槽，仅供测试构造固定场景。
func (engine *Engine) SetChunkFurnaceForTest(
	key core.ChunkKey,
	slot int,
	value world.FurnaceSlot,
) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return
	}
	dimension.UpdateReadyChunk(key.Pos, func(chunk *world.Chunk) { chunk.SetFurnace(slot, value) })
}

// AdvanceFurnacesForBenchmark 只在活动区块上推进熔炉本身，
func (engine *Engine) AdvanceFurnacesForBenchmark() {
	burnTicks := engine.tunables.FurnaceBurnTicks
	smeltTicks := engine.tunables.FurnaceSmeltTicks
	for _, key := range engine.activeInterestKeys() {
		dimension := engine.dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		chunk, ok := dimension.ReadyChunk(key.Pos)
		if !ok {
			continue
		}
		advanceChunkFurnaces(chunk, burnTicks, smeltTicks)
	}
}

// ActiveInterestKeysForTest 暴露本 tick 的活动区块集合，仅供测试断言使用。
func (engine *Engine) ActiveInterestKeysForTest() []core.ChunkKey {
	return engine.activeInterestKeys()
}

// furnaceView 定位一个熔炉引用当前指向的槽；引用失效时返回 false。
func (engine *Engine) furnaceView(ref core.ContainerRef) (*world.Chunk, world.FurnaceSlot, bool) {
	chunk, ok := engine.containerChunk(ref)
	if !ok {
		return nil, world.FurnaceSlot{}, false
	}
	furnace := chunk.Furnace(int(ref.Slot))
	if !furnace.Active || furnace.Generation != ref.Generation {
		return nil, world.FurnaceSlot{}, false
	}
	return chunk, furnace, true
}

func moveFurnaceStack(
	inventory core.Inventory,
	furnace world.FurnaceSlot,
	from, to uint8,
) (core.Inventory, world.FurnaceSlot, bool) {
	if from >= core.FurnaceViewSlots || to >= core.FurnaceViewSlots || from == to {
		return inventory, furnace, false
	}
	if to == core.FurnaceOutputSlot {
		return inventory, furnace, false
	}
	if !inventory.Valid() || !furnace.Valid() || !furnace.Active {
		return inventory, furnace, false
	}
	if from < core.InventorySlots && to < core.InventorySlots {
		next, ok := inventory.MoveStack(from, to)
		return next, furnace, ok
	}

	source, ok := furnaceViewSlot(inventory, furnace, from)
	if !ok || source.Item == core.ItemNone {
		return inventory, furnace, false
	}
	target, ok := furnaceViewSlot(inventory, furnace, to)
	if !ok {
		return inventory, furnace, false
	}

	nextSource, nextTarget, ok := mergeStacks(source, target)
	if !ok {
		return inventory, furnace, false
	}

	nextInventory, nextFurnace := inventory, furnace
	if nextInventory, nextFurnace, ok = setFurnaceViewSlot(
		nextInventory, nextFurnace, from, nextSource,
	); !ok {
		return inventory, furnace, false
	}
	if nextInventory, nextFurnace, ok = setFurnaceViewSlot(
		nextInventory, nextFurnace, to, nextTarget,
	); !ok {
		return inventory, furnace, false
	}
	if !nextFurnace.Valid() || !nextInventory.Valid() {
		return inventory, furnace, false
	}
	return nextInventory, nextFurnace, true
}

func furnaceViewSlot(
	inventory core.Inventory,
	furnace world.FurnaceSlot,
	slot uint8,
) (core.ItemStack, bool) {
	switch slot {
	case core.FurnaceInputSlot:
		return furnace.Input, true
	case core.FurnaceFuelSlot:
		return furnace.Fuel, true
	case core.FurnaceOutputSlot:
		return furnace.Output, true
	default:
		return inventory.Slot(slot)
	}
}

func setFurnaceViewSlot(
	inventory core.Inventory,
	furnace world.FurnaceSlot,
	slot uint8,
	stack core.ItemStack,
) (core.Inventory, world.FurnaceSlot, bool) {
	switch slot {
	case core.FurnaceInputSlot:
		if !stack.Valid() {
			return inventory, furnace, false
		}
		if stack.Item != core.ItemNone {
			if _, ok := core.SmeltingOutput(stack.Item); !ok {
				return inventory, furnace, false
			}
		}
		oldItem := furnace.Input.Item
		furnace.Input = stack
		if oldItem != stack.Item {
			furnace.ProgressTicks = 0
		}
	case core.FurnaceFuelSlot:
		if !allowedFurnaceStack(stack, core.ItemCoal) {
			return inventory, furnace, false
		}
		furnace.Fuel = stack
	case core.FurnaceOutputSlot:
		if !stack.Valid() {
			return inventory, furnace, false
		}
		switch stack.Item {
		case core.ItemNone, core.ItemIronIngot, core.ItemGlass, core.ItemBrick:
			furnace.Output = stack
		default:
			return inventory, furnace, false
		}
	default:
		next, ok := inventory.SetSlot(slot, stack)
		if !ok {
			return inventory, furnace, false
		}
		inventory = next
	}
	return inventory, furnace, true
}

func allowedFurnaceStack(stack core.ItemStack, allowed core.ItemID) bool {
	if stack.Item == core.ItemNone {
		return stack.Count == 0
	}
	return stack.Item == allowed && stack.Count >= 1 && stack.Count <= core.MaxStackCount
}

// SetPlayerInventoryForTest 改写某个会话的权威物品状态，仅供纵向测试构造固定场景。
func (engine *Engine) SetPlayerInventoryForTest(
	id SessionID,
	mutate func(core.Inventory) core.Inventory,
) {
	session := engine.sessions[id]
	if session == nil || session.player == nil {
		return
	}
	session.player.inventory = mutate(session.player.inventory)
	session.player.inventoryDirty = true
}

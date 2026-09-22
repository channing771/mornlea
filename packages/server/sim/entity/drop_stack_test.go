package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// `dropStackCommand` builds a whole-stack drop. `view` selects the `StackView*`
// domain, `slot` is its unified source index, and only container views carry
// `ref`. Authority derives the amount and drop position.
func dropStackCommand(
	session SessionID,
	sequence uint64,
	view, slot uint8,
	ref core.FurnaceRef,
) Command {
	return Command{
		Session:   session,
		Sequence:  sequence,
		Kind:      CommandDropStack,
		StackView: view,
		Slot:      slot,
		Furnace:   ref,
	}
}

// `dropStackFootDrop` returns the first active drop registered at the player's
// foot block in slot order.
func dropStackFootDrop(t *testing.T, engine *Engine, session SessionID) (world.DropSlot, bool) {
	t.Helper()
	state := engine.sessions[session]
	chunk, ok := engine.dimension(state.dimension).ReadyChunk(core.BlockPos{X: int32(state.player.state.Position.X()), Y: int32(state.player.state.Position.Y()), Z: int32(state.player.state.Position.Z())}.Chunk())
	if !ok {
		t.Fatal("脚底区块不可用")
	}
	want := benchFootBlockIndex(state)
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		if drop.Active && drop.BlockIndex == want {
			return drop, true
		}
	}
	return world.DropSlot{}, false
}

// `TestDropStackInventoryViewDropsWholeStackAtFeet` covers the inventory-view
// panel-drop scenario: all 12 stones leave the source slot and appear unchanged
// at the foot block without rejection.
func TestDropStackInventoryViewDropsWholeStackAtFeet(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 12}
	engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

	result := settlePlayerInteractionsTick(engine, []Command{
		dropStackCommand(session, 2, StackViewInventory, 0, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("背包域整组丢弃被拒绝: %+v", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{}) {
		t.Fatalf("来源格 = %+v，想要清空", got)
	}
	drop, ok := dropStackFootDrop(t, engine, session)
	if !ok {
		t.Fatal("脚底方块上没有登记掉落物")
	}
	if drop.Stack != (core.ItemStack{Item: core.ItemStone, Count: 12}) {
		t.Fatalf("掉落物 = %+v，想要整组 12 个石头", drop.Stack)
	}
	// Q drops and panel drops share publication and pickup delay. `advanceDrops`
	// has already advanced the delay once in this tick, from 40 to 39.
	wantDelay := engine.tunables.PlayerDropPickupDelayTicks - 1
	if drop.PickupDelayTicks != wantDelay {
		t.Fatalf("拾取延迟 = %d，想要 %d", drop.PickupDelayTicks, wantDelay)
	}
}

// `TestDropStackCraftingViewCoversGridAndBackpackRegions` covers both crafting
// grid indices 0..8 and inventory indices 9..44, including grid dirty state.
func TestDropStackCraftingViewCoversGridAndBackpackRegions(t *testing.T) {
	t.Run("网格来源", func(t *testing.T) {
		grid := CraftingGrid{Size: CraftingGridSizePersonal}
		grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
		engine, session := stackSplitReadyPlayer(t, core.Inventory{}, grid)

		result := settlePlayerInteractionsTick(engine, []Command{
			dropStackCommand(session, 2, StackViewCrafting, 0, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("网格来源丢弃被拒绝: %+v", result.Rejected)
		}
		player := engine.sessions[session].player
		if player.crafting.Slots[0] != (core.ItemStack{}) {
			t.Fatalf("网格来源格 = %+v，想要清空", player.crafting.Slots[0])
		}
		if drop, ok := dropStackFootDrop(t, engine, session); !ok ||
			drop.Stack != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
			t.Fatalf("掉落物 = %+v ok=%v，想要整组 3 个石头", drop.Stack, ok)
		}
	})
	t.Run("背包来源", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Backpack[2] = core.ItemStack{Item: core.ItemDirt, Count: 5}
		// Unified index 9+11 selects inventory slot 11, or `Backpack[2]`.
		engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{Size: CraftingGridSizePersonal})

		result := settlePlayerInteractionsTick(engine, []Command{
			dropStackCommand(session, 2, StackViewCrafting, 9+11, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("背包来源丢弃被拒绝: %+v", result.Rejected)
		}
		if got := engine.sessions[session].player.inventory.Backpack[2]; got != (core.ItemStack{}) {
			t.Fatalf("背包来源格 = %+v，想要清空", got)
		}
		if drop, ok := dropStackFootDrop(t, engine, session); !ok ||
			drop.Stack != (core.ItemStack{Item: core.ItemDirt, Count: 5}) {
			t.Fatalf("掉落物 = %+v ok=%v，想要整组 5 个泥土", drop.Stack, ok)
		}
	})
}

// `TestDropStackChestViewDropsWholeStack` covers chest settlement through the
// deferred `containerMoves` chunk-write phase.
func TestDropStackChestViewDropsWholeStack(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 8}
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 12}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)
	command := dropStackCommand(session, 3, StackViewContainer, core.ChestFirstSlot, ref)

	// Command handling only queues container drops; it neither settles nor rejects.
	preliminary := applyPlayerCommandsTick(engine, []Command{command})
	if len(preliminary.Rejected) != 0 {
		t.Fatalf("命令阶段不应拒绝容器域丢弃: %+v", preliminary.Rejected)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got.Count != 12 {
		t.Fatalf("命令阶段提前清空箱子来源格: %+v", got)
	}

	settled := finishPlayerWorldTick(engine, []Command{command})
	if len(settled.Rejected) != 0 {
		t.Fatalf("区块写相位丢弃被拒绝: %+v", settled.Rejected)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{}) {
		t.Fatalf("箱子来源格 = %+v，想要清空", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemDirt, Count: 8}) {
		t.Fatalf("丢弃箱子物品不得改动玩家背包: %+v", got)
	}
	if drop, ok := dropStackFootDrop(t, engine, session); !ok ||
		drop.Stack != (core.ItemStack{Item: core.ItemStone, Count: 12}) {
		t.Fatalf("掉落物 = %+v ok=%v，想要整组 12 个石头", drop.Stack, ok)
	}
}

// `TestDropStackFurnaceViewDropsFuelAndOutput` proves both fuel and output may
// be removed as whole stacks; output type restrictions apply only to insertion.
func TestDropStackFurnaceViewDropsFuelAndOutput(t *testing.T) {
	t.Run("燃料格", func(t *testing.T) {
		furnace := world.FurnaceSlot{}
		furnace.Fuel = core.ItemStack{Item: core.ItemCoal, Count: 6}
		engine, session, ref := stackSplitFurnaceFixture(t, core.Inventory{}, furnace, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			dropStackCommand(session, 3, StackViewContainer, core.FurnaceFuelSlot, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("燃料格丢弃被拒绝: %+v", result.Rejected)
		}
		if got := stackSplitFurnaceAt(t, engine, ref).Fuel; got != (core.ItemStack{}) {
			t.Fatalf("燃料格 = %+v，想要清空", got)
		}
		if drop, ok := dropStackFootDrop(t, engine, session); !ok ||
			drop.Stack != (core.ItemStack{Item: core.ItemCoal, Count: 6}) {
			t.Fatalf("掉落物 = %+v ok=%v，想要整组 6 个煤", drop.Stack, ok)
		}
	})
	t.Run("输出格", func(t *testing.T) {
		furnace := world.FurnaceSlot{}
		furnace.Output = core.ItemStack{Item: core.ItemIronIngot, Count: 2}
		engine, session, ref := stackSplitFurnaceFixture(t, core.Inventory{}, furnace, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			dropStackCommand(session, 3, StackViewContainer, core.FurnaceOutputSlot, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("输出格丢弃被拒绝: %+v", result.Rejected)
		}
		if got := stackSplitFurnaceAt(t, engine, ref).Output; got != (core.ItemStack{}) {
			t.Fatalf("输出格 = %+v，想要清空", got)
		}
		if drop, ok := dropStackFootDrop(t, engine, session); !ok ||
			drop.Stack != (core.ItemStack{Item: core.ItemIronIngot, Count: 2}) {
			t.Fatalf("掉落物 = %+v ok=%v，想要整组 2 个铁锭", drop.Stack, ok)
		}
	})
}

// `TestDropStackRejectsEmptySlot` proves an empty source rejects atomically at
// settlement without changing inventory or drops.
func TestDropStackRejectsEmptySlot(t *testing.T) {
	engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{Size: CraftingGridSizePersonal})
	before := engine.sessions[session].player.inventory

	result := settlePlayerInteractionsTick(engine, []Command{
		dropStackCommand(session, 2, StackViewInventory, 4, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidSlot {
		t.Fatalf("result=%+v，想要恰好一次 RejectInvalidSlot", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory; got != before {
		t.Fatalf("被拒绝的丢弃修改了背包: %+v", got)
	}
	if _, ok := dropStackFootDrop(t, engine, session); ok {
		t.Fatal("被拒绝的丢弃生成了掉落物")
	}
}

// `TestDropStackRejectsDropCapacityAtomically` proves exhausted drop capacity
// rejects without changing the source slot or chunk drops.
func TestDropStackRejectsDropCapacityAtomically(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})
	state := engine.sessions[session]
	chunk, ok := engine.dimension(state.dimension).ReadyChunk(core.BlockPos{X: int32(state.player.state.Position.X()), Y: int32(state.player.state.Position.Y()), Z: int32(state.player.state.Position.Z())}.Chunk())
	if !ok {
		t.Fatal("脚底区块不可用")
	}
	for slot := range core.DropsPerChunk {
		chunk.SetDrop(slot, world.DropSlot{
			Active:     true,
			Stack:      core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount},
			BlockIndex: uint32(slot),
		})
	}

	result := settlePlayerInteractionsTick(engine, []Command{
		dropStackCommand(session, 2, StackViewInventory, 0, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectDropCapacity {
		t.Fatalf("result=%+v，想要恰好一次 RejectDropCapacity", result.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("容量拒绝后来源格被修改: %+v", got)
	}
}

// `TestDropStackValueDomainRejects` pins atomic rejection for unknown views,
// references on non-container views, out-of-range or inactive grid slots,
// missing view relationships, and empty container sources.
func TestDropStackValueDomainRejects(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{Size: CraftingGridSizePersonal})
	chestRef := core.ContainerRef{
		Dimension: core.Overworld, Chunk: core.ChunkPos{},
		Kind: core.ContainerKindChest, Slot: 0, Generation: 1,
	}
	for name, command := range map[string]Command{
		"未知视图":     dropStackCommand(session, 2, 3, 0, core.FurnaceRef{}),
		"背包视图携带引用": dropStackCommand(session, 2, StackViewInventory, 0, chestRef),
		"背包视图索引越界": dropStackCommand(session, 2, StackViewInventory, core.InventorySlots, core.FurnaceRef{}),
		"合成视图索引越界": dropStackCommand(session, 2, StackViewCrafting, craftingViewSlots, core.FurnaceRef{}),
		"个人网格扩展格":  dropStackCommand(session, 2, StackViewCrafting, 4, core.FurnaceRef{}),
	} {
		t.Run(name, func(t *testing.T) {
			result := settlePlayerInteractionsTick(engine, []Command{command})
			if len(result.Rejected) != 1 {
				t.Fatalf("result=%+v，想要恰好一次拒绝", result.Rejected)
			}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 4}) {
				t.Fatalf("被拒绝的丢弃修改了背包: %+v", got)
			}
			if _, ok := dropStackFootDrop(t, engine, session); ok {
				t.Fatal("被拒绝的丢弃生成了掉落物")
			}
		})
	}

	// Container rejection occurs in the deferred `containerMoves` chunk-write
	// phase; missing view relationships and empty sources are checked separately.
	var chestInventory core.Inventory
	chestEngine, chestSession, ref := stackSplitChestFixture(t, chestInventory, world.ChestSlot{}, CraftingGrid{})
	unviewed := finishPlayerWorldTick(chestEngine, []Command{
		dropStackCommand(chestSession, 3, StackViewContainer, 0, ref),
	})
	if len(unviewed.Rejected) != 1 || unviewed.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("未查看容器 result=%+v，想要恰好一次 RejectInvalidInput", unviewed.Rejected)
	}
	openStackSplitChest(t, chestEngine, chestSession)
	result := finishPlayerWorldTick(chestEngine, []Command{
		dropStackCommand(chestSession, 4, StackViewContainer, core.ChestFirstSlot, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidSlot {
		t.Fatalf("容器空源 result=%+v，想要恰好一次 RejectInvalidSlot", result.Rejected)
	}
}

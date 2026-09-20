package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// dropStackCommand 构造一条整组丢弃命令：view 是 StackView* 视图域，slot 是
// 该域的来源统一索引；容器视图携带 ref，其余视图传零值引用。投放位置与
// 数量都由服务端从权威状态推导。
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

// dropStackFootDrop 返回玩家脚底方块上当前登记的掉落物（按槽位顺序找到
// 首个落在脚底索引上的活动掉落物），没有则返回 false。
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

// TestDropStackInventoryViewDropsWholeStackAtFeet 验收 spec 场景「拖出面板
// 出现整组掉落物」的背包域形态：来源格 12 个石头整组取出，掉落物按脚底
// 方块登记且数量保持 12，来源格清空，无拒绝。
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
	// 与 Q 键单件丢弃同一投放出口：拾取延迟取同一 tunable，且同 tick 的
	// `advanceDrops` 已把它推进一格（40 → 39），防止捡回抢跑。
	wantDelay := engine.tunables.PlayerDropPickupDelayTicks - 1
	if drop.PickupDelayTicks != wantDelay {
		t.Fatalf("拾取延迟 = %d，想要 %d", drop.PickupDelayTicks, wantDelay)
	}
}

// TestDropStackCraftingViewCoversGridAndBackpackRegions 验收合成域两种来源：
// 网格统一格 0..8 与背包统一格 9..44 都能整组拖出，网格来源同时清网格脏位。
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
		// 统一视图 9+11 = 背包格 11（Backpack[2]）。
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

// TestDropStackChestViewDropsWholeStack 验收容器域（箱子）：箱子来源格整组
// 取出脚下投放、来源格清空，经 containerMoves 延迟通道在区块写相位结算。
func TestDropStackChestViewDropsWholeStack(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 8}
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 12}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)
	command := dropStackCommand(session, 3, StackViewContainer, core.ChestFirstSlot, ref)

	// 命令阶段只入队：容器域丢弃既不结算也不拒绝。
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

// TestDropStackFurnaceViewDropsFuelAndOutput 验收容器域（熔炉）：燃料格与
// 输出格都可整组拖出（输出格只限制写入物品类型，取出丢弃不受限）。
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

// TestDropStackRejectsEmptySlot 验收 spec 场景「空槽拖出不产生掉落」：结算
// 时点来源已空整单拒绝，背包与掉落状态零变化。
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

// TestDropStackRejectsDropCapacityAtomically 锁定容量拒绝的原子性：脚底掉落
// 容量占满时整单拒绝，来源格与区块掉落零变化。
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

// TestDropStackValueDomainRejects 锁定值域与查看关系拒绝：未知视图、非容器
// 视图携带引用、索引越界、个人网格扩展格、未建立查看关系与容器空源都按
// 既有拒绝语义整单拒绝且状态零变化。
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

	// 容器域走 containerMoves 延迟通道，拒绝在区块写相位（FinishWorld）产生；
	// 未建立查看关系与空源分别验收。
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

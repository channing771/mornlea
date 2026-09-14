package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestQuickMoveFurnaceSlotPriority 验收 spec 场景「背包快捷搬运进熔炉的槽位
// 优先级」：生铁（熔炼输入）整堆进输入槽、煤整堆进燃料槽、熔炼输入并入同类
// 未满输入槽不重置进度；既非熔炼输入也非煤的物品整单拒绝。
func TestQuickMoveFurnaceSlotPriority(t *testing.T) {
	t.Run("生铁整堆进输入槽", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemRawIron, Count: 8}
		engine, session, ref := stackSplitFurnaceFixture(t, inventory, world.FurnaceSlot{}, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("生铁快捷搬运被拒绝: %+v", result.Rejected)
		}
		got := stackSplitFurnaceAt(t, engine, ref)
		if got.Input != (core.ItemStack{Item: core.ItemRawIron, Count: 8}) {
			t.Fatalf("输入槽 = %+v，想要整堆 8", got.Input)
		}
		if got.Fuel != (core.ItemStack{}) {
			t.Fatalf("燃料槽不应承接生铁: %+v", got.Fuel)
		}
		if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{}) {
			t.Fatalf("来源格 = %+v，想要清空", hotbar)
		}
	})
	t.Run("煤整堆进燃料槽", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemCoal, Count: 5}
		furnace := world.FurnaceSlot{Input: core.ItemStack{Item: core.ItemSand, Count: 3}}
		engine, session, ref := stackSplitFurnaceFixture(t, inventory, furnace, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("煤快捷搬运被拒绝: %+v", result.Rejected)
		}
		got := stackSplitFurnaceAt(t, engine, ref)
		if got.Fuel != (core.ItemStack{Item: core.ItemCoal, Count: 5}) {
			t.Fatalf("燃料槽 = %+v，想要整堆 5", got.Fuel)
		}
		if got.Input != (core.ItemStack{Item: core.ItemSand, Count: 3}) {
			t.Fatalf("煤不是熔炼输入，输入格不得改动: %+v", got.Input)
		}
	})
	t.Run("输入并入同类未满不重置进度", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemRawIron, Count: 5}
		furnace := world.FurnaceSlot{
			Input:         core.ItemStack{Item: core.ItemRawIron, Count: 3},
			ProgressTicks: 137,
			BurnTicks:     1463,
		}
		engine, session, ref := stackSplitFurnaceFixture(t, inventory, furnace, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("同类并入被拒绝: %+v", result.Rejected)
		}
		got := stackSplitFurnaceAt(t, engine, ref)
		if got.Input != (core.ItemStack{Item: core.ItemRawIron, Count: 8}) {
			t.Fatalf("输入槽 = %+v，想要 8", got.Input)
		}
		if got.ProgressTicks != 137 || got.BurnTicks != 1463 {
			t.Fatalf("同类并入不应重置进度: %+v", got)
		}
	})
	t.Run("既非输入也非燃料拒绝", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 8}
		engine, session, ref := stackSplitFurnaceFixture(t, inventory, world.FurnaceSlot{}, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
		})
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
			t.Fatalf("石头快捷搬运 result=%+v，想要恰好一次 RejectInvalidInput", result)
		}
		if got := stackSplitFurnaceAt(t, engine, ref); got.Input != (core.ItemStack{}) ||
			got.Fuel != (core.ItemStack{}) || got.Output != (core.ItemStack{}) {
			t.Fatalf("被拒绝的搬运修改了熔炉: %+v", got)
		}
		if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar.Count != 8 {
			t.Fatalf("被拒绝的搬运修改了玩家物品: %+v", hotbar)
		}
	})
	t.Run("铁锭不进输出槽", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronIngot, Count: 3}
		engine, session, ref := stackSplitFurnaceFixture(t, inventory, world.FurnaceSlot{}, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
		})
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
			t.Fatalf("铁锭快捷搬运 result=%+v，想要恰好一次 RejectInvalidInput", result)
		}
		if got := stackSplitFurnaceAt(t, engine, ref); got.Input != (core.ItemStack{}) ||
			got.Fuel != (core.ItemStack{}) || got.Output != (core.ItemStack{}) {
			t.Fatalf("输出槽不得作为快捷搬运目标: %+v", got)
		}
	})
}

// TestQuickMoveFurnaceBackpackToInputNoFitRejects 锁定熔炉优先级的无可容纳
// 拒绝：输入槽同类已满且物品不是燃料时整单拒绝、零改动。
func TestQuickMoveFurnaceBackpackToInputNoFitRejects(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemRawIron, Count: 8}
	furnace := world.FurnaceSlot{
		Input: core.ItemStack{Item: core.ItemRawIron, Count: core.MaxStackCount},
	}
	engine, session, ref := stackSplitFurnaceFixture(t, inventory, furnace, CraftingGrid{})
	openStackSplitFurnace(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		quickMoveCommand(session, 3, StackViewContainer, 0, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("输入槽满 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitFurnaceAt(t, engine, ref); got.Input != (core.ItemStack{Item: core.ItemRawIron, Count: core.MaxStackCount}) {
		t.Fatalf("被拒绝的搬运修改了熔炉: %+v", got)
	}
	if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar.Count != 8 {
		t.Fatalf("被拒绝的搬运修改了玩家物品: %+v", hotbar)
	}
}

// TestQuickMoveFurnaceContainerRegionToBackpack 锁定熔炉容器区→背包方向：
// 输入/燃料/输出格作为来源都按拾取四相位序整堆并入背包，余量留源。
func TestQuickMoveFurnaceContainerRegionToBackpack(t *testing.T) {
	t.Run("输入整堆并入并清空", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemRawIron, Count: 34}
		furnace := world.FurnaceSlot{Input: core.ItemStack{Item: core.ItemRawIron, Count: 30}}
		engine, session, ref := stackSplitFurnaceFixture(t, inventory, furnace, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, core.FurnaceInputSlot, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("输入→背包被拒绝: %+v", result.Rejected)
		}
		if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemRawIron, Count: 64}) {
			t.Fatalf("同类合并格 = %+v，想要并到 64", hotbar)
		}
		if got := stackSplitFurnaceAt(t, engine, ref).Input; got != (core.ItemStack{}) {
			t.Fatalf("输入来源格 = %+v，想要清空", got)
		}
	})
	t.Run("输出余量留源", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemIronIngot, Count: 58}
		quickMoveFillExcept(&inventory, []uint8{0}, core.ItemDirt)
		furnace := world.FurnaceSlot{Output: core.ItemStack{Item: core.ItemIronIngot, Count: 10}}
		engine, session, ref := stackSplitFurnaceFixture(t, inventory, furnace, CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, core.FurnaceOutputSlot, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("输出部分吸收应成功而非拒绝: %+v", result.Rejected)
		}
		if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemIronIngot, Count: core.MaxStackCount}) {
			t.Fatalf("同类合并格 = %+v，想要并到 64", hotbar)
		}
		if got := stackSplitFurnaceAt(t, engine, ref).Output; got != (core.ItemStack{Item: core.ItemIronIngot, Count: 4}) {
			t.Fatalf("输出来源格 = %+v，想要余 4", got)
		}
	})
}

// TestQuickMoveFurnaceRejectsBrokenRepackInvariant 锁定熔炉→背包增量的回收
// 预演与箱子路径同形：整堆并入占用网格回收依赖的唯一空格时整体拒绝。
func TestQuickMoveFurnaceRejectsBrokenRepackInvariant(t *testing.T) {
	furnace := world.FurnaceSlot{Input: core.ItemStack{Item: core.ItemRawIron, Count: 30}}
	engine, session, ref := stackSplitFurnaceFixture(
		t, stackSplitContainerRepackInventory(), furnace, stackSplitContainerRepackGrid(),
	)
	openStackSplitFurnace(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		quickMoveCommand(session, 3, StackViewContainer, core.FurnaceInputSlot, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("破坏回收不变量的搬运 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitFurnaceAt(t, engine, ref).Input; got != (core.ItemStack{Item: core.ItemRawIron, Count: 30}) {
		t.Fatalf("被拒绝的搬运修改了熔炉: %+v", got)
	}
	player := engine.sessions[session].player
	if player.inventory.Backpack[0] != (core.ItemStack{}) {
		t.Fatalf("被拒绝的搬运修改了背包: %+v", player.inventory)
	}
}

// TestQuickMoveFurnaceValueDomainRejects 锁定熔炉域值域：熔炉统一视图上界
// 与箱子专属栏位来源都按 RejectInvalidInput 拒绝，不依赖网络层已按 Kind
// 校验。
func TestQuickMoveFurnaceValueDomainRejects(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemRawIron, Count: 4}
	engine, session, ref := stackSplitFurnaceFixture(t, inventory, world.FurnaceSlot{}, CraftingGrid{})
	openStackSplitFurnace(t, engine, session)

	for name, command := range map[string]Command{
		"熔炉视图上界": quickMoveCommand(session, 3, StackViewContainer, core.FurnaceViewSlots, ref),
		"箱子专属栏位": quickMoveCommand(session, 3, StackViewContainer, core.ChestFirstSlot+5, ref),
	} {
		t.Run(name, func(t *testing.T) {
			result := finishPlayerWorldTick(engine, []Command{command})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("result=%+v，想要恰好一次 RejectInvalidInput", result)
			}
			if got := stackSplitFurnaceAt(t, engine, ref); got.Input != (core.ItemStack{}) {
				t.Fatalf("被拒绝的搬运修改了熔炉: %+v", got)
			}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got.Count != 4 {
				t.Fatalf("被拒绝的搬运修改了玩家物品: %+v", got)
			}
		})
	}
}

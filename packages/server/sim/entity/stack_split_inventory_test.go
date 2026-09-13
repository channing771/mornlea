package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// stackSplitCommand 构造一条部分数量移动命令：view 是 StackView* 视图域，
// from/to 是该域的统一索引，single 在半组（false）与单件（true）两档间选择；
// 容器视图携带 ref，其余视图传零值引用。
func stackSplitCommand(
	session SessionID,
	sequence uint64,
	view, from, to uint8,
	single bool,
	ref core.FurnaceRef,
) Command {
	return Command{
		Session:   session,
		Sequence:  sequence,
		Kind:      CommandMoveStackPartial,
		StackView: view,
		Slot:      from,
		ToSlot:    to,
		Single:    single,
		Furnace:   ref,
	}
}

// stackSplitReadyPlayer 构造一个已激活玩家并按需改写其物品与合成网格，随后
// 跑一个空命令 tick 冲掉夹具置位的 dirty 标志，让测试对发布与 dirty 断言
// 从零开始。
func stackSplitReadyPlayer(
	t *testing.T,
	inventory core.Inventory,
	grid CraftingGrid,
) (*Engine, SessionID) {
	t.Helper()
	engine, session := readyMovementPlayer(t)
	engine.SetPlayerInventoryForTest(session, func(core.Inventory) core.Inventory {
		return inventory
	})
	if grid != (CraftingGrid{}) {
		engine.SetPlayerCraftingGridForTest(session, func(CraftingGrid) CraftingGrid {
			return grid
		})
	}
	applyPlayerCommandsTick(engine, nil)
	return engine, session
}

// TestStackSplitInventoryHalfDerivesCeilingAmount 验收 spec 场景「半组移动数量
// 由服务端推导」的背包域形态：7 个来源移出恰好 ceil(7/2)=4，来源剩 3；命令
// 阶段内联结算并发布一次完整物品状态。
func TestStackSplitInventoryHalfDerivesCeilingAmount(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

	result := applyPlayerCommandsTick(engine, []Command{
		stackSplitCommand(session, 2, StackViewInventory, 0, 4, false, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("背包域半组移动被拒绝: %+v", result.Rejected)
	}
	got := engine.sessions[session].player.inventory
	if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("来源格 = %+v，想要剩 3", got.Hotbar.Slots[0])
	}
	if got.Hotbar.Slots[4] != (core.ItemStack{Item: core.ItemStone, Count: 4}) {
		t.Fatalf("目标格 = %+v，想要恰好 4 个", got.Hotbar.Slots[4])
	}
	if len(result.Inventories) != 1 {
		t.Fatalf("成功移动应发布恰好一次完整物品状态: %+v", result.Inventories)
	}
}

// TestStackSplitInventorySingleAndHalfMergeIntoNearlyFull 验收 spec 场景
// 「单件移动进同类未满格」：63/64 的同类目标并入恰好 1 个；半组对 63/64 的
// 目标也只按剩余容量截断为 1，余量留源。
func TestStackSplitInventorySingleAndHalfMergeIntoNearlyFull(t *testing.T) {
	t.Run("单件", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 5}
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount - 1}
		engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

		result := applyPlayerCommandsTick(engine, []Command{
			stackSplitCommand(session, 2, StackViewInventory, 0, 1, true, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("单件并入未满格被拒绝: %+v", result.Rejected)
		}
		got := engine.sessions[session].player.inventory
		if got.Hotbar.Slots[1].Count != core.MaxStackCount {
			t.Fatalf("目标格 = %+v，想要恰好 64", got.Hotbar.Slots[1])
		}
		if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 4}) {
			t.Fatalf("来源格 = %+v，想要减 1 剩 4", got.Hotbar.Slots[0])
		}
	})
	t.Run("半组按剩余容量截断", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 7}
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount - 1}
		engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

		result := applyPlayerCommandsTick(engine, []Command{
			stackSplitCommand(session, 2, StackViewInventory, 0, 1, false, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("半组截断合并被拒绝: %+v", result.Rejected)
		}
		got := engine.sessions[session].player.inventory
		if got.Hotbar.Slots[1].Count != core.MaxStackCount {
			t.Fatalf("目标格 = %+v，想要恰好 64", got.Hotbar.Slots[1])
		}
		if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 6}) {
			t.Fatalf("来源格 = %+v，想要只移走 1 剩 6", got.Hotbar.Slots[0])
		}
	})
}

// TestStackSplitInventoryRejectsDifferentItemTarget 验收 spec 场景「异类非空
// 目标整单拒绝」：半组与单件对异类非空目标都稳定拒绝且任何栏位零变化——
// 部分移动不做交换。
func TestStackSplitInventoryRejectsDifferentItemTarget(t *testing.T) {
	for name, single := range map[string]bool{"半组": false, "单件": true} {
		t.Run(name, func(t *testing.T) {
			var inventory core.Inventory
			inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
			inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemDirt, Count: 10}
			engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

			result := applyPlayerCommandsTick(engine, []Command{
				stackSplitCommand(session, 2, StackViewInventory, 0, 4, single, core.FurnaceRef{}),
			})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("异类目标 result=%+v，想要恰好一次 RejectInvalidInput", result)
			}
			got := engine.sessions[session].player.inventory
			if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 7}) ||
				got.Hotbar.Slots[4] != (core.ItemStack{Item: core.ItemDirt, Count: 10}) {
				t.Fatalf("被拒绝的移动修改了栏位: %+v", got)
			}
			if len(result.Inventories) != 0 {
				t.Fatalf("被拒绝的移动不应发布物品状态: %+v", result.Inventories)
			}
		})
	}
}

// TestStackSplitInventoryRejectsEmptySource 锁定数量纪律：空源的推导数量为 0，
// 整单按 RejectInvalidInput 拒绝且零改动。
func TestStackSplitInventoryRejectsEmptySource(t *testing.T) {
	engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{})

	result := applyPlayerCommandsTick(engine, []Command{
		stackSplitCommand(session, 2, StackViewInventory, 0, 4, false, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("空源 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := engine.sessions[session].player.inventory; got != (core.Inventory{}) {
		t.Fatalf("空源拒绝修改了物品: %+v", got)
	}
}

// TestStackSplitInventoryValueDomainRejects 锁定背包域值域：索引越界
// RejectInvalidSlot、同格与非容器视图携带容器引用 RejectInvalidInput、未知
// 视图域 RejectInvalidInput。
func TestStackSplitInventoryValueDomainRejects(t *testing.T) {
	ref := core.FurnaceRef{
		Dimension: core.Overworld, Kind: core.ContainerKindChest, Slot: 3, Generation: 1,
	}
	cases := []struct {
		name    string
		command Command
		reason  RejectReason
	}{
		{"目标索引越界", stackSplitCommand(1, 2, StackViewInventory, 0, core.InventorySlots, false, core.FurnaceRef{}), RejectInvalidSlot},
		{"来源索引越界", stackSplitCommand(1, 2, StackViewInventory, core.InventorySlots, 0, false, core.FurnaceRef{}), RejectInvalidSlot},
		{"同格", stackSplitCommand(1, 2, StackViewInventory, 3, 3, false, core.FurnaceRef{}), RejectInvalidInput},
		{"背包视图携带容器引用", stackSplitCommand(1, 2, StackViewInventory, 0, 4, false, ref), RejectInvalidInput},
		{"未知视图域", stackSplitCommand(1, 2, 3, 0, 4, false, core.FurnaceRef{}), RejectInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{})
			tc.command.Session = session
			result := applyPlayerCommandsTick(engine, []Command{tc.command})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != tc.reason {
				t.Fatalf("result=%+v，想要恰好一次 %d", result, tc.reason)
			}
		})
	}
}

// TestStackSplitRejectsWhenPlayerNotReady 锁定未就绪拒绝：未激活玩家在背包域
// （命令阶段）与容器域（区块写相位）都按 RejectPlayerNotReady 稳定拒绝。
func TestStackSplitRejectsWhenPlayerNotReady(t *testing.T) {
	t.Run("背包域命令阶段", func(t *testing.T) {
		engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{})
		engine.sessions[session].player.lifecycle = PlayerPendingSpawn
		result := applyPlayerCommandsTick(engine, []Command{
			stackSplitCommand(session, 2, StackViewInventory, 0, 4, false, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectPlayerNotReady {
			t.Fatalf("未就绪 result=%+v，想要恰好一次 RejectPlayerNotReady", result)
		}
	})
	t.Run("容器域区块写相位", func(t *testing.T) {
		engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{})
		engine.sessions[session].player.lifecycle = PlayerPendingSpawn
		command := stackSplitCommand(session, 2, StackViewContainer, 0, core.ChestFirstSlot, false, core.FurnaceRef{})
		result := finishPlayerWorldTick(engine, []Command{command})
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectPlayerNotReady {
			t.Fatalf("未就绪容器域 result=%+v，想要恰好一次 RejectPlayerNotReady", result)
		}
	})
}

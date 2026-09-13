package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// quickMoveCommand 构造一条快捷搬运命令：view 是 StackView* 视图域，from 是
// 该域的来源统一索引；容器视图携带 ref，其余视图传零值引用。命令不携带
// 目标——目标序是服务端权威推导的固定确定性契约。
func quickMoveCommand(
	session SessionID,
	sequence uint64,
	view, from uint8,
	ref core.FurnaceRef,
) Command {
	return Command{
		Session:   session,
		Sequence:  sequence,
		Kind:      CommandQuickMoveStack,
		StackView: view,
		Slot:      from,
		Furnace:   ref,
	}
}

// quickMoveFillExcept 在 inventory 上把除 skip 外的全部格子填满异类物品，
// 构造「对侧仅剩有限容量」的快捷搬运场景。
func quickMoveFillExcept(inventory *core.Inventory, skip []uint8, filler core.ItemID) {
	occupied := make(map[uint8]bool, len(skip))
	for _, slot := range skip {
		occupied[slot] = true
	}
	for slot := uint8(0); slot < core.InventorySlots; slot++ {
		if !occupied[slot] {
			next, _ := inventory.SetSlot(slot, core.ItemStack{Item: filler, Count: core.MaxStackCount})
			*inventory = next
		}
	}
}

// TestQuickMoveInventoryHotbarToBackpack 验收纯背包面板快捷栏→背包方向：
// 区域受限两相位序（同类未满升序合并先于空格升序占用）、绝不跨区承接、
// 余量留源部分吸收。
func TestQuickMoveInventoryHotbarToBackpack(t *testing.T) {
	t.Run("同类合并先于空格且不跨区", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
		inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemDirt, Count: 5}
		inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 63}
		inventory.Backpack[1] = core.ItemStack{Item: core.ItemDirt, Count: 10}
		engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

		result := applyPlayerCommandsTick(engine, []Command{
			quickMoveCommand(session, 2, StackViewInventory, 0, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("快捷栏→背包被拒绝: %+v", result.Rejected)
		}
		got := engine.sessions[session].player.inventory
		if got.Backpack[0] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
			t.Fatalf("同类合并格 = %+v，想要并到 64", got.Backpack[0])
		}
		if got.Backpack[2] != (core.ItemStack{Item: core.ItemStone, Count: 63}) {
			t.Fatalf("空格承接余量 = %+v，想要 63", got.Backpack[2])
		}
		if got.Hotbar.Slots[0] != (core.ItemStack{}) {
			t.Fatalf("来源格 = %+v，想要清空", got.Hotbar.Slots[0])
		}
		if got.Hotbar.Slots[1] != (core.ItemStack{}) {
			t.Fatalf("对侧区域之外的快捷栏空格不得承接: %+v", got.Hotbar.Slots[1])
		}
		if got.Hotbar.Slots[2] != (core.ItemStack{Item: core.ItemDirt, Count: 5}) ||
			got.Backpack[1] != (core.ItemStack{Item: core.ItemDirt, Count: 10}) {
			t.Fatalf("无关栏位被改动: %+v", got)
		}
		if len(result.Inventories) != 1 {
			t.Fatalf("成功移动应发布恰好一次完整物品状态: %+v", result.Inventories)
		}
	})
	t.Run("余量留源部分吸收", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
		inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 62}
		quickMoveFillExcept(&inventory, []uint8{0, 9}, core.ItemDirt)
		engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

		result := applyPlayerCommandsTick(engine, []Command{
			quickMoveCommand(session, 2, StackViewInventory, 0, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("部分吸收应成功而非拒绝: %+v", result.Rejected)
		}
		got := engine.sessions[session].player.inventory
		if got.Backpack[0] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
			t.Fatalf("同类合并格 = %+v，想要并到 64", got.Backpack[0])
		}
		if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 62}) {
			t.Fatalf("来源格 = %+v，想要余 62", got.Hotbar.Slots[0])
		}
	})
}

// TestQuickMoveInventoryBackpackToHotbar 验收背包→快捷栏方向：同一份区域
// 受限两相位序反向作用，快捷栏之外的背包空格不得承接。
func TestQuickMoveInventoryBackpackToHotbar(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 30}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 3}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewInventory, 9, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("背包→快捷栏被拒绝: %+v", result.Rejected)
	}
	got := engine.sessions[session].player.inventory
	if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
		t.Fatalf("同类合并格 = %+v，想要并到 64", got.Hotbar.Slots[0])
	}
	if got.Hotbar.Slots[2] != (core.ItemStack{Item: core.ItemStone, Count: 30}) {
		t.Fatalf("空格承接余量 = %+v，想要 30", got.Hotbar.Slots[2])
	}
	if got.Backpack[0] != (core.ItemStack{}) {
		t.Fatalf("来源格 = %+v，想要清空", got.Backpack[0])
	}
	if got.Backpack[1] != (core.ItemStack{}) {
		t.Fatalf("对侧区域之外的背包空格不得承接: %+v", got.Backpack[1])
	}
}

// TestQuickMoveInventoryRejectsWhenNoFit 验收 spec 场景「无可容纳位置整单
// 拒绝」的纯背包形态：对侧全部异类非空时整单 RejectInvalidInput 且任何
// 栏位零变化、零发布。
func TestQuickMoveInventoryRejectsWhenNoFit(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	quickMoveFillExcept(&inventory, []uint8{0}, core.ItemDirt)
	engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{})

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewInventory, 0, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("无可容纳 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := engine.sessions[session].player.inventory; got != inventory {
		t.Fatalf("被拒绝的移动修改了栏位: %+v", got)
	}
	if len(result.Inventories) != 0 {
		t.Fatalf("被拒绝的移动不应发布物品状态: %+v", result.Inventories)
	}
}

// TestQuickMoveInventoryRejectsEmptySource 锁定空源拒绝：零物品可移动按
// RejectInvalidInput 整单拒绝。
func TestQuickMoveInventoryRejectsEmptySource(t *testing.T) {
	engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{})

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewInventory, 0, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("空源 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := engine.sessions[session].player.inventory; got != (core.Inventory{}) {
		t.Fatalf("空源拒绝修改了物品: %+v", got)
	}
}

// TestQuickMoveInventoryValueDomainRejects 锁定背包域值域：索引越界
// RejectInvalidSlot、非容器视图携带容器引用与未知视图域 RejectInvalidInput。
func TestQuickMoveInventoryValueDomainRejects(t *testing.T) {
	ref := core.FurnaceRef{
		Dimension: core.Overworld, Kind: core.ContainerKindChest, Slot: 3, Generation: 1,
	}
	cases := []struct {
		name    string
		command Command
		reason  RejectReason
	}{
		{"来源索引越界", quickMoveCommand(1, 2, StackViewInventory, core.InventorySlots, core.FurnaceRef{}), RejectInvalidSlot},
		{"背包视图携带容器引用", quickMoveCommand(1, 2, StackViewInventory, 0, ref), RejectInvalidInput},
		{"未知视图域", quickMoveCommand(1, 2, 3, 0, core.FurnaceRef{}), RejectInvalidInput},
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

// TestQuickMoveRejectsWhenPlayerNotReady 锁定未就绪拒绝：背包域（命令阶段
// 内联）与容器域（区块写相位延迟）都按 RejectPlayerNotReady 稳定拒绝。
func TestQuickMoveRejectsWhenPlayerNotReady(t *testing.T) {
	t.Run("背包域命令阶段", func(t *testing.T) {
		engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{})
		engine.sessions[session].player.lifecycle = PlayerPendingSpawn
		result := applyPlayerCommandsTick(engine, []Command{
			quickMoveCommand(session, 2, StackViewInventory, 0, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectPlayerNotReady {
			t.Fatalf("未就绪 result=%+v，想要恰好一次 RejectPlayerNotReady", result)
		}
	})
	t.Run("容器域区块写相位", func(t *testing.T) {
		engine, session := stackSplitReadyPlayer(t, core.Inventory{}, CraftingGrid{})
		engine.sessions[session].player.lifecycle = PlayerPendingSpawn
		command := quickMoveCommand(session, 2, StackViewContainer, 0, core.FurnaceRef{})
		result := finishPlayerWorldTick(engine, []Command{command})
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectPlayerNotReady {
			t.Fatalf("未就绪容器域 result=%+v，想要恰好一次 RejectPlayerNotReady", result)
		}
	})
}

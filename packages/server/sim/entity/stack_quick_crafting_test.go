package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestQuickMoveCraftingGridToBackpackPickupOrder 验收网格→背包方向：整堆按
// 拾取四相位序（快捷栏同类、快捷栏空格、背包同类、背包空格）并入背包，
// 快捷栏空格优先于背包同类合并，网格来源格清空。
func TestQuickMoveCraftingGridToBackpackPickupOrder(t *testing.T) {
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	grid.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 3}
	var inventory core.Inventory
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 5}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 63}
	engine, session := stackSplitReadyPlayer(t, inventory, grid)

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewCrafting, 0, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("网格→背包被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	// 四相位序：快捷栏没有同类石头，快捷栏空格（0）先于背包同类（63/64）
	// 承接，整堆 64 全部进入快捷栏 0。
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 64}) {
		t.Fatalf("快捷栏空格 = %+v，想要承接整堆 64", player.inventory.Hotbar.Slots[0])
	}
	if player.inventory.Backpack[0] != (core.ItemStack{Item: core.ItemStone, Count: 63}) {
		t.Fatalf("背包同类格 = %+v，想要保持 63 不变", player.inventory.Backpack[0])
	}
	if player.crafting.Slots[0] != (core.ItemStack{}) {
		t.Fatalf("网格来源格 = %+v，想要清空", player.crafting.Slots[0])
	}
	if player.crafting.Slots[1] != (core.ItemStack{Item: core.ItemDirt, Count: 3}) {
		t.Fatalf("无关网格格被改动: %+v", player.crafting.Slots[1])
	}
	if len(result.Craftings) != 1 || len(result.Inventories) != 1 {
		t.Fatalf("成功移动应各发布一次网格与物品状态: craftings=%+v inventories=%+v",
			result.Craftings, result.Inventories)
	}
}

// TestQuickMoveCraftingGridToBackpackFullyAbsorbsPartialCapacity 锁定网格→
// 背包在容量紧张时的完整吸收：唯一空格 + 同类余量恰好吸收整堆（快捷搬运
// 的吸收序与回收序同构，容量只要能装回网格就必然装得下整堆迁移）。
func TestQuickMoveCraftingGridToBackpackFullyAbsorbsPartialCapacity(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 20}
	quickMoveFillExcept(&inventory, []uint8{0, 9}, core.ItemSand)
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	grid.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 40}
	engine, session := stackSplitReadyPlayer(t, inventory, grid)

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewCrafting, 1, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("整堆迁移被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 60}) {
		t.Fatalf("同类合并格 = %+v，想要 60", player.inventory.Hotbar.Slots[0])
	}
	if player.crafting.Slots[1] != (core.ItemStack{}) {
		t.Fatalf("网格来源格 = %+v，想要清空", player.crafting.Slots[1])
	}
}

// TestQuickMoveCraftingBackpackToGridFirstFitting 验收背包→网格方向：网格
// 0..有效尺寸 升序进首个「空或同类未满」格（异类与同类满格跳过），余量留源。
func TestQuickMoveCraftingBackpackToGridFirstFitting(t *testing.T) {
	t.Run("异类跳过同类未满承接", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
		grid := CraftingGrid{Size: CraftingGridSizePersonal}
		grid.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 10}
		grid.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 32}
		engine, session := stackSplitReadyPlayer(t, inventory, grid)

		result := applyPlayerCommandsTick(engine, []Command{
			quickMoveCommand(session, 2, StackViewCrafting, 9, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("背包→网格被拒绝: %+v", result.Rejected)
		}
		player := engine.sessions[session].player
		if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 10}) {
			t.Fatalf("异类网格格被改动: %+v", player.crafting.Slots[0])
		}
		if player.crafting.Slots[1] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
			t.Fatalf("同类未满格 = %+v，想要并到 64", player.crafting.Slots[1])
		}
		if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 32}) {
			t.Fatalf("来源格 = %+v，想要余 32", player.inventory.Hotbar.Slots[0])
		}
	})
	t.Run("同类满格跳过空格承接", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
		grid := CraftingGrid{Size: CraftingGridSizePersonal}
		grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
		engine, session := stackSplitReadyPlayer(t, inventory, grid)

		result := applyPlayerCommandsTick(engine, []Command{
			quickMoveCommand(session, 2, StackViewCrafting, 9, core.FurnaceRef{}),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("空格承接被拒绝: %+v", result.Rejected)
		}
		player := engine.sessions[session].player
		if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
			t.Fatalf("同类满格 = %+v，想要保持 64", player.crafting.Slots[0])
		}
		if player.crafting.Slots[1] != (core.ItemStack{Item: core.ItemStone, Count: 64}) {
			t.Fatalf("空网格格 = %+v，想要承接整堆", player.crafting.Slots[1])
		}
		if player.inventory.Hotbar.Slots[0] != (core.ItemStack{}) {
			t.Fatalf("来源格 = %+v，想要清空", player.inventory.Hotbar.Slots[0])
		}
	})
}

// TestQuickMoveCraftingScanLimitedToActiveExtent 锁定目标序的网格范围：
// 个人 2×2 只扫描格 0..3，扩展格 4..8 即使为空也不得承接（升序扫描上界是
// 有效尺寸，不是物理存储上界）。
func TestQuickMoveCraftingScanLimitedToActiveExtent(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	for slot := 0; slot < int(personalGridExtent); slot++ {
		grid.Slots[slot] = core.ItemStack{Item: core.ItemDirt, Count: 10}
	}
	engine, session := stackSplitReadyPlayer(t, inventory, grid)

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewCrafting, 9, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("个人网格无可容纳 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	player := engine.sessions[session].player
	if player.inventory.Hotbar.Slots[0].Count != 64 {
		t.Fatalf("被拒绝的移动修改了背包: %+v", player.inventory.Hotbar.Slots[0])
	}
	for slot := int(personalGridExtent); slot < core.CraftingGridSlots; slot++ {
		if player.crafting.Slots[slot] != (core.ItemStack{}) {
			t.Fatalf("扩展格 %d 不应承接: %+v", slot, player.crafting.Slots[slot])
		}
	}
}

// TestQuickMoveCraftingWorkbenchExtentCoversExtendedGrid 是范围判定的对照：
// 工作台 3×3 的扩展格既是合法目标也是合法来源。
func TestQuickMoveCraftingWorkbenchExtentCoversExtendedGrid(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	grid := CraftingGrid{Size: CraftingGridSizeWorkbench}
	grid.Slots[8] = core.ItemStack{Item: core.ItemDirt, Count: 5}
	engine, session := stackSplitReadyPlayer(t, inventory, grid)

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewCrafting, 9, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("工作台扩展格承接被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 7}) {
		t.Fatalf("网格格 0 = %+v，想要承接 7", player.crafting.Slots[0])
	}

	reverse := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 3, StackViewCrafting, 8, core.FurnaceRef{}),
	})
	if len(reverse.Rejected) != 0 {
		t.Fatalf("扩展格来源被拒绝: %+v", reverse.Rejected)
	}
	player = engine.sessions[session].player
	if player.crafting.Slots[8] != (core.ItemStack{}) {
		t.Fatalf("扩展格 8 = %+v，想要清空", player.crafting.Slots[8])
	}
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 5}) {
		t.Fatalf("背包承接格 = %+v，想要 5 个泥土", player.inventory.Hotbar.Slots[0])
	}
}

// stackQuickRepackFixture 构造「背包仅剩 1 个空格、网格占用合法」的回收
// 边界（与部分移动的夹具同形）：快捷栏 0 是唯一同类合并余量，唯一空格是
// 背包 0，网格格 0/1 的回收依赖这些容量。
func stackQuickRepackFixture(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 10}
	for slot := 1; slot < core.HotbarSlots; slot++ {
		inventory.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemSand, Count: core.MaxStackCount}
	}
	for slot := 1; slot < core.BackpackSlots; slot++ {
		inventory.Backpack[slot] = core.ItemStack{Item: core.ItemSand, Count: core.MaxStackCount}
	}
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 40}
	grid.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 30}
	return stackSplitReadyPlayer(t, inventory, grid)
}

// TestQuickMoveCraftingBackpackToGridPreservesRepack 是回收纪律的对照钉：
// 背包→网格的部分合并（60 并入网格同类格、4 留源）不破坏回收不变量——
// 同物品的网格增量与背包减量守恒，网格满堆沙仍可装回背包的同类余量，
// 必须成功；回收预演不得过度拒绝。
func TestQuickMoveCraftingBackpackToGridPreservesRepack(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 10}
	for slot := 1; slot < core.HotbarSlots; slot++ {
		inventory.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount}
	}
	inventory.Backpack[0] = core.ItemStack{}
	inventory.Backpack[1] = core.ItemStack{Item: core.ItemSand, Count: core.MaxStackCount}
	inventory.Backpack[2] = core.ItemStack{Item: core.ItemSand, Count: 60}
	for slot := 3; slot < core.BackpackSlots; slot++ {
		inventory.Backpack[slot] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount}
	}
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 40}
	grid.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: 30}
	grid.Slots[2] = core.ItemStack{Item: core.ItemSand, Count: 4}
	engine, session := stackSplitReadyPlayer(t, inventory, grid)

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewCrafting, 19, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("不破坏回收不变量的部分合并被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if player.crafting.Slots[2] != (core.ItemStack{Item: core.ItemSand, Count: core.MaxStackCount}) {
		t.Fatalf("网格同类格 = %+v，想要并到 64", player.crafting.Slots[2])
	}
	if player.inventory.Backpack[1] != (core.ItemStack{Item: core.ItemSand, Count: 4}) {
		t.Fatalf("来源格 = %+v，想要余 4", player.inventory.Backpack[1])
	}
}

// TestQuickMoveCraftingGridSourceAllowsFittingMove 是边界精确性对照：同一
// 布局下网格石头并入快捷栏同类不破坏回收不变量，必须成功。
func TestQuickMoveCraftingGridSourceAllowsFittingMove(t *testing.T) {
	engine, session := stackQuickRepackFixture(t)

	result := applyPlayerCommandsTick(engine, []Command{
		quickMoveCommand(session, 2, StackViewCrafting, 0, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("不破坏回收不变量的移动被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 50}) {
		t.Fatalf("同类合并格 = %+v，想要 50", player.inventory.Hotbar.Slots[0])
	}
	if player.crafting.Slots[0] != (core.ItemStack{}) {
		t.Fatalf("网格来源格 = %+v，想要清空", player.crafting.Slots[0])
	}
}

// TestQuickMoveCraftingValueDomainRejects 锁定合成域值域：统一视图上界 45、
// 个人网格扩展格来源、空源与合成视图携带容器引用逐条拒绝。
func TestQuickMoveCraftingValueDomainRejects(t *testing.T) {
	ref := core.FurnaceRef{
		Dimension: core.Overworld, Kind: core.ContainerKindFurnace, Slot: 0, Generation: 1,
	}
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	cases := []struct {
		name    string
		command Command
		reason  RejectReason
	}{
		{"统一视图上界", quickMoveCommand(1, 2, StackViewCrafting, craftingViewSlots, core.FurnaceRef{}), RejectInvalidSlot},
		{"个人网格扩展格来源", quickMoveCommand(1, 2, StackViewCrafting, 4, core.FurnaceRef{}), RejectInvalidSlot},
		{"空源背包格", quickMoveCommand(1, 2, StackViewCrafting, 11, core.FurnaceRef{}), RejectInvalidInput},
		{"合成视图携带容器引用", quickMoveCommand(1, 2, StackViewCrafting, 9, ref), RejectInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{Size: CraftingGridSizePersonal})
			tc.command.Session = session
			result := applyPlayerCommandsTick(engine, []Command{tc.command})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != tc.reason {
				t.Fatalf("result=%+v，想要恰好一次 %d", result, tc.reason)
			}
		})
	}
}

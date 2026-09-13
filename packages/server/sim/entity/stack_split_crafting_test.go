package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestStackSplitCraftingHalfFromBackpackIntoEmptyGrid 验收合成域半组推导：
// 背包格（统一视图 9..44）7 个来源移出恰好 4 个进空网格格，来源剩 3；
// 成功后背包与网格状态都发布。
func TestStackSplitCraftingHalfFromBackpackIntoEmptyGrid(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	engine, session := stackSplitReadyPlayer(t, inventory, CraftingGrid{Size: CraftingGridSizePersonal})

	result := applyPlayerCommandsTick(engine, []Command{
		stackSplitCommand(session, 2, StackViewCrafting, 9, 0, false, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("合成域半组移动被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("来源格 = %+v，想要剩 3", player.inventory.Hotbar.Slots[0])
	}
	if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 4}) {
		t.Fatalf("网格格 = %+v，想要恰好 4 个", player.crafting.Slots[0])
	}
	if len(result.Craftings) != 1 || len(result.Inventories) != 1 {
		t.Fatalf("成功移动应各发布一次网格与物品状态: craftings=%+v inventories=%+v",
			result.Craftings, result.Inventories)
	}
}

// TestStackSplitCraftingSingleIntoNearlyFullGridSlot 验收单件并入同类未满
// 网格格：63/64 目标恰好并入 1 个，来源减 1。
func TestStackSplitCraftingSingleIntoNearlyFullGridSlot(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 5}
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	grid.Slots[1] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount - 1}
	engine, session := stackSplitReadyPlayer(t, inventory, grid)

	result := applyPlayerCommandsTick(engine, []Command{
		stackSplitCommand(session, 2, StackViewCrafting, 9, 1, true, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("单件并入网格被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if player.crafting.Slots[1].Count != core.MaxStackCount {
		t.Fatalf("网格格 = %+v，想要恰好 64", player.crafting.Slots[1])
	}
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 4}) {
		t.Fatalf("来源格 = %+v，想要减 1 剩 4", player.inventory.Hotbar.Slots[0])
	}
}

// TestStackSplitCraftingHalfFromGridIntoBackpack 锁定网格→背包方向：7 个来源
// 的半组把 4 个移进空背包格，来源网格格剩 3。
func TestStackSplitCraftingHalfFromGridIntoBackpack(t *testing.T) {
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	grid.Slots[2] = core.ItemStack{Item: core.ItemSand, Count: 7}
	engine, session := stackSplitReadyPlayer(t, core.Inventory{}, grid)

	result := applyPlayerCommandsTick(engine, []Command{
		stackSplitCommand(session, 2, StackViewCrafting, 2, 9, false, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("网格→背包半组移动被拒绝: %+v", result.Rejected)
	}
	player := engine.sessions[session].player
	if player.crafting.Slots[2] != (core.ItemStack{Item: core.ItemSand, Count: 3}) {
		t.Fatalf("网格来源格 = %+v，想要剩 3", player.crafting.Slots[2])
	}
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemSand, Count: 4}) {
		t.Fatalf("背包目标格 = %+v，想要恰好 4 个", player.inventory.Hotbar.Slots[0])
	}
}

// TestStackSplitCraftingRejectsDifferentItemTarget 验收合成域异类非空目标：
// 半组/单件都整单拒绝且网格与背包逐格不变（网格域沿既有不交换规则）。
func TestStackSplitCraftingRejectsDifferentItemTarget(t *testing.T) {
	for name, single := range map[string]bool{"半组": false, "单件": true} {
		t.Run(name, func(t *testing.T) {
			var inventory core.Inventory
			inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
			grid := CraftingGrid{Size: CraftingGridSizePersonal}
			grid.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 10}
			engine, session := stackSplitReadyPlayer(t, inventory, grid)

			result := applyPlayerCommandsTick(engine, []Command{
				stackSplitCommand(session, 2, StackViewCrafting, 9, 0, single, core.FurnaceRef{}),
			})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("异类网格目标 result=%+v，想要恰好一次 RejectInvalidInput", result)
			}
			player := engine.sessions[session].player
			if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 7}) ||
				player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 10}) {
				t.Fatalf("被拒绝的移动修改了状态: inventory=%+v grid=%+v",
					player.inventory, player.crafting)
			}
		})
	}
}

// stackSplitRepackBoundaryFixture 构造 spec 场景「网格处于合法占用状态且背包
// 仅剩 1 个空格」：背包 36 格中 34 格满沙、快捷栏 0 放 10 个石头（留合并
// 余量），唯一空格是背包 0（统一视图 18）；个人 2×2 网格格 0 放 40 个石头、
// 格 1 放 30 个泥土。初始回收不变量成立：石头并入快捷栏 0、泥土占唯一空格。
type stackSplitRepackFixture struct {
	engine  *Engine
	session SessionID
}

func stackSplitRepackBoundaryFixture(t *testing.T) stackSplitRepackFixture {
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
	engine, session := stackSplitReadyPlayer(t, inventory, grid)
	return stackSplitRepackFixture{engine: engine, session: session}
}

// TestStackSplitCraftingRejectsBrokenRepackInvariant 验收 spec 场景「合成域
// 部分移动保持重打包不变量」：把网格格 0 的半组（20 个石头）移进唯一空背包
// 格会占用网格泥土回收所依赖的空位，回收预演失败时整单拒绝且逐格不变。
func TestStackSplitCraftingRejectsBrokenRepackInvariant(t *testing.T) {
	fixture := stackSplitRepackBoundaryFixture(t)

	result := applyPlayerCommandsTick(fixture.engine, []Command{
		stackSplitCommand(fixture.session, 2, StackViewCrafting, 0, 18, false, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("破坏回收不变量的移动 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	player := fixture.engine.sessions[fixture.session].player
	if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 40}) ||
		player.crafting.Slots[1] != (core.ItemStack{Item: core.ItemDirt, Count: 30}) {
		t.Fatalf("被拒绝的移动修改了网格: %+v", player.crafting)
	}
	if player.inventory.Backpack[0] != (core.ItemStack{}) ||
		player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 10}) {
		t.Fatalf("被拒绝的移动修改了背包: %+v", player.inventory)
	}
}

// TestStackSplitCraftingRepackBoundaryAllowsFittingMove 是边界精确性对照：
// 同一布局下把网格格 0 的单件并入快捷栏 0（同类合并、不占空格）不破坏回收
// 不变量，必须成功——预演不得过度拒绝。
func TestStackSplitCraftingRepackBoundaryAllowsFittingMove(t *testing.T) {
	fixture := stackSplitRepackBoundaryFixture(t)

	result := applyPlayerCommandsTick(fixture.engine, []Command{
		stackSplitCommand(fixture.session, 2, StackViewCrafting, 0, 9, true, core.FurnaceRef{}),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("不破坏回收不变量的单件移动被拒绝: %+v", result.Rejected)
	}
	player := fixture.engine.sessions[fixture.session].player
	if player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 11}) {
		t.Fatalf("目标格 = %+v，想要 11", player.inventory.Hotbar.Slots[0])
	}
	if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 39}) {
		t.Fatalf("网格来源格 = %+v，想要剩 39", player.crafting.Slots[0])
	}
}

// TestStackSplitCraftingValueDomainRejects 锁定合成域值域：统一视图上界 45、
// 个人网格扩展格 4..8、双背包端、同格、空源与合成视图携带容器引用逐条拒绝。
func TestStackSplitCraftingValueDomainRejects(t *testing.T) {
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
		{"统一视图上界", stackSplitCommand(1, 2, StackViewCrafting, 9, craftingViewSlots, false, core.FurnaceRef{}), RejectInvalidSlot},
		{"个人网格扩展格", stackSplitCommand(1, 2, StackViewCrafting, 9, 4, false, core.FurnaceRef{}), RejectInvalidSlot},
		{"双背包端", stackSplitCommand(1, 2, StackViewCrafting, 9, 10, false, core.FurnaceRef{}), RejectInvalidInput},
		{"同格", stackSplitCommand(1, 2, StackViewCrafting, 2, 2, false, core.FurnaceRef{}), RejectInvalidInput},
		{"空源", stackSplitCommand(1, 2, StackViewCrafting, 11, 0, false, core.FurnaceRef{}), RejectInvalidInput},
		{"合成视图携带容器引用", stackSplitCommand(1, 2, StackViewCrafting, 9, 0, false, ref), RejectInvalidInput},
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

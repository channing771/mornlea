package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// stackSplitChestFixture 在玩家脚下放一只已登记槽位的箱子并按需改写玩家
// 物品与网格，返回箱子引用。查看关系由 openStackSplitChest 显式建立，让
// 未查看路径可测。
func stackSplitChestFixture(
	t *testing.T,
	inventory core.Inventory,
	chest world.ChestSlot,
	grid CraftingGrid,
) (*Engine, SessionID, core.ContainerRef) {
	t.Helper()
	engine, session := stackSplitReadyPlayer(t, inventory, grid)
	index, indexed := world.ChunkBlockIndex(core.BlockPos{})
	if !indexed {
		t.Fatal("箱子方块没有区块索引")
	}
	engine.SetBlockForTest(core.BlockPos{}, core.ChestID)
	chest.BlockIndex = index
	chest.Active = true
	if chest.Generation == 0 {
		chest.Generation = 1
	}
	engine.SetChunkChestForTest(core.ChunkKey{Dimension: core.Overworld}, 0, chest)
	ref := core.ContainerRef{
		Dimension: core.Overworld, Chunk: core.ChunkPos{},
		Kind: core.ContainerKindChest, Slot: 0, Generation: chest.Generation,
	}
	return engine, session, ref
}

// openStackSplitChest 用权威射线命令建立箱子查看关系。
func openStackSplitChest(t *testing.T, engine *Engine, session SessionID) {
	t.Helper()
	opened := applyPlayerCommandsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandOpenFurnace, Pitch: fluidLookDown,
	}})
	if len(opened.Rejected) != 0 || len(opened.Chests) != 1 {
		t.Fatalf("打开箱子失败: rejected=%+v chests=%+v", opened.Rejected, opened.Chests)
	}
}

// stackSplitChestAt 读取箱子当前权威值。
func stackSplitChestAt(t *testing.T, engine *Engine, ref core.ContainerRef) world.ChestSlot {
	t.Helper()
	_, chest, ok := engine.chestView(ref)
	if !ok {
		t.Fatal("箱子引用失效")
	}
	return chest
}

// TestStackSplitChestHalfIntoChestRegion 验收箱子视图半组推导：背包格 7 个
// 来源移出恰好 4 个进箱子格（统一视图 36），来源剩 3。
func TestStackSplitChestHalfIntoChestRegion(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	engine, session, ref := stackSplitChestFixture(t, inventory, world.ChestSlot{}, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestFirstSlot, false, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("箱子域半组移动被拒绝: %+v", result.Rejected)
	}
	chest := stackSplitChestAt(t, engine, ref)
	if chest.Items[0] != (core.ItemStack{Item: core.ItemStone, Count: 4}) {
		t.Fatalf("箱子格 = %+v，想要恰好 4 个", chest.Items[0])
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("来源格 = %+v，想要剩 3", got)
	}
}

// TestStackSplitChestSingleIntoNearlyFullChestSlot 验收单件并入 63/64 的箱子
// 同类格：恰好并入 1 个。
func TestStackSplitChestSingleIntoNearlyFullChestSlot(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 5}
	chest := world.ChestSlot{}
	chest.Items[2] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount - 1}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestFirstSlot+2, true, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("单件并入箱子被拒绝: %+v", result.Rejected)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[2]; got.Count != core.MaxStackCount {
		t.Fatalf("箱子格 = %+v，想要恰好 64", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemDirt, Count: 4}) {
		t.Fatalf("来源格 = %+v，想要减 1 剩 4", got)
	}
}

// TestStackSplitChestRegionRoutingPreserved 锁定区域路由逐字复用：容器视图内
// 背包区内部（0→20）走 Inventory.MoveStackAmount，箱子区内部（40→45）走
// 统一栏位合并，两条路径的半组语义一致。
func TestStackSplitChestRegionRoutingPreserved(t *testing.T) {
	t.Run("背包区内部", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
		engine, session, ref := stackSplitChestFixture(t, inventory, world.ChestSlot{}, CraftingGrid{})
		openStackSplitChest(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			stackSplitCommand(session, 3, StackViewContainer, 0, 20, false, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("背包区内部半组移动被拒绝: %+v", result.Rejected)
		}
		got := engine.sessions[session].player.inventory
		if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
			t.Fatalf("来源格 = %+v，想要剩 3", got.Hotbar.Slots[0])
		}
		if got.Backpack[11] != (core.ItemStack{Item: core.ItemStone, Count: 4}) {
			t.Fatalf("目标格 = %+v，想要 4 个", got.Backpack[11])
		}
	})
	t.Run("箱子区内部", func(t *testing.T) {
		chest := world.ChestSlot{}
		chest.Items[4] = core.ItemStack{Item: core.ItemDirt, Count: 7}
		engine, session, ref := stackSplitChestFixture(t, core.Inventory{}, chest, CraftingGrid{})
		openStackSplitChest(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			stackSplitCommand(session, 3, StackViewContainer, 40, 45, false, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("箱子区内部半组移动被拒绝: %+v", result.Rejected)
		}
		got := stackSplitChestAt(t, engine, ref)
		if got.Items[4] != (core.ItemStack{Item: core.ItemDirt, Count: 3}) {
			t.Fatalf("来源格 = %+v，想要剩 3", got.Items[4])
		}
		if got.Items[9] != (core.ItemStack{Item: core.ItemDirt, Count: 4}) {
			t.Fatalf("目标格 = %+v，想要 4 个", got.Items[9])
		}
	})
}

// TestStackSplitChestPartialOutOfChestKeepsRemainder 锁定箱子→背包方向的
// 半组：30 个来源移出 15 个并入同类部分格，箱子留 15。
func TestStackSplitChestPartialOutOfChestKeepsRemainder(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 10}
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 30}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, core.ChestFirstSlot, 0, false, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("箱子→背包半组移动被拒绝: %+v", result.Rejected)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 15}) {
		t.Fatalf("箱子来源格 = %+v，想要剩 15", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 25}) {
		t.Fatalf("目标格 = %+v，想要 25", got)
	}
}

// TestStackSplitChestRejectsDifferentItemTargetWithoutSwap 锁定容器域异类
// 非空目标整单拒绝且不交换：整堆移动对异类目标交换是既有语义，部分移动
// MUST 拒绝，两侧逐格不变。
func TestStackSplitChestRejectsDifferentItemTargetWithoutSwap(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemDirt, Count: 10}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestFirstSlot, false, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("异类箱子目标 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{Item: core.ItemDirt, Count: 10}) {
		t.Fatalf("被拒绝的移动修改了箱子: %+v", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 7}) {
		t.Fatalf("被拒绝的移动修改了玩家物品: %+v", got)
	}
}

// TestStackSplitChestSettlesInDeferredChunkPhase 锁定结算相位：容器域部分
// 移动在命令阶段只入队不结算（与整堆跨容器移动共享延迟相位），权威状态在
// 区块写相位才变化。
func TestStackSplitChestSettlesInDeferredChunkPhase(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	engine, session, ref := stackSplitChestFixture(t, inventory, world.ChestSlot{}, CraftingGrid{})
	openStackSplitChest(t, engine, session)
	command := stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestFirstSlot, false, ref)

	// 只运行命令阶段：延迟入队的移动既不结算也不产生拒绝，状态零变化。
	preliminary := applyPlayerCommandsTick(engine, []Command{command})
	if len(preliminary.Rejected) != 0 {
		t.Fatalf("命令阶段不应拒绝容器域移动: %+v", preliminary.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got.Count != 7 {
		t.Fatalf("命令阶段提前结算: %+v", got)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{}) {
		t.Fatalf("命令阶段提前写箱子: %+v", got)
	}

	// 同一命令经完整相位链（命令收集 + FinishWorld 区块写相位）结算。
	settled := finishPlayerWorldTick(engine, []Command{command})
	if len(settled.Rejected) != 0 {
		t.Fatalf("区块写相位结算被拒绝: %+v", settled.Rejected)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 4}) {
		t.Fatalf("箱子格 = %+v，想要 4 个", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("来源格 = %+v，想要剩 3", got)
	}
}

// stackSplitContainerRepackInventory 构造「背包仅剩 1 个空格、网格占用合法」
// 的重打包边界：快捷栏 0 放 10 个石头提供同类合并余量，其余 34 格满沙，
// 唯一空格是背包 0（容器视图/统一视图 18）。
func stackSplitContainerRepackInventory() core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 10}
	for slot := 1; slot < core.HotbarSlots; slot++ {
		inventory.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemSand, Count: core.MaxStackCount}
	}
	for slot := 1; slot < core.BackpackSlots; slot++ {
		inventory.Backpack[slot] = core.ItemStack{Item: core.ItemSand, Count: core.MaxStackCount}
	}
	return inventory
}

// stackSplitContainerRepackGrid 是占用唯一空格回收预算的网格：泥土 30 需要
// 背包 0 那个空格才能整体装回。
func stackSplitContainerRepackGrid() CraftingGrid {
	grid := CraftingGrid{Size: CraftingGridSizePersonal}
	grid.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 30}
	return grid
}

// TestStackSplitChestRejectsBrokenRepackInvariant 锁定箱子→背包增量的回收
// 预演：半组移动若占用网格回收依赖的唯一空格，整体拒绝且两侧逐格不变。
func TestStackSplitChestRejectsBrokenRepackInvariant(t *testing.T) {
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 30}
	engine, session, ref := stackSplitChestFixture(
		t, stackSplitContainerRepackInventory(), chest, stackSplitContainerRepackGrid(),
	)
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, core.ChestFirstSlot, 18, false, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("破坏回收不变量的移动 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 30}) {
		t.Fatalf("被拒绝的移动修改了箱子: %+v", got)
	}
	player := engine.sessions[session].player
	if player.inventory.Backpack[0] != (core.ItemStack{}) ||
		player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 10}) {
		t.Fatalf("被拒绝的移动修改了背包: %+v", player.inventory)
	}
	if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 30}) {
		t.Fatalf("被拒绝的移动修改了网格: %+v", player.crafting)
	}
}

// TestStackSplitChestRejectsWhenNotViewing 锁定查看关系复用：未建立查看关系
// 或引用与查看中的容器不一致时整单拒绝。
func TestStackSplitChestRejectsWhenNotViewing(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	engine, session, ref := stackSplitChestFixture(t, inventory, world.ChestSlot{}, CraftingGrid{})

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestFirstSlot, false, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("未查看容器 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{}) {
		t.Fatalf("被拒绝的移动修改了箱子: %+v", got)
	}

	openStackSplitChest(t, engine, session)
	stale := ref
	stale.Generation = ref.Generation + 1
	result = finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 4, StackViewContainer, 0, core.ChestFirstSlot, false, stale),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("过期引用 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
}

// TestStackSplitChestValueDomainRejects 锁定箱子域值域：统一视图上界 63 与
// 空源按 RejectInvalidInput 拒绝且零改动。
func TestStackSplitChestValueDomainRejects(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	engine, session, ref := stackSplitChestFixture(t, inventory, world.ChestSlot{}, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	for name, command := range map[string]Command{
		"目标越界": stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestViewSlots, false, ref),
		"空源":   stackSplitCommand(session, 3, StackViewContainer, 5, core.ChestFirstSlot, false, ref),
	} {
		t.Run(name, func(t *testing.T) {
			result := finishPlayerWorldTick(engine, []Command{command})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("result=%+v，想要恰好一次 RejectInvalidInput", result)
			}
			if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{}) {
				t.Fatalf("被拒绝的移动修改了箱子: %+v", got)
			}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 7}) {
				t.Fatalf("被拒绝的移动修改了玩家物品: %+v", got)
			}
		})
	}
}

// TestStackSplitContainerDeterministicReplay 验收确定性：相同布局与命令序列
// 在两个独立引擎上逐格一致（半组进箱、单件并入同类、半组回背包）。
func TestStackSplitContainerDeterministicReplay(t *testing.T) {
	run := func() (core.Inventory, world.ChestSlot) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
		chest := world.ChestSlot{}
		chest.Items[2] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount - 1}
		engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
		openStackSplitChest(t, engine, session)
		commands := []Command{
			stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestFirstSlot, false, ref),
			stackSplitCommand(session, 4, StackViewContainer, 0, core.ChestFirstSlot+2, true, ref),
			stackSplitCommand(session, 5, StackViewContainer, core.ChestFirstSlot, 20, false, ref),
		}
		for index, command := range commands {
			result := finishPlayerWorldTick(engine, []Command{command})
			if len(result.Rejected) != 0 {
				t.Fatalf("第 %d 步被拒绝: %+v", index, result.Rejected)
			}
		}
		return engine.sessions[session].player.inventory, stackSplitChestAt(t, engine, ref)
	}
	firstInventory, firstChest := run()
	secondInventory, secondChest := run()
	if firstInventory != secondInventory || firstChest != secondChest {
		t.Fatalf("重放不一致: inventory=%+v/%+v chest=%+v/%+v",
			firstInventory, secondInventory, firstChest, secondChest)
	}
	if firstChest.Items[2].Count != core.MaxStackCount {
		t.Fatalf("夹具失效：单件未并满箱子格: %+v", firstChest.Items[2])
	}
	if firstChest.Items[0] != (core.ItemStack{Item: core.ItemStone, Count: 2}) {
		t.Fatalf("夹具失效：半组回背包后箱子格应剩 2: %+v", firstChest.Items[0])
	}
	if firstInventory.Backpack[11] != (core.ItemStack{Item: core.ItemStone, Count: 2}) {
		t.Fatalf("夹具失效：回背包目标格应得 2: %+v", firstInventory.Backpack[11])
	}
}

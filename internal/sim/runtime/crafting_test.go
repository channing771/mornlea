package runtime_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// crafting_test.go：合成网格在 sim 层的语义——网格移动、产物取出、回收不变量
// 与工作台生命周期。真实合成只经格子工作台路径发生。

// —— 网格移动与产物取出的测试助手 ——

// craftingMoveCommand 构造一条网格移动命令。from/to 是统一视图格：
// 网格 0..8、背包 9..44（背包统一格 = 既有物品栏索引 + 9）。
func craftingMoveCommand(
	session runtime.SessionID,
	sequence uint64,
	from, to uint8,
) runtime.Command {
	return runtime.Command{
		Session:  session,
		Sequence: sequence,
		Kind:     runtime.CommandMoveCraftingStack,
		Slot:     from,
		ToSlot:   to,
	}
}

// craftingTakeCommand 构造一条产物取出命令。
func craftingTakeCommand(session runtime.SessionID, sequence uint64) runtime.Command {
	return runtime.Command{
		Session:  session,
		Sequence: sequence,
		Kind:     runtime.CommandTakeCraftingOutput,
	}
}

// currentCraftingGrid 读取引擎当前的权威合成网格与派生产物。
func currentCraftingGrid(
	t *testing.T,
	engine *runtime.Engine,
	session runtime.SessionID,
) (runtime.CraftingGrid, core.ItemStack) {
	t.Helper()
	grid, output, ok := engine.PlayerCrafting(session)
	if !ok {
		t.Fatalf("会话 %d 没有权威合成网格", session)
	}
	return grid, output
}

// personalGrid 断言当前网格是默认的个人 2×2 并返回其内容。
func personalGrid(
	t *testing.T,
	engine *runtime.Engine,
	session runtime.SessionID,
) runtime.CraftingGrid {
	t.Helper()
	grid, _ := currentCraftingGrid(t, engine, session)
	if grid.Size != 2 {
		t.Fatalf("网格尺寸 = %d，想要个人网格 2", grid.Size)
	}
	return grid
}

// TestCraftingMoveInventoryIntoEmptyGridSlot 覆盖 spec 场景「背包到网格的整堆移动」：
// 目标网格格为空时整堆移入、来源格清空，并在同一 tick 发布网格与物品状态。
func TestCraftingMoveInventoryIntoEmptyGridSlot(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	engine.Enqueue(craftingMoveCommand(session, 2, 9, 0))
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("合法移动被拒绝: %+v", result.Rejected)
	}
	grid := personalGrid(t, engine, session)
	if grid.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("网格格 0 = %+v，想要 3 个石头", grid.Slots[0])
	}
	if got := currentInventory(t, engine, session); got.Hotbar.Slots[0] != (core.ItemStack{}) {
		t.Fatalf("来源背包格未清空: %+v", got.Hotbar.Slots[0])
	}
	if len(result.Inventories) != 1 || len(result.Craftings) != 1 {
		t.Fatalf("成功移动应各发布一次物品与网格状态: Inv=%d Grid=%d",
			len(result.Inventories), len(result.Craftings))
	}
}

// TestCraftingMoveMergesSameItemUpToStackLimit 覆盖 spec 场景「同物品按栈上限合并」：
// 网格 60 木板 + 背包 10 木板 → 网格 64、背包剩 6。
func TestCraftingMoveMergesSameItemUpToStackLimit(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 60}
	stocked.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemOakPlanks, Count: 10}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	engine.Enqueue(craftingMoveCommand(session, 2, 9, 0))
	engine.Enqueue(craftingMoveCommand(session, 3, 10, 0))
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("合法合并被拒绝: %+v", result.Rejected)
	}
	grid := personalGrid(t, engine, session)
	if grid.Slots[0] != (core.ItemStack{Item: core.ItemOakPlanks, Count: core.MaxStackCount}) {
		t.Fatalf("合并后网格格 0 = %+v，想要 64 个木板", grid.Slots[0])
	}
	if got := currentInventory(t, engine, session).Hotbar; got.Slots[1].Count != 6 {
		t.Fatalf("合并后来源格 = %+v，想要剩余 6 个", got.Slots[1])
	}
}

// TestCraftingMoveRejectsDifferentItemTarget 覆盖 spec 场景「不同物品不合并」：
// 网格目标格已有石头时移入木板 MUST 拒绝且两侧逐格不变（不做交换）。
func TestCraftingMoveRejectsDifferentItemTarget(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
	stocked.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemOakPlanks, Count: 4}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	engine.Enqueue(craftingMoveCommand(session, 2, 9, 1))
	engine.Step()
	engine.Enqueue(craftingMoveCommand(session, 3, 10, 1))
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
		t.Fatalf("异类移动 result=%+v，想要恰一条 invalid_input 拒绝", result)
	}
	grid := personalGrid(t, engine, session)
	if grid.Slots[1] != (core.ItemStack{Item: core.ItemStone, Count: 2}) {
		t.Fatalf("被拒绝的移动改写了网格格 1: %+v", grid.Slots[1])
	}
	got := currentInventory(t, engine, session)
	if got.Hotbar.Slots[1] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 4}) {
		t.Fatalf("被拒绝的移动改写了背包格: %+v", got.Hotbar.Slots[1])
	}
	if len(result.Inventories) != 0 || len(result.Craftings) != 0 {
		t.Fatalf("被拒绝的移动仍发布状态: Inv=%d Grid=%d",
			len(result.Inventories), len(result.Craftings))
	}
}

// TestCraftingMoveRejectsEmptySourceAndSameSlot 覆盖「空源或同格 MUST 拒绝」。
func TestCraftingMoveRejectsEmptySourceAndSameSlot(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	cases := []struct {
		name     string
		from, to uint8
	}{
		{"空源", 10, 0},
		{"同格", 9, 9},
	}
	for index, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// 每条命令的序号必须严格递增：上一条被拒的命令也已推进 lastSequence。
			engine.Enqueue(craftingMoveCommand(
				session, uint64(2+index), testCase.from, testCase.to,
			))
			result := engine.Step()
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
				t.Fatalf("移动 result=%+v，想要 invalid_input 拒绝", result)
			}
			if len(result.Inventories) != 0 || len(result.Craftings) != 0 {
				t.Fatalf("被拒绝的移动仍发布状态: %+v", result)
			}
		})
	}
	if got := currentInventory(t, engine, session); got != stocked {
		t.Fatalf("被拒绝的移动修改了物品状态: %+v", got)
	}
}

// TestCraftingMoveRejectsBothEndsInInventory 覆盖 spec 场景「两端都在背包区时拒绝」：
// 背包之间的移动只能走既有 `CommandMoveInventoryStack`。
func TestCraftingMoveRejectsBothEndsInInventory(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	engine.Enqueue(craftingMoveCommand(session, 2, 9, 10))
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
		t.Fatalf("双背包端 result=%+v，想要 invalid_input 拒绝", result)
	}
	if got := currentInventory(t, engine, session); got != stocked {
		t.Fatalf("被拒绝的移动修改了物品状态: %+v", got)
	}
}

// TestCraftingMoveRejectsPersonalExtendedSlots 覆盖 spec 场景
// 「个人网格拒绝扩展格」：有效尺寸 2 时格 4..8 无论作为源还是目标都拒绝。
func TestCraftingMoveRejectsPersonalExtendedSlots(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	for slot := uint8(4); slot <= 8; slot++ {
		engine.Enqueue(craftingMoveCommand(session, uint64(2+2*slot), 9, slot))
		engine.Enqueue(craftingMoveCommand(session, uint64(3+2*slot), slot, 0))
	}
	result := engine.Step()
	if len(result.Rejected) != 10 {
		t.Fatalf("扩展格移动 result=%+v，想要 10 条全部拒绝", result)
	}
	for _, rejection := range result.Rejected {
		if rejection.Reason != runtime.RejectInvalidSlot {
			t.Fatalf("扩展格拒绝原因 = %d，想要 invalid_slot", rejection.Reason)
		}
	}
	if got := currentInventory(t, engine, session); got != stocked {
		t.Fatalf("被拒绝的扩展格移动修改了物品状态: %+v", got)
	}
}

// TestCraftingMoveRejectsOutOfRangeSlots 覆盖统一视图值域：任一端 ≥45 拒绝。
func TestCraftingMoveRejectsOutOfRangeSlots(t *testing.T) {
	engine, session := readyFlatPlayer(t)

	engine.Enqueue(craftingMoveCommand(session, 2, 45, 0))
	engine.Enqueue(craftingMoveCommand(session, 3, 0, 45))
	result := engine.Step()
	if len(result.Rejected) != 2 {
		t.Fatalf("越界移动 result=%+v，想要 2 条拒绝", result)
	}
	for _, rejection := range result.Rejected {
		if rejection.Reason != runtime.RejectInvalidSlot {
			t.Fatalf("越界拒绝原因 = %d，想要 invalid_slot", rejection.Reason)
		}
	}
}

// TestCraftingMoveBetweenGridSlots 覆盖 grid↔grid 移动：同类合并与整堆迁移
// 在网格内部同样成立。
func TestCraftingMoveBetweenGridSlots(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 30}
	stocked.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemOakPlanks, Count: 30}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	engine.Enqueue(craftingMoveCommand(session, 2, 9, 0))
	engine.Enqueue(craftingMoveCommand(session, 3, 10, 1))
	engine.Step()
	engine.Enqueue(craftingMoveCommand(session, 4, 0, 1))
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("网格内合并被拒绝: %+v", result.Rejected)
	}
	grid := personalGrid(t, engine, session)
	if grid.Slots[1] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 60}) ||
		grid.Slots[0] != (core.ItemStack{}) {
		t.Fatalf("网格内合并结果 = %+v", grid.Slots)
	}

	engine.Enqueue(craftingMoveCommand(session, 5, 1, 2))
	result = engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("网格内整堆迁移被拒绝: %+v", result.Rejected)
	}
	grid = personalGrid(t, engine, session)
	if grid.Slots[2] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 60}) ||
		grid.Slots[1] != (core.ItemStack{}) {
		t.Fatalf("网格内整堆迁移结果 = %+v", grid.Slots)
	}
}

// TestCraftingTakeOutputConsumesMatchOnceIntoInventory 覆盖 spec 场景
// 「取出消费一次并产出入背包」：对匹配形状的每个非空格恰减 1，
// 完整产物经稳定插入入背包，产物格按剩余网格重新派生。
func TestCraftingTakeOutputConsumesMatchOnceIntoInventory(t *testing.T) {
	var stocked core.Inventory
	for slot := range 4 {
		stocked.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemStone, Count: 8}
	}
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	for slot := range 4 {
		engine.Enqueue(craftingMoveCommand(session, uint64(2+slot), uint8(9+slot), uint8(slot)))
	}
	engine.Step()
	grid, output := currentCraftingGrid(t, engine, session)
	if want := (core.ItemStack{Item: core.ItemStoneBrick, Count: 4}); output != want {
		t.Fatalf("派生产物 = %+v，想要 %+v", output, want)
	}

	engine.Enqueue(craftingTakeCommand(session, 10))
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("合法取出被拒绝: %+v", result.Rejected)
	}
	got := currentInventory(t, engine, session)
	// 四格石头整堆移出后快捷栏 0..3 已空，产物按稳定插入落在首个空格（快捷栏 0）。
	if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStoneBrick, Count: 4}) {
		t.Fatalf("产物未入背包: %+v", got.Hotbar.Slots[0])
	}
	grid, output = currentCraftingGrid(t, engine, session)
	for slot := range 4 {
		if grid.Slots[slot] != (core.ItemStack{Item: core.ItemStone, Count: 7}) {
			t.Fatalf("格 %d = %+v，想要恰减 1 剩 7", slot, grid.Slots[slot])
		}
	}
	if want := (core.ItemStack{Item: core.ItemStoneBrick, Count: 4}); output != want {
		t.Fatalf("取出后派生产物 = %+v，剩余网格应继续匹配同一条配方", output)
	}
	if len(result.Inventories) != 1 || len(result.Craftings) != 1 {
		t.Fatalf("取出应各发布一次物品与网格状态: Inv=%d Grid=%d",
			len(result.Inventories), len(result.Craftings))
	}
}

// TestCraftingTakeOutputRejectedWithoutMatch 覆盖「产物只由当前匹配派生」：
// 空网格没有任何产物，取出 MUST 拒绝且不发布任何状态。
func TestCraftingTakeOutputRejectedWithoutMatch(t *testing.T) {
	engine, session := readyFlatPlayer(t)

	engine.Enqueue(craftingTakeCommand(session, 2))
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
		t.Fatalf("无匹配取出 result=%+v，想要 invalid_input 拒绝", result)
	}
	if _, output := currentCraftingGrid(t, engine, session); output != (core.ItemStack{}) {
		t.Fatalf("空网格派生了产物: %+v", output)
	}
	if len(result.Inventories) != 0 || len(result.Craftings) != 0 {
		t.Fatalf("被拒绝的取出仍发布状态: %+v", result)
	}
}

// TestCraftingTakeOutputCapacityRehearsal 覆盖 spec 场景「背包容量不足时拒绝取出」：
// 取出前必须预演「完整产物 + 消费后剩余网格」都能装回背包。构造上刻意用
// 原木 1×1 → 4 木板：少一个空格时产物放得下、剩余原木放不下，恰好区分
// 「只预演产物」与「预演产物 + 剩余网格」两种实现。
func TestCraftingTakeOutputCapacityRehearsal(t *testing.T) {
	t.Run("恰可放", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakLog, Count: 2}
		for slot := uint8(1); slot < core.HotbarSlots; slot++ {
			inventory.Hotbar.Slots[slot] = core.ItemStack{
				Item: core.ItemStone, Count: core.MaxStackCount,
			}
		}
		for slot := uint8(0); slot < core.BackpackSlots-1; slot++ {
			inventory.Backpack[slot] = core.ItemStack{
				Item: core.ItemStone, Count: core.MaxStackCount,
			}
		}
		// 留一个空格：加上移动清空的来源格，取出后产物与剩余原木各得其所。
		engine, session := readyFlatPlayerWithInventory(t, inventory)
		engine.Enqueue(craftingMoveCommand(session, 2, 9, 0))
		engine.Step()

		engine.Enqueue(craftingTakeCommand(session, 3))
		result := engine.Step()
		if len(result.Rejected) != 0 {
			t.Fatalf("恰可放的取出被拒绝: %+v", result.Rejected)
		}
		grid := personalGrid(t, engine, session)
		if grid.Slots[0] != (core.ItemStack{Item: core.ItemOakLog, Count: 1}) {
			t.Fatalf("消费后网格 = %+v，想要恰减 1", grid.Slots[0])
		}
		got := currentInventory(t, engine, session)
		if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 4}) {
			t.Fatalf("产物 = %+v，想要 4 个木板进最低空格", got.Hotbar.Slots[0])
		}
	})

	t.Run("少一格不可放", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakLog, Count: 2}
		for slot := uint8(1); slot < core.HotbarSlots; slot++ {
			inventory.Hotbar.Slots[slot] = core.ItemStack{
				Item: core.ItemStone, Count: core.MaxStackCount,
			}
		}
		for slot := uint8(0); slot < core.BackpackSlots; slot++ {
			inventory.Backpack[slot] = core.ItemStack{
				Item: core.ItemStone, Count: core.MaxStackCount,
			}
		}
		// 不留任何空格：整堆移出后来源格成为唯一空格，只够产物或剩余原木之一。
		engine, session := readyFlatPlayerWithInventory(t, inventory)
		engine.Enqueue(craftingMoveCommand(session, 2, 9, 0))
		engine.Step()
		before := currentInventory(t, engine, session)
		gridBefore, _ := currentCraftingGrid(t, engine, session)

		engine.Enqueue(craftingTakeCommand(session, 3))
		result := engine.Step()
		if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
			t.Fatalf("少一格取出 result=%+v，想要 invalid_input 拒绝", result)
		}
		if got := currentInventory(t, engine, session); got != before {
			t.Fatalf("被拒绝的取出修改了物品状态: %+v", got)
		}
		if grid, _ := currentCraftingGrid(t, engine, session); grid != gridBefore {
			t.Fatalf("被拒绝的取出修改了网格: %+v", grid)
		}
		if len(result.Inventories) != 0 || len(result.Craftings) != 0 {
			t.Fatalf("被拒绝的取出仍发布状态: %+v", result)
		}
	})
}

// TestCraftingDurabilityItemsNeverConsume 覆盖 spec 场景「耐久物品不作为材料」：
// 工具可以放进网格，但所在格被配方形状覆盖时匹配 MUST 失败，取出不消耗它，
// 移回背包时耐久原样保留。
func TestCraftingDurabilityItemsNeverConsume(t *testing.T) {
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	pickaxe := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full}
	var stocked core.Inventory
	for slot := range 3 {
		stocked.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemStone, Count: 2}
	}
	stocked.Hotbar.Slots[3] = pickaxe
	engine, session := readyFlatPlayerWithInventory(t, stocked)

	engine.Enqueue(craftingMoveCommand(session, 2, 9, 0))
	engine.Enqueue(craftingMoveCommand(session, 3, 10, 1))
	engine.Enqueue(craftingMoveCommand(session, 4, 11, 2))
	engine.Enqueue(craftingMoveCommand(session, 5, 12, 3))
	engine.Step()

	if _, output := currentCraftingGrid(t, engine, session); output != (core.ItemStack{}) {
		t.Fatalf("含工具的网格派生了产物: %+v", output)
	}
	engine.Enqueue(craftingTakeCommand(session, 6))
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
		t.Fatalf("含工具取出 result=%+v，想要 invalid_input 拒绝", result)
	}
	grid := personalGrid(t, engine, session)
	if grid.Slots[3] != pickaxe {
		t.Fatalf("取出尝试消耗了工具: %+v", grid.Slots[3])
	}

	// 工具整堆移回背包，耐久字段必须原样保留。
	engine.Enqueue(craftingMoveCommand(session, 7, 3, 10))
	engine.Step()
	if got := currentInventory(t, engine, session).Hotbar; got.Slots[1] != pickaxe {
		t.Fatalf("移回背包的工具 = %+v，想要原样 %+v", got.Slots[1], pickaxe)
	}
}

// —— 任务组 2.2：回收不变量覆盖全部背包增量入口 ——

// repackTestDrop 在玩家脚下放置一个可立即拾取的掉落物（拾取延迟为零）。
func repackTestDrop(
	t *testing.T,
	engine *runtime.Engine,
	stack core.ItemStack,
) {
	t.Helper()
	engine.SetChunkDropForTest(core.ChunkKey{Dimension: core.Overworld}, 0, world.DropSlot{
		Generation: 1,
		Active:     true,
		Stack:      stack,
		BlockIndex: dropTargetIndex(t),
	})
}

// repackOracle 在测试侧独立复算回收不变量：与权威实现相同地按格序把网格
// 物品经 `core.Inventory.AddStack` 稳定装入背包副本。它是性质测试的判定
// oracle，故意不复用被测代码。
func repackOracle(inventory core.Inventory, grid runtime.CraftingGrid) bool {
	for slot := uint8(0); slot < core.CraftingGridSlots; slot++ {
		stack := grid.Slots[slot]
		if stack.Item == core.ItemNone {
			continue
		}
		var leftover core.ItemStack
		inventory, leftover = inventory.AddStack(stack)
		if leftover.Count != 0 {
			return false
		}
	}
	return true
}

// repackMarginalSetup 构造「36 格接近满载 + 网格占一格」的边界状态：
// 快捷栏 0 是唯一同类可并栈的 63 石头，快捷栏 1 因整堆移出成为唯一空格，
// 网格格 0 压着 64 个石头——它回收时只能靠那个空格（可并栈空间恰为 1）。
// 返回引擎、会话与递增的命令序号指针。
func repackMarginalSetup(t *testing.T) (*runtime.Engine, runtime.SessionID, *uint64) {
	t.Helper()
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 63}
	for slot := uint8(1); slot < core.HotbarSlots; slot++ {
		inventory.Hotbar.Slots[slot] = core.ItemStack{
			Item: core.ItemStone, Count: core.MaxStackCount,
		}
	}
	for slot := uint8(0); slot < core.BackpackSlots; slot++ {
		inventory.Backpack[slot] = core.ItemStack{
			Item: core.ItemStone, Count: core.MaxStackCount,
		}
	}
	engine, session := readyFlatPlayerWithInventory(t, inventory)
	sequence := uint64(2)
	engine.Enqueue(craftingMoveCommand(session, sequence, 10, 0))
	engine.Step()
	grid, _ := currentCraftingGrid(t, engine, session)
	if grid.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
		t.Fatalf("边界夹具的网格格 0 = %+v", grid.Slots[0])
	}
	return engine, session, &sequence
}

// TestCraftingRepackPickupHonoursMergeAndEmptyBudgets 覆盖 spec 场景
// 「拾取不得挤占回收所需空间」：网格占用了唯一的空格回收预算时，
// 只有恰好并入同类余量的拾取被放行；会占掉空格或耗尽并栈余量的拾取
// MUST 整堆留在世界且权威物品状态不变。
func TestCraftingRepackPickupHonoursMergeAndEmptyBudgets(t *testing.T) {
	cases := []struct {
		name    string
		drop    core.ItemStack
		accepts bool
	}{
		// 1 个石头恰好并入 63 的同类余量：不消耗空格预算，拾取放行。
		{"同类恰可并栈放行", core.ItemStack{Item: core.ItemStone, Count: 1}, true},
		// 2 个石头只有 1 格余量：第 2 个需要空格，而空格是网格回收所必需。
		{"同类多一个即拒绝", core.ItemStack{Item: core.ItemStone, Count: 2}, false},
		// 异类没有并栈余地，必然占用空格预算。
		{"异类需要空格拒绝", core.ItemStack{Item: core.ItemDirt, Count: 1}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			engine, session, _ := repackMarginalSetup(t)
			repackTestDrop(t, engine, testCase.drop)
			before := currentInventory(t, engine, session)
			gridBefore, _ := currentCraftingGrid(t, engine, session)

			result := engine.Step()
			chunk, _, ok := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})
			if !ok {
				t.Fatal("中心区块不可用")
			}
			drop := chunk.Drop(0)
			if testCase.accepts {
				if drop.Active {
					t.Fatalf("放行的拾取没有取走掉落物: %+v", drop)
				}
				if got := currentInventory(t, engine, session); got == before {
					t.Fatalf("放行的拾取没有修改物品状态: %+v", got)
				}
			} else {
				if !drop.Active || drop.Stack != testCase.drop {
					t.Fatalf("被拒拾取改变了掉落物: %+v，想要原样 %+v", drop.Stack, testCase.drop)
				}
				if got := currentInventory(t, engine, session); got != before {
					t.Fatalf("被拒拾取修改了物品状态: %+v", got)
				}
				if grid, _ := currentCraftingGrid(t, engine, session); grid != gridBefore {
					t.Fatalf("被拒拾取修改了网格: %+v", grid)
				}
				if len(result.Inventories) != 0 {
					t.Fatalf("被拒拾取仍发布物品状态: %+v", result.Inventories)
				}
			}
			// 无论放行与否，tick 结束时回收不变量必须成立。
			if inventory := currentInventory(t, engine, session); !repackOracle(inventory, gridState(t, engine, session)) {
				t.Fatalf("拾取后回收不变量被破坏: inv=%+v", inventory)
			}
		})
	}
}

// gridState 读取当前权威网格，是性质断言的简写。
func gridState(t *testing.T, engine *runtime.Engine, session runtime.SessionID) runtime.CraftingGrid {
	t.Helper()
	grid, _ := currentCraftingGrid(t, engine, session)
	return grid
}

// TestCraftingRepackMiningDropStaysInWorldWhenMarginal 覆盖 spec 场景
// 「采掘掉落保持不变量」：边界状态下采掘照常完成（方块变空气、不拒绝），
// 但掉落物入背包当且仅当回收不变量保持——这里必须留在世界里。
func TestCraftingRepackMiningDropStaysInWorldWhenMarginal(t *testing.T) {
	engine, session, sequence := repackMarginalSetup(t)

	*sequence++
	mined := mineUntilComplete(t, engine, session, sequence, 0, lookDown, 5)
	if len(mined.Rejected) != 0 || len(mined.Changes) != 1 {
		t.Fatalf("边界采掘 result=%+v，想要照常完成不拒绝", mined)
	}
	// 采掘完成在 advanceMining（晚于本 tick 的 advanceDrops），补一个释放
	// 采掘键的 tick 让新掉落物真正经过拾取判定。
	*sequence++
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: *sequence, Kind: runtime.CommandPlayerInput,
		Pitch: lookDown, Mining: false,
	})
	engine.Step()
	// 掉落物（1 草方块）只能占空格或找同类余量，两者都会破坏网格回收，
	// 因此必须留在世界且物品状态不变。
	chunk, _, ok := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})
	if !ok {
		t.Fatal("中心区块不可用")
	}
	found := false
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active {
			found = true
			if drop.Stack != (core.ItemStack{Item: core.ItemGrass, Count: 1}) {
				t.Fatalf("采掘掉落物 = %+v，想要 1 个草方块", drop.Stack)
			}
		}
	}
	if !found {
		t.Fatal("边界采掘的掉落物没有留在世界")
	}
	if grid, _ := currentCraftingGrid(t, engine, session); !repackOracle(currentInventory(t, engine, session), grid) {
		t.Fatal("采掘后回收不变量被破坏")
	}
}

// TestCraftingRepackContainerMoveRejectedWhenItWouldBreakInvariant 覆盖
// 跨容器移动这条背包增量入口：箱子 → 背包的移动若会挤占网格回收所需的
// 唯一空格，MUST 拒绝且背包、箱子与网格逐格不变。箱内物品不进网格语义，
// 但移动结果让背包多了一堆，回收不变量同样约束它。
func TestCraftingRepackContainerMoveRejectedWhenItWouldBreakInvariant(t *testing.T) {
	engine, session, sequence := repackMarginalSetup(t)

	// 在玩家脚下放一个箱子并放入 2 个泥土，随后打开它。
	key := core.ChunkKey{Dimension: core.Overworld}
	index := dropTargetIndex(t)
	engine.SetBlockForTest(core.BlockPos{}, core.ChestID)
	engine.SetChunkChestForTest(key, 0, world.ChestSlot{
		Generation: 1,
		Active:     true,
		BlockIndex: index,
		Items:      [core.ChestSlots]core.ItemStack{},
	})
	chest := world.ChestSlot{Generation: 1, Active: true, BlockIndex: index}
	chest.Items[0] = core.ItemStack{Item: core.ItemDirt, Count: 2}
	engine.SetChunkChestForTest(key, 0, chest)

	*sequence++
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: *sequence, Kind: runtime.CommandOpenFurnace, Pitch: lookDown,
	})
	if result := engine.Step(); len(result.Rejected) != 0 {
		t.Fatalf("打开箱子被拒绝: %+v", result.Rejected)
	}

	inventoryBefore := currentInventory(t, engine, session)
	gridBefore, _ := currentCraftingGrid(t, engine, session)
	*sequence++
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: *sequence, Kind: runtime.CommandMoveFurnaceStack,
		Furnace: core.ContainerRef{
			Dimension: core.Overworld, Kind: core.ContainerKindChest, Slot: 0,
			Generation: 1,
		},
		Slot:   core.ChestFirstSlot,
		ToSlot: 1,
	})
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
		t.Fatalf("破坏不变量的跨容器移动 result=%+v，想要 invalid_input 拒绝", result)
	}
	if got := currentInventory(t, engine, session); got != inventoryBefore {
		t.Fatalf("被拒的跨容器移动修改了背包: %+v", got)
	}
	if grid, _ := currentCraftingGrid(t, engine, session); grid != gridBefore {
		t.Fatalf("被拒的跨容器移动修改了网格: %+v", grid)
	}
	chunk, _, _ := engine.CloneReadyChunk(key)
	gotChest := chunk.Chest(0)
	if !gotChest.Active || gotChest.Items[0] != (core.ItemStack{Item: core.ItemDirt, Count: 2}) {
		t.Fatalf("被拒的跨容器移动修改了箱子: %+v", gotChest.Items[0])
	}
}

// TestCraftingRepackMoveRejectedWhenItWouldBreakInvariant 是防御分支测试：
// 用测试钩子构造命令无法构造的「回收预算恰好只剩空格」的网格状态，
// 随后的合法语义移动（空目标整堆迁移）因预演发现移动后网格无法回收而
// MUST 拒绝且逐格不变。正常命令路径维持不变量，这条分支正常不可达。
func TestCraftingRepackMoveRejectedWhenItWouldBreakInvariant(t *testing.T) {
	var inventory core.Inventory
	for slot := range inventory.Hotbar.Slots {
		inventory.Hotbar.Slots[slot] = core.ItemStack{
			Item: core.ItemStone, Count: core.MaxStackCount,
		}
	}
	for slot := range inventory.Backpack {
		inventory.Backpack[slot] = core.ItemStack{
			Item: core.ItemStone, Count: core.MaxStackCount,
		}
	}
	engine, session := readyFlatPlayerWithInventory(t, inventory)
	// 钩子注入一个满背包无法回收的圆石堆：任何向网格加压的移动都必须被
	// 移动层的不变量预演拦下。
	engine.SetPlayerCraftingGridForTest(session, func(grid runtime.CraftingGrid) runtime.CraftingGrid {
		grid.Slots[0] = core.ItemStack{Item: core.ItemCobblestone, Count: core.MaxStackCount}
		return grid
	})

	engine.Enqueue(craftingMoveCommand(session, 2, 9, 1))
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
		t.Fatalf("破坏不变量的移动 result=%+v，想要 invalid_input 拒绝", result)
	}
	grid, _ := currentCraftingGrid(t, engine, session)
	if grid.Slots[1] != (core.ItemStack{}) || grid.Slots[0].Count != core.MaxStackCount {
		t.Fatalf("被拒的移动改写了网格: %+v", grid.Slots)
	}
	if got := currentInventory(t, engine, session); got != inventory {
		t.Fatalf("被拒的移动修改了物品状态: %+v", got)
	}
}

// TestCraftingRepackInvariantHoldsAtTickEnd 是 spec「任意权威 tick 结束时
// 36 格背包总能无损容纳网格全部物品」的性质测试：表驱动地在边界状态下
// 施加一组状态变换（移动、取出、拾取、采掘），断言每个 tick 结束时回收
// 不变量恒成立。若任一背包增量入口漏守卫，这里的拾取步骤即红。
func TestCraftingRepackInvariantHoldsAtTickEnd(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 63}
	for slot := uint8(1); slot < core.HotbarSlots; slot++ {
		inventory.Hotbar.Slots[slot] = core.ItemStack{
			Item: core.ItemStone, Count: core.MaxStackCount,
		}
	}
	for slot := uint8(0); slot < core.BackpackSlots-2; slot++ {
		inventory.Backpack[slot] = core.ItemStack{
			Item: core.ItemStone, Count: core.MaxStackCount,
		}
	}
	// 留两个空格并备一组原料：木板 ×2 与原木 ×2 供网格与取出步骤使用。
	inventory.Backpack[core.BackpackSlots-2] = core.ItemStack{
		Item: core.ItemOakPlanks, Count: 2,
	}
	inventory.Backpack[core.BackpackSlots-1] = core.ItemStack{
		Item: core.ItemOakLog, Count: 2,
	}
	engine, session := readyFlatPlayerWithInventory(t, inventory)
	sequence := uint64(2)
	assertInvariant := func(label string) {
		t.Helper()
		if !repackOracle(currentInventory(t, engine, session), gridState(t, engine, session)) {
			t.Fatalf("%s 之后回收不变量被破坏", label)
		}
	}

	// 原木入网格并取出：产物与剩余原木各占一个空格，预算恰好足够。
	engine.Enqueue(craftingMoveCommand(session, sequence, 44, 0))
	engine.Step()
	assertInvariant("原木入网格")
	engine.Enqueue(craftingTakeCommand(session, sequence+1))
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("预算内取出被拒绝: %+v", result.Rejected)
	}
	assertInvariant("取出产物")

	// 木板并入网格（合并语义），再尝试一次会破坏预算的拾取。
	engine.Enqueue(craftingMoveCommand(session, sequence+2, 43, 0))
	engine.Step()
	assertInvariant("木板并入网格")
	repackTestDrop(t, engine, core.ItemStack{Item: core.ItemDirt, Count: 1})
	engine.Step()
	assertInvariant("拾取被拒后")

	// 同类并栈放行的拾取：石头 ×1 并入 63 的余量，不消耗空格。
	repackTestDrop(t, engine, core.ItemStack{Item: core.ItemStone, Count: 1})
	engine.Step()
	assertInvariant("同类并栈拾取后")

	// 采掘照常完成，掉落是否入包由不变量裁决；再补一个拾取判定 tick。
	mineUntilComplete(t, engine, session, &sequence, 0, lookDown, 5)
	sequence++
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: sequence, Kind: runtime.CommandPlayerInput,
		Pitch: lookDown, Mining: false,
	})
	engine.Step()
	assertInvariant("采掘完成后")
}

// —— 任务组 2.3/2.4：工作台打开与生命周期回收 ——

// openWorkbenchUnderPlayer 把玩家脚下方块换成工作台并用既有容器交互命令打开，
// 返回递增后的下一条命令序号。打开成功即断言网格尺寸为 3。
func openWorkbenchUnderPlayer(
	t *testing.T,
	engine *runtime.Engine,
	session runtime.SessionID,
	sequence uint64,
) uint64 {
	t.Helper()
	engine.SetBlockForTest(core.BlockPos{}, core.WorkbenchID)
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: sequence, Kind: runtime.CommandOpenFurnace, Pitch: lookDown,
	})
	if result := engine.Step(); len(result.Rejected) != 0 {
		t.Fatalf("打开工作台被拒绝: %+v", result.Rejected)
	}
	grid, _ := currentCraftingGrid(t, engine, session)
	if grid.Size != 3 {
		t.Fatalf("打开后网格尺寸 = %d，想要 3", grid.Size)
	}
	return sequence + 1
}

// fillExtendedSlots 打开工作台后把五堆物品分别放进扩展格 4..8。
func fillExtendedSlots(
	t *testing.T,
	engine *runtime.Engine,
	session runtime.SessionID,
	sequence uint64,
	item core.ItemID,
) uint64 {
	t.Helper()
	for index := range 5 {
		engine.Enqueue(craftingMoveCommand(
			session, sequence+uint64(index), uint8(10+index), uint8(4+index),
		))
	}
	if result := engine.Step(); len(result.Rejected) != 0 {
		t.Fatalf("填充扩展格被拒绝: %+v", result.Rejected)
	}
	return sequence + 5
}

// TestWorkbenchOpenSetsSizeThreeWithoutContainerRef 覆盖 spec 场景「射线交互
// 打开 3×3」：打开只提升该玩家网格尺寸并发布完整状态，不占用任何容器引用
// （不发布熔炉/箱子状态，也不产生关闭通知）。
func TestWorkbenchOpenSetsSizeThreeWithoutContainerRef(t *testing.T) {
	engine, session := readyFlatPlayer(t)

	engine.SetBlockForTest(core.BlockPos{}, core.WorkbenchID)
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: 2, Kind: runtime.CommandOpenFurnace, Pitch: lookDown,
	})
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("打开工作台被拒绝: %+v", result.Rejected)
	}
	if len(result.Craftings) != 1 || result.Craftings[0].Size != 3 ||
		result.Craftings[0].Session != session {
		t.Fatalf("打开后应发布尺寸 3 的网格状态: %+v", result.Craftings)
	}
	if len(result.Furnaces) != 0 || len(result.FurnaceEnds) != 0 || len(result.Chests) != 0 {
		t.Fatalf("工作台不得占用容器引用: Furnaces=%d Ends=%d Chests=%d",
			len(result.Furnaces), len(result.FurnaceEnds), len(result.Chests))
	}
	grid, _ := currentCraftingGrid(t, engine, session)
	if grid.Size != 3 {
		t.Fatalf("网格尺寸 = %d，想要 3", grid.Size)
	}
}

// TestWorkbenchOpenRejectsNonWorkbenchTarget 覆盖打开的目标校验：
// 射线命中普通方块（草）时按既有容器打开语义拒绝且尺寸保持 2。
func TestWorkbenchOpenRejectsNonWorkbenchTarget(t *testing.T) {
	engine, session := readyFlatPlayer(t)

	engine.Enqueue(runtime.Command{
		Session: session, Sequence: 2, Kind: runtime.CommandOpenFurnace, Pitch: lookDown,
	})
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectNoTarget {
		t.Fatalf("非工作台目标 result=%+v，想要 no_target 拒绝", result)
	}
	if grid, _ := currentCraftingGrid(t, engine, session); grid.Size != 2 {
		t.Fatalf("被拒的打开改了尺寸: %d", grid.Size)
	}
}

// TestWorkbenchMiningTakesFifteenTicksAndDropsOneWorkbench 覆盖 spec 场景
// 「放置与挖回」的采掘半边：工作台与橡木木板同价（木质 tier 15 tick），
// 徒手可挖，破坏后掉落恰好一个工作台物品。
func TestWorkbenchMiningTakesFifteenTicksAndDropsOneWorkbench(t *testing.T) {
	engine, session := readyFlatPlayer(t)
	engine.SetBlockForTest(core.BlockPos{}, core.WorkbenchID)

	sequence := uint64(1)
	result := mineUntilComplete(t, engine, session, &sequence, 0, lookDown, 14)
	if len(result.Changes) != 0 || len(result.Rejected) != 0 {
		t.Fatalf("14 tick 不应完成采掘: %+v", result)
	}
	sequence++
	engine.Enqueue(runtime.Command{
		Session: session, Sequence: sequence, Kind: runtime.CommandPlayerInput,
		Pitch: lookDown, Mining: true,
	})
	final := engine.Step()
	if len(final.Rejected) != 0 || len(final.Changes) != 1 ||
		final.Changes[0].Changes[0] != (runtime.BlockChange{Position: core.BlockPos{}, Block: core.AirID}) {
		t.Fatalf("第 15 tick 采掘 result=%+v", final)
	}
	chunk, _, ok := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld})
	if !ok {
		t.Fatal("中心区块不可用")
	}
	found := false
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active {
			if found {
				t.Fatalf("工作台掉落超过一堆: 槽 %d = %+v", slot, drop)
			}
			found = true
			if drop.Stack != (core.ItemStack{Item: core.ItemWorkbench, Count: 1}) {
				t.Fatalf("工作台掉落 = %+v，想要恰好 1 个工作台", drop.Stack)
			}
		}
	}
	if !found {
		t.Fatal("挖回工作台没有产生掉落物")
	}
}

// TestWorkbenchGridsOfTwoPlayersAreIndependent 覆盖 spec 场景「多玩家网格独立」：
// 两名玩家同时打开同一台工作台，各自网格互不可见。
func TestWorkbenchGridsOfTwoPlayersAreIndependent(t *testing.T) {
	engine, first := readyFlatPlayer(t)
	second := runtime.SessionID(2)
	engine.RegisterSession(second, core.Overworld, core.ChunkPos{})
	for range 30 {
		result := engine.Step()
		if len(result.Players) == 2 && result.Players[0].Ready && result.Players[1].Ready {
			break
		}
	}
	engine.SetPlayerPositionForTest(second, mgl32.Vec3{0.5, 1, 0.5})
	engine.Step()
	engine.SetBlockForTest(core.BlockPos{}, core.WorkbenchID)
	engine.Enqueue(runtime.Command{
		Session: first, Sequence: 2, Kind: runtime.CommandOpenFurnace, Pitch: lookDown,
	})
	engine.Enqueue(runtime.Command{
		Session: second, Sequence: 2, Kind: runtime.CommandOpenFurnace, Pitch: lookDown,
	})
	if result := engine.Step(); len(result.Rejected) != 0 {
		t.Fatalf("双玩家打开被拒绝: %+v", result.Rejected)
	}

	engine.Enqueue(craftingMoveCommand(first, 3, 9, 0))
	engine.SetPlayerInventoryForTest(first, func(inventory core.Inventory) core.Inventory {
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
		return inventory
	})
	engine.Step()

	firstGrid, _ := currentCraftingGrid(t, engine, first)
	if firstGrid.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("玩家甲的网格格 0 = %+v", firstGrid.Slots[0])
	}
	secondGrid, _ := currentCraftingGrid(t, engine, second)
	if secondGrid.Size != 3 {
		t.Fatalf("玩家乙尺寸 = %d，想要 3", secondGrid.Size)
	}
	for slot := uint8(0); slot < core.CraftingGridSlots; slot++ {
		if secondGrid.Slots[slot] != (core.ItemStack{}) {
			t.Fatalf("玩家甲的操作泄漏进乙的格 %d: %+v", slot, secondGrid.Slots[slot])
		}
	}
}

// TestCraftingCloseCommandRepacksExtendedSlots 覆盖 spec 场景「关闭工作台先回收
// 扩展格」：主动关闭先把格 4..8 无损装回背包再把尺寸降为 2，格 0..3 内容保持。
func TestCraftingCloseCommandRepacksExtendedSlots(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 4}
	for slot := uint8(1); slot <= 5; slot++ {
		stocked.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemCobblestone, Count: 2}
	}
	engine, session := readyFlatPlayerWithInventory(t, stocked)
	sequence := openWorkbenchUnderPlayer(t, engine, session, 2)
	engine.Enqueue(craftingMoveCommand(session, sequence, 9, 0))
	engine.Step()
	sequence = fillExtendedSlots(t, engine, session, sequence+1, core.ItemCobblestone)

	engine.Enqueue(runtime.Command{
		Session: session, Sequence: sequence, Kind: runtime.CommandCloseFurnace,
	})
	result := engine.Step()
	if len(result.Rejected) != 0 {
		t.Fatalf("关闭工作台被拒绝: %+v", result.Rejected)
	}
	grid := personalGrid(t, engine, session)
	if grid.Slots[0] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 4}) {
		t.Fatalf("关闭后格 0 = %+v，想要保持木板", grid.Slots[0])
	}
	for slot := uint8(4); slot <= 8; slot++ {
		if grid.Slots[slot] != (core.ItemStack{}) {
			t.Fatalf("关闭后扩展格 %d = %+v，想要已回收", slot, grid.Slots[slot])
		}
	}
	// 五堆圆石共 10 个必须全部回到背包。
	got := currentInventory(t, engine, session)
	total := 0
	for slot := uint8(0); slot < core.InventorySlots; slot++ {
		stack, _ := got.Slot(slot)
		if stack.Item == core.ItemCobblestone {
			total += int(stack.Count)
		}
	}
	if total != 10 {
		t.Fatalf("回收后背包圆石共 %d 个，想要 10", total)
	}
	if len(result.Craftings) != 1 || result.Craftings[0].Size != 2 {
		t.Fatalf("关闭应发布尺寸 2 的网格状态: %+v", result.Craftings)
	}
}

// TestCraftingCloseCommandRejectedWhenRepackImpossible 覆盖「无法完整回收时拒绝
// 关闭请求且状态不变」。正常命令路径维持回收不变量，这条分支不可达，因此用
// 测试钩子注入无法回收的扩展格堆；锚点工作台仍在原地，生命周期检查不会介入。
func TestCraftingCloseCommandRejectedWhenRepackImpossible(t *testing.T) {
	engine, session := readyFlatPlayerWithInventory(t, fullTestInventory())
	sequence := openWorkbenchUnderPlayer(t, engine, session, 2)
	engine.SetPlayerCraftingGridForTest(session, func(grid runtime.CraftingGrid) runtime.CraftingGrid {
		grid.Slots[5] = core.ItemStack{Item: core.ItemCobblestone, Count: core.MaxStackCount}
		return grid
	})

	engine.Enqueue(runtime.Command{
		Session: session, Sequence: sequence, Kind: runtime.CommandCloseFurnace,
	})
	result := engine.Step()
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != runtime.RejectInvalidInput {
		t.Fatalf("不可回收的关闭 result=%+v，想要 invalid_input 拒绝", result)
	}
	grid, _ := currentCraftingGrid(t, engine, session)
	if grid.Size != 3 || grid.Slots[5].Count != core.MaxStackCount {
		t.Fatalf("被拒的关闭改写了状态: size=%d slot5=%+v", grid.Size, grid.Slots[5])
	}
	// 网格状态仍会发布一次：测试钩子改写网格本身置脏，这与拒绝无关。
	if len(result.Craftings) != 1 {
		t.Fatalf("钩子置脏后应发布一次网格状态: %+v", result.Craftings)
	}
}

// TestCraftingWalkAwayClosesWorkbenchAndRepacks 覆盖 spec 场景「走远自动回收
// 关闭」：玩家离开触及距离的同一权威 tick，先回收格 4..8 再把尺寸降回 2。
func TestCraftingWalkAwayClosesWorkbenchAndRepacks(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 4}
	for slot := uint8(1); slot <= 5; slot++ {
		stocked.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemCobblestone, Count: 2}
	}
	engine, session := readyFlatPlayerWithInventory(t, stocked)
	sequence := openWorkbenchUnderPlayer(t, engine, session, 2)
	engine.Enqueue(craftingMoveCommand(session, sequence, 9, 0))
	engine.Step()
	fillExtendedSlots(t, engine, session, sequence+1, core.ItemCobblestone)

	engine.SetPlayerPositionForTest(session, mgl32.Vec3{10.5, 1, 10.5})
	result := engine.Step()
	grid := personalGrid(t, engine, session)
	if grid.Slots[0] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 4}) {
		t.Fatalf("走远后格 0 = %+v，想要保持", grid.Slots[0])
	}
	for slot := uint8(4); slot <= 8; slot++ {
		if grid.Slots[slot] != (core.ItemStack{}) {
			t.Fatalf("走远后扩展格 %d = %+v，想要已回收", slot, grid.Slots[slot])
		}
	}
	if len(result.Craftings) != 1 || result.Craftings[0].Size != 2 {
		t.Fatalf("走远应发布尺寸 2 的最终网格: %+v", result.Craftings)
	}
}

// TestCraftingWorkbenchMinedClosesSameTick 覆盖 spec 场景「工作台被挖立即关闭」：
// 方块变空气的同一权威 tick，打开者的网格按关闭规则回收并回到尺寸 2。
// 这里由打开者自己挖掉脚下工作台：采掘不受网格打开抑制，语义与其他采掘者相同。
func TestCraftingWorkbenchMinedClosesSameTick(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemOakPlanks, Count: 4}
	for slot := uint8(1); slot <= 5; slot++ {
		stocked.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemCobblestone, Count: 2}
	}
	engine, session := readyFlatPlayerWithInventory(t, stocked)
	sequence := openWorkbenchUnderPlayer(t, engine, session, 2)
	engine.Enqueue(craftingMoveCommand(session, sequence, 9, 0))
	engine.Step()
	fillExtendedSlots(t, engine, session, sequence+1, core.ItemCobblestone)

	// 打开者徒手挖掉脚下的工作台（木质 tier 15 tick）。
	minerSequence := uint64(100)
	mined := mineUntilComplete(t, engine, session, &minerSequence, 0, lookDown, 15)
	if len(mined.Rejected) != 0 || len(mined.Changes) != 1 {
		t.Fatalf("采掘工作台 result=%+v", mined)
	}
	grid := personalGrid(t, engine, session)
	if grid.Slots[0] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 4}) {
		t.Fatalf("被挖后格 0 = %+v，想要保持", grid.Slots[0])
	}
	for slot := uint8(4); slot <= 8; slot++ {
		if grid.Slots[slot] != (core.ItemStack{}) {
			t.Fatalf("被挖后扩展格 %d = %+v，想要同 tick 回收", slot, grid.Slots[slot])
		}
	}
	// 回收的五堆圆石共 10 个全部回到背包。
	got := currentInventory(t, engine, session)
	total := 0
	for slot := uint8(0); slot < core.InventorySlots; slot++ {
		stack, _ := got.Slot(slot)
		if stack.Item == core.ItemCobblestone {
			total += int(stack.Count)
		}
	}
	if total != 10 {
		t.Fatalf("被挖回收后背包圆石共 %d 个，想要 10", total)
	}
}

// TestCraftingDisconnectRepacksGridIntoSnapshotInventory 覆盖 spec 场景
// 「断线后物品回到背包」：断线（`UnregisterSession`）先无损回收全部 9 格再取
// 持久化快照；存续期间的周期性快照（`PlayerSnapshot`）MUST NOT 回收在玩玩家的
// 网格。
func TestCraftingDisconnectRepacksGridIntoSnapshotInventory(t *testing.T) {
	var stocked core.Inventory
	stocked.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 5}
	engine, session := readyFlatPlayerWithInventory(t, stocked)
	engine.Enqueue(craftingMoveCommand(session, 2, 9, 2))
	engine.Step()

	// 周期性观察不回收：网格内容原样保留。
	if _, ok := engine.PlayerSnapshot(session); !ok {
		t.Fatal("在玩玩家没有快照")
	}
	grid, _ := currentCraftingGrid(t, engine, session)
	if grid.Slots[2] != (core.ItemStack{Item: core.ItemStone, Count: 5}) {
		t.Fatalf("周期性快照回收了在玩网格: %+v", grid.Slots[2])
	}

	snapshot, ok := engine.UnregisterSession(session)
	if !ok {
		t.Fatal("断线快照不可用")
	}
	if snapshot.Inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 5}) {
		t.Fatalf("断线快照未包含网格回收: %+v", snapshot.Inventory.Hotbar.Slots[0])
	}
	if _, _, ok := engine.PlayerCrafting(session); ok {
		t.Fatal("断线后网格仍可观察")
	}
}

// TestCraftingDisconnectPanicsWhenRepackImpossible 锁定断线回收的内部错误路径：
// 回收不变量保证合法状态不可达；一旦不可回收状态出现（这里用钩子构造），
// 断线持久化 MUST 以 panic 暴露而不是静默丢物。
func TestCraftingDisconnectPanicsWhenRepackImpossible(t *testing.T) {
	engine, session := readyFlatPlayerWithInventory(t, fullTestInventory())
	engine.SetPlayerCraftingGridForTest(session, func(grid runtime.CraftingGrid) runtime.CraftingGrid {
		grid.Slots[2] = core.ItemStack{Item: core.ItemCobblestone, Count: core.MaxStackCount}
		return grid
	})

	defer func() {
		if recover() == nil {
			t.Fatal("不可回收的断线持久化必须 panic 暴露内部错误")
		}
	}()
	engine.UnregisterSession(session)
}

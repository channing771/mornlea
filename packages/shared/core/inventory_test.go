package core_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestInventoryShapeIsFixed(t *testing.T) {
	if core.BackpackSlots != 27 {
		t.Fatalf("BackpackSlots = %d，契约要求 27", core.BackpackSlots)
	}
	if core.InventorySlots != core.HotbarSlots+core.BackpackSlots {
		t.Fatalf("InventorySlots = %d，想要 %d", core.InventorySlots, core.HotbarSlots+core.BackpackSlots)
	}
	var inventory core.Inventory
	if len(inventory.Backpack) != core.BackpackSlots {
		t.Fatalf("len(Backpack) = %d，想要 %d", len(inventory.Backpack), core.BackpackSlots)
	}
	if !inventory.Valid() {
		t.Fatal("零值 Inventory 应当有效")
	}
}

func TestInventoryValidRejectsBadSlots(t *testing.T) {
	badHotbar := core.Inventory{}
	badHotbar.Hotbar.Selected = core.HotbarSlots
	badBackpackItem := core.Inventory{}
	badBackpackItem.Backpack[3] = core.ItemStack{Item: core.ItemID(4242), Count: 1}
	badBackpackCount := core.Inventory{}
	badBackpackCount.Backpack[0] = core.ItemStack{
		Item: core.ItemStone, Count: core.MaxStackCount + 1,
	}
	ghost := core.Inventory{}
	ghost.Backpack[26] = core.ItemStack{Item: core.ItemNone, Count: 2}

	for name, inventory := range map[string]core.Inventory{
		"越界选中栏位":  badHotbar,
		"未知背包物品":  badBackpackItem,
		"背包数量超限":  badBackpackCount,
		"空物品非零数量": ghost,
	} {
		if inventory.Valid() {
			t.Fatalf("%s：非法 Inventory 被接受", name)
		}
	}
}

func TestInventorySlotIndexMapsHotbarThenBackpack(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemStone, Count: 1}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemDirt, Count: 2}
	inventory.Backpack[core.BackpackSlots-1] = core.ItemStack{Item: core.ItemGrass, Count: 3}

	cases := []struct {
		slot uint8
		want core.ItemStack
	}{
		{2, core.ItemStack{Item: core.ItemStone, Count: 1}},
		{core.HotbarSlots, core.ItemStack{Item: core.ItemDirt, Count: 2}},
		{core.InventorySlots - 1, core.ItemStack{Item: core.ItemGrass, Count: 3}},
	}
	for _, tc := range cases {
		got, ok := inventory.Slot(tc.slot)
		if !ok || got != tc.want {
			t.Fatalf("Slot(%d) = %+v, %v，想要 %+v, true", tc.slot, got, ok, tc.want)
		}
	}
	if _, ok := inventory.Slot(core.InventorySlots); ok {
		t.Fatal("越界索引被接受")
	}
}

func TestInventoryAddStackFillsHotbarBeforeBackpack(t *testing.T) {
	var inventory core.Inventory
	// 快捷栏同类未满优先，其次快捷栏空格。
	inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemStone, Count: 62}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 60}

	next, remainder := inventory.AddStack(core.ItemStack{Item: core.ItemStone, Count: 3})
	if remainder != (core.ItemStack{}) {
		t.Fatalf("余量 = %+v，想要全部装入", remainder)
	}
	if next.Hotbar.Slots[4].Count != core.MaxStackCount {
		t.Fatalf("快捷栏同类格 = %+v，想要补满", next.Hotbar.Slots[4])
	}
	if next.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 1}) {
		t.Fatalf("快捷栏空格 = %+v，想要接收剩余 1 个", next.Hotbar.Slots[0])
	}
	if next.Backpack[0].Count != 60 {
		t.Fatalf("背包同类格被提前使用: %+v", next.Backpack[0])
	}
	if inventory.Hotbar.Slots[4].Count != 62 {
		t.Fatal("AddStack 必须在值副本上完成")
	}
}

func TestInventoryAddStackFallsBackToBackpack(t *testing.T) {
	var inventory core.Inventory
	for slot := range inventory.Hotbar.Slots {
		inventory.Hotbar.Slots[slot] = core.ItemStack{
			Item: core.ItemDirt, Count: core.MaxStackCount,
		}
	}
	inventory.Backpack[5] = core.ItemStack{Item: core.ItemStone, Count: 63}

	next, remainder := inventory.AddStack(core.ItemStack{Item: core.ItemStone, Count: 4})
	if remainder != (core.ItemStack{}) {
		t.Fatalf("余量 = %+v，想要全部装入", remainder)
	}
	if next.Backpack[5].Count != core.MaxStackCount {
		t.Fatalf("背包同类格 = %+v，想要补满", next.Backpack[5])
	}
	if next.Backpack[0] != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("背包最低空格 = %+v，想要接收剩余 3 个", next.Backpack[0])
	}
}

func TestInventoryAddStackKeepsRemainderWhenFull(t *testing.T) {
	var inventory core.Inventory
	for slot := range inventory.Hotbar.Slots {
		inventory.Hotbar.Slots[slot] = core.ItemStack{
			Item: core.ItemDirt, Count: core.MaxStackCount,
		}
	}
	for slot := range inventory.Backpack {
		inventory.Backpack[slot] = core.ItemStack{
			Item: core.ItemDirt, Count: core.MaxStackCount,
		}
	}

	next, remainder := inventory.AddStack(core.ItemStack{Item: core.ItemStone, Count: 5})
	if next != inventory {
		t.Fatal("全满时 Inventory 必须保持不变")
	}
	if remainder != (core.ItemStack{Item: core.ItemStone, Count: 5}) {
		t.Fatalf("余量 = %+v，想要原样保留", remainder)
	}
}

func TestInventoryAddStackPartiallyFills(t *testing.T) {
	var inventory core.Inventory
	for slot := range inventory.Hotbar.Slots {
		inventory.Hotbar.Slots[slot] = core.ItemStack{
			Item: core.ItemDirt, Count: core.MaxStackCount,
		}
	}
	for slot := range inventory.Backpack {
		inventory.Backpack[slot] = core.ItemStack{
			Item: core.ItemDirt, Count: core.MaxStackCount,
		}
	}
	inventory.Backpack[9] = core.ItemStack{Item: core.ItemStone, Count: 62}

	next, remainder := inventory.AddStack(core.ItemStack{Item: core.ItemStone, Count: 5})
	if next.Backpack[9].Count != core.MaxStackCount {
		t.Fatalf("部分装入后背包格 = %+v", next.Backpack[9])
	}
	if remainder != (core.ItemStack{Item: core.ItemStone, Count: 3}) {
		t.Fatalf("余量 = %+v，想要保留 3 个", remainder)
	}
}

func TestInventoryAddStackRejectsInvalidStack(t *testing.T) {
	var inventory core.Inventory
	invalid := []core.ItemStack{
		{},
		{Item: core.ItemStone, Count: 0},
		{Item: core.ItemNone, Count: 3},
		{Item: core.ItemID(4242), Count: 1},
		{Item: core.ItemStone, Count: core.MaxStackCount + 1},
		{Item: core.ItemStonePickaxe, Count: 1},
		{Item: core.ItemStonePickaxe, Count: 1, Durability: 132},
		{Item: core.ItemStone, Count: 1, Durability: 1},
	}
	for _, stack := range invalid {
		next, remainder := inventory.AddStack(stack)
		if next != inventory || remainder != stack {
			t.Fatalf("非法堆 %+v 被处理: next=%+v remainder=%+v", stack, next, remainder)
		}
	}
}

func TestInventoryToolStackLimits(t *testing.T) {
	t.Run("第二把石镐使用下一个空格", func(t *testing.T) {
		var inventory core.Inventory
		firstTool := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 73}
		secondTool := core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 41}
		first, remainder := inventory.AddStack(firstTool)
		if remainder != (core.ItemStack{}) {
			t.Fatalf("第一把石镐余量 = %+v", remainder)
		}
		next, remainder := first.AddStack(secondTool)
		if remainder != (core.ItemStack{}) {
			t.Fatalf("第二把石镐余量 = %+v", remainder)
		}
		if next.Hotbar.Slots[0] != firstTool || next.Hotbar.Slots[1] != secondTool {
			t.Fatalf("两把石镐落点 = %+v / %+v，想要两个单格工具", next.Hotbar.Slots[0], next.Hotbar.Slots[1])
		}
		if !next.Valid() {
			t.Fatal("添加磨损工具后 Inventory 应当仍然有效")
		}
	})

	t.Run("同类工具不能移动合并", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 73}
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 41}
		next, ok := inventory.MoveStack(0, 1)
		if ok || next != inventory {
			t.Fatalf("同类工具移动 = %+v, %v，想要原值和 false", next, ok)
		}
	})

	t.Run("不同工具互换", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 73}
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 149}
		next, ok := inventory.MoveStack(0, 1)
		if !ok || next.Hotbar.Slots[0].Item != core.ItemIronPickaxe || next.Hotbar.Slots[1].Item != core.ItemStonePickaxe {
			t.Fatalf("不同工具交换 = %+v, %v", next, ok)
		}
	})

	t.Run("普通物品仍合并至 64", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
		inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 63}
		next, ok := inventory.MoveStack(0, 1)
		if !ok || next.Hotbar.Slots[0] != (core.ItemStack{}) || next.Hotbar.Slots[1].Count != 64 {
			t.Fatalf("普通物品合并 = %+v, %v", next, ok)
		}
	})

	t.Run("SetSlot 拒绝两个工具", func(t *testing.T) {
		inventory, ok := (core.Inventory{}).SetSlot(0, core.ItemStack{Item: core.ItemStonePickaxe, Count: 2, Durability: 131})
		if ok || inventory != (core.Inventory{}) {
			t.Fatalf("SetSlot 接受两个工具: %+v, %v", inventory, ok)
		}
	})
}

func TestInventoryMoveStackIntoEmptySlot(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 7}

	next, ok := inventory.MoveStack(1, core.HotbarSlots+4)
	if !ok {
		t.Fatal("向空格移动应当成功")
	}
	if next.Hotbar.Slots[1] != (core.ItemStack{}) {
		t.Fatalf("来源格 = %+v，想要清空", next.Hotbar.Slots[1])
	}
	if next.Backpack[4] != (core.ItemStack{Item: core.ItemStone, Count: 7}) {
		t.Fatalf("目标格 = %+v，想要整堆移入", next.Backpack[4])
	}
	if inventory.Hotbar.Slots[1].Count != 7 {
		t.Fatal("MoveStack 必须在值副本上完成")
	}
}

func TestInventoryMoveStackMergesAndKeepsRemainder(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 10}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 60}

	next, ok := inventory.MoveStack(0, core.HotbarSlots)
	if !ok {
		t.Fatal("同类合并应当成功")
	}
	if next.Backpack[0].Count != core.MaxStackCount {
		t.Fatalf("目标格 = %+v，想要补满到 64", next.Backpack[0])
	}
	if next.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 6}) {
		t.Fatalf("来源格 = %+v，想要保留 6 个", next.Hotbar.Slots[0])
	}
}

func TestInventoryMoveStackSwapsDifferentItems(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemStone, Count: 2}
	inventory.Backpack[7] = core.ItemStack{Item: core.ItemGrass, Count: 5}

	next, ok := inventory.MoveStack(3, core.HotbarSlots+7)
	if !ok {
		t.Fatal("异类交换应当成功")
	}
	if next.Hotbar.Slots[3] != (core.ItemStack{Item: core.ItemGrass, Count: 5}) ||
		next.Backpack[7] != (core.ItemStack{Item: core.ItemStone, Count: 2}) {
		t.Fatalf("交换结果 = %+v / %+v", next.Hotbar.Slots[3], next.Backpack[7])
	}
}

func TestInventoryMoveStackRejectsInvalidRequests(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}

	cases := []struct {
		name     string
		from, to uint8
	}{
		{"同格", 0, 0},
		{"来源越界", core.InventorySlots, 0},
		{"目标越界", 0, core.InventorySlots},
		{"空来源", 5, 0},
		{"同类目标已满", 0, 1},
	}
	for _, tc := range cases {
		next, ok := inventory.MoveStack(tc.from, tc.to)
		if ok {
			t.Fatalf("%s：非法移动被接受", tc.name)
		}
		if next != inventory {
			t.Fatalf("%s：非法移动修改了原值", tc.name)
		}
	}
}

// craftingCell 是合成网格消费测试夹具的一条放置记录：把 count 个 item 放进
// slot。与 recipe_test.go 的 `gridCell` 分开是因为这里需要非 1 的数量。
type craftingCell struct {
	slot  uint8
	item  core.ItemID
	count uint8
}

// buildConsumableGrid 按放置记录铺一个合成网格，未列出的格保持空。
func buildConsumableGrid(cells ...craftingCell) [core.CraftingGridSlots]core.ItemStack {
	var grid [core.CraftingGridSlots]core.ItemStack
	for _, cell := range cells {
		grid[cell.slot] = core.ItemStack{Item: cell.item, Count: cell.count}
	}
	return grid
}

// gridItemCount 统计网格内全部物品总数，用于「消费恰减、产物不回流」的守恒
// 断言。
func gridItemCount(grid [core.CraftingGridSlots]core.ItemStack) int {
	total := 0
	for _, stack := range grid {
		total += int(stack.Count)
	}
	return total
}

// TestConsumeRecipeDecrementsEachCoveredCellOnce 覆盖 spec Requirement「产物取出
// 一次恰合成一次且原子」的消费半边：石砖配方（2×2 石头）摆在 3×3 右下角，
// 消费对被形状覆盖的四个格各恰减 1、形状之外的格保持空、网格总物品数恰减 4，
// 且产物不写回网格（本函数根本不接触背包）。
func TestConsumeRecipeDecrementsEachCoveredCellOnce(t *testing.T) {
	pattern, ok := core.Recipe(core.RecipeStoneBricks)
	if !ok {
		t.Fatal("石砖配方不可用")
	}
	grid := buildConsumableGrid(
		craftingCell{4, core.ItemStone, 5}, craftingCell{5, core.ItemStone, 5},
		craftingCell{7, core.ItemStone, 5}, craftingCell{8, core.ItemStone, 5},
	)
	next, ok := core.ConsumeRecipe(3, grid, pattern)
	if !ok {
		t.Fatal("匹配形状的消费失败")
	}
	for _, slot := range []uint8{4, 5, 7, 8} {
		if next[slot] != (core.ItemStack{Item: core.ItemStone, Count: 4}) {
			t.Fatalf("被覆盖格 %d = %+v，想要 5−1=4 个石头", slot, next[slot])
		}
	}
	for _, slot := range []uint8{0, 1, 2, 3, 6} {
		if next[slot] != (core.ItemStack{}) {
			t.Fatalf("形状之外的格 %d 不再是空栈: %+v", slot, next[slot])
		}
	}
	if got, want := gridItemCount(next), gridItemCount(grid)-4; got != want {
		t.Fatalf("消费后网格物品总数 = %d，想要恰减 4 到 %d（产物不得写回网格）", got, want)
	}
	if grid[4].Count != 5 {
		t.Fatal("ConsumeRecipe 修改了调用方的原网格")
	}
}

// TestConsumeRecipeClearsEmptiedCells 锁定「扣到零的格规范化为空栈」：单格
// 原木配方的消费把那格清成零值，而不是留下数量 0 的残留栈。
func TestConsumeRecipeClearsEmptiedCells(t *testing.T) {
	pattern, ok := core.Recipe(core.RecipeOakPlanks)
	if !ok {
		t.Fatal("木板配方不可用")
	}
	grid := buildConsumableGrid(craftingCell{4, core.ItemOakLog, 1})
	next, ok := core.ConsumeRecipe(3, grid, pattern)
	if !ok {
		t.Fatal("单格原木消费失败")
	}
	if next[4] != (core.ItemStack{}) {
		t.Fatalf("扣空的格 = %+v，想要零值空栈", next[4])
	}
}

// TestConsumeRecipeAlignsWithActualPlacement 锁定消费跟随实际摆放的对齐：木棍
// 配方（纵向两木板）摆在中列，消费扣的是中列两格而不是形状的规范化位置
// （左上角）；镜像位开启的合成形状按镜像对齐消费。
func TestConsumeRecipeAlignsWithActualPlacement(t *testing.T) {
	sticks, ok := core.Recipe(core.RecipeStick)
	if !ok {
		t.Fatal("木棍配方不可用")
	}
	grid := buildConsumableGrid(
		craftingCell{1, core.ItemOakPlanks, 2}, craftingCell{4, core.ItemOakPlanks, 2},
	)
	next, ok := core.ConsumeRecipe(3, grid, sticks)
	if !ok {
		t.Fatal("中列木棍摆放的消费失败")
	}
	if next[1] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 1}) ||
		next[4] != (core.ItemStack{Item: core.ItemOakPlanks, Count: 1}) {
		t.Fatalf("消费未对齐实际摆放: %+v / %+v", next[1], next[4])
	}

	// 合成的左右不对称形状（开镜像位）：镜像摆放必须经镜像对齐被消费。
	asymmetric := core.RecipePattern{
		Width: 2, Height: 2, Mirror: true,
		Cells: [core.CraftingGridSlots]core.ItemID{
			core.ItemStone, core.ItemGlass, core.ItemNone,
			core.ItemDirt, core.ItemNone, core.ItemNone,
		},
		Output: core.ItemStack{Item: core.ItemStoneBrick, Count: 1},
	}
	// 镜像摆放 = 形状逐列翻转：G/S 顶排、泥土落在 (x1,y1) 即格 4。
	mirrored := buildConsumableGrid(
		craftingCell{0, core.ItemGlass, 2}, craftingCell{1, core.ItemStone, 2},
		craftingCell{4, core.ItemDirt, 2},
	)
	next, ok = core.ConsumeRecipe(3, mirrored, asymmetric)
	if !ok {
		t.Fatal("镜像摆放的消费失败：镜像位开启的形状必须按镜像对齐消费")
	}
	for _, slot := range []uint8{0, 1, 4} {
		if next[slot].Count != 1 {
			t.Fatalf("镜像对齐消费后格 %d = %+v，想要数量 1", slot, next[slot])
		}
	}
}

// TestConsumeRecipeRejectsDurablesAsMaterial 覆盖 spec Scenario「耐久物品不作为
// 材料」的消费层防御：石镐占了形状覆盖的格时，即使其余三格是石头、形状对齐
// 看似完整，消费也必须失败且网格一字不改（匹配层 `MatchCraftingGrid` 同样
// 拒绝，这里直接喂 pattern 证明消费层不依赖匹配层先行把关）。
func TestConsumeRecipeRejectsDurablesAsMaterial(t *testing.T) {
	pattern, ok := core.Recipe(core.RecipeStoneBricks)
	if !ok {
		t.Fatal("石砖配方不可用")
	}
	grid := buildConsumableGrid(
		craftingCell{0, core.ItemStone, 2}, craftingCell{1, core.ItemStone, 2},
		craftingCell{3, core.ItemStone, 2},
		craftingCell{4, core.ItemStonePickaxe, 1},
	)
	grid[4].Durability = 131
	next, ok := core.ConsumeRecipe(3, grid, pattern)
	if ok {
		t.Fatal("带耐久的工具被当作材料消费")
	}
	if next != grid {
		t.Fatalf("失败消费修改了原网格: %+v", next)
	}
}

// TestConsumeRecipeFailureReturnsOriginalGrid 锁定全部失败路径的原子性：空
// 网格、宽高不符、被覆盖格物品不同、被覆盖格数量为零、有效尺寸之外的格有
// 残留、非法尺寸与「镜像位关闭却按镜像摆放」——一律返回原网格与 false，
// 绝不留下部分扣减。
func TestConsumeRecipeFailureReturnsOriginalGrid(t *testing.T) {
	sticks, _ := core.Recipe(core.RecipeStick)
	furnace, _ := core.Recipe(core.RecipeFurnace)
	// 左右不对称且关闭镜像位的合成形状（注册表 1..13 没有这种组合——
	// 不对称的工具配方全部关镜像、开镜像的形状全部对称，见 design.md D3），
	// 专门证明消费层与匹配层共用同一条「镜像位即重试开关」纪律。
	strictAsymmetric := core.RecipePattern{
		Width: 2, Height: 2, Mirror: false,
		Cells: [core.CraftingGridSlots]core.ItemID{
			core.ItemStone, core.ItemGlass, core.ItemNone,
			core.ItemDirt, core.ItemNone, core.ItemNone,
		},
		Output: core.ItemStack{Item: core.ItemStoneBrick, Count: 1},
	}
	cases := []struct {
		name    string
		size    uint8
		grid    [core.CraftingGridSlots]core.ItemStack
		pattern core.RecipePattern
	}{
		{"空网格", 3, [core.CraftingGridSlots]core.ItemStack{}, sticks},
		{"宽高不符", 3, buildConsumableGrid(
			craftingCell{0, core.ItemOakPlanks, 2}, craftingCell{1, core.ItemOakPlanks, 2},
		), sticks},
		{"覆盖格物品不同", 3, buildConsumableGrid(
			craftingCell{0, core.ItemOakPlanks, 2}, craftingCell{3, core.ItemDirt, 2},
		), sticks},
		{"覆盖格数量为零", 3, buildConsumableGrid(
			craftingCell{0, core.ItemOakPlanks, 2}, craftingCell{3, core.ItemOakPlanks, 0},
		), sticks},
		{"个人网格扩展格残留", 2, buildConsumableGrid(
			craftingCell{0, core.ItemOakPlanks, 2}, craftingCell{3, core.ItemOakPlanks, 2},
			craftingCell{5, core.ItemDirt, 1},
		), sticks},
		{"非法尺寸", 4, buildConsumableGrid(
			craftingCell{0, core.ItemOakPlanks, 2}, craftingCell{3, core.ItemOakPlanks, 2},
		), sticks},
		{"镜像位关闭的镜像摆放", 3, buildConsumableGrid(
			craftingCell{0, core.ItemGlass, 2}, craftingCell{1, core.ItemStone, 2},
			craftingCell{4, core.ItemDirt, 2},
		), strictAsymmetric},
		// 形状空洞被占：熔炉圆环的中格放着泥土（哪怕只占形状的空格、不撑大
		// 包围盒），消费层也必须拒绝——空格上只允许零值空栈。
		{"形状空洞被占", 3, buildConsumableGrid(
			craftingCell{0, core.ItemCobblestone, 2}, craftingCell{1, core.ItemCobblestone, 2},
			craftingCell{2, core.ItemCobblestone, 2}, craftingCell{3, core.ItemCobblestone, 2},
			craftingCell{4, core.ItemDirt, 1},
			craftingCell{5, core.ItemCobblestone, 2}, craftingCell{6, core.ItemCobblestone, 2},
			craftingCell{7, core.ItemCobblestone, 2}, craftingCell{8, core.ItemCobblestone, 2},
		), furnace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next, ok := core.ConsumeRecipe(tc.size, tc.grid, tc.pattern)
			if ok {
				t.Fatalf("非法消费被接受: %+v", next)
			}
			if next != tc.grid {
				t.Fatalf("失败消费修改了原网格: %+v", next)
			}
		})
	}
}

func BenchmarkInventoryAddStack(b *testing.B) {
	var inventory core.Inventory
	for slot := range inventory.Hotbar.Slots {
		inventory.Hotbar.Slots[slot] = core.ItemStack{
			Item: core.ItemDirt, Count: core.MaxStackCount,
		}
	}
	stack := core.ItemStack{Item: core.ItemStone, Count: 64}
	b.ReportAllocs()
	for b.Loop() {
		inventory.AddStack(stack)
	}
}

func BenchmarkInventoryMoveStack(b *testing.B) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 32}
	inventory.Backpack[26] = core.ItemStack{Item: core.ItemGrass, Count: 32}
	b.ReportAllocs()
	for b.Loop() {
		inventory.MoveStack(0, core.InventorySlots-1)
	}
}

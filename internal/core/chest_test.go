package core_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestChestIDsAppendAtEnd 锁定箱子的三个稳定编号都追加在既有编号末尾，
// 插入到中间会平移后续 ID 并破坏既有存档与线上字节。
func TestChestIDsAppendAtEnd(t *testing.T) {
	if core.ChestID != core.IronBlockID+1 {
		t.Fatalf("ChestID = %d，必须紧随 IronBlockID(%d) 之后", core.ChestID, core.IronBlockID)
	}
	if core.ItemChest != core.ItemBrokenIronPickaxe+1 {
		t.Fatalf("ItemChest = %d，必须紧随 ItemBrokenIronPickaxe(%d) 之后", core.ItemChest, core.ItemBrokenIronPickaxe)
	}
	if core.RecipeChest != core.RecipeIronPickaxe+1 {
		t.Fatalf("RecipeChest = %d，必须紧随 RecipeIronPickaxe(%d) 之后", core.RecipeChest, core.RecipeIronPickaxe)
	}
	if core.RecipeOakPlanks != core.RecipeChest+1 {
		t.Fatalf("RecipeOakPlanks = %d，必须追加在 RecipeChest(%d) 之后", core.RecipeOakPlanks, core.RecipeChest)
	}
}

// TestChestIsPlaceableAndDrops 覆盖箱子物品放置后写入 ChestID，
// 破坏箱子方块掉落箱子本身。
func TestChestIsPlaceableAndDrops(t *testing.T) {
	block, ok := core.ItemPlacement(core.ItemChest)
	if !ok || block != core.ChestID {
		t.Fatalf("ItemPlacement(箱子) = (%d, %v)，想要 (%d, true)", block, ok, core.ChestID)
	}
	item, ok := core.BlockDrop(core.ChestID)
	if !ok || item != core.ItemChest {
		t.Fatalf("BlockDrop(箱子) = (%d, %v)，想要 (%d, true)", item, ok, core.ItemChest)
	}
}

// TestRegisteredItemAcceptsChestStack 覆盖箱子是没有耐久的可堆叠方块物品，
// 单格上限与其余方块物品一致为 MaxStackCount。
func TestRegisteredItemAcceptsChestStack(t *testing.T) {
	if !core.RegisteredItem(core.ItemChest) {
		t.Fatal("箱子物品未被登记为合法")
	}
	limit, ok := core.ItemStackLimit(core.ItemChest)
	if !ok || limit != core.MaxStackCount {
		t.Fatalf("ItemStackLimit(箱子) = (%d, %v)，想要 (%d, true)", limit, ok, core.MaxStackCount)
	}
	if _, hasDurability := core.ItemMaxDurability(core.ItemChest); hasDurability {
		t.Fatal("箱子不应该有耐久上限")
	}
	stack := core.ItemStack{Item: core.ItemChest, Count: 1}
	if !stack.Valid() {
		t.Fatalf("满足堆叠规则的箱子物品栈被判定非法: %+v", stack)
	}
}

// TestRecipeChestIsFixed 锁定箱子配方的形状：3×3 橡木木板圆环（中格为空）
// 合成 1 个箱子。格子工作台变更起箱子不再吃石头——圆环形状与箱子的「容器」
// 身份对齐，石头圆环让位给熔炉配方。
func TestRecipeChestIsFixed(t *testing.T) {
	pattern, ok := core.Recipe(core.RecipeChest)
	if !ok {
		t.Fatal("箱子配方不可用")
	}
	want := core.RecipePattern{
		Width: 3, Height: 3, Mirror: true,
		Cells: [core.CraftingGridSlots]core.ItemID{
			core.ItemOakPlanks, core.ItemOakPlanks, core.ItemOakPlanks,
			core.ItemOakPlanks, core.ItemNone, core.ItemOakPlanks,
			core.ItemOakPlanks, core.ItemOakPlanks, core.ItemOakPlanks,
		},
		Output: core.ItemStack{Item: core.ItemChest, Count: 1},
	}
	if pattern != want {
		t.Fatalf("箱子配方 = %+v，想要 %+v", pattern, want)
	}
}

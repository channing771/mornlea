package core

import "testing"

// 本文件是形状匹配器的白盒单元层：注册表 recipe 1..19 里不存在「左右不对称
// 且开 `Mirror` 位」的形状（唯一不对称的石锄/铁锄按 design.md D3 刻意关闭
// 镜像位），「水平镜像允许、垂直翻转与旋转永不允许」只能在合成形状上对私有
// `matchesPattern` 直接证明。注册表级的匹配行为由 recipe_test.go 的
// `MatchCraftingGrid` 用例覆盖。

// asymmetricPattern 是一条左右上下都不对称的合成形状（开镜像位）：
//
//	S G
//	D .
//
// S=石头、G=玻璃、D=泥土。它让镜像、翻转与旋转产生互不相同的形状，
// 是区分四种几何操作的唯一夹具。
var asymmetricPattern = RecipePattern{
	Width: 2, Height: 2, Mirror: true,
	Cells: [CraftingGridSlots]ItemID{
		ItemStone, ItemGlass, ItemNone,
		ItemDirt, ItemNone, ItemNone,
		ItemNone, ItemNone, ItemNone,
	},
	Output: ItemStack{Item: ItemStoneBrick, Count: 1},
}

func TestSwordRecipePatternsAreStable(t *testing.T) {
	tests := []struct {
		id      RecipeID
		wantID  RecipeID
		pattern RecipePattern
	}{
		{RecipeWoodenSword, 17, RecipePattern{
			Width: 1, Height: 3, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemOakPlanks, ItemNone, ItemNone,
				ItemOakPlanks, ItemNone, ItemNone,
				ItemStick, ItemNone, ItemNone,
			},
			Output: ItemStack{Item: ItemWoodenSword, Count: 1, Durability: 59},
		}},
		{RecipeStoneSword, 18, RecipePattern{
			Width: 1, Height: 3, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemCobblestone, ItemNone, ItemNone,
				ItemCobblestone, ItemNone, ItemNone,
				ItemStick, ItemNone, ItemNone,
			},
			Output: ItemStack{Item: ItemStoneSword, Count: 1, Durability: 131},
		}},
		{RecipeIronSword, 19, RecipePattern{
			Width: 1, Height: 3, Mirror: true,
			Cells: [CraftingGridSlots]ItemID{
				ItemIronIngot, ItemNone, ItemNone,
				ItemIronIngot, ItemNone, ItemNone,
				ItemStick, ItemNone, ItemNone,
			},
			Output: ItemStack{Item: ItemIronSword, Count: 1, Durability: 250},
		}},
	}
	for _, test := range tests {
		if test.id != test.wantID {
			t.Fatalf("recipe ID = %d，想要 %d", test.id, test.wantID)
		}
		pattern, ok := recipePattern(test.id)
		if !ok || pattern != test.pattern {
			t.Fatalf("recipe %d = %+v, %v，想要 %+v", test.id, pattern, ok, test.pattern)
		}
	}
}

// TestMatchesPatternAllowsOnlyHorizontalMirror 锁定 spec Requirement「形状匹配
// 裁边且仅允许水平镜像」的对称性集合：正向与水平镜像必须命中（镜像位开启时），
// 垂直翻转与 180° 旋转必须双方向（正向比较与镜像比较）都失败。
func TestMatchesPatternAllowsOnlyHorizontalMirror(t *testing.T) {
	// 正向形状摆在网格中央（origin 1,1），外围空行列由裁边归一化负责。
	forward := [CraftingGridSlots]ItemID{
		ItemNone, ItemNone, ItemNone,
		ItemNone, ItemStone, ItemGlass,
		ItemNone, ItemDirt, ItemNone,
	}
	// 水平镜像：S 与 G 交换列、D 跟随左列。
	mirrored := [CraftingGridSlots]ItemID{
		ItemNone, ItemNone, ItemNone,
		ItemNone, ItemGlass, ItemStone,
		ItemNone, ItemNone, ItemDirt,
	}
	// 垂直翻转：S/D 交换行。
	flipped := [CraftingGridSlots]ItemID{
		ItemNone, ItemNone, ItemNone,
		ItemNone, ItemDirt, ItemGlass,
		ItemNone, ItemStone, ItemNone,
	}
	// 180° 旋转 = 水平镜像 + 垂直翻转。
	rotated := [CraftingGridSlots]ItemID{
		ItemNone, ItemNone, ItemNone,
		ItemNone, ItemNone, ItemDirt,
		ItemNone, ItemGlass, ItemStone,
	}

	const originX, originY = 1, 1
	if !matchesPattern(3, forward, asymmetricPattern, originX, originY, false) {
		t.Fatal("正向摆放未匹配：匹配器连基准情形都失败")
	}
	if !matchesPattern(3, mirrored, asymmetricPattern, originX, originY, true) {
		t.Fatal("水平镜像未命中：镜像位开启的形状必须接受镜像摆放")
	}
	if matchesPattern(3, mirrored, asymmetricPattern, originX, originY, false) {
		t.Fatal("镜像摆放命中了正向比较：镜像重试分支与正向分支不可互换")
	}
	for name, cells := range map[string][CraftingGridSlots]ItemID{
		"垂直翻转":   flipped,
		"180°旋转": rotated,
	} {
		for _, mirror := range []bool{false, true} {
			if matchesPattern(3, cells, asymmetricPattern, originX, originY, mirror) {
				t.Fatalf("%s 在 mirror=%v 时命中：翻转与旋转永不参与匹配", name, mirror)
			}
		}
	}
}

// TestMatchesPatternHonorsMirrorSwitchOff 锁定 `Mirror` 位语义的另一半：
// `matchesPattern` 是纯几何比较器（不读 `Mirror`，重试门控在调用方），因此
// 镜像摆在正向比较下必须与形状逐格不符——配合调用方「未开镜像位就不重试」
// 的门控（注册表级由 recipe_test.go 的石锄镜像用例证明），镜像位关闭的形状
// 不可能命中镜像摆放。
func TestMatchesPatternHonorsMirrorSwitchOff(t *testing.T) {
	strict := asymmetricPattern
	strict.Mirror = false
	mirrored := [CraftingGridSlots]ItemID{
		ItemGlass, ItemStone, ItemNone,
		ItemNone, ItemDirt, ItemNone,
		ItemNone, ItemNone, ItemNone,
	}
	forward := [CraftingGridSlots]ItemID{
		ItemStone, ItemGlass, ItemNone,
		ItemDirt, ItemNone, ItemNone,
		ItemNone, ItemNone, ItemNone,
	}
	if matchesPattern(3, mirrored, strict, 0, 0, false) {
		t.Fatal("镜像摆在正向比较下命中了：镜像摆放与原形状逐格不符")
	}
	if !matchesPattern(3, forward, strict, 0, 0, false) {
		t.Fatal("夹具自检失败：正向摆放必须命中正向比较")
	}
}

// TestTrimPatternBoundsBoundingBox 锁定裁边归一化的包围盒计算：中波单格
// （origin 1,1、1×1）、2×2 的 L 形（2×2 包围盒）与全空网格（ok=false）。
func TestTrimPatternBoundsBoundingBox(t *testing.T) {
	originX, originY, width, height, ok := trimPattern(3, [CraftingGridSlots]ItemID{
		ItemNone, ItemNone, ItemNone,
		ItemNone, ItemStone, ItemNone,
		ItemNone, ItemNone, ItemNone,
	})
	if !ok || originX != 1 || originY != 1 || width != 1 || height != 1 {
		t.Fatalf("中波单格裁边 = (%d,%d,%d×%d,%v)，想要 (1,1,1×1)", originX, originY, width, height, ok)
	}

	// 个人 2×2 网格按 stride 2 解释：格 0,1,2 有物（L 形）时包围盒是满 2×2。
	originX, originY, width, height, ok = trimPattern(2, [CraftingGridSlots]ItemID{
		ItemStone, ItemGlass, ItemNone,
		ItemDirt, ItemNone, ItemNone,
		ItemNone, ItemNone, ItemNone,
	})
	if !ok || originX != 0 || originY != 0 || width != 2 || height != 2 {
		t.Fatalf("2×2 L 形裁边 = (%d,%d,%d×%d,%v)，想要 (0,0,2×2)", originX, originY, width, height, ok)
	}

	if _, _, _, _, ok := trimPattern(3, [CraftingGridSlots]ItemID{}); ok {
		t.Fatal("全空网格的裁边必须失败")
	}
}

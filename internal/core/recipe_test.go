package core_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func TestStoneBrickIDsStayStable(t *testing.T) {
	if core.StoneBrickID != core.BedrockID+1 {
		t.Fatalf("StoneBrickID = %d，必须追加在既有方块 ID 之后", core.StoneBrickID)
	}
	if core.ItemStoneBrick != core.ItemGrass+1 {
		t.Fatalf("ItemStoneBrick = %d，必须追加在既有物品 ID 之后", core.ItemStoneBrick)
	}
	if core.RecipeStoneBricks != 1 {
		t.Fatalf("RecipeStoneBricks = %d，契约要求 1", core.RecipeStoneBricks)
	}
}

func TestStoneBrickIsPlaceableAndDrops(t *testing.T) {
	block, ok := core.ItemPlacement(core.ItemStoneBrick)
	if !ok || block != core.StoneBrickID {
		t.Fatalf("ItemPlacement(石砖) = (%d, %v)，想要 (%d, true)", block, ok, core.StoneBrickID)
	}
	item, ok := core.BlockDrop(core.StoneBrickID)
	if !ok || item != core.ItemStoneBrick {
		t.Fatalf("BlockDrop(石砖) = (%d, %v)，想要 (%d, true)", item, ok, core.ItemStoneBrick)
	}
}

func TestM4EResourceIDsAreStable(t *testing.T) {
	if core.CoalOreID != 7 || core.IronOreID != 8 ||
		core.FurnaceID != 9 || core.IronBlockID != 10 {
		t.Fatal("M4E 方块 ID 漂移")
	}
	if core.ItemCoal != 5 || core.ItemRawIron != 6 || core.ItemIronIngot != 7 ||
		core.ItemFurnace != 8 || core.ItemIronBlock != 9 {
		t.Fatal("M4E 物品 ID 漂移")
	}
}

func TestRegisteredItemSeparatesValidityFromPlacement(t *testing.T) {
	registered := []core.ItemID{
		core.ItemStone, core.ItemDirt, core.ItemGrass, core.ItemStoneBrick,
		core.ItemCoal, core.ItemRawIron, core.ItemIronIngot,
		core.ItemFurnace, core.ItemIronBlock,
	}
	for _, item := range registered {
		if !core.RegisteredItem(item) {
			t.Fatalf("物品 %d 未被登记为合法", item)
		}
	}
	if core.RegisteredItem(core.ItemNone) || core.RegisteredItem(core.ItemID(4242)) {
		t.Fatal("空物品或未知物品被登记为合法")
	}
	// 煤炭、粗铁、铁锭合法但没有放置映射。
	for _, item := range []core.ItemID{core.ItemCoal, core.ItemRawIron, core.ItemIronIngot} {
		if _, ok := core.ItemPlacement(item); ok {
			t.Fatalf("物品 %d 不应可放置", item)
		}
	}
	for _, tc := range []struct {
		item  core.ItemID
		block core.BlockID
	}{
		{core.ItemFurnace, core.FurnaceID},
		{core.ItemIronBlock, core.IronBlockID},
	} {
		if block, ok := core.ItemPlacement(tc.item); !ok || block != tc.block {
			t.Fatalf("物品 %d 放置映射 = (%d, %v)", tc.item, block, ok)
		}
	}
}

func TestM4EBlockDrops(t *testing.T) {
	for _, tc := range []struct {
		block core.BlockID
		item  core.ItemID
	}{
		{core.CoalOreID, core.ItemCoal},
		{core.IronOreID, core.ItemRawIron},
		{core.FurnaceID, core.ItemFurnace},
		{core.IronBlockID, core.ItemIronBlock},
	} {
		item, ok := core.BlockDrop(tc.block)
		if !ok || item != tc.item {
			t.Fatalf("方块 %d 掉落 = (%d, %v)，想要 %d", tc.block, item, ok, tc.item)
		}
	}
}

// gridCell 是合成网格测试夹具的一条放置记录：把 item 放进 slot。
type gridCell struct {
	slot uint8
	item core.ItemID
}

// buildCraftingGrid 按「槽位 → 物品」列表铺一个合成网格，每格数量 1，
// 未列出的格保持空。数量 1 已覆盖匹配器的全部语义（匹配只看物品与占位）。
func buildCraftingGrid(cells ...gridCell) [core.CraftingGridSlots]core.ItemStack {
	var grid [core.CraftingGridSlots]core.ItemStack
	for _, cell := range cells {
		grid[cell.slot] = core.ItemStack{Item: cell.item, Count: 1}
	}
	return grid
}

// TestRecipePatternUsesUnifiedNineSlotStorage 锁定统一 9 格存储：个人 2×2 与
// 工作台 3×3 共用同一组格，协议侧的固定 9 格编码依赖这个上界。
func TestRecipePatternUsesUnifiedNineSlotStorage(t *testing.T) {
	if core.CraftingGridSlots != 9 {
		t.Fatalf("CraftingGridSlots = %d，必须统一为 9", core.CraftingGridSlots)
	}
}

// TestMatchCraftingGridNormalizesPlacementPosition 覆盖 spec Requirement
// 「形状匹配裁边且仅允许水平镜像」的裁边归一化：同一 2×2 石头形状摆在 3×3
// 网格的四个角落（各自忽略不同的外围空行列）与个人 2×2 网格的唯一位置，
// 五种摆放都匹配同一条石砖配方。
func TestMatchCraftingGridNormalizesPlacementPosition(t *testing.T) {
	corners := []struct {
		name  string
		size  uint8
		slots [4]uint8
	}{
		{"3×3 左上", 3, [4]uint8{0, 1, 3, 4}},
		{"3×3 右上", 3, [4]uint8{1, 2, 4, 5}},
		{"3×3 左下", 3, [4]uint8{3, 4, 6, 7}},
		{"3×3 右下", 3, [4]uint8{4, 5, 7, 8}},
		{"2×2 唯一位置", 2, [4]uint8{0, 1, 2, 3}},
	}
	for _, corner := range corners {
		t.Run(corner.name, func(t *testing.T) {
			cells := make([]gridCell, 0, 4)
			for _, slot := range corner.slots {
				cells = append(cells, gridCell{slot: slot, item: core.ItemStone})
			}
			id, output, ok := core.MatchCraftingGrid(corner.size, buildCraftingGrid(cells...))
			if !ok || id != core.RecipeStoneBricks {
				t.Fatalf("匹配 = (%d, %v)，想要石砖配方 %d", id, ok, core.RecipeStoneBricks)
			}
			if output != (core.ItemStack{Item: core.ItemStoneBrick, Count: 4}) {
				t.Fatalf("产物 = %+v，想要 4 个石砖", output)
			}
		})
	}
}

// TestMatchCraftingGridPreservesInteriorHole 锁定「内部空洞保留」：熔炉配方是
// 3×3 圆石圆环、中格为空；把中格放上任何物品（同为圆石或无关的铁锭）都必须
// 失去匹配——裁边只裁外围空行列，不吞掉形状内部的空洞。
func TestMatchCraftingGridPreservesInteriorHole(t *testing.T) {
	ring := []uint8{0, 1, 2, 3, 5, 6, 7, 8}
	build := func(center core.ItemID) [core.CraftingGridSlots]core.ItemStack {
		cells := make([]gridCell, 0, 9)
		for _, slot := range ring {
			cells = append(cells, gridCell{slot: slot, item: core.ItemCobblestone})
		}
		if center != core.ItemNone {
			cells = append(cells, gridCell{slot: 4, item: center})
		}
		return buildCraftingGrid(cells...)
	}

	id, _, ok := core.MatchCraftingGrid(3, build(core.ItemNone))
	if !ok || id != core.RecipeFurnace {
		t.Fatalf("中空圆环匹配 = (%d, %v)，想要熔炉配方 %d", id, ok, core.RecipeFurnace)
	}
	for _, center := range []core.ItemID{core.ItemCobblestone, core.ItemIronIngot} {
		if _, _, ok := core.MatchCraftingGrid(3, build(center)); ok {
			t.Fatalf("中心被 %d 填充后仍匹配：内部空洞必须保留", center)
		}
	}
}

// TestMatchCraftingGridRejectsExtraItems 锁定「网格内存在配方形状之外的额外
// 物品时不匹配」：三连小麦的下一行多一格小麦，裁边包围盒从 1×3 变 3×2，
// 面包配方的宽高不再相等，任何配方都不得匹配。
func TestMatchCraftingGridRejectsExtraItems(t *testing.T) {
	row := buildCraftingGrid(
		gridCell{3, core.ItemWheat}, gridCell{4, core.ItemWheat}, gridCell{5, core.ItemWheat},
	)
	if id, _, ok := core.MatchCraftingGrid(3, row); !ok || id != core.RecipeBread {
		t.Fatalf("横排三小麦匹配 = (%d, %v)，想要面包配方 %d", id, ok, core.RecipeBread)
	}
	withExtra := row
	withExtra[6] = core.ItemStack{Item: core.ItemWheat, Count: 1}
	if _, _, ok := core.MatchCraftingGrid(3, withExtra); ok {
		t.Fatal("形状外多放一个物品仍产生匹配")
	}
}

// TestMatchCraftingGridEmptyGridHasNoMatch 锁定「空网格 MUST 无产物」：
// 两种有效尺寸下的全空网格都不匹配任何配方。
func TestMatchCraftingGridEmptyGridHasNoMatch(t *testing.T) {
	for _, size := range []uint8{2, 3} {
		if id, output, ok := core.MatchCraftingGrid(size, [core.CraftingGridSlots]core.ItemStack{}); ok || id != 0 || output != (core.ItemStack{}) {
			t.Fatalf("尺寸 %d 的空网格产生了匹配: (%d, %+v, %v)", size, id, output, ok)
		}
	}
}

// TestMatchCraftingGridRejectsInvalidSize 锁定防御分支一：size 只允许 2 或 3。
// 正常权威路径不会构造别的尺寸，但匹配器是纯函数，任何越界输入都必须无匹配
// 而不是按错误的 stride 解读格布局（尺寸 4 会把格 9 读出数组界外，尺寸 0/1
// 会把 2×2 形状错解）。对照组：同一网格在有效尺寸下匹配石砖配方。
func TestMatchCraftingGridRejectsInvalidSize(t *testing.T) {
	grid := buildCraftingGrid(
		gridCell{0, core.ItemStone}, gridCell{1, core.ItemStone},
		gridCell{2, core.ItemStone}, gridCell{3, core.ItemStone},
	)
	if id, _, ok := core.MatchCraftingGrid(2, grid); !ok || id != core.RecipeStoneBricks {
		t.Fatalf("尺寸 2 的 2×2 石头匹配 = (%d, %v)，想要石砖配方（对照组）", id, ok)
	}
	for _, size := range []uint8{0, 1, 4, 255} {
		if id, output, ok := core.MatchCraftingGrid(size, grid); ok || id != 0 || output != (core.ItemStack{}) {
			t.Fatalf("非法尺寸 %d 产生了匹配: (%d, %+v, %v)", size, id, output, ok)
		}
	}
}

// TestMatchCraftingGridRejectsResidueBeyondEffectiveSize 锁定防御分支二：
// 个人网格（尺寸 2）的格 4..8 残留任何物品时一律无匹配。权威移动路径保证
// 缩容前先回收扩展格，这里是防御层——若残留未被回收（例如未来的生命周期
// 缺口），旧内容绝不允许靠 3×3 布局继续匹配。
func TestMatchCraftingGridRejectsResidueBeyondEffectiveSize(t *testing.T) {
	personal := buildCraftingGrid(
		gridCell{0, core.ItemStone}, gridCell{1, core.ItemStone},
		gridCell{2, core.ItemStone}, gridCell{3, core.ItemStone},
	)
	if _, _, ok := core.MatchCraftingGrid(2, personal); !ok {
		t.Fatal("对照组失败：干净的 2×2 石头必须匹配石砖配方")
	}
	for _, slot := range []uint8{4, 5, 6, 7, 8} {
		withResidue := personal
		withResidue[slot] = core.ItemStack{Item: core.ItemStone, Count: 1}
		if _, _, ok := core.MatchCraftingGrid(2, withResidue); ok {
			t.Fatalf("个人网格格 %d 残留物品仍产生匹配：扩展格内容必须被无视匹配拒绝", slot)
		}
	}
}

// TestMatchCraftingGridPersonalGridCannotMatchFullSizeRecipes 锁定 spec
// Scenario「2×2 网格不能匹配 3×3 配方」：个人网格的四个合法格按 2×2 行主序
// 解释，摆不下任何宽或高为 3 的形状——同样的三个格（0,1,2）在 3×3 下是横排
// 三连、匹配面包，在 2×2 下是 L 形、不匹配任何配方。
func TestMatchCraftingGridPersonalGridCannotMatchFullSizeRecipes(t *testing.T) {
	grid := buildCraftingGrid(
		gridCell{0, core.ItemWheat}, gridCell{1, core.ItemWheat}, gridCell{2, core.ItemWheat},
	)
	if id, _, ok := core.MatchCraftingGrid(3, grid); !ok || id != core.RecipeBread {
		t.Fatalf("3×3 下的三连小麦匹配 = (%d, %v)，想要面包配方", id, ok)
	}
	if _, _, ok := core.MatchCraftingGrid(2, grid); ok {
		t.Fatal("个人 2×2 网格匹配了宽为 3 的配方")
	}
	// 防空转：个人网格摆满四个小麦（2×2 满形）也必须无匹配——2×2 小麦不是
	// 任何配方，这证明上面的拒绝不只是「格子数不够」。
	full := buildCraftingGrid(
		gridCell{0, core.ItemWheat}, gridCell{1, core.ItemWheat},
		gridCell{2, core.ItemWheat}, gridCell{3, core.ItemWheat},
	)
	if _, _, ok := core.MatchCraftingGrid(2, full); ok {
		t.Fatal("2×2 小麦满形产生了匹配")
	}
}

// TestMatchCraftingGridToolShapesRejectMirrorAndFlip 覆盖 spec Requirement
// 「形状匹配裁边且仅允许水平镜像」的镜像纪律：工具类配方按 design.md D3 关闭
// 镜像位（避免左右手双解），因此石锄的镜像摆放必须失败；垂直翻转与旋转从来
// 不在允许之列，石镐的垂直翻转必须失败。正向摆放作为两条拒绝的对照组。
func TestMatchCraftingGridToolShapesRejectMirrorAndFlip(t *testing.T) {
	hoe := buildCraftingGrid(
		gridCell{0, core.ItemStone}, gridCell{3, core.ItemStone},
		gridCell{1, core.ItemStick}, gridCell{4, core.ItemStick},
	)
	if id, _, ok := core.MatchCraftingGrid(3, hoe); !ok || id != core.RecipeStoneHoe {
		t.Fatalf("石锄正向摆放匹配 = (%d, %v)，想要 %d", id, ok, core.RecipeStoneHoe)
	}
	mirroredHoe := buildCraftingGrid(
		gridCell{0, core.ItemStick}, gridCell{3, core.ItemStick},
		gridCell{1, core.ItemStone}, gridCell{4, core.ItemStone},
	)
	if _, _, ok := core.MatchCraftingGrid(3, mirroredHoe); ok {
		t.Fatal("石锄的镜像摆放产生了匹配：工具配方的镜像位必须关闭")
	}

	pickaxe := buildCraftingGrid(
		gridCell{0, core.ItemStone}, gridCell{1, core.ItemStone}, gridCell{2, core.ItemStone},
		gridCell{4, core.ItemStick}, gridCell{7, core.ItemStick},
	)
	if id, _, ok := core.MatchCraftingGrid(3, pickaxe); !ok || id != core.RecipeStonePickaxe {
		t.Fatalf("石镐正向摆放匹配 = (%d, %v)，想要 %d", id, ok, core.RecipeStonePickaxe)
	}
	flippedPickaxe := buildCraftingGrid(
		gridCell{6, core.ItemStone}, gridCell{7, core.ItemStone}, gridCell{8, core.ItemStone},
		gridCell{1, core.ItemStick}, gridCell{4, core.ItemStick},
	)
	if _, _, ok := core.MatchCraftingGrid(3, flippedPickaxe); ok {
		t.Fatal("石镐的垂直翻转产生了匹配：翻转与旋转永不参与匹配")
	}
}

// TestSingleCellRecipeMatchesAnywhere 覆盖 spec Scenario「单格配方的位置无关」：
// 单个橡木原木放在 3×3 的全部九个格位与个人 2×2 的全部四个合法格位，
// 十三种摆放都匹配木板配方。
func TestSingleCellRecipeMatchesAnywhere(t *testing.T) {
	assertMatches := func(t *testing.T, size, slot uint8) {
		t.Helper()
		id, output, ok := core.MatchCraftingGrid(size, buildCraftingGrid(gridCell{slot, core.ItemOakLog}))
		if !ok || id != core.RecipeOakPlanks {
			t.Fatalf("单格原木在格 %d（尺寸 %d）匹配 = (%d, %v)，想要 %d",
				slot, size, id, ok, core.RecipeOakPlanks)
		}
		if output != (core.ItemStack{Item: core.ItemOakPlanks, Count: 4}) {
			t.Fatalf("木板配方产物 = %+v", output)
		}
	}
	for _, slot := range []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		assertMatches(t, 3, slot)
	}
	for _, slot := range []uint8{0, 1, 2, 3} {
		assertMatches(t, 2, slot)
	}
}

// TestRecipeShapeTableOneToThirteenIsFrozen 覆盖 spec Requirement「固定配方具有
// 稳定语义」与 Scenario「既有配方编号不因新增而位移」：逐条冻结 recipe 1..13
// 的编号、形状（裁边宽高 + stride-3 九格物品表）、产物与镜像位。任何一条被
// 挤位、改写形状或翻动镜像位都会变红——配方 ID 是协议稳定值，重排会让客户端
// 已经发出的请求指向别的配方。
//
// Cells 的书写按 3×3 行主序直观排列（每行三个字面量），与 `RecipePattern`
// 的存储布局一一对应；`core.ItemNone` 即形状的空格。
func TestRecipeShapeTableOneToThirteenIsFrozen(t *testing.T) {
	frozen := []struct {
		id      core.RecipeID
		pattern core.RecipePattern
	}{
		{1, core.RecipePattern{Width: 2, Height: 2, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemStone, core.ItemStone, core.ItemNone,
				core.ItemStone, core.ItemStone, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemStoneBrick, Count: 4}}},
		{2, core.RecipePattern{Width: 3, Height: 3, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemCobblestone, core.ItemCobblestone, core.ItemCobblestone,
				core.ItemCobblestone, core.ItemNone, core.ItemCobblestone,
				core.ItemCobblestone, core.ItemCobblestone, core.ItemCobblestone,
			},
			Output: core.ItemStack{Item: core.ItemFurnace, Count: 1}}},
		{3, core.RecipePattern{Width: 3, Height: 3, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemIronIngot, core.ItemIronIngot, core.ItemIronIngot,
				core.ItemIronIngot, core.ItemIronIngot, core.ItemIronIngot,
				core.ItemIronIngot, core.ItemIronIngot, core.ItemIronIngot,
			},
			Output: core.ItemStack{Item: core.ItemIronBlock, Count: 1}}},
		{4, core.RecipePattern{Width: 3, Height: 3, Mirror: false,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemStone, core.ItemStone, core.ItemStone,
				core.ItemNone, core.ItemStick, core.ItemNone,
				core.ItemNone, core.ItemStick, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 131}}},
		{5, core.RecipePattern{Width: 3, Height: 3, Mirror: false,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemIronIngot, core.ItemIronIngot, core.ItemIronIngot,
				core.ItemNone, core.ItemStick, core.ItemNone,
				core.ItemNone, core.ItemStick, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 250}}},
		{6, core.RecipePattern{Width: 3, Height: 3, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemOakPlanks, core.ItemOakPlanks, core.ItemOakPlanks,
				core.ItemOakPlanks, core.ItemNone, core.ItemOakPlanks,
				core.ItemOakPlanks, core.ItemOakPlanks, core.ItemOakPlanks,
			},
			Output: core.ItemStack{Item: core.ItemChest, Count: 1}}},
		{7, core.RecipePattern{Width: 1, Height: 1, Mirror: true,
			Cells:  [core.CraftingGridSlots]core.ItemID{core.ItemOakLog},
			Output: core.ItemStack{Item: core.ItemOakPlanks, Count: 4}}},
		{8, core.RecipePattern{Width: 2, Height: 2, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemGlass, core.ItemGlass, core.ItemNone,
				core.ItemGlass, core.ItemGlass, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemLightBlock, Count: 4}}},
		{9, core.RecipePattern{Width: 2, Height: 2, Mirror: false,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemStone, core.ItemStick, core.ItemNone,
				core.ItemStone, core.ItemStick, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: 131}}},
		{10, core.RecipePattern{Width: 2, Height: 2, Mirror: false,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemIronIngot, core.ItemStick, core.ItemNone,
				core.ItemIronIngot, core.ItemStick, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemIronHoe, Count: 1, Durability: 250}}},
		{11, core.RecipePattern{Width: 3, Height: 1, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemWheat, core.ItemWheat, core.ItemWheat,
			},
			Output: core.ItemStack{Item: core.ItemBread, Count: 1}}},
		// 木棍：纵向两块木板（1×2）。它连同 `ItemStick` 一起，是镐与锄形状的
		// 前置配方——先有木棍才谈得上任何工具。
		{12, core.RecipePattern{Width: 1, Height: 2, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemOakPlanks, core.ItemNone, core.ItemNone,
				core.ItemOakPlanks, core.ItemNone, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemStick, Count: 4}}},
		// 工作台：2×2 木板。它把个人 2×2 网格升到 3×3 的能力与配方表闭环。
		{13, core.RecipePattern{Width: 2, Height: 2, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemOakPlanks, core.ItemOakPlanks, core.ItemNone,
				core.ItemOakPlanks, core.ItemOakPlanks, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemWorkbench, Count: 1}}},
		{14, core.RecipePattern{Width: 2, Height: 3, Mirror: true,
			Cells: [core.CraftingGridSlots]core.ItemID{
				core.ItemOakPlanks, core.ItemOakPlanks, core.ItemNone,
				core.ItemOakPlanks, core.ItemOakPlanks, core.ItemNone,
				core.ItemOakPlanks, core.ItemOakPlanks, core.ItemNone,
			},
			Output: core.ItemStack{Item: core.ItemDoor, Count: 3}}},
	}
	for _, tc := range frozen {
		pattern, ok := core.Recipe(tc.id)
		if !ok || pattern != tc.pattern {
			t.Fatalf("recipe %d = %+v, %v，想要 %+v", tc.id, pattern, ok, tc.pattern)
		}
		if !pattern.Output.Valid() {
			t.Fatalf("recipe %d 产物 %+v 不是合法物品栈", tc.id, pattern.Output)
		}
	}
	// 编号位次一并冻结：1..14 连续且新末项紧随面包配方（11）之后。
	if core.RecipeStick != core.RecipeBread+1 || core.RecipeWorkbench != core.RecipeStick+1 || core.RecipeDoor != core.RecipeWorkbench+1 {
		t.Fatalf("新配方位次 = stick %d / workbench %d / door %d，必须紧随 RecipeBread(%d) 连续追加",
			core.RecipeStick, core.RecipeWorkbench, core.RecipeDoor, core.RecipeBread)
	}
}

// TestRecipeRejectsUnknownIDs 覆盖 spec Scenario「未登记配方被拒绝」：
// recipe 0、批次其余功能线尚未合流的 `15..18`（火把与三把剑、白床）以及
// 任意更大编号都必须稳定拒绝且不产生产物。写成 `RecipeDoor+1` 起步而
// 不是裸字面量，下次追加配方时这段循环自动跟着末项走。
func TestRecipeRejectsUnknownIDs(t *testing.T) {
	unknown := []core.RecipeID{0}
	for id := core.RecipeDoor + 1; id <= core.RecipeDoor+5; id++ {
		unknown = append(unknown, id)
	}
	unknown = append(unknown, 200, 255)
	for _, id := range unknown {
		if pattern, ok := core.Recipe(id); ok {
			t.Fatalf("recipe %d 被接受为 %+v：表末之后的编号必须稳定拒绝", id, pattern)
		}
	}
	// 15..18 是批次计划里的既定编号段（火把/剑/床，归 A-02/A-03/A-05）：
	// 在它们合流之前逐个点名拒绝，比「表末 +1」更能钉住「暂缺但已规划」。
	for id := core.RecipeID(15); id <= 18; id++ {
		if _, ok := core.Recipe(id); ok {
			t.Fatalf("规划中的 recipe %d 在合流前被注册", id)
		}
	}
}

// TestRegisteredRecipeCellsStayInsideShapeBounds 是通用注册表不变量：对全部
// 已注册配方穷举断言——宽高落在 1..3、形状子矩形内至少有一个非空格、
// Width×Height 子矩形之外的格恒为 `ItemNone`。第三条是匹配器的隐含前提：
// `matchesPattern` 只比较子矩形内的格，若形状把材料写到子矩形外，那格会被
// 静默忽略、配方从此少一份原料也照常匹配。本用例不冻结具体形状（那是
// `TestRecipeShapeTableOneToThirteenIsFrozen` 的职责），只守结构不变量，
// 未来追加 recipe 14..18 时自动生效；编号连续性断言同时钉住「注册表无空洞」。
func TestRegisteredRecipeCellsStayInsideShapeBounds(t *testing.T) {
	checked := 0
	for id := core.RecipeID(1); ; id++ {
		pattern, ok := core.Recipe(id)
		if !ok {
			break
		}
		if pattern.Width < 1 || pattern.Width > 3 || pattern.Height < 1 || pattern.Height > 3 {
			t.Fatalf("recipe %d 宽高 = %d×%d，必须落在 1..3", id, pattern.Width, pattern.Height)
		}
		materials := 0
		for y := uint8(0); y < 3; y++ {
			for x := uint8(0); x < 3; x++ {
				inside := x < pattern.Width && y < pattern.Height
				cell := pattern.Cells[y*3+x]
				if !inside && cell != core.ItemNone {
					t.Fatalf("recipe %d 的格 (%d,%d)=%d 越出 %d×%d 子矩形：匹配器会静默忽略它",
						id, x, y, cell, pattern.Width, pattern.Height)
				}
				if inside && cell != core.ItemNone {
					materials++
				}
			}
		}
		if materials == 0 {
			t.Fatalf("recipe %d 的形状没有任何材料格", id)
		}
		checked++
	}
	// 注册表从 1 起无空洞连续注册到末项常量：循环按「首个未注册即停」推进，
	// 中间留洞会让后面的配方全部漏检，这里用计数把洞钉出来。
	if checked != int(core.RecipeDoor) {
		t.Fatalf("注册表枚举到 %d 条，想要与末项常量一致的 %d 条（注册表出现空洞？）",
			checked, core.RecipeDoor)
	}
}

// TestPickaxeRecipesOutputFullDurability 锁定工具配方的产物是满耐久的合法
// 工具栈——产物耐久直接写在配方表里（字面量来源，不反查函数）。
func TestPickaxeRecipesOutputFullDurability(t *testing.T) {
	for _, test := range []struct {
		id   core.RecipeID
		want core.ItemStack
	}{
		{core.RecipeStonePickaxe, core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 131}},
		{core.RecipeIronPickaxe, core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 250}},
		{core.RecipeStoneHoe, core.ItemStack{Item: core.ItemStoneHoe, Count: 1, Durability: 131}},
		{core.RecipeIronHoe, core.ItemStack{Item: core.ItemIronHoe, Count: 1, Durability: 250}},
	} {
		pattern, ok := core.Recipe(test.id)
		if !ok || pattern.Output != test.want || !pattern.Output.Valid() {
			t.Fatalf("Recipe(%d) 产物 = %+v, %v，想要合法满耐久工具 %+v",
				test.id, pattern.Output, ok, test.want)
		}
	}
}

// TestNonToolRecipesOutputZeroDurability 锁定非工具产物的耐久必须为零：
// 耐久字段对没有耐久概念的物品必须是零值，否则同物品的两个栈会因无意义
// 字段拒绝合并。
func TestNonToolRecipesOutputZeroDurability(t *testing.T) {
	for _, id := range []core.RecipeID{
		core.RecipeStoneBricks,
		core.RecipeFurnace,
		core.RecipeIronBlock,
		core.RecipeChest,
		core.RecipeOakPlanks,
		core.RecipeLightBlock,
		core.RecipeBread,
		core.RecipeStick,
		core.RecipeWorkbench,
	} {
		pattern, ok := core.Recipe(id)
		if !ok || pattern.Output.Durability != 0 {
			t.Fatalf("Recipe(%d) 产物 = %+v, %v，非工具耐久必须为 0", id, pattern.Output, ok)
		}
	}
}

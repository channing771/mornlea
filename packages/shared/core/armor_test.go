package core_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestIronArmorItemsAreRegisteredWithFixedSemantics 锁定四件铁质护甲的稳定
// 编号与物品语义：堆叠上限 1、不经 `ItemPlacement` 放置、各自登记耐久上限
// （数值由 armor 域单一真源经 `ItemMaxDurability` 呈现）、拥有唯一中文显示名
// 与 machine name。护甲不可放置、不是任何方块的掉落物，唯一来源是护甲配方。
func TestIronArmorItemsAreRegisteredWithFixedSemantics(t *testing.T) {
	tests := []struct {
		name       string
		item       core.ItemID
		id         core.ItemID
		durability uint16
		display    string
	}{
		{"铁头盔", core.ItemIronHelmet, 58, 165, "铁头盔"},
		{"铁胸甲", core.ItemIronChestplate, 59, 240, "铁胸甲"},
		{"铁护腿", core.ItemIronLeggings, 60, 225, "铁护腿"},
		{"铁靴子", core.ItemIronBoots, 61, 195, "铁靴子"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.item != test.id {
				t.Fatalf("ItemID = %d，想要 %d", test.item, test.id)
			}
			if !core.RegisteredItem(test.item) {
				t.Fatalf("物品 %d 未注册", test.item)
			}
			if limit, ok := core.ItemStackLimit(test.item); !ok || limit != 1 {
				t.Fatalf("ItemStackLimit(%d) = (%d,%v)，想要 (1,true)", test.item, limit, ok)
			}
			if block, ok := core.ItemPlacement(test.item); ok {
				t.Fatalf("ItemPlacement(护甲 %d) = (%d,%v)，护甲不经 ItemPlacement 放置", test.item, block, ok)
			}
			gotDurability, hasDurability := core.ItemMaxDurability(test.item)
			if !hasDurability || gotDurability != test.durability {
				t.Fatalf("ItemMaxDurability(%d) = (%d,%v)，想要 (%d,true)",
					test.item, gotDurability, hasDurability, test.durability)
			}
			if display, ok := core.ItemDisplayName(test.item); !ok || display != test.display {
				t.Fatalf("ItemDisplayName(%d) = (%q,%v)，想要 (%q,true)", test.item, display, ok, test.display)
			}
			if machineName, ok := core.CanonicalItemName(test.item); !ok || machineName == "" {
				t.Fatalf("CanonicalItemName(%d) = (%q,%v)，护甲必须有 machine name", test.item, machineName, ok)
			}
			full := core.ItemStack{Item: test.item, Count: 1, Durability: test.durability}
			if !full.Valid() {
				t.Fatalf("满耐久护甲栈 %+v 应当有效", full)
			}
			if (core.ItemStack{Item: test.item, Count: 2, Durability: test.durability}).Valid() {
				t.Fatalf("护甲 %d 堆叠上限为 1，两件一栈必须无效", test.item)
			}
		})
	}
	if core.ItemIDMax != 62 {
		t.Fatalf("ItemIDMax = %d，护甲四件追加后必须为 62", core.ItemIDMax)
	}
}

// TestNoBlockDropsArmorPieces 穷举守护「护甲不出现在任何 BlockDrop 表」：
// 世界上没有任何方块采掘出护甲，唯一来源是护甲配方。
func TestNoBlockDropsArmorPieces(t *testing.T) {
	armor := map[core.ItemID]bool{
		core.ItemIronHelmet:     true,
		core.ItemIronChestplate: true,
		core.ItemIronLeggings:   true,
		core.ItemIronBoots:      true,
	}
	for block := core.BlockID(0); block < core.BlockIDMax; block++ {
		if item, ok := core.BlockDrop(block); ok && armor[item] {
			t.Fatalf("方块 %d 掉落护甲 %d：护甲唯一来源是配方", block, item)
		}
	}
}

// TestArmorPiecesMapToUniqueSlots 锁定件→槽唯一映射：四件各归一槽、互不重叠，
// 非护甲物品没有槽位。装备动作的目标槽位由本映射唯一确定。
func TestArmorPiecesMapToUniqueSlots(t *testing.T) {
	tests := []struct {
		item core.ItemID
		want core.ArmorSlot
	}{
		{core.ItemIronHelmet, core.ArmorSlotHead},
		{core.ItemIronChestplate, core.ArmorSlotChest},
		{core.ItemIronLeggings, core.ArmorSlotLegs},
		{core.ItemIronBoots, core.ArmorSlotFeet},
	}
	seen := make(map[core.ArmorSlot]core.ItemID, len(tests))
	for _, test := range tests {
		slot, ok := core.ArmorSlotOf(test.item)
		if !ok || slot != test.want {
			t.Fatalf("ArmorSlotOf(%d) = (%d,%v)，想要 (%d,true)", test.item, slot, ok, test.want)
		}
		if previous, exists := seen[slot]; exists {
			t.Fatalf("槽位 %d 被物品 %d 与 %d 同时占用：件→槽映射必须唯一", slot, previous, test.item)
		}
		seen[slot] = test.item
	}
	if core.ArmorSlotCount != 4 {
		t.Fatalf("ArmorSlotCount = %d，契约要求头/胸/腿/脚四槽", core.ArmorSlotCount)
	}
	for _, item := range []core.ItemID{core.ItemNone, core.ItemIronIngot, core.ItemStoneSword, core.ItemIDMax} {
		if slot, ok := core.ArmorSlotOf(item); ok {
			t.Fatalf("非护甲物品 %d 被映射到槽位 %d", item, slot)
		}
	}
}

// TestArmorPointsSumsIntactPiecesAndClamps 锁定穿戴点数求和：只累计完好件，
// 损坏形态（耐久归零）与空槽贡献 0 点，总和截断到 `MaxArmorPoints`。
func TestArmorPointsSumsIntactPiecesAndClamps(t *testing.T) {
	helmet := core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}
	chest := core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: 240}
	legs := core.ItemStack{Item: core.ItemIronLeggings, Count: 1, Durability: 225}
	boots := core.ItemStack{Item: core.ItemIronBoots, Count: 1, Durability: 195}
	// 损坏形态沿既有耐久机制的「数量 1、耐久 0」表达：耐久归零的护甲件保留
	// 在槽内、不消失（与损坏剑同族），贡献 0 点。
	brokenLegs := core.ItemStack{Item: core.ItemIronLeggings, Count: 1, Durability: 0}
	brokenBoots := core.ItemStack{Item: core.ItemIronBoots, Count: 1, Durability: 0}

	cases := []struct {
		name string
		worn [core.ArmorSlotCount]core.ItemStack
		want uint8
	}{
		{"全空", [core.ArmorSlotCount]core.ItemStack{}, 0},
		{"满套完好", [core.ArmorSlotCount]core.ItemStack{helmet, chest, legs, boots}, 15},
		{"头胸完好腿脚损坏", [core.ArmorSlotCount]core.ItemStack{helmet, chest, brokenLegs, brokenBoots}, 8},
		{"单件头盔", [core.ArmorSlotCount]core.ItemStack{helmet}, 2},
		{"耐久剩 1 仍完好", [core.ArmorSlotCount]core.ItemStack{{Item: core.ItemIronHelmet, Count: 1, Durability: 1}}, 2},
		{"耐久越界不算完好", [core.ArmorSlotCount]core.ItemStack{{Item: core.ItemIronHelmet, Count: 1, Durability: 166}}, 0},
		{"零数量栈不持有护甲", [core.ArmorSlotCount]core.ItemStack{{Item: core.ItemIronHelmet, Count: 0, Durability: 165}}, 0},
		{"非护甲物品不贡献", [core.ArmorSlotCount]core.ItemStack{{Item: core.ItemIronIngot, Count: 1}}, 0},
	}
	for _, tc := range cases {
		if got := core.ArmorPoints(tc.worn); got != tc.want {
			t.Fatalf("%s：ArmorPoints = %d，想要 %d", tc.name, got, tc.want)
		}
	}
	// 截断不变量：铁质满套 15 点不会触顶；用四件胸甲（各 6 点、合计 24）构造
	// 越界输入，钉住点数永远落在 0..MaxArmorPoints。
	over := [core.ArmorSlotCount]core.ItemStack{chest, chest, chest, chest}
	if got := core.ArmorPoints(over); got != core.MaxArmorPoints {
		t.Fatalf("四胸甲求和 = %d，必须截断到 MaxArmorPoints(%d)", got, core.MaxArmorPoints)
	}
	if core.MaxArmorPoints != 20 {
		t.Fatalf("MaxArmorPoints = %d，契约要求 20", core.MaxArmorPoints)
	}
}

// TestReducedDamageFormulaVectors 锁定减免公式
// `max(1, damage×(100−4×points)/100)` 的整数语义：每点护甲减 4% 伤害、
// 乘除为整数运算且除法向零取整、有效伤害下限 1。damage MUST 为正，
// 非正输入没有游戏语义，防御性返回 0。
func TestReducedDamageFormulaVectors(t *testing.T) {
	cases := []struct {
		damage int32
		points uint8
		want   int32
	}{
		{3, 0, 3},     // 无护甲：原样返回
		{3, 15, 1},    // 3×40/100 = 1（取整后 max 1）
		{3, 20, 1},    // 3×20/100 = 0 → 下限 1
		{4, 15, 1},    // 满套铁甲挨木剑：4×40/100 = 1（1.6 向零取整）
		{5, 15, 2},    // 5×40/100 = 2（整除，不进位）
		{199, 1, 191}, // 199×96/100 = 191（191.04 向零取整）
	}
	for _, tc := range cases {
		if got := core.ReducedDamage(tc.damage, tc.points); got != tc.want {
			t.Fatalf("ReducedDamage(%d, %d) = %d，想要 %d", tc.damage, tc.points, got, tc.want)
		}
	}
	// 1 点伤害在任何点数组合下都不得减到 0。
	for points := uint8(0); points <= core.MaxArmorPoints; points++ {
		if got := core.ReducedDamage(1, points); got != 1 {
			t.Fatalf("ReducedDamage(1, %d) = %d，任何点数组合的有效伤害下限都是 1", points, got)
		}
	}
	// 点数到达 25 时折扣恰为 100%，超过后折扣为负，公式同样收敛到下限 1。
	if got := core.ReducedDamage(1000, 25); got != 1 {
		t.Fatalf("ReducedDamage(1000, 25) = %d，想要下限 1", got)
	}
	// 非正伤害防御性返回 0，由调用方兜底。
	for _, damage := range []int32{0, -1, -1000} {
		if got := core.ReducedDamage(damage, 15); got != 0 {
			t.Fatalf("ReducedDamage(%d, 15) = %d，非正伤害必须返回 0", damage, got)
		}
	}
}

// TestArmorRecipesMatchAndOutputFullDurability 锁定四条护甲配方的稳定编号、
// 形状匹配与满耐久产物：3×3 工作台网格按形状摆放铁锭，命中对应配方并取出
// 恰好一件满耐久护甲。3×2 形状（头盔/靴子）在工作台网格内上下平移仍命中，
// 裁边归一化对护甲形状同样成立。
func TestArmorRecipesMatchAndOutputFullDurability(t *testing.T) {
	tests := []struct {
		name   string
		id     core.RecipeID
		number core.RecipeID
		grids  [][core.CraftingGridSlots]core.ItemStack
		output core.ItemStack
	}{
		{"铁头盔", core.RecipeIronHelmet, 21, [][core.CraftingGridSlots]core.ItemStack{
			buildCraftingGrid(
				gridCell{0, core.ItemIronIngot}, gridCell{1, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
				gridCell{3, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
			),
			// 下移一排：裁边后仍是同一形状。
			buildCraftingGrid(
				gridCell{3, core.ItemIronIngot}, gridCell{4, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
				gridCell{6, core.ItemIronIngot}, gridCell{8, core.ItemIronIngot},
			),
		}, core.ItemStack{Item: core.ItemIronHelmet, Count: 1, Durability: 165}},
		{"铁胸甲", core.RecipeIronChestplate, 22, [][core.CraftingGridSlots]core.ItemStack{
			buildCraftingGrid(
				gridCell{0, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
				gridCell{3, core.ItemIronIngot}, gridCell{4, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
				gridCell{6, core.ItemIronIngot}, gridCell{7, core.ItemIronIngot}, gridCell{8, core.ItemIronIngot},
			),
		}, core.ItemStack{Item: core.ItemIronChestplate, Count: 1, Durability: 240}},
		{"铁护腿", core.RecipeIronLeggings, 23, [][core.CraftingGridSlots]core.ItemStack{
			buildCraftingGrid(
				gridCell{0, core.ItemIronIngot}, gridCell{1, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
				gridCell{3, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
				gridCell{6, core.ItemIronIngot}, gridCell{8, core.ItemIronIngot},
			),
		}, core.ItemStack{Item: core.ItemIronLeggings, Count: 1, Durability: 225}},
		{"铁靴子", core.RecipeIronBoots, 24, [][core.CraftingGridSlots]core.ItemStack{
			buildCraftingGrid(
				gridCell{0, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
				gridCell{3, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
			),
			// 占上两排是配方形状；下移到中下两排同样命中（裁边语义）。
			buildCraftingGrid(
				gridCell{3, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
				gridCell{6, core.ItemIronIngot}, gridCell{8, core.ItemIronIngot},
			),
		}, core.ItemStack{Item: core.ItemIronBoots, Count: 1, Durability: 195}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.id != test.number {
				t.Fatalf("RecipeID = %d，想要 %d", test.id, test.number)
			}
			for _, grid := range test.grids {
				id, output, ok := core.MatchCraftingGrid(3, grid)
				if !ok || id != test.id {
					t.Fatalf("匹配 = (%d, %v)，想要 %s配方 %d", id, ok, test.name, test.id)
				}
				if output != test.output {
					t.Fatalf("产物 = %+v，想要满耐久 %+v", output, test.output)
				}
				if !output.Valid() {
					t.Fatalf("产物 %+v 不是合法物品栈", output)
				}
			}
		})
	}
}

// TestArmorRecipesRejectInvalidShapes 锁定形状不匹配不出产物：换料、补空洞、
// 垂直翻转与残缺摆放都不得命中护甲配方；九格全满铁锭仍命中铁块配方而非胸甲
// （内部空洞是形状的一部分）。个人 2×2 网格摆不下任何宽为 3 的护甲形状。
func TestArmorRecipesRejectInvalidShapes(t *testing.T) {
	armorRecipes := map[core.RecipeID]bool{
		core.RecipeIronHelmet: true, core.RecipeIronChestplate: true,
		core.RecipeIronLeggings: true, core.RecipeIronBoots: true,
	}
	assertNoArmorMatch := func(t *testing.T, name string, grid [core.CraftingGridSlots]core.ItemStack) {
		t.Helper()
		id, output, ok := core.MatchCraftingGrid(3, grid)
		if ok && armorRecipes[id] {
			t.Fatalf("%s 命中了护甲配方 %d（产物 %+v）：形状纪律失守", name, id, output)
		}
	}

	// 头盔：次排中格补满（6 锭）与缺一枚顶锭都失配。
	assertNoArmorMatch(t, "头盔补空洞", buildCraftingGrid(
		gridCell{0, core.ItemIronIngot}, gridCell{1, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
		gridCell{3, core.ItemIronIngot}, gridCell{4, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
	))
	assertNoArmorMatch(t, "头盔缺料", buildCraftingGrid(
		gridCell{0, core.ItemIronIngot}, gridCell{1, core.ItemIronIngot},
		gridCell{3, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
	))
	// 换料：任一格不是铁锭即失配。
	assertNoArmorMatch(t, "头盔换料", buildCraftingGrid(
		gridCell{0, core.ItemStone}, gridCell{1, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
		gridCell{3, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
	))
	// 胸甲顶中补满变成九格全满铁锭——必须命中既有铁块配方而不是护甲配方。
	full := buildCraftingGrid(
		gridCell{0, core.ItemIronIngot}, gridCell{1, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
		gridCell{3, core.ItemIronIngot}, gridCell{4, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
		gridCell{6, core.ItemIronIngot}, gridCell{7, core.ItemIronIngot}, gridCell{8, core.ItemIronIngot},
	)
	if id, _, ok := core.MatchCraftingGrid(3, full); !ok || id != core.RecipeIronBlock {
		t.Fatalf("九格全满铁锭 = (%d, %v)，想要铁块配方 %d", id, ok, core.RecipeIronBlock)
	}
	// 护腿垂直翻转（顶排 3 锭挪到底排）失配：垂直翻转永不参与匹配。
	assertNoArmorMatch(t, "护腿垂直翻转", buildCraftingGrid(
		gridCell{0, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
		gridCell{3, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
		gridCell{6, core.ItemIronIngot}, gridCell{7, core.ItemIronIngot}, gridCell{8, core.ItemIronIngot},
	))
	// 靴子中列补满（5 锭）失配。
	assertNoArmorMatch(t, "靴子补空洞", buildCraftingGrid(
		gridCell{0, core.ItemIronIngot}, gridCell{2, core.ItemIronIngot},
		gridCell{3, core.ItemIronIngot}, gridCell{4, core.ItemIronIngot}, gridCell{5, core.ItemIronIngot},
	))

	// 个人 2×2 网格按 2×2 解释四格，摆不下宽为 3 的护甲形状。
	personal := buildCraftingGrid(
		gridCell{0, core.ItemIronIngot}, gridCell{1, core.ItemIronIngot},
		gridCell{2, core.ItemIronIngot}, gridCell{3, core.ItemIronIngot},
	)
	if id, _, ok := core.MatchCraftingGrid(2, personal); ok {
		t.Fatalf("个人 2×2 网格匹配了护甲配方 %d", id)
	}
}

// TestArmorRecipesConsumeExactlyTheirIngots 锁定原料扣除：合成一次护甲后
// 形状内的铁锭逐格恰减 1，其余格保持规范空栈；任一材料格数量不足时整次
// 扣除失败且网格原值不变。
func TestArmorRecipesConsumeExactlyTheirIngots(t *testing.T) {
	pattern, ok := core.Recipe(core.RecipeIronHelmet)
	if !ok {
		t.Fatal("头盔配方未注册")
	}
	var grid [core.CraftingGridSlots]core.ItemStack
	for _, slot := range []uint8{0, 1, 2, 3, 5} {
		grid[slot] = core.ItemStack{Item: core.ItemIronIngot, Count: 2}
	}
	next, ok := core.ConsumeRecipe(3, grid, pattern)
	if !ok {
		t.Fatal("五格双锭的头盔网格应当可以扣除原料")
	}
	for _, slot := range []uint8{0, 1, 2, 3, 5} {
		if next[slot] != (core.ItemStack{Item: core.ItemIronIngot, Count: 1}) {
			t.Fatalf("格 %d = %+v，合成一次后应剩 1 个铁锭", slot, next[slot])
		}
	}
	for slot, stack := range next {
		switch slot {
		case 0, 1, 2, 3, 5:
			continue
		}
		if stack != (core.ItemStack{}) {
			t.Fatalf("形状外的格 %d = %+v，必须保持空", slot, stack)
		}
	}

	// 数量不足：单格缺料时整次扣除失败，原网格逐位不变。
	short := grid
	short[5] = core.ItemStack{}
	rejected, ok := core.ConsumeRecipe(3, short, pattern)
	if ok {
		t.Fatal("缺料网格不应扣除成功")
	}
	if rejected != short {
		t.Fatal("扣除失败时网格必须保持原值")
	}
}

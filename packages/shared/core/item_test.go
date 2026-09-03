package core_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestCanonicalItemIDsStayStable(t *testing.T) {
	got := []core.ItemID{
		core.ItemNone,
		core.ItemStone,
		core.ItemDirt,
		core.ItemGrass,
	}
	for i, id := range got {
		if id != core.ItemID(i) {
			t.Fatalf("ItemID[%d] = %d，协议要求固定为 %d", i, id, i)
		}
	}
}

// TestItemIDMaxGuardsExhaustiveEnumeration 锁定 ItemIDMax 独占哨兵与枚举末项的
// 关系：当前最后一个合法物品必须是损坏铁剑。物品演进
// 纪律是只能在哨兵之前追加；将来追加新物品时第一个断言变红，迫使开发者同步
// 审视全部以「item < ItemIDMax」为穷举界的测试（例如 companion 的 place 注册表
// 覆盖测试），而不是让穷举测试静默失去对新物品的覆盖。
func TestItemIDMaxGuardsExhaustiveEnumeration(t *testing.T) {
	if core.ItemBrokenIronSword != core.ItemIDMax-1 {
		t.Fatalf("ItemID 枚举末项不再是 ItemBrokenIronSword（ItemIDMax-1 = %d）；"+
			"新增物品必须同步审视全部以 ItemIDMax 为穷举界的测试", core.ItemIDMax-1)
	}
	// 哨兵之外不得再出现已注册物品：若有人把新物品追加在哨兵之后，穷举界会
	// 静默漏掉它，这里用有界前瞻扫描兜底报警。扫描宽度只作绊线不是完备证明。
	for item := core.ItemIDMax; item < core.ItemIDMax+64; item++ {
		if core.RegisteredItem(item) {
			t.Fatalf("物品 %d 注册在 ItemIDMax 哨兵之外，独占穷举界失效", item)
		}
	}
}

func TestItemIDsAppendOnly(t *testing.T) {
	if core.ItemBrokenIronSword != core.ItemIDMax-1 {
		t.Fatal("Broken iron sword must be last before Max")
	}
	if core.ItemRottenFlesh != core.ItemBed-1 {
		t.Fatal("Rotten flesh must sit right before Bed")
	}
	if core.ItemTorch != core.ItemRottenFlesh-1 {
		t.Fatal("Torch must sit right before rotten flesh")
	}
	if _, ok := core.ItemStackLimit(core.ItemPotato); !ok {
		t.Fatal("potato stack missing")
	}
	if _, ok := core.ItemStackLimit(core.ItemCarrot); !ok {
		t.Fatal("carrot stack missing")
	}
	if _, ok := core.ItemStackLimit(core.ItemPoisonousPotato); !ok {
		t.Fatal("poisonous potato stack missing")
	}
	if block, ok := core.ItemPlacement(core.ItemPotato); !ok || block != core.PotatoStage0ID {
		t.Fatalf("ItemPlacement(ItemPotato)=(%d,%v), want (%d,true)", block, ok, core.PotatoStage0ID)
	}
	if block, ok := core.ItemPlacement(core.ItemCarrot); !ok || block != core.CarrotStage0ID {
		t.Fatalf("ItemPlacement(ItemCarrot)=(%d,%v), want (%d,true)", block, ok, core.CarrotStage0ID)
	}
	if block, ok := core.ItemPlacement(core.ItemPoisonousPotato); ok || block != core.AirID {
		t.Fatalf("ItemPlacement(ItemPoisonousPotato)=(%d,%v), want (AirID,false): poisonous not placeable", block, ok)
	}
}

func TestSwordItemsAreRegisteredWithFixedSemantics(t *testing.T) {
	tests := []struct {
		name       string
		item       core.ItemID
		id         core.ItemID
		durability uint16
		broken     core.ItemID
		damage     int32
		intact     bool
	}{
		{"木剑", core.ItemWoodenSword, 47, 59, core.ItemBrokenWoodenSword, 4, true},
		{"石剑", core.ItemStoneSword, 48, 131, core.ItemBrokenStoneSword, 5, true},
		{"铁剑", core.ItemIronSword, 49, 250, core.ItemBrokenIronSword, 6, true},
		{"损坏木剑", core.ItemBrokenWoodenSword, 50, 0, core.ItemNone, 2, false},
		{"损坏石剑", core.ItemBrokenStoneSword, 51, 0, core.ItemNone, 2, false},
		{"损坏铁剑", core.ItemBrokenIronSword, 52, 0, core.ItemNone, 2, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.item != test.id {
				t.Fatalf("ItemID = %d，想要 %d", test.item, test.id)
			}
			if limit, ok := core.ItemStackLimit(test.item); !ok || limit != 1 {
				t.Fatalf("ItemStackLimit(%d) = (%d,%v)，想要 (1,true)", test.item, limit, ok)
			}
			if got := core.IsIntactSword(test.item); got != test.intact {
				t.Fatalf("IsIntactSword(%d) = %v，想要 %v", test.item, got, test.intact)
			}
			if got := core.WeaponDamage(test.item); got != test.damage {
				t.Fatalf("WeaponDamage(%d) = %d，想要 %d", test.item, got, test.damage)
			}
			gotDurability, hasDurability := core.ItemMaxDurability(test.item)
			if gotDurability != test.durability || hasDurability != test.intact {
				t.Fatalf("ItemMaxDurability(%d) = (%d,%v)，想要 (%d,%v)",
					test.item, gotDurability, hasDurability, test.durability, test.intact)
			}
			gotBroken, hasBroken := core.ItemBrokenForm(test.item)
			if gotBroken != test.broken || hasBroken != test.intact {
				t.Fatalf("ItemBrokenForm(%d) = (%d,%v)，想要 (%d,%v)",
					test.item, gotBroken, hasBroken, test.broken, test.intact)
			}
			stack := core.ItemStack{Item: test.item, Count: 1, Durability: test.durability}
			if !stack.Valid() {
				t.Fatalf("剑物品栈 %+v 应当有效", stack)
			}
		})
	}
	if core.ItemIDMax != 53 {
		t.Fatalf("ItemIDMax = %d，想要 53", core.ItemIDMax)
	}
	for _, item := range []core.ItemID{core.ItemNone, core.ItemDirt} {
		if core.IsIntactSword(item) {
			t.Fatalf("普通物品 %d 被识别为完好剑", item)
		}
		if got := core.WeaponDamage(item); got != 2 {
			t.Fatalf("WeaponDamage(%d) = %d，想要基础伤害 2", item, got)
		}
	}
}

func TestCommonBlockMaterialsAreFixedAndRoundTrip(t *testing.T) {
	tests := []struct {
		block core.BlockID
		item  core.ItemID
	}{
		{core.CobblestoneID, core.ItemCobblestone},
		{core.SmoothStoneID, core.ItemSmoothStone},
		{core.SandID, core.ItemSand},
		{core.GravelID, core.ItemGravel},
		{core.OakLogID, core.ItemOakLog},
		{core.OakPlanksID, core.ItemOakPlanks},
		{core.LeavesID, core.ItemLeaves},
		{core.GlassID, core.ItemGlass},
		{core.BrickID, core.ItemBrick},
		{core.WhiteWoolID, core.ItemWhiteWool},
		{core.RoofTileID, core.ItemRoofTile},
		{core.ClayID, core.ItemClay},
		{core.SnowBlockID, core.ItemSnowBlock},
		{core.MossyCobblestoneID, core.ItemMossyCobblestone},
	}
	for i, tc := range tests {
		if tc.block != core.LightBlockID+1+core.BlockID(i) || tc.item != core.ItemLightBlock+1+core.ItemID(i) {
			t.Fatalf("材料 %d 的稳定编号不连续: block=%d item=%d", i, tc.block, tc.item)
		}
		if !core.RegisteredBlock(tc.block) || !core.RegisteredItem(tc.item) {
			t.Fatalf("材料 %d 未注册", i)
		}
		if got, ok := core.ItemPlacement(tc.item); !ok || got != tc.block {
			t.Fatalf("ItemPlacement(%d)=(%d,%v)，想要 (%d,true)", tc.item, got, ok, tc.block)
		}
		if got, ok := core.BlockDrop(tc.block); !ok || got != tc.item {
			t.Fatalf("BlockDrop(%d)=(%d,%v)，想要 (%d,true)", tc.block, got, ok, tc.item)
		}
		if limit, ok := core.ItemStackLimit(tc.item); !ok || limit != 64 {
			t.Fatalf("ItemStackLimit(%d)=(%d,%v)，想要 (64,true)", tc.item, limit, ok)
		}
	}
	// MossyCobblestoneID+1 是 WaterSourceID、WaterLevel7ID+1 是 FarmlandDryID，
	// 两者都已注册；未注册编号的独占上界只能用 BlockIDMax 表达。
	if core.RegisteredBlock(core.BlockIDMax) {
		t.Fatal("未知方块被注册")
	}
}

func TestToolIDsAndStackLimitsAreStable(t *testing.T) {
	if core.ItemStonePickaxe != 10 || core.ItemIronPickaxe != 11 {
		t.Fatalf("工具 ID 发生变化: stone=%d iron=%d", core.ItemStonePickaxe, core.ItemIronPickaxe)
	}
	tests := []struct {
		item core.ItemID
		want uint8
		ok   bool
	}{
		{core.ItemStone, 64, true},
		{core.ItemIronBlock, 64, true},
		{core.ItemStonePickaxe, 1, true},
		{core.ItemIronPickaxe, 1, true},
		{core.ItemNone, 0, false},
		{core.ItemID(4242), 0, false},
	}
	for _, test := range tests {
		got, ok := core.ItemStackLimit(test.item)
		if got != test.want || ok != test.ok {
			t.Errorf("ItemStackLimit(%d)=(%d,%v)，想要 (%d,%v)",
				test.item, got, ok, test.want, test.ok)
		}
	}
	for _, tool := range []core.ItemID{core.ItemStonePickaxe, core.ItemIronPickaxe} {
		full, _ := core.ItemMaxDurability(tool)
		if !(core.ItemStack{Item: tool, Count: 1, Durability: full}).Valid() {
			t.Errorf("单个工具 %d 应当有效", tool)
		}
		if (core.ItemStack{Item: tool, Count: 2, Durability: full}).Valid() {
			t.Errorf("两个工具 %d 必须无效", tool)
		}
	}
}

func TestHotbarShapeIsFixed(t *testing.T) {
	var h core.Hotbar
	if core.HotbarSlots != 9 {
		t.Fatalf("HotbarSlots = %d，契约要求 9", core.HotbarSlots)
	}
	if core.MaxStackCount != 64 {
		t.Fatalf("MaxStackCount = %d，契约要求 64", core.MaxStackCount)
	}
	if len(h.Slots) != core.HotbarSlots {
		t.Fatalf("len(Slots) = %d，想要 %d", len(h.Slots), core.HotbarSlots)
	}
	if !h.Valid() {
		t.Fatal("零值快捷栏应当有效：9 个空栏位且选中栏位 0")
	}
}

func TestItemStackValid(t *testing.T) {
	cases := []struct {
		name  string
		stack core.ItemStack
		want  bool
	}{
		{"空栏位", core.ItemStack{}, true},
		{"单个石头", core.ItemStack{Item: core.ItemStone, Count: 1}, true},
		{"满堆叠", core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount}, true},
		{"超出堆叠上限", core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount + 1}, false},
		{"非空物品零数量", core.ItemStack{Item: core.ItemGrass, Count: 0}, false},
		{"空物品非零数量", core.ItemStack{Item: core.ItemNone, Count: 3}, false},
		{"未知物品", core.ItemStack{Item: core.ItemID(9999), Count: 1}, false},
	}
	for _, tc := range cases {
		if got := tc.stack.Valid(); got != tc.want {
			t.Fatalf("%s：Valid() = %v，想要 %v", tc.name, got, tc.want)
		}
	}
}

func TestHotbarValidRejectsBadState(t *testing.T) {
	valid := core.Hotbar{Selected: 8}
	valid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	if !valid.Valid() {
		t.Fatal("合法快捷栏被拒绝")
	}

	outOfRange := valid
	outOfRange.Selected = core.HotbarSlots
	if outOfRange.Valid() {
		t.Fatal("越界选中栏位必须被拒绝")
	}

	badSlot := valid
	badSlot.Slots[3] = core.ItemStack{Item: core.ItemNone, Count: 1}
	if badSlot.Valid() {
		t.Fatal("空物品与非零数量的组合必须被拒绝")
	}

	unknownItem := valid
	unknownItem.Slots[5] = core.ItemStack{Item: core.ItemID(200), Count: 1}
	if unknownItem.Valid() {
		t.Fatal("未知物品必须被拒绝")
	}
}

func TestHotbarAddPrefersSameItemBeforeEmptySlot(t *testing.T) {
	var h core.Hotbar
	h.Slots[2] = core.ItemStack{Item: core.ItemDirt, Count: 63}

	got, ok := h.Add(core.ItemDirt)
	if !ok {
		t.Fatal("同类未满栏位应当可以接收物品")
	}
	if got.Slots[2] != (core.ItemStack{Item: core.ItemDirt, Count: 64}) {
		t.Fatalf("栏位 2 = %+v，想要 64 个泥土", got.Slots[2])
	}
	if got.Slots[0] != (core.ItemStack{}) {
		t.Fatalf("栏位 0 = %+v，应当保持为空", got.Slots[0])
	}
	if h.Slots[2].Count != 63 {
		t.Fatal("Add 必须在值副本上完成，原值不得被修改")
	}
}

func TestHotbarAddUsesLowestEmptySlot(t *testing.T) {
	var h core.Hotbar
	h.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	h.Slots[2] = core.ItemStack{Item: core.ItemGrass, Count: 1}
	h.Slots[3] = core.ItemStack{Item: core.ItemStone, Count: 1}
	h.Slots[4] = core.ItemStack{Item: core.ItemStone, Count: 1}
	h.Slots[6] = core.ItemStack{Item: core.ItemStone, Count: 1}
	h.Slots[7] = core.ItemStack{Item: core.ItemStone, Count: 1}
	h.Slots[8] = core.ItemStack{Item: core.ItemStone, Count: 1}

	got, ok := h.Add(core.ItemDirt)
	if !ok {
		t.Fatal("存在空栏位时 Add 应当成功")
	}
	if got.Slots[1] != (core.ItemStack{Item: core.ItemDirt, Count: 1}) {
		t.Fatalf("栏位 1 = %+v，想要 1 个泥土", got.Slots[1])
	}
	if got.Slots[5] != (core.ItemStack{}) {
		t.Fatalf("栏位 5 = %+v，应当保持为空", got.Slots[5])
	}
}

func TestHotbarAddSkipsFullSameItemStack(t *testing.T) {
	var h core.Hotbar
	h.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount}

	got, ok := h.Add(core.ItemDirt)
	if !ok {
		t.Fatal("满栏位之后仍有空栏位时应当成功")
	}
	if got.Slots[0].Count != core.MaxStackCount {
		t.Fatalf("满栏位数量 = %d，不得超过 %d", got.Slots[0].Count, core.MaxStackCount)
	}
	if got.Slots[1] != (core.ItemStack{Item: core.ItemDirt, Count: 1}) {
		t.Fatalf("栏位 1 = %+v，想要 1 个泥土", got.Slots[1])
	}
}

func TestHotbarAddFailsWhenFull(t *testing.T) {
	var h core.Hotbar
	for i := range h.Slots {
		h.Slots[i] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
	}

	got, ok := h.Add(core.ItemDirt)
	if ok {
		t.Fatal("没有空间时 Add 必须失败")
	}
	if got != h {
		t.Fatal("Add 失败时快捷栏必须保持不变")
	}
}

func TestHotbarAddRejectsUnknownItem(t *testing.T) {
	var h core.Hotbar
	for _, item := range []core.ItemID{core.ItemNone, core.ItemID(77)} {
		got, ok := h.Add(item)
		if ok {
			t.Fatalf("Add(%d) 必须失败", item)
		}
		if got != h {
			t.Fatalf("Add(%d) 失败时快捷栏必须保持不变", item)
		}
	}
}

func TestHotbarAddRejectsToolsWithoutDurability(t *testing.T) {
	var hotbar core.Hotbar
	for _, item := range []core.ItemID{core.ItemStonePickaxe, core.ItemIronPickaxe} {
		got, ok := hotbar.Add(item)
		if ok || got != hotbar {
			t.Fatalf("Add(%d) = %+v, %v，没有耐久参数时必须拒绝工具", item, got, ok)
		}
	}

	got, ok := hotbar.Add(core.ItemDirt)
	if !ok || got.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 1}) {
		t.Fatalf("普通物品 Add = %+v, %v", got, ok)
	}
}

func TestHotbarConsumeNormalizesEmptySlot(t *testing.T) {
	var h core.Hotbar
	h.Slots[4] = core.ItemStack{Item: core.ItemDirt, Count: 1}

	got, ok := h.Consume(4)
	if !ok {
		t.Fatal("非空栏位应当可以消耗")
	}
	if got.Slots[4] != (core.ItemStack{}) {
		t.Fatalf("栏位 4 = %+v，想要规范空栏位", got.Slots[4])
	}
	if h.Slots[4].Count != 1 {
		t.Fatal("Consume 必须在值副本上完成，原值不得被修改")
	}
}

func TestHotbarConsumeDecrementsCount(t *testing.T) {
	var h core.Hotbar
	h.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 2}

	got, ok := h.Consume(1)
	if !ok {
		t.Fatal("非空栏位应当可以消耗")
	}
	if got.Slots[1] != (core.ItemStack{Item: core.ItemStone, Count: 1}) {
		t.Fatalf("栏位 1 = %+v，想要 1 个石头", got.Slots[1])
	}
}

func TestHotbarConsumeRejectsEmptyOrInvalidSlot(t *testing.T) {
	var h core.Hotbar
	h.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}

	for _, slot := range []uint8{1, core.HotbarSlots, 255} {
		got, ok := h.Consume(slot)
		if ok {
			t.Fatalf("Consume(%d) 必须失败", slot)
		}
		if got != h {
			t.Fatalf("Consume(%d) 失败时快捷栏必须保持不变", slot)
		}
	}
}

// TestFluidBlocksDoNotProduceItems 锁定「流体不进物品表」：全部 8 个流体编号
// 采掘不产出任何物品——流体只追加在 BlockID 枚举，自己不带任何 ItemID。
//
// 本用例原先还断言 `ItemIDMax == ItemMossyCobblestone+1`，用来表达"流体没有
// 新增物品"。那条断言把流体的性质写成了对物品表末项的锁定，任何**别的**变更
// 追加物品都会让它变红（农业追加 6 个物品即触发），因此已改由
// TestItemIDMaxGuardsExhaustiveEnumeration 统一守护物品表末项，这里只保留
// 与流体真正相关的部分。
func TestFluidBlocksDoNotProduceItems(t *testing.T) {
	for _, block := range []core.BlockID{
		core.WaterSourceID, core.WaterLevel1ID, core.WaterLevel2ID, core.WaterLevel3ID,
		core.WaterLevel4ID, core.WaterLevel5ID, core.WaterLevel6ID, core.WaterLevel7ID,
	} {
		if item, ok := core.BlockDrop(block); ok || item != core.ItemNone {
			t.Fatalf("BlockDrop(%d) = (%d,%v)，想要 (ItemNone,false)：流体不应产出物品", block, item, ok)
		}
	}
}

// TestNoItemPlacesAsFluid 是 spec Scenario「流体不可放置」的穷举守护：当前没有
// 任何物品能放置为流体方块（因为没有任何物品映射到流体 BlockID），这条断言
// 锁住这个事实，将来有人加"水桶"一类物品让 ItemPlacement 映射到流体编号时，
// 这里会第一个报警。
func TestNoItemPlacesAsFluid(t *testing.T) {
	for item := core.ItemID(0); item < core.ItemIDMax; item++ {
		if block, ok := core.ItemPlacement(item); ok && core.IsFluid(block) {
			t.Fatalf("物品 %d 可放置为流体方块 %d", item, block)
		}
	}
}

func TestBlockDropMapping(t *testing.T) {
	cases := []struct {
		block core.BlockID
		item  core.ItemID
		ok    bool
	}{
		{core.StoneID, core.ItemStone, true},
		{core.DirtID, core.ItemDirt, true},
		{core.GrassID, core.ItemGrass, true},
		{core.AirID, core.ItemNone, false},
		{core.BarrierID, core.ItemNone, false},
		{core.BedrockID, core.ItemNone, false},
		{core.BlockID(4242), core.ItemNone, false},
	}
	for _, tc := range cases {
		item, ok := core.BlockDrop(tc.block)
		if ok != tc.ok || item != tc.item {
			t.Fatalf("BlockDrop(%d) = (%d, %v)，想要 (%d, %v)", tc.block, item, ok, tc.item, tc.ok)
		}
	}
}

func TestItemPlacementMapping(t *testing.T) {
	cases := []struct {
		item  core.ItemID
		block core.BlockID
		ok    bool
	}{
		{core.ItemStone, core.StoneID, true},
		{core.ItemDirt, core.DirtID, true},
		{core.ItemGrass, core.GrassID, true},
		{core.ItemNone, core.AirID, false},
		{core.ItemID(4242), core.AirID, false},
	}
	for _, tc := range cases {
		block, ok := core.ItemPlacement(tc.item)
		if ok != tc.ok || block != tc.block {
			t.Fatalf("ItemPlacement(%d) = (%d, %v)，想要 (%d, %v)", tc.item, block, ok, tc.block, tc.ok)
		}
	}
}

func TestBlockDropAndItemPlacementRoundTrip(t *testing.T) {
	for _, block := range []core.BlockID{core.StoneID, core.DirtID, core.GrassID} {
		item, ok := core.BlockDrop(block)
		if !ok {
			t.Fatalf("BlockDrop(%d) 应当有掉落物", block)
		}
		got, ok := core.ItemPlacement(item)
		if !ok || got != block {
			t.Fatalf("ItemPlacement(BlockDrop(%d)) = (%d, %v)，想要 (%d, true)", block, got, ok, block)
		}
	}
}

func TestItemMaxDurabilityCoversToolsOnly(t *testing.T) {
	for _, test := range []struct {
		item core.ItemID
		want uint16
	}{
		{core.ItemStonePickaxe, 131},
		{core.ItemIronPickaxe, 250},
	} {
		got, ok := core.ItemMaxDurability(test.item)
		if !ok || got != test.want {
			t.Fatalf("物品 %d 耐久上限 = (%d,%v)，想要 (%d,true)", test.item, got, ok, test.want)
		}
	}
	// 非工具、损坏物品与未注册物品都没有耐久上限。
	for _, item := range []core.ItemID{
		core.ItemStone, core.ItemCoal, core.ItemIronIngot,
		core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe,
		core.ItemNone,
	} {
		if got, ok := core.ItemMaxDurability(item); ok || got != 0 {
			t.Fatalf("物品 %d 耐久上限 = (%d,%v)，想要 (0,false)", item, got, ok)
		}
	}
}

func TestItemBrokenFormMapsEachTool(t *testing.T) {
	for _, test := range []struct {
		item core.ItemID
		want core.ItemID
	}{
		{core.ItemStonePickaxe, core.ItemBrokenStonePickaxe},
		{core.ItemIronPickaxe, core.ItemBrokenIronPickaxe},
	} {
		got, ok := core.ItemBrokenForm(test.item)
		if !ok || got != test.want {
			t.Fatalf("物品 %d 损坏形态 = (%d,%v)，想要 (%d,true)", test.item, got, ok, test.want)
		}
	}
	// 损坏物品不会再损坏一次。
	for _, item := range []core.ItemID{
		core.ItemStone, core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe,
	} {
		if got, ok := core.ItemBrokenForm(item); ok || got != core.ItemNone {
			t.Fatalf("物品 %d 损坏形态 = (%d,%v)，想要 (ItemNone,false)", item, got, ok)
		}
	}
}

func TestBrokenToolsAreRegisteredAndUnstackable(t *testing.T) {
	for _, item := range []core.ItemID{
		core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe,
	} {
		if !core.RegisteredItem(item) {
			t.Fatalf("损坏物品 %d 未注册", item)
		}
		if limit, ok := core.ItemStackLimit(item); !ok || limit != 1 {
			t.Fatalf("损坏物品 %d 单格上限 = (%d,%v)，想要 (1,true)", item, limit, ok)
		}
	}
}

// TestGridCraftingIDsAppendBeforeSentinels 锁定格子工作台批次追加的稳定编号：
// 木棍 `ItemStick=37`、工作台物品 `ItemWorkbench=38`、骨粉 `ItemBoneMeal=39`
// （三者 + 马铃薯/胡萝卜/毒土豆都曾紧贴 `ItemIDMax` 哨兵之前；门、火把、腐肉、
// 床与剑批次依次追加后，哨兵现为 53），
// 工作台方块 `WorkbenchID=45`（紧随
// `WheatStage7ID`，后接马铃薯/胡萝卜，该批次落定后 `BlockIDMax` 为 62；门 9 个
// 与火把五形态追加后现为 76）。
// 编号是协议稳定值：插入或重排会平移后续编号，破坏既有存档与线上字节。
func TestGridCraftingIDsAppendBeforeSentinels(t *testing.T) {
	if core.ItemStick != 37 {
		t.Fatalf("ItemStick = %d，必须稳定为 37 且紧随 ItemBread(%d)",
			core.ItemStick, core.ItemBread)
	}
	if core.ItemWorkbench != core.ItemStick+1 {
		t.Fatalf("ItemWorkbench = %d，必须紧随 ItemStick(%d)",
			core.ItemWorkbench, core.ItemStick)
	}
	if core.ItemWorkbench != 38 {
		t.Fatalf("ItemWorkbench = %d，必须稳定为 38", core.ItemWorkbench)
	}
	if core.ItemBoneMeal != core.ItemWorkbench+1 {
		t.Fatalf("ItemBoneMeal = %d，必须紧随 ItemWorkbench(%d)",
			core.ItemBoneMeal, core.ItemWorkbench)
	}
	if core.ItemPotato != core.ItemBoneMeal+1 {
		t.Fatalf("ItemPotato = %d，必须紧随 ItemBoneMeal(%d)",
			core.ItemPotato, core.ItemBoneMeal)
	}
	if core.ItemCarrot != core.ItemPotato+1 {
		t.Fatalf("ItemCarrot = %d，必须紧随 ItemPotato(%d)",
			core.ItemCarrot, core.ItemPotato)
	}
	if core.ItemPoisonousPotato != core.ItemCarrot+1 {
		t.Fatalf("ItemPoisonousPotato = %d，必须紧随 ItemCarrot(%d)",
			core.ItemPoisonousPotato, core.ItemCarrot)
	}
	if core.ItemDoor != core.ItemPoisonousPotato+1 {
		t.Fatalf("ItemDoor = %d，必须紧随 ItemPoisonousPotato(%d)",
			core.ItemDoor, core.ItemPoisonousPotato)
	}
	// 火把物品紧随门物品追加（面向相关的可放置物品，放置映射走
	// PlaceableBlockAtFace）；夜行者死亡掉落的腐肉物品紧随火把追加，
	// 床物品紧随腐肉追加（可合成的双格放置物品），剑批次随后占用 47..52。
	if core.ItemTorch != core.ItemDoor+1 {
		t.Fatalf("ItemTorch = %d，必须紧随 ItemDoor(%d)",
			core.ItemTorch, core.ItemDoor)
	}
	if core.ItemRottenFlesh != core.ItemTorch+1 {
		t.Fatalf("ItemRottenFlesh = %d，必须紧随 ItemTorch(%d)",
			core.ItemRottenFlesh, core.ItemTorch)
	}
	if core.ItemBed != core.ItemRottenFlesh+1 {
		t.Fatalf("ItemBed = %d，必须紧随 ItemRottenFlesh(%d)",
			core.ItemBed, core.ItemRottenFlesh)
	}
	if core.ItemIDMax != 53 {
		t.Fatalf("ItemIDMax = %d，必须在剑批次后移到 53", core.ItemIDMax)
	}
	if core.WorkbenchID != 45 {
		t.Fatalf("WorkbenchID = %d，必须稳定为 45 且紧随 WheatStage7ID(%d)",
			core.WorkbenchID, core.WheatStage7ID)
	}
	if core.PotatoStage0ID != core.WorkbenchID+1 {
		t.Fatalf("PotatoStage0ID = %d，必须紧随 WorkbenchID(%d)",
			core.PotatoStage0ID, core.WorkbenchID)
	}
	if core.CarrotStage0ID != core.PotatoStage7ID+1 {
		t.Fatalf("CarrotStage0ID = %d，必须紧随 PotatoStage7ID(%d)",
			core.CarrotStage0ID, core.PotatoStage7ID)
	}
	if core.DoorLowerSouthClosed != core.CarrotStage7ID+1 {
		t.Fatalf("DoorLowerSouthClosed = %d，必须紧随 CarrotStage7ID(%d)",
			core.DoorLowerSouthClosed, core.CarrotStage7ID)
	}
	if core.DoorUpper != core.DoorLowerEastOpen+1 {
		t.Fatalf("DoorUpper = %d，必须紧随 DoorLowerEastOpen(%d)",
			core.DoorUpper, core.DoorLowerEastOpen)
	}
	// 火把五形态紧随门方块追加，床八形态紧随火把追加，短草再追加为 84，方块侧哨兵随之后移到 85。
	if core.TorchStandingID != core.DoorUpper+1 {
		t.Fatalf("TorchStandingID = %d，必须紧随 DoorUpper(%d)",
			core.TorchStandingID, core.DoorUpper)
	}
	if core.BedFootSouthID != core.TorchWallNegZID+1 {
		t.Fatalf("BedFootSouthID = %d，必须紧随 TorchWallNegZID(%d)",
			core.BedFootSouthID, core.TorchWallNegZID)
	}
	if core.ShortGrassID != core.BedHeadEastID+1 || core.BlockIDMax != 85 {
		t.Fatalf("短草/BlockIDMax = %d/%d，必须紧随 BedHeadEastID(%d) 后移到 84/85",
			core.ShortGrassID, core.BlockIDMax, core.BedHeadEastID)
	}
}

// TestStickIsStackableMaterialAndNeverPlaceable 锁定木棍的物品语义：合成的
// 中间材料，可堆叠 64、不可放置、没有耐久也不是任何工具。
func TestStickIsStackableMaterialAndNeverPlaceable(t *testing.T) {
	limit, ok := core.ItemStackLimit(core.ItemStick)
	if !ok || limit != core.MaxStackCount {
		t.Fatalf("ItemStackLimit(木棍) = (%d,%v)，想要 (%d,true)",
			limit, ok, core.MaxStackCount)
	}
	if _, hasDurability := core.ItemMaxDurability(core.ItemStick); hasDurability {
		t.Fatal("木棍不应该有耐久上限")
	}
	if block, ok := core.ItemPlacement(core.ItemStick); ok {
		t.Fatalf("ItemPlacement(木棍) = (%d,%v)，木棍不可放置", block, ok)
	}
	if stack := (core.ItemStack{Item: core.ItemStick, Count: 1}); !stack.Valid() {
		t.Fatal("单根木棍物品栈必须合法")
	}
}

// TestWorkbenchItemPlacesAndDropsBack 锁定工作台物品的完整闭环：可堆叠 64、
// 没有耐久，放置写入 `WorkbenchID`，采掘工作台方块掉回恰好 1 个工作台物品。
func TestWorkbenchItemPlacesAndDropsBack(t *testing.T) {
	limit, ok := core.ItemStackLimit(core.ItemWorkbench)
	if !ok || limit != core.MaxStackCount {
		t.Fatalf("ItemStackLimit(工作台) = (%d,%v)，想要 (%d,true)",
			limit, ok, core.MaxStackCount)
	}
	if _, hasDurability := core.ItemMaxDurability(core.ItemWorkbench); hasDurability {
		t.Fatal("工作台物品不应该有耐久上限")
	}
	block, ok := core.ItemPlacement(core.ItemWorkbench)
	if !ok || block != core.WorkbenchID {
		t.Fatalf("ItemPlacement(工作台) = (%d,%v)，想要 (%d,true)",
			block, ok, core.WorkbenchID)
	}
	item, ok := core.BlockDrop(core.WorkbenchID)
	if !ok || item != core.ItemWorkbench {
		t.Fatalf("BlockDrop(工作台) = (%d,%v)，想要 (%d,true)",
			item, ok, core.ItemWorkbench)
	}
}

// TestTorchItemIsRegisteredStackableMaterial 锁定火把物品语义：编号 44（紧随
// 门物品；其后依次追加腐肉、床与剑批次，ItemIDMax 后移到 53）、
// 堆叠 64、没有耐久、不是工具、不是食物。
// 放置不经 ItemPlacement（面向无关的旧窗口），只经 PlaceableBlockAtFace 的
// 面 → 形态映射——因此火把对 ItemPlacement 必须保持不可放置，防止任何调用方
// 绕开面映射直接写出「默认形态」。
func TestTorchItemIsRegisteredStackableMaterial(t *testing.T) {
	if core.ItemTorch != 44 {
		t.Fatalf("ItemTorch = %d，必须稳定为 44 且紧随 ItemDoor(%d)",
			core.ItemTorch, core.ItemDoor)
	}
	if core.ItemTorch != core.ItemDoor+1 {
		t.Fatalf("ItemTorch = %d，必须紧随 ItemDoor(%d)",
			core.ItemTorch, core.ItemDoor)
	}
	if core.ItemRottenFlesh != core.ItemTorch+1 {
		t.Fatalf("ItemRottenFlesh = %d，必须紧随 ItemTorch(%d)",
			core.ItemRottenFlesh, core.ItemTorch)
	}
	if core.ItemBed != core.ItemRottenFlesh+1 {
		t.Fatalf("ItemBed = %d，必须紧随 ItemRottenFlesh(%d)",
			core.ItemBed, core.ItemRottenFlesh)
	}
	if core.ItemIDMax != 53 {
		t.Fatalf("ItemIDMax = %d，必须后移到 53", core.ItemIDMax)
	}
	if !core.RegisteredItem(core.ItemTorch) {
		t.Fatal("ItemTorch 未注册")
	}
	if limit, ok := core.ItemStackLimit(core.ItemTorch); !ok || limit != core.MaxStackCount {
		t.Fatalf("ItemStackLimit(火把) = (%d, %v)，想要 (%d, true)",
			limit, ok, core.MaxStackCount)
	}
	if _, hasDurability := core.ItemMaxDurability(core.ItemTorch); hasDurability {
		t.Fatal("火把不应有耐久上限")
	}
	if _, isTool := core.ItemBrokenForm(core.ItemTorch); isTool {
		t.Fatal("火把不是工具，没有损坏形态")
	}
	if _, _, isFood := core.FoodValue(core.ItemTorch); isFood {
		t.Fatal("火把不是食物")
	}
	if stack := (core.ItemStack{Item: core.ItemTorch, Count: 1}); !stack.Valid() {
		t.Fatal("单个火把物品栈必须合法")
	}
	if block, ok := core.ItemPlacement(core.ItemTorch); ok || block != core.AirID {
		t.Fatalf("ItemPlacement(火把) = (%d, %v)，火把放置只走 PlaceableBlockAtFace 面映射",
			block, ok)
	}
}

// TestTorchFormsDropBackOneTorch 锁定五种火把形态的掉落映射：采掘任何形态都
// 掉回恰好一个火把物品，形态差异不产生额外产物或损耗。
func TestTorchFormsDropBackOneTorch(t *testing.T) {
	for _, block := range []core.BlockID{
		core.TorchStandingID,
		core.TorchWallPosXID,
		core.TorchWallNegXID,
		core.TorchWallPosZID,
		core.TorchWallNegZID,
	} {
		item, ok := core.BlockDrop(block)
		if !ok || item != core.ItemTorch {
			t.Fatalf("BlockDrop(火把形态 %d) = (%d, %v)，想要 (%d, true)",
				block, item, ok, core.ItemTorch)
		}
	}
}

func TestItemDoorPlacementDrop(t *testing.T) {
	if core.ItemDoor != 43 || core.ItemIDMax != 53 {
		t.Fatal("ItemDoor IDs")
	}
	if got, ok := core.ItemPlacement(core.ItemDoor); !ok || got != core.DoorLowerSouthClosed {
		t.Fatal("placement")
	}
	if got, ok := core.BlockDrop(core.DoorUpper); !ok || got != core.ItemDoor {
		t.Fatal("drop")
	}
	if got, ok := core.BlockDrop(core.DoorLowerSouthClosed); !ok || got != core.ItemDoor {
		t.Fatal("drop lower")
	}
}

func TestItemStackValidEnforcesDurabilityDomain(t *testing.T) {
	// 有耐久上限的物品：耐久必须落在 1..上限。
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	for _, test := range []struct {
		name  string
		stack core.ItemStack
		want  bool
	}{
		{"满耐久工具", core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full}, true},
		{"半耐久工具", core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 1}, true},
		{"零耐久工具", core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 0}, false},
		{"超上限工具", core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full + 1}, false},
		{"非工具带耐久", core.ItemStack{Item: core.ItemStone, Count: 1, Durability: 1}, false},
		{"非工具零耐久", core.ItemStack{Item: core.ItemStone, Count: 1}, true},
		{"损坏物品带耐久", core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: 1, Durability: 1}, false},
		{"损坏物品零耐久", core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: 1}, true},
		{"空栏位", core.ItemStack{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.stack.Valid(); got != test.want {
				t.Fatalf("Valid() = %v，想要 %v", got, test.want)
			}
		})
	}
}

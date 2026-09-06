package render

import (
	"bytes"
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定右手持物三形态判定：只认调用方传入的已确认选中槽，空槽与未
// 注册物品一律按无持物处理且不 panic。

func TestViewmodelHeldKindEmptySelection(t *testing.T) {
	empty := []core.ItemStack{
		{},
		{Item: core.ItemNone, Count: 0},
		{Item: core.ItemStone, Count: 0},
	}
	for _, stack := range empty {
		if got := ViewmodelHeldKindOf(stack); got != ViewmodelHeldNone {
			t.Fatalf("持物形态(%+v) = %d，想要无持物", stack, got)
		}
	}
}

func TestViewmodelHeldKindUnregisteredItem(t *testing.T) {
	stacks := []core.ItemStack{
		{Item: core.ItemIDMax, Count: 1},
		{Item: core.ItemIDMax + 7, Count: 1},
	}
	for _, stack := range stacks {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("未注册物品(%d)引起 panic：%v", stack.Item, recovered)
				}
			}()
			if got := ViewmodelHeldKindOf(stack); got != ViewmodelHeldNone {
				t.Fatalf("持物形态(未注册 %d) = %d，想要无持物", stack.Item, got)
			}
		}()
	}
}

func TestViewmodelHeldKindBlockItems(t *testing.T) {
	blocks := []core.ItemID{
		core.ItemStone, core.ItemDirt, core.ItemGrass, core.ItemOakLog,
		core.ItemWorkbench, core.ItemChest, core.ItemFurnace,
		// 放置映射命中即方块微缩立方：作物种子与门床同样走立方分支。
		core.ItemWheatSeeds, core.ItemPotato, core.ItemCarrot,
		core.ItemDoor, core.ItemBed,
	}
	for _, item := range blocks {
		if _, ok := core.ItemPlacement(item); !ok {
			t.Fatalf("前置条件崩了：物品 %d 不在放置表里", item)
		}
		stack := core.ItemStack{Item: item, Count: 1}
		if got := ViewmodelHeldKindOf(stack); got != ViewmodelHeldBlock {
			t.Fatalf("持物形态(物品 %d) = %d，想要方块", item, got)
		}
	}
}

func TestViewmodelHeldKindNonBlockItems(t *testing.T) {
	items := []core.ItemID{
		core.ItemBread, core.ItemStick, core.ItemTorch,
		core.ItemCoal, core.ItemIronIngot,
		core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword,
		core.ItemStonePickaxe, core.ItemIronPickaxe,
		core.ItemStoneHoe, core.ItemIronHoe,
		core.ItemRawBeef, core.ItemCookedBeef, core.ItemRottenFlesh,
	}
	for _, item := range items {
		if !core.RegisteredItem(item) {
			t.Fatalf("前置条件崩了：物品 %d 未注册", item)
		}
		if _, ok := core.ItemPlacement(item); ok {
			t.Fatalf("前置条件崩了：物品 %d 可放置，不属于本组", item)
		}
		stack := core.ItemStack{Item: item, Count: 1}
		if got := ViewmodelHeldKindOf(stack); got != ViewmodelHeldItem {
			t.Fatalf("持物形态(物品 %d) = %d，想要扁长条物品", item, got)
		}
	}
}

func TestViewmodelEncodeHeldSilhouette(t *testing.T) {
	player := core.PlayerID{1, 2, 3}
	encode := func(stack core.ItemStack) []byte {
		encoder := &ViewmodelEncoder{}
		return append([]byte(nil), encoder.EncodeViewmodelInstances(nil, viewmodelTestInput(player, stack, 10))...)
	}

	empty := encode(core.ItemStack{})
	if len(empty) != 2*avatarInstanceBytes {
		t.Fatalf("空手编码长度 = %d，想要 2 个实例（左右手、无持物）", len(empty)/avatarInstanceBytes)
	}

	block := encode(core.ItemStack{Item: core.ItemStone, Count: 1})
	if len(block) != 3*avatarInstanceBytes {
		t.Fatalf("持方块编码长度 = %d，想要 3 个实例（左右手 + 持物）", len(block)/avatarInstanceBytes)
	}
	blockSize := decodedPartSize(block, 2)
	if !approxEqual(blockSize[0], blockSize[1]) || !approxEqual(blockSize[1], blockSize[2]) {
		t.Fatalf("持方块几何 = %v，想要微缩立方（三轴近似相等）", blockSize)
	}

	tool := encode(core.ItemStack{Item: core.ItemIronSword, Count: 1})
	if len(tool) != 3*avatarInstanceBytes {
		t.Fatalf("持剑编码长度 = %d，想要 3 个实例（左右手 + 持物）", len(tool)/avatarInstanceBytes)
	}
	toolSize := decodedPartSize(tool, 2)
	if !(toolSize[1] > 2*toolSize[0] && toolSize[1] > 2*toolSize[2]) {
		t.Fatalf("持剑几何 = %v，想要扁长条（纵轴显著长于另两轴）", toolSize)
	}
}

func TestViewmodelHeldSwitchesOnlyOnConfirmedSelection(t *testing.T) {
	player := core.PlayerID{5}
	stone := core.ItemStack{Item: core.ItemStone, Count: 1}
	sword := core.ItemStack{Item: core.ItemIronSword, Count: 1}
	encoder := &ViewmodelEncoder{}
	encode := func(stack core.ItemStack, tick uint64) []byte {
		return append([]byte(nil), encoder.EncodeViewmodelInstances(nil, viewmodelTestInput(player, stack, tick))...)
	}
	// 确认到达前调用方继续传入旧确认栈：形态保持方块，不切换。
	first := encode(stone, 70)
	withheld := encode(stone, 71)
	if got := ViewmodelHeldKindOf(stone); got != ViewmodelHeldBlock {
		t.Fatalf("前置条件崩了：石头形态 = %d", got)
	}
	if !bytes.Equal(first, withheld) {
		t.Fatalf("确认未到达时形态变化，想要保持旧确认值")
	}
	// 确认到达后下一帧传入新确认栈：形态恰在该帧切换为扁长条。
	switched := encode(sword, 72)
	if got := ViewmodelHeldKindOf(sword); got != ViewmodelHeldItem {
		t.Fatalf("前置条件崩了：剑形态 = %d", got)
	}
	if bytes.Equal(withheld, switched) {
		t.Fatalf("确认到达后形态未切换，想要下一帧切换")
	}
	if count := len(switched) / avatarInstanceBytes; count != 3 {
		t.Fatalf("切换后实例数 = %d，想要 3", count)
	}
	if size := decodedPartSize(switched, 2); !(size[1] > 2*size[0] && size[1] > 2*size[2]) {
		t.Fatalf("切换后持物几何 = %v，想要扁长条", size)
	}
}

func TestViewmodelEncodeUnregisteredSelection(t *testing.T) {
	player := core.PlayerID{6}
	encoder := &ViewmodelEncoder{}
	input := viewmodelTestInput(player, core.ItemStack{Item: core.ItemIDMax, Count: 1}, 75)
	input.Mining = true
	input.AttackTick = 75
	var out []byte
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("编码未注册选中引起 panic：%v", recovered)
			}
		}()
		out = encoder.EncodeViewmodelInstances(nil, input)
	}()
	if count := len(out) / avatarInstanceBytes; count != 2 {
		t.Fatalf("未注册选中实例数 = %d，想要 2（双手、无持物）", count)
	}
}

func TestViewmodelHeldBlockAppearance(t *testing.T) {
	// 完整立方体取世界顶面代表层，颜色走贴图采样中性白。
	material, color := viewmodelHeldBlockAppearance(core.ItemStone)
	if material != uint32(assets.LayerStone) {
		t.Fatalf("持方块材质 = %d，想要世界顶面代表层 %d", material, uint32(assets.LayerStone))
	}
	if color != [4]float32{1, 1, 1, 1} {
		t.Fatalf("持方块颜色 = %v，想要贴图采样中性白", color)
	}
	// 非完整立方放置物（种子）取图标同源层，同样六面采样。
	seedsMaterial, seedsColor := viewmodelHeldBlockAppearance(core.ItemWheatSeeds)
	if seedsMaterial != uint32(assets.LayerItemWheatSeeds) {
		t.Fatalf("持种子材质 = %d，想要图标同源层 %d", seedsMaterial, uint32(assets.LayerItemWheatSeeds))
	}
	if seedsColor != [4]float32{1, 1, 1, 1} {
		t.Fatalf("持种子颜色 = %v，想要贴图采样中性白", seedsColor)
	}
}

func TestViewmodelHeldColorFallback(t *testing.T) {
	// 已登记基色的物品复用共享色经呈现明暗（与手臂同源的 `avatarShade`
	// 系数，保证浅色工具在亮背景下可辨；共享注册色本身不动）。
	if color := viewmodelHeldColor(core.ItemStonePickaxe); color != avatarShade(ItemColor(core.ItemStonePickaxe), 0.82) {
		t.Fatalf("持镐颜色 = %v，想要呈现明暗 %v", color, avatarShade(ItemColor(core.ItemStonePickaxe), 0.82))
	}
	// 未覆盖的已注册物品回落中性不透明色（同样经呈现明暗），而非透明黑。
	wantNeutral := avatarShade(viewmodelHeldNeutralColor, 0.82)
	for _, item := range []core.ItemID{core.ItemBread, core.ItemStick, core.ItemTorch} {
		color := viewmodelHeldColor(item)
		if color != wantNeutral {
			t.Fatalf("持物(物品 %d)颜色 = %v，想要中性呈现色 %v", item, color, wantNeutral)
		}
		if color[3] != 1 {
			t.Fatalf("持物(物品 %d)不透明度 = %v，想要 1", item, color[3])
		}
	}
}

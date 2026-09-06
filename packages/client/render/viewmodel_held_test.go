package render

import (
	"testing"

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

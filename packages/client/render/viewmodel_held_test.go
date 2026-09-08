package render

import (
	"bytes"
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
			t.Fatalf("持物形态(物品 %d) = %d，想要图标轮廓物品", item, got)
		}
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
	// 确认到达后下一帧传入新确认栈：形态恰在该帧切换为图标轮廓。
	switched := encode(sword, 72)
	if got := ViewmodelHeldKindOf(sword); got != ViewmodelHeldItem {
		t.Fatalf("前置条件崩了：剑形态 = %d", got)
	}
	if bytes.Equal(withheld, switched) {
		t.Fatalf("确认到达后形态未切换，想要下一帧切换")
	}
	if len(switched) <= 7*avatarInstanceBytes {
		t.Fatal("工具没有像素轮廓")
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
	if count := len(out) / avatarInstanceBytes; count != 1 {
		t.Fatalf("未注册选中实例数 = %d，想要 1（主手、无持物）", count)
	}
}

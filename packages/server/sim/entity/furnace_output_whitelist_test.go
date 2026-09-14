package entity

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// smeltingOutputImage 遍历整个 `ItemID` 值域推导 `core.SmeltingOutput` 的
// 全部产物，而不是手写输入清单：未来扩展熔炼映射时，新产物自动进入本
// 守卫的期望集合，漏改输出白名单会让测试立刻失败。
func smeltingOutputImage() map[core.ItemID]bool {
	image := make(map[core.ItemID]bool)
	for id := range uint32(math.MaxUint16) + 1 {
		if output, ok := core.SmeltingOutput(core.ItemID(id)); ok {
			image[output] = true
		}
	}
	return image
}

// TestSetFurnaceViewSlotOutputWhitelistMatchesSmeltingImage 锁定
// `setFurnaceViewSlot` 输出槽白名单恰好接受 `core.SmeltingOutput` 的全部
// 产物 ∪ {ItemNone}。白名单少于产物集合时，从输出格取出产物的余量无法
// 写回，整条移动命令被拒收，熔炉界面会被冻结。
func TestSetFurnaceViewSlotOutputWhitelistMatchesSmeltingImage(t *testing.T) {
	image := smeltingOutputImage()
	if len(image) == 0 {
		t.Fatal("熔炼产物表为空，守卫失去意义")
	}
	for id := range uint32(math.MaxUint16) + 1 {
		item := core.ItemID(id)
		stack := core.ItemStack{}
		if item != core.ItemNone {
			stack = core.ItemStack{Item: item, Count: 1}
		}
		_, furnace, ok := setFurnaceViewSlot(
			core.Inventory{},
			world.FurnaceSlot{Generation: 1, Active: true},
			core.FurnaceOutputSlot,
			stack,
		)
		want := item == core.ItemNone || image[item]
		if ok != want {
			t.Errorf("输出槽写入物品 %d ok=%v，想要 %v", item, ok, want)
			continue
		}
		if ok && furnace.Output != stack {
			t.Errorf("输出槽写入物品 %d 后 Output=%+v", item, furnace.Output)
		}
	}
}

// TestFurnaceMovesFromCookedBeefOutputSucceed 回归锁定生牛肉熔炼产物长期
// 缺失的白名单漏洞：输出格装熟牛肉的熔炉必须可以通过整堆、单件与快捷
// 搬运三种路径从输出格取回产物。
func TestFurnaceMovesFromCookedBeefOutputSucceed(t *testing.T) {
	furnaceWithOutput := func(count uint8) world.FurnaceSlot {
		return world.FurnaceSlot{Output: core.ItemStack{Item: core.ItemCookedBeef, Count: count}}
	}
	t.Run("整堆取出", func(t *testing.T) {
		engine, session, ref := stackSplitFurnaceFixture(t, core.Inventory{}, furnaceWithOutput(5), CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{{
			Session: session, Sequence: 3, Kind: CommandMoveFurnaceStack,
			Furnace: ref, Slot: core.FurnaceOutputSlot, ToSlot: 0,
		}})
		if len(result.Rejected) != 0 {
			t.Fatalf("整堆取出熟牛肉被拒绝: %+v", result.Rejected)
		}
		if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemCookedBeef, Count: 5}) {
			t.Fatalf("目标格 = %+v，想要整堆 5", hotbar)
		}
		if got := stackSplitFurnaceAt(t, engine, ref).Output; got != (core.ItemStack{}) {
			t.Fatalf("输出格 = %+v，想要清空", got)
		}
	})
	t.Run("单件取出", func(t *testing.T) {
		engine, session, ref := stackSplitFurnaceFixture(t, core.Inventory{}, furnaceWithOutput(5), CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			stackSplitCommand(session, 3, StackViewContainer, core.FurnaceOutputSlot, 0, true, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("单件取出熟牛肉被拒绝: %+v", result.Rejected)
		}
		if got := stackSplitFurnaceAt(t, engine, ref).Output; got != (core.ItemStack{Item: core.ItemCookedBeef, Count: 4}) {
			t.Fatalf("输出格 = %+v，想要余 4", got)
		}
		if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemCookedBeef, Count: 1}) {
			t.Fatalf("目标格 = %+v，想要恰好 1 个", hotbar)
		}
	})
	t.Run("快捷搬运", func(t *testing.T) {
		engine, session, ref := stackSplitFurnaceFixture(t, core.Inventory{}, furnaceWithOutput(5), CraftingGrid{})
		openStackSplitFurnace(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, core.FurnaceOutputSlot, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("快捷搬运熟牛肉被拒绝: %+v", result.Rejected)
		}
		if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemCookedBeef, Count: 5}) {
			t.Fatalf("目标格 = %+v，想要整堆 5", hotbar)
		}
		if got := stackSplitFurnaceAt(t, engine, ref).Output; got != (core.ItemStack{}) {
			t.Fatalf("输出格 = %+v，想要清空", got)
		}
	})
}

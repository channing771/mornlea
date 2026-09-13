package world_test

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// smeltingOutputImage 遍历整个 `ItemID` 值域推导 `core.SmeltingOutput` 的
// 全部产物，而不是手写输入清单：未来扩展熔炼映射时，新产物自动进入本
// 守卫的期望集合，漏改白名单会让测试立刻失败。
func smeltingOutputImage() map[core.ItemID]bool {
	image := make(map[core.ItemID]bool)
	for id := range uint32(math.MaxUint16) + 1 {
		if output, ok := core.SmeltingOutput(core.ItemID(id)); ok {
			image[output] = true
		}
	}
	return image
}

// TestFurnaceOutputWhitelistMatchesSmeltingImage 锁定 `validFurnaceOutput`
// 的白名单恰好等于 `core.SmeltingOutput` 的全部产物 ∪ {ItemNone}：少一项
// 时熔炉 tick 把产物写进输出格后整只熔炉被判非法——存档按损坏拒绝、区块
// 保存与全部界面移动连锁失败；多一项则放宽了固定产物约束。
func TestFurnaceOutputWhitelistMatchesSmeltingImage(t *testing.T) {
	image := smeltingOutputImage()
	if len(image) == 0 {
		t.Fatal("熔炼产物表为空，守卫失去意义")
	}
	for id := range uint32(math.MaxUint16) + 1 {
		item := core.ItemID(id)
		slot := world.FurnaceSlot{Generation: 1, Active: true}
		if item != core.ItemNone {
			slot.Output = core.ItemStack{Item: item, Count: 1}
		}
		want := item == core.ItemNone || image[item]
		if got := slot.Valid(); got != want {
			t.Errorf("输出格对物品 %d Valid=%v，想要 %v", item, got, want)
		}
	}
}

// TestFurnaceSlotAcceptsCookedBeefOutput 回归锁定生牛肉熔炼产物长期缺失的
// 白名单漏洞：输出格装熟牛肉的熔炉必须通过 `FurnaceSlot.Valid()`。
func TestFurnaceSlotAcceptsCookedBeefOutput(t *testing.T) {
	slot := world.FurnaceSlot{
		Generation: 1,
		Active:     true,
		Input:      core.ItemStack{Item: core.ItemRawBeef, Count: 2},
		Fuel:       core.ItemStack{Item: core.ItemCoal, Count: 1},
		Output:     core.ItemStack{Item: core.ItemCookedBeef, Count: 3},
	}
	if !slot.Valid() {
		t.Fatal("输出格装熟牛肉的熔炉被判非法")
	}
}

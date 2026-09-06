package render

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定挥动六档映射与参数表：斧铲在配方与采掘规则落地前取镐档默认
// 值，落地后只改表值，不动编码布局。

func TestViewmodelTierMapping(t *testing.T) {
	cases := []struct {
		name  string
		stack core.ItemStack
		want  ViewmodelTier
	}{
		{"空槽", core.ItemStack{}, ViewmodelTierEmptyHand},
		{"零数量", core.ItemStack{Item: core.ItemStone, Count: 0}, ViewmodelTierEmptyHand},
		{"未注册", core.ItemStack{Item: core.ItemIDMax, Count: 1}, ViewmodelTierEmptyHand},
		{"方块", core.ItemStack{Item: core.ItemStone, Count: 1}, ViewmodelTierBlock},
		{"种子同样方块档", core.ItemStack{Item: core.ItemWheatSeeds, Count: 1}, ViewmodelTierBlock},
		{"木剑", core.ItemStack{Item: core.ItemWoodenSword, Count: 1}, ViewmodelTierSword},
		{"石剑", core.ItemStack{Item: core.ItemStoneSword, Count: 1}, ViewmodelTierSword},
		{"铁剑", core.ItemStack{Item: core.ItemIronSword, Count: 1}, ViewmodelTierSword},
		{"断剑仍剑档", core.ItemStack{Item: core.ItemBrokenStoneSword, Count: 1}, ViewmodelTierSword},
		{"石镐", core.ItemStack{Item: core.ItemStonePickaxe, Count: 1}, ViewmodelTierPick},
		{"铁镐", core.ItemStack{Item: core.ItemIronPickaxe, Count: 1}, ViewmodelTierPick},
		{"断镐仍镐档", core.ItemStack{Item: core.ItemBrokenIronPickaxe, Count: 1}, ViewmodelTierPick},
		{"石锄", core.ItemStack{Item: core.ItemStoneHoe, Count: 1}, ViewmodelTierHoe},
		{"铁锄", core.ItemStack{Item: core.ItemIronHoe, Count: 1}, ViewmodelTierHoe},
		{"断锄仍锄档", core.ItemStack{Item: core.ItemBrokenStoneHoe, Count: 1}, ViewmodelTierHoe},
		{"食物沿用空手档", core.ItemStack{Item: core.ItemBread, Count: 1}, ViewmodelTierEmptyHand},
		{"火把沿用空手档", core.ItemStack{Item: core.ItemTorch, Count: 1}, ViewmodelTierEmptyHand},
		{"材料沿用空手档", core.ItemStack{Item: core.ItemCoal, Count: 1}, ViewmodelTierEmptyHand},
	}
	for _, tc := range cases {
		if got := ViewmodelTierOf(tc.stack); got != tc.want {
			t.Fatalf("%s：档位 = %d，想要 %d", tc.name, got, tc.want)
		}
	}
}

func TestViewmodelSwingTablePinsValues(t *testing.T) {
	cases := []struct {
		tier        ViewmodelTier
		amplitude   float32
		periodTicks uint64
	}{
		{ViewmodelTierEmptyHand, 0.5, 12},
		{ViewmodelTierBlock, 0.4, 14},
		{ViewmodelTierSword, 0.7, 8},
		{ViewmodelTierPick, 0.7, 10},
		// 锄与斧取镐档默认值：与镐逐值相等，规则落地后只改这两行。
		{ViewmodelTierHoe, 0.7, 10},
		{ViewmodelTierAxe, 0.7, 10},
	}
	for _, tc := range cases {
		amplitude, period := ViewmodelSwingParams(tc.tier)
		if amplitude != tc.amplitude || period != tc.periodTicks {
			t.Fatalf("档位 %d 参数 = (%v,%d)，想要 (%v,%d)", tc.tier, amplitude, period, tc.amplitude, tc.periodTicks)
		}
		if amplitude <= 0 || period == 0 {
			t.Fatalf("档位 %d 参数非法：摆幅必须为正、周期非零", tc.tier)
		}
	}
}

func TestViewmodelSwordAndBlockAmplitudesDiffer(t *testing.T) {
	swordAmp, _ := ViewmodelSwingParams(ViewmodelTierSword)
	blockAmp, _ := ViewmodelSwingParams(ViewmodelTierBlock)
	if swordAmp == blockAmp {
		t.Fatalf("剑与方块摆幅相等（%v），相同相位下旋转角将无法区分", swordAmp)
	}
}

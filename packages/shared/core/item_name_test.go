package core

import (
	"strings"
	"testing"
)

// TestItemDisplayNameCoversEveryRegisteredItem 锁定弹条与 tooltip 共用的显示名
// 来源全量无缺名：凡已注册物品（`RegisteredItem` 为真）都必须拿到非空中文名，
// 未注册编号（含零值 `ItemNone` 与哨兵之后的越界值）必须返回 false——调用方
// 依赖「缺名即不显示」的布尔边界，绝不能靠空字符串区分。
func TestItemDisplayNameCoversEveryRegisteredItem(t *testing.T) {
	for item := ItemNone; item < ItemIDMax; item++ {
		name, ok := ItemDisplayName(item)
		if !RegisteredItem(item) {
			if ok {
				t.Fatalf("未注册物品 %d 拿到显示名 %q，想要 false", item, name)
			}
			continue
		}
		if !ok || strings.TrimSpace(name) == "" {
			t.Fatalf("已注册物品 %d 缺少中文显示名（ok=%v name=%q）", item, ok, name)
		}
	}
	// ItemID 是 uint16，-1 会溢出；用足够大的显式越界值见证哨兵之外返回 false。
	for _, item := range []ItemID{ItemIDMax, ItemIDMax + 1, ItemIDMax + 64, 65535} {
		if name, ok := ItemDisplayName(item); ok {
			t.Fatalf("越界物品 %d 拿到显示名 %q，想要 false", item, name)
		}
	}
}

// TestItemDisplayNameFallsBackToBlockName 锁定「能放置完整立方体方块的物品
// 回退该方块显示名」的来源契约：这些物品不进显式表，名字必须与
// `BlockDisplayName` 逐字节一致，避免同一方块在方块侧与物品侧出现两个名字。
// 形态分歧的放置物（门/床/火把/作物阶段）的方块名是形态名而非物品名，由
// 显式表覆盖，不在本断言之列。
func TestItemDisplayNameFallsBackToBlockName(t *testing.T) {
	for _, test := range []struct {
		item  ItemID
		block BlockID
	}{
		{ItemStone, StoneID},
		{ItemDirt, DirtID},
		{ItemGrass, GrassID},
		{ItemStoneBrick, StoneBrickID},
		{ItemFurnace, FurnaceID},
		{ItemIronBlock, IronBlockID},
		{ItemChest, ChestID},
		{ItemLightBlock, LightBlockID},
		{ItemCobblestone, CobblestoneID},
		{ItemSmoothStone, SmoothStoneID},
		{ItemSand, SandID},
		{ItemGravel, GravelID},
		{ItemOakLog, OakLogID},
		{ItemOakPlanks, OakPlanksID},
		{ItemLeaves, LeavesID},
		{ItemGlass, GlassID},
		{ItemBrick, BrickID},
		{ItemWhiteWool, WhiteWoolID},
		{ItemRoofTile, RoofTileID},
		{ItemClay, ClayID},
		{ItemSnowBlock, SnowBlockID},
		{ItemMossyCobblestone, MossyCobblestoneID},
		{ItemWorkbench, WorkbenchID},
	} {
		blockName, ok := BlockDisplayName(test.block)
		if !ok {
			t.Fatalf("方块 %d 缺少显示名，回退来源失效", test.block)
		}
		if name, ok := ItemDisplayName(test.item); !ok || name != blockName {
			t.Fatalf("物品 %d 显示名=%q（ok=%v），想要方块回退 %q", test.item, name, ok, blockName)
		}
	}
}

// TestItemDisplayNameExplicitNamesAreStable 锁定非方块物品的显式表：这些名字
// 会出现在玩家可见的弹条与 tooltip 里，措辞变化即呈现变化，必须显式审视。
func TestItemDisplayNameExplicitNamesAreStable(t *testing.T) {
	for _, test := range []struct {
		item ItemID
		want string
	}{
		{ItemCoal, "煤炭"},
		{ItemRawIron, "粗铁"},
		{ItemIronIngot, "铁锭"},
		{ItemStonePickaxe, "石镐"},
		{ItemIronPickaxe, "铁镐"},
		{ItemBrokenStonePickaxe, "损坏的石镐"},
		{ItemBrokenIronPickaxe, "损坏的铁镐"},
		{ItemStoneHoe, "石锄"},
		{ItemIronHoe, "铁锄"},
		{ItemBrokenStoneHoe, "损坏的石锄"},
		{ItemBrokenIronHoe, "损坏的铁锄"},
		{ItemWheatSeeds, "小麦种子"},
		{ItemWheat, "小麦"},
		{ItemBread, "面包"},
		{ItemStick, "木棍"},
		{ItemBoneMeal, "骨粉"},
		{ItemPotato, "马铃薯"},
		{ItemCarrot, "胡萝卜"},
		{ItemPoisonousPotato, "毒马铃薯"},
		{ItemDoor, "木门"},
		{ItemTorch, "火把"},
		{ItemBed, "床"},
		{ItemRottenFlesh, "腐肉"},
	} {
		if name, ok := ItemDisplayName(test.item); !ok || name != test.want {
			t.Fatalf("物品 %d 显示名=%q（ok=%v），想要 %q", test.item, name, ok, test.want)
		}
	}
}

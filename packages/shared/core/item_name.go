package core

// itemDisplayNames 是不能回退方块名的物品的显式中文显示名表，客户端弹条与
// tooltip 同源消费，不新增 wire 字段。
//
// 入表的是两类物品：
//
//   - 非方块物品（工具、食物、材料等）：没有可回退的方块；
//   - 放置形态与「物品名」分歧的放置物（门/床/火把/作物阶段）：它们的
//     `ItemPlacement` 目标是形态方块，`BlockDisplayName` 给出的是
//     「木门下半_南关」这类形态名而非玩家口中的物品名，回退反而误导。
//
// 完整立方体类放置物（石头、熔炉、箱子等）刻意不入表，走 `ItemPlacement`
// 回退 `BlockDisplayName`，由 `item_name_test.go` 锁定两侧同源。
var itemDisplayNames = map[ItemID]string{
	ItemCoal:               "煤炭",
	ItemRawIron:            "粗铁",
	ItemIronIngot:          "铁锭",
	ItemStonePickaxe:       "石镐",
	ItemIronPickaxe:        "铁镐",
	ItemBrokenStonePickaxe: "损坏的石镐",
	ItemBrokenIronPickaxe:  "损坏的铁镐",
	ItemStoneHoe:           "石锄",
	ItemIronHoe:            "铁锄",
	ItemBrokenStoneHoe:     "损坏的石锄",
	ItemBrokenIronHoe:      "损坏的铁锄",
	ItemWheatSeeds:         "小麦种子",
	ItemWheat:              "小麦",
	ItemBread:              "面包",
	ItemStick:              "木棍",
	ItemBoneMeal:           "骨粉",
	ItemPotato:             "马铃薯",
	ItemCarrot:             "胡萝卜",
	ItemPoisonousPotato:    "毒马铃薯",
	ItemDoor:               "木门",
	ItemTorch:              "火把",
	ItemBed:                "床",
	ItemRottenFlesh:        "腐肉",
	ItemRawBeef:            "生牛肉",
	ItemCookedBeef:         "熟牛肉",
	ItemWoodenSword:        "木剑",
	ItemStoneSword:         "石剑",
	ItemIronSword:          "铁剑",
	ItemBrokenWoodenSword:  "损坏的木剑",
	ItemBrokenStoneSword:   "损坏的石剑",
	ItemBrokenIronSword:    "损坏的铁剑",
}

// ItemDisplayName 返回物品的中文显示名，供客户端物品名弹条与容器 tooltip
// 消费。解析顺序：先查显式表，再对可放置物品回退目标方块的
// `BlockDisplayName`；两者都缺省（未注册物品、空栏位）返回 false，调用方
// 按「缺名即不显示」处理，不得用空字符串充当显示名。
func ItemDisplayName(id ItemID) (string, bool) {
	if name, ok := itemDisplayNames[id]; ok {
		return name, true
	}
	block, ok := ItemPlacement(id)
	if !ok {
		return "", false
	}
	return BlockDisplayName(block)
}

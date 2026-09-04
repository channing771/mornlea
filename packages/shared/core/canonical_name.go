package core

// canonicalBlockNames 是 machine-facing 方块英文名的唯一注册表。数组索引就是
// 稳定 `BlockID`；中文显示名仍由 `BlockDisplayName` 独立服务 UI。
var canonicalBlockNames = [...]string{
	AirID:                "air",
	BarrierID:            "barrier",
	StoneID:              "stone",
	DirtID:               "dirt",
	GrassID:              "grass",
	BedrockID:            "bedrock",
	StoneBrickID:         "stone_brick",
	CoalOreID:            "coal_ore",
	IronOreID:            "iron_ore",
	FurnaceID:            "furnace",
	IronBlockID:          "iron_block",
	ChestID:              "chest",
	LightBlockID:         "light_block",
	CobblestoneID:        "cobblestone",
	SmoothStoneID:        "smooth_stone",
	SandID:               "sand",
	GravelID:             "gravel",
	OakLogID:             "oak_log",
	OakPlanksID:          "oak_planks",
	LeavesID:             "leaves",
	GlassID:              "glass",
	BrickID:              "brick",
	WhiteWoolID:          "white_wool",
	RoofTileID:           "roof_tile",
	ClayID:               "clay",
	SnowBlockID:          "snow_block",
	MossyCobblestoneID:   "mossy_cobblestone",
	WaterSourceID:        "water_source",
	WaterLevel1ID:        "water_level_1",
	WaterLevel2ID:        "water_level_2",
	WaterLevel3ID:        "water_level_3",
	WaterLevel4ID:        "water_level_4",
	WaterLevel5ID:        "water_level_5",
	WaterLevel6ID:        "water_level_6",
	WaterLevel7ID:        "water_level_7",
	FarmlandDryID:        "farmland_dry",
	FarmlandWetID:        "farmland_wet",
	WheatStage0ID:        "wheat_stage_0",
	WheatStage1ID:        "wheat_stage_1",
	WheatStage2ID:        "wheat_stage_2",
	WheatStage3ID:        "wheat_stage_3",
	WheatStage4ID:        "wheat_stage_4",
	WheatStage5ID:        "wheat_stage_5",
	WheatStage6ID:        "wheat_stage_6",
	WheatStage7ID:        "wheat_stage_7",
	WorkbenchID:          "workbench",
	PotatoStage0ID:       "potato_stage_0",
	PotatoStage1ID:       "potato_stage_1",
	PotatoStage2ID:       "potato_stage_2",
	PotatoStage3ID:       "potato_stage_3",
	PotatoStage4ID:       "potato_stage_4",
	PotatoStage5ID:       "potato_stage_5",
	PotatoStage6ID:       "potato_stage_6",
	PotatoStage7ID:       "potato_stage_7",
	CarrotStage0ID:       "carrot_stage_0",
	CarrotStage1ID:       "carrot_stage_1",
	CarrotStage2ID:       "carrot_stage_2",
	CarrotStage3ID:       "carrot_stage_3",
	CarrotStage4ID:       "carrot_stage_4",
	CarrotStage5ID:       "carrot_stage_5",
	CarrotStage6ID:       "carrot_stage_6",
	CarrotStage7ID:       "carrot_stage_7",
	DoorLowerSouthClosed: "door_lower_south_closed",
	DoorLowerSouthOpen:   "door_lower_south_open",
	DoorLowerWestClosed:  "door_lower_west_closed",
	DoorLowerWestOpen:    "door_lower_west_open",
	DoorLowerNorthClosed: "door_lower_north_closed",
	DoorLowerNorthOpen:   "door_lower_north_open",
	DoorLowerEastClosed:  "door_lower_east_closed",
	DoorLowerEastOpen:    "door_lower_east_open",
	DoorUpper:            "door_upper",
	TorchStandingID:      "torch_standing",
	TorchWallPosXID:      "torch_wall_pos_x",
	TorchWallNegXID:      "torch_wall_neg_x",
	TorchWallPosZID:      "torch_wall_pos_z",
	TorchWallNegZID:      "torch_wall_neg_z",
	BedFootSouthID:       "bed_foot_south",
	BedFootWestID:        "bed_foot_west",
	BedFootNorthID:       "bed_foot_north",
	BedFootEastID:        "bed_foot_east",
	BedHeadSouthID:       "bed_head_south",
	BedHeadWestID:        "bed_head_west",
	BedHeadNorthID:       "bed_head_north",
	BedHeadEastID:        "bed_head_east",
	ShortGrassID:         "short_grass",
}

// explicitCanonicalItemNames 只登记没有同名完整方块的物品。完整方块物品通过
// `ItemPlacement` 复用目标方块名，避免再维护一张平行的英文字符串白名单。
var explicitCanonicalItemNames = map[ItemID]string{
	ItemCoal:               "coal",
	ItemRawIron:            "raw_iron",
	ItemIronIngot:          "iron_ingot",
	ItemStonePickaxe:       "stone_pickaxe",
	ItemIronPickaxe:        "iron_pickaxe",
	ItemBrokenStonePickaxe: "broken_stone_pickaxe",
	ItemBrokenIronPickaxe:  "broken_iron_pickaxe",
	ItemStoneHoe:           "stone_hoe",
	ItemIronHoe:            "iron_hoe",
	ItemBrokenStoneHoe:     "broken_stone_hoe",
	ItemBrokenIronHoe:      "broken_iron_hoe",
	ItemWheatSeeds:         "wheat_seeds",
	ItemWheat:              "wheat",
	ItemBread:              "bread",
	ItemStick:              "stick",
	ItemBoneMeal:           "bone_meal",
	ItemPotato:             "potato",
	ItemCarrot:             "carrot",
	ItemPoisonousPotato:    "poisonous_potato",
	ItemDoor:               "door",
	ItemTorch:              "torch",
	ItemRottenFlesh:        "rotten_flesh",
	ItemBed:                "bed",
	ItemRawBeef:            "raw_beef",
	ItemCookedBeef:         "cooked_beef",
	ItemWoodenSword:        "wooden_sword",
	ItemStoneSword:         "stone_sword",
	ItemIronSword:          "iron_sword",
	ItemBrokenWoodenSword:  "broken_wooden_sword",
	ItemBrokenStoneSword:   "broken_stone_sword",
	ItemBrokenIronSword:    "broken_iron_sword",
}

var canonicalItemNames = buildCanonicalItemNames()
var canonicalBlockIDs = buildCanonicalBlockIDs()
var canonicalItemIDs = buildCanonicalItemIDs()

// CanonicalBlockName 返回已注册方块的小写 ASCII machine name。未知编号不生成
// 数值、中文或 `unknown_*` 回退名。
func CanonicalBlockName(id BlockID) (string, bool) {
	if !RegisteredBlock(id) || int(id) >= len(canonicalBlockNames) {
		return "", false
	}
	name := canonicalBlockNames[id]
	return name, name != ""
}

// BlockIDByCanonicalName 把精确 machine name 反查为方块编号。大小写、空白或
// 未注册拼写都 fail closed。
func BlockIDByCanonicalName(name string) (BlockID, bool) {
	id, ok := canonicalBlockIDs[name]
	return id, ok
}

// CanonicalItemName 返回非空已注册物品的小写 ASCII machine name。空物品和
// 未知编号不生成回退名。
func CanonicalItemName(id ItemID) (string, bool) {
	if id == ItemNone || id >= ItemIDMax {
		return "", false
	}
	name := canonicalItemNames[id]
	return name, name != ""
}

// ItemIDByCanonicalName 把精确 machine name 反查为非空物品编号。
func ItemIDByCanonicalName(name string) (ItemID, bool) {
	id, ok := canonicalItemIDs[name]
	return id, ok
}

func buildCanonicalItemNames() [ItemIDMax]string {
	var names [ItemIDMax]string
	for id := ItemID(1); id < ItemIDMax; id++ {
		if name, ok := explicitCanonicalItemNames[id]; ok {
			names[id] = name
			continue
		}
		block, ok := ItemPlacement(id)
		if !ok {
			continue
		}
		name, ok := CanonicalBlockName(block)
		if ok {
			names[id] = name
		}
	}
	return names
}

func buildCanonicalBlockIDs() map[string]BlockID {
	ids := make(map[string]BlockID, BlockIDMax)
	for id := BlockID(0); id < BlockIDMax; id++ {
		if name, ok := CanonicalBlockName(id); ok {
			ids[name] = id
		}
	}
	return ids
}

func buildCanonicalItemIDs() map[string]ItemID {
	ids := make(map[string]ItemID, ItemIDMax-1)
	for id := ItemID(1); id < ItemIDMax; id++ {
		if name, ok := CanonicalItemName(id); ok {
			ids[name] = id
		}
	}
	return ids
}

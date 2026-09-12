package core

// ArmorSlot 是护甲槽位编号：头/胸/腿/脚四槽的顺序是协议稳定值，新槽位只能
// 追加在 `ArmorSlotCount` 哨兵之前（与 `ItemIDMax` 同形的哨兵纪律），插入或
// 重排会平移后续槽位编号、破坏穿戴状态的线上与存档字节。
type ArmorSlot uint8

const (
	ArmorSlotHead ArmorSlot = iota
	ArmorSlotChest
	ArmorSlotLegs
	ArmorSlotFeet
	// ArmorSlotCount 是合法槽位的独占上界（末槽 + 1），本身不是槽位枚举
	// 成员；它同时是穿戴数组的固定长度，`ArmorPoints` 的入参以它为长度。
	ArmorSlotCount
)

// MaxArmorPoints 是四槽护甲点数之和的截断上限。当前铁质一套合计 15 点不会
// 触顶，上限刻意取 20 为更高阶护甲留位；协议侧点数字段的合法上界同为此值。
const MaxArmorPoints uint8 = 20

// 铁质护甲的耐久上限是 armor 域的单一真源值：`ItemMaxDurability` 登记四件
// 护甲时引用这里的常量，护甲配方产物耐久同样引用，任何其他位置不得复制
// 数值。护甲的磨损口径是「每次产生减免的近战受击每件完好护甲恰扣 1 点」，
// 数值只表达相对耐磨度，与剑/镐耐久数值同一整数口径。
const (
	ironHelmetMaxDurability     uint16 = 165
	ironChestplateMaxDurability uint16 = 240
	ironLeggingsMaxDurability   uint16 = 225
	ironBootsMaxDurability      uint16 = 195
)

// armorPiece 是一件护甲的域内属性：唯一归属槽位、护甲点数与耐久上限。
type armorPiece struct {
	slot       ArmorSlot
	points     uint8
	durability uint16
}

// armorPieceOf 是护甲件 → 域属性的单一真源表：件→槽映射、点数与耐久上限
// 只允许从本表读取。其他包经 `ArmorSlotOf` / `ArmorPoints` 间接消费，同包的
// `ItemMaxDurability` 与护甲配方产物耐久引用同表常量，禁止任何位置复制表值。
func armorPieceOf(item ItemID) (armorPiece, bool) {
	switch item {
	case ItemIronHelmet:
		return armorPiece{slot: ArmorSlotHead, points: 2, durability: ironHelmetMaxDurability}, true
	case ItemIronChestplate:
		return armorPiece{slot: ArmorSlotChest, points: 6, durability: ironChestplateMaxDurability}, true
	case ItemIronLeggings:
		return armorPiece{slot: ArmorSlotLegs, points: 5, durability: ironLeggingsMaxDurability}, true
	case ItemIronBoots:
		return armorPiece{slot: ArmorSlotFeet, points: 2, durability: ironBootsMaxDurability}, true
	default:
		return armorPiece{}, false
	}
}

// ArmorSlotOf 返回护甲件唯一归属的槽位；非护甲物品返回 false。装备动作以此
// 映射确定目标槽位：四件映射四槽各一、绝不重叠，任何放置方不得自建映射。
func ArmorSlotOf(item ItemID) (ArmorSlot, bool) {
	piece, ok := armorPieceOf(item)
	if !ok {
		return 0, false
	}
	return piece.slot, true
}

// ArmorPoints 求和四槽穿戴中完好护甲件的点数并截断到 `MaxArmorPoints`。
//
// 完好判定沿用既有耐久机制：护甲件以「数量 1、耐久 1..上限」的物品栈表达
// 完好；耐久归零即损坏形态——与损坏剑同族，损坏件保留在槽内、不消失，只是
// 贡献 0 点。耐久越出 1..上限或数量为零的栈不是合法持有的完好件，一律按
// 0 点处理（与 `ItemStack.Valid` 的耐久域同口径，fail closed）。当前铁质
// 满套合计 15 点不会触顶，截断分支为未来更高阶护甲保留。
func ArmorPoints(worn [ArmorSlotCount]ItemStack) uint8 {
	total := 0
	for _, stack := range worn {
		piece, ok := armorPieceOf(stack.Item)
		if !ok {
			continue
		}
		if stack.Count < 1 || stack.Durability < 1 || stack.Durability > piece.durability {
			continue
		}
		total += int(piece.points)
	}
	if total > int(MaxArmorPoints) {
		return MaxArmorPoints
	}
	return uint8(total)
}

// ReducedDamage 按护甲点数计算近战有效伤害：每点护甲减免 4% 原始伤害，
// 有效伤害下限为 1——任何点数组合都不得把一次受击减到 0 点。
//
// damage MUST 为正：非正伤害没有游戏语义，防御性返回 0，调用方须自行兜底。
// 乘除为整数运算、除法向零取整；中间量取更宽的 int64 再收敛回 int32，避免
// 极端伤害值在 int32 域乘法回绕破坏确定性——对一切常规输入，结果与纯
// int32 整数运算逐位一致。
func ReducedDamage(damage int32, points uint8) int32 {
	if damage <= 0 {
		return 0
	}
	reduced := int64(damage) * (100 - 4*int64(points)) / 100
	if reduced < 1 {
		return 1
	}
	return int32(reduced)
}

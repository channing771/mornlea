package core

// BlockID 是跨客户端/服务端稳定的全局方块编号。
type BlockID uint16

// DimensionID 标识一个世界维度。
type DimensionID int32

// BlockFace 标识方块的六个轴对齐面。
type BlockFace uint8

// ChunkKey 在指定维度中唯一定位一个区块。
type ChunkKey struct {
	Dimension DimensionID
	Pos       ChunkPos
}

// SectionKey 在指定维度中唯一定位一个区段。
type SectionKey struct {
	Dimension DimensionID
	Pos       SectionPos
}

// Overworld 是 M2A 使用的主世界维度。
const Overworld DimensionID = 0

// 方块 ID 是协议稳定值，不能重排。
const (
	AirID BlockID = iota
	BarrierID
	StoneID
	DirtID
	GrassID
	BedrockID
	StoneBrickID
	CoalOreID
	IronOreID
	FurnaceID
	IronBlockID
	ChestID
	LightBlockID
	CobblestoneID
	SmoothStoneID
	SandID
	GravelID
	OakLogID
	OakPlanksID
	LeavesID
	GlassID
	BrickID
	WhiteWoolID
	RoofTileID
	ClayID
	SnowBlockID
	MossyCobblestoneID
	// 以下 8 个是流体方块编号，只能追加在 MossyCobblestoneID 之后：方块 ID 是
	// 协议稳定值，重排会破坏既有存档与线上字节。WaterSourceID 是水的源方块
	// （满格，流动规则下永不自然消失）；WaterLevel1ID..WaterLevel7ID 是流动水，
	// 数字越小水量越强（1 最强、7 最弱），符合 Minecraft 系水流传统语义。
	WaterSourceID
	WaterLevel1ID
	WaterLevel2ID
	WaterLevel3ID
	WaterLevel4ID
	WaterLevel5ID
	WaterLevel6ID
	WaterLevel7ID
	// 以下农业方块编号（2 耕地 + 8 小麦 + 8 马铃薯 + 8 胡萝卜）与工作台方块
	// 只能追加在 WaterLevel7ID 之后：方块 ID 是协议稳定值，重排会破坏既有存档与
	// 线上字节。农业 26 个 + 工作台 1 个 = 27 个，`BlockIDMax` 恒居末。
	//
	// FarmlandDryID / FarmlandWetID 是锄头翻地得到的耕地，只有干湿两态：湿润
	// 由附近流体决定（见变更 authoritative-farming 的湿润判定），两态共用同一
	// 掉落（1 泥土），干湿差别只影响作物生长速度与外观。
	FarmlandDryID
	FarmlandWetID
	// WheatStage0ID..WheatStage7ID 是小麦的八个生长阶段，每阶段一个稳定编号
	// （沿用流体「每等级一个编号」的模式）。阶段 0 是刚种下的种子，阶段 7 是
	// 成熟可收获态；编号连续递增即生长方向，因此推进一阶段就是 +1。
	// 把阶段放进方块编号而不是附加状态字节，是为了让区块 schema、采掘表、碰撞
	// 表与 mesh registry 快照天然覆盖农业，不新开任何存储或传输通道。
	WheatStage0ID
	WheatStage1ID
	WheatStage2ID
	WheatStage3ID
	WheatStage4ID
	WheatStage5ID
	WheatStage6ID
	WheatStage7ID
	// WorkbenchID 是工作台方块（格子工作台批次追加，只能追加在 WheatStage7ID
	// 之后）：普通完整立方体、不透明、不发光，可放置可采掘并掉回 1 个工作台
	// 物品。打开工作台只把玩家合成网格的有效尺寸从 2 提到 3——它是普通方块
	// 而不是容器，不占用容器引用或区块槽位，也不持久化任何槽位记录。
	WorkbenchID
	PotatoStage0ID
	PotatoStage1ID
	PotatoStage2ID
	PotatoStage3ID
	PotatoStage4ID
	PotatoStage5ID
	PotatoStage6ID
	PotatoStage7ID
	CarrotStage0ID
	CarrotStage1ID
	CarrotStage2ID
	CarrotStage3ID
	CarrotStage4ID
	CarrotStage5ID
	CarrotStage6ID
	CarrotStage7ID
	// 以下五个是火把方块编号（落地 + 四向墙面形态），只能追加在 CarrotStage7ID
	// 之后：方块 ID 是协议稳定值，重排会破坏既有存档与线上字节。TorchStandingID
	// 是落地形态（支撑格在正下方）；TorchWallPosXID..TorchWallNegZID 是贴墙形态，
	// 形态名与放置时的命中面同名（火把贴在支撑块的哪个侧面），63..66 按
	// +X/−X/+Z/−Z 顺序冻结。墙面形态的支撑格位于火把的命中面反方向
	// （face.Opposite()），由放置执行方消费；命中面 → 形态的唯一映射窗口是
	// PlaceableBlockAtFace。火把按契约不提供碰撞体（physics 侧接线后恒为空）、
	// 非不透明、非流体、发光 14，光照与瞄准属性见 block_properties.go。
	TorchStandingID
	TorchWallPosXID
	TorchWallNegXID
	TorchWallPosZID
	TorchWallNegZID
	// BlockIDMax 是合法方块编号的独占上界（最后一个合法 BlockID + 1），本身不是
	// 方块枚举成员，与物品侧的 ItemIDMax 同形。它供哨兵与穷举测试以
	// 「id < BlockIDMax」表达「全部已注册方块」，替代「某个具体编号恰为枚举末项」
	// 的脆弱写法：后者在追加新编号时会静默退化成子集——历史上以
	// MossyCobblestoneID 为界的哨兵就曾在五个包里失效（其中一处是真实行为回归），
	// 以 WaterLevel7ID 为界的循环上界也让新编号全程漏出门禁。新方块只能追加在
	// 本哨兵之前（哨兵始终紧随末项），farming_test.go 的枚举末项守护断言负责在
	// 追加时报警。
	BlockIDMax
)

// RegisteredBlock 报告 id 是否是已注册的稳定方块编号。
func RegisteredBlock(id BlockID) bool {
	return id < BlockIDMax
}

const (
	BlockFaceNegX BlockFace = iota
	BlockFacePosX
	BlockFaceNegY
	BlockFacePosY
	BlockFaceNegZ
	BlockFacePosZ
	BlockFaceNone BlockFace = 0xff
)

// Opposite 返回相对的方块面。
func (f BlockFace) Opposite() BlockFace {
	if f == BlockFaceNone {
		return BlockFaceNone
	}
	if f > BlockFacePosZ {
		panic("core: invalid BlockFace")
	}
	return f ^ 1
}

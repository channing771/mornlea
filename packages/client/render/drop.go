package render

import (
	"math"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

const (
	// maxItemDrops 是 HUD 之外掉落物渲染的固定 CPU/GPU 容量。
	maxItemDrops = core.MaxSessionDrops

	dropInstanceOffset = 256
	dropInstanceSize   = maxItemDrops * avatarInstanceBytes
	dropIndirectOffset = dropInstanceOffset + dropInstanceSize
	dropUploadBytes    = dropIndirectOffset + avatarIndirectBytes

	dropCubeSize     = float32(0.25)
	dropFloatHeight  = float32(0.08)
	dropSpinPeriod   = 80
	dropFloatPeriod  = 48
	dropBaseAltitude = float32(0.5)
)

// ItemDrop 是一个权威掉落物的渲染输入；位置由方块位置决定，支撑高度由
// app 层从只读镜像算出后随本结构传入，不另开通道。
type ItemDrop struct {
	ID    core.DropID
	Block core.BlockPos
	Item  core.ItemID
	// SupportY 是掉落方块下方首个不透明方块顶面的世界高度（方块 Y+1）。
	// HasSupport 为假表示镜像无数据或支撑超出扫描定界，此时按生成高度保持、
	// 不下落。呈现下落不反馈服务端。
	SupportY   float32
	HasSupport bool
}

type dropPhase struct {
	spin  float32
	float float32
}

// dropAnimationPhase 由 server tick 与稳定 ID 混合得到确定性相位。
func dropAnimationPhase(serverTick uint64, id core.DropID) dropPhase {
	offset := uint64(id.Generation)*31 + uint64(id.Slot)*7 +
		uint64(uint32(id.Chunk.X))*3 + uint64(uint32(id.Chunk.Z))
	spinTick := (serverTick + offset) % dropSpinPeriod
	floatTick := (serverTick + offset) % dropFloatPeriod
	return dropPhase{
		spin:  2 * math.Pi * float32(spinTick) / dropSpinPeriod,
		float: 2 * math.Pi * float32(floatTick) / dropFloatPeriod,
	}
}

// ItemColor 返回 HUD 与掉落物共享的稳定物品基色。
func ItemColor(item core.ItemID) [4]float32 {
	switch item {
	case core.ItemStone:
		return [4]float32{128.0 / 255, 128.0 / 255, 128.0 / 255, 1}
	case core.ItemDirt:
		return [4]float32{134.0 / 255, 96.0 / 255, 67.0 / 255, 1}
	case core.ItemGrass:
		return [4]float32{88.0 / 255, 140.0 / 255, 60.0 / 255, 1}
	case core.ItemStoneBrick:
		return [4]float32{122.0 / 255, 118.0 / 255, 112.0 / 255, 1}
	case core.ItemCoal:
		return [4]float32{38.0 / 255, 38.0 / 255, 40.0 / 255, 1}
	case core.ItemRawIron:
		return [4]float32{196.0 / 255, 154.0 / 255, 118.0 / 255, 1}
	case core.ItemIronIngot:
		return [4]float32{220.0 / 255, 220.0 / 255, 224.0 / 255, 1}
	case core.ItemFurnace:
		return [4]float32{88.0 / 255, 86.0 / 255, 88.0 / 255, 1}
	case core.ItemIronBlock:
		return [4]float32{214.0 / 255, 214.0 / 255, 216.0 / 255, 1}
	case core.ItemChest:
		return [4]float32{156.0 / 255, 108.0 / 255, 58.0 / 255, 1}
	case core.ItemStonePickaxe:
		return [4]float32{104.0 / 255, 112.0 / 255, 120.0 / 255, 1}
	case core.ItemIronPickaxe:
		return [4]float32{190.0 / 255, 198.0 / 255, 210.0 / 255, 1}
	case core.ItemBrokenStonePickaxe:
		return [4]float32{66.0 / 255, 60.0 / 255, 58.0 / 255, 1}
	case core.ItemBrokenIronPickaxe:
		return [4]float32{96.0 / 255, 88.0 / 255, 92.0 / 255, 1}
	case core.ItemWoodenSword:
		return [4]float32{184.0 / 255, 134.0 / 255, 72.0 / 255, 1}
	case core.ItemStoneSword:
		return [4]float32{148.0 / 255, 148.0 / 255, 148.0 / 255, 1}
	case core.ItemIronSword:
		return [4]float32{200.0 / 255, 205.0 / 255, 210.0 / 255, 1}
	case core.ItemBrokenWoodenSword:
		return [4]float32{92.0 / 255, 67.0 / 255, 33.0 / 255, 1}
	case core.ItemBrokenStoneSword:
		return [4]float32{82.0 / 255, 82.0 / 255, 80.0 / 255, 1}
	case core.ItemBrokenIronSword:
		return [4]float32{115.0 / 255, 120.0 / 255, 125.0 / 255, 1}
	case core.ItemRawBeef:
		return [4]float32{152.0 / 255, 76.0 / 255, 55.0 / 255, 1}
	case core.ItemCookedBeef:
		return [4]float32{78.0 / 255, 55.0 / 255, 33.0 / 255, 1}
	default:
		return [4]float32{}
	}
}

// itemDropColor 复用与程序化方块一致的稳定基色。
func itemDropColor(item core.ItemID) ([4]float32, bool) {
	if !core.RegisteredItem(item) {
		return [4]float32{}, false
	}
	return ItemColor(item), true
}

// ItemDropBlock 把掉落物的区块内索引还原为世界方块位置。
func ItemDropBlock(chunk core.ChunkPos, blockIndex uint32) (core.BlockPos, bool) {
	return world.BlockPosFromChunkIndex(chunk, blockIndex)
}

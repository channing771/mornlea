package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
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

// ItemDrop 是一个权威掉落物的渲染输入；位置由方块位置决定。
type ItemDrop struct {
	ID    core.DropID
	Block core.BlockPos
	Item  core.ItemID
}

func buildItemDropParts(dst []avatarPart, serverTick uint64, drops []ItemDrop) []avatarPart {
	for _, drop := range drops {
		if len(dst) == maxItemDrops {
			break
		}
		material, ok := itemDropMaterial(drop.Item)
		if !ok {
			continue
		}
		phase := dropAnimationPhase(serverTick, drop.ID)
		center := mgl32.Vec3{
			float32(drop.Block.X) + 0.5,
			float32(drop.Block.Y) + dropBaseAltitude +
				dropFloatHeight*float32(math.Sin(float64(phase.float))),
			float32(drop.Block.Z) + 0.5,
		}
		transform := mgl32.Translate3D(center.X(), center.Y(), center.Z()).
			Mul4(mgl32.HomogRotate3DY(phase.spin)).
			Mul4(mgl32.Scale3D(dropCubeSize, dropCubeSize, dropCubeSize))
		// 贴图分支忽略颜色通道（avatar.wgsl），填中性白保持编码确定。
		dst = append(dst, avatarPart{transform: transform, color: [4]float32{1, 1, 1, 1}, material: material})
	}
	return dst
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

// itemDropMaterial 把掉落物品映射到材质 atlas 的采样层：可放置物品取注册表
// 顶面代表层（与世界同格同源，见 `TestItemDropMaterialsMatchRegistryTopFace`
// 的同源断言）；种子/马铃薯/胡萝卜取成熟层（stage0 仅约 4% 像素覆盖，在小
// 方块上近乎不可见）；矿石产物取同源方块族层；食物取牛肉/小麦层；木棍/骨粉
// 取合成与颜色家族层；工具按配方材质族（石→卵石、铁→铁块、木→木板）取层。
// 未注册物品返回 false（不可见，与变更前一致）。新增可掉落物品必须同步本
// 表，否则 `TestItemDropMaterialCoversAllRegisteredItems` 即红。
func itemDropMaterial(item core.ItemID) (uint32, bool) {
	switch item {
	case core.ItemStone:
		return uint32(assets.LayerStone), true
	case core.ItemDirt:
		return uint32(assets.LayerDirt), true
	case core.ItemGrass:
		return uint32(assets.LayerGrassTop), true
	case core.ItemStoneBrick:
		return uint32(assets.LayerStoneBrick), true
	case core.ItemFurnace:
		return uint32(assets.LayerFurnace), true
	case core.ItemIronBlock:
		return uint32(assets.LayerIronBlock), true
	case core.ItemChest:
		return uint32(assets.LayerChest), true
	case core.ItemLightBlock:
		return uint32(assets.LayerLightBlock), true
	case core.ItemCobblestone:
		return uint32(assets.LayerCobblestone), true
	case core.ItemSmoothStone:
		return uint32(assets.LayerSmoothStone), true
	case core.ItemSand:
		return uint32(assets.LayerSand), true
	case core.ItemGravel:
		return uint32(assets.LayerGravel), true
	case core.ItemOakLog:
		return uint32(assets.LayerOakLogTop), true
	case core.ItemOakPlanks:
		return uint32(assets.LayerOakPlanks), true
	case core.ItemLeaves:
		return uint32(assets.LayerLeaves), true
	case core.ItemGlass:
		return uint32(assets.LayerGlass), true
	case core.ItemBrick:
		return uint32(assets.LayerBrick), true
	case core.ItemWhiteWool:
		return uint32(assets.LayerWhiteWool), true
	case core.ItemRoofTile:
		return uint32(assets.LayerRoofTile), true
	case core.ItemClay:
		return uint32(assets.LayerClay), true
	case core.ItemSnowBlock:
		return uint32(assets.LayerSnowTop), true
	case core.ItemMossyCobblestone:
		return uint32(assets.LayerMossyCobblestone), true
	case core.ItemWorkbench:
		return uint32(assets.LayerWorkbenchTop), true
	case core.ItemDoor:
		return uint32(assets.LayerDoor), true
	case core.ItemBed:
		return uint32(assets.LayerBedFootSouth), true
	case core.ItemTorch:
		return uint32(assets.LayerTorch), true
	case core.ItemCoal:
		return uint32(assets.LayerCoalOre), true
	case core.ItemRawIron:
		return uint32(assets.LayerIronOre), true
	case core.ItemIronIngot:
		return uint32(assets.LayerIronBlock), true
	case core.ItemRawBeef:
		return uint32(assets.LayerRawBeef), true
	case core.ItemCookedBeef:
		return uint32(assets.LayerCookedBeef), true
	case core.ItemWheat, core.ItemWheatSeeds, core.ItemBread:
		return uint32(assets.LayerWheat7), true
	case core.ItemPotato, core.ItemPoisonousPotato:
		return uint32(assets.LayerPotato7), true
	case core.ItemCarrot:
		return uint32(assets.LayerCarrot7), true
	case core.ItemRottenFlesh:
		return uint32(assets.LayerCookedBeef), true
	case core.ItemStick:
		return uint32(assets.LayerOakPlanks), true
	case core.ItemBoneMeal:
		return uint32(assets.LayerWhiteWool), true
	case core.ItemStonePickaxe, core.ItemBrokenStonePickaxe,
		core.ItemStoneHoe, core.ItemBrokenStoneHoe,
		core.ItemStoneSword, core.ItemBrokenStoneSword:
		return uint32(assets.LayerCobblestone), true
	case core.ItemIronPickaxe, core.ItemBrokenIronPickaxe,
		core.ItemIronHoe, core.ItemBrokenIronHoe,
		core.ItemIronSword, core.ItemBrokenIronSword:
		return uint32(assets.LayerIronBlock), true
	case core.ItemWoodenSword, core.ItemBrokenWoodenSword:
		return uint32(assets.LayerOakPlanks), true
	default:
		return 0, false
	}
}

// ItemDropBlock 把掉落物的区块内索引还原为世界方块位置。
func ItemDropBlock(chunk core.ChunkPos, blockIndex uint32) (core.BlockPos, bool) {
	return world.BlockPosFromChunkIndex(chunk, blockIndex)
}

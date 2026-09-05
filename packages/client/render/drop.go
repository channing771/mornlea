package render

import (
	"math"

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
	// dropFlakeSize/dropFlakeThin 是非方块掉落薄片的平面边长与厚度：竖立的
	// 单张贴图牌（约 1/2 缩放），平面竖直（宽与高为边长、厚度沿前后），绕 Y
	// 旋转；前后大面采样同一层，双面同图；实例仍是立方体图元，零管线变更。
	dropFlakeSize = float32(0.5)
	dropFlakeThin = float32(0.02)
)

// ItemDrop 是一个权威掉落物的渲染输入；位置由方块位置决定，支撑高度由
// app 层从只读镜像算出后随本结构传入，不另开通道。`DeathTick` 是可选的关联
// 死亡 tick（0 表示未关联）：关联掉落在死亡相位 50% 前不渲染，50% 起 scale-in
// 渐显并叠一次白色闪光；纯呈现启发式，拾取走权威不受影响。
type ItemDrop struct {
	ID    core.DropID
	Block core.BlockPos
	Item  core.ItemID
	// SupportY 是掉落方块下方首个不透明方块顶面的世界高度（方块 Y+1）。
	// HasSupport 为假表示镜像无数据或支撑超出扫描定界，此时按生成高度保持、
	// 不下落。呈现下落不反馈服务端。
	SupportY   float32
	HasSupport bool
	// DeathTick 为零表示未关联死亡；非零时呈现侧按死亡相位滞后渐显。
	DeathTick uint64
}

// deathLinkedDropAppearance 计算关联掉落在当前权威 tick 的呈现：相位 50% 前
// 不可见，50% 起以 scale-in 渐显（首现约一成、保留末长满），首现 3 tick 内叠
// 一次白色闪光。返回值是（可见、缩放进度 0..1、白闪）。
func deathLinkedDropAppearance(serverTick, deathTick uint64) (bool, float32, bool) {
	const half = PassiveDeathTicks / 2
	var elapsed uint64
	if serverTick > deathTick {
		elapsed = serverTick - deathTick
	}
	if elapsed < half {
		return false, 0, false
	}
	progress := float32(elapsed-half+1) / float32(half+1)
	if progress > 1 {
		progress = 1
	}
	return true, progress, elapsed-half < 3
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

// itemDropMaterial 把掉落物品映射到材质 atlas 的采样层：可放置物品取注册表
// 顶面代表层（与世界同格同源，见 `TestItemDropMaterialsMatchRegistryTopFace`
// 的同源断言）；透明轮廓物品直接复用 UI 图标所在层，世界薄片与栏位因此采样
// 同一份像素。未注册物品返回 false（不可见，与变更前一致）。新增可掉落物品
// 必须进入方块放置表或 `ItemIconLayer`，否则全覆盖测试即红。
func itemDropMaterial(item core.ItemID) (uint32, bool) {
	if layer, ok := assets.ItemIconLayer(item); ok {
		return uint32(layer), true
	}
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
	default:
		return 0, false
	}
}

// itemDropFlake 报告掉落物是否按非方块薄片呈现：不可放置的物品（食物/工
// 具/火把等）一律薄片；可放置但放置体不是完整立方体的（作物/门/床）同样薄
// 片，其余方块类保持迷你立方体。火把不在 `ItemPlacement` 表里（形态经
// `PlaceableBlockAtFace` 按命中面选择），天然落入薄片分支。
func itemDropFlake(item core.ItemID) bool {
	switch item {
	case core.ItemWheatSeeds, core.ItemPotato, core.ItemCarrot, core.ItemDoor, core.ItemBed:
		return true
	}
	_, ok := core.ItemPlacement(item)
	return !ok
}

// ItemDropBlock 把掉落物的区块内索引还原为世界方块位置。
func ItemDropBlock(chunk core.ChunkPos, blockIndex uint32) (core.BlockPos, bool) {
	return world.BlockPosFromChunkIndex(chunk, blockIndex)
}

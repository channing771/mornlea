package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是第一人称双手 viewmodel 的 CPU 编码：屏幕左右两侧的双手与右手
// 持物，几何与颜色同第三人称手臂同源，全部相位由权威 tick 与触发沿派生，
// 不读墙钟。输出复用 avatar 实例布局（96 字节/实例），绘制由 Rust 渲染器
// 承担；帧 TLV 装配不在本文件，属于跨语言帧编码的职责。

const (
	// ViewmodelMaxInstances 是单帧 viewmodel 实例恒定上限：左手、右手、
	// 持物各一，另保留一位给副手扩展占位。
	ViewmodelMaxInstances = 4
	// ViewmodelAttackFrames 是命中确认后的攻击挥动窗（帧）：第 1 帧起挥，
	// 第 6 帧后回中立持握，与既有命中 marker 的 6 帧语义同源。
	ViewmodelAttackFrames = 6
)

// ViewmodelHeldKind 是右手持物三形态：只由已确认选中槽决定。
type ViewmodelHeldKind uint8

const (
	// ViewmodelHeldNone 表示无持物：空槽或未注册物品，只有右手。
	ViewmodelHeldNone ViewmodelHeldKind = iota
	// ViewmodelHeldBlock 表示手持方块：微缩立方。
	ViewmodelHeldBlock
	// ViewmodelHeldItem 表示手持物品：扁长条程序化几何。
	ViewmodelHeldItem
)

// ViewmodelTier 是挥动摆幅与节奏的参数档：空手、方块、剑、镐、铲、斧六档。
type ViewmodelTier uint8

const (
	// ViewmodelTierEmptyHand 是空手档：非工具持物（食物、火把、材料等）
	// 同样沿用本档摆幅。
	ViewmodelTierEmptyHand ViewmodelTier = iota
	// ViewmodelTierBlock 是手持方块档。
	ViewmodelTierBlock
	// ViewmodelTierSword 是剑档（含损坏形态）。
	ViewmodelTierSword
	// ViewmodelTierPick 是镐档（含损坏形态）。
	ViewmodelTierPick
	// ViewmodelTierHoe 是锄档：规则落地前取镐档默认值。
	ViewmodelTierHoe
	// ViewmodelTierAxe 是斧档：核心尚无斧物品，参数取镐档默认值占位。
	ViewmodelTierAxe
)

// ViewmodelInput 是单帧 viewmodel 编码输入：`Selected` 必须是已确认镜像的
// 选中槽（本地选择请求未确认时调用方不得提前填入）；`Mining` 由调用方按既
// 有呈现信号门控（采掘 active、目标有效、裂纹阶段合法、游戏相位）；
// `AttackTick` 是最后确认的 `CombatHit` 权威 tick，无命中时填零。
type ViewmodelInput struct {
	Player     core.PlayerID
	Selected   core.ItemStack
	Tick       uint64
	Mining     bool
	AttackTick uint64
}

// ViewmodelHeldKindOf 把已确认选中槽映射为持物形态：空槽（零值、`ItemNone`
// 或数量为零）与未注册物品一律无持物且不 panic；`core.ItemPlacement` 命
// 中的为手持方块，其余已注册物品为手持物品。
func ViewmodelHeldKindOf(stack core.ItemStack) ViewmodelHeldKind {
	if stack.Count == 0 || stack.Item == core.ItemNone || !core.RegisteredItem(stack.Item) {
		return ViewmodelHeldNone
	}
	if _, ok := core.ItemPlacement(stack.Item); ok {
		return ViewmodelHeldBlock
	}
	return ViewmodelHeldItem
}

// ViewmodelTierOf 把已确认选中槽映射为挥动档：空槽与未注册物品走空手档；
// 可放置物品走方块档；剑（含损坏形态）走剑档；镐（含损坏形态）走镐档；锄
// （含损坏形态）走锄档；其余已注册非工具物品沿用空手档。
func ViewmodelTierOf(stack core.ItemStack) ViewmodelTier {
	if stack.Count == 0 || stack.Item == core.ItemNone || !core.RegisteredItem(stack.Item) {
		return ViewmodelTierEmptyHand
	}
	if _, ok := core.ItemPlacement(stack.Item); ok {
		return ViewmodelTierBlock
	}
	switch stack.Item {
	case core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword,
		core.ItemBrokenWoodenSword, core.ItemBrokenStoneSword, core.ItemBrokenIronSword:
		return ViewmodelTierSword
	case core.ItemStonePickaxe, core.ItemIronPickaxe,
		core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe:
		return ViewmodelTierPick
	case core.ItemStoneHoe, core.ItemIronHoe,
		core.ItemBrokenStoneHoe, core.ItemBrokenIronHoe:
		return ViewmodelTierHoe
	default:
		return ViewmodelTierEmptyHand
	}
}

// viewmodelSwingParam 是单档的挥动参数：摆幅（弧度）与完整挥动周期（权威
// tick 数）。呈现侧常量，不进任何线上契约，改表不动编码布局与 ABI。
type viewmodelSwingParam struct {
	amplitude   float32
	periodTicks uint64
}

// viewmodelSwingTable 是六档摆幅与节奏参数表：斧与铲在配方与采掘规则落地
// 前取镐档默认值，落地后只改这两行的表值。
var viewmodelSwingTable = [...]viewmodelSwingParam{
	ViewmodelTierEmptyHand: {amplitude: 0.5, periodTicks: 12},
	ViewmodelTierBlock:     {amplitude: 0.4, periodTicks: 14},
	ViewmodelTierSword:     {amplitude: 0.8, periodTicks: 8},
	ViewmodelTierPick:      {amplitude: 0.6, periodTicks: 10},
	ViewmodelTierHoe:       {amplitude: 0.6, periodTicks: 10},
	ViewmodelTierAxe:       {amplitude: 0.6, periodTicks: 10},
}

// ViewmodelSwingParams 返回指定档的摆幅与周期：越界档位回落到空手档，编码
// 永不因档位失配 panic。
func ViewmodelSwingParams(tier ViewmodelTier) (amplitude float32, periodTicks uint64) {
	if tier > ViewmodelTierAxe {
		tier = ViewmodelTierEmptyHand
	}
	param := viewmodelSwingTable[tier]
	return param.amplitude, param.periodTicks
}

// ViewmodelMiningAngle 是挖掘挥动的纯相位函数：自 `anchorTick` 起随权威
// tick 循环摆动，不读墙钟、帧间隔与本地随机数；同 `(tick, 档, 触发沿)` 重
// 放逐帧相同。`tick` 回退时调用方重锚（以当前 tick 为新锚），旧相位不延续。
func ViewmodelMiningAngle(tick, anchorTick uint64, tier ViewmodelTier) float32 {
	amplitude, period := ViewmodelSwingParams(tier)
	var elapsed uint64
	if tick > anchorTick {
		elapsed = tick - anchorTick
	}
	phase := 2 * math.Pi * float64(elapsed%period) / float64(period)
	return amplitude * float32(math.Sin(phase))
}

// ViewmodelAttackAngle 是攻击挥动的纯相位函数：`attackAge` 为触发后的帧龄，
// 窗内完成一次正弦挥动（首帧即起挥），窗满回零；同 `(帧龄, 档)` 重放逐帧相同。
func ViewmodelAttackAngle(attackAge uint8, tier ViewmodelTier) float32 {
	if attackAge >= ViewmodelAttackFrames {
		return 0
	}
	amplitude, _ := ViewmodelSwingParams(tier)
	phase := math.Pi * float64(attackAge+1) / float64(ViewmodelAttackFrames+1)
	return amplitude * float32(math.Sin(phase))
}

var (
	// viewmodelArmSize 与第三人称手臂同源：0.1×0.7×0.25。
	viewmodelArmSize = mgl32.Vec3{0.1, 0.7, 0.25}
	// viewmodelLeftCenter/viewmodelRightCenter 是相机空间的双手中心：左右
	// 对称、下沉前置；臂长 0.7 使臂根落在屏底之外，只留前臂入画。
	viewmodelLeftCenter  = mgl32.Vec3{-0.3, -0.45, -0.8}
	viewmodelRightCenter = mgl32.Vec3{0.3, -0.45, -0.8}
	// viewmodelRightPivot 是右手挥动转轴（臂根）：与手臂底面齐平。
	viewmodelRightPivot = mgl32.Vec3{0.3, -0.8, -0.8}
	// viewmodelHeldBlockCenter/viewmodelHeldBlockSize 是手持方块的微缩立方：
	// 落在右手上方，与世界同源材质。
	viewmodelHeldBlockCenter = mgl32.Vec3{0.3, 0.03, -0.85}
	viewmodelHeldBlockSize   = mgl32.Vec3{0.22, 0.22, 0.22}
	// viewmodelHeldItemCenter/viewmodelHeldItemSize 是手持物品的扁长条：纵
	// 轴显著长于另两轴，与立方剪影可辨，被右手握持。
	viewmodelHeldItemCenter = mgl32.Vec3{0.3, -0.05, -0.82}
	viewmodelHeldItemSize   = mgl32.Vec3{0.09, 0.5, 0.12}
)

// viewmodelHeldNeutralColor 是未登记基色物品的中性呈现色：`ItemColor` 只覆
// 盖部分物品，未覆盖的已注册物品走本色而非透明黑，保证持物可见。
var viewmodelHeldNeutralColor = [4]float32{0.75, 0.7, 0.65, 1}

// viewmodelHeldColor 返回扁长条持物的纯色：复用与 HUD、掉落物共享的稳定基
// 色，未覆盖的物品回落中性色。
func viewmodelHeldColor(item core.ItemID) [4]float32 {
	if color, ok := itemDropColor(item); ok && color[3] != 0 {
		return color
	}
	return viewmodelHeldNeutralColor
}

// viewmodelHeldBlockAppearance 返回手持方块的材质与颜色：单实例只带一层号，
// 取与掉落物同源的世界顶面代表层（顶面/侧面与世界一致在此离散度下成立）；
// 无世界层可取时回落纯色分支。
func viewmodelHeldBlockAppearance(item core.ItemID) (material uint32, color [4]float32) {
	if layer, ok := itemDropMaterial(item); ok {
		return layer, [4]float32{1, 1, 1, 1}
	}
	return avatarMaterialSolid, viewmodelHeldColor(item)
}

// ViewmodelEncoder 持有 viewmodel 编码的复用缓冲与挥动边沿状态：热路径零
// 分配；状态只服务呈现（挖掘锚、攻击窗），不进协议与存档。
type ViewmodelEncoder struct {
	parts []avatarPart
	// mining 是上一帧的挖掘门控，miningAnchor 是本轮挖掘的起始权威 tick。
	mining       bool
	miningAnchor uint64
	// lastTick 是上一帧权威 tick，用于回退检测；lastAttackTick 是已见的
	// 最新命中触发沿，陈旧与重复确认不得重启窗口。
	lastTick       uint64
	lastAttackTick uint64
	// attackOpen 为真表示攻击窗进行中，attackAge 为窗内帧龄。
	attackOpen bool
	attackAge  uint8
}

// ResetViewmodel 清零挥动的边沿状态并保留复用缓冲：断线重连、会话重置与场
// 景切换后由装配层调用，下一帧起按新输入重新锚定。
func (e *ViewmodelEncoder) ResetViewmodel() {
	e.parts = e.parts[:0]
	e.mining = false
	e.miningAnchor = 0
	e.lastTick = 0
	e.lastAttackTick = 0
	e.attackOpen = false
	e.attackAge = 0
}

// EncodeViewmodelInstances 把单帧 viewmodel 编码为 96 字节/实例的字节流，
// 与 avatar 实例布局同形。`input` 为 nil 表示无 viewmodel 输入（非游戏相
// 位或会话未存活）：输出为空且不扰动编码器状态。`dst` 会被重置复用，调用
// 方保证容量即零分配。单帧实例恒不超过 `ViewmodelMaxInstances`。
//
// 边沿语义：挖掘上升沿以本 tick 为锚，持续期间相位随 tick 循环，下降沿回
// 中立；命中触发沿（严格递增的 `AttackTick`）开启 6 帧窗口，窗内攻击挥动
// 优先于挖掘，窗满自闭；tick 回退时挖掘重锚、攻击窗清空。
func (e *ViewmodelEncoder) EncodeViewmodelInstances(dst []byte, input *ViewmodelInput) []byte {
	if input == nil {
		return dst[:0]
	}
	if input.Tick < e.lastTick {
		e.miningAnchor = input.Tick
		e.attackOpen = false
	}
	if input.Mining && !e.mining {
		e.miningAnchor = input.Tick
	}
	e.mining = input.Mining
	if input.AttackTick != 0 && input.AttackTick > e.lastAttackTick {
		e.lastAttackTick = input.AttackTick
		e.attackOpen = true
		e.attackAge = 0
	}
	e.lastTick = input.Tick
	tier := ViewmodelTierOf(input.Selected)
	var angle float32
	if e.attackOpen {
		angle = ViewmodelAttackAngle(e.attackAge, tier)
		e.attackAge++
		if e.attackAge >= ViewmodelAttackFrames {
			e.attackOpen = false
		}
	} else if e.mining {
		angle = ViewmodelMiningAngle(input.Tick, e.miningAnchor, tier)
	}
	e.parts = buildViewmodelParts(e.parts[:0], input, angle)
	dst = growEncodeBuffer(dst, len(e.parts)*avatarInstanceBytes)
	encodeAvatarPartsInto(dst, e.parts)
	return dst
}

// buildViewmodelParts 装配单帧实例：左手静态占位，右手绕臂根按挥动角旋转，
// 持物随右手同轴旋转。双手颜色与材质与同身份第三人称手臂同源。
func buildViewmodelParts(dst []avatarPart, input *ViewmodelInput, angle float32) []avatarPart {
	key := EntityKey{Kind: EntityPlayer, ID: [16]byte(input.Player)}
	base := avatarColor(key)
	handColor := avatarShade(base, 0.82)
	material := uint32(assets.LayerHumanSageHead)
	if swingPhaseID(key)%2 != 0 {
		material = uint32(assets.LayerHumanClayHead)
	}
	armMaterial := material + 12
	root := mgl32.Ident4()
	dst = append(dst, swungLimb(root, mgl32.Vec3{}, viewmodelLeftCenter, viewmodelArmSize,
		mgl32.Ident4(), 0, handColor, armMaterial))
	rotation := mgl32.Ident4()
	if angle != 0 {
		rotation = mgl32.HomogRotate3DX(angle)
	}
	dst = append(dst, swungLimb(root, viewmodelRightPivot, viewmodelRightCenter, viewmodelArmSize,
		rotation, angle, handColor, armMaterial))
	switch ViewmodelHeldKindOf(input.Selected) {
	case ViewmodelHeldBlock:
		heldMaterial, heldColor := viewmodelHeldBlockAppearance(input.Selected.Item)
		dst = append(dst, swungLimb(root, viewmodelRightPivot, viewmodelHeldBlockCenter, viewmodelHeldBlockSize,
			rotation, angle, heldColor, heldMaterial))
	case ViewmodelHeldItem:
		dst = append(dst, swungLimb(root, viewmodelRightPivot, viewmodelHeldItemCenter, viewmodelHeldItemSize,
			rotation, angle, viewmodelHeldColor(input.Selected.Item), avatarMaterialSolid))
	default:
	}
	return dst
}

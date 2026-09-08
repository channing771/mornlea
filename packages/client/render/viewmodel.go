package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是第一人称主手 viewmodel 的 CPU 编码：主手及其持物，
// 手臂颜色与材质同第三人称同源，全部相位由权威 tick 与触发沿派生，
// 不读墙钟。输出复用 avatar 实例布局（96 字节/实例），绘制由 Rust 渲染器
// 承担；帧 TLV 装配不在本文件，属于跨语言帧编码的职责。

const (
	// ViewmodelMaxInstances 是单帧 viewmodel 实例恒定上限：主手一实例与
	// 至多 16×16 个图标棱柱。
	ViewmodelMaxInstances = 257
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
	// ViewmodelHeldItem 表示手持物品：有厚度的原创图标轮廓。
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
// `CamPos`/`CamYaw`/`CamPitch` 是本帧呈现相机的位姿：根变换由它派生，相机
// 空间偏移经根变换烘焙为世界变换后由既有世界投影绘制；零值即旧链的单位根，
// 重放比较必须连同位姿一起固定。
type ViewmodelInput struct {
	Player     core.PlayerID
	Selected   core.ItemStack
	Tick       uint64
	Mining     bool
	AttackTick uint64
	CamPos     mgl32.Vec3
	CamYaw     float32
	CamPitch   float32
	// Registry 是本帧 atlas 同源的只读资产；nil 使用默认素材。
	Registry *assets.Registry
}

// ViewmodelHeldKindOf 把已确认选中槽映射为持物形态：空槽（零值、`ItemNone`
// 或数量为零）与未注册物品一律无持物且不 panic；原创轮廓图标优先，
// 其余 `core.ItemPlacement` 命中的物品为微缩方块。
func ViewmodelHeldKindOf(stack core.ItemStack) ViewmodelHeldKind {
	if stack.Count == 0 || stack.Item == core.ItemNone || !core.RegisteredItem(stack.Item) {
		return ViewmodelHeldNone
	}
	if _, ok := assets.ItemIconLayer(stack.Item); ok {
		return ViewmodelHeldItem
	}
	if _, ok := core.ItemPlacement(stack.Item); ok {
		return ViewmodelHeldBlock
	}
	return ViewmodelHeldItem
}

// ViewmodelTierOf 把已确认选中槽映射为挥动档：空槽与未注册物品走空手档；
// 完整方块走方块档；剑（含损坏形态）走剑档；镐（含损坏形态）走镐档；锄
// （含损坏形态）走锄档；其余已注册非工具物品沿用空手档。斧档不可经物品到
// 达：核心尚无斧物品，`ViewmodelTierAxe` 只供参数表占位与未来扩展。
func ViewmodelTierOf(stack core.ItemStack) ViewmodelTier {
	if stack.Count == 0 || stack.Item == core.ItemNone || !core.RegisteredItem(stack.Item) {
		return ViewmodelTierEmptyHand
	}
	if ViewmodelHeldKindOf(stack) == ViewmodelHeldBlock {
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
// 前取镐档默认值，落地后只改这两行的表值。摆幅按抓帧目检调定：工具三档统
// 一 0.7（峰值挥动保持手持物在框内且屏面位移可辨，更大摆幅会使峰值帧冲出
// 画面，见视觉基线报告；档位区分改由周期承担——剑 8 tick 最快、镐 10、空
// 手 12、方块 14；周期一律不动）。
var viewmodelSwingTable = [...]viewmodelSwingParam{
	ViewmodelTierEmptyHand: {amplitude: 0.5, periodTicks: 12},
	ViewmodelTierBlock:     {amplitude: 0.4, periodTicks: 14},
	ViewmodelTierSword:     {amplitude: 0.7, periodTicks: 8},
	ViewmodelTierPick:      {amplitude: 0.7, periodTicks: 10},
	ViewmodelTierHoe:       {amplitude: 0.7, periodTicks: 10},
	ViewmodelTierAxe:       {amplitude: 0.7, periodTicks: 10},
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

// ViewmodelMiningAngle 是挖掘挥动的纯相位函数：自锚点起随权威
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

// 默认输入与生产输入复用同一图标缓存算法；构造仅发生在包初始化阶段。
var viewmodelDefaultRegistry = assets.NewDefaultRegistry()

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
// 优先于挖掘，窗满自闭；tick 回退时挖掘重锚、攻击窗与触发沿一起清空。
func (e *ViewmodelEncoder) EncodeViewmodelInstances(dst []byte, input *ViewmodelInput) []byte {
	if input == nil {
		return dst[:0]
	}
	if input.Tick < e.lastTick {
		// 回退即新会话：挖掘重锚、攻击窗与触发沿一起清空，否则旧大值会
		// 把新会话的小 tick 命中误判为陈旧而丢掉首挥。
		e.miningAnchor = input.Tick
		e.attackOpen = false
		e.attackAge = 0
		e.lastAttackTick = 0
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

// viewmodelRootFromCameraPose 把本帧相机位姿变为实例根变换：相机空间偏移
// （前为 −Z）经该根烘焙为世界变换。旋转按偏航绕 Y 再俯仰绕 X 的顺序组合，与
// 展示相机的朝向公式同构（零偏航零俯仰朝 −Z），经逐字验算与同位姿视图矩阵
// 的逆一致；零位姿退化为单位阵，旧链逐字节不动。
func viewmodelRootFromCameraPose(pos mgl32.Vec3, yaw, pitch float32) mgl32.Mat4 {
	return mgl32.Translate3D(pos[0], pos[1], pos[2]).
		Mul4(mgl32.HomogRotate3DY(yaw)).
		Mul4(mgl32.HomogRotate3DX(pitch))
}

// buildViewmodelParts 把主手与持物装在同一握持根；挥动只作用于根，局部握点
// 不随相位漂移。相机根在最外层，保持任意世界位姿下的第一人称构图。
func buildViewmodelParts(dst []avatarPart, input *ViewmodelInput, angle float32) []avatarPart {
	key := EntityKey{Kind: EntityPlayer, ID: [16]byte(input.Player)}
	material := uint32(assets.LayerHumanSageHead)
	if swingPhaseID(key)%2 != 0 {
		material = uint32(assets.LayerHumanClayHead)
	}
	// 屏面滚转配合较小俯仰，避免正向峰值把整个臂根抬入画面。
	root := viewmodelRootFromCameraPose(input.CamPos, input.CamYaw, input.CamPitch).
		Mul4(mgl32.Translate3D(.38, -.30, -.95)).
		Mul4(mgl32.HomogRotate3DZ(28*math.Pi/180 - angle*.30)).
		Mul4(mgl32.HomogRotate3DX(angle * .35))
	add := func(frame mgl32.Mat4, center, size mgl32.Vec3, color [4]float32, material uint32) {
		dst = append(dst, avatarPart{transform: frame.Mul4(mgl32.Translate3D(center[0], center[1], center[2])).Mul4(mgl32.Scale3D(size[0], size[1], size[2])), color: color, material: material})
	}
	add(root, mgl32.Vec3{0, -.28, .05}, mgl32.Vec3{.16, .60, .18}, avatarShade(avatarColor(key), .82), material+12)
	registry := input.Registry
	if registry == nil {
		registry = viewmodelDefaultRegistry
	}
	switch ViewmodelHeldKindOf(input.Selected) {
	case ViewmodelHeldBlock:
		block, _ := core.ItemPlacement(input.Selected.Item)
		frame := root.Mul4(mgl32.Translate3D(0, .12, 0)).Mul4(mgl32.HomogRotate3DX(.24)).Mul4(mgl32.HomogRotate3DY(-.55))
		// 顶底面完整覆盖，侧面退让顶底厚度，前后面再退让左右厚度；
		// 六面只接触不重叠，外表面封闭且边缘没有共面闪烁。
		const side = float32(.25)
		const thickness = float32(.002)
		const offset = (side - thickness) / 2
		const inner = side - 2*thickness
		faces := [...]mesh.Face{mesh.FacePosX, mesh.FaceNegX, mesh.FacePosY, mesh.FaceNegY, mesh.FacePosZ, mesh.FaceNegZ}
		centers := [...]mgl32.Vec3{{offset, 0, 0}, {-offset, 0, 0}, {0, offset, 0}, {0, -offset, 0}, {0, 0, offset}, {0, 0, -offset}}
		sizes := [...]mgl32.Vec3{{thickness, inner, side}, {thickness, inner, side}, {side, thickness, side}, {side, thickness, side}, {inner, inner, thickness}, {inner, inner, thickness}}
		for i, face := range faces {
			add(frame, centers[i], sizes[i], [4]float32{1, 1, 1, 1}, uint32(registry.Material(block, face)))
		}
	case ViewmodelHeldItem:
		// 图稿的柄从左下伸向右上；各类握点取实际柄内像素，损坏形态共享柄位。
		gripX, gripY, pixel := float32(8), float32(12), float32(.032)
		// 铁锭图稿止于第十行，托点下移一像素以贴住拳面。
		if input.Selected.Item == core.ItemIronIngot {
			gripY = 11
		}
		switch ViewmodelTierOf(input.Selected) {
		case ViewmodelTierSword:
			gripX, gripY, pixel = 5.5, 12, .035
		case ViewmodelTierPick:
			gripX, gripY, pixel = 5, 12, .035
		case ViewmodelTierHoe:
			gripX, gripY, pixel = 5.5, 11.5, .035
		}
		frame := root.Mul4(mgl32.HomogRotate3DY(-.25)).Mul4(mgl32.HomogRotate3DX(-.25))
		parts, _ := registry.ItemIconPrisms(input.Selected.Item)
		for _, part := range parts {
			center := mgl32.Vec3{(float32(part.X) + float32(part.Width)/2 - gripX) * pixel, (gripY - float32(part.Y) - .5) * pixel, 0}
			add(frame, center, mgl32.Vec3{float32(part.Width) * pixel, pixel, .045}, part.Color, avatarMaterialSolid)
		}
	}
	return dst
}

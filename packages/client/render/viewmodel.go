package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件是第一人称主手 viewmodel 的 CPU 编码：主手及其持物，
// 手臂颜色与材质同第三人称同源，全部相位由显式本地呈现时间派生，
// 不读墙钟。输出复用 avatar 实例布局（96 字节/实例），绘制由 Rust 渲染器
// 承担；帧 TLV 装配不在本文件，属于跨语言帧编码的职责。

const (
	// ViewmodelMaxInstances 是单帧 viewmodel 实例恒定上限：最多八个主手部件与
	// 至多 16×16 个图标棱柱。
	ViewmodelMaxInstances = 264
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

// ViewmodelInput 是单帧呈现快照：`Selected` 只取已确认选中，
// `SwingActive` / `SwingPhase` 只取本地有效点击时钟。权威 tick、裂纹和命中
// 不进入动作编码；相机位姿和逻辑尺寸共同固定重放的构图。
type ViewmodelInput struct {
	// SwingActive / SwingPhase 是有效点击驱动的单调呈现时间，重复编码不推进动作。
	SwingActive bool
	SwingPhase  float32
	Player      core.PlayerID
	Selected    core.ItemStack
	CamPos      mgl32.Vec3
	CamYaw      float32
	CamPitch    float32
	// Registry 是本帧 atlas 同源的只读资产；nil 使用默认素材。
	Registry *assets.Registry
	// `ViewportWidth`/`ViewportHeight` 使用 HUD 同源逻辑像素；`FovY` 为世界相机垂直弧度。
	// 零尺寸回落 1280×720，零 FOV 回落 70 度，供无窗口调用保持确定。
	ViewportWidth, ViewportHeight, FovY float32
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

// viewmodelSwingParam 是摆幅与节奏档；`periodTicks` 仅表示 50ms 呈现采样数，
// 不消费服务端 tick。参数不进线上契约，布局与 ABI 不变。
type viewmodelSwingParam struct {
	amplitude   float32
	periodTicks uint64
}

// viewmodelSwingTable 保留类别摆幅与周期差异；剑 400ms、镐/锄 500ms、
// 空手 400ms、方块 450ms。尚无斧物品，预留档沿用镐参数。
var viewmodelSwingTable = [...]viewmodelSwingParam{
	ViewmodelTierEmptyHand: {amplitude: 0.5, periodTicks: 8},
	ViewmodelTierBlock:     {amplitude: 0.4, periodTicks: 9},
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

// 默认输入与生产输入复用同一图标缓存算法；构造仅发生在包初始化阶段。
var viewmodelDefaultRegistry = assets.NewDefaultRegistry()

// ViewmodelEncoder 只保留几何复用缓冲；动作由输入的显式呈现相位决定。
type ViewmodelEncoder struct{ parts []avatarPart }

// ResetViewmodel 清除复用部件；动作时钟由应用层在会话边界一并重置。
func (e *ViewmodelEncoder) ResetViewmodel() { e.parts = e.parts[:0] }

// EncodeViewmodelInstances 把单帧 viewmodel 编码为 96 字节/实例的字节流，
// 与 avatar 实例布局同形。`input` 为 nil 表示无 viewmodel 输入（非游戏相
// 位或会话未存活）：输出为空且不扰动编码器状态。`dst` 会被重置复用，调用
// 方保证容量即零分配。单帧实例恒不超过 `ViewmodelMaxInstances`。
func (e *ViewmodelEncoder) EncodeViewmodelInstances(dst []byte, input *ViewmodelInput) []byte {
	if input == nil {
		return dst[:0]
	}
	phase := float32(0)
	if input.SwingActive {
		phase = input.SwingPhase
	}
	e.parts = buildViewmodelParts(e.parts[:0], input, phase)
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
func buildViewmodelParts(dst []avatarPart, input *ViewmodelInput, phase float32) []avatarPart {
	key := EntityKey{Kind: EntityPlayer, ID: [16]byte(input.Player)}
	material := uint32(assets.LayerHumanSageHead)
	if swingPhaseID(key)%2 != 0 {
		material = uint32(assets.LayerHumanClayHead)
	}
	root := viewmodelRootFromCameraPose(input.CamPos, input.CamYaw, input.CamPitch).Mul4(viewmodelGripRoot(input, phase))
	add := func(frame mgl32.Mat4, center, size mgl32.Vec3, color [4]float32, material uint32) {
		dst = append(dst, avatarPart{transform: frame.Mul4(mgl32.Translate3D(center[0], center[1], center[2])).Mul4(mgl32.Scale3D(size[0], size[1], size[2])), color: color, material: material})
	}
	registry := input.Registry
	if registry == nil {
		registry = viewmodelDefaultRegistry
	}
	// 头底面的肤色取当前身份 atlas，避免独立调色板与材质覆盖脱节。
	px := registry.LayerRGBA(int(material) + 3)
	skin := [4]float32{float32(px[0]) / 255, float32(px[1]) / 255, float32(px[2]) / 255, 1}
	// 主手迎光面的暖色对比只作用于当前采样，不改变第三人称或公共着色器。
	skin[0], skin[1], skin[2] = min(1, skin[0]*1.70), min(1, skin[1]*.85), skin[2]*.70
	// 袖子的中部布料色来自当前身份原材质，避免整条第三人称袖纹挤成白色腕带。
	coatPixels := registry.LayerRGBA(int(material) + 12)
	coatOffset := (6*16 + 8) * 4
	coat := [4]float32{float32(coatPixels[coatOffset]) / 255, float32(coatPixels[coatOffset+1]) / 255, float32(coatPixels[coatOffset+2]) / 255, 1}
	// 掌轴更直立，宽截面保留闭拳厚度；前臂主要从底部而非右侧进入。
	hand := root.Mul4(mgl32.HomogRotate3DZ(-20 * math.Pi / 180))
	add(hand.Mul4(mgl32.HomogRotate3DX(-.10)), mgl32.Vec3{0, -.48, .03}, mgl32.Vec3{.22, .78, .185}, coat, avatarMaterialSolid)
	add(hand, mgl32.Vec3{0, -.075, .025}, mgl32.Vec3{.22, .04, .185}, avatarShade(coat, 1.03), avatarMaterialSolid)
	add(hand, mgl32.Vec3{0, 0, 0}, mgl32.Vec3{.175, .09, .125}, avatarShade(skin, .42), avatarMaterialSolid)
	add(hand, mgl32.Vec3{-.080, .002, .063}, mgl32.Vec3{.032, .060, .040}, avatarShade(skin, .70), avatarMaterialSolid)
	// 四个局部分面代替板条指节，闭拳与袖子的暗侧保持完整体积。
	add(hand, mgl32.Vec3{0, 0, .0635}, mgl32.Vec3{.175, .09, .002}, skin, avatarMaterialSolid)
	add(hand, mgl32.Vec3{0, .0455, 0}, mgl32.Vec3{.175, .001, .125}, avatarShade(skin, 1.08), avatarMaterialSolid)
	add(hand.Mul4(mgl32.HomogRotate3DX(-.10)), mgl32.Vec3{0, -.48, .123}, mgl32.Vec3{.22, .78, .001}, avatarShade(coat, 1.08), avatarMaterialSolid)
	add(hand, mgl32.Vec3{-.108, -.075, .025}, mgl32.Vec3{.004, .04, .185}, avatarShade(coat, .68), avatarMaterialSolid)

	switch ViewmodelHeldKindOf(input.Selected) {
	case ViewmodelHeldBlock:
		block, _ := core.ItemPlacement(input.Selected.Item)
		frame := root.Mul4(mgl32.HomogRotate3DZ(-30 * math.Pi / 180)).Mul4(mgl32.Translate3D(0, .20, 0)).Mul4(mgl32.HomogRotate3DX(.65)).Mul4(mgl32.HomogRotate3DY(.10))
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
		toolParts, solidTool := registry.ItemToolParts(input.Selected.Item)
		if solidTool {
			// 工具独立抵消前臂斜角；刃头向握点右上方伸出，柄仍穿过拳掌。
			width, height := input.ViewportWidth, input.ViewportHeight
			if width <= 0 || height <= 0 {
				width, height = 1280, 720
			}
			// 窄屏逐渐扶正工具，4:3 单独收窄工具比例，给完整头部保留挥动余量。
			normal := min(float32(1), max(float32(0), (width-800)/200))
			toolScale := float32(.85) * min(float32(1), width/height/(16.0/9))
			toolYaw := float32(-1.15)
			if ViewmodelTierOf(input.Selected) == ViewmodelTierPick {
				toolYaw = .30
			}
			toolOffset, toolHeight := float32(.002), float32(-.011)
			if ViewmodelTierOf(input.Selected) != ViewmodelTierSword {
				toolOffset, toolHeight = .007, -.015
			}
			// 掌握与工具向相机前置，保持中立投影不变，避免柄被更后的袖口切成碎片。
			neutral := viewmodelGripRoot(input, 0)
			heldLocal := neutral.Inv().Mul4(mgl32.Scale3D(.82, .82, .82)).Mul4(neutral)
			heldRoot := root.Mul4(heldLocal)
			frame := heldRoot.Mul4(mgl32.Translate3D(toolOffset, toolHeight, 0)).Mul4(mgl32.HomogRotate3DZ((-45 - 15*normal) * math.Pi / 180)).Mul4(mgl32.HomogRotate3DY(toolYaw)).Mul4(mgl32.Scale3D(toolScale, toolScale, toolScale))
			// 握工具时掌面沿柄收拢，拇指沿柄下垂，折指包住上侧；袖子仍共享同一入画轮廓。
			graspLocal := heldLocal.Mul4(mgl32.Translate3D(toolOffset, toolHeight, 0)).Mul4(mgl32.HomogRotate3DZ((-45 - 15*normal) * math.Pi / 180))
			grasp := root.Mul4(graspLocal)
			setHand := func(index int, center, size mgl32.Vec3, color [4]float32) {
				dst[index] = avatarPart{transform: grasp.Mul4(mgl32.Translate3D(center[0], center[1], center[2])).Mul4(mgl32.Scale3D(size[0], size[1], size[2])), color: color, material: avatarMaterialSolid}
			}
			setHand(2, mgl32.Vec3{.005, -.02, .0175}, mgl32.Vec3{.13, .12, .125}, avatarShade(skin, .55))
			setHand(3, mgl32.Vec3{-.075, -.02, .060}, mgl32.Vec3{.055, .11, .050}, avatarShade(skin, .85))
			setHand(4, mgl32.Vec3{.005, -.02, .081}, mgl32.Vec3{.13, .12, .002}, skin)
			setHand(5, mgl32.Vec3{-.025, .065, .055}, mgl32.Vec3{.090, .055, .075}, avatarShade(skin, 1.05))
			// 前置掌握与袖口之间保留真实腕部体积；中立时由掌面遮住，转腕时也不会露出断口。
			wristStart := mgl32.HomogRotate3DZ(-20 * math.Pi / 180).Mul4x1(mgl32.Vec4{.04, -.075, .025, 1}).Vec3()
			wristEnd := graspLocal.Mul4x1(mgl32.Vec4{.05, -.015, .0175, 1}).Vec3()
			wristAxis := wristEnd.Sub(wristStart)
			wristCenter := wristStart.Add(wristEnd).Mul(.5)
			wristFrame := root.Mul4(mgl32.Translate3D(wristCenter[0], wristCenter[1], wristCenter[2])).Mul4(mgl32.QuatBetweenVectors(mgl32.Vec3{0, 1, 0}, wristAxis.Normalize()).Mat4())
			dst[7] = avatarPart{transform: wristFrame.Mul4(mgl32.Scale3D(.07, wristAxis.Len()+.04, .07)), color: avatarShade(skin, .65), material: avatarMaterialSolid}
			for _, part := range toolParts {
				partFrame := frame.Mul4(mgl32.Translate3D(part.Center[0], part.Center[1], part.Center[2])).Mul4(mgl32.HomogRotate3DZ(part.RotationZ))
				if part.Beveled {
					partFrame = partFrame.Mul4(mgl32.Scale3D(part.Size[0]/1.7071068, part.Size[1]/1.4142136, part.Size[2]/1.7071068)).Mul4(mgl32.HomogRotate3DY(math.Pi / 4)).Mul4(mgl32.HomogRotate3DZ(math.Pi / 4))
					add(partFrame, mgl32.Vec3{}, mgl32.Vec3{1, 1, 1}, part.Color, avatarMaterialSolid)
				} else {
					add(partFrame, mgl32.Vec3{}, mgl32.Vec3(part.Size), part.Color, avatarMaterialSolid)
				}
			}
			return dst
		}
		// 非工具图标沿原有像素承托点装配，完整立方仍走独立六面路径。
		frame := root.Mul4(mgl32.Translate3D(0, .035, 0)).Mul4(mgl32.HomogRotate3DZ(-30 * math.Pi / 180)).Mul4(mgl32.HomogRotate3DY(-.45)).Mul4(mgl32.HomogRotate3DX(-.20)).Mul4(mgl32.Scale3D(.85, .85, .85))
		gripX, gripY, pixel := float32(8), float32(12), float32(.032)
		// 弓的握把位于图稿左侧；默认图标中心落在透明区，需对齐实际弓臂。
		if input.Selected.Item == core.ItemBow || input.Selected.Item == core.ItemBrokenBow {
			gripX = 2
			gripY = 11
		}
		// 头盔面甲中心是透明开口，改由左颊下缘接入收窄后的拳掌。
		if input.Selected.Item == core.ItemBone {
			gripX, gripY = 6, 11
		}
		if input.Selected.Item == core.ItemIronBoots {
			gripY = 11
		}
		if input.Selected.Item == core.ItemIronHelmet {
			gripX, gripY = 6, 10
		}
		parts, _ := registry.ItemIconPrisms(input.Selected.Item)
		for _, part := range parts {
			center := mgl32.Vec3{(float32(part.X) + float32(part.Width)/2 - gripX) * pixel, (gripY - float32(part.Y) - .5) * pixel, 0}
			add(frame, center, mgl32.Vec3{float32(part.Width) * pixel, pixel, .045}, part.Color, avatarMaterialSolid)
		}
	}
	return dst
}

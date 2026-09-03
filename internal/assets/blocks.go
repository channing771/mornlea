package assets

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/mesh"
	"github.com/channing771/mornlea/internal/world"
)

const (
	LayerStone uint16 = iota
	LayerDirt
	LayerGrassTop
	LayerGrassSide
	LayerBedrock
	LayerStoneBrick
	LayerCoalOre
	LayerIronOre
	LayerFurnace
	LayerIronBlock
	LayerChest
	LayerLightBlock
	LayerLeaves
	LayerGlass
	LayerCobblestone
	LayerSmoothStone
	LayerSand
	LayerGravel
	LayerOakLogSide
	LayerOakLogTop
	LayerOakPlanks
	LayerBrick
	LayerWhiteWool
	LayerRoofTile
	LayerClay
	LayerSnowTop
	LayerSnowSide
	LayerMossyCobblestone
	// LayerWater 是 8 个流体编号共用的材质层。它必须独立于任何固体层：
	// 上传路径正是按 material 把水面 quad 分流到半透明 water pass 的
	// （见 internal/render.SectionScheduler），共用石头层等于水面被画进
	// 不透明 terrain pass。
	LayerWater
	// LayerFarmlandDry / LayerFarmlandWet 是耕地的干湿两态。耕地在视觉上仍是
	// 满立方体、仍然不透明，因此它们是普通固体层，不进植物集合。
	LayerFarmlandDry
	LayerFarmlandWet
	// LayerWheat0..LayerCarrot7 是全部作物生长阶段的材质层，**必须连续且保持
	// 31..54 不变**（小麦 8 + 马铃薯 8 + 胡萝卜 8）：Rust 侧 `quad.rs` 与
	// `internal/mesh` 的 PlantMaterialFirst/Last 用闭区间
	// [PLANT_MATERIAL_FIRST, PLANT_MATERIAL_LAST] 识别作物，另以
	// PlantMaterialShortGrass 识别离散短草层，从而决定一格出交叉斜面而非轴向面。
	// 两侧没有共享常量也没有生成步骤，只能人手同步——数值一侧由 internal/mesh 的植物材质常量与
	// TestPlantMaterialLayersMatchMeshContract 钉住，跨语言一侧由真的喂一次
	// Rust mesher 的真实作物与短草 parity 测试兜底。
	//
	// 在这 24 层**之前**插入任何新层都会整体平移它们，于是 Rust 会把别的方块当成
	// 植物、把植物当成普通方块——新层一律追加在 LayerCarrot7 之后。
	LayerWheat0
	LayerWheat1
	LayerWheat2
	LayerWheat3
	LayerWheat4
	LayerWheat5
	LayerWheat6
	LayerWheat7
	LayerPotato0
	LayerPotato1
	LayerPotato2
	LayerPotato3
	LayerPotato4
	LayerPotato5
	LayerPotato6
	LayerPotato7
	LayerCarrot0
	LayerCarrot1
	LayerCarrot2
	LayerCarrot3
	LayerCarrot4
	LayerCarrot5
	LayerCarrot6
	LayerCarrot7
	LayerDoor
	// LayerWorkbenchTop / LayerWorkbenchSide / LayerWorkbenchBottom 是工作台的
	// 顶/侧/底三个原创程序化木质层（格子工作台批次追加）。工作台是普通不透明
	// 立方体，三面各占一层、互不复用也不复用橡木木板层；按 LayerWheat0 处注释
	// 的纪律，新层一律追加在 LayerCarrot7 之后以保持植物区间连续；LayerDoor
	// 作为首个非植物层紧接 LayerCarrot7（55），工作台三层随之后移，保持 Plant 31..54 连续。
	LayerWorkbenchTop
	LayerWorkbenchSide
	LayerWorkbenchBottom
	// LayerTorch 是五种火把形态共用的一张竖直火柄 cutout 材质层（窄木柄 +
	// 暖色火芯）。层号冻结为 59：Rust client 的 terrain.wgsl 火把材质门控
	// （torch_material 函数）与 shaders.rs 的 TORCH_MATERIAL 常量各自硬编码
	// 该值，三处没有共享定义也没有生成步骤，改层号必须同批同步。仍按
	// LayerWheat0 处注释的纪律追加在枚举末位，不扰动植物区间。
	LayerTorch
	// LayerBedFootSouth..LayerBedHeadEast 是床八形态各占一张的原创程序化床面
	// 层（床尾/床头 × 南西北东）。层号仍按 LayerWheat0 处纪律追加在枚举末位；
	// 床头段与床尾段同序平移 4（床头层 = 床尾层 + 4），与床方块编号的排布
	// 同形。八层只在床方块的顶面使用（四个侧面与底面复用橡木木板层表达床架），
	// 且都是完全不透明的普通固体层（不进 cutout 集合）。朝向差异由逐形态层
	// 表达：每张床面层把亮色带（床头的枕头、床尾的毯沿）画在朝向对应的边上。
	LayerBedFootSouth
	LayerBedFootWest
	LayerBedFootNorth
	LayerBedFootEast
	LayerBedHeadSouth
	LayerBedHeadWest
	LayerBedHeadNorth
	LayerBedHeadEast
	// LayerShortGrass 是短草独占的原创程序化 cutout 层。它只能追加在既有
	// 55..67 层之后；植物材质谓词把 68 作为单点加入，不能把中间的门、工作台、
	// 火把与床层一并扩进植物集合。
	LayerShortGrass
	// LayerCrack0..LayerCrack9 是采掘裂纹 overlay 的 10 个离散阶段层（中心
	// 初始裂点 → 大面积破裂），由渲染层的 crack pass 按权威采集进度采样其中
	// 一层，叠加在目标方块六面之上；像素见 crackTexture。层号冻结为 69..78
	// （LayerShortGrass 追加为 68 后顺延）：裂纹实例以 f32 层号直接索引 atlas，
	// 呈现层从 LayerCrack0 派生各阶段层号而不复制常量。仍按 LayerWheat0 处
	// 注释的纪律追加在枚举末位，不扰动植物/耕地/火把/床区间。这些层不参与
	// 任何方块材质映射（Registry.Material 不返回它们，方块的材质面永远落不到
	// 裂纹层），只经裂纹实例流被采样；裂纹是透明背景上的原创深棕/深灰系像素
	// （背景 alpha 0、裂纹像素 alpha 255 的二值 cutout，分类见 isCutoutLayer），
	// 不替换、不修改原方块材质。
	LayerCrack0
	LayerCrack1
	LayerCrack2
	LayerCrack3
	LayerCrack4
	LayerCrack5
	LayerCrack6
	LayerCrack7
	LayerCrack8
	LayerCrack9
	layerCount
)

type textureBinding struct {
	name  string
	layer uint16
}

var textureBindings = [...]textureBinding{
	{name: "stone", layer: LayerStone},
	{name: "dirt", layer: LayerDirt},
	{name: "grass_top", layer: LayerGrassTop},
	{name: "grass_side", layer: LayerGrassSide},
	{name: "bedrock", layer: LayerBedrock},
	{name: "stone_brick", layer: LayerStoneBrick},
	{name: "coal_ore", layer: LayerCoalOre},
	{name: "iron_ore", layer: LayerIronOre},
	{name: "furnace", layer: LayerFurnace},
	{name: "iron_block", layer: LayerIronBlock},
	{name: "chest", layer: LayerChest},
	{name: "light_block", layer: LayerLightBlock},
	{name: "leaves", layer: LayerLeaves},
	{name: "glass", layer: LayerGlass},
	{name: "cobblestone", layer: LayerCobblestone},
	{name: "smooth_stone", layer: LayerSmoothStone},
	{name: "sand", layer: LayerSand},
	{name: "gravel", layer: LayerGravel},
	{name: "oak_log_side", layer: LayerOakLogSide},
	{name: "oak_log_top", layer: LayerOakLogTop},
	{name: "oak_planks", layer: LayerOakPlanks},
	{name: "brick", layer: LayerBrick},
	{name: "white_wool", layer: LayerWhiteWool},
	{name: "roof_tile", layer: LayerRoofTile},
	{name: "clay", layer: LayerClay},
	{name: "snow_top", layer: LayerSnowTop},
	{name: "snow_side", layer: LayerSnowSide},
	{name: "mossy_cobblestone", layer: LayerMossyCobblestone},
	{name: "water", layer: LayerWater},
	{name: "farmland_dry", layer: LayerFarmlandDry},
	{name: "farmland_wet", layer: LayerFarmlandWet},
	{name: "wheat_0", layer: LayerWheat0},
	{name: "wheat_1", layer: LayerWheat1},
	{name: "wheat_2", layer: LayerWheat2},
	{name: "wheat_3", layer: LayerWheat3},
	{name: "wheat_4", layer: LayerWheat4},
	{name: "wheat_5", layer: LayerWheat5},
	{name: "wheat_6", layer: LayerWheat6},
	{name: "wheat_7", layer: LayerWheat7},
	{name: "potato_0", layer: LayerPotato0},
	{name: "potato_1", layer: LayerPotato1},
	{name: "potato_2", layer: LayerPotato2},
	{name: "potato_3", layer: LayerPotato3},
	{name: "potato_4", layer: LayerPotato4},
	{name: "potato_5", layer: LayerPotato5},
	{name: "potato_6", layer: LayerPotato6},
	{name: "potato_7", layer: LayerPotato7},
	{name: "carrot_0", layer: LayerCarrot0},
	{name: "carrot_1", layer: LayerCarrot1},
	{name: "carrot_2", layer: LayerCarrot2},
	{name: "carrot_3", layer: LayerCarrot3},
	{name: "carrot_4", layer: LayerCarrot4},
	{name: "carrot_5", layer: LayerCarrot5},
	{name: "carrot_6", layer: LayerCarrot6},
	{name: "carrot_7", layer: LayerCarrot7},
	{name: "door", layer: LayerDoor},
	{name: "workbench_top", layer: LayerWorkbenchTop},
	{name: "workbench_side", layer: LayerWorkbenchSide},
	{name: "workbench_bottom", layer: LayerWorkbenchBottom},
	// 火把层的绑定只是材质包覆盖的命名槽位：仓库自身不携带任何 torch.png，
	// 内嵌默认包与用户包未提供该文件时程序化像素原样生效。
	{name: "torch", layer: LayerTorch},
	// 床八面的绑定同样是覆盖槽位：床面为原创程序化像素，内嵌默认包不含
	// bed_*.png，用户包提供同名文件时按槽位覆盖。
	{name: "bed_foot_south", layer: LayerBedFootSouth},
	{name: "bed_foot_west", layer: LayerBedFootWest},
	{name: "bed_foot_north", layer: LayerBedFootNorth},
	{name: "bed_foot_east", layer: LayerBedFootEast},
	{name: "bed_head_south", layer: LayerBedHeadSouth},
	{name: "bed_head_west", layer: LayerBedHeadWest},
	{name: "bed_head_north", layer: LayerBedHeadNorth},
	{name: "bed_head_east", layer: LayerBedHeadEast},
	// 短草层是覆盖槽位：默认包可不含该文件并保留程序化像素，用户包可按路径覆盖。
	{name: "short_grass", layer: LayerShortGrass},
	// 裂纹十层的绑定同样只是材质包覆盖的命名槽位：仓库自身不携带任何 crack
	// png，内嵌默认包与用户包未提供该文件时程序化裂纹像素原样生效（镜像
	// torch/bed 的仅覆盖槽位语义）。
	{name: "crack_0", layer: LayerCrack0},
	{name: "crack_1", layer: LayerCrack1},
	{name: "crack_2", layer: LayerCrack2},
	{name: "crack_3", layer: LayerCrack3},
	{name: "crack_4", layer: LayerCrack4},
	{name: "crack_5", layer: LayerCrack5},
	{name: "crack_6", layer: LayerCrack6},
	{name: "crack_7", layer: LayerCrack7},
	{name: "crack_8", layer: LayerCrack8},
	{name: "crack_9", layer: LayerCrack9},
}

// Registry 是方块属性与材质的注册表。
type Registry struct {
	layers       [layerCount][]byte
	meshSnapshot mesh.RegistrySnapshot
}

// NewRegistry 构造注册表并生成全部占位材质。
func NewRegistry() *Registry {
	r := &Registry{}
	r.layers[LayerStone] = stoneTexture()
	r.layers[LayerDirt] = dirtTexture()
	r.layers[LayerGrassTop] = grassTopTexture()
	r.layers[LayerGrassSide] = grassSideTexture()
	r.layers[LayerBedrock] = noisyTexture(rgb{R: 60, G: 60, B: 64}, 28, 0x3F19)
	r.layers[LayerStoneBrick] = stoneBrickTexture()
	r.layers[LayerCoalOre] = oreTexture(rgb{R: 38, G: 40, B: 44})
	r.layers[LayerIronOre] = oreTexture(rgb{R: 194, G: 140, B: 104})
	r.layers[LayerFurnace] = furnaceTexture()
	r.layers[LayerIronBlock] = ironBlockTexture()
	r.layers[LayerChest] = chestTexture()
	r.layers[LayerLightBlock] = lightBlockTexture()
	r.layers[LayerLeaves] = leavesTexture()
	r.layers[LayerGlass] = glassTexture()
	r.layers[LayerCobblestone] = cobblestoneTexture()
	r.layers[LayerSmoothStone] = smoothStoneTexture()
	r.layers[LayerSand] = sandTexture()
	r.layers[LayerGravel] = gravelTexture()
	r.layers[LayerOakLogSide] = oakLogSideTexture()
	r.layers[LayerOakLogTop] = oakLogTopTexture()
	r.layers[LayerOakPlanks] = oakPlanksTexture()
	r.layers[LayerBrick] = brickTexture()
	r.layers[LayerWhiteWool] = whiteWoolTexture()
	r.layers[LayerRoofTile] = roofTileTexture()
	r.layers[LayerClay] = clayTexture()
	r.layers[LayerSnowTop] = snowTopTexture()
	r.layers[LayerSnowSide] = snowSideTexture()
	r.layers[LayerMossyCobblestone] = mossyCobblestoneTexture()
	r.layers[LayerWater] = waterTexture()
	r.layers[LayerFarmlandDry] = farmlandDryTexture()
	r.layers[LayerFarmlandWet] = farmlandWetTexture()
	for stage := 0; stage < wheatStageCount; stage++ {
		r.layers[LayerWheat0+uint16(stage)] = wheatTexture(stage)
	}
	for stage := 0; stage < wheatStageCount; stage++ {
		r.layers[LayerPotato0+uint16(stage)] = potatoTexture(stage)
	}
	for stage := 0; stage < wheatStageCount; stage++ {
		r.layers[LayerCarrot0+uint16(stage)] = carrotTexture(stage)
	}
	r.layers[LayerDoor] = doorTexture()
	r.layers[LayerWorkbenchTop] = workbenchTopTexture()
	r.layers[LayerWorkbenchSide] = workbenchSideTexture()
	r.layers[LayerWorkbenchBottom] = workbenchBottomTexture()
	r.layers[LayerTorch] = torchTexture()
	for dir := 0; dir < 4; dir++ {
		r.layers[LayerBedFootSouth+uint16(dir)] = bedTopTexture(false, dir)
		r.layers[LayerBedHeadSouth+uint16(dir)] = bedTopTexture(true, dir)
	}
	r.layers[LayerShortGrass] = shortGrassTexture()
	for stage := 0; stage < crackStageCount; stage++ {
		r.layers[LayerCrack0+uint16(stage)] = crackTexture(stage)
	}
	// ids 覆盖 core 的全部已注册方块编号，上界一律用独占哨兵 core.BlockIDMax
	// 表达——写死某个具体末位编号（历史上写过 WaterLevel7ID）会在追加新编号时
	// 静默退化成子集，新方块就永远进不了快照。Rust 侧的
	// RegistryView::face_visible 只做位图查表、缺条目一律判不可见，漏掉谁就等于
	// 谁永远不出面（流体当年正是这样差点画不出水）。
	// 条目数必须不超过 internal/mesh.nativeMaxRegistryEntries 与 Rust 的
	// MAX_REGISTRY_ENTRIES（当前已注册 85 个方块，上限 96；上限扩容必须
	// Go/Rust 两侧同批同步）。
	ids := make([]world.BlockID, 0, int(core.BlockIDMax))
	for id := core.AirID; id < core.BlockIDMax; id++ {
		ids = append(ids, id)
	}
	snapshot, err := mesh.BuildRegistrySnapshot(ids, r)
	if err != nil {
		panic("assets: 构建 mesh registry snapshot: " + err.Error())
	}
	r.meshSnapshot = snapshot
	return r
}

// Opaque 返回方块是否完全不透明。实现 mesh.Registry。
//
// 不透明判定完全转调 core.BlockOpaque——那是全仓唯一的不透明判定表，本包不得
// 保留任何重复分支；新增透明或实心方块只改 core 一张表，这里与 mesh registry
// 快照自动跟随（与 Emission/LightAttenuation 的转调同形）。
//
// 这条转调必须保持**无条件**：internal/mesh/visibility.go 的
// ComputeConnectivity 洪水填充直接拿活体 Section 的方块数据调用本函数，那条
// 路径根本不经过快照。core 表对流体判 false，若在这里或 core 里丢掉这条排除，
// 整片水会被当成实心遮挡体，区段面连通性塌成全不可达，进而错误剔除水体后方
// 的整批区段。守卫见 internal/mesh 的
// TestConnectivityTreatsFluidAsTransparentOnLiveSectionData。
// core 表对植物同样判 false：植物与玻璃、树叶同属 cutout 类，几何是方块内部
// 的两片交叉斜面，既不填满格子也不该挡光。这一条直接决定了「作物下方的耕地
// 仍被照亮」——Rust 天空光 BFS 的阻断判据就是本函数（light.rs 的 build_sky
// 只看 opaque），植物一旦不透明，它下方那格的派生天空光会归零、耕地顶面变全黑。
// 火把与植物同类判 false：零碰撞的窄柱/贴墙形态既不填满格子也不遮挡邻面，且
// 自身是发光体，被判成不透明会同时挡死邻域光照与出面。床同样判 false：9/16
// 半高方块只占格子的下半，与门同属「不满格即不遮光」的透明分类——判成不透明
// 会让床上方格的派生天空光归零、床面在夜间反而被错误压暗。
func (r *Registry) Opaque(id world.BlockID) bool {
	return core.BlockOpaque(id)
}

// FaceVisible 返回当前方块朝向相邻方块的面是否可绘制。实现 mesh.Registry。
//
// 本函数是全系统唯一的出面规则来源：它在 BuildRegistrySnapshot 里被调用一次，
// 把结果烘焙成 Visibility 位图（见 internal/mesh/registry.go），Rust 的
// RegistryView::face_visible 只是对这张位图查表，自己不含任何规则。因此流体的
// 出面规则也只能写在这里，且由既有的通用判定自然导出：
//
//   - 流体 → 流体：adjacent 非空气且 Opaque(流体)=false，落到 `return r.Opaque(id)`
//     即 false，水体内部不产生面；
//   - 流体 → 空气：直接 true，水面出几何；
//   - 流体 → 不透明方块（含头顶压着实心方块的情形）：被 `r.Opaque(adjacent)` 拦下，
//     不可见；
//   - 不透明方块 → 流体：落到 `return r.Opaque(id)` 即 true，水下地形不会消失。
//
// 历史注意：流体尚未纳入 mesh snapshot ids 范围时，这里曾对 id 与 adjacent 两侧
// 各有一处 `core.IsFluid(...)` 补偿分支，用来跟 Rust 的「缺条目即不出面」对齐。
// 流体入快照后它们已随之删除；若被误加回来，水的每一对 (id,adjacent) 都会被烘焙
// 成永久不可见、水彻底画不出来，而这件事**不会**让任何既有断言变红——守卫是
// internal/assets 的 TestFluidFaceVisibilityRules 与 internal/mesh 的
// TestNativeOracleParityWaterSurface。
// 植物是本函数唯一「自己一个轴向面都不出」的方块类：它的几何是 Rust mesher 另行
// 补出的两片交叉斜面（每格 4 条 quad），六个轴向面一条都不要。规则写在这里而不是
// Rust 里，是因为 Rust 的 face_visible 只对本函数烘焙出的位图查表、自己不含规则。
// 反方向不受影响——相邻方块**朝向**作物的面仍然可见，因为作物非不透明，判定落到
// 末尾的 `return r.Opaque(id)`。
func (r *Registry) FaceVisible(id, adjacent world.BlockID) bool {
	if !core.RegisteredBlock(id) || id == core.AirID ||
		!core.RegisteredBlock(adjacent) || r.Opaque(adjacent) {
		return false
	}
	if core.IsPlant(id) {
		return false
	}
	// 门是厚度 3/16 的薄板，非不透明。关闭时门面贴合方块边缘，开启时薄边旋转 90°。
	// 两态在 FaceVisible 层面均按“薄板”处理：本身非不透明，朝向空气的面可见，
	// 朝向不透明邻居的面已被上方 Opaque(adjacent) 拦下；其余方向（包括门对门、
	// 门对水/作物）按薄板语义可见，避免门被相邻非不透明方块错误剔除。
	// 方向相关的贴边/旋转剔除由几何阶段按 DoorDir 决定，此处只保留薄板可见性
	// 基准，不引入方向分支，保持快照位图与后续几何一致。
	if core.IsDoor(id) {
		if adjacent == core.AirID {
			return true
		}
		// 门对非不透明方块（水、玻璃、树叶、另一扇门等）仍需出面，否则门面会在
		// 这些方块旁消失；门对门的内部面由两侧几何各自决定，此处不过滤。
		return true
	}
	// 床是 9/16 半高方块，与门同属「本身非不透明、不满格」的薄几何：朝空气
	// 与一切非不透明邻居都需出面（否则床贴着玻璃/另一张床时侧面被错误剔除）；
	// 朝不透明邻居的面已被上方 Opaque(adjacent) 拦下。半高贴边细节由模型
	// 几何阶段承载，可见性位图只保留薄几何的基准可见性。
	if core.IsBed(id) {
		return true
	}
	if adjacent == core.AirID {
		return true
	}
	return r.Opaque(id)
}

// Material 返回方块某个面的材质层号。实现 mesh.Registry。
func (r *Registry) Material(id world.BlockID, f mesh.Face) uint16 {
	switch id {
	case core.StoneID:
		return LayerStone
	case core.DirtID:
		return LayerDirt
	case core.BedrockID:
		return LayerBedrock
	case core.StoneBrickID:
		return LayerStoneBrick
	case core.CoalOreID:
		return LayerCoalOre
	case core.IronOreID:
		return LayerIronOre
	case core.FurnaceID:
		return LayerFurnace
	case core.IronBlockID:
		return LayerIronBlock
	case core.ChestID:
		return LayerChest
	case core.LightBlockID:
		return LayerLightBlock
	case core.LeavesID:
		return LayerLeaves
	case core.GlassID:
		return LayerGlass
	case core.CobblestoneID:
		return LayerCobblestone
	case core.SmoothStoneID:
		return LayerSmoothStone
	case core.SandID:
		return LayerSand
	case core.GravelID:
		return LayerGravel
	case core.OakLogID:
		if f == mesh.FacePosY || f == mesh.FaceNegY {
			return LayerOakLogTop
		}
		return LayerOakLogSide
	case core.OakPlanksID:
		return LayerOakPlanks
	case core.BrickID:
		return LayerBrick
	case core.WhiteWoolID:
		return LayerWhiteWool
	case core.RoofTileID:
		return LayerRoofTile
	case core.ClayID:
		return LayerClay
	case core.SnowBlockID:
		if f == mesh.FacePosY {
			return LayerSnowTop
		}
		return LayerSnowSide
	case core.MossyCobblestoneID:
		return LayerMossyCobblestone
	// 门：上下半共用同一木门层，单值材质，无方向分支。
	case core.DoorLowerSouthClosed, core.DoorLowerSouthOpen, core.DoorLowerWestClosed, core.DoorLowerWestOpen, core.DoorLowerNorthClosed, core.DoorLowerNorthOpen, core.DoorLowerEastClosed, core.DoorLowerEastOpen, core.DoorUpper:
		return LayerDoor
	// 工作台：顶面是操作台面、底面是素箱底、四侧共用侧面板——与原木、雪块
	// 同形的「按轴分层」立方体。
	case core.WorkbenchID:
		if f == mesh.FacePosY {
			return LayerWorkbenchTop
		}
		if f == mesh.FaceNegY {
			return LayerWorkbenchBottom
		}
		return LayerWorkbenchSide
	case core.FarmlandDryID:
		return LayerFarmlandDry
	case core.FarmlandWetID:
		return LayerFarmlandWet
	case core.GrassID:
		switch f {
		case mesh.FacePosY:
			return LayerGrassTop
		case mesh.FaceNegY:
			return LayerDirt
		default:
			return LayerGrassSide
		}
	default:
		// 8 个流体编号共用同一个水材质层：mesh 的 registry 条目每方块只有
		// 6 个 material，塞不下等级；等级信息走独立的 FluidHeight 字段。
		if core.IsFluid(id) {
			return LayerWater
		}
		// 24 个作物阶段（小麦/马铃薯/胡萝卜各 8）各占一层，六个面共用同一层：交叉斜面没有"朝向"可言，
		// 而 Rust mesher 正是靠「六个面的 material 都落在植物区间」认出植物格的。
		if core.IsPotato(id) {
			return LayerPotato0 + uint16(core.CropStage(id))
		}
		if core.IsCarrot(id) {
			return LayerCarrot0 + uint16(core.CropStage(id))
		}
		if core.IsCrop(id) {
			return LayerWheat0 + uint16(core.CropStage(id))
		}
		if core.IsWildGrass(id) {
			return LayerShortGrass
		}
		// 五种火把形态共用同一张竖直火柄 cutout 层：Rust 的 model dispatcher
		// 只读 face 0 的 material，几何（交叉斜面/贴面斜板）由 model tag 决定，
		// 材质六面同层不会串味。
		if core.IsTorch(id) {
			return LayerTorch
		}
		// 床的八个形态按「顶面床面层 + 其余面床架层」分层（与原木、雪块、
		// 工作台同形的按面分层）：顶面各用与朝向冻结同序的专属床面层，四个
		// 侧面与底面复用橡木木板层表达橡木床架。Rust 的 model dispatcher 按
		// face 枚举序（0..5 = −X/+X/−Y/+Y/−Z/+Z）取材质：平顶 quad 读
		// face 3（PosY）取床面层、四片侧板各读自身面材质落在橡木板层——
		// 本分支的按面取值与 emit_bed 的按面读取必须逐面同序，错位即材质缝。
		if core.IsBed(id) {
			if f == mesh.FacePosY {
				return bedTopLayer(id)
			}
			return LayerOakPlanks
		}
		return LayerStone
	}
}

// bedTopLayer 返回床形态对应的床面层号：床尾四向 LayerBedFootSouth..East、
// 床头四向同序平移 4。调用方保证 id 是床方块；层排布与方块编号冻结顺序
// 严格同形（床头 = 床尾 + 4），这是「朝向 ↔ 层」的唯一映射。
func bedTopLayer(id core.BlockID) uint16 {
	return LayerBedFootSouth + uint16(id-core.BedFootSouthID)
}

// Emission 返回方块固定发出的方块光等级。实现 mesh.Registry。
//
// 发光判定完全转调 core.BlockEmission——那是全仓唯一的光源表（发光方块 15、
// 五种火把形态 14、其余 0），本包不得保留任何重复分支；新增发光方块只改
// core 一张表，这里与 mesh registry 快照自动跟随。
func (r *Registry) Emission(id world.BlockID) uint8 {
	return core.BlockEmission(id)
}

// fluidSourceHeightRaw 是源方块（level 0）的 4-bit 高度原值。
//
// 取 14 而非 15：实际高度是 (h_raw+1)/16，14 即 15/16，让源方块的水面比方块顶面
// 略低一线，水柱内部再由「上方是流体则取满格 15」补齐（见 mesh 的角高度推导）。
// 于是 h_raw(level) = 14 - level，最弱的 level 7 仍有 7 即半格高度，不会退化成
// 零面积的水面。
const fluidSourceHeightRaw = 14

// FluidHeight 返回方块孤立时的 4-bit 流体高度原值 h_raw。实现 mesh.Registry。
//
// 非流体返回 0 这个哨兵值——真流体的 h_raw 恒在 7..=14，0 不会与之冲突。
// 本函数是「流体等级 → 高度」映射的**唯一**真值源：它被烘焙进 mesh registry
// 快照送给 Rust，Rust 只消费高度、不知道等级。
func (r *Registry) FluidHeight(id world.BlockID) uint8 {
	if !core.IsFluid(id) {
		return 0
	}
	return fluidSourceHeightRaw - core.FluidLevel(id)
}

// LightAttenuation 返回天空光穿过该方块时的额外衰减。实现 mesh.Registry。
//
// 衰减判定完全转调 core.BlockLightAttenuation——与发光表同为 core 的单一事实
// 源（八个流体编号 1、其余 0），本包不得保留任何重复分支。值经 registry 快照
// 送过 ABI 边界，由 Rust 的天空光 BFS 逐步查表扣减——竖直向下穿过流体因此
// 不再无损。方块光模型不消费本值：水与玻璃一样直接阻断。
func (r *Registry) LightAttenuation(id world.BlockID) uint8 {
	return core.BlockLightAttenuation(id)
}

// farmlandTopRaw 是干/湿耕地共用的 4-bit 顶面高度原值。
//
// 取 14 而非更小的值：实际呈现高度是 (14+1)/16 = 15/16，与 internal/physics
// 的耕地碰撞体高度（`farmlandCollisionHeight` = 0.9375）完全一致——可见几何与
// 碰撞边界从此同线，玩家站在耕地上时脚部位置与顶面齐平。14 也是高度域
// 1..=14 的上界：15 非法（满格必须用哨兵 0 表达），没有再高的选择余地。
const farmlandTopRaw = 14

// BlockTopRaw 返回方块的 4-bit 顶面高度原值。实现 mesh.RegistryReader。
//
// 只有干/湿耕地返回非零（见 `farmlandTopRaw`）；其余方块——包括全部流体——
// 返回「满格」哨兵 0。流体的 0 不只是缺省：mesher 对流体的角高度走邻域
// 平均、对 block_top_raw 走常量，两条几何路径互斥，编码两侧的域校验同样按
// 「`FluidHeight` 与 `BlockTopRaw` 不同时非零」拒绝（见 internal/mesh 的
// `BuildRegistrySnapshot` 与 Rust 的 `RegistryView::validate`）。
func (r *Registry) BlockTopRaw(id world.BlockID) uint8 {
	if id == core.FarmlandDryID || id == core.FarmlandWetID {
		return farmlandTopRaw
	}
	return 0
}

// MeshSnapshot 返回构造时冻结的网格 registry 快照。
func (r *Registry) MeshSnapshot() mesh.RegistrySnapshot { return r.meshSnapshot }

// Model 返回方块的有限模型 tag，实现 mesh.ModelReader（可选扩展接口）。
//
// 封闭集合：0=默认（无模型覆写，满格/短方块/流体/植物继续走既有判定）、
// 1=火把落地、2..5=火把墙面 +X/−X/+Z/−Z——与火把方块编号 71..75 严格同序，
// 因此映射就是「编号相对首形态的偏移 +1」；6=床，床尾/床头 × 四向八形态
// 共用同一床几何（9/16 半高板），朝向差异由逐形态床面材质层表达而非几何。
// 其余全部方块（含未注册与越界编号）恒 0；tag 经 BuildRegistrySnapshot 冻结
// 进快照，由 Rust greedy 的 model dispatcher 消费（7 起的未知值在快照与
// Rust 两侧都被拒绝）。
func (r *Registry) Model(id world.BlockID) uint8 {
	if core.IsBed(id) {
		return 6
	}
	if !core.IsTorch(id) {
		return 0
	}
	return uint8(id - core.TorchStandingID + 1)
}

// isCutoutLayer 报告某个材质层是否走 alpha cutout（二值 alpha + 保覆盖率降采样）。
//
// 判据必须与 terrain.wgsl 里 `c.a < 0.5` 那条 discard 覆盖的层集合一致：这些层的
// mip 链要用 downsampleCutout 保住覆盖率，否则远处的细结构会整片消失。裂纹层
// 同属此类且更要紧：裂纹像素在 16×16 里本就稀疏，普通平均降采样会把 alpha
// 一路稀释到 0，整条裂纹在远处无影无踪。
func isCutoutLayer(layer int) bool {
	return layer == int(LayerLeaves) || layer == int(LayerGlass) ||
		(layer >= int(LayerWheat0) && layer <= int(LayerCarrot7)) || layer == int(LayerTorch) ||
		layer == int(LayerShortGrass) ||
		(layer >= int(LayerCrack0) && layer <= int(LayerCrack9))
}

func (r *Registry) LayerCount() int { return int(layerCount) }

func (r *Registry) LayerRGBA(layer int) []byte { return r.layers[layer] }

var _ mesh.Registry = (*Registry)(nil)

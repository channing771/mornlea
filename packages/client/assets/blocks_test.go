package assets_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

func TestRegistryFaceVisible(t *testing.T) {
	r := assets.NewRegistry()
	tests := []struct {
		name         string
		id, adjacent core.BlockID
		want         bool
	}{
		{"空气不出面", core.AirID, core.AirID, false},
		// 未注册编号一律用独占哨兵 core.BlockIDMax 表达：写死具体编号（历史上
		// 写过 MossyCobblestoneID+1、WaterLevel7ID+1）会在追加新方块时静默
		// 变成已注册。
		{"未知当前方块不出面", core.BlockIDMax, core.AirID, false},
		{"石头面向空气", core.StoneID, core.AirID, true},
		{"石头面向未知方块关闭", core.StoneID, core.BlockIDMax, false},
		// 流体已纳入 mesh snapshot 的 ids 范围：水面对空气出面，水下地形
		// 也要透过水出面。id 侧和 adjacent 侧都要覆盖到。
		{"流体面向空气出面", core.WaterSourceID, core.AirID, true},
		{"石头面向流体出面", core.StoneID, core.WaterLevel1ID, true},
		{"石头被石头遮住", core.StoneID, core.StoneID, false},
		{"石头面向玻璃保留", core.StoneID, core.GlassID, true},
		{"玻璃被石头遮住", core.GlassID, core.StoneID, false},
		{"玻璃同类内部面剔除", core.GlassID, core.GlassID, false},
		{"树叶同类内部面剔除", core.LeavesID, core.LeavesID, false},
		{"不同 cutout 内部面剔除", core.GlassID, core.LeavesID, false},
		{"玻璃面向空气", core.GlassID, core.AirID, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.FaceVisible(world.BlockID(tt.id), world.BlockID(tt.adjacent)); got != tt.want {
				t.Fatalf("FaceVisible(%d, %d) = %v，想要 %v", tt.id, tt.adjacent, got, tt.want)
			}
		})
	}
}

// TestFluidBlocksAreTransparentAndDark 锁定「mesh 注册表登记流体」：8 个流体
// 编号 Opaque 一律为 false（沿用既有透明方块路径，不被当作不透明遮挡体），
// Emission 一律为 0（不发光）。
func TestFluidBlocksAreTransparentAndDark(t *testing.T) {
	registry := assets.NewRegistry()
	for id := core.WaterSourceID; id <= core.WaterLevel7ID; id++ {
		if registry.Opaque(id) {
			t.Fatalf("流体方块 %d 的 Opaque 应为 false", id)
		}
		if got := registry.Emission(id); got != 0 {
			t.Fatalf("流体方块 %d 的 Emission = %d，想要 0", id, got)
		}
	}
}

// TestFluidHeightMapsLevelToRawHeightExhaustively 对 8 个流体编号穷举断言
// h_raw(level) = 14 - level：源方块 14（即 15/16），最弱的 level 7 得 7（即
// 半格），并断言 0 这个「非流体」哨兵在全部已注册非流体编号上成立——这是
// 「0 不会与合法流体高度冲突」这条位布局前提的可执行守卫。
func TestFluidHeightMapsLevelToRawHeightExhaustively(t *testing.T) {
	registry := assets.NewRegistry()
	for level := uint8(0); level <= 7; level++ {
		id := core.WaterSourceID + world.BlockID(level)
		if got, want := registry.FluidHeight(id), 14-level; got != want {
			t.Fatalf("FluidHeight(level=%d) = %d，想要 %d", level, got, want)
		}
		if got := registry.LightAttenuation(id); got != 1 {
			t.Fatalf("LightAttenuation(level=%d) = %d，想要 1", level, got)
		}
	}
	// 高度必须严格随等级递减，且最弱等级仍有半格（7 即 8/16），不会退化成零面。
	if got, want := registry.FluidHeight(core.WaterLevel7ID), uint8(7); got != want {
		t.Fatalf("最弱流体 FluidHeight=%d，想要 %d", got, want)
	}
	// 上界必须是「全部已注册非流体方块」，不能只扫到流体编号之前：流体之后
	// 追加的编号（农业方块）同样是非流体，写死 WaterSourceID 会让它们漏测。
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if core.IsFluid(id) {
			continue
		}
		if got := registry.FluidHeight(id); got != 0 {
			t.Fatalf("非流体 %d 的 FluidHeight=%d，想要哨兵 0", id, got)
		}
		if got := registry.LightAttenuation(id); got != 0 {
			t.Fatalf("非流体 %d 的 LightAttenuation=%d，想要 0", id, got)
		}
	}
}

// TestBlockTopRawSinksFarmlandOnly 钉住顶面高度通道的第一个消费者：干/湿
// 耕地填 14（呈现高度 15/16，与物理碰撞体 farmlandCollisionHeight 一致），
// 其余全部已注册方块——含 8 个流体编号——都是「满格」哨兵 0。
//
// 流体的 0 不只是缺省：mesher 对流体走邻域平均角高度、对 block_top_raw 走
// 常量，两条几何路径互斥，这里从数据源头保证流体永远不进常量路径。
func TestBlockTopRawSinksFarmlandOnly(t *testing.T) {
	registry := assets.NewRegistry()
	for _, id := range []core.BlockID{core.FarmlandDryID, core.FarmlandWetID} {
		if got := registry.BlockTopRaw(id); got != 14 {
			t.Fatalf("耕地 %d 的 BlockTopRaw=%d，想要 14（15/16 呈现高度）", id, got)
		}
	}
	// 上界必须是「全部已注册方块」：写死某个具体末位编号会在追加新方块时
	// 静默退化成子集，新方块就漏出「非耕地必须为哨兵 0」的检查。
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if id == core.FarmlandDryID || id == core.FarmlandWetID {
			continue
		}
		if got := registry.BlockTopRaw(id); got != 0 {
			t.Fatalf("非耕地 %d 的 BlockTopRaw=%d，想要满格哨兵 0", id, got)
		}
	}
}

func TestRegistryMeshSnapshotMatchesRegistry(t *testing.T) {
	registry := assets.NewRegistry()
	snapshot := registry.MeshSnapshot()
	// 快照必须逐条覆盖全部已注册方块：条目数与上界都用独占哨兵 core.BlockIDMax
	// 表达，写死具体末位编号会让新追加的方块静默漏出本用例。
	if got, want := len(snapshot.Blocks), int(core.BlockIDMax); got != want {
		t.Fatalf("snapshot block 数=%d，想要 %d", got, want)
	}
	for id := core.AirID; id < core.BlockIDMax; id++ {
		block := snapshot.Blocks[int(id)]
		if block.FluidHeight != registry.FluidHeight(id) || block.LightAttenuation != registry.LightAttenuation(id) ||
			block.BlockTopRaw != registry.BlockTopRaw(id) {
			t.Fatalf("block %d snapshot 的流体/衰减/顶面高度字段=%+v", id, block)
		}
		if block.ID != id || block.Opaque != registry.Opaque(id) || block.Emission != registry.Emission(id) {
			t.Fatalf("block %d snapshot=%+v", id, block)
		}
		for face := mesh.Face(0); face < 6; face++ {
			if got, want := block.Materials[face], registry.Material(id, face); got != want {
				t.Fatalf("block %d face %d material=%d，想要 %d", id, face, got, want)
			}
		}
		for adjacent := core.AirID; adjacent < core.BlockIDMax; adjacent++ {
			if got, want := snapshot.FaceVisible(id, adjacent), registry.FaceVisible(id, adjacent); got != want {
				t.Fatalf("FaceVisible(%d, %d)=%v，想要 %v", id, adjacent, got, want)
			}
		}
	}
}

func TestStoneBrickHasOwnMaterialLayer(t *testing.T) {
	registry := assets.NewRegistry()
	layer := registry.Material(core.StoneBrickID, mesh.FacePosY)
	if layer != assets.LayerStoneBrick {
		t.Fatalf("石砖材质层 = %d，想要 %d", layer, assets.LayerStoneBrick)
	}
	pixels := registry.LayerRGBA(int(layer))
	stone := registry.LayerRGBA(int(assets.LayerStone))
	if len(pixels) != len(stone) {
		t.Fatalf("石砖材质长度 = %d，想要与石头一致 %d", len(pixels), len(stone))
	}
	if string(pixels) == string(stone) {
		t.Fatal("石砖材质与石头完全相同")
	}
}

func TestM4EBlocksHaveDistinctMaterialLayers(t *testing.T) {
	registry := assets.NewRegistry()
	seen := map[uint32]core.BlockID{}
	for _, block := range []core.BlockID{
		core.StoneID, core.StoneBrickID,
		core.CoalOreID, core.IronOreID, core.FurnaceID, core.IronBlockID,
		core.ChestID,
	} {
		layer := registry.Material(block, mesh.FacePosY)
		if other, dup := seen[uint32(layer)]; dup {
			t.Fatalf("方块 %d 与 %d 共用材质层 %d", block, other, layer)
		}
		seen[uint32(layer)] = block
		if len(registry.LayerRGBA(int(layer))) == 0 {
			t.Fatalf("方块 %d 的材质层为空", block)
		}
	}
}

func TestCutoutBlocksHaveOwnMaterialLayers(t *testing.T) {
	registry := assets.NewRegistry()
	for _, tt := range []struct {
		block core.BlockID
		want  uint16
	}{
		{core.LeavesID, assets.LayerLeaves},
		{core.GlassID, assets.LayerGlass},
	} {
		if got := registry.Material(tt.block, mesh.FacePosY); got != tt.want {
			t.Fatalf("方块 %d 材质层 = %d，想要 %d", tt.block, got, tt.want)
		}
	}
}

func TestCommonMaterialFaceMappings(t *testing.T) {
	r := assets.NewRegistry()
	if r.Material(core.OakLogID, mesh.FacePosY) != assets.LayerOakLogTop ||
		r.Material(core.OakLogID, mesh.FaceNegY) != assets.LayerOakLogTop ||
		r.Material(core.OakLogID, mesh.FacePosX) != assets.LayerOakLogSide {
		t.Fatal("竖向原木顶底/侧面映射错误")
	}
	for _, tt := range []struct {
		block core.BlockID
		layer uint16
	}{
		{core.CobblestoneID, assets.LayerCobblestone},
		{core.SmoothStoneID, assets.LayerSmoothStone},
		{core.SandID, assets.LayerSand},
		{core.GravelID, assets.LayerGravel},
		{core.OakPlanksID, assets.LayerOakPlanks},
		{core.LeavesID, assets.LayerLeaves},
		{core.GlassID, assets.LayerGlass},
		{core.BrickID, assets.LayerBrick},
		{core.WhiteWoolID, assets.LayerWhiteWool},
		{core.RoofTileID, assets.LayerRoofTile},
		{core.ClayID, assets.LayerClay},
		{core.MossyCobblestoneID, assets.LayerMossyCobblestone},
	} {
		if got := r.Material(tt.block, mesh.FacePosX); got != tt.layer {
			t.Fatalf("方块 %d 材质层=%d，想要 %d", tt.block, got, tt.layer)
		}
		if r.Material(tt.block, mesh.FacePosY) != tt.layer {
			t.Fatalf("方块 %d 不应按面变化", tt.block)
		}
	}
	if r.Material(core.SnowBlockID, mesh.FacePosY) != assets.LayerSnowTop ||
		r.Material(core.SnowBlockID, mesh.FacePosX) != assets.LayerSnowSide {
		t.Fatal("雪块顶面与侧面应使用不同材质")
	}
}

// TestChestHasOwnMaterialLayer 覆盖箱子拥有独立于木质褐色相邻方块的材质层。
func TestChestHasOwnMaterialLayer(t *testing.T) {
	registry := assets.NewRegistry()
	layer := registry.Material(core.ChestID, mesh.FacePosY)
	if layer != assets.LayerChest {
		t.Fatalf("箱子材质层 = %d，想要 %d", layer, assets.LayerChest)
	}
	pixels := registry.LayerRGBA(int(layer))
	dirt := registry.LayerRGBA(int(assets.LayerDirt))
	if len(pixels) != len(dirt) {
		t.Fatalf("箱子材质长度 = %d，想要与泥土一致 %d", len(pixels), len(dirt))
	}
	if string(pixels) == string(dirt) {
		t.Fatal("箱子材质与泥土完全相同")
	}
}

// TestLightBlockUsesIndependentLayerAndFixedEmission 杀死复用任一既有层、
// 发光块边框或中心颜色错误，以及发光等级或非光源默认值错误的变异。
func TestLightBlockUsesIndependentLayerAndFixedEmission(t *testing.T) {
	registry := assets.NewRegistry()
	layer := registry.Material(core.LightBlockID, mesh.FacePosY)
	if layer != assets.LayerLightBlock {
		t.Fatalf("发光块材质层=%d，想要 %d", layer, assets.LayerLightBlock)
	}
	pixels := registry.LayerRGBA(int(layer))
	if len(pixels) != 16*16*4 {
		t.Fatalf("发光块材质长度=%d，想要 %d", len(pixels), 16*16*4)
	}
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 255 {
			t.Fatalf("像素 %d alpha=%d，想要 255", i/4, pixels[i])
		}
	}
	if got := [4]byte{pixels[0], pixels[1], pixels[2], pixels[3]}; got != [4]byte{164, 106, 30, 255} {
		t.Fatalf("发光块边框 RGBA=%v，想要 [164 106 30 255]", got)
	}
	center := (7*16 + 7) * 4
	if got := [4]byte{pixels[center], pixels[center+1], pixels[center+2], pixels[center+3]}; got != [4]byte{255, 226, 112, 255} {
		t.Fatalf("发光块中心 RGBA=%v，想要 [255 226 112 255]", got)
	}
	if got := registry.Emission(core.LightBlockID); got != 15 {
		t.Fatalf("发光块 Emission=%d，想要 15", got)
	}
	for _, id := range []world.BlockID{core.StoneID, core.ChestID, world.BlockID(999)} {
		if got := registry.Emission(id); got != 0 {
			t.Fatalf("非光源 %d Emission=%d，想要 0", id, got)
		}
	}
}

// TestFluidFaceVisibilityRules 穷举流体与全部已注册方块的两个方向组合，锁定三条
// 流体出面规则。它们是 Rust 侧 `RegistryView::face_visible` 实际生效的规则来源：
// Rust 只做位图查表，位图由 BuildRegistrySnapshot 用本函数烘焙，所以规则写在这里、
// 只能在这里被验证。
//
//   - 同为流体的相邻面不可见（水体内部不产生面）；
//   - 流体对空气可见（水面出几何）；
//   - 流体紧邻不透明方块的面不可见（含「流体在实心方块下方」）；
//   - 反方向：不透明方块朝向流体的面可见（水下地形不会消失）。
func TestFluidFaceVisibilityRules(t *testing.T) {
	registry := assets.NewRegistry()
	// 计数用于最后的防空转守卫：三类结论各自都必须真的被走到过。
	var fluidToFluid, fluidToAir, fluidToOpaque, opaqueToFluid int
	for id := core.WaterSourceID; id <= core.WaterLevel7ID; id++ {
		// adjacent 侧要穷举「全部已注册方块」，上界用 core.BlockIDMax。
		for adjacent := core.AirID; adjacent < core.BlockIDMax; adjacent++ {
			got := registry.FaceVisible(id, adjacent)
			switch {
			case core.IsFluid(adjacent):
				if got {
					t.Fatalf("流体 %d 朝向流体 %d 出面了：水体内部不得产生面", id, adjacent)
				}
				fluidToFluid++
			case adjacent == core.AirID:
				if !got {
					t.Fatalf("流体 %d 朝向空气没有出面：水面画不出来", id)
				}
				fluidToAir++
			case registry.Opaque(adjacent):
				if got {
					t.Fatalf("流体 %d 朝向不透明方块 %d 出面了：该面被完全遮住", id, adjacent)
				}
				fluidToOpaque++
				// 反方向必须可见，否则水下地形整片消失。
				if !registry.FaceVisible(adjacent, id) {
					t.Fatalf("不透明方块 %d 朝向流体 %d 没有出面：水下地形会消失", adjacent, id)
				}
				opaqueToFluid++
			}
		}
	}
	// 防空转守卫排在真实故障断言之后：若某一类组合一次都没走到，上面的断言
	// 对该类规则就是恒真的，此时红的应当是这里。
	if fluidToFluid != 8*8 || fluidToAir != 8 || fluidToOpaque == 0 || opaqueToFluid == 0 {
		t.Fatalf("覆盖不足：流体-流体=%d（想要 64）、流体-空气=%d（想要 8）、"+
			"流体-不透明=%d、不透明-流体=%d（后两者均须大于 0）",
			fluidToFluid, fluidToAir, fluidToOpaque, opaqueToFluid)
	}
}

// TestFluidBlocksUseDedicatedWaterMaterialLayer 锁定「流体有独立材质层」：
// 8 个流体编号的全部 6 个面都必须返回 assets.LayerWater，且该层不得与任何
// 非流体方块的任何面共用。
//
// 这条断言承重在于：流体曾经落在 Material 的 `default: return LayerStone`，
// 于是水面会顶着石头纹理混进**不透明** terrain pass。上传路径按 material
// 分流水面 quad，没有独立材质层就没有分流依据。
func TestFluidBlocksUseDedicatedWaterMaterialLayer(t *testing.T) {
	registry := assets.NewRegistry()
	faces := []mesh.Face{
		mesh.FaceNegX, mesh.FacePosX, mesh.FaceNegY,
		mesh.FacePosY, mesh.FaceNegZ, mesh.FacePosZ,
	}
	for id := core.WaterSourceID; id <= core.WaterLevel7ID; id++ {
		for _, face := range faces {
			if got := registry.Material(id, face); got != assets.LayerWater {
				t.Fatalf("Material(%d, face=%d) = %d，想要 LayerWater(%d)",
					id, face, got, assets.LayerWater)
			}
		}
	}
	// 反向守卫：任何非流体的已注册方块都不得落到水层，否则「按 material
	// 分流」会把不透明几何一起拖进半透明 pass。
	// 上界必须是「全部已注册方块」：写死 MossyCobblestoneID 时它恰好是流体之前
	// 的末项，看起来等价于全域，流体之后再追加编号就静默漏测了。
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if core.IsFluid(id) {
			continue
		}
		for _, face := range faces {
			if registry.Material(id, face) == assets.LayerWater {
				t.Fatalf("非流体方块 %d 的 face=%d 落到了水材质层", id, face)
			}
		}
	}
}

// TestPlantMaterialLayersMatchMeshContract 是植物 material 区间在 Go 侧的
// **唯一**机械守卫。
//
// 区间的数值真值源是本包的层枚举，但常量必须住在 packages/client/mesh（assets 依赖
// mesh，反向不成立），Rust 的 quad.rs 还硬编码了第三份。三份没有共享定义也
// 没有生成步骤：在 LayerWheat0 之前插一层会整体平移这段区间，而那件事**不会**
// 让任何材质或渲染断言变红——Rust 会把别的方块当成植物、把小麦当成普通方块。
// 本条钉住 Go 两处相等，跨语言那一侧由 packages/client/mesh 的
// TestNativeOracleParityWheatCrossPlanes 真的喂一次 Rust mesher 兜底。
func TestPlantMaterialLayersMatchMeshContract(t *testing.T) {
	if assets.LayerWheat0 != mesh.PlantMaterialFirst {
		t.Fatalf("LayerWheat0=%d，mesh.PlantMaterialFirst=%d", assets.LayerWheat0, mesh.PlantMaterialFirst)
	}
	if assets.LayerCarrot7 != mesh.PlantMaterialLast {
		t.Fatalf("LayerCarrot7=%d，mesh.PlantMaterialLast=%d", assets.LayerCarrot7, mesh.PlantMaterialLast)
	}
	if assets.LayerShortGrass != mesh.PlantMaterialShortGrass {
		t.Fatalf("LayerShortGrass=%d，mesh.PlantMaterialShortGrass=%d", assets.LayerShortGrass, mesh.PlantMaterialShortGrass)
	}
	// 区间必须恰好覆盖 24 个作物层（小麦 8 + 马铃薯 8 + 胡萝卜 8），一个不多一个不少：区间放宽会把相邻的耕地层
	// 也当成植物，那两层会被渲染成交叉斜面。
	if got := int(mesh.PlantMaterialLast - mesh.PlantMaterialFirst + 1); got != 24 {
		t.Fatalf("植物 material 区间覆盖 %d 层，想要 24", got)
	}
	for _, layer := range []uint16{assets.LayerFarmlandDry, assets.LayerFarmlandWet, assets.LayerWater, assets.LayerLeaves} {
		if mesh.PlantMaterial(layer) {
			t.Fatalf("非植物层 %d 落进了植物 material 区间", layer)
		}
	}
}

// TestCropsAreCutoutAndEmitNoAxialFaces 锁定 Ruling 6：作物与玻璃、树叶同类。
//
// 每一条都是**位置性**的：耕地作为同批新增的农业方块留在对照组里，它必须仍然
// 不透明、仍然照常出面。若把作物的规则误写成"整批农业方块"，耕地那半边立刻红。
// 覆盖小麦/马铃薯/胡萝卜全部 24 个阶段（3×8），新增作物必须同为 cutout 且不出轴向面。
func TestCropsAreCutoutAndEmitNoAxialFaces(t *testing.T) {
	r := assets.NewRegistry()
	for id := core.WheatStage0ID; id <= core.CarrotStage7ID; id++ {
		if !core.IsCrop(id) {
			continue
		}
		if r.Opaque(id) {
			t.Fatalf("作物 %d 是不透明的：它必须与玻璃、树叶同类，否则会挡死下方耕地的天空光", id)
		}
		if r.LightAttenuation(id) != 0 {
			t.Fatalf("作物 %d 的天空光额外衰减=%d，想要 0", id, r.LightAttenuation(id))
		}
		if r.FluidHeight(id) != 0 {
			t.Fatalf("作物 %d 的 FluidHeight=%d，想要非流体哨兵 0", id, r.FluidHeight(id))
		}
		if r.Emission(id) != 0 {
			t.Fatalf("作物 %d 会发光", id)
		}
		// 作物自己一个轴向面都不出：几何全部来自 Rust 补的交叉斜面。
		for _, adjacent := range []world.BlockID{core.AirID, core.GlassID, core.WaterSourceID, core.WheatStage0ID, core.FarmlandDryID} {
			if r.FaceVisible(id, adjacent) {
				t.Fatalf("作物 %d 对相邻 %d 出了轴向面", id, adjacent)
			}
		}
		// 反方向不受影响：相邻方块朝向作物的面仍然可见。
		if !r.FaceVisible(core.StoneID, id) {
			t.Fatalf("石头朝向作物 %d 的面消失了：作物非不透明，不该遮挡邻居出面", id)
		}
	}

	// 对照组：耕地是视觉上的满立方体，仍然不透明、仍然出面。
	for _, id := range []world.BlockID{core.FarmlandDryID, core.FarmlandWetID} {
		if !r.Opaque(id) {
			t.Fatalf("耕地 %d 不再是不透明方块", id)
		}
		if !r.FaceVisible(id, core.AirID) {
			t.Fatalf("耕地 %d 朝向空气的面消失了", id)
		}
	}
}

// TestCropsUseDedicatedWheatMaterialLayers 钉住 24 个阶段各占一层、六面同层（小麦/马铃薯/胡萝卜各 8）。
//
// 六面同层不是可有可无的细节：Rust 的 is_plant 只读 face 0 的 material，某个面
// 被写成别的层就会让那一面在别处被当成普通方块。
func TestCropsUseDedicatedWheatMaterialLayers(t *testing.T) {
	r := assets.NewRegistry()
	seen := map[uint16]world.BlockID{}
	for id := core.WheatStage0ID; id <= core.CarrotStage7ID; id++ {
		if !core.IsCrop(id) {
			continue
		}
		want := r.Material(id, mesh.FaceNegX)
		if !mesh.PlantMaterial(want) {
			t.Fatalf("作物 %d 的 material=%d 不在植物区间内", id, want)
		}
		for face := mesh.Face(0); face < 6; face++ {
			if got := r.Material(id, face); got != want {
				t.Fatalf("作物 %d 的面 %d material=%d，想要与其余面一致的 %d", id, face, got, want)
			}
		}
		if other, ok := seen[want]; ok {
			t.Fatalf("作物 %d 与 %d 共用材质层 %d：24 个阶段必须各占一层", id, other, want)
		}
		seen[want] = id
	}
	if len(seen) != 24 {
		t.Fatalf("24 个阶段只用了 %d 个材质层", len(seen))
	}
	// 耕地干湿必须各占一层，且都不是小麦层。
	dry := r.Material(core.FarmlandDryID, mesh.FacePosY)
	wet := r.Material(core.FarmlandWetID, mesh.FacePosY)
	if dry == wet {
		t.Fatal("耕地干湿共用同一材质层")
	}
	if mesh.PlantMaterial(dry) || mesh.PlantMaterial(wet) {
		t.Fatalf("耕地材质层 %d/%d 落进了植物区间", dry, wet)
	}
}

// TestRegistryLightDelegatesToCore 锁定「唯一发光判定表」：Registry 的
// Emission/LightAttenuation 对全部已注册方块与越界编号都必须与 core 的两张表
// 逐点恒等——assets 不得保留任何与 core 重复的判定分支，服务端生成判定与
// 客户端注册表都只消费 core 这一张表。
func TestRegistryLightDelegatesToCore(t *testing.T) {
	registry := assets.NewRegistry()
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if got, want := registry.Emission(id), core.BlockEmission(id); got != want {
			t.Fatalf("Emission(%d) = %d，与 core.BlockEmission 的 %d 不一致", id, got, want)
		}
		if got, want := registry.LightAttenuation(id), core.BlockLightAttenuation(id); got != want {
			t.Fatalf("LightAttenuation(%d) = %d，与 core.BlockLightAttenuation 的 %d 不一致", id, got, want)
		}
	}
	// 越界编号（哨兵与其后一格、远端编号）同样转调恒等，均为 0。
	for _, id := range []world.BlockID{core.BlockIDMax, core.BlockIDMax + 1, world.BlockID(65535)} {
		if got := registry.Emission(id); got != 0 {
			t.Fatalf("Emission(越界 %d) = %d，想要 0", id, got)
		}
		if got := registry.LightAttenuation(id); got != 0 {
			t.Fatalf("LightAttenuation(越界 %d) = %d，想要 0", id, got)
		}
	}
	// 关键取值点名：发光方块 15、五种火把 14、流体衰减 1。
	if got := registry.Emission(core.LightBlockID); got != 15 {
		t.Fatalf("Emission(发光方块) = %d，想要 15", got)
	}
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		if got := registry.Emission(id); got != 14 {
			t.Fatalf("Emission(火把形态 %d) = %d，想要 14", id, got)
		}
	}
	for id := core.WaterSourceID; id <= core.WaterLevel7ID; id++ {
		if got := registry.LightAttenuation(id); got != 1 {
			t.Fatalf("LightAttenuation(流体 %d) = %d，想要 1", id, got)
		}
	}
}

// TestRegistryOpaqueDelegatesToCore 锁定「唯一不透明判定表」：Registry 的
// Opaque 对全部已注册方块与越界编号都必须与 core.BlockOpaque 逐点恒等——
// assets 不得保留任何与 core 重复的判定分支，服务端夜行者的局部暗度判定与
// 客户端注册表都只消费 core 这一张表。
func TestRegistryOpaqueDelegatesToCore(t *testing.T) {
	registry := assets.NewRegistry()
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if got, want := registry.Opaque(id), core.BlockOpaque(id); got != want {
			t.Fatalf("Opaque(%d) = %v，与 core.BlockOpaque 的 %v 不一致", id, got, want)
		}
	}
	// 越界编号（哨兵与其后一格、远端编号）同样转调恒等，均为 false。
	for _, id := range []world.BlockID{core.BlockIDMax, core.BlockIDMax + 1, world.BlockID(65535)} {
		if got, want := registry.Opaque(id), core.BlockOpaque(id); got != want {
			t.Fatalf("Opaque(越界 %d) = %v，与 core.BlockOpaque 的 %v 不一致", id, got, want)
		}
	}
}

// TestTorchFormsAreNotOpaque 锁定火把方块的注册表属性：五种形态全部非不透明
// （与玻璃、树叶、作物同属透明类），否则会遮挡邻面、阻断光照。
func TestTorchFormsAreNotOpaque(t *testing.T) {
	registry := assets.NewRegistry()
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		if registry.Opaque(id) {
			t.Fatalf("火把形态 %d 是不透明方块：火把必须与玻璃、作物同类", id)
		}
	}
}

// TestTorchEmissionEntersMeshSnapshot 锁定「火把与发光方块的 emission 经同一
// 张表进入 mesh registry 快照」：快照是发光值送过 ABI 边界的唯一通道，五种
// 形态都必须以 14 冻结在快照里。
func TestTorchEmissionEntersMeshSnapshot(t *testing.T) {
	registry := assets.NewRegistry()
	snapshot := registry.MeshSnapshot()
	if got, want := len(snapshot.Blocks), int(core.BlockIDMax); got != want {
		t.Fatalf("snapshot block 数 = %d，想要覆盖全部已注册方块的 %d", got, want)
	}
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		if got := snapshot.Blocks[int(id)].Emission; got != 14 {
			t.Fatalf("snapshot 中火把形态 %d 的 Emission = %d，想要 14", id, got)
		}
	}
	if got := snapshot.Blocks[int(core.LightBlockID)].Emission; got != 15 {
		t.Fatalf("snapshot 中发光方块的 Emission = %d，想要 15", got)
	}
}

// TestTorchFormsUseDedicatedCutoutLayer 锁定火把的原创程序化材质契约：
// 五种形态六面共用同一层（Rust mesher 的 model dispatcher 只读 face 0 的
// material，六面同层才不会在某一面串味）、层号冻结为 59（terrain.wgsl 的
// torch_material 门控函数与 Rust 侧的 TORCH_MATERIAL 常量各自硬编码该值，
// 本断言是 Go 侧的唯一机械钉子）、alpha 只取 0/255 的 cutout 语义、与既有
// 全部层逐像素不同；内嵌默认包携带同槽位贴图并覆盖该层，程序化像素仍是
// 独立可构造的回退基线。
func TestTorchFormsUseDedicatedCutoutLayer(t *testing.T) {
	registry := assets.NewRegistry()
	if got := assets.LayerTorch; got != 59 {
		t.Fatalf("LayerTorch=%d，想要冻结值 59（Rust 着色器门控与常量都硬编码该层号）", got)
	}
	if mesh.PlantMaterial(assets.LayerTorch) {
		t.Fatalf("火把材质层 %d 落进植物区间：会被渲染成交叉斜面而非模型几何", assets.LayerTorch)
	}
	for id := core.TorchStandingID; id <= core.TorchWallNegZID; id++ {
		for face := mesh.Face(0); face < 6; face++ {
			if got := registry.Material(id, face); got != assets.LayerTorch {
				t.Fatalf("火把形态 %d 的 face %d 材质层=%d，想要共用 LayerTorch(%d)",
					id, face, got, assets.LayerTorch)
			}
		}
	}
	// 反向守卫：非火把方块的任何面都不得落到火把层。
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if core.IsTorch(id) {
			continue
		}
		for face := mesh.Face(0); face < 6; face++ {
			if got := registry.Material(id, face); got == assets.LayerTorch {
				t.Fatalf("非火把方块 %d 的 face %d 落到了火把材质层", id, face)
			}
		}
	}

	px := registry.LayerRGBA(int(assets.LayerTorch))
	if len(px) != 16*16*4 {
		t.Fatalf("火把材质长度=%d，想要 %d", len(px), 16*16*4)
	}
	opaque, transparent := 0, 0
	for i := 3; i < len(px); i += 4 {
		switch px[i] {
		case 0:
			transparent++
		case 255:
			opaque++
		default:
			t.Fatalf("火把层像素 %d 的 alpha=%d，cutout 只允许 0/255", i/4, px[i])
		}
	}
	if opaque == 0 || transparent == 0 {
		t.Fatalf("火把层不同时包含透明(%d)与不透明(%d)像素", transparent, opaque)
	}
	for layer := 0; layer < registry.LayerCount(); layer++ {
		if uint16(layer) == assets.LayerTorch {
			continue
		}
		if string(px) == string(registry.LayerRGBA(layer)) {
			t.Fatalf("火把层与既有第 %d 层逐像素相同", layer)
		}
	}
	// 内嵌默认包携带火把贴图：默认火把层即包内像素，程序化像素仍是独立
	// 可构造的回退基线；包内文件一旦缺失或改名，这里会先于呈现变红。
	if got, want := assets.NewDefaultRegistry().LayerRGBA(int(assets.LayerTorch)), px; string(got) == string(want) {
		t.Fatal("默认材质包未覆盖火把层：换肤后火把纹理必须来自内嵌新包")
	}
}

// TestTorchTextureIsNarrowHandleWithWarmFlame 锁定火把图层的像素结构：窄木柄
// （中列棕柄自底向上）+ 顶部暖色火芯（外橙内黄），其余透明。火芯刻意画在
// 第 2 行及以下：墙面形态的斜板顶缘只抬到 14/16，纹理前两行被几何裁掉，
// 火芯再往上画墙面火把就只剩木柄。
func TestTorchTextureIsNarrowHandleWithWarmFlame(t *testing.T) {
	px := assets.NewRegistry().LayerRGBA(int(assets.LayerTorch))
	if len(px) != 16*16*4 {
		t.Fatalf("火把材质长度=%d，想要 %d", len(px), 16*16*4)
	}
	opaqueColumns := map[int]bool{}
	opaque := 0
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if alphaAt(px, x, y) == 0 {
				continue
			}
			opaque++
			opaqueColumns[x] = true
		}
	}
	for x := range opaqueColumns {
		if x < 6 || x > 9 {
			t.Fatalf("第 %d 列出现不透明像素：火把必须是窄柄（仅中间 4 列）", x)
		}
	}
	if opaque < 20 || opaque > 64 {
		t.Fatalf("不透明像素=%d，窄柄火把应在 20..64 之间", opaque)
	}
	// 木柄行（第 7 行及以下）只有中间两列。
	for y := 7; y < 16; y++ {
		for _, x := range [...]int{5, 6, 9, 10} {
			if alphaAt(px, x, y) != 0 {
				t.Fatalf("木柄行 %d 的第 %d 列不透明：柄身必须只有中间两列", y, x)
			}
		}
	}
	flame := pixel(px, 7, 4)
	if flame[0] < 230 || int(flame[0])-int(flame[2]) < 120 {
		t.Fatalf("火芯核心颜色=%v，想要亮暖色（R>=230 且 R-B>=120）", flame)
	}
	edge := pixel(px, 6, 3)
	if edge[0] < 210 || int(edge[0])-int(edge[2]) < 150 {
		t.Fatalf("火芯外圈颜色=%v，想要橙色（R>=210 且 R-B>=150）", edge)
	}
	wood := pixel(px, 7, 12)
	if wood[0] < 90 || wood[0] > 150 || wood[2] > 80 || int(wood[0])-int(wood[2]) < 30 {
		t.Fatalf("木柄颜色=%v，想要棕色（R 90..150、B<=80、R-B>=30）", wood)
	}
	for _, corner := range [][2]int{{0, 0}, {15, 0}, {0, 15}, {15, 15}, {4, 9}} {
		if alphaAt(px, corner[0], corner[1]) != 0 {
			t.Fatalf("角落 (%d,%d) 不透明：火把图层边缘必须透明", corner[0], corner[1])
		}
	}
}

// TestFiniteModelTagsCoverTorchAndBed 锁定有限模型 tag 的生产登记：五种火把
// 形态按方块编号顺序 71..75 → tag 1..5（1=落地、2..5=墙面 +X/−X/+Z/−Z），
// 床八形态（76..83）共用保留的 tag 6，其余全部已注册方块与越界编号保持默认
// 0；tag 随 registry 快照冻结送过 ABI 边界。
func TestFiniteModelTagsCoverTorchAndBed(t *testing.T) {
	registry := assets.NewRegistry()
	// Registry 必须实现可选的 mesh.ModelReader：BuildRegistrySnapshot 靠类型
	// 断言接入，未实现则所有方块静默回落 tag 0，火把与床永远不出模型几何。
	var reader mesh.RegistryReader = registry
	if _, ok := reader.(mesh.ModelReader); !ok {
		t.Fatal("assets.Registry 未实现 mesh.ModelReader：模型 tag 无法进入快照")
	}
	for id := core.AirID; id < core.BlockIDMax; id++ {
		want := uint8(0)
		switch {
		case core.IsTorch(id):
			want = uint8(id - core.TorchStandingID + 1)
		case core.IsBed(id):
			want = 6
		}
		if got := registry.Model(id); got != want {
			t.Fatalf("Model(%d)=%d，想要 %d", id, got, want)
		}
	}
	for _, id := range []world.BlockID{core.BlockIDMax, world.BlockID(65535)} {
		if got := registry.Model(id); got != 0 {
			t.Fatalf("越界编号 %d 的 Model=%d，想要 0", id, got)
		}
	}
	snapshot := registry.MeshSnapshot()
	for id := core.AirID; id < core.BlockIDMax; id++ {
		want := uint8(0)
		switch {
		case core.IsTorch(id):
			want = uint8(id - core.TorchStandingID + 1)
		case core.IsBed(id):
			want = 6
		}
		if got := snapshot.Blocks[int(id)].Model; got != want {
			t.Fatalf("快照中方块 %d 的 Model=%d，想要 %d", id, got, want)
		}
	}
}

// TestWorkbenchUsesDedicatedWoodenLayers 锁定 spec Requirement「工作台方块与
// 打开生命周期」的呈现契约：顶/侧/底三面各用独立的原创程序化木质层，互不
// 共用、也不与既有橡木木板层复用；工作台是不发光的普通不透明立方体，朝
// 空气出面、被不透明邻居遮挡。
func TestWorkbenchUsesDedicatedWoodenLayers(t *testing.T) {
	registry := assets.NewRegistry()
	if got := registry.Material(core.WorkbenchID, mesh.FacePosY); got != assets.LayerWorkbenchTop {
		t.Fatalf("工作台顶面材质层 = %d，想要 %d", got, assets.LayerWorkbenchTop)
	}
	if got := registry.Material(core.WorkbenchID, mesh.FaceNegY); got != assets.LayerWorkbenchBottom {
		t.Fatalf("工作台底面材质层 = %d，想要 %d", got, assets.LayerWorkbenchBottom)
	}
	for _, face := range []mesh.Face{mesh.FaceNegX, mesh.FacePosX, mesh.FaceNegZ, mesh.FacePosZ} {
		if got := registry.Material(core.WorkbenchID, face); got != assets.LayerWorkbenchSide {
			t.Fatalf("工作台侧面（face %d）材质层 = %d，想要 %d", face, got, assets.LayerWorkbenchSide)
		}
	}

	// 「原创程序化」必须是三个互不相同的新层，不是彼此或橡木木板的别名：
	// 逐层比对全部字节，任何一层缺失或复用都会在这里变红。
	top := registry.LayerRGBA(int(assets.LayerWorkbenchTop))
	side := registry.LayerRGBA(int(assets.LayerWorkbenchSide))
	bottom := registry.LayerRGBA(int(assets.LayerWorkbenchBottom))
	planks := registry.LayerRGBA(int(assets.LayerOakPlanks))
	// 三层都不得落进植物 material 区间：Rust mesher 靠「material 在
	// [PlantMaterialFirst, PlantMaterialLast] 内」决定出交叉斜面而非轴向面，
	// 工作台是普通立方体，进区间就会被画成两片麦秆。
	for _, layer := range []uint16{assets.LayerWorkbenchTop, assets.LayerWorkbenchSide, assets.LayerWorkbenchBottom} {
		if mesh.PlantMaterial(layer) {
			t.Fatalf("工作台材质层 %d 落进了植物区间：会被渲染成交叉斜面", layer)
		}
	}
	for _, layer := range []struct {
		name   string
		pixels []byte
	}{
		{"顶面", top}, {"侧面", side}, {"底面", bottom},
	} {
		if len(layer.pixels) != 16*16*4 {
			t.Fatalf("工作台%s材质长度 = %d，想要 %d", layer.name, len(layer.pixels), 16*16*4)
		}
		for i := 3; i < len(layer.pixels); i += 4 {
			if layer.pixels[i] != 255 {
				t.Fatalf("工作台%s是不透明方块，像素 alpha 必须全为 255", layer.name)
			}
		}
	}
	if string(top) == string(side) || string(top) == string(bottom) || string(side) == string(bottom) {
		t.Fatal("工作台三层材质中存在复用：顶/侧/底必须各自独立")
	}
	if string(top) == string(planks) || string(side) == string(planks) || string(bottom) == string(planks) {
		t.Fatal("工作台材质层复用了橡木木板层")
	}

	if !registry.Opaque(core.WorkbenchID) {
		t.Fatal("工作台必须是不透明方块")
	}
	if got := registry.Emission(core.WorkbenchID); got != 0 {
		t.Fatalf("工作台 Emission = %d，想要 0", got)
	}
	if !registry.FaceVisible(core.WorkbenchID, core.AirID) {
		t.Fatal("工作台朝向空气的面消失了")
	}
	if registry.FaceVisible(core.WorkbenchID, core.StoneID) {
		t.Fatal("工作台朝向不透明邻居的面必须被遮挡")
	}
}

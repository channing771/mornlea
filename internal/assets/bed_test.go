package assets_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// allBedForms 列出床的八个稳定形态（床尾/床头 × 南西北东），与方块编号冻结
// 顺序一致。
func allBedForms() []core.BlockID {
	return []core.BlockID{
		core.BedFootSouthID, core.BedFootWestID, core.BedFootNorthID, core.BedFootEastID,
		core.BedHeadSouthID, core.BedHeadWestID, core.BedHeadNorthID, core.BedHeadEastID,
	}
}

// bedTopLayers 按形态顺序列出八个床面层的期望层号：床尾四向紧随 LayerTorch，
// 床头四向同序平移 4（与方块编号的床头 = 床尾 + 4 同形）。
func bedTopLayers() []uint16 {
	return []uint16{
		assets.LayerBedFootSouth, assets.LayerBedFootWest,
		assets.LayerBedFootNorth, assets.LayerBedFootEast,
		assets.LayerBedHeadSouth, assets.LayerBedHeadWest,
		assets.LayerBedHeadNorth, assets.LayerBedHeadEast,
	}
}

// TestBedFormsUseDedicatedTopLayers 锁定床的呈现契约一半：八个形态各占一张
// 原创程序化床面层（顶面专用），层号紧随 LayerTorch 连续冻结；四个侧面与底
// 面复用橡木木板层表达床架。床面层互不复用既有层，保证「原创橡木配色床面」
// 不会与其他方块串味。
func TestBedFormsUseDedicatedTopLayers(t *testing.T) {
	registry := assets.NewRegistry()
	layers := bedTopLayers()
	for i, id := range allBedForms() {
		want := assets.LayerTorch + 1 + uint16(i)
		if layers[i] != want {
			t.Fatalf("床面层 %d 的层号 = %d，想要紧随 LayerTorch 之后的 %d", i, layers[i], want)
		}
		if got := registry.Material(id, mesh.FacePosY); got != layers[i] {
			t.Fatalf("床形态 %d 顶面材质层 = %d，想要专属床面层 %d", id, got, layers[i])
		}
		for _, face := range []mesh.Face{mesh.FaceNegX, mesh.FacePosX, mesh.FaceNegZ, mesh.FacePosZ, mesh.FaceNegY} {
			if got := registry.Material(id, face); got != assets.LayerOakPlanks {
				t.Fatalf("床形态 %d 面 %d 材质层 = %d，想要床架用的橡木木板层", id, face, got)
			}
		}
	}
	// 层枚举的收尾段已按追加纪律接续：短草层（68）紧随床区间，裂纹区间
	// （LayerCrack0..9 = 69..78）再随短草之后；床与短草之间、短草与裂纹之间
	// 插层在此撞上断言；总层数的冻结值由裂纹层号冻结测试守护。
	if assets.LayerShortGrass != assets.LayerBedHeadEast+1 {
		t.Fatalf("LayerShortGrass = %d，不紧贴床区间上界 %d，插层检测失效", assets.LayerShortGrass, assets.LayerBedHeadEast+1)
	}
	if assets.LayerCrack0 != assets.LayerShortGrass+1 {
		t.Fatalf("LayerCrack0 = %d，不紧贴短草层上界 %d，插层检测失效", assets.LayerCrack0, assets.LayerShortGrass+1)
	}
	// 八张床面层都必须是完整尺寸的程序化像素（内嵌默认包不含床，程序化层
	// 即最终呈现）。
	for _, layer := range layers {
		px := registry.LayerRGBA(int(layer))
		if len(px) != 16*16*4 {
			t.Fatalf("床面层 %d 像素长度 = %d，想要 %d", layer, len(px), 16*16*4)
		}
	}
}

// TestBedFormsAreNotOpaqueAndStayFaceVisible 锁定床的呈现契约另一半：床是
// 9/16 短方块，不遮光也不遮挡邻面（与门、火把同一透明分类）；朝空气与非不
// 透明邻居出面，被不透明邻居遮挡。
func TestBedFormsAreNotOpaqueAndStayFaceVisible(t *testing.T) {
	registry := assets.NewRegistry()
	for _, id := range allBedForms() {
		if registry.Opaque(id) {
			t.Fatalf("床形态 %d 被判成不透明：9/16 短方块不得遮光或遮挡邻面", id)
		}
		if !registry.FaceVisible(id, core.AirID) {
			t.Fatalf("床形态 %d 朝空气的面不可见", id)
		}
		if !registry.FaceVisible(id, core.GlassID) {
			t.Fatalf("床形态 %d 朝玻璃（非不透明）的面被错误剔除", id)
		}
		if registry.FaceVisible(id, core.StoneID) {
			t.Fatalf("床形态 %d 朝不透明方块的面应当被遮挡", id)
		}
	}
}

// TestBedTexturePinsHeadDirectionEdge 锁定床面层的原创像素结构：床架包边、
// 床头朝向边的亮色带（床头层是枕头、床尾层是毯沿）落在朝向对应的边上。
// 顶面 UV 约定与 terrain.wgsl 的 face_uv 一致：u=world.z（列，+Z 列号大）、
// v=world.x（行，+X 行号大），因此南向床头亮带在末列、西向在首行、北向在
// 首列、东向在末行。四个朝向逐两不同、床尾与床头不同——多朝向床形态在
// 夜间场景的可辨性由这一层保证。
func TestBedTexturePinsHeadDirectionEdge(t *testing.T) {
	registry := assets.NewRegistry()

	// edgeBandLuma 返回某条边（0=首列、1=末列、2=首行、3=末行）内侧 2px 带
	// 的平均亮度，用于判定枕头/毯沿亮带落在哪条边。
	edgeBandLuma := func(px []byte, edge int) int {
		total, count := 0, 0
		for i := 2; i < 14; i++ {
			var x, y int
			switch edge {
			case 0:
				x, y = 2, i
			case 1:
				x, y = 13, i
			case 2:
				x, y = i, 2
			default:
				x, y = i, 13
			}
			p := pixel(px, x, y)
			total += int(p[0]) + int(p[1]) + int(p[2])
			count += 3
		}
		return total / count
	}
	// wantEdge 是各朝向的亮带期望边：南=末列、西=首行、北=首列、东=末行。
	wantEdges := map[string]int{"South": 1, "West": 2, "North": 0, "East": 3}
	dirs := []string{"South", "West", "North", "East"}
	for i, dir := range dirs {
		headPx := registry.LayerRGBA(int(bedTopLayers()[4+i]))
		footPx := registry.LayerRGBA(int(bedTopLayers()[i]))
		want := wantEdges[dir]
		for edge := 0; edge < 4; edge++ {
			brighter := edgeBandLuma(headPx, edge) >= edgeBandLuma(headPx, want)
			if edge != want && brighter {
				t.Fatalf("床头_%s 层的第 %d 号边亮度不低于床头边：亮带不在朝向边上", dir, edge)
			}
		}
		// 床头边的枕头（床头层）必须明显亮于毯沿（床尾层）：床尾/床头的
		// 半边身份由这条亮度差锁定，两层互换或丢层都会在此变红。
		if edgeBandLuma(footPx, want) >= edgeBandLuma(headPx, want) {
			t.Fatalf("床尾_%s 层的床头边没有毯沿亮带或枕头层丢失", dir)
		}
		// 床架包边：四角必须是深色包边（R 明显低于床垫基色）。
		for _, corner := range [][2]int{{0, 0}, {15, 0}, {0, 15}, {15, 15}} {
			p := pixel(footPx, corner[0], corner[1])
			if p[0] > 140 || p[2] > 90 {
				t.Fatalf("床尾_%s 层角点 %v 不是深色床架包边：%v", dir, corner, p)
			}
		}
	}
	// 四个朝向两两不同（亮带位置 + 噪声盐共同保证）。
	for _, half := range []struct {
		base int
		name string
	}{{0, "床尾"}, {4, "床头"}} {
		seen := map[[16 * 16 * 4]byte]bool{}
		for i := 0; i < 4; i++ {
			px := registry.LayerRGBA(int(bedTopLayers()[half.base+i]))
			var key [16 * 16 * 4]byte
			copy(key[:], px)
			if seen[key] {
				t.Fatalf("%s层的两个朝向纹理完全相同", half.name)
			}
			seen[key] = true
		}
	}
}

// TestBedFormsModelTagSix 锁定床的有限模型 tag：八个形态共用保留的 tag 6
// （单值床几何），随 registry 快照冻结送过 ABI 边界；越界编号保持默认 0。
// 火把 1..5 的其余映射由 TestTorchFormsModelTags 穷举守护。
func TestBedFormsModelTagSix(t *testing.T) {
	registry := assets.NewRegistry()
	for _, id := range allBedForms() {
		if got := registry.Model(id); got != 6 {
			t.Fatalf("Model(床形态 %d) = %d，想要保留的床 tag 6", id, got)
		}
	}
	snapshot := registry.MeshSnapshot()
	for _, id := range allBedForms() {
		entry := snapshot.Blocks[int(id)]
		if entry.Model != 6 {
			t.Fatalf("快照中方块 %d 的 Model = %d，想要 6", id, entry.Model)
		}
	}
	for _, id := range []world.BlockID{core.BlockIDMax, world.BlockID(65535)} {
		if got := registry.Model(id); got != 0 {
			t.Fatalf("越界编号 %d 的 Model = %d，想要默认 0", id, got)
		}
	}
}

// bedMeshNeighborhood 复刻 mesh 包 fullyLoadedAirNeighborhood 的形状：3×3×3
// 全空气邻域 + 中心区段、高度表置为存在，让光照走真实天空光路径。床用例
// 无法复用 mesh 内部夹具（包不可导入），在此按同构重建。
func bedMeshNeighborhood() *world.Neighborhood {
	n := &world.Neighborhood{Center: world.NewSection(), SectionY: 8}
	for dx := range n.Around {
		for dy := range n.Around[dx] {
			for dz := range n.Around[dx][dy] {
				n.Around[dx][dy][dz] = world.NewSection()
			}
		}
	}
	for dx := range n.HeightsPresent {
		for dz := range n.HeightsPresent[dx] {
			n.HeightsPresent[dx][dz] = true
		}
	}
	return n
}

// TestBedSurfaceLayerReachesMesherThroughProductionRegistry 穿透生产链锁定
// 材质缝：生产注册表 assets.NewRegistry() 直接喂 mesh.MeshSection，床的平顶
// quad 必须携带该形态专属床面层、四片侧板落在橡木木板层（床架）——face →
// 层的路由若在生产注册表与 Rust 发射端之间断链（例如五条 quad 被糊成同一
// 张床架层图），八个朝向形态会整体退成纯木板色，本用例立即变红。夹具的路
// 由断言天然非同质：床面层与橡木木板层是两张不同的像素图。
func TestBedSurfaceLayerReachesMesherThroughProductionRegistry(t *testing.T) {
	registry := assets.NewRegistry()
	for _, tc := range []struct {
		name    string
		id      core.BlockID
		wantTop uint16
	}{
		{"床尾_南", core.BedFootSouthID, assets.LayerBedFootSouth},
		{"床头_东", core.BedHeadEastID, assets.LayerBedHeadEast},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := bedMeshNeighborhood()
			n.Center.Blocks.Set(8, 8, 8, tc.id)
			quads := mesh.MeshSection(n, registry, mesh.NewLightScratch())
			top, sides := 0, 0
			for _, quad := range quads {
				if quad.X != 8 || quad.Y != 8 || quad.Z != 8 {
					t.Fatalf("床格漏出格外 quad：face=%v mat=%d 坐标=(%d,%d,%d)",
						quad.Face, quad.Mat, quad.X, quad.Y, quad.Z)
				}
				switch {
				case quad.Face == mesh.FacePosY:
					if quad.Mat != tc.wantTop {
						t.Fatalf("床顶面材质 = %d，想要专属床面层 %d：材质路由断链，床会退成纯木板色", quad.Mat, tc.wantTop)
					}
					if quad.Corners != [4]uint8{8, 8, 8, 8} {
						t.Fatalf("床顶面角高度 = %v，想要 9/16（原值 8）", quad.Corners)
					}
					top++
				case quad.Mat == assets.LayerOakPlanks:
					sides++
				default:
					t.Fatalf("床格出现意外材质 %d（face %v）：床面/床架分层被破坏", quad.Mat, quad.Face)
				}
			}
			if top != 1 || sides != 4 {
				t.Fatalf("床顶面 %d 条 / 侧板 %d 条，想要 1+4", top, sides)
			}
		})
	}
}

// TestBedLayerNumbersMatchClientShaderContract 是床 material 区间在 Go 侧的
// 机械守卫：八张床面层的层号被 Rust 客户端的 BED_MATERIAL_FIRST/LAST 常量与
// terrain.wgsl 的 bed_material 字面量各硬编码一份（三方没有共享定义，只能
// 人手同步）。床是角高度短方块（9/16 半高板），客户端按 material 区间把床
// quad 分流到角高度解码路径；在层枚举的床**之前**插层会整体平移区间，床
// quad 会脱门走 w/h 尺寸解码、摊成盖住邻格的巨型石板——本条与 Rust 侧
// farmland_tests.rs 的 bed_range_constants_match_go_layer_enum 及其床渲染
// 回归是仅有的报警点。反引号纪律与 TestFarmlandLayerNumbersMatchClientShaderContract
// 同款：wgsl 函数与 Rust 测试名不是 Go 声明，一律纯文本提及。
func TestBedLayerNumbersMatchClientShaderContract(t *testing.T) {
	// 字面量与 Rust 客户端 shaders.rs 的两个常量逐一对应，改任何一侧必须同步。
	if want := uint16(60); assets.LayerBedFootSouth != want {
		t.Fatalf("客户端床区间下界应为 %d（Rust 客户端 BED_MATERIAL_FIRST），实测 LayerBedFootSouth=%d", want, assets.LayerBedFootSouth)
	}
	if want := uint16(67); assets.LayerBedHeadEast != want {
		t.Fatalf("客户端床区间上界应为 %d（Rust 客户端 BED_MATERIAL_LAST），实测 LayerBedHeadEast=%d", want, assets.LayerBedHeadEast)
	}
	// 区间与两侧邻居紧贴：火把层在前、短草作为单一新层紧随床区间、裂纹区间
	// （LayerCrack0..9）再随短草之后，插层必然撞上断言。
	if assets.LayerTorch != assets.LayerBedFootSouth-1 {
		t.Fatalf("LayerTorch=%d 不紧贴床区间下界，插层检测失效", assets.LayerTorch)
	}
	if assets.LayerShortGrass != assets.LayerBedHeadEast+1 {
		t.Fatalf("LayerShortGrass=%d 未紧随床区间上界 %d", assets.LayerShortGrass, assets.LayerBedHeadEast)
	}
	registry := assets.NewRegistry()
	if assets.LayerCrack0 != assets.LayerShortGrass+1 {
		t.Fatalf("LayerCrack0=%d 不紧贴短草层上界 %d，插层检测失效", assets.LayerCrack0, assets.LayerShortGrass+1)
	}

	// 游戏编号 → 材质层的映射是客户端判别的实际输入：八个床形态的顶面
	// material 都必须落在本区间内，否则 shader 不走角高度路径。
	for _, id := range allBedForms() {
		mat := registry.Material(id, mesh.FacePosY)
		if mat < assets.LayerBedFootSouth || mat > assets.LayerBedHeadEast {
			t.Fatalf("床形态 %d 的顶面 material=%d 落在客户端床区间 [60,67] 之外", id, mat)
		}
	}
}

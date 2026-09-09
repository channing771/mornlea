package assets

import (
	"bytes"
	"image/color"
	"testing"
	"testing/fstest"

	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// expectedSaplingLayer 是树苗材质层的冻结层号：只追加在雪层之后（= 164），
// `layerCount` 随之到 165。同一个数值被 `packages/client/mesh` 的植物材质常量
// 与 Rust `quad.rs` 的植物集合各复述一份，改号必须三处同批同步。
const expectedSaplingLayer uint16 = 164

// TestSaplingUsesAppendedProceduralCutoutLayer 锁定树苗的呈现契约：材质层只追加
// 在雪层之后（层号 164、`layerCount` 165）、六面共用同一 cutout 层、非不透明、
// 一个轴向面都不出、走 cutout mip 路径、程序化纹理是 16×16 二值 alpha 且非空。
//
// 树苗与短草同形（植物格由 Rust mesher 另行补出四条交叉斜面），但它是独立的
// 方块与物品，因此层号、材质映射与快照条目都必须逐项钉住——层号是 Rust 侧
// 植物集合的第三份副本，重排任何既有层号都会让两侧同时错认方块。
func TestSaplingUsesAppendedProceduralCutoutLayer(t *testing.T) {
	registry := NewRegistry()
	if LayerSapling != expectedSaplingLayer {
		t.Fatalf("LayerSapling = %d，想要冻结值 %d", LayerSapling, expectedSaplingLayer)
	}
	if LayerSapling != LayerSnowLayerSide+1 {
		t.Fatalf("树苗层 %d 必须紧随雪层侧层 %d 之后追加", LayerSapling, LayerSnowLayerSide)
	}
	if got, want := registry.LayerCount(), int(expectedSaplingLayer)+1; got != want {
		t.Fatalf("LayerCount = %d，想要 %d", got, want)
	}
	// 只追加：既有冻结层号一个都不能动。植物区间、门、火把、床、短草、裂纹、
	// 牛、物品与人物层的首项全部保持原值，任何重排都会在这里变红。
	if LayerWheat0 != 31 || LayerCarrot7 != 54 || LayerDoor != 55 || LayerTorch != 59 ||
		LayerBedFootSouth != 60 || LayerBedHeadEast != 67 || LayerShortGrass != 68 ||
		LayerCrack0 != 69 || LayerCrack9 != 78 || LayerCowHide != 79 ||
		LayerItemCoal != 83 || LayerItemWaterBucket != 113 || LayerHumanSageHead != 114 {
		t.Fatalf("追加树苗层移动了既有层号：wheat0=%d carrot7=%d door=%d torch=%d "+
			"bed=%d..%d shortGrass=%d crack=%d..%d cow=%d itemCoal=%d itemWater=%d human=%d",
			LayerWheat0, LayerCarrot7, LayerDoor, LayerTorch, LayerBedFootSouth,
			LayerBedHeadEast, LayerShortGrass, LayerCrack0, LayerCrack9, LayerCowHide,
			LayerItemCoal, LayerItemWaterBucket, LayerHumanSageHead)
	}

	// 材质映射：六个面共用同一层（交叉斜面没有朝向可言，Rust mesher 正是靠
	// 「六个面的 material 都落在植物集合」认出植物格）。
	for face := mesh.Face(0); face < 6; face++ {
		if got := registry.Material(core.SaplingID, face); got != expectedSaplingLayer {
			t.Fatalf("树苗 face %d material = %d，想要 %d", face, got, expectedSaplingLayer)
		}
	}
	// 反向守卫：任何非树苗方块都不得落到树苗层。
	for id := core.AirID; id < core.BlockIDMax; id++ {
		if core.IsSapling(id) {
			continue
		}
		for face := mesh.Face(0); face < 6; face++ {
			if got := registry.Material(id, face); got == expectedSaplingLayer {
				t.Fatalf("非树苗方块 %d 的 face %d 落到了树苗材质层", id, face)
			}
		}
	}

	// 树苗是零碰撞、非不透明、不发光的植物：轴向面一条都不出，其余方块朝它
	// 的面仍然可见（判定落到 `Opaque(id)` 的反向分支）。
	for _, adjacent := range []core.BlockID{
		core.AirID, core.GlassID, core.WaterSourceID, core.StoneID,
		core.ShortGrassID, core.SaplingID,
	} {
		if registry.FaceVisible(core.SaplingID, adjacent) {
			t.Fatalf("树苗朝相邻方块 %d 产生了轴向面", adjacent)
		}
	}
	if !registry.FaceVisible(core.StoneID, core.SaplingID) {
		t.Fatal("石头朝向树苗的面消失了：树苗不得遮挡邻居出面")
	}
	if registry.Opaque(core.SaplingID) {
		t.Fatal("树苗不得作为完整遮光方块")
	}
	if got := registry.Emission(core.SaplingID); got != 0 {
		t.Fatalf("树苗 Emission = %d，想要 0", got)
	}
	if got := registry.LightAttenuation(core.SaplingID); got != 0 {
		t.Fatalf("树苗 LightAttenuation = %d，想要 0", got)
	}
	if got := registry.FluidHeight(core.SaplingID); got != 0 {
		t.Fatalf("树苗 FluidHeight = %d，想要非流体哨兵 0", got)
	}
	if got := registry.BlockTopRaw(core.SaplingID); got != 0 {
		t.Fatalf("树苗 BlockTopRaw = %d，想要满格哨兵 0", got)
	}
	if got := registry.Model(core.SaplingID); got != 0 {
		t.Fatalf("树苗 Model = %d，想要默认 0（交叉斜面走植物路径）", got)
	}
	if !isCutoutLayer(int(expectedSaplingLayer)) {
		t.Fatal("树苗层未进入 cutout mip 路径")
	}

	pixels := registry.LayerRGBA(int(expectedSaplingLayer))
	if len(pixels) != 16*16*4 {
		t.Fatalf("树苗程序化纹理长度 = %d，想要 %d", len(pixels), 16*16*4)
	}
	opaque, transparent := 0, 0
	for index := 3; index < len(pixels); index += 4 {
		switch pixels[index] {
		case 0:
			transparent++
		case 255:
			opaque++
		default:
			t.Fatalf("树苗 alpha[%d] = %d，想要 0 或 255", index/4, pixels[index])
		}
	}
	if opaque == 0 || transparent == 0 {
		t.Fatalf("树苗程序化纹理不完整：不透明像素=%d，透明像素=%d", opaque, transparent)
	}
	if bytes.Equal(pixels, registry.LayerRGBA(int(LayerShortGrass))) {
		t.Fatal("树苗纹理与短草层逐像素相同：独立层号必须各有像素")
	}

	// 快照自动跟随：树苗必须逐条进入 mesh registry 快照（Rust 只消费快照，
	// 漏登记等于树苗永远不出面）。
	snapshot := registry.MeshSnapshot()
	if got, want := len(snapshot.Blocks), int(core.BlockIDMax); got != want {
		t.Fatalf("mesh registry 条目 = %d，想要覆盖全部已注册方块的 %d", got, want)
	}
	block := snapshot.Blocks[int(core.SaplingID)]
	if block.ID != core.SaplingID || block.Opaque || block.Emission != 0 ||
		block.FluidHeight != 0 || block.LightAttenuation != 0 ||
		block.BlockTopRaw != 0 || block.Model != 0 {
		t.Fatalf("树苗的快照字段 = %+v", block)
	}
	for face := 0; face < 6; face++ {
		if block.Materials[face] != expectedSaplingLayer {
			t.Fatalf("快照中树苗 face %d 的 material = %d，想要 %d",
				face, block.Materials[face], expectedSaplingLayer)
		}
	}
	if snapshot.FaceVisible(core.SaplingID, core.AirID) {
		t.Fatal("快照中树苗朝向空气的面可见：轴向面必须一条都不出")
	}
}

// TestSaplingLayerStaysInternalToTexturePacks 锁定树苗层是 `LayerItemCoal` 之后的
// 原创内部层：不开放材质包文件覆盖（无绑定槽位，与雪层、人物层同口径），用户包
// 提供同名文件也不得改变该层像素。
func TestSaplingLayerStaysInternalToTexturePacks(t *testing.T) {
	for _, binding := range textureBindings {
		if binding.layer == expectedSaplingLayer {
			t.Fatalf("树苗层 %d 被开放为材质包槽位 %q", expectedSaplingLayer, binding.name)
		}
	}
	registry := NewRegistry()
	before := bytes.Clone(registry.LayerRGBA(int(expectedSaplingLayer)))
	_, encoded := solidPNG(t, 16, 16, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	if err := applyPack(registry, fstest.MapFS{
		"pack.json":            {Data: manifest(t, "树苗覆盖测试包")},
		"textures/sapling.png": {Data: encoded},
	}); err != nil {
		t.Fatalf("applyPack() error = %v", err)
	}
	if got := registry.LayerRGBA(int(expectedSaplingLayer)); !bytes.Equal(got, before) {
		t.Fatal("用户 textures/sapling.png 覆盖了内部树苗层")
	}
}

// TestSaplingItemIconReusesBlockMaterialLayer 锁定树苗物品图标复用方块材质层：
// 不新增 `LayerItem*` 常量、不给 `ItemIconLayer` 加分支，图标仍由 `blockItemTexture`
// 从树苗材质层合成（与可放置方块同一条回退路径）。
func TestSaplingItemIconReusesBlockMaterialLayer(t *testing.T) {
	if layer, outlined := ItemIconLayer(core.ItemSapling); outlined {
		t.Fatalf("树苗物品新增了独立图标层 %d，必须复用方块材质层", layer)
	}
	registry := NewRegistry()
	icon, ok := registry.ItemIconRGBA(core.ItemSapling)
	if !ok || len(icon) != 16*16*4 {
		t.Fatalf("树苗物品图标缺失：ok=%v 长度=%d", ok, len(icon))
	}
	sapling := registry.LayerRGBA(int(expectedSaplingLayer))
	if want := blockItemTexture(sapling, sapling); !bytes.Equal(icon, want) {
		t.Fatal("树苗图标不是方块材质层的等距立方体合成")
	}
	opaque := 0
	for index := 3; index < len(icon); index += 4 {
		if icon[index] != 0 {
			opaque++
		}
	}
	if opaque == 0 {
		t.Fatal("树苗图标全透明")
	}
}

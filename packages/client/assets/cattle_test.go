package assets

import (
	"bytes"
	"image/color"
	"testing"
	"testing/fstest"
)

// TestCattleLayersAreAppendedAfterCrack 锁定牛四层的层号追加纪律：牛皮、牛头、
// 生牛肉、熟牛肉只能追加在既有枚举末位（裂纹之后），植物 31..54、火把、床、
// 短草与裂纹的冻结层号一律不动。
func TestCattleLayersAreAppendedAfterCrack(t *testing.T) {
	if got := int(LayerCowHide); got != 79 {
		t.Fatalf("LayerCowHide=%d，想要追加值 79", got)
	}
	if got := int(LayerCowHead); got != 80 {
		t.Fatalf("LayerCowHead=%d，想要追加值 80", got)
	}
	if got := int(LayerRawBeef); got != 81 {
		t.Fatalf("LayerRawBeef=%d，想要追加值 81", got)
	}
	if got := int(LayerCookedBeef); got != 82 {
		t.Fatalf("LayerCookedBeef=%d，想要追加值 82", got)
	}
	if got := int(layerCount); got != 83 {
		t.Fatalf("layerCount=%d，想要 83", got)
	}
	if LayerCowHide != LayerCrack9+1 {
		t.Fatalf("LayerCowHide=%d 不紧贴裂纹区间上界 %d，插层检测失效", LayerCowHide, LayerCrack9+1)
	}
	// 植物区间逐端点钉死：中间插入任何一层都会整体平移，Rust 会把作物当成
	// 普通方块。
	if got := int(LayerWheat0); got != 31 {
		t.Fatalf("LayerWheat0=%d，想要冻结值 31", got)
	}
	if got := int(LayerCarrot7); got != 54 {
		t.Fatalf("LayerCarrot7=%d，想要冻结值 54", got)
	}
	if got := int(LayerCrack9); got != 78 {
		t.Fatalf("LayerCrack9=%d，想要冻结值 78", got)
	}
}

// TestCattleLayersCutoutClassification 锁定牛四层的透明分类：牛皮与牛头是牛身
// 六面的不透明采样层，不得进 cutout 集合；生熟牛肉是掉落物与图标用的镂空
// 图标，进 cutout 集合走保覆盖率 mip 路径。
func TestCattleLayersCutoutClassification(t *testing.T) {
	for _, layer := range []uint16{LayerCowHide, LayerCowHead} {
		if isCutoutLayer(int(layer)) {
			t.Fatalf("牛身层 %d 进了 cutout 集合：身体面会被 discard 啃出破洞", layer)
		}
	}
	for _, layer := range []uint16{LayerRawBeef, LayerCookedBeef} {
		if !isCutoutLayer(int(layer)) {
			t.Fatalf("牛肉层 %d 未进 cutout 集合：稀疏图标在 mip 链上会被平均到消失", layer)
		}
	}
	if isCutoutLayer(int(LayerBedHeadEast)) {
		t.Fatalf("床层 %d 不应进 cutout 集合", int(LayerBedHeadEast))
	}
	if !isCutoutLayer(int(LayerCrack9)) {
		t.Fatalf("裂纹末层 %d 掉出了 cutout 集合", int(LayerCrack9))
	}
}

// TestCattleProceduralFallbackPixels 锁定四层程序化回退的像素契约：牛身两层
// 全图不透明且非纯色（斑点/噪点/明暗）；牛肉两层是二值 alpha 的镂空图标且
// 生偏红、熟偏棕、两层互不相同。`applyPack` 缺文件时这些像素即最终呈现。
func TestCattleProceduralFallbackPixels(t *testing.T) {
	r := NewRegistry()
	for _, layer := range []uint16{LayerCowHide, LayerCowHead, LayerRawBeef, LayerCookedBeef} {
		px := r.LayerRGBA(int(layer))
		if len(px) != texSize*texSize*4 {
			t.Fatalf("层 %d 像素长度=%d，想要 %d", layer, len(px), texSize*texSize*4)
		}
	}
	for _, layer := range []uint16{LayerCowHide, LayerCowHead} {
		px := r.LayerRGBA(int(layer))
		for i := 3; i < len(px); i += 4 {
			if px[i] != 255 {
				t.Fatalf("牛身层 %d 的像素 %d alpha=%d，想要全图不透明 255", layer, i/4, px[i])
			}
		}
	}
	for _, layer := range []uint16{LayerRawBeef, LayerCookedBeef} {
		px := r.LayerRGBA(int(layer))
		opaque, transparent := 0, 0
		for i := 3; i < len(px); i += 4 {
			switch px[i] {
			case 0:
				transparent++
			case 255:
				opaque++
			default:
				t.Fatalf("牛肉层 %d 的像素 %d alpha=%d，cutout 只允许 0/255", layer, i/4, px[i])
			}
		}
		if opaque == 0 || transparent == 0 {
			t.Fatalf("牛肉层 %d 不同时包含透明(%d)与不透明(%d)像素", layer, transparent, opaque)
		}
	}
	hide := r.LayerRGBA(int(LayerCowHide))
	head := r.LayerRGBA(int(LayerCowHead))
	if bytes.Equal(hide, head) {
		t.Fatal("牛皮与牛头逐像素相同：两层必须可辨")
	}
	raw := r.LayerRGBA(int(LayerRawBeef))
	cooked := r.LayerRGBA(int(LayerCookedBeef))
	if bytes.Equal(raw, cooked) {
		t.Fatal("生熟牛肉逐像素相同：两层必须可辨")
	}
	if got := opaqueAverage(raw); got[0] < got[2]+40 {
		t.Fatalf("生牛肉平均色 R=%d B=%d，想要偏红（R-B>=40）", got[0], got[2])
	}
	if got, want := opaqueAverage(cooked), opaqueAverage(raw); got[0]-got[2] >= want[0]-want[2] {
		t.Fatalf("熟牛肉 R-B=%d 未低于生牛肉 R-B=%d：熟肉应偏棕", got[0]-got[2], want[0]-want[2])
	}
	// 四层都不得与任何既有层复用。
	for _, layer := range []uint16{LayerCowHide, LayerCowHead, LayerRawBeef, LayerCookedBeef} {
		px := r.LayerRGBA(int(layer))
		for other := 0; other < int(LayerCowHide); other++ {
			if bytes.Equal(px, r.LayerRGBA(other)) {
				t.Fatalf("层 %d 与既有第 %d 层逐像素相同", layer, other)
			}
		}
	}
}

// opaqueAverage 返回一层材质不透明像素的平均 RGB，用于比较色相。
func opaqueAverage(px []byte) [3]int {
	var sum [3]int
	count := 0
	for i := 0; i < len(px); i += 4 {
		if px[i+3] == 0 {
			continue
		}
		sum[0] += int(px[i])
		sum[1] += int(px[i+1])
		sum[2] += int(px[i+2])
		count++
	}
	if count == 0 {
		return sum
	}
	for i := range sum {
		sum[i] /= count
	}
	return sum
}

// TestCattleBindingsResolveToDedicatedLayers 锁定四个覆盖槽位：材质包可按槽位
// 覆盖四层（镜像火把与床的仅覆盖槽位语义），名字或层号错位会让覆盖文件静默
// 落到别的层上。
func TestCattleBindingsResolveToDedicatedLayers(t *testing.T) {
	byName := make(map[string]uint16, len(textureBindings))
	for _, binding := range textureBindings {
		byName[binding.name] = binding.layer
	}
	for _, tt := range []struct {
		name  string
		layer uint16
	}{
		{"cow_hide", LayerCowHide},
		{"cow_head", LayerCowHead},
		{"raw_beef", LayerRawBeef},
		{"cooked_beef", LayerCookedBeef},
	} {
		layer, ok := byName[tt.name]
		if !ok {
			t.Fatalf("textureBindings 缺少槽位 %q", tt.name)
		}
		if layer != tt.layer {
			t.Fatalf("槽位 %q 解析到层 %d，想要 %d", tt.name, layer, tt.layer)
		}
	}
}

// TestCattlePackOverrideKeepsProceduralFallback 锁定程序化回退语义：用户包未
// 提供文件时四层保留程序化像素，提供同名文件时按槽位覆盖且不影响其他层。
func TestCattlePackOverrideKeepsProceduralFallback(t *testing.T) {
	procedural := NewRegistry()
	before := snapshotLayers(procedural)
	if err := applyPack(procedural, fstest.MapFS{
		"pack.json": {Data: manifest(t, "空牛包")},
	}); err != nil {
		t.Fatalf("applyPack() error = %v", err)
	}
	assertLayersEqual(t, procedural, before)

	registry := NewRegistry()
	_, encoded := solidPNG(t, 16, 16, color.NRGBA{R: 200, G: 170, B: 150, A: 255})
	root := fstest.MapFS{
		"pack.json":                {Data: manifest(t, "牛覆盖包")},
		"textures/cow_hide.png":    {Data: encoded},
		"textures/raw_beef.png":    {Data: encoded},
		"textures/cooked_beef.png": {Data: encoded},
	}
	if err := applyPack(registry, root); err != nil {
		t.Fatalf("applyPack() error = %v", err)
	}
	for _, layer := range []uint16{LayerCowHide, LayerRawBeef, LayerCookedBeef} {
		if got := registry.LayerRGBA(int(layer)); len(got) == 0 || got[0] != 200 {
			t.Fatalf("层 %d 未被用户包覆盖", layer)
		}
	}
	if got, want := registry.LayerRGBA(int(LayerCowHead)), before[LayerCowHead]; !bytes.Equal(got, want) {
		t.Fatal("未覆盖的牛头层被顺带修改")
	}
}

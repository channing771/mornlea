//go:build darwin

package assets

import (
	"testing"
)

func TestDownsampleHalvesSizeAndAveragesRGBA(t *testing.T) {
	src := []byte{
		10, 20, 30, 40, 30, 40, 50, 60,
		50, 60, 70, 80, 70, 80, 90, 100,
	}
	got := downsample(src, 2)
	want := []byte{40, 50, 60, 70}
	if len(got) != len(want) {
		t.Fatalf("2×2 降采样长度 = %d，想要 %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("通道 %d = %d，想要 %d", i, got[i], want[i])
		}
	}
}

func TestDownsampleCutoutPreservesCoverageAndRGBMean(t *testing.T) {
	src := []byte{
		10, 20, 30, 255, 30, 40, 50, 0,
		50, 60, 70, 0, 70, 80, 90, 0,
	}
	if got := downsample(src, 2); got[3] != 63 {
		t.Fatalf("普通降采样 alpha = %d，想要 63", got[3])
	}
	got := downsampleCutout(src, 2)
	want := []byte{40, 50, 60, 255}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cutout 通道 %d = %d，想要 %d", i, got[i], want[i])
		}
	}

	transparent := downsampleCutout(make([]byte, 2*2*4), 2)
	if transparent[3] != 0 {
		t.Fatalf("全透明 cutout 降采样 alpha = %d，想要 0", transparent[3])
	}
}

// TestWheatLayersTakeTheCutoutMipPath 是**位置性**断言：植物层（小麦/马铃薯/胡萝卜共 24 层）必须真的走
// downsampleCutout 那条分支，而不是仅仅"存在一条 mip 链"。
//
// 只断言链长或最后一层非空是恒真的——两条分支都给得出。这里比的是同一层在两条
// 分支下的产物：麦秆细到 1 像素，盒式降采样把 alpha 平均到 64 上下，一过
// `c.a < 0.5` 的 discard 远处整片作物就消失了，而保覆盖率降采样给 255。
func TestWheatLayersTakeTheCutoutMipPath(t *testing.T) {
	r := NewRegistry()
	for layer := LayerWheat0; layer <= LayerCarrot7; layer++ {
		chain := r.layerMipChain(int(layer))
		if len(chain) != atlasMips {
			t.Fatalf("小麦层 %d 的 mip 链长度 = %d，想要 %d", layer, len(chain), atlasMips)
		}
		src := r.LayerRGBA(int(layer))
		wantCutout := downsampleCutout(src, texSize)
		wantPlain := downsample(src, texSize)
		if string(chain[1]) != string(wantCutout) {
			t.Fatalf("小麦层 %d 的 mip 1 不是 downsampleCutout 的产物", layer)
		}
		// 夹具承重守卫排在真实断言之后：两条分支必须真的不同，否则上面那条
		// 断言在"随便哪条分支"下都成立。
		if string(wantCutout) == string(wantPlain) {
			t.Fatalf("小麦层 %d 的两条降采样分支产物相同，位置性断言退化为恒真", layer)
		}
	}
}

func TestCutoutMipChainKeepsOpaqueCoverage(t *testing.T) {
	r := NewRegistry()
	layers := []uint16{LayerLeaves, LayerGlass}
	for layer := LayerWheat0; layer <= LayerCarrot7; layer++ {
		layers = append(layers, layer)
	}
	for _, layer := range layers {
		px, size := r.LayerRGBA(int(layer)), texSize
		for size > 1 {
			px = downsampleCutout(px, size)
			size /= 2
		}
		if px[3] != 255 {
			t.Fatalf("cutout 层 %d 的 1×1 mip alpha = %d，想要 255", layer, px[3])
		}
	}
}

// TestAtlasPixelsScalesWithLayerCount 钉住 atlas 导出与层枚举的同步：层数必须
// 等于 layerCount（短草与裂纹层追加后即 79），总字节数必须等于层数 × 每层完整 mip
// 链字节数（逐 mip 尺寸从 atlasMips/texSize 推导，不复制魔法数）。两侧任何
// 一方脱钩（导出漏层、层枚举与上传长度不一致）都会让 Rust 侧 upload_atlas
// 的长度校验或纹理内容直接出错。
func TestAtlasPixelsScalesWithLayerCount(t *testing.T) {
	r := NewRegistry()
	layers, pixels := r.AtlasPixels()
	if layers != int(layerCount) {
		t.Fatalf("AtlasPixels 层数 = %d，想要 layerCount = %d", layers, int(layerCount))
	}
	perLayer := 0
	for mip := 0; mip < atlasMips; mip++ {
		size := texSize >> mip
		perLayer += size * size * 4
	}
	if got, want := len(pixels), layers*perLayer; got != want {
		t.Fatalf("AtlasPixels 字节数 = %d，想要 %d（%d 层 × 每层 mip 链 %d 字节）", got, want, layers, perLayer)
	}
}

// TestCrackLayersTakeTheCutoutMipPath 镜像 TestWheatLayersTakeTheCutoutMipPath
// 的**位置性**断言：裂纹 10 层必须真的走 downsampleCutout 那条分支。裂纹像素
// 比麦秆更稀疏，普通平均降采样会把 alpha 一路稀释，远处整条裂纹从 mip 链上
// 消失。对照组用不透明的石头层钉住另一条分支：固体层必须仍走平均降采样，
// 裂纹的 cutout 判据不得越界把固体层拖进来。
func TestCrackLayersTakeTheCutoutMipPath(t *testing.T) {
	r := NewRegistry()
	for layer := LayerCrack0; layer <= LayerCrack9; layer++ {
		chain := r.layerMipChain(int(layer))
		if len(chain) != atlasMips {
			t.Fatalf("裂纹层 %d 的 mip 链长度 = %d，想要 %d", layer, len(chain), atlasMips)
		}
		src := r.LayerRGBA(int(layer))
		wantCutout := downsampleCutout(src, texSize)
		wantPlain := downsample(src, texSize)
		if string(chain[1]) != string(wantCutout) {
			t.Fatalf("裂纹层 %d 的 mip 1 不是 downsampleCutout 的产物", layer)
		}
		// 夹具承重守卫排在真实断言之后：两条分支必须真的不同，否则上面那条
		// 断言在"随便哪条分支"下都成立。
		if string(wantCutout) == string(wantPlain) {
			t.Fatalf("裂纹层 %d 的两条降采样分支产物相同，位置性断言退化为恒真", layer)
		}
	}
	// 对照组：石头层（不透明固体层）的 mip 1 必须是普通平均降采样的产物。
	if got, want := r.layerMipChain(int(LayerStone))[1], downsample(r.LayerRGBA(int(LayerStone)), texSize); string(got) != string(want) {
		t.Fatal("石头层的 mip 1 不是普通平均降采样的产物：cutout 判据越过了裂纹区间")
	}
}

func TestDownsampleMipChainEndsAtOnePixel(t *testing.T) {
	px := make([]byte, 16*16*4)
	size := 16
	for size > 1 {
		px = downsample(px, size)
		size /= 2
		if len(px) != size*size*4 {
			t.Fatalf("mip %dx%d 长度 = %d", size, size, len(px))
		}
	}
}

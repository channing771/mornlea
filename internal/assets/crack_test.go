package assets

import (
	"fmt"
	"testing"
)

// TestCrackLayerNumbersAreFrozen 是裂纹 10 层的层号冻结守卫。
//
// 层数值真值源是本文件 blocks.go 的层枚举，但裂纹实例以 f32 层号直接索引
// atlas，呈现层从 LayerCrack0 派生各阶段层号——层号一旦平移，裂纹会被采样到
// 别的材质层上。本条钉住 LayerCrack0..LayerCrack9 = 68..77、layerCount = 78，
// 且裂纹区间紧贴床区间上界：在床与裂纹之间插层必然撞上断言。植物 31..54、
// 耕地 29/30、火把 59、床 60..67 各区间另有专属守卫（farmland/plant/torch/bed
// 的既有测试），此处不重复。
func TestCrackLayerNumbersAreFrozen(t *testing.T) {
	if got := int(LayerCrack0); got != 68 {
		t.Fatalf("LayerCrack0=%d，想要冻结值 68", got)
	}
	if got := int(LayerCrack9); got != 77 {
		t.Fatalf("LayerCrack9=%d，想要冻结值 77", got)
	}
	if got := int(layerCount); got != 78 {
		t.Fatalf("layerCount=%d，想要 78", got)
	}
	if LayerCrack0 != LayerBedHeadEast+1 {
		t.Fatalf("LayerCrack0=%d 不紧贴床区间上界 %d，插层检测失效", LayerCrack0, LayerBedHeadEast+1)
	}
	for stage := 0; stage < crackStageCount; stage++ {
		if got := int(LayerCrack0) + stage; got != 68+stage {
			t.Fatalf("裂纹层 %d 的层号=%d，想要连续冻结值 %d", stage, got, 68+stage)
		}
	}
}

// TestCrackTextureIsDeterministic 锁定裂纹纹理的确定性：同阶段重复生成必须
// 逐字节一致——裂纹像素只来自固定种子（hash2 的既定噪声习惯），不引入
// math/rand 全局源或时间，两份注册表、两次构建之间不允许漂移。
func TestCrackTextureIsDeterministic(t *testing.T) {
	for stage := 0; stage < crackStageCount; stage++ {
		first := crackTexture(stage)
		second := crackTexture(stage)
		if len(first) != texSize*texSize*4 {
			t.Fatalf("阶段 %d 裂纹纹理长度=%d，想要 %d", stage, len(first), texSize*texSize*4)
		}
		for i := range first {
			if first[i] != second[i] {
				t.Fatalf("阶段 %d 第 %d 字节两次生成不一致：%d vs %d", stage, i, first[i], second[i])
			}
		}
	}
}

// TestCrackTextureGrowsIncrementally 锁定裂纹随阶段**增量生长**：阶段 i 的
// 非透明像素集合必须包含阶段 i-1 的全部像素（裂纹只延伸、不移动、不消失），
// 每个阶段都非空，且阶段 0（中心初始裂点）的覆盖严格小于阶段 9（大面积
// 破裂）——同时阶段 9 仍必须保留透明背景，不是整张脏面。
func TestCrackTextureGrowsIncrementally(t *testing.T) {
	previous := map[int]bool{}
	for stage := 0; stage < crackStageCount; stage++ {
		px := crackTexture(stage)
		current := map[int]bool{}
		for i := 3; i < len(px); i += 4 {
			if px[i] != 0 {
				current[i/4] = true
			}
		}
		if len(current) == 0 {
			t.Fatalf("阶段 %d 一个裂纹像素都没有", stage)
		}
		for point := range previous {
			if !current[point] {
				t.Fatalf("阶段 %d 丢失了阶段 %d 的像素 (%d,%d)：裂纹必须增量生长",
					stage, stage-1, point%texSize, point/texSize)
			}
		}
		if stage == 0 {
			if len(current) > 8 {
				t.Fatalf("阶段 0 的裂纹像素=%d，初始裂点应当只有中心附近少量几个", len(current))
			}
		}
		if stage == crackStageCount-1 {
			if len(current) <= len(previous) {
				t.Fatalf("阶段 9 的裂纹像素=%d 未多于阶段 8 的 %d：末阶段应当大面积破裂", len(current), len(previous))
			}
			if len(current) >= texSize*texSize {
				t.Fatal("阶段 9 铺满整张纹理：裂纹必须保留透明背景")
			}
		}
		previous = current
	}
	if len(previous) == 0 {
		t.Fatal("裂纹纹理整体为空")
	}
}

// TestCrackTextureUsesBinaryAlphaAndDeepWarmColors 锁定裂纹像素的配色契约：
// alpha 只取 0/255（terrain 系 cutout 的 `c.a` 阈值 discard 前提，mip 链由
// downsampleCutout 保住覆盖率）；非透明像素落在 0x2a..0x4a 的暖深灰棕域
// （R≥G≥B），且至少用满三档明暗做出像素层次——既不能跳出深色域，也不能
// 退化成单一颜色的平面。
func TestCrackTextureUsesBinaryAlphaAndDeepWarmColors(t *testing.T) {
	shades := map[[3]byte]bool{}
	for stage := 0; stage < crackStageCount; stage++ {
		px := crackTexture(stage)
		for i := 0; i < len(px); i += 4 {
			switch px[i+3] {
			case 0:
				if px[i] != 0 || px[i+1] != 0 || px[i+2] != 0 {
					t.Fatalf("阶段 %d 像素 %d 透明但 RGB 非零：(%d,%d,%d)",
						stage, i/4, px[i], px[i+1], px[i+2])
				}
			case 255:
				for _, channel := range [3]byte{px[i], px[i+1], px[i+2]} {
					if channel < 0x2a || channel > 0x4a {
						t.Fatalf("阶段 %d 像素 %d 的颜色 (%d,%d,%d) 跳出 0x2a..0x4a 深色域",
							stage, i/4, px[i], px[i+1], px[i+2])
					}
				}
				if px[i] < px[i+1] || px[i+1] < px[i+2] {
					t.Fatalf("阶段 %d 像素 %d 的颜色 (%d,%d,%d) 不是暖色（R≥G≥B）",
						stage, i/4, px[i], px[i+1], px[i+2])
				}
				shades[[3]byte{px[i], px[i+1], px[i+2]}] = true
			default:
				t.Fatalf("阶段 %d 像素 %d 的 alpha=%d，cutout 只允许 0/255", stage, i/4, px[i+3])
			}
		}
	}
	if len(shades) < 3 {
		t.Fatalf("裂纹只用了 %d 档颜色，想要至少 3 档明暗层次", len(shades))
	}
}

// TestCrackLayersAreCutout 是**位置性**断言：裂纹 10 层必须真的被
// isCutoutLayer 判进 cutout 集合（alpha 二值 + downsampleCutout 保覆盖率
// 的 mip 降采样），并且紧邻的床层仍在集合外——把判据写成
// 「layer >= LayerCrack0」这类开区间会同时把无关层拖进来。
func TestCrackLayersAreCutout(t *testing.T) {
	for stage := 0; stage < crackStageCount; stage++ {
		layer := int(LayerCrack0) + stage
		if !isCutoutLayer(layer) {
			t.Fatalf("裂纹层 %d 未进 cutout 集合：普通平均降采样会让稀疏裂纹在 mip 链上整层消失", layer)
		}
	}
	if isCutoutLayer(int(LayerBedHeadEast)) {
		t.Fatalf("床层 %d 不应进 cutout 集合：裂纹判据越过了区间下界", int(LayerBedHeadEast))
	}
}

// TestCrackBindingsResolveToDedicatedLayers 锁定 crack_0..crack_9 的绑定槽位：
// 十个名字逐一解析到对应阶段层号，材质包可按槽位覆盖（镜像 torch/bed 的
// 仅覆盖槽位语义，仓库自身不携带 crack png）。名字或层号错位会让覆盖文件
// 静默落到别的裂纹阶段上。
func TestCrackBindingsResolveToDedicatedLayers(t *testing.T) {
	byName := make(map[string]uint16, len(textureBindings))
	for _, binding := range textureBindings {
		byName[binding.name] = binding.layer
	}
	for stage := 0; stage < crackStageCount; stage++ {
		name := fmt.Sprintf("crack_%d", stage)
		layer, ok := byName[name]
		if !ok {
			t.Fatalf("textureBindings 缺少槽位 %q", name)
		}
		if want := LayerCrack0 + uint16(stage); layer != want {
			t.Fatalf("槽位 %q 解析到层 %d，想要 %d", name, layer, want)
		}
	}
}

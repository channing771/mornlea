package assets

import (
	"fmt"
	"testing"
)

// TestCrackLayerNumbersAreFrozen 是裂纹 10 层的层号冻结守卫。
//
// 层数值真值源是本文件 blocks.go 的层枚举，但裂纹实例以 f32 层号直接索引
// atlas，呈现层从 LayerCrack0 派生各阶段层号——层号一旦平移，裂纹会被采样到
// 别的材质层上。本条钉住 LayerCrack0..LayerCrack9 = 69..78；短草层 68 插入床
// 区间之后顺延，牛四层 79..82 与物品图标层再随裂纹之后追加。裂纹区间
// 紧贴短草层上界：在短草与裂纹之间插层必然撞上断言。植物 31..54、耕地 29/30、
// 火把 59、床 60..67、短草 68 各区间另有专属守卫（farmland/plant/torch/bed/
// short_grass 的既有测试），牛四层由牛层号追加测试守护，此处不重复。
func TestCrackLayerNumbersAreFrozen(t *testing.T) {
	if got := int(LayerCrack0); got != 69 {
		t.Fatalf("LayerCrack0=%d，想要冻结值 69", got)
	}
	if got := int(LayerCrack9); got != 78 {
		t.Fatalf("LayerCrack9=%d，想要冻结值 78", got)
	}
	if got := int(layerCount); got != int(LayerItemBrokenIronSword)+1 {
		t.Fatalf("layerCount=%d，想要覆盖末个物品层 %d", got, int(LayerItemBrokenIronSword)+1)
	}
	if LayerCrack0 != LayerShortGrass+1 {
		t.Fatalf("LayerCrack0=%d 不紧贴短草层上界 %d，插层检测失效", LayerCrack0, LayerShortGrass+1)
	}
	// 每个命名常量都逐条钉在字面量上：只断言首尾两个端点时，中间常量被
	// 误挪（插入/删除一个层）不会让端点断言变红。
	for _, tt := range []struct {
		name  string
		layer uint16
		want  int
	}{
		{"LayerCrack1", LayerCrack1, 70},
		{"LayerCrack2", LayerCrack2, 71},
		{"LayerCrack3", LayerCrack3, 72},
		{"LayerCrack4", LayerCrack4, 73},
		{"LayerCrack5", LayerCrack5, 74},
		{"LayerCrack6", LayerCrack6, 75},
		{"LayerCrack7", LayerCrack7, 76},
		{"LayerCrack8", LayerCrack8, 77},
	} {
		if got := int(tt.layer); got != tt.want {
			t.Fatalf("%s=%d，想要冻结值 %d", tt.name, got, tt.want)
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
// 每个阶段都非空；阶段 0（中心初始裂点）保持小簇；阶段 2（早期）的裂纹
// 必须仍聚在中心邻域——同类体素游戏的可观察节奏是「早阶段只有中心几点
// 短裂口、随阶段向外生长」，若生成算法退化成早期即有长裂缝贯穿半张纹理，
// 阶段间的直觉差异就没了；阶段 9（末阶段）的破裂网必须触及纹理四边
// （裂纹从中心长到边），且保持较大覆盖但保留透明背景。
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
		if stage == 2 {
			for point := range current {
				x, y := point%texSize, point/texSize
				if dx, dy := x-8, y-8; dx < -6 || dx > 6 || dy < -6 || dy > 6 {
					t.Fatalf("阶段 2 的像素 (%d,%d) 距中心超过 6：早期裂纹应聚在中心邻域", x, y)
				}
			}
		}
		if stage == crackStageCount-1 {
			if len(current) <= len(previous) {
				t.Fatalf("阶段 9 的裂纹像素=%d 未多于阶段 8 的 %d：末阶段应当大面积破裂", len(current), len(previous))
			}
			if len(current) < 80 {
				t.Fatalf("阶段 9 的裂纹像素=%d：末阶段应当是铺开的大面积破裂网", len(current))
			}
			if len(current) >= texSize*texSize/2 {
				t.Fatal("阶段 9 覆盖过半：裂纹必须保留透明背景为主")
			}
			for _, edge := range []struct {
				name  string
				touch bool
			}{
				{"上边", anyEdge(current, func(x, y int) bool { return y == 0 })},
				{"下边", anyEdge(current, func(x, y int) bool { return y == texSize-1 })},
				{"左边", anyEdge(current, func(x, y int) bool { return x == 0 })},
				{"右边", anyEdge(current, func(x, y int) bool { return x == texSize-1 })},
			} {
				if !edge.touch {
					t.Fatalf("阶段 9 未触及%s：破裂网应从中心长到四边", edge.name)
				}
			}
		}
		previous = current
	}
	if len(previous) == 0 {
		t.Fatal("裂纹纹理整体为空")
	}
}

// anyEdge 报告裂纹像素集合里是否存在满足判据的边缘像素（阶段 9 四边可达
// 断言的辅助）。
func anyEdge(points map[int]bool, match func(x, y int) bool) bool {
	for point := range points {
		if match(point%texSize, point/texSize) {
			return true
		}
	}
	return false
}

// TestCrackTextureUsesBinaryAlphaAndDeepWarmColors 锁定裂纹像素的配色契约：
// alpha 只取 0/255（terrain 系 cutout 的 `c.a` 阈值 discard 前提，mip 链由
// downsampleCutout 保住覆盖率）；非透明像素落在 0x10..0x38 的近黑暖棕域
// （R≥G≥B）——裂纹要读作方块表面上的深色阴影缝，在浅色与深色材质上都可
// 辨认，配色提亮会丢掉对比；且跨阶段累计 MUST 至少用满 3 档密度明暗
// ——断面深度层次（宽缝内芯最深、细线居中）是硬契约；「孤立碎屑最浅」
// 的第 4 档依赖真正孤立的像素，在这个网络密度下天然稀少，允许缺席，
// 不作为硬性要求。
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
					if channel < 0x10 || channel > 0x38 {
						t.Fatalf("阶段 %d 像素 %d 的颜色 (%d,%d,%d) 跳出 0x10..0x38 近黑深色域",
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
		t.Fatalf("裂纹只用了 %d 档颜色，密度分层的断面深度至少要 3 档", len(shades))
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

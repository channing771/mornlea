package hud

import (
	"fmt"
	"math"
	"testing"
)

// stabilityAtlasWidths 是图集 UV 稳定性性质扫描的宽度集：全部是固定列宽
// 16 纹素的倍数，首项是当前真实宽度（50 列 × 16），其余项模拟未来扩列，
// 同时覆盖 2 的幂（除法精确舍入）与非 2 幂（除法有舍入噪声）两类行为。
// 仅被本文件的三个稳定性测试使用，不进共享 helper 中心。
var stabilityAtlasWidths = [...]int{800, 816, 832, 1024, 4096}

// stabilityMinMarginTexels 是 delta spec 要求的最小安全裕度：解码回纹素空间后
// 的列界必须距两侧列边界各不少于 1/512 纹素，才能保证 f32 重归一化噪声
// （量级 W·2^-24 纹素）永不翻转采样归属。
const stabilityMinMarginTexels = 1.0 / 512.0

// TestHotbarColumnUVKeepsMarginFromColumnBoundaries 钉死 delta spec
// 「HUD 图集列采样稳定性」Scenario「扩列后既有 cell 解码纹素集合不变」的
// 前半句：任意宽度下任意列的解码左右界必须严格落在本列
// [列×16, (列+1)×16) 内，且距两侧列边界各不少于 1/512 纹素。
//
// 杀死的变异：删掉亚纹素收进回到精确边界计算（float32 除法舍入会让解码界
// 贴到列边界上甚至越界），或把收进余量缩小到与重归一化噪声同量级——两者
// 都让「距边界 ≥ 1/512 纹素」失守；既有容差 0.01 纹素的回归测试对这类贴边
// 变异不敏感，本测试以 spec 裕度作为更紧的判据。
func TestHotbarColumnUVKeepsMarginFromColumnBoundaries(t *testing.T) {
	for _, width := range stabilityAtlasWidths {
		columns := width / hotbarTextureSize
		for column := 0; column < columns; column++ {
			uv := hotbarColumnUV(column, width)
			left := float64(uv[0]) * float64(width)
			right := float64(uv[2]) * float64(width)
			start := float64(column * hotbarTextureSize)
			end := float64((column + 1) * hotbarTextureSize)
			if margin := left - start; margin < stabilityMinMarginTexels {
				t.Fatalf("宽度 %d 列 %d 左界解码 texel=%v，距列左界仅 %v，不足 %v",
					width, column, left, margin, stabilityMinMarginTexels)
			}
			if margin := end - right; margin < stabilityMinMarginTexels {
				t.Fatalf("宽度 %d 列 %d 右界解码 texel=%v，距列右界仅 %v，不足 %v",
					width, column, right, margin, stabilityMinMarginTexels)
			}
		}
	}
}

// TestHotbarAdjacentColumnUVsDoNotOverlap 钉死 delta spec Scenario
// 「相邻列采样区间互不侵入」：任意图集宽度下，前列右界的解码值不得超过
// 后列左界的解码值——两列区间一旦在共享边界附近重叠，图标就会采到相邻列
// 的材质而互相串味。
//
// 杀死的变异：左右两侧收进不对称（例如只收左界不收右界）、或某侧收进量取
// 负值（外扩越界），都会使前列右界越过下列左界。
func TestHotbarAdjacentColumnUVsDoNotOverlap(t *testing.T) {
	for _, width := range stabilityAtlasWidths {
		columns := width / hotbarTextureSize
		for column := 0; column+1 < columns; column++ {
			prevRight := float64(hotbarColumnUV(column, width)[2]) * float64(width)
			nextLeft := float64(hotbarColumnUV(column+1, width)[0]) * float64(width)
			if prevRight > nextLeft {
				t.Fatalf("宽度 %d 相邻列 %d/%d 区间重叠：前列右界 texel=%v 大于后列左界 texel=%v",
					width, column, column+1, prevRight, nextLeft)
			}
		}
	}
}

// TestHotbarColumnUVDecodesToSameTexelsAcrossAtlasWidths 钉死 delta spec
// Scenario「扩列后既有 cell 解码纹素集合不变」的 AND 子句：以列内均匀分布的
// 探针位置解码，扩列前后得到的纹素下标集合必须完全相同。探针取每纹素中点
// t_i = (i+0.5)/16，经该列 UV 区间线性插值、乘宽度、floor 映射回相对列起点的
// 纹素下标，模拟 HUD quad 在非整数缩放下实际采到的纹素。
//
// 杀死的变异：任何让解码结果随图集宽度漂移的实现——收进量随宽度缩放、
// 插值精度不足、或按宽度重排 UV——都会让同一列在不同宽度下解析到不同纹素
// （甚至解码出 -1 或 16 这类列外下标），不同宽度的探针集合随之出现差异。
func TestHotbarColumnUVDecodesToSameTexelsAcrossAtlasWidths(t *testing.T) {
	minColumns := stabilityAtlasWidths[0] / hotbarTextureSize
	for _, width := range stabilityAtlasWidths {
		if columns := width / hotbarTextureSize; columns < minColumns {
			minColumns = columns
		}
	}
	// probeSet 返回指定列在指定图集宽度下 16 个均匀探针解码出的纹素下标集合；
	// 合法值是 0..15 的某个子集，解码出列外下标直接判失败。
	probeSet := func(width, column int) [16]bool {
		uv := hotbarColumnUV(column, width)
		var set [16]bool
		for i := range hotbarTextureSize {
			probe := (float64(i) + 0.5) / float64(hotbarTextureSize)
			u := float64(uv[0]) + probe*(float64(uv[2])-float64(uv[0]))
			rel := int(math.Floor(u*float64(width))) - column*hotbarTextureSize
			if rel < 0 || rel >= hotbarTextureSize {
				t.Fatalf("宽度 %d 列 %d 探针 %d 解码出列外纹素下标 %d", width, column, i, rel)
			}
			set[rel] = true
		}
		return set
	}
	for column := 0; column < minColumns; column++ {
		baseline := probeSet(stabilityAtlasWidths[0], column)
		for _, width := range stabilityAtlasWidths[1:] {
			got := probeSet(width, column)
			if got == baseline {
				continue
			}
			diff := ""
			for i := range hotbarTextureSize {
				if got[i] != baseline[i] {
					diff += fmt.Sprintf(" [%d]:基准=%v 实际=%v", i, baseline[i], got[i])
				}
			}
			t.Fatalf("宽度 %d 列 %d 探针纹素集合与基准宽度 %d 不同：%s",
				width, column, stabilityAtlasWidths[0], diff)
		}
	}
}

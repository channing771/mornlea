package hud

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/channing771/mornlea/internal/assets"
)

// 图标精修（design D6）的守护测试：HUD 图集固定 cell 的像素 mask 就地重绘后，
// 必须保持「二值 alpha、语义色族、剪影同族、无杂点」四条性质，并且不得回退到
// 精修前的旧基线剪影。重绘只动 painter 的像素数据，图集列布局与 UV 收进不在
// 本测试射程内（由图集列采样稳定性测试另行钉死）。

// legacyIconCellDigests 是精修前各固定 cell 的 SHA256 旧基线：重绘后的 cell
// 必须与旧基线不同——这条断言杀死「把 mask 改回旧剪影」的回退变异，也让
// 「新 mask 确实重绘过」成为被测试钉住的事实而不是口头承诺。
var legacyIconCellDigests = map[int]string{
	hotbarEmptyHeartColumn:     "bdb177bfce78a5bca1d04eed6b6ce954f222cfb0f41b43a8813ea32054ee4d26",
	hotbarHalfHeartColumn:      "9eef31f2c9548f026ce5988bf3b3f6cb5ad592c25b111d58a5adfc48272c6224",
	hotbarFullHeartColumn:      "cb940fd384be20ccc48135828d0b7dcd9286130215295f73d2de3ebb276a4416",
	hotbarEmptyBubbleColumn:    "7aad989947193a882c88f7210afae1fbebce180ba22ea3d1926253b0b47af365",
	hotbarFullBubbleColumn:     "3d60edf7c03d74fa7c2bbf184f144a80105bfe743da689f21c45a320eeae50f9",
	hotbarEmptyDrumstickColumn: "deb1316bf75c14273ce8e47781bf12f4c93d2f0af03e034db6d57befe40e4e9a",
	hotbarFullDrumstickColumn:  "ef98ab478ef3e8f276feacce353bd021012ba2ecfc6e9bf18eda47ea4510bfe5",
	hotbarContainerSlotColumn:  "c8fbbcecbad25516ffc0cfe0cd059d0411e0dc8dfdf0ffcb15bc36be912f945c",
	hotbarCraftingTitleColumn:  "46bb2abb0eafaf190e521c9f817e577bd167e9457cdf243248e49a389d1d641a",
	hotbarChestTitleColumn:     "45efb42cf7471a26897db1bf7abda26118627e9e5f562bd49e730f9417ce3f89",
	hotbarFurnaceTitleColumn:   "eeae7602ddeaa765f721249abb08fff682a255afa41b7193dbcae49567e2bb39",
	hotbarFurnaceFlameColumn:   "893c0456e406e39d960eb9ebc1aad770c2799c2d785493ad6374f2f978ac0f6e",
	hotbarFurnaceArrowColumn:   "2d66cf6ba62ec8e6f7deafff4f4b2d5b8a1180eaf54e0df84c6eb6db97aeb0fb",
}

// TestHotbarIconRefinementReplacesLegacyMasks 钉住「精修后的每个固定 cell 都
// 不是旧基线像素」。旧摘要只作为回退守卫存在；当前像素的精确回归钉在
// `TestHotbarTextureAtlasUIIconsAreDistinctBinaryAndDeterministic`。
func TestHotbarIconRefinementReplacesLegacyMasks(t *testing.T) {
	pixels := buildHotbarTextureAtlas(assets.NewRegistry())
	for column, legacy := range legacyIconCellDigests {
		cell := hotbarTextureCell(pixels, column)
		got := hex.EncodeToString(mustCellDigest(t, cell))
		if got == legacy {
			t.Fatalf("列 %d 的像素仍与精修前旧基线相同，图标重绘被回退", column)
		}
	}
}

func mustCellDigest(t *testing.T, cell []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(cell)
	return sum[:]
}

// iconCellStats 汇总一个 16×16 cell 的不透明像素统计：数量、8-连通组件数与
// 最小组件大小。连通性是「去杂点」的机械判据：孤立的 1-2 px 组件就是杂点。
func iconCellStats(cell []byte) (opaque int, components, smallest int) {
	at := func(x, y int) bool {
		return cell[(y*hotbarTextureSize+x)*4+3] == 255
	}
	seen := make([]bool, hotbarTextureSize*hotbarTextureSize)
	smallest = 1 << 30
	for y := range hotbarTextureSize {
		for x := range hotbarTextureSize {
			if !at(x, y) || seen[y*hotbarTextureSize+x] {
				continue
			}
			components++
			size := 0
			stack := [][2]int{{x, y}}
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if seen[p[1]*hotbarTextureSize+p[0]] {
					continue
				}
				seen[p[1]*hotbarTextureSize+p[0]] = true
				size++
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := p[0]+dx, p[1]+dy
						if nx >= 0 && nx < hotbarTextureSize && ny >= 0 && ny < hotbarTextureSize &&
							at(nx, ny) && !seen[ny*hotbarTextureSize+nx] {
							stack = append(stack, [2]int{nx, ny})
						}
					}
				}
			}
			if size < smallest {
				smallest = size
			}
			opaque += size
		}
	}
	if components == 0 {
		smallest = 0
	}
	return opaque, components, smallest
}

// TestHotbarIconCellsKeepBinaryAlphaAndSemanticColorFamilies 钉住 D1/D6 的两条
// 硬约束：图标像素 alpha 只有 0/255（无半透明边缘），且每个图标的全部不透明
// 像素都落在声明的语义色族内——心红系、气泡青白系、鸡腿棕系、凹槽冷灰、标题
// 单色米白、火焰橙黄系、箭头中性灰。任何引入新色相或杂色的变异在此失败。
func TestHotbarIconCellsKeepBinaryAlphaAndSemanticColorFamilies(t *testing.T) {
	pixels := buildHotbarTextureAtlas(assets.NewRegistry())
	families := map[int]struct {
		name      string
		minOpaque int
		family    func(r, g, b byte) bool
	}{
		hotbarEmptyHeartColumn:     {"心红系", 80, func(r, g, b byte) bool { return r > g && r > b }},
		hotbarHalfHeartColumn:      {"心红系", 80, func(r, g, b byte) bool { return r > g && r > b }},
		hotbarFullHeartColumn:      {"心红系", 80, func(r, g, b byte) bool { return r > g && r > b }},
		hotbarEmptyBubbleColumn:    {"气泡青白系", 90, func(r, g, b byte) bool { return r < g && r < b }},
		hotbarFullBubbleColumn:     {"气泡青白系", 90, func(r, g, b byte) bool { return r < g && r < b }},
		hotbarEmptyDrumstickColumn: {"鸡腿棕系", 64, func(r, g, b byte) bool { return r > g && g > b }},
		hotbarFullDrumstickColumn:  {"鸡腿棕系", 64, func(r, g, b byte) bool { return r > g && g > b }},
		hotbarContainerSlotColumn:  {"凹槽冷灰", 256, func(r, g, b byte) bool { return g > r && b > g }},
		hotbarCraftingTitleColumn:  {"标题米白", 20, func(r, g, b byte) bool { return r > 200 && g > 200 && b > 200 }},
		hotbarChestTitleColumn:     {"标题米白", 20, func(r, g, b byte) bool { return r > 200 && g > 200 && b > 200 }},
		hotbarFurnaceTitleColumn:   {"标题米白", 20, func(r, g, b byte) bool { return r > 200 && g > 200 && b > 200 }},
		hotbarFurnaceFlameColumn:   {"火焰橙黄系", 28, func(r, g, b byte) bool { return r > g && g > b }},
		hotbarFurnaceArrowColumn: {"箭头中性", 28, func(r, g, b byte) bool {
			lo, hi := minByte(r, g, b), maxByte(r, g, b)
			return hi-lo <= 20 && lo >= 100
		}},
	}
	for column, test := range families {
		cell := hotbarTextureCell(pixels, column)
		opaque := 0
		for offset := 3; offset < len(cell); offset += 4 {
			switch alpha := cell[offset]; alpha {
			case 0:
			case 255:
				opaque++
				r, g, b := cell[offset-3], cell[offset-2], cell[offset-1]
				if !test.family(r, g, b) {
					t.Fatalf("列 %d（%s）像素 RGB=(%d,%d,%d) 越出语义色族", column, test.name, r, g, b)
				}
			default:
				t.Fatalf("列 %d（%s）alpha=%d，想要 0 或 255", column, test.name, alpha)
			}
		}
		if opaque < test.minOpaque {
			t.Fatalf("列 %d（%s）不透明像素=%d，少于可辨识下限 %d", column, test.name, opaque, test.minOpaque)
		}
	}
}

func minByte(values ...byte) byte {
	min := byte(255)
	for _, value := range values {
		if value < min {
			min = value
		}
	}
	return min
}

func maxByte(values ...byte) byte {
	var max byte
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

// TestHotbarIconSilhouettesStaySameFamily 钉住「剪影与既有同族可辨识」：重绘
// 前后剪影的覆盖规模必须同量级（不透明像素数落在旧值的既定倍率带内），生存
// 图标保持单个连通剪影、标题 cell 恰为两枚连通字形；凹槽保持满幅 16×16。
// legacyIconOpaqueCounts 是精修前的实测旧值，仅作倍率基准。
func TestHotbarIconSilhouettesStaySameFamily(t *testing.T) {
	legacyIconOpaqueCounts := map[int]int{
		hotbarEmptyHeartColumn:     142,
		hotbarEmptyBubbleColumn:    112,
		hotbarEmptyDrumstickColumn: 112,
		hotbarFurnaceFlameColumn:   31,
		hotbarFurnaceArrowColumn:   44,
	}
	pixels := buildHotbarTextureAtlas(assets.NewRegistry())
	for column, legacy := range legacyIconOpaqueCounts {
		cell := hotbarTextureCell(pixels, column)
		opaque, components, smallest := iconCellStats(cell)
		if components != 1 {
			t.Fatalf("列 %d 剪影有 %d 个连通组件，想要单一剪影", column, components)
		}
		ratio := float64(opaque) / float64(legacy)
		if ratio < 0.6 || ratio > 1.6 {
			t.Fatalf("列 %d 不透明像素=%d，相对旧剪影 %d 的倍率 %.2f 越出 [0.6,1.6]",
				column, opaque, legacy, ratio)
		}
		_ = smallest
	}
	// 凹槽是所有栏位共用的满幅底：必须铺满整个 16×16 cell。
	if opaque, _, _ := iconCellStats(hotbarTextureCell(pixels, hotbarContainerSlotColumn)); opaque != 256 {
		t.Fatalf("凹槽不透明像素=%d，想要满幅 256", opaque)
	}
	// 标题 cell 恰为两枚字形：组件数=2 且没有小于 8 px 的碎字形（杂点）。
	for _, column := range []int{hotbarCraftingTitleColumn, hotbarChestTitleColumn, hotbarFurnaceTitleColumn} {
		opaque, components, smallest := iconCellStats(hotbarTextureCell(pixels, column))
		if components != 2 {
			t.Fatalf("标题列 %d 有 %d 个连通组件，想要恰两枚字形", column, components)
		}
		if smallest < 8 {
			t.Fatalf("标题列 %d 最小字形只有 %d px，疑似杂点", column, smallest)
		}
		if opaque < 20 {
			t.Fatalf("标题列 %d 不透明像素=%d，字形过小", column, opaque)
		}
	}
}

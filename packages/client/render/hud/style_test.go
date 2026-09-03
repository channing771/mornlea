package hud

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

// style.go 是容器保留面呈现色的唯一样式来源（design D1）。本文件从两端钉住
// 这件事：一端把每个令牌的数值钉成字面量（精修令牌值时必须显式改这里），另一端
// 扫描包内生产源码，禁止在 style.go 之外出现第二份 float32 呈现色字面量。常显
// 层（快捷栏贴条、状态行、氧气、采掘/进食轨道、准星、聊天）专属令牌已随其绘制
// 退役，前端改用 `tokens.css` 的 `--hud-*` 段。

// TestStyleTokensMatchPinnedValues 把令牌表钉成字面量：这里是令牌数值的唯一
// 变更入口。强调色按语义拆成两族分别钉值：`accentSelected`（鼠尾草绿，选中）
// 与 `accentProgress`（麦金，进度/产物/来源）。
func TestStyleTokensMatchPinnedValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		token [4]float32
		want  [4]float32
	}{
		{"panelShadow", panelShadow, [4]float32{0.180, 0.129, 0.082, 0.90}},
		{"panelSurface", panelSurface, [4]float32{0.941, 0.894, 0.784, 0.94}},
		{"panelBorderLight", panelBorderLight, [4]float32{0.541, 0.416, 0.282, 0.80}},
		{"slotWell", slotWell, [4]float32{0.851, 0.776, 0.604, 0.92}},
		{"slotWellEdge", slotWellEdge, [4]float32{0.984, 0.961, 0.894, 0.30}},
		{"accentSelected", accentSelected, [4]float32{0.494, 0.612, 0.388, 0.98}},
		{"accentProgress", accentProgress, [4]float32{0.851, 0.663, 0.306, 0.98}},
		{"textPrimaryFg", textPrimaryFg, [4]float32{0.969, 0.941, 0.871, 1}},
		{"textPrimaryShadow", textPrimaryShadow, [4]float32{0.165, 0.118, 0.071, 0.85}},
		{"textOnPanelFg", textOnPanelFg, [4]float32{0.239, 0.180, 0.125, 1}},
		{"hotbarSelectedInnerColor", hotbarSelectedInnerColor, accentSelected},
		{"miningTrackColor", miningTrackColor, [4]float32{0.05, 0.05, 0.06, 0.78}},
		{"durabilityTrackColor", durabilityTrackColor, [4]float32{0.05, 0.05, 0.06, 0.85}},
		{"durabilityHealthyColor", durabilityHealthyColor, [4]float32{0.30, 0.78, 0.36, 0.95}},
		{"durabilityLowColor", durabilityLowColor, [4]float32{0.90, 0.35, 0.25, 0.95}},
		{"containerSourceHighlightColor", containerSourceHighlightColor, [4]float32{0.851, 0.663, 0.306, 0.98}},
	} {
		if test.token != test.want {
			t.Errorf("令牌 %s=%v，想要钉值 %v", test.name, test.token, test.want)
		}
	}
}

// TestStyleTokensAreTheOnlyFloatColorSource 扫描包内生产源码（非测试、非
// style.go），断言不存在第二份 float32 呈现色字面量：面板、文字、进度、选中
// 等呈现色只能引用 style.go 令牌。图标 painter 的 [4]byte 像素调色板是声明
// 内的例外（语义色族由图集 mask 测试按色族守护），多元素的 viewport 字面量
// 与运行期乘法合成色不会被本扫描匹配。
func TestStyleTokensAreTheOnlyFloatColorSource(t *testing.T) {
	allowedOutsideStyleFile := map[[4]float32]string{
		{1, 1, 1, 1}: "纹理采样四边形的中性乘色",
	}
	literalPattern := regexp.MustCompile(`\[4\]float32\{([^}]*)\}`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读取包目录: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || name == "style.go" {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读取 %s: %v", name, err)
		}
		for _, match := range literalPattern.FindAllStringSubmatch(string(source), -1) {
			parts := strings.Split(match[1], ",")
			if len(parts) != 4 {
				continue
			}
			var value [4]float32
			parseable := true
			for index, part := range parts {
				component, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
				if err != nil {
					parseable = false
					break
				}
				value[index] = float32(component)
			}
			if !parseable {
				continue
			}
			if reason, ok := allowedOutsideStyleFile[value]; ok {
				if reason == "" {
					t.Errorf("%s 的白名单理由为空", name)
				}
				continue
			}
			t.Errorf("%s 出现令牌表之外的颜色字面量 %v：呈现色必须迁入 style.go", name, value)
		}
	}
}

// TestHotbarDigitsUseTextPrimaryTokens 钉住快捷栏数量数字并入同一套文字双层
// 规范：颜色取文字令牌，阴影层相对前景层偏移 1 design px。
func TestHotbarDigitsUseTextPrimaryTokens(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	appendHotbarCountScaled(&layout, atlas, 64, 100, 200, 1)
	if len(layout.glyphs) != 4 {
		t.Fatalf("数量 64 字形=%d，想要两个阴影加两个前景", len(layout.glyphs))
	}
	for index, glyph := range layout.glyphs {
		want := textPrimaryShadow
		if index >= 2 {
			want = textPrimaryFg
		}
		if glyph.Color != want {
			t.Fatalf("数字字形 %d 颜色=%v，想要令牌 %v", index, glyph.Color, want)
		}
	}
	for index := range 2 {
		shadow, foreground := layout.glyphs[index], layout.glyphs[index+2]
		if shadow.X != foreground.X+1 || shadow.Y != foreground.Y+1 {
			t.Fatalf("数字阴影 %d 偏移=(%v,%v)，想要右下 1 design px",
				index, shadow.X-foreground.X, shadow.Y-foreground.Y)
		}
	}
}

// TestOpenPanelsConsumePanelTokens 钉住打开态面板族全部取自面板令牌：外扩
// 投影取 `panelShadow`，表面取 `panelSurface`，四边描边取 `panelBorderLight`；
// 右侧配方栏入口取槽位令牌 `slotWell`/`slotWellEdge`，产物格轮廓取麦金
// `accentProgress`、选中内衬取鼠尾草绿 `accentSelected`，两族强调不同格。
// 三类容器视图的面板同源。
func TestOpenPanelsConsumePanelTokens(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, nil, 1280, 800)
	// 面板族是投影、表面与四边亮边（标题为纹理 cell 不在此列）。
	family := got.quads[:containerPanelQuads-1]
	wantColors := [containerPanelQuads - 1][4]float32{
		panelShadow, panelSurface,
		panelBorderLight, panelBorderLight, panelBorderLight, panelBorderLight,
	}
	for index, want := range wantColors {
		if family[index].Color != want {
			t.Fatalf("打开态面板层 %d 颜色=%v，想要令牌 %v", index, family[index].Color, want)
		}
	}

	var chest hotbarLayout
	chestGot := layoutInventory(&chest, atlas, core.Inventory{}, true, -1, nil, nil, &ChestOverlay{}, 1280, 800)
	chestFamily := chestGot.quads[:containerPanelQuads-1]
	for index, want := range wantColors {
		if chestFamily[index].Color != want {
			t.Fatalf("箱子面板层 %d 颜色=%v，想要令牌 %v", index, chestFamily[index].Color, want)
		}
	}

	// 配方栏十条入口：井取 `slotWell`、上沿内高光取 `slotWellEdge`。
	var crafting hotbarLayout
	craftingGot := layoutInventory(&crafting, atlas, core.Inventory{}, true, -1, &CraftingOverlay{Size: 2}, nil, nil, 1280, 800)
	wells, edges := 0, 0
	for _, quad := range craftingGot.quads {
		switch quad.Color {
		case slotWell:
			wells++
		case slotWellEdge:
			edges++
		}
	}
	if wells != recipeEntryCount || edges != recipeEntryCount {
		t.Fatalf("配方栏井/高光=%d/%d，想要各 %d", wells, edges, recipeEntryCount)
	}
	// 双强调分工：麦金 quad 只剩产物格轮廓底衬，鼠尾草绿 quad 只剩打开态选中格
	// 内衬；同帧并存时两族不互占语义。数量之外还把两枚 quad 的矩形分别钉到各自
	// 的布局推导值：内衬外扩 `hotbarSelectBorder` 包住当前选中格，轮廓底衬外扩
	// `craftingOutputOutlineExpand` 包住产物格，两族几何不同，颜色互换或错格
	// 都会在矩形断言上失败。外扩常量的量级由常量断言钉住，矩形断言负责颜色
	// 互换与错格——矩形期望值与生产代码共享同一常量，常量自身漂移不会变红。
	if hotbarSelectBorder != 3 {
		t.Fatalf("选中内衬外扩=%v，想要 3 design px", hotbarSelectBorder)
	}
	if craftingOutputOutlineExpand != 1 {
		t.Fatalf("产物轮廓外扩=%v，想要 1 design px", craftingOutputOutlineExpand)
	}
	progressCount, selectedCount := 0, 0
	var progressQuad, selectedQuad hotbarInstance
	for _, quad := range craftingGot.quads {
		switch quad.Color {
		case accentProgress:
			progressQuad, progressCount = quad, progressCount+1
		case accentSelected:
			selectedQuad, selectedCount = quad, selectedCount+1
		}
	}
	if progressCount != craftingOutputOutlineQuads {
		t.Fatalf("麦金 quad=%d，想要产物格轮廓 %d", progressCount, craftingOutputOutlineQuads)
	}
	if selectedCount != 1 {
		t.Fatalf("鼠尾草绿 quad=%d，想要选中内衬 1", selectedCount)
	}
	scale := craftingGot.scale
	expand := craftingOutputOutlineExpand * scale
	outputX, outputY := craftingOutputOrigin(2, 1280, 800)
	if progressQuad.X != outputX-expand || progressQuad.Y != outputY-expand ||
		progressQuad.Width != hotbarSlotSize*scale+2*expand ||
		progressQuad.Height != hotbarSlotSize*scale+2*expand {
		t.Fatalf("麦金矩形=%+v，想要包住产物格 (%f,%f) 并外扩 %v",
			progressQuad, outputX, outputY, craftingOutputOutlineExpand)
	}
	// 夹具零值即选中格 0：`inventorySlotOrigin` 以选中下标推导内衬矩形，
	// 因此这里的推导值就是打开态首格的内衬。
	selectBorder := hotbarSelectBorder * scale
	selectedX, selectedY := inventorySlotOrigin(0, 1280, 800)
	if selectedQuad.X != selectedX-selectBorder || selectedQuad.Y != selectedY-selectBorder ||
		selectedQuad.Width != (hotbarSlotSize+2*hotbarSelectBorder)*scale ||
		selectedQuad.Height != (hotbarSlotSize+2*hotbarSelectBorder)*scale {
		t.Fatalf("鼠尾草绿矩形=%+v，想要包住选中格 (%f,%f) 并外扩 %v",
			selectedQuad, selectedX, selectedY, hotbarSelectBorder)
	}
}

// TestProgressColorsComeFromTokens 钉住耐久条、熔炉进度轨道与容器来源高亮的
// 颜色全部来自 style.go 语义令牌：条形几何由既有测试守护，这里只锁颜色来源。
func TestProgressColorsComeFromTokens(t *testing.T) {
	scale := hudScale(1920, 1080)
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	var worn hotbarLayout
	appendDurabilityBarScaled(&worn, 0, core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full / 2}, 1920, 1080, scale)
	if len(worn.quads) != 2 {
		t.Fatalf("磨损工具耐久 quad=%d，想要底槽加填充共 2", len(worn.quads))
	}
	if worn.quads[0].Color != durabilityTrackColor {
		t.Fatalf("耐久底槽色=%v，想要令牌 %v", worn.quads[0].Color, durabilityTrackColor)
	}
	if worn.quads[1].Color != durabilityHealthyColor {
		t.Fatalf("耐久健康填充=%v，想要令牌 %v", worn.quads[1].Color, durabilityHealthyColor)
	}
	var low hotbarLayout
	appendDurabilityBarScaled(&low, 0, core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 1}, 1920, 1080, scale)
	if low.quads[1].Color != durabilityLowColor {
		t.Fatalf("低耐久填充=%v，想要令牌 %v", low.quads[1].Color, durabilityLowColor)
	}

	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, core.InventorySlots, nil, nil, fullChestOverlay(), 1280, 800)
	wantX, wantY := chestSlotOrigin(0, 1280, 800)
	selectBorder := hotbarSelectBorder * got.scale
	found := false
	for _, quad := range got.quads {
		if quad.X == wantX-selectBorder && quad.Y == wantY-selectBorder &&
			quad.Width == (hotbarSlotSize+2*hotbarSelectBorder)*got.scale {
			if quad.Color != containerSourceHighlightColor {
				t.Fatalf("来源高亮色=%v，想要令牌 %v", quad.Color, containerSourceHighlightColor)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("未找到箱子来源格高亮 quad")
	}
}

// 编译期守护：确认本文件消费的呈现接口仍然存在（`render.GlyphSource` 是文字
// 路径的唯一图集来源）。
var _ render.GlyphSource = (*fakeNameTagAtlas)(nil)

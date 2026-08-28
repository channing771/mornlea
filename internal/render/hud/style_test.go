package hud

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// style.go 是 HUD 呈现色的唯一样式来源（design D1）。本文件从两端钉住这件事：
// 一端把每个令牌的数值钉成字面量（精修令牌值时必须显式改这里），另一端扫描
// 包内生产源码，禁止在 style.go 之外出现第二份 float32 呈现色字面量。

// TestStyleTokensMatchPinnedValues 把令牌表钉成字面量：这里是令牌数值的唯一
// 变更入口。底部快捷栏贴条保持「投影+表面」双层无边（不接 1px 亮边），其
// 专属色值因此与 `panelShadow`/`panelSurface` 并存，见各令牌注释。
func TestStyleTokensMatchPinnedValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		token [4]float32
		want  [4]float32
	}{
		{"panelShadow", panelShadow, [4]float32{0.008, 0.010, 0.014, 0.90}},
		{"panelSurface", panelSurface, [4]float32{0.045, 0.052, 0.062, 0.84}},
		{"panelBorderLight", panelBorderLight, [4]float32{0.92, 0.93, 0.95, 0.16}},
		{"slotWell", slotWell, [4]float32{0.020, 0.024, 0.030, 0.92}},
		{"slotWellEdge", slotWellEdge, [4]float32{1, 1, 1, 0.07}},
		{"accentAmber", accentAmber, [4]float32{1, 0.72, 0.24, 0.98}},
		{"textPrimaryFg", textPrimaryFg, [4]float32{0.96, 0.96, 0.97, 1}},
		{"textPrimaryShadow", textPrimaryShadow, [4]float32{0, 0, 0, 0.85}},
		{"crosshairShadow", crosshairShadow, [4]float32{0, 0, 0, 0.55}},
		{"crosshairFg", crosshairFg, [4]float32{0.96, 0.96, 0.96, 0.92}},
		{"hotbarPanelShadowColor", hotbarPanelShadowColor, [4]float32{0.012, 0.015, 0.02, 0.94}},
		{"hotbarPanelSurfaceColor", hotbarPanelSurfaceColor, [4]float32{0.045, 0.052, 0.06, 0.96}},
		{"hotbarSelectedOuterColor", hotbarSelectedOuterColor, [4]float32{0.96, 0.92, 0.72, 1}},
		{"hotbarSelectedInnerColor", hotbarSelectedInnerColor, accentAmber},
		{"miningTrackColor", miningTrackColor, [4]float32{0.05, 0.05, 0.06, 0.78}},
		{"miningHarvestableColor", miningHarvestableColor, [4]float32{0.30, 0.78, 0.36, 0.95}},
		{"miningBlockedColor", miningBlockedColor, [4]float32{0.95, 0.55, 0.15, 0.95}},
		{"miningCapColor", miningCapColor, [4]float32{0.96, 1, 0.76, 1}},
		{"miningNotchColor", miningNotchColor, [4]float32{0.18, 0.12, 0.08, 1}},
		{"durabilityTrackColor", durabilityTrackColor, [4]float32{0.05, 0.05, 0.06, 0.85}},
		{"durabilityHealthyColor", durabilityHealthyColor, [4]float32{0.30, 0.78, 0.36, 0.95}},
		{"durabilityLowColor", durabilityLowColor, [4]float32{0.90, 0.35, 0.25, 0.95}},
		{"eatingFillColor", eatingFillColor, [4]float32{0.92, 0.78, 0.42, 0.95}},
		{"containerSourceHighlightColor", containerSourceHighlightColor, [4]float32{0.25, 0.72, 1, 0.98}},
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
				_ = reason
				continue
			}
			t.Errorf("%s 出现令牌表之外的颜色字面量 %v：呈现色必须迁入 style.go", name, value)
		}
	}
}

// TestChatTextUsesTextPrimaryTokens 钉住聊天文字与全部 HUD 文字统一的双层
// 规范：先阴影后前景，阴影向右下偏移 1 design px，两层的颜色都来自
// style.go 文字令牌。
func TestChatTextUsesTextPrimaryTokens(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	appendChatText(&layout, atlas, "界", 100, 200, 1)
	if len(layout.glyphs) != 2 {
		t.Fatalf("单 rune 聊天字形=%d，想要阴影加前景共 2", len(layout.glyphs))
	}
	shadow, foreground := layout.glyphs[0], layout.glyphs[1]
	if shadow.Color != textPrimaryShadow {
		t.Fatalf("聊天阴影色=%v，想要令牌 %v", shadow.Color, textPrimaryShadow)
	}
	if foreground.Color != textPrimaryFg {
		t.Fatalf("聊天前景色=%v，想要令牌 %v", foreground.Color, textPrimaryFg)
	}
	if shadow.X != foreground.X+1 || shadow.Y != foreground.Y+1 {
		t.Fatalf("聊天阴影偏移=(%v,%v)，想要相对前景右下 1 design px",
			shadow.X-foreground.X, shadow.Y-foreground.Y)
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
	appendHotbarCount(&layout, atlas, 64, 100, 200)
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

// TestOpenPanelsConsumePanelTokens 钉住打开态面板族全部取自面板令牌：外层
// 背衬与快捷栏行取 `panelShadow`（贴条行的凹陷分组），背包表面取
// `panelSurface`，分组分隔线取 `panelBorderLight`；三类容器视图的面板同源。
// 面板仍保持双层（投影+表面）结构，本测试不引入也不允许任何描边 quad。
func TestOpenPanelsConsumePanelTokens(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
	// 准星先追加，其后的 `openInventoryPanelQuads` 个实例才是面板层。
	panels := got.quads[crosshairQuads:][:openInventoryPanelQuads]
	wantColors := [openInventoryPanelQuads][4]float32{
		panelShadow, panelSurface, panelShadow, panelBorderLight,
	}
	for index, want := range wantColors {
		if panels[index].Color != want {
			t.Fatalf("打开态面板层 %d 颜色=%v，想要令牌 %v", index, panels[index].Color, want)
		}
	}

	var chest hotbarLayout
	appendChestGrid(&chest, atlas, ChestOverlay{}, 1280, 800)
	if chest.quads[0].Color != panelSurface {
		t.Fatalf("箱子面板颜色=%v，想要令牌 panelSurface", chest.quads[0].Color)
	}
	var furnace hotbarLayout
	appendFurnaceRow(&furnace, atlas, FurnaceOverlay{}, 1280, 800)
	if furnace.quads[0].Color != panelSurface {
		t.Fatalf("熔炉面板颜色=%v，想要令牌 panelSurface", furnace.quads[0].Color)
	}
	var crafting hotbarLayout
	appendCraftingGrid(&crafting, atlas, CraftingOverlay{}, 1280, 800)
	if crafting.quads[0].Color != panelSurface {
		t.Fatalf("合成面板颜色=%v，想要令牌 panelSurface", crafting.quads[0].Color)
	}

	// 聊天栈同属浮动面板语言：输入行取更深的投影色作背衬，历史行取表面色。
	var chat hotbarLayout
	appendChatOverlay(&chat, atlas, ChatOverlay{Open: true, Input: "界", Lines: []string{"界"}}, 1280, 720)
	if len(chat.quads) != 2 {
		t.Fatalf("聊天面板 quad=%d，想要输入加历史共 2", len(chat.quads))
	}
	if chat.quads[0].Color != panelShadow {
		t.Fatalf("聊天输入背衬=%v，想要令牌 panelShadow", chat.quads[0].Color)
	}
	if chat.quads[1].Color != panelSurface {
		t.Fatalf("聊天历史背衬=%v，想要令牌 panelSurface", chat.quads[1].Color)
	}
}

// TestProgressColorsComeFromTokens 钉住耐久条、进食条与容器来源高亮的颜色
// 全部来自 style.go 语义令牌：条形几何由既有测试守护，这里只锁颜色来源。
func TestProgressColorsComeFromTokens(t *testing.T) {
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	var worn hotbarLayout
	appendDurabilityBar(&worn, 0, core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full / 2}, 1920, 1080)
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
	appendDurabilityBar(&low, 0, core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 1}, 1920, 1080)
	if low.quads[1].Color != durabilityLowColor {
		t.Fatalf("低耐久填充=%v，想要令牌 %v", low.quads[1].Color, durabilityLowColor)
	}

	var eating hotbarLayout
	appendEatingBar(&eating, EatingOverlay{Active: true, Progress: 0.5}, MiningOverlay{}, 1280, 720)
	if len(eating.quads) != 2 {
		t.Fatalf("进食条 quad=%d，想要轨道加填充共 2", len(eating.quads))
	}
	if eating.quads[0].Color != miningTrackColor {
		t.Fatalf("进食轨道色=%v，想要与采掘共用令牌", eating.quads[0].Color)
	}
	if eating.quads[1].Color != eatingFillColor {
		t.Fatalf("进食填充色=%v，想要令牌 %v", eating.quads[1].Color, eatingFillColor)
	}

	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, core.InventorySlots, nil, nil, fullChestOverlay(),
		MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: false}, 1280, 800)
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

// TestAppendCrosshairRejectsDegenerateInput 直接命中 `appendCrosshair` 的
// 门控：隐藏、零宽与零高 framebuffer 都必须零实例，正尺寸恰好产生投影加
// 前景共 4 个实例。这是对呈现入口最短路径的直达测试，不经过完整布局。
func TestAppendCrosshairRejectsDegenerateInput(t *testing.T) {
	for _, test := range []struct {
		name          string
		overlay       CrosshairOverlay
		width, height float32
		wantQuads     int
	}{
		{"隐藏", CrosshairOverlay{Visible: false}, 1280, 800, 0},
		{"零宽", CrosshairOverlay{Visible: true}, 0, 800, 0},
		{"零高", CrosshairOverlay{Visible: true}, 1280, 0, 0},
		{"负宽", CrosshairOverlay{Visible: true}, -1, 800, 0},
		{"可见", CrosshairOverlay{Visible: true}, 1280, 800, crosshairQuads},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			layout.scale = hudScale(false, test.width, test.height)
			appendCrosshair(&layout, test.overlay, test.width, test.height)
			if len(layout.quads) != test.wantQuads {
				t.Fatalf("%s quads=%d，想要 %d", test.name, len(layout.quads), test.wantQuads)
			}
			if test.wantQuads == 0 {
				return
			}
			for index, quad := range layout.quads {
				want := crosshairFg
				if index < 2 {
					want = crosshairShadow
				}
				if quad.Color != want {
					t.Fatalf("准星臂 %d 颜色=%v，想要 %v", index, quad.Color, want)
				}
			}
		})
	}
}

// 编译期守护：确认本文件消费的呈现接口仍然存在（`render.GlyphSource` 是文字
// 路径的唯一图集来源）。
var _ render.GlyphSource = (*fakeNameTagAtlas)(nil)

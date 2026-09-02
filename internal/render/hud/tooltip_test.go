package hud

import (
	"testing"
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/core"
)

// TestTooltipShowsItemNameAtPointer 锁定 tooltip 的呈现契约：指针悬停非空栏位
// （含产物格）时，在指针右下侧呈现投影加表面双层背景与阴影加前景双层中文名，
// 背景矩形完整包住墨迹并整体位于 framebuffer 内。
func TestTooltipShowsItemNameAtPointer(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	const width, height = float32(1280), float32(800)
	inventory := fullTestInventory()
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemRawIron, Count: 12}
	x, y := inventorySlotOrigin(core.HotbarSlots, width, height)
	tooltip := TooltipOverlay{Valid: true, CursorX: float64(x) + 1, CursorY: float64(y) + 1}

	var layout hotbarLayout
	appendTooltipOverlay(&layout, atlas, tooltip, inventory, nil, nil, nil, width, height)
	if len(layout.quads) != tooltipQuads {
		t.Fatalf("tooltip quads=%d，想要投影加表面共 %d", len(layout.quads), tooltipQuads)
	}
	shadow, surface := layout.quads[0], layout.quads[1]
	if shadow.Color != panelShadow || surface.Color != panelSurface {
		t.Fatalf("tooltip 背景=%+v/%+v，想要 panelShadow/panelSurface", shadow, surface)
	}
	expand := panelShadowExpand * layout.scale
	if shadow.Width != surface.Width+2*expand || shadow.Height != surface.Height+2*expand {
		t.Fatalf("tooltip 投影未按 %v design px 外扩: %+v / %+v", panelShadowExpand, shadow, surface)
	}
	// 指针右下侧：表面左上角在指针的右下方。
	gap := tooltipCursorGap * layout.scale
	if surface.X < x+gap || surface.Y < y+gap {
		t.Fatalf("tooltip 未在指针右下侧: surface=(%v,%v) 指针=(%v,%v)", surface.X, surface.Y, x, y)
	}
	// 双层字形：阴影加前景，文本与 core.ItemDisplayName 同源。前景取面板文字
	// 令牌 `textOnPanelFg`：背景是 `panelSurface` 暖羊皮纸，暖白世界浮层字对比
	// 不足；阴影层仍走 `textPrimaryShadow`。
	wantName, ok := core.ItemDisplayName(core.ItemRawIron)
	if !ok || len(layout.glyphs) != utf8.RuneCountInString(wantName)*2 {
		t.Fatalf("tooltip glyphs=%d，想要「%s」双层共 %d", len(layout.glyphs), wantName, utf8.RuneCountInString(wantName)*2)
	}
	for index, glyph := range layout.glyphs {
		wantColor := textOnPanelFg
		if index < len(layout.glyphs)/2 {
			wantColor = textPrimaryShadow
		}
		if glyph.Color != wantColor {
			t.Fatalf("tooltip 字形 %d 颜色=%v，想要 %v", index, glyph.Color, wantColor)
		}
	}
	// 背景矩形必须包住全部墨迹，且整体位于 framebuffer 内。
	for index, glyph := range layout.glyphs {
		if glyph.X < surface.X || glyph.Y < surface.Y ||
			glyph.X+glyph.Width > surface.X+surface.Width || glyph.Y+glyph.Height > surface.Y+surface.Height {
			t.Fatalf("tooltip 字形 %d 越出背景: %+v / %+v", index, glyph, surface)
		}
		if glyph.X < 0 || glyph.Y < 0 || glyph.X+glyph.Width > width || glyph.Y+glyph.Height > height {
			t.Fatalf("tooltip 字形 %d 越出 framebuffer: %+v", index, glyph)
		}
	}
}

// TestTooltipFlipsAtFramebufferEdge 锁定翻转规则：在宽度约束收口的窗口里，指针
// 悬停配方栏最右入口时 tooltip 右沿越出 framebuffer，翻转到指针左上侧后仍完全
// 位于 framebuffer 内。
func TestTooltipFlipsAtFramebufferEdge(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	// 660×800 让宽度约束成为唯一收口项：面板右沿贴近 framebuffer 右缘。
	const width, height = float32(660), float32(800)
	scale := hudScale(width, height)
	entryX, entryY := recipeButtonOrigin(recipeEntryCount-1, width, height)
	// 指针落在最右一条入口的右沿内侧：它右侧只剩栏间隙与内边距。
	tooltip := TooltipOverlay{
		Valid:   true,
		CursorX: float64(entryX + recipeColumnWidth*scale - 1),
		CursorY: float64(entryY + 1),
	}
	var layout hotbarLayout
	appendTooltipOverlay(&layout, atlas, tooltip, fullTestInventory(), &CraftingOverlay{Size: 2}, nil, nil, width, height)
	if len(layout.quads) != tooltipQuads {
		t.Fatalf("tooltip quads=%d，想要 %d", len(layout.quads), tooltipQuads)
	}
	surface := layout.quads[1]
	if surface.X > float32(tooltip.CursorX) || surface.Y > float32(tooltip.CursorY) {
		t.Fatalf("tooltip 未翻转到指针左上: surface=(%v,%v) 指针=(%v,%v)",
			surface.X, surface.Y, tooltip.CursorX, tooltip.CursorY)
	}
	if surface.X < 0 || surface.Y < 0 || surface.X+surface.Width > width || surface.Y+surface.Height > height {
		t.Fatalf("翻转后的 tooltip 越出 framebuffer: %+v", surface)
	}
}

// TestTooltipZeroInstancesForEmptyOrOutsideOrClosed 锁定零实例门控：空栏位、
// 面板外指针、未确认打开与零尺寸 framebuffer 都不产生任何 quad 或字形。
func TestTooltipZeroInstancesForEmptyOrOutsideOrClosed(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	const width, height = float32(1280), float32(800)
	inventory := fullTestInventory()
	// 背包 1 号格清空，其余格全满：悬停它不得产生实例。
	inventory.Backpack[1] = core.ItemStack{}
	emptyX, emptyY := inventorySlotOrigin(core.HotbarSlots+1, width, height)
	outside := TooltipOverlay{Valid: true, CursorX: 3, CursorY: 3}
	closed := TooltipOverlay{Valid: false, CursorX: float64(emptyX) + 1, CursorY: float64(emptyY) + 1}
	for _, test := range []struct {
		name    string
		tooltip TooltipOverlay
	}{
		{"空栏位", TooltipOverlay{Valid: true, CursorX: float64(emptyX) + 1, CursorY: float64(emptyY) + 1}},
		{"面板外", outside},
		{"未打开", closed},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			appendTooltipOverlay(&layout, atlas, test.tooltip, inventory, nil, nil, nil, width, height)
			if len(layout.quads) != 0 || len(layout.glyphs) != 0 {
				t.Fatalf("%s 产生了实例: quads=%d glyphs=%d", test.name, len(layout.quads), len(layout.glyphs))
			}
		})
	}
	var layout hotbarLayout
	appendTooltipOverlay(&layout, atlas, TooltipOverlay{Valid: true, CursorX: 1, CursorY: 1},
		inventory, nil, nil, nil, 0, height)
	if len(layout.quads) != 0 || len(layout.glyphs) != 0 {
		t.Fatalf("零宽 framebuffer 产生了实例: quads=%d glyphs=%d", len(layout.quads), len(layout.glyphs))
	}
}

// TestTooltipTextMatchesRegistryDisplayName 锁定同源性：tooltip 的文本与字形
// 都来自 `core.ItemDisplayName`，按书写顺序逐 rune 取自同一批 atlas cell。
func TestTooltipTextMatchesRegistryDisplayName(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	const width, height = float32(1280), float32(800)
	inventory := fullTestInventory()
	// 石镐是带耐久物品：合法栈必须携带 1..max 的耐久值，否则镜像整体失效。
	fullDurability, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: fullDurability}
	x, y := inventorySlotOrigin(core.HotbarSlots, width, height)
	tooltip := TooltipOverlay{Valid: true, CursorX: float64(x) + 1, CursorY: float64(y) + 1}

	var layout hotbarLayout
	appendTooltipOverlay(&layout, atlas, tooltip, inventory, nil, nil, nil, width, height)
	name, ok := core.ItemDisplayName(core.ItemStonePickaxe)
	if !ok {
		t.Fatal("石镐缺显示名")
	}
	runes := []rune(name)
	// 阴影层在前、前景层在后，各 rune 数量与显示名一致。
	if len(layout.glyphs) != len(runes)*2 {
		t.Fatalf("tooltip 字形=%d，想要「%s」双层共 %d", len(layout.glyphs), name, len(runes)*2)
	}
	for pass := range 2 {
		for index, char := range runes {
			glyph := layout.glyphs[pass*len(runes)+index]
			want := atlas.Glyph(char)
			if glyph.U0 != want.U0 || glyph.V0 != want.V0 || glyph.U1 != want.U1 || glyph.V1 != want.V1 {
				t.Fatalf("pass %d rune %d UV=%v，想要注册表显示名 cell %v", pass, index,
					[4]float32{glyph.U0, glyph.V0, glyph.U1, glyph.V1},
					[4]float32{want.U0, want.V0, want.U1, want.V1})
			}
		}
	}
}

// TestTooltipCoversFurnaceOutputAndRecipeEntries 补全悬停覆盖：熔炉产物格与
// 配方入口的产物都在 tooltip 语义内。
func TestTooltipCoversFurnaceOutputAndRecipeEntries(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	const width, height = float32(1280), float32(800)
	overlay := fullFurnaceOverlay()
	x, y := furnaceSlotOrigin(2, width, height)
	tooltip := TooltipOverlay{Valid: true, CursorX: float64(x) + 1, CursorY: float64(y) + 1}
	var layout hotbarLayout
	appendTooltipOverlay(&layout, atlas, tooltip, core.Inventory{}, nil, overlay, nil, width, height)
	if len(layout.glyphs) == 0 {
		t.Fatal("熔炉产物格未产生 tooltip 字形")
	}

	recipeX, recipeY := recipeButtonOrigin(0, width, height)
	recipeTooltip := TooltipOverlay{Valid: true, CursorX: float64(recipeX) + 1, CursorY: float64(recipeY) + 1}
	var recipeLayout hotbarLayout
	appendTooltipOverlay(&recipeLayout, atlas, recipeTooltip, core.Inventory{}, &CraftingOverlay{Size: 2}, nil, nil, width, height)
	if len(recipeLayout.glyphs) == 0 {
		t.Fatal("配方入口未产生 tooltip 字形")
	}
}

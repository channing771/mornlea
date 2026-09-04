package hud

import (
	"testing"
	"unicode/utf8"

	"github.com/channing771/mornlea/packages/shared/core"
)

// panelTestViews 枚举四类容器视图与各自的非背包命中清单：测试与实现共用同一组
// 视图构造，保证穷举范围与真实打开路径一致。
type panelTestCase struct {
	name     string
	view     containerView
	crafting *CraftingOverlay
	overlay  *FurnaceOverlay
	chest    *ChestOverlay
	// recipeColumn 标记个人合成面板因右侧配方栏而加宽。
	recipeColumn bool
	// extraHits 返回该视图在 36 个统一栏位之外的格（下标, origin）。
	extraHits func(t *testing.T, width, height float32) []panelHit
}

type panelHit struct {
	slot int
	x, y float32
}

func panelTestCases() []panelTestCase {
	return []panelTestCase{
		{
			name: "个人合成", view: containerViewCrafting, crafting: &CraftingOverlay{Size: 2},
			extraHits: func(t *testing.T, width, height float32) []panelHit {
				hits := []panelHit{}
				for slot := range 4 {
					x, y := craftingGridSlotOrigin(slot, 2, width, height)
					hits = append(hits, panelHit{slot, x, y})
				}
				x, y := craftingOutputOrigin(2, width, height)
				return append(hits, panelHit{-1, x, y})
			},
			// 个人合成面板因右侧配方栏更宽。
			recipeColumn: true,
		},
		{
			name: "工作台", view: containerViewCrafting, crafting: &CraftingOverlay{Size: 3},
			extraHits: func(t *testing.T, width, height float32) []panelHit {
				hits := []panelHit{}
				for slot := range core.CraftingGridSlots {
					x, y := craftingGridSlotOrigin(slot, 3, width, height)
					hits = append(hits, panelHit{slot, x, y})
				}
				x, y := craftingOutputOrigin(3, width, height)
				return append(hits, panelHit{-1, x, y})
			},
			recipeColumn: true,
		},
		{
			name: "熔炉", view: containerViewFurnace, overlay: &FurnaceOverlay{},
			extraHits: func(t *testing.T, width, height float32) []panelHit {
				hits := []panelHit{}
				for index := range 3 {
					x, y := furnaceSlotOrigin(index, width, height)
					hits = append(hits, panelHit{core.InventorySlots + index, x, y})
				}
				return hits
			},
		},
		{
			name: "箱子", view: containerViewChest, chest: &ChestOverlay{},
			extraHits: func(t *testing.T, width, height float32) []panelHit {
				hits := []panelHit{}
				for index := range core.ChestSlots {
					x, y := chestSlotOrigin(index, width, height)
					hits = append(hits, panelHit{core.InventorySlots + index, x, y})
				}
				return hits
			},
		},
	}
}

// TestContainerPanelFamilyIsSingleSource 钉住面板族的构成与单源性质：四类面板
// 恰好追加投影、表面、四边 1 design px 亮边与一个标题 quad；四类面板共用同一
// 原点（箱子/熔炉与个人面板左沿对齐，配方栏向右展开），全部栏位原点都落在
// 面板矩形内部。
func TestContainerPanelFamilyIsSingleSource(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	const width, height = float32(1280), float32(800)
	var layout hotbarLayout
	wantOriginX, wantOriginY := panelOrigin(width, height,
		containerPanelWidth*hudScale(width, height),
		containerPanelHeight*hudScale(width, height),
		openBottomStackTop(width, height))
	for _, test := range panelTestCases() {
		t.Run(test.name, func(t *testing.T) {
			got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1,
				test.crafting, test.overlay, test.chest, width, height)
			// 面板族是布局最先追加的 containerPanelQuads 个实例：投影、表面与四边亮边。
			family := got.quads[:containerPanelQuads-1]
			expand := panelShadowExpand * got.scale
			edge := panelBorderWidth * got.scale
			wantPanelWidth := containerPanelWidth * got.scale
			if test.recipeColumn {
				wantPanelWidth = recipePanelWidth * got.scale
			}
			want := []hotbarInstance{
				{X: wantOriginX - expand, Y: wantOriginY - expand,
					Width: wantPanelWidth + 2*expand, Height: containerPanelHeight*got.scale + 2*expand,
					Color: panelShadow},
				{X: wantOriginX, Y: wantOriginY,
					Width: wantPanelWidth, Height: containerPanelHeight * got.scale,
					Color: panelSurface},
				{X: wantOriginX, Y: wantOriginY, Width: wantPanelWidth, Height: edge,
					Color: panelBorderLight},
				{X: wantOriginX, Y: wantOriginY + containerPanelHeight*got.scale - edge,
					Width: wantPanelWidth, Height: edge, Color: panelBorderLight},
				{X: wantOriginX, Y: wantOriginY, Width: edge, Height: containerPanelHeight * got.scale,
					Color: panelBorderLight},
				{X: wantOriginX + wantPanelWidth - edge, Y: wantOriginY,
					Width: edge, Height: containerPanelHeight * got.scale, Color: panelBorderLight},
			}
			for index, wantQuad := range want {
				if family[index] != wantQuad {
					t.Fatalf("面板族 %d = %+v，想要 %+v", index, family[index], wantQuad)
				}
			}
			// 标题恰一个，位于面板顶部共享内容列的左沿。
			title := got.quads[containerPanelQuads-1]
			wantUV := hotbarTextureUV(containerTitleColumn(test.view))
			if gotUV := [4]float32{title.U0, title.V0, title.U1, title.V1}; gotUV != wantUV {
				t.Fatalf("标题 UV=%v，想要列 cell %v", gotUV, wantUV)
			}
			if title.X != wantOriginX+hotbarPanelPadding*got.scale ||
				title.Y != wantOriginY+containerTitleGap*got.scale ||
				title.Width != containerTitleSize*got.scale || title.Height != containerTitleSize*got.scale {
				t.Fatalf("标题=%+v，未落在面板顶部内容列左沿", title)
			}
			// 个人面板向右展开配方栏，其余三类面板宽度相同：左沿必须一致。
			frame := openContainerPanel(test.view, width, height)
			if frame.x != wantOriginX || frame.y != wantOriginY {
				t.Fatalf("面板原点=(%v,%v)，想要 panelOrigin 推导的 (%v,%v)", frame.x, frame.y, wantOriginX, wantOriginY)
			}
		})
	}
}

// TestContainerSlotOriginsStayInsidePanel 穷举四类面板的全部可交互矩形：36 个
// 统一栏位、图示区格、产物格与十条配方入口都必须落在面板矩形内部且互不重叠。
func TestContainerSlotOriginsStayInsidePanel(t *testing.T) {
	const width, height = float32(1280), float32(800)
	scale := hudScale(width, height)
	slotSize := hotbarSlotSize * scale
	for _, test := range panelTestCases() {
		t.Run(test.name, func(t *testing.T) {
			frame := openContainerPanel(test.view, width, height)
			cells := []hotbarInstance{}
			for slot := range core.InventorySlots {
				x, y := inventorySlotOrigin(slot, width, height)
				cells = append(cells, hotbarInstance{X: x, Y: y, Width: slotSize, Height: slotSize})
			}
			for _, hit := range test.extraHits(t, width, height) {
				cells = append(cells, hotbarInstance{X: hit.x, Y: hit.y, Width: slotSize, Height: slotSize})
			}
			if test.view == containerViewCrafting {
				for row := range recipeEntryCount {
					x, y := recipeButtonOrigin(row, width, height)
					cells = append(cells, hotbarInstance{
						X: x, Y: y, Width: recipeColumnWidth * scale, Height: recipeEntryHeight * scale,
					})
				}
			}
			for index, cell := range cells {
				if cell.X < frame.x || cell.Y < frame.y ||
					cell.X+cell.Width > frame.x+frame.width || cell.Y+cell.Height > frame.y+frame.height {
					t.Fatalf("格 %d 越出面板: cell=%+v panel=(%v,%v,%v,%v)",
						index, cell, frame.x, frame.y, frame.width, frame.height)
				}
				for other := index + 1; other < len(cells); other++ {
					if rectanglesIntersect(cell, cells[other]) {
						t.Fatalf("格 %d 与格 %d 相交: %+v / %+v", index, other, cell, cells[other])
					}
				}
			}
		})
	}
}

// TestContainerPanelAvoidsBottomReservedStatusSpace 钉住状态栈预留：两行状态行
// 已迁 WebView 组件，但打开态面板（含外扩投影）仍以下方状态栈上沿为居中下界，
// 面板矩形必须落在该上沿之上，任何尺寸 framebuffer 内都不得侵入预留区。
func TestContainerPanelAvoidsBottomReservedStatusSpace(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, size := range [][2]float32{{1280, 800}, {640, 360}, {240, 40}, {800, 17}} {
		width, height := size[0], size[1]
		var layout hotbarLayout
		got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, &ChestOverlay{}, width, height)
		stackTop := openBottomStackTop(width, height)
		for _, quad := range got.quads[:containerPanelQuads] {
			if quad.Y+quad.Height > stackTop {
				t.Fatalf("framebuffer %v 面板族侵入底部状态栈预留区: %+v（上沿 %v）",
					size, quad, stackTop)
			}
		}
	}
}

// TestContainerPanelViewsShareSlotGeometry 钉住视图无关性：面板垂直原点只依赖
// framebuffer 与统一面板高度，四类视图之间切换时 36 个统一栏位的命中矩形
// 不得漂移。
func TestContainerPanelViewsShareSlotGeometry(t *testing.T) {
	const width, height = float32(1280), float32(800)
	for slot := range core.InventorySlots {
		wantX, wantY := inventorySlotOrigin(slot, width, height)
		for _, test := range panelTestCases() {
			frame := openContainerPanel(test.view, width, height)
			x := frame.contentLeft + float32(slot%core.HotbarSlots)*(hotbarSlotSize+hotbarSlotGap)*frame.scale
			y := wantY
			if slot >= core.HotbarSlots {
				continue
			}
			if x != wantX || y != frame.hotbarY {
				t.Fatalf("视图 %s 栏位 %d=(%v,%v)，想要共享面板几何 (%v,%v)",
					test.name, slot, x, y, wantX, frame.hotbarY)
			}
		}
	}
}

// TestRecipeButtonAtCoversTenEntries 锁定右侧配方栏的命中契约：十条入口自上而
// 下、左上闭右下开，命中的配方 ID 与既有固定配方表逐一对应；栏间空隙不命中。
func TestRecipeButtonAtCoversTenEntries(t *testing.T) {
	const width, height = float32(1280), float32(800)
	if recipeEntryCount != len(inventoryRecipeIDs) {
		t.Fatalf("配方入口数=%d，想要与固定配方表一致 %d", recipeEntryCount, len(inventoryRecipeIDs))
	}
	scale := hudScale(width, height)
	for row, recipe := range inventoryRecipeIDs {
		x, y := recipeButtonOrigin(row, width, height)
		got, ok := RecipeButtonAt(float64(x)+1, float64(y)+1, uint32(width), uint32(height))
		if !ok || got != recipe {
			t.Fatalf("入口 %d 命中 = %d, %v，想要配方 %d", row, got, ok, recipe)
		}
		if _, ok := RecipeButtonAt(float64(x+recipeColumnWidth*scale), float64(y+recipeEntryHeight*scale/2), uint32(width), uint32(height)); ok {
			t.Fatalf("入口 %d 右边界外仍被命中", row)
		}
		if _, ok := RecipeButtonAt(float64(x+recipeColumnWidth*scale/2), float64(y+recipeEntryHeight*scale), uint32(width), uint32(height)); ok {
			t.Fatalf("入口 %d 下边界外仍被命中", row)
		}
	}
	if _, ok := RecipeButtonAt(1, 1, uint32(width), uint32(height)); ok {
		t.Fatal("面板外命中被接受")
	}
	if _, ok := RecipeButtonAt(1, 1, 0, uint32(height)); ok {
		t.Fatal("零宽 framebuffer 命中被接受")
	}
}

// TestOpenWorstQuadBudgetIsFullyAccounted 是打开最坏固定预算的实算断言：箱子
// 视图取合法最坏组合（面板族、选中与来源、36 格双层物品、九条耐久、满 27 格
// 箱子与悬停 tooltip 背景），精确构成以命名常量钉住，任何一环增减都必须显式
// 重审本值与固定容量。常显层退役后关闭态零实例，打开态 218 是 GPU 保留面唯一
// 的最坏分支。
func TestOpenWorstQuadBudgetIsFullyAccounted(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, maxQuadTestInventory(), true, 5, nil, nil, fullChestOverlay(), 1280, 800)
	appendTooltipOverlay(&got, atlas, tooltipAtSlot(), fullTestInventory(), nil, nil, fullChestOverlay(), 1280, 800)

	// 实算分解：面板族 7 + 选中 1 + 来源 1 + 36 格 + 72 色块 + 18 耐久
	// + 箱子内容 81 + tooltip 背景 2 = 218。
	const openWorstQuads = containerPanelQuads + 2 +
		core.InventorySlots + core.InventorySlots*2 + core.HotbarSlots*2 +
		chestContentQuads + tooltipQuads
	if len(got.quads) != openWorstQuads {
		t.Fatalf("打开最坏 quads=%d，想要实算 %d", len(got.quads), openWorstQuads)
	}
	if openWorstQuads != 218 {
		t.Fatalf("打开最坏钉值=%d，想要实算 218", openWorstQuads)
	}
	if openInventoryQuads != containerPanelQuads+2+core.InventorySlots+
		core.InventorySlots*2+core.HotbarSlots*2+maxOverlayQuads+tooltipQuads {
		t.Fatal("openInventoryQuads 公式与实算分解不一致")
	}
	if len(got.quads) > maxHotbarQuads {
		t.Fatalf("打开最坏 quads=%d 超出固定容量 %d", len(got.quads), maxHotbarQuads)
	}
}

// TestOpenWorstGlyphBudgetIncludesTooltip 锁定 glyph 最坏：36 格两位数量与满箱
// 两位数量之上再叠加悬停 tooltip 的双层字形，仍不超过固定 768。聊天与弹条字形
// 流随常显层退役，分支预算由增长余量吸收，总上限不变。
func TestOpenWorstGlyphBudgetIncludesTooltip(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	// 「损坏的石镐」是注册表内最长显示名（5 rune），双层共 10 个字形。
	name := "损坏的石镐"
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, fullTestInventory(), true, 5, nil, nil, fullChestOverlay(), 1280, 800)
	// 悬停箱子 0 号格，格内放注册表最长显示名的物品，见证 tooltip 双层字形最坏。
	longNameChest := fullChestOverlay()
	longNameChest.Items[0] = core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: 1}
	hoverX, hoverY := chestSlotOrigin(0, 1280, 800)
	appendTooltipOverlay(&got, atlas, TooltipOverlay{
		Valid: true, CursorX: float64(hoverX) + 1, CursorY: float64(hoverY) + 1,
	}, fullTestInventory(), nil, nil, longNameChest, 1280, 800)

	tooltipWorst := utf8.RuneCountInString(name) * 2
	if got := len(got.glyphs); got != core.InventorySlots*4+chestGlyphs+tooltipWorst {
		t.Fatalf("打开最坏 glyphs=%d，想要 %d", got,
			core.InventorySlots*4+chestGlyphs+tooltipWorst)
	}
	// 预算按 8 rune 截断上限封顶（tooltipGlyphs=16），注册表实测见证 262。
	if want := core.InventorySlots*4 + chestGlyphs + tooltipGlyphs; want != 268 {
		t.Fatalf("打开态 glyph 预算=%d，想要钉值 268", want)
	}
	if len(got.glyphs) > maxHotbarGlyphs {
		t.Fatalf("glyph 最坏=%d 超出固定容量 %d", len(got.glyphs), maxHotbarGlyphs)
	}
	// 固定 glyph 上限 768 与 offset/总容量由 renderer_test.go 钉住；这里只钉
	// 预算不超上限。
	if maxHotbarGlyphs != 768 {
		t.Fatalf("glyph 固定上限=%d，想要 768", maxHotbarGlyphs)
	}
}

// tooltipAtSlot 返回一个悬停在箱子 0 号格内的 tooltip 输入。
func tooltipAtSlot() TooltipOverlay {
	x, y := chestSlotOrigin(0, 1280, 800)
	return TooltipOverlay{Valid: true, CursorX: float64(x) + 1, CursorY: float64(y) + 1}
}

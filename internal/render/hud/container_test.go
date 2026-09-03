package hud

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 杀死变异：任一容器分支若继续使用纯色栏位、采样到错误 cell 或覆盖旧 item tile，
// 同一套原创凹槽将无法统一 36/39/63 格与合成网格、产物格。
func TestContainerPixelCellsUseSharedSlotUV(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	const width, height = float32(1280), float32(800)
	wantUV := hotbarTextureUV(hotbarContainerSlotColumn)
	wantColor := [4]float32{1, 1, 1, 1}

	assertSlot := func(t *testing.T, layout hotbarLayout, x, y float32) {
		t.Helper()
		for _, quad := range layout.quads {
			if quad.X == x && quad.Y == y && quad.Width == hotbarSlotSize*layout.scale && quad.Height == hotbarSlotSize*layout.scale {
				gotUV := [4]float32{quad.U0, quad.V0, quad.U1, quad.V1}
				if gotUV != wantUV || quad.Color != wantColor {
					t.Fatalf("栏位 (%f,%f)=%+v，想要凹槽 UV=%v", x, y, quad, wantUV)
				}
				return
			}
		}
		t.Fatalf("没有栏位 quad (%f,%f)", x, y)
	}

	var layout hotbarLayout
	workbench := &CraftingOverlay{Size: 3}
	for _, view := range []struct {
		name      string
		crafting  *CraftingOverlay
		overlay   *FurnaceOverlay
		chest     *ChestOverlay
		gridSlots int
	}{
		{"合成", workbench, nil, nil, 9},
		{"熔炉", nil, &FurnaceOverlay{}, nil, 0},
		{"箱子", nil, nil, &ChestOverlay{}, 0},
	} {
		t.Run(view.name, func(t *testing.T) {
			got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, view.crafting, view.overlay, view.chest, width, height)
			for slot := range core.InventorySlots {
				x, y := inventorySlotOrigin(slot, width, height)
				assertSlot(t, got, x, y)
			}
			switch {
			case view.chest != nil:
				for slot := range core.ChestSlots {
					x, y := chestSlotOrigin(slot, width, height)
					assertSlot(t, got, x, y)
				}
			case view.overlay != nil:
				for slot := range 3 {
					x, y := furnaceSlotOrigin(slot, width, height)
					assertSlot(t, got, x, y)
				}
			default:
				for slot := range view.gridSlots {
					x, y := craftingGridSlotOrigin(slot, 3, width, height)
					assertSlot(t, got, x, y)
				}
				outputX, outputY := craftingOutputOrigin(3, width, height)
				assertSlot(t, got, outputX, outputY)
			}
		})
	}
}

// TestContainerTitlesUseAtlasCells 锁定每个互斥 overlay 只追加一个 atlas 标题，
// 标题位于面板顶部共享内容列左沿，并且标题不借用动态 glyph 流。
func TestContainerTitlesUseAtlasCells(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	const width, height = float32(1280), float32(800)
	for _, test := range []struct {
		name     string
		crafting *CraftingOverlay
		overlay  *FurnaceOverlay
		chest    *ChestOverlay
		column   int
		glyphs   int
	}{
		// 合成视图的 glyph 来自右侧配方栏三条数量为 4 的产物（双层）。
		{"合成", &CraftingOverlay{Size: 3}, nil, nil, hotbarCraftingTitleColumn, 6},
		{"熔炉", nil, &FurnaceOverlay{}, nil, hotbarFurnaceTitleColumn, 0},
		{"箱子", nil, nil, &ChestOverlay{}, hotbarChestTitleColumn, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, test.crafting, test.overlay, test.chest, width, height)
			wantUV := hotbarTextureUV(test.column)
			frame := openPanelAnchor(width, height)
			count := 0
			for _, quad := range got.quads {
				if [4]float32{quad.U0, quad.V0, quad.U1, quad.V1} != wantUV {
					continue
				}
				count++
				if quad.X != frame.contentLeft ||
					quad.Y != frame.y+containerTitleGap*frame.scale ||
					quad.Width != containerTitleSize*frame.scale || quad.Height != containerTitleSize*frame.scale {
					t.Fatalf("标题=%+v，未放在面板顶部内容列左沿", quad)
				}
			}
			if count != 1 {
				t.Fatalf("标题 quad=%d，想要 1", count)
			}
			if len(got.glyphs) != test.glyphs {
				t.Fatalf("标题意外进入 glyph 流：glyphs=%d，想要 %d", len(got.glyphs), test.glyphs)
			}
		})
	}
}

// TestCraftingOverlayDrawsPersonalAndWorkbenchGrids 锁定合成区的组成：个人面板
// 恰好 2×2、工作台恰好 3×3，产物格带琥珀轮廓底衬与静态箭头，右侧配方栏恒为
// 十条入口；个人扩展格 4..8 不画，全部网格格与产物格画物品双层色块与数量。
func TestCraftingOverlayDrawsPersonalAndWorkbenchGrids(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	const width, height = float32(1280), float32(800)

	personal := &CraftingOverlay{Size: 2}
	personal.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 12}
	personal.Slots[3] = core.ItemStack{Item: core.ItemOakLog, Count: 1}
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, personal, nil, nil, width, height)
	// 面板族、选中框、36 格，加合成区内容（轮廓、5 个凹槽、2 个双层色块、
	// 箭头）与十条配方入口。
	if len(got.quads) != containerPanelQuads+1+core.InventorySlots+11+recipeColumnQuads {
		t.Fatalf("个人网格 quads=%d，想要基础 %d 加合成内容 11 与配方栏 %d",
			len(got.quads), containerPanelQuads+1+core.InventorySlots, recipeColumnQuads)
	}
	for slot := range 4 {
		x, y := craftingGridSlotOrigin(slot, 2, width, height)
		if !hasQuadAt(got.quads, x, y) {
			t.Fatalf("个人网格格 %d 未绘制在 (%f,%f)", slot, x, y)
		}
	}
	// 个人视图只覆盖左上 2×2 区域（row 0 在上）：3×3 坐标里的右列与底行
	//（格 2、5、6、7、8）一律不画；个人格 0..3 与 3×3 格 0、1、3、4 同位。
	for _, slot := range []int{2, 5, 6, 7, 8} {
		x, y := craftingGridSlotOrigin(slot, 3, width, height)
		if hasQuadAt(got.quads, x, y) {
			t.Fatalf("个人视图画出了 3×3 专属格 %d", slot)
		}
	}
	// 个人网格的数量 12 出 4 个字形；配方栏三条数量为 4 的产物各出 2 个，共 10。
	if len(got.glyphs) != 10 {
		t.Fatalf("个人网格数字=%d，想要网格 4 加配方栏 6 共 10", len(got.glyphs))
	}

	workbench := &CraftingOverlay{Size: 3}
	for slot := range core.CraftingGridSlots {
		workbench.Slots[slot] = core.ItemStack{Item: core.ItemStone, Count: 64}
	}
	// 产物取两位数量：glyph 容量按 10 个格全部两位数字锁定（craftingGlyphs）。
	workbench.Output = core.ItemStack{Item: core.ItemStoneBrick, Count: 12}
	got = layoutInventory(&layout, atlas, core.Inventory{}, true, -1, workbench, nil, nil, width, height)
	// 面板族 + 选中框 + 36 格，加合成内容 32（轮廓 + 10 凹槽 + 20 色块 +
	// 箭头）与配方栏 30。
	if len(got.quads) != containerPanelQuads+1+core.InventorySlots+craftingContentQuads+recipeColumnQuads {
		t.Fatalf("满工作台 quads=%d，想要基础 %d 加合成内容 %d 与配方栏 %d", len(got.quads),
			containerPanelQuads+1+core.InventorySlots, craftingContentQuads, recipeColumnQuads)
	}
	if craftingContentQuads != 32 || craftingGlyphs != 40 {
		t.Fatalf("合成内容容量 quads/glyphs=%d/%d，想要 32/40", craftingContentQuads, craftingGlyphs)
	}
	for slot := range core.CraftingGridSlots {
		x, y := craftingGridSlotOrigin(slot, 3, width, height)
		if !hasQuadAt(got.quads, x, y) {
			t.Fatalf("工作台格 %d 未绘制", slot)
		}
	}
	outputX, outputY := craftingOutputOrigin(3, width, height)
	if !hasQuadAt(got.quads, outputX, outputY) {
		t.Fatalf("产物格未绘制在 (%f,%f)", outputX, outputY)
	}
	if len(got.glyphs) != craftingGlyphs+6 {
		t.Fatalf("满工作台数字=%d，想要网格 %d 加配方栏 6", len(got.glyphs), craftingGlyphs)
	}

	// 空产物格只画凹槽不画色块：产物格内容由权威镜像决定，客户端不预测。
	empty := &CraftingOverlay{Size: 2}
	empty.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
	got = layoutInventory(&layout, atlas, core.Inventory{}, true, -1, empty, nil, nil, width, height)
	outputX, outputY = craftingOutputOrigin(2, width, height)
	for _, quad := range got.quads {
		if quad.X == outputX && quad.Y == outputY && quad.Color != ([4]float32{1, 1, 1, 1}) {
			t.Fatalf("空产物格画出了非凹槽内容: %+v", quad)
		}
	}
	// 产物格轮廓底衬取麦金强调：产物属于 `accentProgress` 语义族。
	expand := craftingOutputOutlineExpand * got.scale
	foundOutline := false
	for _, quad := range got.quads {
		if quad.X == outputX-expand && quad.Y == outputY-expand && quad.Color == accentProgress {
			foundOutline = true
			break
		}
	}
	if !foundOutline {
		t.Fatal("产物格缺少麦金轮廓底衬")
	}
}

func hasQuadAt(quads []hotbarInstance, x, y float32) bool {
	for _, quad := range quads {
		if quad.X == x && quad.Y == y && quad.Width > 0 && quad.Height > 0 {
			return true
		}
	}
	return false
}

// TestCraftingSlotAtCoversUnifiedIndices 锁定合成视图的统一命中：网格 0..8、
// 背包 9..44；个人尺寸 2 时扩展格 4..8 永不命中；边界左上闭、右下开。
func TestCraftingSlotAtCoversUnifiedIndices(t *testing.T) {
	for _, test := range []struct {
		size int
	}{
		{2}, {3},
	} {
		width, height := uint32(1280), uint32(800)
		gridExtent := test.size * test.size
		for slot := range gridExtent {
			x, y := craftingGridSlotOrigin(slot, test.size, float32(width), float32(height))
			got, ok := CraftingSlotAt(float64(x), float64(y), width, height, test.size)
			if !ok || got != uint8(slot) {
				t.Fatalf("尺寸 %d 网格格 %d 命中 = %d, %v", test.size, slot, got, ok)
			}
			if _, ok := CraftingSlotAt(
				float64(x+hotbarSlotSize), float64(y+hotbarSlotSize/2), width, height, test.size,
			); ok {
				t.Fatalf("尺寸 %d 网格格 %d 右边界外仍被命中", test.size, slot)
			}
		}
		// 个人视图只覆盖左上 2×2（row 0 在上）：3×3 坐标里的右列与底行
		//（格 2、5、6、7、8）既不画也不命中。
		if test.size == 2 {
			for _, slot := range []int{2, 5, 6, 7, 8} {
				x, y := craftingGridSlotOrigin(slot, 3, float32(width), float32(height))
				if _, ok := CraftingSlotAt(float64(x)+1, float64(y)+1, width, height, 2); ok {
					t.Fatalf("个人专属扩展位置 %d 被命中", slot)
				}
			}
		}
		// 背包 9..44 与背包命中一致偏移 9。
		for _, slot := range []int{0, 8, 9, 35} {
			x, y := inventorySlotOrigin(slot, float32(width), float32(height))
			got, ok := CraftingSlotAt(float64(x)+1, float64(y)+1, width, height, test.size)
			if !ok || int(got) != slot+core.CraftingGridSlots {
				t.Fatalf("尺寸 %d 背包格 %d 命中 = %d, %v，想要 %d", test.size, slot, got, ok, slot+core.CraftingGridSlots)
			}
		}
		if _, ok := CraftingSlotAt(0, 0, width, height, test.size); ok {
			t.Fatal("界外命中被接受")
		}
		if _, ok := CraftingSlotAt(100, 100, 0, 0, test.size); ok {
			t.Fatal("零尺寸 framebuffer 被判为命中")
		}
	}
}

// TestCraftingOutputAtMatchesDrawnGeometry 锁定产物格命中与绘制矩形一致，且
// 产物格不是普通移动目标：CraftingSlotAt 永不返回产物格区域。
func TestCraftingOutputAtMatchesDrawnGeometry(t *testing.T) {
	width, height := uint32(1280), uint32(800)
	for _, size := range []int{2, 3} {
		left, top := craftingOutputOrigin(size, float32(width), float32(height))
		if !CraftingOutputAt(float64(left)+1, float64(top)+1, width, height, size) {
			t.Fatalf("尺寸 %d 产物格内点未命中", size)
		}
		if CraftingOutputAt(float64(left+hotbarSlotSize), float64(top+hotbarSlotSize/2), width, height, size) {
			t.Fatalf("尺寸 %d 产物格右边界外仍被命中", size)
		}
		if CraftingOutputAt(float64(left+hotbarSlotSize/2), float64(top+hotbarSlotSize), width, height, size) {
			t.Fatalf("尺寸 %d 产物格下边界外仍被命中", size)
		}
		if _, ok := CraftingSlotAt(float64(left)+1, float64(top)+1, width, height, size); ok {
			t.Fatalf("尺寸 %d 产物格被当成普通移动目标命中", size)
		}
		// 产物格与全部网格格、配方栏入口互不相交。
		for slot := range size * size {
			gridX, gridY := craftingGridSlotOrigin(slot, size, float32(width), float32(height))
			if rectanglesIntersect(
				hotbarInstance{X: left, Y: top, Width: hotbarSlotSize, Height: hotbarSlotSize},
				hotbarInstance{X: gridX, Y: gridY, Width: hotbarSlotSize, Height: hotbarSlotSize},
			) {
				t.Fatalf("尺寸 %d 产物格与网格格 %d 相交", size, slot)
			}
		}
		for row := range recipeEntryCount {
			buttonX, buttonY := recipeButtonOrigin(row, float32(width), float32(height))
			if rectanglesIntersect(
				hotbarInstance{X: left, Y: top, Width: hotbarSlotSize, Height: hotbarSlotSize},
				hotbarInstance{X: buttonX, Y: buttonY, Width: recipeColumnWidth, Height: recipeEntryHeight},
			) {
				t.Fatalf("尺寸 %d 产物格与配方入口 %d 相交", size, row)
			}
		}
	}
	if CraftingOutputAt(100, 100, 0, 0, 3) {
		t.Fatal("零尺寸 framebuffer 被判为命中")
	}
}

// TestCraftingSourceHighlightCoversUnifiedView 锁定来源高亮覆盖统一视图两端：
// 网格格 0..8 用网格几何，背包 9..44 用背包几何。
func TestCraftingSourceHighlightCoversUnifiedView(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	for _, source := range []int{0, 4, 8, 9, 17, 44} {
		got := layoutInventory(
			&layout, atlas, core.Inventory{}, true, source,
			&CraftingOverlay{Size: 3}, nil, nil, 1280, 800,
		)
		highlight := got.quads[containerPanelQuads+1]
		wantX, wantY := inventorySlotOrigin(source-core.CraftingGridSlots, 1280, 800)
		if source < core.CraftingGridSlots {
			wantX, wantY = craftingGridSlotOrigin(source, 3, 1280, 800)
		}
		border := hotbarSelectBorder * hudScale(1280, 800)
		if highlight.X != wantX-border || highlight.Y != wantY-border {
			t.Fatalf("来源 %d 高亮 = %+v，想要包住 (%f,%f)", source, highlight, wantX, wantY)
		}
	}
	// 越界来源（超出统一视图 0..44）不画高亮；maxQuadTestInventory 的全部物品
	// 色块与九条耐久条都计入基础计数。
	baseQuads := containerPanelQuads + 1 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + craftingContentQuads + recipeColumnQuads
	got := layoutInventory(
		&layout, atlas, maxQuadTestInventory(), true, 45,
		fullCraftingOverlay(), nil, nil, 1280, 800,
	)
	if len(got.quads) != baseQuads {
		t.Fatalf("越界来源多画了高亮: quads=%d，想要 %d", len(got.quads), baseQuads)
	}
}

// 杀死变异：打开态组成漂移（漏面板、漏产物格、多画空格）都会改变精确计数。
func TestInventoryLayoutDrawsCraftingArea(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	if craftingContentQuads != 32 || craftingGlyphs != 40 {
		t.Fatalf("合成内容容量 quads/glyphs=%d/%d，想要 32/40", craftingContentQuads, craftingGlyphs)
	}
	// 空个人网格：面板族、选中框、36 格，与「轮廓 + 5 凹槽 + 箭头」的合成内容
	// 和十条配方入口。
	open := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, nil, 1280, 800)
	if len(open.quads) != containerPanelQuads+1+core.InventorySlots+7+recipeColumnQuads {
		t.Fatalf("空个人网格 quads=%d，想要面板族、选中框、36 格、7 个合成内容与配方栏",
			len(open.quads))
	}
	// 空个人网格没有物品数量；glyph 全部来自配方栏三条数量为 4 的产物。
	if len(open.glyphs) != 6 {
		t.Fatalf("空个人网格数字=%d，想要配方栏 6", len(open.glyphs))
	}

	// 满工作台加来源高亮与全部背包物品是合成视图的合法最坏组合。
	full := maxQuadTestInventory()
	workbench := &CraftingOverlay{Size: 3}
	for slot := range core.CraftingGridSlots {
		workbench.Slots[slot] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
	}
	workbench.Output = core.ItemStack{Item: core.ItemStoneBrick, Count: 4}
	got := layoutInventory(&layout, atlas, full, true, 5, workbench, nil, nil, 1280, 800)
	want := containerPanelQuads + 2 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + craftingContentQuads + recipeColumnQuads
	if len(got.quads) != want {
		t.Fatalf("满工作台 quads=%d，想要 %d", len(got.quads), want)
	}
	if len(got.quads) > maxHotbarQuads {
		t.Fatalf("合成视图超出固定容量: quads=%d > %d", len(got.quads), maxHotbarQuads)
	}
}

// 杀死变异：熔炉/箱子打开时合成区必须被互斥替换，反之亦然。四类视图共用同一
// 面板族（位置无法区分），因此用精确实例计数区分内容组成。
func TestContainerOverlaysReplaceCraftingContent(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	// 空背包 + 空叠加视图的基础：面板族、选中框与 36 格。
	const baseQuads = containerPanelQuads + 1 + core.InventorySlots

	workbench := &CraftingOverlay{Size: 3}
	crafting := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, workbench, nil, nil, 1280, 800)
	// 空 3×3：轮廓、10 个凹槽、箭头与配方栏。
	if len(crafting.quads) != baseQuads+12+recipeColumnQuads {
		t.Fatalf("空工作台视图 quads=%d，想要 %d+12+%d", len(crafting.quads), baseQuads, recipeColumnQuads)
	}

	for _, view := range []struct {
		name    string
		overlay *FurnaceOverlay
		chest   *ChestOverlay
		want    int
	}{
		// 空熔炉：3 个凹槽与 2 条进度底衬。
		{"熔炉", &FurnaceOverlay{}, nil, baseQuads + 5},
		// 空箱子：27 个凹槽。
		{"箱子", nil, &ChestOverlay{}, baseQuads + core.ChestSlots},
	} {
		got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, workbench, view.overlay, view.chest, 1280, 800)
		if len(got.quads) != view.want {
			t.Fatalf("%s 视图 quads=%d，想要 %d（合成区已被互斥替换）", view.name, len(got.quads), view.want)
		}
		titleUV := hotbarTextureUV(hotbarCraftingTitleColumn)
		for _, quad := range got.quads {
			if [4]float32{quad.U0, quad.V0, quad.U1, quad.V1} == titleUV {
				t.Fatalf("%s 视图仍画合成标题", view.name)
			}
		}
	}
	// crafting 与熔炉叠加值同时非 nil 时必须呈现熔炉（互斥优先级确定）：
	// 实例数是熔炉组成（含 2 条非零进度填充），而不是合成内容的 12+配方栏。
	withProgress := layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1,
		&CraftingOverlay{Size: 3}, &FurnaceOverlay{ProgressTicks: 7, BurnTicks: 300}, nil, 1280, 800,
	)
	if len(withProgress.quads) != baseQuads+5+2 {
		t.Fatalf("熔炉优先视图 quads=%d，想要 %d+5+2", len(withProgress.quads), baseQuads)
	}
}

// 杀死变异：遗漏任一熔炉格、放错进度图示或忽略权威计时都会改变实例布局。
func TestFurnaceOverlayDrawsThreeSlotsAndTwoBars(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout

	empty := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, &FurnaceOverlay{}, nil, 1280, 800)
	// 空熔炉：3 个栏位背景与 2 条进度底衬，没有物品色块或填充。
	emptyQuads := len(empty.quads)
	if len(empty.glyphs) != 0 {
		t.Fatalf("空熔炉数字 = %d，想要 0", len(empty.glyphs))
	}

	full := layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1, nil, fullFurnaceOverlay(), nil, 1280, 800,
	)
	if len(full.quads) != emptyQuads+3*2+2 {
		t.Fatalf("满熔炉 quads = %d，想要比空熔炉多 3 个双层色块和 2 条填充", len(full.quads))
	}
	if len(full.glyphs) != 12 {
		t.Fatalf("满熔炉数字 = %d，想要三组两位数含阴影共 12", len(full.glyphs))
	}
}

// TestFurnaceBarCompositionCropsAtlasIcons 验证两条进度的填充复用底衬 quad，
// 同时裁剪实例与 UV，而不是缩放完整图标；火焰居中于输入/燃料之间，箭头横置
// 指向输出栏。
func TestFurnaceBarCompositionCropsAtlasIcons(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	const width, height = float32(1280), float32(800)
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, &FurnaceOverlay{
		BurnTicks:     core.FurnaceBurnTicks / 2,
		ProgressTicks: core.FurnaceSmeltTicks / 2,
	}, nil, width, height)
	flameX, flameY := furnaceFlameOrigin(width, height)
	arrowX, arrowY := furnaceArrowOrigin(width, height)
	flameSize := furnaceFlameSize * got.scale
	arrowWidth := furnaceArrowWidth * got.scale
	arrowHeight := furnaceArrowHeight * got.scale
	fraction := float32(0.5)
	for _, test := range []struct {
		name string
		uv   [4]float32
		want hotbarInstance
	}{
		{
			"火焰自下向上", hotbarTextureUV(hotbarFurnaceFlameColumn), hotbarInstance{
				X: flameX, Y: flameY + flameSize*(1-fraction), Width: flameSize, Height: flameSize * fraction,
			},
		},
		{
			"箭头自左向右", hotbarTextureUV(hotbarFurnaceArrowColumn), hotbarInstance{
				X: arrowX, Y: arrowY, Width: arrowWidth * fraction, Height: arrowHeight,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			matches := 0
			for _, quad := range got.quads {
				if quad.U0 != test.uv[0] ||
					(test.name == "火焰自下向上" && quad.V1 != test.uv[3]) ||
					(test.name == "箭头自左向右" && quad.V0 != test.uv[1]) {
					continue
				}
				matches++
				if quad.X != test.want.X || quad.Y != test.want.Y || quad.Width != test.want.Width || quad.Height != test.want.Height {
					t.Fatalf("填充=%+v，想要实例=%+v", quad, test.want)
				}
				if test.name == "火焰自下向上" {
					if quad.V1 != test.uv[3] {
						t.Fatalf("火焰 V1=%v，想要保留完整 cell 底沿 %v（裁剪只动 V0/Y）", quad.V1, test.uv[3])
					}
					if quad.V0 != test.uv[1]+(test.uv[3]-test.uv[1])*(1-fraction) {
						t.Fatalf("火焰 V0=%v，想要比例端点", quad.V0)
					}
				}
				if test.name == "箭头自左向右" && quad.U1 != test.uv[0]+(test.uv[2]-test.uv[0])*fraction {
					t.Fatalf("箭头 U1=%v，想要比例端点", quad.U1)
				}
			}
			if matches != 1 {
				t.Fatalf("填充 UV quad=%d，想要 1", matches)
			}
		})
	}
	// 火焰区域必须落在输入栏底沿与燃料栏顶沿之间，箭头区域不得与三格相交。
	inputX, inputY := furnaceSlotOrigin(0, width, height)
	fuelX, fuelY := furnaceSlotOrigin(1, width, height)
	outputX, outputY := furnaceSlotOrigin(2, width, height)
	flame := hotbarInstance{X: flameX, Y: flameY, Width: flameSize, Height: flameSize}
	input := hotbarInstance{X: inputX, Y: inputY, Width: hotbarSlotSize * got.scale, Height: hotbarSlotSize * got.scale}
	fuel := hotbarInstance{X: fuelX, Y: fuelY, Width: input.Width, Height: input.Height}
	output := hotbarInstance{X: outputX, Y: outputY, Width: input.Width, Height: input.Height}
	arrow := hotbarInstance{X: arrowX, Y: arrowY, Width: arrowWidth, Height: arrowHeight}
	for _, test := range []struct {
		name string
		a, b hotbarInstance
	}{{"火焰-输入", flame, input}, {"火焰-燃料", flame, fuel}, {"火焰-输出", flame, output},
		{"箭头-输入", arrow, input}, {"箭头-燃料", arrow, fuel}, {"箭头-输出", arrow, output}} {
		if rectanglesIntersect(test.a, test.b) {
			t.Fatalf("%s 图示与栏位相交: %+v / %+v", test.name, test.a, test.b)
		}
	}
}

func TestFurnaceSlotAtCoversUnifiedIndices(t *testing.T) {
	width, height := uint32(1280), uint32(800)
	// 0..35 与背包命中一致。
	for _, slot := range []int{0, 8, 9, 35} {
		x, y := inventorySlotOrigin(slot, float32(width), float32(height))
		got, ok := FurnaceSlotAt(float64(x)+1, float64(y)+1, width, height)
		if !ok || int(got) != slot {
			t.Fatalf("统一索引 %d 命中 = %d, %v", slot, got, ok)
		}
	}
	// 36、37、38 落在熔炉三格上。
	for index := range 3 {
		x, y := furnaceSlotOrigin(index, float32(width), float32(height))
		got, ok := FurnaceSlotAt(float64(x), float64(y), width, height)
		if !ok || got != core.InventorySlots+uint8(index) {
			t.Fatalf("熔炉格 %d 命中 = %d, %v", index, got, ok)
		}
		if _, ok := FurnaceSlotAt(
			float64(x+hotbarSlotSize), float64(y+hotbarSlotSize/2), width, height,
		); ok {
			t.Fatalf("熔炉格 %d 右边界外仍被命中", index)
		}
	}
	if _, ok := FurnaceSlotAt(0, 0, width, height); ok {
		t.Fatal("界外命中被接受")
	}
	if _, ok := FurnaceSlotAt(100, 100, 0, 0); ok {
		t.Fatal("零尺寸 framebuffer 被判为命中")
	}
}

func TestFurnaceSourceHighlightCoversFurnaceSlots(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	for source := range core.FurnaceViewSlots {
		got := layoutInventory(
			&layout, atlas, core.Inventory{}, true, source,
			nil, &FurnaceOverlay{}, nil, 1280, 800,
		)
		// 面板族和当前选中框之后是来源高亮。
		highlight := got.quads[containerPanelQuads+1]
		wantX, wantY := inventorySlotOrigin(source, 1280, 800)
		if source >= core.InventorySlots {
			wantX, wantY = furnaceSlotOrigin(source-core.InventorySlots, 1280, 800)
		}
		border := hotbarSelectBorder * hudScale(1280, 800)
		if highlight.X != wantX-border || highlight.Y != wantY-border {
			t.Fatalf("来源 %d 高亮 = %+v，想要包住 (%f,%f)",
				source, highlight, wantX, wantY)
		}
	}
}

// 杀死变异：漏画任一格背景、错放物品或忽略空格都会改变实例布局。
func TestChestOverlayDraws27SlotsWithItemsAndCounts(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout

	empty := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, &ChestOverlay{}, 1280, 800)
	// 空箱子：27 个栏位背景，没有色块也没有数字。
	if len(empty.glyphs) != 0 {
		t.Fatalf("空箱子数字 = %d，想要 0", len(empty.glyphs))
	}
	emptyQuads := len(empty.quads)

	sparse := ChestOverlay{}
	sparse.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	sparse.Items[13] = core.ItemStack{Item: core.ItemCoal, Count: 5}
	sparse.Items[26] = core.ItemStack{Item: core.ItemIronIngot, Count: 1}
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, &sparse, 1280, 800)
	if len(got.quads) != emptyQuads+3*2 {
		t.Fatalf("三格占用 quads=%d，想要比空箱子多 3 个双层色块", len(got.quads))
	}
	if len(got.glyphs) != 6 {
		t.Fatalf("数字数量 = %d，想要 64/5 含阴影且隐藏 1，共 6 个实例", len(got.glyphs))
	}
	// 物品色块固定追加在箱子凹槽序列之后。
	tiles := got.quads[emptyQuads:len(got.quads)]
	wantItems := []core.ItemID{core.ItemStone, core.ItemCoal, core.ItemIronIngot}
	for index, item := range wantItems {
		face := tiles[index*2+1]
		assertHotbarItemFace(t, face, item)
	}

	full := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, fullChestOverlay(), 1280, 800)
	if len(full.quads) != emptyQuads+core.ChestSlots*2 {
		t.Fatalf("满箱子 quads=%d，想要比空箱子多 %d 个双层色块", len(full.quads), core.ChestSlots)
	}
	if len(full.glyphs) != 108 {
		t.Fatalf("满箱子数字 = %d，想要 27 组两位数含阴影共 108", len(full.glyphs))
	}
}

// 杀死变异：熔炉与箱子叠加值理论上互斥，但函数必须有确定的优先级而不是 panic 或漏画。
func TestChestOverlayTakesPriorityOverFurnaceOverlay(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	both := layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1, &CraftingOverlay{Size: 3}, &FurnaceOverlay{}, &ChestOverlay{}, 1280, 800,
	)
	chestOnly := layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1, nil, nil, &ChestOverlay{}, 1280, 800,
	)
	if len(both.quads) != len(chestOnly.quads) {
		t.Fatalf("两者都非 nil 时 quads=%d，想要与仅箱子相同 %d", len(both.quads), len(chestOnly.quads))
	}
}
func TestChestSlotAtCoversUnifiedIndices(t *testing.T) {
	width, height := uint32(1280), uint32(800)
	// 0..35 与背包命中一致。
	for _, slot := range []int{0, 8, 9, 35} {
		x, y := inventorySlotOrigin(slot, float32(width), float32(height))
		got, ok := ChestSlotAt(float64(x)+1, float64(y)+1, width, height)
		if !ok || int(got) != slot {
			t.Fatalf("统一索引 %d 命中 = %d, %v", slot, got, ok)
		}
	}
	// 36..62 落在箱子 27 格上。
	for index := range core.ChestSlots {
		x, y := chestSlotOrigin(index, float32(width), float32(height))
		got, ok := ChestSlotAt(float64(x), float64(y), width, height)
		if !ok || got != core.InventorySlots+uint8(index) {
			t.Fatalf("箱子格 %d 命中 = %d, %v", index, got, ok)
		}
		if _, ok := ChestSlotAt(
			float64(x+hotbarSlotSize), float64(y+hotbarSlotSize/2), width, height,
		); ok {
			t.Fatalf("箱子格 %d 右边界外仍被命中", index)
		}
	}
	if _, ok := ChestSlotAt(0, 0, width, height); ok {
		t.Fatal("界外命中被接受")
	}
	if _, ok := ChestSlotAt(100, 100, 0, 0); ok {
		t.Fatal("零尺寸 framebuffer 被判为命中")
	}
}
func TestChestSourceHighlightCoversChestSlots(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	for source := range core.ChestViewSlots {
		got := layoutInventory(
			&layout, atlas, core.Inventory{}, true, source,
			nil, nil, &ChestOverlay{}, 1280, 800,
		)
		// 面板族和当前选中框之后是来源高亮。
		highlight := got.quads[containerPanelQuads+1]
		wantX, wantY := inventorySlotOrigin(source, 1280, 800)
		if source >= core.InventorySlots {
			wantX, wantY = chestSlotOrigin(source-core.InventorySlots, 1280, 800)
		}
		border := hotbarSelectBorder * hudScale(1280, 800)
		if highlight.X != wantX-border || highlight.Y != wantY-border {
			t.Fatalf("来源 %d 高亮 = %+v，想要包住 (%f,%f)",
				source, highlight, wantX, wantY)
		}
	}
}

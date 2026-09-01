package hud

import (
	"reflect"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestOpenHeightAndWidthPinVanillaSpacing 锁定打开态的空间契约：统一面板高度
// 自上而下为标题 header、图示区三行、行距、背包三行、行距、快捷栏行与底部内边
// 距（四类视图共用同一高度保证垂直原点视图无关）；打开态高度约束在同一份面板
// 高度之上叠加底边距、两行状态栈与贴条外沿可见净空，宽度约束按配方栏面板的对
// 称等效宽度取值。常显层退役后关闭态不再有 GPU 实例，缩放约束因此只剩打开态。
func TestOpenHeightAndWidthPinVanillaSpacing(t *testing.T) {
	if hotbarBottomMargin != 6 {
		t.Fatalf("hotbarBottomMargin=%v，想要原版关系的 6 design px", hotbarBottomMargin)
	}
	if statusHotbarGap != 10 {
		t.Fatalf("statusHotbarGap=%v，想要主状态行底到贴条外沿 10 design px 的可见净空", statusHotbarGap)
	}
	// 打开态把两行状态栈留在快捷栏下方，其高度约束必须容纳同一份贴条外沿
	// 净空；贴条上沿 padding 已计入面板高度底部的 `hotbarPanelPadding`，
	// 因此这里不再重复相加。
	if openHUDHeight != containerPanelHeight+hotbarBottomMargin+2*(healthHeartSize+statusBarGap)+statusHotbarGap {
		t.Fatal("openHUDHeight 与「面板高度+底边距+两行状态栈+贴条外沿可见净空」的分解不一致")
	}
	if containerPanelHeight != containerHeaderHeight+
		2*(overlayAreaRows*hotbarSlotSize+(overlayAreaRows-1)*hotbarSlotGap)+
		recipeRowGap+inventoryRowGap+hotbarSlotSize+hotbarPanelPadding {
		t.Fatal("containerPanelHeight 与「header+图示区+背包+快捷栏行」的分解不一致")
	}
	if containerPanelHeight != 406 || openHUDHeight != 462 {
		t.Fatalf("面板/打开态高度钉值=%v/%v，想要 406/462", containerPanelHeight, openHUDHeight)
	}
	// 打开态宽度约束按配方栏面板的对称等效宽度取值（个人面板向右伸出共享
	// 内容列一份栏间隙与栏宽，等效两侧各伸出一份，再加两侧屏幕边距）。
	if want := hotbarRowWidth + 2*(hotbarPanelPadding+recipeColumnGap+recipeColumnWidth+hudEdgeMargin); openHUDWidth != want || openHUDWidth != 700 {
		t.Fatalf("openHUDWidth=%v，想要 %v（钉值 700）", openHUDWidth, want)
	}
	// 打开态底部状态栈上沿与高度约束按同一份预留项推导，两处约束描述同一份
	// 构图：面板在「顶边到状态栈上沿」的剩余空间内居中。
	const width, height = float32(1280), float32(800)
	scale := hudScale(width, height)
	hotbarY := height - (hotbarBottomMargin+2*(healthHeartSize+statusBarGap)+statusHotbarGap+
		hotbarPanelPadding+hotbarSlotSize)*scale
	wantStackTop := hotbarY + (hotbarSlotSize+hotbarPanelPadding+statusHotbarGap+statusBarGap)*scale
	if got := openBottomStackTop(width, height); got != wantStackTop {
		t.Fatalf("openBottomStackTop=%v，想要按预留项推导的 %v", got, wantStackTop)
	}
}

// TestHotbarCountsHideOneAndUseShadowedBottomRightDigits 杀死变异：继续显示单件
// 数量、漏掉阴影、错序或失去右下对齐都会改变固定 glyph 预算与面板内数字观感。
func TestHotbarCountsHideOneAndUseShadowedBottomRightDigits(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	appendHotbarCountScaled(&layout, atlas, 1, 100, 200, 1)
	if len(layout.glyphs) != 0 {
		t.Fatalf("单件数量 glyphs=%d，想要隐藏冗余数字 1", len(layout.glyphs))
	}

	appendHotbarCountScaled(&layout, atlas, 64, 100, 200, 1)
	if len(layout.glyphs) != 4 {
		t.Fatalf("数量 64 glyphs=%d，想要两个阴影加两个前景", len(layout.glyphs))
	}
	want := []hotbarInstance{
		{X: 134, Y: 236, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: textPrimaryShadow},
		{X: 139, Y: 236, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: textPrimaryShadow},
		{X: 133, Y: 235, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: textPrimaryFg},
		{X: 138, Y: 235, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: textPrimaryFg},
	}
	if !reflect.DeepEqual(layout.glyphs, want) {
		t.Fatalf("数量 64 glyphs=%+v，想要右下阴影/前景 %+v", layout.glyphs, want)
	}

	layout.glyphs = layout.glyphs[:0]
	appendHotbarCountScaled(&layout, atlas, 64, 100, 200, 0.5)
	if first, second := layout.glyphs[2], layout.glyphs[3]; first.X != 116.5 || second.X != 119 {
		t.Fatalf("0.5x 两位前景 X=%v/%v，想要 tracking 同步缩放且右边缘不动", first.X, second.X)
	}
}

// TestPanelItemTilesUseInsetLayers 杀死变异：移除物品暗边或退回平面色块会破坏
// 面板内容的统一层级（统一栏位的双层物品 tile 是打开态最坏 quad 预算的 72）。
func TestPanelItemTilesUseInsetLayers(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemGrass, Count: 1}

	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, inventory, true, -1, nil, nil, nil, 1280, 800)
	// 物品 tile 收在栏位内部：以「完整落在选中格 0 内」定位，边框与面色各恰
	// 一层（合成图式的箭头图示同为 24 design px，不能按宽度区分）。
	slotX, slotY := inventorySlotOrigin(0, 1280, 800)
	slotSize := hotbarSlotSize * got.scale
	borders, faces := 0, 0
	var border, face hotbarInstance
	for _, quad := range got.quads {
		if quad.X < slotX || quad.Y < slotY ||
			quad.X+quad.Width > slotX+slotSize || quad.Y+quad.Height > slotY+slotSize {
			continue
		}
		switch quad.Width {
		case (hotbarSlotSize - 2*hotbarSwatchInset) * got.scale:
			border, borders = quad, borders+1
		case (hotbarSlotSize - 2*hotbarSwatchInset - 2*hotbarSwatchBorder) * got.scale:
			face, faces = quad, faces+1
		}
	}
	if borders != 1 || faces != 1 {
		t.Fatalf("物品 tile 边框/面色=%d/%d，想要各 1", borders, faces)
	}
	if border.Width <= face.Width || border.Height <= face.Height || border.Color == face.Color {
		t.Fatalf("物品双层色块 border=%+v face=%+v", border, face)
	}
	assertHotbarItemFace(t, face, core.ItemGrass)
}

// TestClosedContainerProducesZeroInstances 锁定关闭态保留面零实例契约：常显层
// （快捷栏贴条与选中框、状态行图标、氧气、采掘/进食轨道、物品名弹条、准星、
// 聊天呈现）已迁 WebView 组件，容器面板与内容只在打开态布局，关闭态因此恰好
// 0 quad / 0 glyph。
func TestClosedContainerProducesZeroInstances(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, maxQuadTestInventory(), false, -1, nil, nil, fullChestOverlay(), 1280, 800)
	if len(got.quads) != 0 || len(got.glyphs) != 0 {
		t.Fatalf("关闭态 quads/glyphs=%d/%d，想要 0/0", len(got.quads), len(got.glyphs))
	}
	// 非法物品状态与零尺寸 framebuffer 同样不得产生任何实例。
	invalid := core.Inventory{Hotbar: core.Hotbar{Selected: core.HotbarSlots}}
	if got := layoutInventory(&layout, atlas, invalid, true, -1, nil, nil, nil, 800, 600); len(got.quads) != 0 {
		t.Fatalf("非法物品状态 quads=%d，想要 0", len(got.quads))
	}
	if got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, nil, 0, 600); len(got.quads) != 0 {
		t.Fatalf("零宽 framebuffer quads=%d，想要 0", len(got.quads))
	}
	if got := layoutInventory(&layout, atlas, maxQuadTestInventory(), false, -1, nil, nil, fullChestOverlay(), 1280, 0); len(got.glyphs) != 0 {
		t.Fatalf("零高 framebuffer glyphs=%d，想要 0", len(got.glyphs))
	}
}

// TestHotbarLayoutStaysWithinFixedCapacity 杀死变异：超过固定 HUD 容量会溢出
// 预分配上传区。关闭态零实例，打开态以箱子视图见证 218 的合法最坏（实算分解
// 见 panel_test.go），glyph 以满格两位数量与 tooltip 双层名见证，都不得突破
// 固定容量。
func TestHotbarLayoutStaysWithinFixedCapacity(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout

	// 打开分支 quad 见证：面板族 + 选中/来源 + 36 格双层物品 + 九条耐久 +
	// 箱子 81 内容 + tooltip 背景共同见证合法最坏；固定容量 320 仍保留可观余量。
	got := layoutInventory(
		&layout, atlas, maxQuadTestInventory(), true, 5, nil, nil, fullChestOverlay(), 1280, 800,
	)
	hoverX, hoverY := chestSlotOrigin(0, 1280, 800)
	appendTooltipOverlay(&got, atlas, TooltipOverlay{
		Valid: true, CursorX: float64(hoverX) + 1, CursorY: float64(hoverY) + 1,
	}, maxQuadTestInventory(), nil, nil, fullChestOverlay(), 1280, 800)
	if len(got.quads) != openInventoryQuads || openInventoryQuads != 218 || len(got.quads) > maxHotbarQuads {
		t.Fatalf("打开分支 quads=%d，分支公式/钉值=%d，固定上限=%d",
			len(got.quads), openInventoryQuads, maxHotbarQuads)
	}

	// glyph 见证：36 格两位数量 + 箱子 27 格两位数量 + tooltip 双层名。预算按
	// 8 rune 截断上限封顶（268），注册表实测最长名见证更小，都不得突破 768。
	longNameChest := fullChestOverlay()
	longNameChest.Items[0] = core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: core.MaxStackCount}
	glyphs := layoutInventory(
		&layout, atlas, fullTestInventory(), true, 5, nil, nil, longNameChest, 1280, 800,
	)
	appendTooltipOverlay(&glyphs, atlas, TooltipOverlay{
		Valid: true, CursorX: float64(hoverX) + 1, CursorY: float64(hoverY) + 1,
	}, fullTestInventory(), nil, nil, longNameChest, 1280, 800)

	// glyph 见证：36 格两位数量 + 箱子 27 格两位数量 + tooltip 双层名。预算按
	// 8 rune 截断上限封顶（268），注册表实测最长名见证更小，都不得突破 768。
	budget := core.InventorySlots*4 + chestGlyphs + tooltipGlyphs
	if budget != 268 {
		t.Fatalf("打开态 glyph 预算=%d，想要钉值 268", budget)
	}
	if len(glyphs.glyphs) > maxHotbarGlyphs {
		t.Fatalf("glyph 见证=%d，超过固定上限 %d", len(glyphs.glyphs), maxHotbarGlyphs)
	}
}

// 杀死变异：放宽显示条件会让满耐久、损坏形态或普通物品多出 quad。
func TestDurabilityBarAppearsOnlyForWornTools(t *testing.T) {
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	for _, test := range []struct {
		name  string
		stack core.ItemStack
		want  int
	}{
		{"满耐久工具不显示", core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full}, 0},
		{"磨损工具显示两个 quad", core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full / 2}, 2},
		{"损坏物品不显示", core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: 1}, 0},
		{"普通方块不显示", core.ItemStack{Item: core.ItemStone, Count: 64}, 0},
		{"空栏位不显示", core.ItemStack{}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			appendDurabilityBarScaled(&layout, 0, test.stack, 1920, 1080, hudScale(1920, 1080))
			if got := len(layout.quads); got != test.want {
				t.Fatalf("quad 数量 = %d，想要 %d", got, test.want)
			}
		})
	}
}

// 杀死变异：固定宽度或整数相除会让低耐久填充条不再正且短于高耐久。
func TestDurabilityBarFillTracksRemaining(t *testing.T) {
	full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
	scale := hudScale(1920, 1080)
	var low, high hotbarLayout
	appendDurabilityBarScaled(&low, 0, core.ItemStack{
		Item: core.ItemIronPickaxe, Count: 1, Durability: 1,
	}, 1920, 1080, scale)
	appendDurabilityBarScaled(&high, 0, core.ItemStack{
		Item: core.ItemIronPickaxe, Count: 1, Durability: full - 1,
	}, 1920, 1080, scale)

	if len(low.quads) != 2 || len(high.quads) != 2 {
		t.Fatalf("quad 数量 = %d / %d，想要各 2", len(low.quads), len(high.quads))
	}
	if low.quads[1].Width >= high.quads[1].Width {
		t.Fatalf("低耐久填充宽度 %v 不小于高耐久 %v", low.quads[1].Width, high.quads[1].Width)
	}
	if low.quads[1].Width <= 0 {
		t.Fatalf("填充宽度 = %v，想要正值", low.quads[1].Width)
	}
}

// 杀死变异：遍历全部 36 格或漏掉背包格会让面板耐久条越界多画。
func TestDurabilityBarLayoutUsesOnlyHotbarRow(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	base := core.Inventory{}
	base.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full}
	base.Backpack[0] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full}
	hotbarWorn := base
	hotbarWorn.Hotbar.Slots[3].Durability--
	backpackWorn := base
	backpackWorn.Backpack[0].Durability--

	baseQuads := len(layoutInventory(
		&hotbarLayout{}, atlas, base, true, -1, nil, nil, nil, 1280, 800,
	).quads)
	got := layoutInventory(
		&hotbarLayout{}, atlas, hotbarWorn, true, -1, nil, nil, nil, 1280, 800,
	)
	if len(got.quads) != baseQuads+2 {
		t.Fatalf("快捷栏磨损工具 quads=%d，想要 %d", len(got.quads), baseQuads+2)
	}
	// 耐久条以轨道色定位（面板内容按「栏位、tile、数量、耐久、容器内容」
	// 顺序追加，索引算式会随视图内容漂移）。
	bars := [2]hotbarInstance{}
	found := 0
	for _, quad := range got.quads {
		if quad.Color == durabilityTrackColor {
			bars[0] = quad
			found++
			continue
		}
		if found == 1 && quad.Color == durabilityHealthyColor {
			bars[1] = quad
		}
	}
	if found != 1 || bars[1].Width == 0 {
		t.Fatalf("未在布局中找到耐久底槽与填充: track=%d fill=%+v", found, bars[1])
	}
	scale := hudScale(1280, 800)
	slotX, slotY := inventorySlotOrigin(3, 1280, 800)
	wantX := slotX + durabilityBarInset*scale
	wantY := slotY + (hotbarSlotSize-durabilityBarInset-durabilityBarHeight)*scale
	wantWidth := (hotbarSlotSize - 2*durabilityBarInset) * scale
	for index, bar := range bars {
		if bar.X != wantX || bar.Y != wantY || bar.Width <= 0 || bar.Width > wantWidth ||
			bar.Height != durabilityBarHeight*scale {
			t.Fatalf("耐久条 %d 未复用面板快捷栏行几何: %+v", index, bar)
		}
	}
	if got := len(layoutInventory(
		&hotbarLayout{}, atlas, backpackWorn, true, -1, nil, nil, nil, 1280, 800,
	).quads); got != baseQuads {
		t.Fatalf("背包栏磨损工具 quads=%d，想要 %d", got, baseQuads)
	}
}

// Mutation killed: mislaying the backpack rows, dropping the source highlight,
// or letting hit-testing drift from the drawn geometry.
func TestInventoryLayoutOpensThreeBackpackRows(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	var inventory core.Inventory
	got := layoutInventory(&layout, atlas, inventory, true, 12, nil, nil, nil, 1280, 800)

	// 面板族 + 选中框 + 来源高亮 + 36 格 + 空个人合成内容（轮廓、5 个凹槽与
	// 箭头）+ 十条配方入口。
	if len(got.quads) != containerPanelQuads+2+core.InventorySlots+7+recipeColumnQuads {
		t.Fatalf("打开时 quads=%d，想要 %d", len(got.quads),
			containerPanelQuads+2+core.InventorySlots+7+recipeColumnQuads)
	}
	// 面板族以投影开头、表面紧随其后：两层的颜色区分投影与表面语义。
	family := got.quads[:containerPanelQuads]
	if family[0].Color != panelShadow || family[1].Color != panelSurface {
		t.Fatalf("面板族前两层=%+v/%+v，想要投影与表面", family[0], family[1])
	}
	// 个人面板向右展开配方栏：宽度比基础面板多出栏间隙与栏宽。
	frame := openPanelAnchor(1280, 800)
	wantWidth := containerPanelWidth*frame.scale + (recipeColumnGap+recipeColumnWidth)*frame.scale
	if family[0].Width != wantWidth+2*panelShadowExpand*frame.scale {
		t.Fatalf("个人面板投影宽=%v，想要含配方栏的 %v", family[0].Width, wantWidth)
	}
	selectedX, selectedY := inventorySlotOrigin(0, 1280, 800)
	scale := hudScale(1280, 800)
	selectBorder := hotbarSelectBorder * scale
	foundSelection := false
	for _, quad := range got.quads {
		if quad.X == selectedX-selectBorder && quad.Y == selectedY-selectBorder &&
			quad.Width == (hotbarSlotSize+2*hotbarSelectBorder)*scale && quad.Height == (hotbarSlotSize+2*hotbarSelectBorder)*scale {
			// 打开态选中格内衬取鼠尾草绿强调（`accentSelected` 语义族）。
			if quad.Color != hotbarSelectedInnerColor {
				t.Fatalf("打开态选中格颜色=%v，想要令牌 hotbarSelectedInnerColor", quad.Color)
			}
			foundSelection = true
			break
		}
	}
	if !foundSelection {
		t.Fatal("未找到打开态选中格")
	}
	for slot := range core.InventorySlots {
		x, y := inventorySlotOrigin(slot, 1280, 800)
		if slot < core.HotbarSlots && y != frame.hotbarY {
			t.Fatalf("快捷栏格 %d 不在面板下段底行: y=%f", slot, y)
		}
		if slot >= core.HotbarSlots && y >= frame.hotbarY {
			t.Fatalf("背包格 %d 未排在快捷栏之上: y=%f", slot, y)
		}
		// 命中函数必须与绘制几何一致。
		if got, ok := InventorySlotAt(float64(x)+1, float64(y)+1, 1280, 800); !ok ||
			got != uint8(slot) {
			t.Fatalf("InventorySlotAt 命中 %d, %v，想要 %d", got, ok, slot)
		}
	}
}
func TestInventorySlotAtRejectsOutsideHits(t *testing.T) {
	if _, ok := InventorySlotAt(0, 0, 1280, 800); ok {
		t.Fatal("界外命中被接受")
	}
	x, y := inventorySlotOrigin(0, 1280, 800)
	if _, ok := InventorySlotAt(float64(x)-1, float64(y)+1, 1280, 800); ok {
		t.Fatal("格子左侧 1 像素被判为命中")
	}
	if _, ok := InventorySlotAt(float64(x)+1, float64(y)+1, 0, 0); ok {
		t.Fatal("零尺寸 framebuffer 被判为命中")
	}
}

// TestOpenHotbarRowFollowsPanelGeometry 防止面板几何与命中缩放漂移：打开态
// 快捷栏行收进浮动面板下段，其 X 必须与状态行锚点同轴，Y 必须等于面板推导的
// 下段底行，保证既有命中几何不漂移。
func TestOpenHotbarRowFollowsPanelGeometry(t *testing.T) {
	const width, height = float32(640), float32(360)
	frame := openPanelAnchor(width, height)
	gotX, gotY := inventorySlotOrigin(0, width, height)
	if gotX != frame.contentLeft || gotY != frame.hotbarY {
		t.Fatalf("打开态快捷栏原点=(%v,%v)，想要面板推导的 (%v,%v)", gotX, gotY, frame.contentLeft, frame.hotbarY)
	}
	// 背包三行紧贴快捷栏行上方、按同一行距堆叠。
	for row := range 3 {
		slot := core.HotbarSlots + row*core.HotbarSlots
		_, y := inventorySlotOrigin(slot, width, height)
		wantY := frame.hotbarY - (inventoryRowGap+float32(3-row)*hotbarSlotSize+float32(2-row)*hotbarSlotGap)*frame.scale
		if y != wantY {
			t.Fatalf("背包行 %d y=%v，想要 %v", row, y, wantY)
		}
	}
}

// 杀死变异：小窗口保持固定 48px 会让上方工作台网格落出 framebuffer；独立缩放命中则会漂移。
func TestOpenInventoryFitsAndHitsAt640x360(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	workbench := &CraftingOverlay{Size: 3}
	workbench.Slots[8] = core.ItemStack{Item: core.ItemStone, Count: 2}
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, workbench, nil, nil, 640, 360)
	for index, quad := range got.quads {
		if quad.X < 0 || quad.Y < 0 || quad.X+quad.Width > 640 || quad.Y+quad.Height > 360 {
			t.Fatalf("quad %d 越界: %+v", index, quad)
		}
	}
	for index, glyph := range got.glyphs {
		if glyph.X < 0 || glyph.Y < 0 || glyph.X+glyph.Width > 640 || glyph.Y+glyph.Height > 360 {
			t.Fatalf("glyph %d 越界: %+v", index, glyph)
		}
	}
	for slot := range core.CraftingGridSlots {
		x, y := craftingGridSlotOrigin(slot, 3, 640, 360)
		gotSlot, ok := CraftingSlotAt(float64(x)+1, float64(y)+1, 640, 360, 3)
		if !ok || gotSlot != uint8(slot) {
			t.Fatalf("缩放网格格 %d 命中=%d,%v", slot, gotSlot, ok)
		}
	}
	outputX, outputY := craftingOutputOrigin(3, 640, 360)
	if !CraftingOutputAt(float64(outputX)+1, float64(outputY)+1, 640, 360, 3) {
		t.Fatal("缩放后的产物格不可命中")
	}
}

// TestInventorySlotAtBoundariesRemainHalfOpen 锁定改动前的左上闭、右下开命中语义。
func TestInventorySlotAtBoundariesRemainHalfOpen(t *testing.T) {
	for _, size := range [][2]uint32{{1280, 720}, {640, 360}, {240, 40}} {
		scale := hudScale(float32(size[0]), float32(size[1]))
		slotSize := hotbarSlotSize * scale
		for slot := range core.InventorySlots {
			left, top := inventorySlotOrigin(slot, float32(size[0]), float32(size[1]))
			for _, point := range [][2]float64{
				{float64(left), float64(top)},
				{float64(left + slotSize*0.75), float64(top + slotSize*0.75)},
			} {
				got, ok := InventorySlotAt(point[0], point[1], size[0], size[1])
				if !ok || got != uint8(slot) {
					t.Fatalf("framebuffer %v slot %d 内点 %v 命中=%d,%v", size, slot, point, got, ok)
				}
			}
			for _, point := range [][2]float64{
				{float64(left + slotSize), float64(top + slotSize*0.5)},
				{float64(left + slotSize*0.5), float64(top + slotSize)},
			} {
				if got, ok := InventorySlotAt(point[0], point[1], size[0], size[1]); ok {
					t.Fatalf("framebuffer %v slot %d 右/下边界 %v 意外命中 %d", size, slot, point, got)
				}
			}
		}
	}
}

// TestContainerSlotGeometryKeepsUnifiedHitTests 穷举三种容器的中心与边界外一点，
// 锁定绘制 origin 和统一索引不会被 header 皮肤带偏。
func TestContainerSlotGeometryKeepsUnifiedHitTests(t *testing.T) {
	for _, size := range [][2]uint32{{1280, 800}, {240, 40}, {800, 17}} {
		width, height := size[0], size[1]
		for _, test := range []struct {
			name   string
			count  int
			origin func(int) (float32, float32)
			hit    func(float64, float64, uint32, uint32) (uint8, bool)
		}{
			{"背包", core.InventorySlots, func(slot int) (float32, float32) {
				return inventorySlotOrigin(slot, float32(width), float32(height))
			}, InventorySlotAt},
			{"熔炉", core.FurnaceViewSlots, func(slot int) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, float32(width), float32(height))
				}
				return furnaceSlotOrigin(slot-core.InventorySlots, float32(width), float32(height))
			}, FurnaceSlotAt},
			{"箱子", core.ChestViewSlots, func(slot int) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, float32(width), float32(height))
				}
				return chestSlotOrigin(slot-core.InventorySlots, float32(width), float32(height))
			}, ChestSlotAt},
		} {
			t.Run(test.name, func(t *testing.T) {
				slotSize := hotbarSlotSize * hudScale(float32(width), float32(height))
				for slot := range test.count {
					left, top := test.origin(slot)
					for _, point := range [][2]float32{{left, top}, {left + slotSize/2, top + slotSize/2}} {
						if got, ok := test.hit(float64(point[0]), float64(point[1]), width, height); !ok || int(got) != slot {
							t.Fatalf("framebuffer %v slot %d 内点 %v 命中=%d,%t", size, slot, point, got, ok)
						}
					}
					if got, ok := test.hit(float64(left-1), float64(top+slotSize/2), width, height); ok && int(got) == slot {
						t.Fatalf("framebuffer %v slot %d 左侧 1px 仍命中当前格", size, slot)
					}
					for _, point := range [][2]float32{{left + slotSize, top + slotSize/2}, {left + slotSize/2, top + slotSize}} {
						if _, ok := test.hit(float64(point[0]), float64(point[1]), width, height); ok {
							t.Fatalf("framebuffer %v slot %d 右/下边界 %v 被命中", size, slot, point)
						}
					}
				}
			})
		}
	}
}

// TestContainerTitleAndBordersAvoidHitCells 验证标题 cell 与四边亮边只占面板
// 顶部与边缘装饰位：标题与所有可交互矩形保持分离，窄高与矮宽窗口仍由同一个
// `hudScale` 收缩。
func TestContainerTitleAndBordersAvoidHitCells(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, size := range [][2]float32{{1280, 800}, {240, 40}, {800, 17}} {
		for _, view := range []struct {
			name     string
			crafting *CraftingOverlay
			overlay  *FurnaceOverlay
			chest    *ChestOverlay
			count    int
			origin   func(int, float32, float32) (float32, float32)
		}{
			{"合成", &CraftingOverlay{Size: 3}, nil, nil, core.InventorySlots, func(slot int, width, height float32) (float32, float32) {
				return inventorySlotOrigin(slot, width, height)
			}},
			{"熔炉", nil, &FurnaceOverlay{}, nil, core.FurnaceViewSlots, func(slot int, width, height float32) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, width, height)
				}
				return furnaceSlotOrigin(slot-core.InventorySlots, width, height)
			}},
			{"箱子", nil, nil, &ChestOverlay{}, core.ChestViewSlots, func(slot int, width, height float32) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, width, height)
				}
				return chestSlotOrigin(slot-core.InventorySlots, width, height)
			}},
		} {
			var layout hotbarLayout
			got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, view.crafting, view.overlay, view.chest, size[0], size[1])
			title := got.quads[containerPanelQuads-1]
			frame := openPanelAnchor(size[0], size[1])
			if title.X != frame.contentLeft || title.Y != frame.y+containerTitleGap*frame.scale {
				t.Fatalf("framebuffer %v %s 标题=%+v，未落在面板顶部内容列左沿", size, view.name, title)
			}
			slotSize := hotbarSlotSize * got.scale
			for slot := range view.count {
				left, top := view.origin(slot, size[0], size[1])
				if rectanglesIntersect(title, hotbarInstance{X: left, Y: top, Width: slotSize, Height: slotSize}) {
					t.Fatalf("framebuffer %v %s 标题与 slot %d 相交", size, view.name, slot)
				}
			}
			if view.overlay == nil && view.chest == nil {
				// 合成视图的标题还必须避开网格格与产物格（网格格不在上面的
				// view.origin 命中清单里，这里单独穷举）。
				for slot := range core.CraftingGridSlots {
					left, top := craftingGridSlotOrigin(slot, 3, size[0], size[1])
					if rectanglesIntersect(title, hotbarInstance{X: left, Y: top, Width: slotSize, Height: slotSize}) {
						t.Fatalf("framebuffer %v 合成标题与网格格 %d 相交", size, slot)
					}
				}
				outputLeft, outputTop := craftingOutputOrigin(3, size[0], size[1])
				if rectanglesIntersect(title, hotbarInstance{X: outputLeft, Y: outputTop, Width: slotSize, Height: slotSize}) {
					t.Fatalf("framebuffer %v 合成标题与产物格相交", size)
				}
			}
			// 四边亮边是面板边缘的 1 design px 装饰：它们必须贴在面板矩形边缘，
			// 不与任何栏位矩形相交（栏位都收在内容区内部）。
			for _, edge := range got.quads[2 : containerPanelQuads-1] {
				if edge.Color != panelBorderLight {
					t.Fatalf("framebuffer %v %s 面板边缘实例=%+v，想要亮边令牌", size, view.name, edge)
				}
				for slot := range view.count {
					left, top := view.origin(slot, size[0], size[1])
					if rectanglesIntersect(edge, hotbarInstance{X: left, Y: top, Width: slotSize, Height: slotSize}) {
						t.Fatalf("framebuffer %v %s 亮边与 slot %d 相交: %+v", size, view.name, slot, edge)
					}
				}
			}
		}
	}
}

func rectanglesIntersect(a, b hotbarInstance) bool {
	return a.X < b.X+b.Width && a.X+a.Width > b.X && a.Y < b.Y+b.Height && a.Y+a.Height > b.Y
}

// TestAppendCountAtSizeKeepsLayeredPenAlignment 锁定 `appendCountAtSize` 的双层
// 笔起点一致性：任意锚定尺寸下阴影层都必须恰在前景层右下 1 design px——统一
// 栏位 48 格与配方栏 24 紧凑行共用同一公式，两层笔起点分叉会让紧凑行的数字
// 水平裂开。
func TestAppendCountAtSizeKeepsLayeredPenAlignment(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	for _, size := range []float32{hotbarSlotSize, recipeEntryHeight} {
		var layout hotbarLayout
		appendCountAtSize(&layout, atlas, 64, 100, 200, size, 1)
		if len(layout.glyphs) != 4 {
			t.Fatalf("size=%v 数量 64 glyphs=%d，想要两位双层共 4", size, len(layout.glyphs))
		}
		for index := range 2 {
			shadow, foreground := layout.glyphs[index], layout.glyphs[index+2]
			if shadow.X != foreground.X+1 || shadow.Y != foreground.Y+1 {
				t.Fatalf("size=%v 数字 %d 阴影=(%v,%v) 前景=(%v,%v)，想要阴影恰在前景右下 1 design px",
					size, index, shadow.X, shadow.Y, foreground.X, foreground.Y)
			}
		}
	}
}

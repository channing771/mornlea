package hud

import (
	"reflect"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestBottomMarginAndClosedHeightPinVanillaSpacing 锁定原版对齐后的底部空间
// 契约：快捷栏下边距收到 6 design px（选中格 3px 外扩描边仍在界内），关闭态
// 联合高度 = 6 边距 + 48 格 + 两行状态栈 2×20 + 采掘轨道 16+12 + 弹条行 6+16
// = 144 design px。任何一端改动都会同时改变缩放行为与 golden，必须显式审视。
func TestBottomMarginAndClosedHeightPinVanillaSpacing(t *testing.T) {
	if hotbarBottomMargin != 6 {
		t.Fatalf("hotbarBottomMargin=%v，想要原版关系的 6 design px", hotbarBottomMargin)
	}
	if want := float32(6 + 48 + 2*(4+16) + 16 + 12 + 22); closedHUDHeight != want {
		t.Fatalf("closedHUDHeight=%v，想要边距+格+两行状态栈+采掘轨道+弹条行 %v", closedHUDHeight, want)
	}
	if closedHUDHeight != 144 {
		t.Fatalf("closedHUDHeight=%v，想要钉值 144", closedHUDHeight)
	}
	// 打开态把两行状态栈移到快捷栏下方，其高度约束必须容纳同样两行，
	// 即比单行状态布局多恰好一行 `healthHeartSize + statusBarGap`。
	if openHUDHeight != hotbarBottomMargin+hotbarSlotSize+
		inventoryRowGap+3*hotbarSlotSize+2*hotbarSlotGap+
		recipeRowGap+overlayAreaRows*hotbarSlotSize+(overlayAreaRows-1)*hotbarSlotGap+
		hotbarPanelPadding+containerHeaderHeight+2*(healthHeartSize+statusBarGap) {
		t.Fatal("openHUDHeight 与「边距+两行状态栈在快捷栏下方」的分解不一致")
	}
}

// TestClosedHotbarCentersNineSlotsWithTwoPanelLayersAndSelectionFrames 验证关闭态
// 快捷栏的非颜色轮廓：删掉任一面板或选中框层、或漂移中心几何都会失败。
func TestClosedHotbarCentersNineSlotsWithTwoPanelLayersAndSelectionFrames(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var inventory core.Inventory
	inventory.Hotbar.Selected = 2
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, inventory, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 800, 600)

	if len(got.quads) != crosshairQuads+4+core.HotbarSlots {
		t.Fatalf("空物品状态 quads=%d，想要准星、双层面板、双层选中框加 9 个栏位", len(got.quads))
	}
	if len(got.glyphs) != 0 {
		t.Fatalf("空快捷栏数字=%d，想要 0", len(got.glyphs))
	}

	total := core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap
	originX := (800 - total) * 0.5
	originY := 600 - hotbarBottomMargin - hotbarSlotSize
	for slot := range core.HotbarSlots {
		wantX := originX + float32(slot)*(hotbarSlotSize+hotbarSlotGap)
		found := false
		for _, quad := range got.quads {
			if quad.X == wantX && quad.Y == originY && quad.Width == hotbarSlotSize && quad.Height == hotbarSlotSize {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("未找到栏位 %d，想要 (%f,%f,%f,%f)",
				slot, wantX, originY, hotbarSlotSize, hotbarSlotSize)
		}
	}
	panels := make([]hotbarInstance, 0, closedHotbarPanelQuads)
	for _, quad := range got.quads {
		if quad.Width > total && quad.Height > hotbarSlotSize {
			panels = append(panels, quad)
		}
	}
	if len(panels) != closedHotbarPanelQuads {
		t.Fatalf("关闭态面板数量=%d，想要 %d", len(panels), closedHotbarPanelQuads)
	}
	outerPanel, innerPanel := panels[0], panels[1]
	if outerPanel.Width < innerPanel.Width {
		outerPanel, innerPanel = innerPanel, outerPanel
	}
	if outerPanel.X >= innerPanel.X || outerPanel.Y >= innerPanel.Y ||
		outerPanel.Width <= innerPanel.Width || outerPanel.Height <= innerPanel.Height ||
		outerPanel.Color == innerPanel.Color {
		t.Fatalf("关闭态面板未形成外阴影和内表面: outer=%+v inner=%+v", outerPanel, innerPanel)
	}
	wantSelectionX := originX + 2*(hotbarSlotSize+hotbarSlotGap) - hotbarSelectBorder
	var selection hotbarInstance
	for _, quad := range got.quads {
		if quad.X == wantSelectionX && quad.Y == originY-hotbarSelectBorder &&
			quad.Width == hotbarSlotSize+2*hotbarSelectBorder && quad.Height == hotbarSlotSize+2*hotbarSelectBorder {
			selection = quad
			break
		}
	}
	if selection.Width == 0 {
		t.Fatal("未找到包住栏位 2 的外扩选中框")
	}
	selectedX := originX + 2*(hotbarSlotSize+hotbarSlotGap)
	foundInner := false
	for _, quad := range got.quads {
		if quad.X > selectedX && quad.Y > originY &&
			quad.X+quad.Width < selectedX+hotbarSlotSize && quad.Y+quad.Height < originY+hotbarSlotSize &&
			quad.Color != selection.Color {
			foundInner = true
			break
		}
	}
	if !foundInner {
		t.Fatal("选中格未形成内缩强调边框")
	}
}

// Mutation killed: swapping item colors, drawing swatches for empty slots, or
// emitting more than two digits per slot breaks the fixed HUD budget.
func TestClosedHotbarDrawsItemSwatchesAndCounts(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	inventory.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemDirt, Count: 9}
	inventory.Hotbar.Slots[8] = core.ItemStack{Item: core.ItemGrass, Count: 1}

	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, inventory, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
	if len(got.quads) != crosshairQuads+4+core.HotbarSlots+3*2 {
		t.Fatalf("quads=%d，想要准星、双层面板、双层选中框、9 个栏位和 3 个双层色块", len(got.quads))
	}
	tiles := got.quads[len(got.quads)-3*2:]
	wantItems := []core.ItemID{core.ItemStone, core.ItemDirt, core.ItemGrass}
	for index, item := range wantItems {
		border, face := tiles[index*2], tiles[index*2+1]
		assertHotbarItemFace(t, face, item)
		if border.Width != hotbarSlotSize-2*hotbarSwatchInset ||
			face.Width != border.Width-2*hotbarSwatchBorder {
			t.Fatalf("色块 %d 尺寸 = %f/%f", index, border.Width, face.Width)
		}
	}
	if len(got.glyphs) != 6 {
		t.Fatalf("数字数量 = %d，想要 64/9 各含阴影且隐藏 1，共 6 个实例", len(got.glyphs))
	}
}

// 杀死变异：继续显示单件数量、漏掉阴影、错序或失去右下对齐都会改变这些实例。
func TestHotbarCountsHideOneAndUseShadowedBottomRightDigits(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	appendHotbarCount(&layout, atlas, 1, 100, 200)
	if len(layout.glyphs) != 0 {
		t.Fatalf("单件数量 glyphs=%d，想要隐藏冗余数字 1", len(layout.glyphs))
	}

	appendHotbarCount(&layout, atlas, 64, 100, 200)
	if len(layout.glyphs) != 4 {
		t.Fatalf("数量 64 glyphs=%d，想要两个阴影加两个前景", len(layout.glyphs))
	}
	want := []hotbarInstance{
		{X: 134, Y: 236, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: [4]float32{0.02, 0.025, 0.03, 0.95}},
		{X: 139, Y: 236, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: [4]float32{0.02, 0.025, 0.03, 0.95}},
		{X: 133, Y: 235, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: [4]float32{1, 0.94, 0.78, 1}},
		{X: 138, Y: 235, Width: 8, Height: 12, U0: 0.1, V0: 0.2, U1: 0.3, V1: 0.4, Color: [4]float32{1, 0.94, 0.78, 1}},
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

// 杀死变异：移除区域面板、物品暗边或退回平面色块会破坏 HUD 的统一层级。
func TestClosedHotbarUsesInsetItemTiles(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemGrass, Count: 1}

	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, inventory, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
	if len(got.quads) != crosshairQuads+15 {
		t.Fatalf("快捷栏 quads=%d，想要准星、双层面板、双层选中框、9 个栏位和双层物品色块共 %d", len(got.quads), crosshairQuads+15)
	}
	border, face := got.quads[len(got.quads)-2], got.quads[len(got.quads)-1]
	if border.Width <= face.Width || border.Height <= face.Height || border.Color == face.Color {
		t.Fatalf("物品双层色块 border=%+v face=%+v", border, face)
	}
	assertHotbarItemFace(t, face, core.ItemGrass)
}

// 杀死变异：超过固定 HUD 容量会溢出预分配上传区。
func TestHotbarLayoutStaysWithinFixedCapacity(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	chatLine := strings.Repeat("中", maxChatRunes)
	chat := ChatOverlay{Open: true, Input: chatLine,
		Lines: []string{chatLine, chatLine, chatLine, chatLine, chatLine, chatLine}}

	// 关闭分支包含准星、双层面板/选中、九格双层物品、九条磨损耐久和最坏不可采
	// 采掘形状；它与打开分支互斥，不能相加进固定容量。
	layoutInventory(
		&layout, atlas, maxQuadTestInventory(), false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, false, 1280, 800)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1}, false, 1280, 800)
	appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, false, 1280, 800)
	appendChatOverlay(&layout, atlas, chat, 1280, 800)
	closedWant := closedHotbarQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(layout.quads) != closedWant || closedWant != 100 ||
		len(layout.quads) > maxHotbarQuads {
		t.Fatalf("关闭分支 quads=%d，分支公式/总上限=%d/%d", len(layout.quads),
			closedWant, maxHotbarQuads)
	}

	// 十行固定配方已被合成网格替换；箱子 27 格是打开分支的 quad 最大 overlay，
	// 并以合法来源高亮见证第二个选中实例；打开态以准星加 257 见证合法最大，
	// 重钉后的固定容量 320 仍保留可观余量。
	layoutInventory(
		&layout, atlas, maxQuadTestInventory(), true, 5, nil, nil, fullChestOverlay(), MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, true, 1280, 800)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1}, true, 1280, 800)
	appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, true, 1280, 800)
	appendChatOverlay(&layout, atlas, chat, 1280, 800)
	openWant := openInventoryQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(layout.quads) != openWant || openWant != 261 || len(layout.quads) > maxHotbarQuads {
		t.Fatalf("打开分支 quads=%d，想要 261 且不超过固定上限 %d", len(layout.quads), maxHotbarQuads)
	}

	// glyph 上限由 36 格两位数量、满箱两位数量与七行聊天共同见证（打开分支），
	// 关闭分支另以满聊天加最长弹条见证，两分支都不得突破重钉后的固定容量。
	layoutInventory(&layout, atlas, fullTestInventory(), true, 5, nil, nil, fullChestOverlay(), MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, true, 1280, 800)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: 0}, true, 1280, 800)
	appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, true, 1280, 800)
	appendChatOverlay(&layout, atlas, chat, 1280, 800)
	openGlyphWant := core.InventorySlots*4 + maxOverlayGlyphs + maxChatGlyphs
	if len(layout.glyphs) != openGlyphWant || openGlyphWant > maxHotbarGlyphs {
		t.Fatalf("glyph 打开分支见证=%d，分支公式=%d，固定上限=%d",
			len(layout.glyphs), openGlyphWant, maxHotbarGlyphs)
	}
	closedGlyphWant := core.HotbarSlots*4 + maxChatGlyphs + popupGlyphs
	layoutInventory(&layout, atlas, fullTestInventory(), false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, false, 1280, 800)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: 0}, false, 1280, 800)
	appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, false, 1280, 800)
	appendChatOverlay(&layout, atlas, chat, 1280, 800)
	appendPopupOverlay(&layout, atlas, PopupOverlay{
		Text: strings.Repeat("界", maxPopupRunes), ShownAtTick: 1, WorldTick: 1, Valid: true,
	}, false, 1280, 800)
	if len(layout.glyphs) != closedGlyphWant || closedGlyphWant > maxHotbarGlyphs {
		t.Fatalf("glyph 关闭分支见证=%d，分支公式=%d，固定上限=%d",
			len(layout.glyphs), closedGlyphWant, maxHotbarGlyphs)
	}
	if len(layout.quads) > maxHotbarQuads {
		t.Fatalf("glyph 见证 quads=%d，超过固定上限 %d", len(layout.quads), maxHotbarQuads)
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
			appendDurabilityBar(&layout, 0, test.stack, 1920, 1080)
			if got := len(layout.quads); got != test.want {
				t.Fatalf("quad 数量 = %d，想要 %d", got, test.want)
			}
		})
	}
}

// 杀死变异：固定宽度或整数相除会让低耐久填充条不再正且短于高耐久。
func TestDurabilityBarFillTracksRemaining(t *testing.T) {
	full, _ := core.ItemMaxDurability(core.ItemIronPickaxe)
	var low, high hotbarLayout
	appendDurabilityBar(&low, 0, core.ItemStack{
		Item: core.ItemIronPickaxe, Count: 1, Durability: 1,
	}, 1920, 1080)
	appendDurabilityBar(&high, 0, core.ItemStack{
		Item: core.ItemIronPickaxe, Count: 1, Durability: full - 1,
	}, 1920, 1080)

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

// 杀死变异：遍历全部 36 格或使用 open 几何会让背包工具也出条或位置漂移。
func TestDurabilityBarLayoutUsesOnlyHotbarGeometry(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	full, _ := core.ItemMaxDurability(core.ItemStonePickaxe)
	base := core.Inventory{}
	base.Hotbar.Slots[3] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full}
	base.Backpack[0] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: full}
	hotbarWorn := base
	hotbarWorn.Hotbar.Slots[3].Durability--
	backpackWorn := base
	backpackWorn.Backpack[0].Durability--

	var layout hotbarLayout
	closedBase := len(layoutInventory(
		&layout, atlas, base, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	).quads)
	closed := layoutInventory(
		&layout, atlas, hotbarWorn, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	if len(closed.quads) != closedBase+2 {
		t.Fatalf("关闭背包的磨损工具 quads=%d，想要 %d", len(closed.quads), closedBase+2)
	}
	closedBars := [2]hotbarInstance{closed.quads[len(closed.quads)-2], closed.quads[len(closed.quads)-1]}

	openBase := len(layoutInventory(
		&layout, atlas, base, true, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	).quads)
	open := layoutInventory(
		&layout, atlas, hotbarWorn, true, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	if len(open.quads) != openBase+2 {
		t.Fatalf("打开背包的快捷栏磨损工具 quads=%d，想要 %d", len(open.quads), openBase+2)
	}
	// 耐久条画在容器叠加区之前；空个人合成区是「面板 + 5 凹槽 + 标题」共 7 个 quad。
	barStart := len(open.quads) - 7 - 2
	openBars := [2]hotbarInstance{open.quads[barStart], open.quads[barStart+1]}
	for index, test := range []struct {
		open bool
		bars [2]hotbarInstance
	}{{false, closedBars}, {true, openBars}} {
		scale := hudScale(test.open, 1280, 800)
		slotX, slotY := inventorySlotOrigin(3, test.open, 1280, 800)
		wantX := slotX + durabilityBarInset*scale
		wantY := slotY + (hotbarSlotSize-durabilityBarInset-durabilityBarHeight)*scale
		wantWidth := (hotbarSlotSize - 2*durabilityBarInset) * scale
		if test.bars[0].X != wantX || test.bars[0].Y != wantY ||
			test.bars[0].Width != wantWidth || test.bars[0].Height != durabilityBarHeight*scale ||
			test.bars[1].X != wantX || test.bars[1].Y != wantY || test.bars[1].Width <= 0 ||
			test.bars[1].Width >= wantWidth {
			t.Fatalf("状态 %d 的快捷栏耐久条未复用统一几何: %+v", index, test.bars)
		}
	}
	if got := len(layoutInventory(
		&layout, atlas, backpackWorn, true, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	).quads); got != openBase {
		t.Fatalf("背包栏磨损工具 quads=%d，想要 %d", got, openBase)
	}
}

// TestMiningOverlayUsesStateSpecificGeometry 验证采掘状态不依赖颜色：删掉亮色末端
// 标记、警示缺口或进度钳制都会改变轨道内的矩形序列。
func TestMiningOverlayUsesStateSpecificGeometry(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	baseQuads := len(layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	).quads)
	if baseQuads != crosshairQuads+4+core.HotbarSlots {
		t.Fatalf("inactive quads=%d，想要准星、双层面板、双层选中框和快捷栏", baseQuads)
	}
	requiredZero := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	if len(requiredZero.quads) != baseQuads {
		t.Fatalf("required=0 quads=%d，想要 %d", len(requiredZero.quads), baseQuads)
	}
	zeroProgress := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, RequiredTicks: 15, Harvestable: true}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	if len(zeroProgress.quads) != baseQuads+1 {
		t.Fatalf("0%% 进度 quads=%d，想要只有轨道", len(zeroProgress.quads))
	}

	green := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15, Harvestable: true}, EatingOverlay{},
		CrosshairOverlay{Visible: true}, 1280, 800,
	)
	if len(green.quads) != baseQuads+3 {
		t.Fatalf("active 6/15 quads=%d，想要轨道、填充和亮色末端标记", len(green.quads))
	}
	background, fill, cap := green.quads[len(green.quads)-3], green.quads[len(green.quads)-2], green.quads[len(green.quads)-1]
	if background.X != 520 || background.Y != 678 ||
		background.Width != 240 || background.Height != 12 ||
		background.Color != ([4]float32{0.05, 0.05, 0.06, 0.78}) {
		t.Fatalf("采掘条背景=%+v", background)
	}
	if fill.X != background.X || fill.Y != background.Y ||
		fill.Width != 96 || fill.Height != background.Height ||
		fill.Color != ([4]float32{0.30, 0.78, 0.36, 0.95}) {
		t.Fatalf("可掉落 6/15 填充=%+v", fill)
	}
	if cap.Width != 8 || cap.Height != background.Height || cap.X != fill.X+fill.Width-cap.Width ||
		cap.X < background.X || cap.X+cap.Width > background.X+background.Width {
		t.Fatalf("可掉落 6/15 末端标记=%+v，不在填充末端或轨道内", cap)
	}

	orange := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	if len(orange.quads) != baseQuads+5 {
		t.Fatalf("不可掉落 6/15 quads=%d，想要轨道、填充和三个固定警示缺口", len(orange.quads))
	}
	blocked := orange.quads[len(orange.quads)-5:]
	if got := blocked[1].Color; got != ([4]float32{0.95, 0.55, 0.15, 0.95}) {
		t.Fatalf("不可掉落颜色=%v", got)
	}
	for index, wantX := range []float32{577, 637, 697} {
		notch := blocked[index+2]
		if notch.X != wantX || notch.Width != 6 || notch.Height != background.Height {
			t.Fatalf("警示缺口 %d=%+v，想要 X=%v、宽 6 且高度与轨道相同", index, notch, wantX)
		}
	}
	for index := range blocked {
		if blocked[index].X == cap.X && blocked[index].Width == cap.Width {
			t.Fatalf("不可掉落使用了可采末端标记: %+v", blocked[index])
		}
	}
	if len(green.quads[baseQuads:]) == len(blocked) {
		t.Fatal("忽略 RGB 后可采与不可采中段采掘几何相同")
	}

	clamped := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 30, RequiredTicks: 15, Harvestable: true}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	)
	clampedFill := clamped.quads[len(clamped.quads)-2]
	clampedCap := clamped.quads[len(clamped.quads)-1]
	if clampedFill.Width != background.Width || clampedCap.X+clampedCap.Width > background.X+background.Width {
		t.Fatalf("超额进度未钳制在轨道内: fill=%+v cap=%+v track=%+v", clampedFill, clampedCap, background)
	}

	openWithoutMining := len(layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800,
	).quads)
	openWithMining := len(layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15, Harvestable: true}, EatingOverlay{},
		CrosshairOverlay{Visible: true}, 1280, 800,
	).quads)
	if openWithMining != openWithoutMining {
		t.Fatalf("打开背包仍绘制采掘条: quads=%d，想要 %d", openWithMining, openWithoutMining)
	}
}

// Mutation killed: accepting an invalid authoritative value or a degenerate
// framebuffer would emit instances for a state the server never confirmed.
func TestHotbarLayoutRejectsInvalidStateAndEmptyFramebuffer(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	invalid := core.Inventory{Hotbar: core.Hotbar{Selected: core.HotbarSlots}}
	var layout hotbarLayout
	if got := layoutInventory(&layout, atlas, invalid, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 800, 600); len(got.quads) != 0 {
		t.Fatalf("非法物品状态 quads=%d，想要 0", len(got.quads))
	}
	if got := layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 0, 600); len(got.quads) != 0 {
		t.Fatalf("零宽 framebuffer quads=%d，想要 0", len(got.quads))
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
	got := layoutInventory(&layout, atlas, inventory, true, 12, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)

	// 准星 + 外框、背包区、快捷栏区与分隔线 + 选中框 + 来源高亮 + 36 格 +
	// 空个人合成区（面板、5 个凹槽与标题）。
	if len(got.quads) != crosshairQuads+openInventoryPanelQuads+2+core.InventorySlots+7 {
		t.Fatalf("打开时 quads=%d，想要 %d", len(got.quads), crosshairQuads+openInventoryPanelQuads+2+core.InventorySlots+7)
	}
	panels := got.quads[crosshairQuads:][:openInventoryPanelQuads]
	if panels[1].Y >= panels[2].Y || panels[1].Color == panels[2].Color || panels[3].Height <= 0 {
		t.Fatalf("背包分组面板不清晰: %+v", panels)
	}
	selectedX, selectedY := inventorySlotOrigin(0, true, 1280, 800)
	scale := hudScale(true, 1280, 800)
	selectBorder := hotbarSelectBorder * scale
	foundSelection := false
	for _, quad := range got.quads {
		if quad.X == selectedX-selectBorder && quad.Y == selectedY-selectBorder &&
			quad.Width == (hotbarSlotSize+2*hotbarSelectBorder)*scale && quad.Height == (hotbarSlotSize+2*hotbarSelectBorder)*scale {
			if quad.Color != ([4]float32{1, 0.72, 0.24, 0.98}) {
				t.Fatalf("打开态选中格颜色=%v，想要保持既有容器视觉", quad.Color)
			}
			foundSelection = true
			break
		}
	}
	if !foundSelection {
		t.Fatal("未找到打开态选中格")
	}
	_, hotbarY, _, _ := hotbarRowBounds(true, 1280, 800)
	for slot := range core.InventorySlots {
		x, y := inventorySlotOrigin(slot, true, 1280, 800)
		if slot < core.HotbarSlots && y != hotbarY {
			t.Fatalf("快捷栏格 %d 不在底行: y=%f", slot, y)
		}
		if slot >= core.HotbarSlots && y >= hotbarY {
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
	x, y := inventorySlotOrigin(0, true, 1280, 800)
	if _, ok := InventorySlotAt(float64(x)-1, float64(y)+1, 1280, 800); ok {
		t.Fatal("格子左侧 1 像素被判为命中")
	}
	if _, ok := InventorySlotAt(float64(x)+1, float64(y)+1, 0, 0); ok {
		t.Fatal("零尺寸 framebuffer 被判为命中")
	}
}

// TestInventorySlotOriginKeepsOpenHotbarScale 防止关闭态共享边界误改打开态的
// 快捷栏行；后者必须继续使用完整容器的缩放比例，保证既有命中几何不漂移。
func TestInventorySlotOriginKeepsOpenHotbarScale(t *testing.T) {
	const width, height = float32(640), float32(360)
	scale := hudScale(true, width, height)
	total := (core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap) * scale
	wantX := (width - total) * 0.5
	wantY := height - (hotbarBottomMargin+2*(healthHeartSize+statusBarGap)+hotbarSlotSize)*scale
	gotX, gotY := inventorySlotOrigin(0, true, width, height)
	if gotX != wantX || gotY != wantY {
		t.Fatalf("打开态快捷栏原点=(%v,%v)，想要容器缩放后的 (%v,%v)", gotX, gotY, wantX, wantY)
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
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, workbench, nil, nil, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 640, 360)
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

// TestResponsiveStatusFitsAndAvoidsOpenInventory 防止状态行在打开容器时继续使用
// 关闭态缩放，覆盖 36 个保持可命中的权威物品格。
func TestResponsiveStatusFitsAndAvoidsOpenInventory(t *testing.T) {
	for _, size := range [][2]float32{{1280, 720}, {640, 360}, {240, 40}} {
		var status hotbarLayout
		appendHealthBar(&status, HealthOverlay{Confirmed: true, Value: 7}, true, size[0], size[1])
		appendOxygenBar(&status, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1}, true, size[0], size[1])
		appendHungerBar(&status, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, true, size[0], size[1])
		if len(status.quads) == 0 {
			t.Fatalf("framebuffer %v：打开态低血/耗氧状态行不可见", size)
		}
		for index, quad := range status.quads {
			if quad.X < 0 || quad.Y < 0 || quad.X+quad.Width > size[0] || quad.Y+quad.Height > size[1] {
				t.Fatalf("framebuffer %v：状态 quad %d 越界: %+v", size, index, quad)
			}
			for slot := range core.InventorySlots {
				left, top := inventorySlotOrigin(slot, true, size[0], size[1])
				slotSize := hotbarSlotSize * hudScale(true, size[0], size[1])
				if rectanglesIntersect(quad, hotbarInstance{X: left, Y: top, Width: slotSize, Height: slotSize}) {
					t.Fatalf("framebuffer %v：状态 quad %d 与可命中格 %d 相交: %+v", size, index, slot, quad)
				}
			}
		}
	}
}

// TestStatusBarsAlignToHotbarEdgesAndStackOxygenOutward 锁定主状态行与快捷栏
// 两端的精确关系，以及氧气沿饥饿右边缘向快捷栏外侧堆叠的方向和行距。
func TestStatusBarsAlignToHotbarEdgesAndStackOxygenOutward(t *testing.T) {
	const width, height = float32(1280), float32(800)
	for _, open := range []bool{false, true} {
		left, hotbarY, totalWidth, scale := hotbarRowBounds(open, width, height)
		right := left + totalWidth
		barWidth := (healthSegmentCount*healthHeartSize +
			(healthSegmentCount-1)*healthHeartGap) * scale
		rowStep := (healthHeartSize + statusBarGap) * scale
		primaryY := hotbarY - rowStep
		oxygenY := primaryY - rowStep
		if open {
			primaryY = hotbarY + (hotbarSlotSize+statusBarGap)*scale
			oxygenY = primaryY + rowStep
		}

		var health, oxygen, hunger hotbarLayout
		appendHealthBar(&health, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, open, width, height)
		appendOxygenBar(&oxygen, OxygenOverlay{Confirmed: true, Value: 0}, open, width, height)
		appendHungerBar(&hunger, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, open, width, height)
		if len(health.quads) != healthQuads || len(oxygen.quads) != oxygenQuads || len(hunger.quads) != hungerQuads {
			t.Fatalf("open=%t health/oxygen/hunger quads=%d/%d/%d，想要 %d/%d/%d",
				open, len(health.quads), len(oxygen.quads), len(hunger.quads), healthQuads, oxygenQuads, hungerQuads)
		}
		if got := health.quads[0].X; got != left {
			t.Fatalf("open=%t health 左沿=%v，想要快捷栏左沿 %v", open, got, left)
		}
		if got := health.quads[len(health.quads)-1].X + health.quads[len(health.quads)-1].Width; got < left+barWidth-0.001 || got > left+barWidth+0.001 {
			t.Fatalf("open=%t health 右沿=%v，想要 %v", open, got, left+barWidth)
		}
		if got := hunger.quads[0].X + hunger.quads[0].Width; got != right {
			t.Fatalf("open=%t hunger 右沿=%v，想要快捷栏右沿 %v", open, got, right)
		}
		if got := oxygen.quads[len(oxygen.quads)-1].X + oxygen.quads[len(oxygen.quads)-1].Width; got != right {
			t.Fatalf("open=%t oxygen 右沿=%v，想要 hunger/快捷栏右沿 %v", open, got, right)
		}
		for index, quad := range health.quads {
			if quad.Y != primaryY {
				t.Fatalf("open=%t health %d Y=%v，想要主行 %v", open, index, quad.Y, primaryY)
			}
		}
		for index, quad := range hunger.quads {
			if quad.Y != primaryY {
				t.Fatalf("open=%t hunger %d Y=%v，想要主行 %v", open, index, quad.Y, primaryY)
			}
		}
		for index, quad := range oxygen.quads {
			if quad.Y != oxygenY {
				t.Fatalf("open=%t oxygen %d Y=%v，想要向外次行 %v", open, index, quad.Y, oxygenY)
			}
		}
	}

	const narrowWidth = float32(480)
	wantScale := (narrowWidth - 2*hudEdgeMargin) /
		(core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap + 2*hotbarPanelPadding)
	if got := hudScale(false, narrowWidth, 1000); got != wantScale {
		t.Fatalf("窄窗口 scale=%v，想要只由既有快捷栏宽度得出 %v", got, wantScale)
	}
}

// TestStatusGeometryDoesNotChangeWhenOxygenHides 证明氧气零实例不改变主状态行。
func TestStatusGeometryDoesNotChangeWhenOxygenHides(t *testing.T) {
	layoutFor := func(oxygen OxygenOverlay) (health, hunger []hotbarInstance) {
		var layout hotbarLayout
		appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: 7}, false, 1280, 800)
		health = append(health, layout.quads...)
		appendOxygenBar(&layout, oxygen, false, 1280, 800)
		start := len(layout.quads)
		appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: 9}, false, 1280, 800)
		hunger = append(hunger, layout.quads[start:]...)
		return health, hunger
	}
	depletedHealth, depletedHunger := layoutFor(OxygenOverlay{Confirmed: true, Value: 0})
	fullHealth, fullHunger := layoutFor(OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks})
	if !reflect.DeepEqual(depletedHealth, fullHealth) || !reflect.DeepEqual(depletedHunger, fullHunger) {
		t.Fatalf("满氧隐藏导致主状态行跳动: depleted=%+v/%+v full=%+v/%+v",
			depletedHealth, depletedHunger, fullHealth, fullHunger)
	}
}

// TestClosedMiningAndChatAnchorAboveReservedOxygenRow 锁定满氧隐藏时仍永久预留
// 次状态行：采掘和聊天都必须锚在完整两行栈上方，不能跟随气泡实例动态折叠。
func TestClosedMiningAndChatAnchorAboveReservedOxygenRow(t *testing.T) {
	const width, height = float32(1280), float32(800)
	_, hotbarY, _, scale := hotbarRowBounds(false, width, height)
	rowStep := (healthHeartSize + statusBarGap) * scale
	oxygenY := hotbarY - 2*rowStep

	var mining hotbarLayout
	appendMiningBar(&mining, MiningOverlay{
		Active: true, ProgressTicks: 4, RequiredTicks: 9,
	}, width, height)
	if len(mining.quads) != miningBarQuads+miningWarningNotches {
		t.Fatalf("mining quads=%d，想要 %d", len(mining.quads), miningBarQuads+miningWarningNotches)
	}
	wantMiningY := oxygenY - (miningBarGap+miningBarHeight)*scale
	if got := mining.quads[0].Y; got != wantMiningY {
		t.Fatalf("mining Y=%v，想要完整两行状态栈上方 %v", got, wantMiningY)
	}

	atlas := newFakeNameTagAtlas()
	var chat hotbarLayout
	appendChatOverlay(&chat, atlas, ChatOverlay{Lines: []string{"状态栈上方"}}, width, height)
	if len(chat.quads) != 1 {
		t.Fatalf("chat panels=%d，想要 1", len(chat.quads))
	}
	wantChatBottom := oxygenY - chatHealthClearance*scale
	if got := chat.quads[0].Y + chat.quads[0].Height; got != wantChatBottom {
		t.Fatalf("chat bottom=%v，想要完整两行状态栈 clearance %v", got, wantChatBottom)
	}
}

// TestOpenStatusStackAvoidsInventoryHitCellsAndRecipes 锁定打开态两行向下外扩，
// 并穷举 36 个真实命中格与合成网格、产物格区域验证互不相交。
func TestOpenStatusStackAvoidsInventoryHitCellsAndRecipes(t *testing.T) {
	for _, size := range [][2]float32{{1280, 800}, {640, 360}, {240, 40}} {
		width, height := size[0], size[1]
		_, hotbarY, _, scale := hotbarRowBounds(true, width, height)
		primaryY := hotbarY + (hotbarSlotSize+statusBarGap)*scale
		oxygenY := primaryY + (healthHeartSize+statusBarGap)*scale
		var status hotbarLayout
		appendHealthBar(&status, HealthOverlay{Confirmed: true, Value: 5}, true, width, height)
		appendOxygenBar(&status, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks / 3}, true, width, height)
		appendHungerBar(&status, HungerOverlay{Confirmed: true, Value: 9}, true, width, height)
		for index, quad := range status.quads {
			wantY := primaryY
			if index >= healthQuads && index < healthQuads+oxygenQuads {
				wantY = oxygenY
			}
			if quad.Y != wantY {
				t.Fatalf("framebuffer %v status quad %d Y=%v，想要 %v", size, index, quad.Y, wantY)
			}
			if quad.X < 0 || quad.Y < 0 || quad.X+quad.Width > width || quad.Y+quad.Height > height {
				t.Fatalf("framebuffer %v status quad %d 越界: %+v", size, index, quad)
			}
			for slot := range core.InventorySlots {
				left, top := inventorySlotOrigin(slot, true, width, height)
				cell := hotbarInstance{X: left, Y: top, Width: hotbarSlotSize * scale, Height: hotbarSlotSize * scale}
				if hit, ok := InventorySlotAt(float64(left+cell.Width/2), float64(top+cell.Height/2), uint32(width), uint32(height)); !ok || hit != uint8(slot) {
					t.Fatalf("framebuffer %v inventory slot %d 中心命中=%d,%t", size, slot, hit, ok)
				}
				if rectanglesIntersect(quad, cell) {
					t.Fatalf("framebuffer %v status quad %d 与 inventory slot %d 相交: %+v / %+v", size, index, slot, quad, cell)
				}
			}
			for slot := range core.CraftingGridSlots {
				left, top := craftingGridSlotOrigin(slot, 3, width, height)
				craft := hotbarInstance{
					X: left, Y: top,
					Width: hotbarSlotSize * scale, Height: hotbarSlotSize * scale,
				}
				if rectanglesIntersect(quad, craft) {
					t.Fatalf("framebuffer %v status quad %d 与合成网格格 %d 相交: %+v / %+v", size, index, slot, quad, craft)
				}
			}
			outputLeft, outputTop := craftingOutputOrigin(3, width, height)
			output := hotbarInstance{
				X: outputLeft, Y: outputTop,
				Width: hotbarSlotSize * scale, Height: hotbarSlotSize * scale,
			}
			if rectanglesIntersect(quad, output) {
				t.Fatalf("framebuffer %v status quad %d 与产物格相交: %+v / %+v", size, index, quad, output)
			}
		}
	}
}

// TestInventorySlotAtBoundariesRemainHalfOpen 锁定改动前的左上闭、右下开命中语义。
func TestInventorySlotAtBoundariesRemainHalfOpen(t *testing.T) {
	for _, size := range [][2]uint32{{1280, 720}, {640, 360}, {240, 40}} {
		scale := hudScale(true, float32(size[0]), float32(size[1]))
		slotSize := hotbarSlotSize * scale
		for slot := range core.InventorySlots {
			left, top := inventorySlotOrigin(slot, true, float32(size[0]), float32(size[1]))
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
				return inventorySlotOrigin(slot, true, float32(width), float32(height))
			}, InventorySlotAt},
			{"熔炉", core.FurnaceViewSlots, func(slot int) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, true, float32(width), float32(height))
				}
				return recipeSlotOrigin(slot-core.InventorySlots, float32(width), float32(height))
			}, FurnaceSlotAt},
			{"箱子", core.ChestViewSlots, func(slot int) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, true, float32(width), float32(height))
				}
				return chestSlotOrigin(slot-core.InventorySlots, float32(width), float32(height))
			}, ChestSlotAt},
		} {
			t.Run(test.name, func(t *testing.T) {
				slotSize := hotbarSlotSize * hudScale(true, float32(width), float32(height))
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

// TestContainerHeaderAvoidsHitCells 验证 header 只扩面板上沿，标题与所有可交互
// 矩形保持分离，窄高与矮宽窗口仍由同一个 `hudScale` 收缩。
func TestContainerHeaderAvoidsHitCells(t *testing.T) {
	baseOpenHUDHeight := hotbarBottomMargin + hotbarSlotSize +
		inventoryRowGap + 3*hotbarSlotSize + 2*hotbarSlotGap +
		recipeRowGap + overlayAreaRows*hotbarSlotSize + (overlayAreaRows-1)*hotbarSlotGap +
		hotbarPanelPadding + 2*(healthHeartSize+statusBarGap)
	if openHUDHeight-baseOpenHUDHeight != containerHeaderHeight {
		t.Fatalf("openHUDHeight header=%v，想要 %v", openHUDHeight-baseOpenHUDHeight, containerHeaderHeight)
	}

	atlas := newFakeNameTagAtlas()
	for _, size := range [][2]float32{{1280, 800}, {240, 40}, {800, 17}} {
		for _, view := range []struct {
			name     string
			crafting *CraftingOverlay
			overlay  *FurnaceOverlay
			chest    *ChestOverlay
			count    int
			origin   func(int, float32, float32) (float32, float32)
			panel    func(float32, float32) hotbarInstance
		}{
			{"合成", &CraftingOverlay{Size: 3}, nil, nil, core.InventorySlots, func(slot int, width, height float32) (float32, float32) {
				return inventorySlotOrigin(slot, true, width, height)
			}, func(width, height float32) hotbarInstance {
				scale := hudScale(true, width, height)
				left, top := craftingGridSlotOrigin(0, 3, width, height)
				_, bottom := craftingGridSlotOrigin(6, 3, width, height)
				outputX, _ := craftingOutputOrigin(3, width, height)
				padding := hotbarPanelPadding * scale
				return hotbarInstance{X: left - padding, Y: top - padding, Width: outputX + hotbarSlotSize*scale - left + 2*padding, Height: bottom + hotbarSlotSize*scale - top + 2*padding}
			}},
			{"熔炉", nil, &FurnaceOverlay{}, nil, core.FurnaceViewSlots, func(slot int, width, height float32) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, true, width, height)
				}
				return recipeSlotOrigin(slot-core.InventorySlots, width, height)
			}, func(width, height float32) hotbarInstance {
				scale := hudScale(true, width, height)
				panelX, slotY := recipeSlotOrigin(0, width, height)
				_, barTop := furnaceBarOrigin(width, height)
				padding := hotbarPanelPadding * scale
				return hotbarInstance{X: panelX - padding, Y: barTop - padding, Width: (3*hotbarSlotSize+2*hotbarSlotGap)*scale + 2*padding, Height: slotY + hotbarSlotSize*scale - barTop + 2*padding}
			}},
			{"箱子", nil, nil, &ChestOverlay{}, core.ChestViewSlots, func(slot int, width, height float32) (float32, float32) {
				if slot < core.InventorySlots {
					return inventorySlotOrigin(slot, true, width, height)
				}
				return chestSlotOrigin(slot-core.InventorySlots, width, height)
			}, func(width, height float32) hotbarInstance {
				scale := hudScale(true, width, height)
				left, bottom := chestSlotOrigin(0, width, height)
				_, top := chestSlotOrigin(core.ChestSlots-core.HotbarSlots, width, height)
				padding := hotbarPanelPadding * scale
				totalWidth := (core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap) * scale
				return hotbarInstance{X: left - padding, Y: top - padding, Width: totalWidth + 2*padding, Height: bottom + hotbarSlotSize*scale - top + 2*padding}
			}},
		} {
			var layout hotbarLayout
			got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, view.crafting, view.overlay, view.chest, MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, size[0], size[1])
			oldPanel := view.panel(size[0], size[1])
			panel := got.quads[crosshairQuads+openInventoryPanelQuads+1+core.InventorySlots]
			yDelta := panel.Y - (oldPanel.Y - containerHeaderHeight*got.scale)
			heightDelta := panel.Height - (oldPanel.Height + containerHeaderHeight*got.scale)
			bottomDelta := panel.Y + panel.Height - oldPanel.Y - oldPanel.Height
			if panel.X != oldPanel.X || panel.Width != oldPanel.Width || yDelta < -0.0001 || yDelta > 0.0001 ||
				heightDelta < -0.0001 || heightDelta > 0.0001 || bottomDelta < -0.0001 || bottomDelta > 0.0001 {
				t.Fatalf("framebuffer %v %s panel=%+v，旧 panel=%+v", size, view.name, panel, oldPanel)
			}
			title := got.quads[len(got.quads)-1]
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
		}
	}
}

func rectanglesIntersect(a, b hotbarInstance) bool {
	return a.X < b.X+b.Width && a.X+a.Width > b.X && a.Y < b.Y+b.Height && a.Y+a.Height > b.Y
}

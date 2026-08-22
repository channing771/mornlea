package hud

import (
	"reflect"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestClosedHotbarCentersNineSlotsWithTwoPanelLayersAndSelectionFrames 验证关闭态
// 快捷栏的非颜色轮廓：删掉任一面板或选中框层、或漂移中心几何都会失败。
func TestClosedHotbarCentersNineSlotsWithTwoPanelLayersAndSelectionFrames(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var inventory core.Inventory
	inventory.Hotbar.Selected = 2
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, inventory, false, -1, nil, nil, MiningOverlay{}, 800, 600)

	if len(got.quads) != 4+core.HotbarSlots {
		t.Fatalf("空物品状态 quads=%d，想要双层面板、双层选中框加 9 个栏位", len(got.quads))
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
	got := layoutInventory(&layout, atlas, inventory, false, -1, nil, nil, MiningOverlay{}, 1280, 800)
	if len(got.quads) != 4+core.HotbarSlots+3*2 {
		t.Fatalf("quads=%d，想要双层面板、双层选中框、9 个栏位和 3 个双层色块", len(got.quads))
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
	got := layoutInventory(&layout, atlas, inventory, false, -1, nil, nil, MiningOverlay{}, 1280, 800)
	if len(got.quads) != 15 {
		t.Fatalf("快捷栏 quads=%d，想要双层面板、双层选中框、9 个栏位和双层物品色块共 15", len(got.quads))
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

	// 关闭分支包含双层面板/选中、九格双层物品、九条磨损耐久和最坏不可采
	// 采掘形状；它与打开分支互斥，不能相加进固定容量。
	layoutInventory(
		&layout, atlas, maxQuadTestInventory(), false, -1, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, 1280, 800,
	)
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, false, 1280, 800)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1}, false, 1280, 800)
	appendChatOverlay(&layout, atlas, chat, 1280, 800)
	closedWant := closedHotbarQuads + healthQuads + oxygenQuads + maxChatQuads
	if len(layout.quads) != closedWant || closedWant != 76 ||
		len(layout.quads) > maxHotbarQuads {
		t.Fatalf("关闭分支 quads=%d，分支公式/总上限=%d/%d", len(layout.quads),
			closedWant, maxHotbarQuads)
	}

	// 十行固定配方是打开分支的 quad 最大 overlay，并以合法来源高亮见证第二个
	// 选中实例；较大互斥分支必须恰好见证总上限。
	layoutInventory(
		&layout, atlas, maxQuadTestInventory(), true, 5, nil, nil, MiningOverlay{}, 1280, 800,
	)
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, true, 1280, 800)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1}, true, 1280, 800)
	appendChatOverlay(&layout, atlas, chat, 1280, 800)
	openWant := openInventoryQuads + healthQuads + oxygenQuads + maxChatQuads
	if len(layout.quads) != openWant || openWant != 245 || len(layout.quads) > maxHotbarQuads {
		t.Fatalf("打开分支 quads=%d，想要 245 且不超过固定上限 %d", len(layout.quads), maxHotbarQuads)
	}

	// glyph 上限由 36 格两位数量、满箱两位数量与七行聊天共同见证。
	layoutInventory(&layout, atlas, fullTestInventory(), true, 5, nil, fullChestOverlay(), MiningOverlay{}, 1280, 800)
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: core.MaxHealth}, true, 1280, 800)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: 0}, true, 1280, 800)
	appendChatOverlay(&layout, atlas, chat, 1280, 800)
	if len(layout.glyphs) != maxHotbarGlyphs {
		t.Fatalf("glyph 上限见证=%d，想要 %d", len(layout.glyphs), maxHotbarGlyphs)
	}
	if len(layout.quads) > maxHotbarQuads {
		t.Fatalf("glyph 上限见证 quads=%d，超过固定上限 %d", len(layout.quads), maxHotbarQuads)
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
		&layout, atlas, base, false, -1, nil, nil, MiningOverlay{}, 1280, 800,
	).quads)
	closed := layoutInventory(
		&layout, atlas, hotbarWorn, false, -1, nil, nil, MiningOverlay{}, 1280, 800,
	)
	if len(closed.quads) != closedBase+2 {
		t.Fatalf("关闭背包的磨损工具 quads=%d，想要 %d", len(closed.quads), closedBase+2)
	}
	closedBars := [2]hotbarInstance{closed.quads[len(closed.quads)-2], closed.quads[len(closed.quads)-1]}

	openBase := len(layoutInventory(
		&layout, atlas, base, true, -1, nil, nil, MiningOverlay{}, 1280, 800,
	).quads)
	open := layoutInventory(
		&layout, atlas, hotbarWorn, true, -1, nil, nil, MiningOverlay{}, 1280, 800,
	)
	if len(open.quads) != openBase+2 {
		t.Fatalf("打开背包的快捷栏磨损工具 quads=%d，想要 %d", len(open.quads), openBase+2)
	}
	barStart := len(open.quads) - recipeQuads - 2
	openBars := [2]hotbarInstance{open.quads[barStart], open.quads[barStart+1]}
	if openBars != closedBars {
		t.Fatalf("打开/关闭背包的耐久条几何不同: open=%+v closed=%+v", openBars, closedBars)
	}
	if got := len(layoutInventory(
		&layout, atlas, backpackWorn, true, -1, nil, nil, MiningOverlay{}, 1280, 800,
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
		&layout, atlas, core.Inventory{}, false, -1, nil, nil, MiningOverlay{}, 1280, 800,
	).quads)
	if baseQuads != 4+core.HotbarSlots {
		t.Fatalf("inactive quads=%d，想要双层面板、双层选中框和快捷栏", baseQuads)
	}
	requiredZero := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6}, 1280, 800,
	)
	if len(requiredZero.quads) != baseQuads {
		t.Fatalf("required=0 quads=%d，想要 %d", len(requiredZero.quads), baseQuads)
	}
	zeroProgress := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil,
		MiningOverlay{Active: true, RequiredTicks: 15, Harvestable: true}, 1280, 800,
	)
	if len(zeroProgress.quads) != baseQuads+1 {
		t.Fatalf("0%% 进度 quads=%d，想要只有轨道", len(zeroProgress.quads))
	}

	green := layoutInventory(
		&layout, atlas, core.Inventory{}, false, -1, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15, Harvestable: true},
		1280, 800,
	)
	if len(green.quads) != baseQuads+3 {
		t.Fatalf("active 6/15 quads=%d，想要轨道、填充和亮色末端标记", len(green.quads))
	}
	background, fill, cap := green.quads[len(green.quads)-3], green.quads[len(green.quads)-2], green.quads[len(green.quads)-1]
	if background.X != 520 || background.Y != 680 ||
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
		&layout, atlas, core.Inventory{}, false, -1, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, 1280, 800,
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
		&layout, atlas, core.Inventory{}, false, -1, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 30, RequiredTicks: 15, Harvestable: true}, 1280, 800,
	)
	clampedFill := clamped.quads[len(clamped.quads)-2]
	clampedCap := clamped.quads[len(clamped.quads)-1]
	if clampedFill.Width != background.Width || clampedCap.X+clampedCap.Width > background.X+background.Width {
		t.Fatalf("超额进度未钳制在轨道内: fill=%+v cap=%+v track=%+v", clampedFill, clampedCap, background)
	}

	openWithoutMining := len(layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1, nil, nil, MiningOverlay{}, 1280, 800,
	).quads)
	openWithMining := len(layoutInventory(
		&layout, atlas, core.Inventory{}, true, -1, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15, Harvestable: true},
		1280, 800,
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
	if got := layoutInventory(&layout, atlas, invalid, false, -1, nil, nil, MiningOverlay{}, 800, 600); len(got.quads) != 0 {
		t.Fatalf("非法物品状态 quads=%d，想要 0", len(got.quads))
	}
	if got := layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, MiningOverlay{}, 0, 600); len(got.quads) != 0 {
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
	got := layoutInventory(&layout, atlas, inventory, true, 12, nil, nil, MiningOverlay{}, 1280, 800)

	// 外框、背包区、快捷栏区与分隔线 + 选中框 + 来源高亮 + 36 格 + 固定配方行。
	if len(got.quads) != openInventoryPanelQuads+2+core.InventorySlots+recipeQuads {
		t.Fatalf("打开时 quads=%d，想要 %d", len(got.quads), openInventoryPanelQuads+2+core.InventorySlots+recipeQuads)
	}
	panels := got.quads[:openInventoryPanelQuads]
	if panels[1].Y >= panels[2].Y || panels[1].Color == panels[2].Color || panels[3].Height <= 0 {
		t.Fatalf("背包分组面板不清晰: %+v", panels)
	}
	selectedX, selectedY := inventorySlotOrigin(0, true, 1280, 800)
	foundSelection := false
	for _, quad := range got.quads {
		if quad.X == selectedX-hotbarSelectBorder && quad.Y == selectedY-hotbarSelectBorder &&
			quad.Width == hotbarSlotSize+2*hotbarSelectBorder && quad.Height == hotbarSlotSize+2*hotbarSelectBorder {
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
	hotbarY := float32(800) - hotbarBottomMargin - hotbarSlotSize
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
	wantY := height - (hotbarBottomMargin+hotbarSlotSize)*scale
	gotX, gotY := inventorySlotOrigin(0, true, width, height)
	if gotX != wantX || gotY != wantY {
		t.Fatalf("打开态快捷栏原点=(%v,%v)，想要容器缩放后的 (%v,%v)", gotX, gotY, wantX, wantY)
	}
}

// 杀死变异：小窗口保持固定 48px 会让上方配方行落出 framebuffer；独立缩放命中则会漂移。
func TestOpenInventoryFitsAndHitsAt640x360(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, char := range hotbarDigits {
		atlas.glyphs[char] = fakeNameTagGlyph(7)
	}
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, true, -1, nil, nil, MiningOverlay{}, 640, 360)
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

	var firstButton hotbarInstance
	for _, quad := range got.quads {
		if quad.Height > 0 && quad.Width/quad.Height > 1.9 && quad.Width/quad.Height < 2.1 {
			firstButton = quad
			break
		}
	}
	if firstButton.Width == 0 {
		t.Fatal("未找到缩放后的合成按钮")
	}
	recipe, ok := RecipeButtonAt(
		float64(firstButton.X+firstButton.Width/2),
		float64(firstButton.Y+firstButton.Height/2),
		640, 360,
	)
	if !ok || recipe != core.RecipeStoneBricks {
		t.Fatalf("缩放按钮命中=%d,%v，想要石砖配方", recipe, ok)
	}
}

// TestResponsiveStatusFitsAndAvoidsOpenInventory 防止状态行在打开容器时继续使用
// 关闭态缩放，覆盖 36 个保持可命中的权威物品格。
func TestResponsiveStatusFitsAndAvoidsOpenInventory(t *testing.T) {
	for _, size := range [][2]float32{{1280, 720}, {640, 360}, {240, 40}} {
		var status hotbarLayout
		appendHealthBar(&status, HealthOverlay{Confirmed: true, Value: 7}, true, size[0], size[1])
		appendOxygenBar(&status, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1}, true, size[0], size[1])
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

func rectanglesIntersect(a, b hotbarInstance) bool {
	return a.X < b.X+b.Width && a.X+a.Width > b.X && a.Y < b.Y+b.Height && a.Y+a.Height > b.Y
}

package hud

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

const (
	// 打开态与关闭态互斥；分别列出合法上限，防止后续样式变化悄悄突破 benchmark
	// scenario v20 口径重钉后的固定上传容量（scenario 常量与文档的同步由
	// capture/benchmark 任务组落地）。准星在两种状态下都最先绘制，计入两个分支。
	// 打开态最坏由箱子视图见证：面板族 + 选中/来源 + 36 格双层物品 + 九条耐久
	// + 箱子 81 内容 quad + 悬停 tooltip 背景，实算分解见 panel_test.go 的
	// 固定预算测试。
	openInventoryQuads = crosshairQuads + containerPanelQuads + 2 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + maxOverlayQuads + tooltipQuads
	closedHotbarQuads = crosshairQuads + closedHotbarPanelQuads + closedHotbarSelectionQuads + core.HotbarSlots +
		core.HotbarSlots*2 + core.HotbarSlots*2 + miningBarQuads + miningWarningNotches
	maxHotbarQuads = 320
	// glyph 上限按「互斥分支取大 + 预留余量」计算：打开态见证 36 格两位数量、
	// 最大 overlay 数量与七行聊天；关闭态见证快捷栏两位数量、七行聊天与最长
	// 物品名弹条（32 rune 双层）。弹条被容器打开抑制（与打开态 overlay 数量
	// 不同帧），两分支不得相加。tooltip 预留 16 给容器悬停名称的双层字形，增
	// 长余量吸收实测最坏之后的自然波动；实测最坏由固定预算测试断言并记录。
	maxHotbarGlyphs = max(core.InventorySlots*4+maxOverlayGlyphs+maxChatGlyphs,
		core.HotbarSlots*4+maxChatGlyphs+popupGlyphs) + tooltipGlyphReserve + glyphGrowthMargin

	// tooltipGlyphReserve 是容器 tooltip（悬停物品名双层字形）的预留预算。
	tooltipGlyphReserve = 16
	// glyphGrowthMargin 是分支最坏之外的固定增长余量，与 quad 侧 320 容量的
	// 余量同源（弹条/准星落地后仍保留可观裕度）。
	glyphGrowthMargin = 52

	// 关闭态快捷栏以外阴影和内表面形成独立面板，选中格再用双层轮廓强调。
	closedHotbarPanelQuads     = 2
	closedHotbarSelectionQuads = 2
	miningBarQuads             = 2
	miningWarningNotches       = 3

	hotbarSlotSize      = float32(48)
	hotbarSlotGap       = float32(4)
	hotbarBottomMargin  = float32(6)
	hotbarSelectBorder  = float32(3)
	hotbarSelectInset   = float32(3)
	hotbarPanelPadding  = float32(6)
	hotbarPanelInset    = float32(2)
	hotbarSwatchInset   = float32(10)
	hotbarSwatchBorder  = float32(2)
	hotbarDigitMargin   = float32(3)
	hotbarDigitTracking = float32(-2)
	durabilityBarHeight = float32(3)
	durabilityBarInset  = float32(4)
	miningBarWidth      = float32(240)
	miningBarHeight     = float32(12)
	miningBarGap        = float32(16)
	miningBarCapWidth   = float32(8)
	miningNotchWidth    = float32(6)
	// 面板下段快捷栏行与背包三行之间的行距。
	inventoryRowGap = float32(12)

	hudEdgeMargin = float32(8)
	// 关闭态联合高度从 framebuffer 下沿覆盖快捷栏、状态行和最坏采掘轨道；
	// `hudScale` 用它保证快捷栏、永久两行状态栈和采掘轨道按同一比例缩小。
	// 末尾的 `popupTrackGap + popupRowHeight` 是物品名弹条行：轨道上沿之上
	// 6 px 间隙加 16 px 文字行，弹条纳入缩放高度防裁剪。
	closedHUDHeight = hotbarBottomMargin + hotbarSlotSize + 2*(statusBarGap+healthHeartSize) +
		miningBarGap + miningBarHeight + popupTrackGap + popupRowHeight
)

// hotbarDigits 是 HUD 需要的全部字形，登录后不再增长。
const hotbarDigits = "0123456789"

type hotbarInstance struct {
	X, Y, Width, Height float32
	U0, V0, U1, V1      float32
	Color               [4]float32
}
type hotbarLayout struct {
	quads  []hotbarInstance
	glyphs []hotbarInstance
	scale  float32
	open   bool
}

// layoutInventory 只依赖 framebuffer 尺寸、完整物品状态、界面开关与容器叠加值，
// 产出固定上限的实例；关闭时只有底部 9 格 HUD。overlay 与 chest 至多一个非 nil：
// overlay 非 nil 时画熔炉三格与两条进度条，chest 非 nil 时画箱子 27 格，
// 两者都为 nil 时画合成网格与产物格（crafting 为 nil 表示网格镜像尚未确认，
// 按空的个人 2×2 呈现）。mining 与 eating 是两条互斥的进度条叠加值
// （采掘激活时进食条让位，见 `appendEatingBar`），只在关闭态出现。
// crosshair 携带应用层的相位门控：准星与物品镜像无关，实例必须最先追加。
func layoutInventory(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	inventory core.Inventory,
	open bool,
	source int,
	crafting *CraftingOverlay,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
	mining MiningOverlay,
	eating EatingOverlay,
	crosshair CrosshairOverlay,
	width, height float32,
) hotbarLayout {
	if dst == nil {
		dst = &hotbarLayout{
			quads:  make([]hotbarInstance, 0, maxHotbarQuads),
			glyphs: make([]hotbarInstance, 0, maxHotbarGlyphs),
		}
	}
	dst.quads = dst.quads[:0]
	dst.glyphs = dst.glyphs[:0]
	dst.scale = hudScale(open, width, height)
	dst.open = open
	if width <= 0 || height <= 0 || !inventory.Valid() {
		return *dst
	}

	// 准星最先追加：容器面板后画覆盖准星（呈现层叠契约，见 `appendCrosshair`）。
	appendCrosshair(dst, crosshair, width, height)

	scale := dst.scale
	slotSize := hotbarSlotSize * scale
	selectBorder := hotbarSelectBorder * scale
	slots := core.HotbarSlots
	if open {
		slots = core.InventorySlots
	}
	// 面板族在准星之后、一切栏位之前追加；视图决定面板宽度（个人合成面板加
	// 右侧配方栏）与标题 cell，原点与各行锚点对四类视图一致。
	view := containerViewCrafting
	switch {
	case chest != nil:
		view = containerViewChest
	case overlay != nil:
		view = containerViewFurnace
	}
	if open {
		appendContainerPanel(dst, view, width, height)
	} else {
		appendClosedHotbarStrip(dst, width, height, scale)
	}
	// 高亮先于栏位表面绘制，栏位只覆盖内部并留下像素边框。
	selectedX, selectedY := inventorySlotOrigin(int(inventory.Hotbar.Selected), open, width, height)
	selectedColor := hotbarSelectedOuterColor
	if open {
		selectedColor = hotbarSelectedInnerColor
	}
	dst.quads = append(dst.quads, hotbarInstance{
		X:      selectedX - selectBorder,
		Y:      selectedY - selectBorder,
		Width:  slotSize + 2*selectBorder,
		Height: slotSize + 2*selectBorder,
		Color:  selectedColor,
	})
	if open && source >= 0 {
		if sourceX, sourceY, ok := containerSourceOrigin(source, crafting, overlay, chest, width, height); ok {
			dst.quads = append(dst.quads, hotbarInstance{
				X:      sourceX - selectBorder,
				Y:      sourceY - selectBorder,
				Width:  slotSize + 2*selectBorder,
				Height: slotSize + 2*selectBorder,
				Color:  containerSourceHighlightColor,
			})
		}
	}
	slotUV := hotbarTextureUV(hotbarContainerSlotColumn)
	for slot := range slots {
		x, y := inventorySlotOrigin(slot, open, width, height)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: slotSize, Height: slotSize,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
	if !open {
		inset := hotbarSelectInset * scale
		dst.quads = append(dst.quads, hotbarInstance{
			X: selectedX + inset, Y: selectedY + inset,
			Width: slotSize - 2*inset, Height: slotSize - 2*inset,
			Color: hotbarSelectedInnerColor,
		})
	}
	for slot := range slots {
		stack, _ := inventory.Slot(uint8(slot))
		if stack.Item == core.ItemNone {
			continue
		}
		x, y := inventorySlotOrigin(slot, open, width, height)
		appendItemTile(dst, stack.Item, x, y, scale)
	}
	for slot := range slots {
		stack, _ := inventory.Slot(uint8(slot))
		if stack.Item == core.ItemNone {
			continue
		}
		x, y := inventorySlotOrigin(slot, open, width, height)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, scale)
	}
	for slot, stack := range inventory.Hotbar.Slots {
		appendDurabilityBarScaled(dst, slot, stack, open, width, height, scale)
	}
	if open {
		switch {
		case chest != nil:
			appendChestContent(dst, atlas, *chest, width, height)
		case overlay != nil:
			appendFurnaceContent(dst, atlas, *overlay, width, height)
		default:
			grid := CraftingOverlay{}
			if crafting != nil {
				grid = *crafting
			}
			appendCraftingContent(dst, atlas, grid, width, height)
		}
	} else {
		appendMiningBar(dst, mining, width, height)
		appendEatingBar(dst, eating, mining, width, height)
	}
	return *dst
}

// appendClosedHotbarStrip 绘制关闭态快捷栏贴条：外阴影加内表面双层、无边，
// 与浮动面板语言刻意区分（任何描边都会突破关闭态最坏恰 100 的固定预算）。
func appendClosedHotbarStrip(dst *hotbarLayout, width, height, scale float32) {
	left, top := inventorySlotOrigin(0, false, width, height)
	padding := hotbarPanelPadding * scale
	totalWidth := hotbarRowWidth * scale
	panelHeight := hotbarSlotSize*scale + 2*padding
	dst.quads = append(dst.quads, hotbarInstance{
		X: left - padding, Y: top - padding,
		Width: totalWidth + 2*padding, Height: panelHeight,
		Color: hotbarPanelShadowColor,
	})
	inset := hotbarPanelInset * scale
	dst.quads = append(dst.quads, hotbarInstance{
		X: left - padding + inset, Y: top - padding + inset,
		Width: totalWidth + 2*padding - 2*inset, Height: panelHeight - 2*inset,
		Color: hotbarPanelSurfaceColor,
	})
}

// appendItemTile 用已有矩形画出带暗边的物品；可放置方块采样真实注册表材质，
// 其他物品继续使用程序化色块。
func appendItemTile(dst *hotbarLayout, item core.ItemID, x, y, scale float32) {
	color := render.ItemColor(item)
	inset := hotbarSwatchInset * scale
	border := hotbarSwatchBorder * scale
	size := (hotbarSlotSize - 2*hotbarSwatchInset) * scale
	face := hotbarInstance{
		X: x + inset + border, Y: y + inset + border,
		Width: size - 2*border, Height: size - 2*border,
		Color: color,
	}
	if uv, ok := hotbarItemUV(item); ok {
		face.U0, face.V0, face.U1, face.V1 = uv[0], uv[1], uv[2], uv[3]
		face.Color = [4]float32{1, 1, 1, 1}
	}
	dst.quads = append(dst.quads, hotbarInstance{
		X: x + inset, Y: y + inset, Width: size, Height: size,
		Color: [4]float32{color[0] * 0.35, color[1] * 0.35, color[2] * 0.35, color[3]},
	}, face)
}

// appendDurabilityBar 在快捷栏栏位下沿绘制背景和剩余耐久比例填充。
// 只有存在耐久上限且尚未满耐久的物品才显示。
func appendDurabilityBar(
	dst *hotbarLayout,
	slot int,
	stack core.ItemStack,
	width, height float32,
) {
	appendDurabilityBarScaled(dst, slot, stack, false, width, height, hudScale(false, width, height))
}
func appendDurabilityBarScaled(
	dst *hotbarLayout,
	slot int,
	stack core.ItemStack,
	open bool,
	width, height, scale float32,
) {
	maxDurability, ok := core.ItemMaxDurability(stack.Item)
	if !ok || maxDurability == 0 || stack.Durability == 0 || stack.Durability >= maxDurability {
		return
	}
	slotX, slotY := inventorySlotOrigin(slot, open, width, height)
	barWidth := (hotbarSlotSize - durabilityBarInset*2) * scale
	x := slotX + durabilityBarInset*scale
	y := slotY + (hotbarSlotSize-durabilityBarInset-durabilityBarHeight)*scale
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: y, Width: barWidth, Height: durabilityBarHeight * scale,
		Color: durabilityTrackColor,
	})
	fraction := float32(stack.Durability) / float32(maxDurability)
	color := durabilityHealthyColor
	if fraction < 0.25 {
		color = durabilityLowColor
	}
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: y, Width: barWidth * fraction, Height: durabilityBarHeight * scale,
		Color: color,
	})
}

// MiningOverlay 是最后确认的权威采掘状态；渲染器不会自行推进它。
// Target/HasTarget 不被 HUD 进度条布局消费：它们是世界空间裂纹呈现
// （`internal/render.BlockCrack`）的定位来源，HasTarget 恒随权威
// MiningActive 置位；capture 既有 fixture 不设置时裂纹天然缺席。
type MiningOverlay struct {
	Active        bool
	Target        core.BlockPos
	HasTarget     bool
	ProgressTicks uint16
	RequiredTicks uint16
	Harvestable   bool
}

// appendMiningBar 在永久预留的两行状态栈上方绘制固定背景和权威比例填充。
func appendMiningBar(dst *hotbarLayout, overlay MiningOverlay, width, height float32) {
	if !overlay.Active || overlay.RequiredTicks == 0 {
		return
	}
	left, _, totalWidth, scale := hotbarRowBounds(false, width, height)
	barWidth := miningBarWidth * scale
	barHeight := miningBarHeight * scale
	x := left + (totalWidth-barWidth)*0.5
	_, _, _, statusTop, _ := statusBarBounds(false, width, height)
	y := statusTop - (miningBarGap+miningBarHeight)*scale
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: y, Width: barWidth, Height: barHeight,
		Color: miningTrackColor,
	})
	fraction := min(float32(overlay.ProgressTicks)/float32(overlay.RequiredTicks), 1)
	if fraction <= 0 {
		return
	}
	color := miningBlockedColor
	if overlay.Harvestable {
		color = miningHarvestableColor
	}
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: y, Width: barWidth * min(fraction, 1), Height: barHeight,
		Color: color,
	})
	if overlay.Harvestable {
		capWidth := miningBarCapWidth * scale
		dst.quads = append(dst.quads, hotbarInstance{
			X: min(max(x+barWidth*fraction-capWidth, x), x+barWidth-capWidth), Y: y,
			Width: capWidth, Height: barHeight,
			Color: miningCapColor,
		})
		return
	}
	notchWidth := miningNotchWidth * scale
	for _, position := range [...]float32{0.25, 0.5, 0.75} {
		dst.quads = append(dst.quads, hotbarInstance{
			X: x + barWidth*position - notchWidth*0.5, Y: y,
			Width: notchWidth, Height: barHeight,
			Color: miningNotchColor,
		})
	}
}

// inventorySlotOrigin 返回统一索引对应格子的左上角像素坐标。
// 索引 0..8 是面板下段的快捷栏行，9..35 是其上方自上而下的三行背包。
// 关闭态没有面板：快捷栏行继续锚在 `hotbarRowBounds` 的既有位置。打开态的
// 全部行都从面板几何（`openPanelAnchor`）推导，与命中测试共用同一原点。
func inventorySlotOrigin(slot int, open bool, width, height float32) (float32, float32) {
	column := slot % core.HotbarSlots
	pitch := (hotbarSlotSize + hotbarSlotGap)
	if !open {
		left, hotbarY, _, scale := hotbarRowBounds(false, width, height)
		return left + float32(column)*pitch*scale, hotbarY
	}
	frame := openPanelAnchor(width, height)
	x := frame.contentLeft + float32(column)*pitch*frame.scale
	if slot < core.HotbarSlots {
		return x, frame.hotbarY
	}
	// 背包第 0 行在最上方，第 2 行紧邻快捷栏。
	row := (slot - core.HotbarSlots) / core.HotbarSlots
	rowsAbove := float32(2 - row)
	y := frame.hotbarY - (inventoryRowGap+(rowsAbove+1)*hotbarSlotSize+rowsAbove*hotbarSlotGap)*frame.scale
	return x, y
}

// hotbarRowBounds 返回对应容器状态下底部状态栈的共享中心边界：关闭态它就是
// 快捷栏行本身；打开态快捷栏行收进浮动面板，这里保留的是状态行锚点所依赖的
// 底部预留区（快捷栏原位 + 下方两行状态栈）。状态行、采掘反馈与弹条复用本
// 函数，避免打开态缩放和非交互状态行各算一套几何。
func hotbarRowBounds(open bool, width, height float32) (left, top, totalWidth, scale float32) {
	scale = hudScale(open, width, height)
	totalWidth = (core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap) * scale
	left = (width - totalWidth) * 0.5
	bottomMargin := hotbarBottomMargin
	if open {
		// 主状态行与氧气行整体移到快捷栏下方：底部 6px 边距只够收口，两行
		// 各占一格 `healthHeartSize + statusBarGap`，满氧隐藏时也不收缩。
		bottomMargin += 2 * (healthHeartSize + statusBarGap)
	}
	top = height - (bottomMargin+hotbarSlotSize)*scale
	return left, top, totalWidth, scale
}

// statusBarBounds 返回快捷栏左右边缘、主状态行、向外氧气行与共享缩放。
// 满氧只省略实例，这个几何始终保留，避免主行、采掘和聊天随气泡显隐跳动。
func statusBarBounds(open bool, width, height float32) (
	left, right, primaryY, oxygenY, scale float32,
) {
	left, hotbarY, totalWidth, scale := hotbarRowBounds(open, width, height)
	right = left + totalWidth
	rowStep := (statusBarGap + healthHeartSize) * scale
	primaryY = hotbarY - rowStep
	oxygenY = primaryY - rowStep
	if open {
		primaryY = hotbarY + (hotbarSlotSize+statusBarGap)*scale
		oxygenY = primaryY + rowStep
	}
	return left, right, primaryY, oxygenY, scale
}

func hudScale(open bool, width, height float32) float32 {
	if width <= 0 || height <= 0 {
		return 1
	}
	scale := float32(1)
	hotbarContentWidth := core.HotbarSlots*hotbarSlotSize +
		(core.HotbarSlots-1)*hotbarSlotGap + 2*hotbarPanelPadding
	if available := width - 2*hudEdgeMargin; available < hotbarContentWidth {
		scale = max(available/hotbarContentWidth, 0)
	}
	if open {
		// 打开态宽度约束取最宽视图：个人面板的右侧配方栏向右伸出共享内容列，
		// 对称等效宽度见 `openHUDWidth`；高度约束含统一面板高度。
		if available := width - 2*hudEdgeMargin; available < openHUDWidth {
			scale = min(scale, max(available/openHUDWidth, 0))
		}
		if available := height - 2*hudEdgeMargin; available < openHUDHeight {
			scale = min(scale, max(available/openHUDHeight, 0))
		}
	} else if available := height - 2*hudEdgeMargin; available < closedHUDHeight {
		scale = min(scale, max(available/closedHUDHeight, 0))
	}
	return scale
}

// InventorySlotAt 把光标像素坐标映射为背包界面中的统一索引 0..35。
// 命中格子之外返回 false，与绘制共用同一套几何常量。
func InventorySlotAt(cursorX, cursorY float64, width, height uint32) (uint8, bool) {
	if width == 0 || height == 0 {
		return 0, false
	}
	x, y := float32(cursorX), float32(cursorY)
	slotSize := hotbarSlotSize * hudScale(true, float32(width), float32(height))
	for slot := range core.InventorySlots {
		left, top := inventorySlotOrigin(slot, true, float32(width), float32(height))
		if x >= left && x < left+slotSize && y >= top && y < top+slotSize {
			return uint8(slot), true
		}
	}
	return 0, false
}

// appendHotbarCount 在栏位右下角排布最多两位数量数字。
func appendHotbarCount(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	count uint8,
	slotX, slotY float32,
) {
	appendHotbarCountScaled(dst, atlas, count, slotX, slotY, 1)
}
func appendHotbarCountScaled(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	count uint8,
	slotX, slotY, scale float32,
) {
	appendCountAtSize(dst, atlas, count, slotX, slotY, hotbarSlotSize, scale)
}

// appendCountAtSize 在给定尺寸的格内右下角排布最多两位数量数字：快捷栏 48 格
// 与配方栏 24 紧凑行共用同一套双层规范与右下对齐公式，只有锚定尺寸不同。
func appendCountAtSize(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	count uint8,
	slotX, slotY, size, scale float32,
) {
	if count <= 1 {
		return
	}
	var digits [2]rune
	length := 0
	if count >= 10 {
		digits[length] = rune('0' + count/10)
		length++
	}
	digits[length] = rune('0' + count%10)
	length++

	advance := float32(0)
	for index := range length {
		advance += atlas.Glyph(digits[index]).Advance * scale
	}
	tracking := float32(0)
	if length == 2 {
		tracking = hotbarDigitTracking * scale
		advance += tracking
	}
	penX := slotX + (size-hotbarDigitMargin)*scale - advance
	baseline := slotY + (size-hotbarDigitMargin)*scale
	for index := range length {
		glyph := atlas.Glyph(digits[index])
		dst.glyphs = append(dst.glyphs, hotbarInstance{
			X:      penX + glyph.BearingX*scale + scale,
			Y:      baseline - glyph.BearingY*scale + scale,
			Width:  glyph.Width * scale,
			Height: glyph.Height * scale,
			U0:     glyph.U0, V0: glyph.V0, U1: glyph.U1, V1: glyph.V1,
			Color: textPrimaryShadow,
		})
		penX += glyph.Advance * scale
		if index+1 < length {
			penX += tracking
		}
	}
	penX = slotX + (size-hotbarDigitMargin)*scale - advance
	for index := range length {
		glyph := atlas.Glyph(digits[index])
		dst.glyphs = append(dst.glyphs, hotbarInstance{
			X:      penX + glyph.BearingX*scale,
			Y:      baseline - glyph.BearingY*scale,
			Width:  glyph.Width * scale,
			Height: glyph.Height * scale,
			U0:     glyph.U0, V0: glyph.V0, U1: glyph.U1, V1: glyph.V1,
			Color: textPrimaryFg,
		})
		penX += glyph.Advance * scale
		if index+1 < length {
			penX += tracking
		}
	}
}

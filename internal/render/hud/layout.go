package hud

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

const (
	// 打开态与关闭态互斥；分别列出合法上限，防止后续样式变化悄悄突破 benchmark
	// scenario v19 已锁定的 267 quad 固定上传容量。
	openInventoryQuads = openInventoryPanelQuads + 2 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + maxOverlayQuads
	closedHotbarQuads = closedHotbarPanelQuads + closedHotbarSelectionQuads + core.HotbarSlots +
		core.HotbarSlots*2 + core.HotbarSlots*2 + miningBarQuads + miningWarningNotches
	maxHotbarQuads = 267
	// 数量最多两位数（2..64），每个数字包含阴影与前景两个实例。
	maxHotbarGlyphs = core.InventorySlots*4 + maxOverlayGlyphs + maxChatGlyphs

	// 打开背包时依次绘制外框、背包区、快捷栏区和分隔线。
	openInventoryPanelQuads = 4
	// 关闭态快捷栏以外阴影和内表面形成独立面板，选中格再用双层轮廓强调。
	closedHotbarPanelQuads     = 2
	closedHotbarSelectionQuads = 2
	miningBarQuads             = 2
	miningWarningNotches       = 3

	hotbarSlotSize      = float32(48)
	hotbarSlotGap       = float32(4)
	hotbarBottomMargin  = float32(24)
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
	// 背包界面在快捷栏之上再放 3 行，并与快捷栏留出一段间隔。
	inventoryRowGap = float32(12)
	// 三类容器共享的标题 header 只扩展 overlay 面板，不改变任何栏位原点或命中区域。
	containerTitleSize    = float32(16)
	containerTitleGap     = float32(4)
	containerHeaderHeight = float32(20)

	hudEdgeMargin = float32(8)
	// openHUDHeight 是打开背包时从合成面板上沿到 framebuffer 下沿的设计高度，
	// hudScale 用它把整个界面缩进窗口。自下而上依次是：快捷栏下边距与一格、
	// 背包三行与行间隔、合成行间隔与最下一条合成行、其余合成行、面板上边距。
	// 底部另永久保留主状态行和向下外扩的氧气行；其中快捷栏既有 24px 下边距
	// 已容纳主行，额外增加一行只为氧气，满氧隐藏时也不收缩。
	// 写成随 len(inventoryRecipeIDs) 增长的表达式而不是字面量，否则每追加一条
	// 配方最上面的行都会被挤出窗口上沿。
	openHUDHeight = hotbarBottomMargin + hotbarSlotSize +
		inventoryRowGap + 3*hotbarSlotSize + 2*hotbarSlotGap +
		recipeRowGap + hotbarSlotSize +
		float32(len(inventoryRecipeIDs)-1)*(hotbarSlotSize+hotbarSlotGap) +
		hotbarPanelPadding + containerHeaderHeight + healthHeartSize + statusBarGap
	// 关闭态联合高度从 framebuffer 下沿覆盖快捷栏、状态行和最坏采掘轨道；
	// `hudScale` 用它保证快捷栏、永久两行状态栈和采掘轨道按同一比例缩小。
	closedHUDHeight = hotbarBottomMargin + hotbarSlotSize + 2*(statusBarGap+healthHeartSize) +
		miningBarGap + miningBarHeight
)

var (
	hotbarPanelShadowColor   = [4]float32{0.012, 0.015, 0.02, 0.94}
	hotbarPanelSurfaceColor  = [4]float32{0.045, 0.052, 0.06, 0.96}
	hotbarSelectedOuterColor = [4]float32{0.96, 0.92, 0.72, 1}
	hotbarSelectedInnerColor = [4]float32{1, 0.72, 0.24, 0.98}
	miningTrackColor         = [4]float32{0.05, 0.05, 0.06, 0.78}
	miningHarvestableColor   = [4]float32{0.30, 0.78, 0.36, 0.95}
	miningBlockedColor       = [4]float32{0.95, 0.55, 0.15, 0.95}
	miningCapColor           = [4]float32{0.96, 1, 0.76, 1}
	miningNotchColor         = [4]float32{0.18, 0.12, 0.08, 1}
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
// 两者都为 nil 时画固定合成行。mining 与 eating 是两条互斥的进度条叠加值
// （采掘激活时进食条让位，见 `appendEatingBar`），只在关闭态出现。
func layoutInventory(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	inventory core.Inventory,
	open bool,
	source int,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
	mining MiningOverlay,
	eating EatingOverlay,
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

	scale := dst.scale
	slotSize := hotbarSlotSize * scale
	selectBorder := hotbarSelectBorder * scale
	slots := core.HotbarSlots
	if open {
		slots = core.InventorySlots
	}
	appendInventoryPanel(dst, open, width, height, scale)
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
		if sourceX, sourceY, ok := containerSourceOrigin(source, overlay, chest, width, height); ok {
			dst.quads = append(dst.quads, hotbarInstance{
				X:      sourceX - selectBorder,
				Y:      sourceY - selectBorder,
				Width:  slotSize + 2*selectBorder,
				Height: slotSize + 2*selectBorder,
				Color:  [4]float32{0.25, 0.72, 1, 0.98},
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
			appendChestGrid(dst, atlas, *chest, width, height)
		case overlay != nil:
			appendFurnaceRow(dst, atlas, *overlay, width, height)
		default:
			appendRecipeRows(dst, atlas, inventory, width, height)
		}
	} else {
		appendMiningBar(dst, mining, width, height)
		appendEatingBar(dst, eating, mining, width, height)
	}
	return *dst
}
func appendInventoryPanel(dst *hotbarLayout, open bool, width, height, scale float32) {
	left, hotbarY := inventorySlotOrigin(0, open, width, height)
	top := hotbarY
	if open {
		_, top = inventorySlotOrigin(core.HotbarSlots, true, width, height)
	}
	padding := hotbarPanelPadding * scale
	totalWidth := (core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap) * scale
	if !open {
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
		return
	}
	dst.quads = append(dst.quads, hotbarInstance{
		X: left - padding, Y: top - padding,
		Width: totalWidth + 2*padding, Height: hotbarY + hotbarSlotSize*scale - top + 2*padding,
		Color: [4]float32{0.025, 0.03, 0.035, 0.88},
	})
	_, backpackBottomY := inventorySlotOrigin(core.InventorySlots-1, true, width, height)
	innerPadding := padding * 0.5
	dst.quads = append(dst.quads,
		hotbarInstance{
			X: left - innerPadding, Y: top - innerPadding,
			Width:  totalWidth + 2*innerPadding,
			Height: backpackBottomY + hotbarSlotSize*scale - top + 2*innerPadding,
			Color:  [4]float32{0.045, 0.052, 0.06, 0.94},
		},
		hotbarInstance{
			X: left - innerPadding, Y: hotbarY - innerPadding,
			Width: totalWidth + 2*innerPadding, Height: hotbarSlotSize*scale + 2*innerPadding,
			Color: [4]float32{0.06, 0.052, 0.04, 0.94},
		},
		hotbarInstance{
			X: left, Y: (backpackBottomY + hotbarSlotSize*scale + hotbarY) * 0.5,
			Width: totalWidth, Height: scale * 2,
			Color: [4]float32{0.25, 0.30, 0.34, 0.92},
		},
	)
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
		Color: [4]float32{0.05, 0.05, 0.06, 0.85},
	})
	fraction := float32(stack.Durability) / float32(maxDurability)
	color := [4]float32{0.30, 0.78, 0.36, 0.95}
	if fraction < 0.25 {
		color = [4]float32{0.90, 0.35, 0.25, 0.95}
	}
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: y, Width: barWidth * fraction, Height: durabilityBarHeight * scale,
		Color: color,
	})
}

// MiningOverlay 是最后确认的权威采掘状态；渲染器不会自行推进它。
type MiningOverlay struct {
	Active        bool
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
// 索引 0..8 是底部快捷栏行，9..35 是其上方自上而下的三行背包。
func inventorySlotOrigin(slot int, open bool, width, height float32) (float32, float32) {
	column := slot % core.HotbarSlots
	left, hotbarY, _, scale := hotbarRowBounds(open, width, height)
	x := left + float32(column)*(hotbarSlotSize+hotbarSlotGap)*scale
	if !open {
		return x, hotbarY
	}
	if slot < core.HotbarSlots {
		return x, hotbarY
	}
	// 背包第 0 行在最上方，第 2 行紧邻快捷栏。
	row := (slot - core.HotbarSlots) / core.HotbarSlots
	rowsAbove := float32(2 - row)
	y := hotbarY - (inventoryRowGap+(rowsAbove+1)*hotbarSlotSize+rowsAbove*hotbarSlotGap)*scale
	return x, y
}

// hotbarRowBounds 返回对应容器状态下快捷栏的共享中心边界，供快捷栏、状态行与
// 采掘反馈复用，避免打开态命中缩放和非交互状态行各算一套几何。
func hotbarRowBounds(open bool, width, height float32) (left, top, totalWidth, scale float32) {
	scale = hudScale(open, width, height)
	totalWidth = (core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap) * scale
	left = (width - totalWidth) * 0.5
	bottomMargin := hotbarBottomMargin
	if open {
		// 主状态行原本已由 24px 下边距容纳；再上移一行，给向下外扩的氧气
		// 保留同样的 16px 高度和 4px 底部余量。
		bottomMargin += healthHeartSize + statusBarGap
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
	penX := slotX + (hotbarSlotSize-hotbarDigitMargin)*scale - advance
	baseline := slotY + (hotbarSlotSize-hotbarDigitMargin)*scale
	for index := range length {
		glyph := atlas.Glyph(digits[index])
		dst.glyphs = append(dst.glyphs, hotbarInstance{
			X:      penX + glyph.BearingX*scale + scale,
			Y:      baseline - glyph.BearingY*scale + scale,
			Width:  glyph.Width * scale,
			Height: glyph.Height * scale,
			U0:     glyph.U0, V0: glyph.V0, U1: glyph.U1, V1: glyph.V1,
			Color: [4]float32{0.02, 0.025, 0.03, 0.95},
		})
		penX += glyph.Advance * scale
		if index+1 < length {
			penX += tracking
		}
	}
	penX = slotX + (hotbarSlotSize-hotbarDigitMargin)*scale - advance
	for index := range length {
		glyph := atlas.Glyph(digits[index])
		dst.glyphs = append(dst.glyphs, hotbarInstance{
			X:      penX + glyph.BearingX*scale,
			Y:      baseline - glyph.BearingY*scale,
			Width:  glyph.Width * scale,
			Height: glyph.Height * scale,
			U0:     glyph.U0, V0: glyph.V0, U1: glyph.U1, V1: glyph.V1,
			Color: [4]float32{1, 0.94, 0.78, 1},
		})
		penX += glyph.Advance * scale
		if index+1 < length {
			penX += tracking
		}
	}
}

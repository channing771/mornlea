package hud

import (
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// 打开态是 GPU 保留面唯一产生实例的分支：容器面板族、选中格与来源格高亮、
	// 统一栏位凹槽、双层物品 tile、面板快捷栏行耐久条、最坏容器内容与 tooltip
	// 背景，实算分解见 panel_test.go 的固定预算测试。常显层（快捷栏贴条与选中
	// 框、状态行图标、氧气气泡、采掘/进食轨道、物品名弹条、准星、聊天呈现）已
	// 迁 WebView HUD 组件呈现，GPU 保留面在关闭容器界面时零实例。
	openInventoryQuads = containerPanelQuads + 2 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + maxOverlayQuads + tooltipQuads
	maxHotbarQuads = 320
	// glyph 上限按「打开态分支最坏 + 固定增长余量」计算：分支见证 36 格两位
	// 数量、箱子 27 格两位数量与 tooltip 双层显示名（8 rune 截断上限封顶）。
	// 聊天行与物品名弹条的字形流随常显层绘制退役离开本公式，缩出的容量全部归
	// 入增长余量，固定上限 768 与 glyph offset 由此保持不变；实测最坏由固定
	// 预算测试断言并记录。
	maxHotbarGlyphs = core.InventorySlots*4 + maxOverlayGlyphs + tooltipGlyphs + glyphGrowthMargin

	// glyphGrowthMargin 是分支最坏之外的固定增长余量，与 quad 侧 320 容量的
	// 余量同源。常显层退役让它从 52 扩到 500：差额正是旧公式里的七行聊天与
	// 最长弹条预留，按「缺口由增长余量吸收、总上限不变」的口径重钉。
	glyphGrowthMargin = 500

	hotbarSlotSize     = float32(48)
	hotbarSlotGap      = float32(4)
	hotbarBottomMargin = float32(6)
	// `statusHotbarGap` 是主状态行底与快捷栏贴条外沿之间的可见净空。状态行本身
	// 已迁 WebView 组件，但打开态面板的垂直居中仍以「主状态行顶」为下界（见
	// `openBottomStackTop`），这份净空因此继续参与打开态几何与高度约束。
	statusHotbarGap     = float32(10)
	hotbarSelectBorder  = float32(3)
	hotbarPanelPadding  = float32(6)
	hotbarSwatchInset   = float32(10)
	hotbarSwatchBorder  = float32(2)
	hotbarDigitMargin   = float32(3)
	hotbarDigitTracking = float32(-2)
	durabilityBarHeight = float32(3)
	durabilityBarInset  = float32(4)
	// 面板下段快捷栏行与背包三行之间的行距。
	inventoryRowGap = float32(12)

	hudEdgeMargin = float32(8)
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
}

// layoutInventory 布局容器保留面：只依赖 framebuffer 尺寸、完整物品状态、容器
// 叠加值与来源格，产出固定上限的实例。容器界面关闭时零实例——常显层已迁
// WebView，容器面板与内容只在打开态布局。overlay 与 chest 至多一个非 nil：
// overlay 非 nil 时画熔炉三格与两条进度条，chest 非 nil 时画箱子 27 格，两者都
// 为 nil 时画合成网格与产物格（crafting 为 nil 表示网格镜像尚未确认，按空的
// 个人 2×2 呈现）。
func layoutInventory(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	inventory core.Inventory,
	open bool,
	source int,
	crafting *CraftingOverlay,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
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
	dst.scale = hudScale(width, height)
	if width <= 0 || height <= 0 || !inventory.Valid() || !open {
		return *dst
	}

	scale := dst.scale
	slotSize := hotbarSlotSize * scale
	selectBorder := hotbarSelectBorder * scale
	// 面板族在一切栏位之前追加；视图决定面板宽度（个人合成面板加右侧配方栏）
	// 与标题 cell，原点与各行锚点对四类视图一致。
	view := containerViewCrafting
	switch {
	case chest != nil:
		view = containerViewChest
	case overlay != nil:
		view = containerViewFurnace
	}
	appendContainerPanel(dst, view, width, height)
	// 高亮先于栏位表面绘制，栏位只覆盖内部并留下像素边框。打开态选中格内衬取
	// 鼠尾草绿强调（`hotbarSelectedInnerColor`），来源格取麦金族高亮。
	selectedX, selectedY := inventorySlotOrigin(int(inventory.Hotbar.Selected), width, height)
	dst.quads = append(dst.quads, hotbarInstance{
		X:      selectedX - selectBorder,
		Y:      selectedY - selectBorder,
		Width:  slotSize + 2*selectBorder,
		Height: slotSize + 2*selectBorder,
		Color:  hotbarSelectedInnerColor,
	})
	if source >= 0 {
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
	for slot := range core.InventorySlots {
		x, y := inventorySlotOrigin(slot, width, height)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: slotSize, Height: slotSize,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
	for slot := range core.InventorySlots {
		stack, _ := inventory.Slot(uint8(slot))
		if stack.Item == core.ItemNone {
			continue
		}
		x, y := inventorySlotOrigin(slot, width, height)
		appendItemTile(dst, stack.Item, x, y, scale)
	}
	for slot := range core.InventorySlots {
		stack, _ := inventory.Slot(uint8(slot))
		if stack.Item == core.ItemNone {
			continue
		}
		x, y := inventorySlotOrigin(slot, width, height)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, scale)
	}
	for slot, stack := range inventory.Hotbar.Slots {
		appendDurabilityBarScaled(dst, slot, stack, width, height, scale)
	}
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
	return *dst
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

// appendDurabilityBarScaled 在面板快捷栏行栏位下沿绘制背景和剩余耐久比例填充。
// 只有存在耐久上限且尚未满耐久的物品才显示。
func appendDurabilityBarScaled(
	dst *hotbarLayout,
	slot int,
	stack core.ItemStack,
	width, height, scale float32,
) {
	maxDurability, ok := core.ItemMaxDurability(stack.Item)
	if !ok || maxDurability == 0 || stack.Durability == 0 || stack.Durability >= maxDurability {
		return
	}
	slotX, slotY := inventorySlotOrigin(slot, width, height)
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

// inventorySlotOrigin 返回统一索引对应格子的左上角像素坐标。
// 索引 0..8 是面板下段的快捷栏行，9..35 是其上方自上而下的三行背包。
// 全部行都从面板几何（`openPanelAnchor`）推导，与命中测试共用同一原点。
func inventorySlotOrigin(slot int, width, height float32) (float32, float32) {
	column := slot % core.HotbarSlots
	pitch := (hotbarSlotSize + hotbarSlotGap)
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

func hudScale(width, height float32) float32 {
	if width <= 0 || height <= 0 {
		return 1
	}
	scale := float32(1)
	// 打开态宽度约束取最宽视图：个人面板的右侧配方栏向右伸出共享内容列，
	// 对称等效宽度见 `openHUDWidth`；高度约束含统一面板高度与底部两行状态栈
	// 预留。关闭态已无 GPU 实例，不再有任何缩放约束。
	if available := width - 2*hudEdgeMargin; available < openHUDWidth {
		scale = min(scale, max(available/openHUDWidth, 0))
	}
	if available := height - 2*hudEdgeMargin; available < openHUDHeight {
		scale = min(scale, max(available/openHUDHeight, 0))
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
	slotSize := hotbarSlotSize * hudScale(float32(width), float32(height))
	for slot := range core.InventorySlots {
		left, top := inventorySlotOrigin(slot, float32(width), float32(height))
		if x >= left && x < left+slotSize && y >= top && y < top+slotSize {
			return uint8(slot), true
		}
	}
	return 0, false
}

// appendHotbarCountScaled 在栏位右下角排布最多两位数量数字。
func appendHotbarCountScaled(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	count uint8,
	slotX, slotY, scale float32,
) {
	appendCountAtSize(dst, atlas, count, slotX, slotY, hotbarSlotSize, scale)
}

// appendCountAtSize 在给定尺寸的格内右下角排布最多两位数量数字：统一栏位 48 格
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

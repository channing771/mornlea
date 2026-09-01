package hud

import (
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// tooltip.go 实现容器面板的悬停物品名 tooltip：指针悬停非空栏位（含产物格与
// 配方入口产物）时，在指针右下侧呈现投影加表面双层背景与阴影加前景双层中文
// 名；越出 framebuffer 时翻转到指针左上。名称与物品名弹条同源
// （`core.ItemDisplayName`），只消费已确认镜像与本地指针，零预测、零新协议。

const (
	// tooltipQuads 是 tooltip 背景的固定 quad 数：外扩投影与表面。
	tooltipQuads = 2
	// maxTooltipRunes 是 tooltip 可见 rune 上限（含省略号），与聊天/弹条同一
	// 截断约定；物品显示名实际最长 5 rune，上限只为 glyph 预算封顶。
	maxTooltipRunes = 8
	// tooltipGlyphs 是 tooltip 最坏字形数：8 rune × 阴影/前景双层。
	tooltipGlyphs = maxTooltipRunes * 2
	// tooltipPadding 是背景矩形内边距、tooltipCursorGap 是背景与指针的间隙
	// （design px）。
	tooltipPadding   = float32(4)
	tooltipCursorGap = float32(2)
)

// TooltipOverlay 是容器悬停 tooltip 的呈现输入：应用层在容器界面打开时把本帧
// 指针坐标传入（与点击命中同源的 `window.CursorPos`）；Valid 为 false 或容器
// 未打开时渲染层不产生任何实例。指针只读本地窗口状态，零预测。
type TooltipOverlay struct {
	Valid   bool
	CursorX float64
	CursorY float64
}

// tooltipStackAt 解析指针悬停栏位的权威内容：与绘制共用同一组命中函数，指针
// 落在当前打开视图的某个非空格（含产物格与配方入口）时返回该格物品。返回
// false 表示空栏位、视图外或没有打开的容器视图。
func tooltipStackAt(
	tooltip TooltipOverlay,
	inventory core.Inventory,
	crafting *CraftingOverlay,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
	width, height float32,
) (core.ItemStack, bool) {
	if !tooltip.Valid || width <= 0 || height <= 0 || !inventory.Valid() {
		return core.ItemStack{}, false
	}
	w, h := uint32(width), uint32(height)
	switch {
	case chest != nil:
		// 箱子视图：统一栏位 0..62，36..62 落在箱子 27 格上。
		slot, ok := ChestSlotAt(tooltip.CursorX, tooltip.CursorY, w, h)
		if !ok {
			return core.ItemStack{}, false
		}
		if slot < core.InventorySlots {
			stack, _ := inventory.Slot(slot)
			return stack, stack.Item != core.ItemNone
		}
		stack := chest.Items[slot-core.InventorySlots]
		return stack, stack.Item != core.ItemNone
	case overlay != nil:
		// 熔炉视图：统一栏位 0..38，36/37/38 依次是输入、燃料、输出。
		slot, ok := FurnaceSlotAt(tooltip.CursorX, tooltip.CursorY, w, h)
		if !ok {
			return core.ItemStack{}, false
		}
		if slot < core.InventorySlots {
			stack, _ := inventory.Slot(slot)
			return stack, stack.Item != core.ItemNone
		}
		switch slot {
		case core.FurnaceInputSlot:
			return overlay.Input, overlay.Input.Item != core.ItemNone
		case core.FurnaceFuelSlot:
			return overlay.Fuel, overlay.Fuel.Item != core.ItemNone
		case core.FurnaceOutputSlot:
			return overlay.Output, overlay.Output.Item != core.ItemNone
		}
		return core.ItemStack{}, false
	default:
		// 合成视图：网格格、产物格与配方入口产物都在 tooltip 语义内。未确认
		// 镜像按个人 2×2 呈现，网格内容取零值（全部为空格）。
		size := craftingSize(crafting)
		if slot, ok := CraftingSlotAt(tooltip.CursorX, tooltip.CursorY, w, h, size); ok {
			if slot < core.CraftingGridSlots {
				stack := core.ItemStack{}
				if crafting != nil {
					stack = crafting.Slots[slot]
				}
				return stack, stack.Item != core.ItemNone
			}
			stack, _ := inventory.Slot(slot - core.CraftingGridSlots)
			return stack, stack.Item != core.ItemNone
		}
		if CraftingOutputAt(tooltip.CursorX, tooltip.CursorY, w, h, size) {
			output := core.ItemStack{}
			if crafting != nil {
				output = crafting.Output
			}
			return output, output.Item != core.ItemNone
		}
		if recipeID, ok := RecipeButtonAt(tooltip.CursorX, tooltip.CursorY, w, h); ok {
			if recipe, ok := core.Recipe(recipeID); ok {
				return recipe.Output, true
			}
		}
	}
	return core.ItemStack{}, false
}

// requestTooltipText 为当前悬停的非空栏位请求显示名字形；没有可呈现的 tooltip
// 时不请求，避免为不呈现的文本扩张字形图集。返回是否发生了请求，供 `Prepare`
// 决定是否冲刷上传。
func requestTooltipText(
	atlas render.GlyphSource,
	tooltip TooltipOverlay,
	inventory core.Inventory,
	crafting *CraftingOverlay,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
	open bool,
	width, height float32,
) bool {
	if !open {
		return false
	}
	stack, ok := tooltipStackAt(tooltip, inventory, crafting, overlay, chest, width, height)
	if !ok {
		return false
	}
	name, ok := core.ItemDisplayName(stack.Item)
	if !ok {
		return false
	}
	visible := truncateVisibleRunes(name, maxTooltipRunes)
	if visible == "" {
		return false
	}
	atlas.Request(visible)
	return true
}

// appendTooltipOverlay 布局悬停 tooltip：先解析悬停栏位，再在指针右下侧放置
// 外扩投影与表面双层背景，最后以阴影加前景双层字形呈现中文名。背景矩形按
// 字形墨迹包围盒取界并夹回 framebuffer 内；空栏位、面板外、未打开与零尺寸
// framebuffer 都不产生任何实例。
func appendTooltipOverlay(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	tooltip TooltipOverlay,
	inventory core.Inventory,
	crafting *CraftingOverlay,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
	width, height float32,
) {
	if width <= 0 || height <= 0 {
		return
	}
	stack, ok := tooltipStackAt(tooltip, inventory, crafting, overlay, chest, width, height)
	if !ok {
		return
	}
	name, ok := core.ItemDisplayName(stack.Item)
	if !ok {
		return
	}
	text := truncateVisibleRunes(name, maxTooltipRunes)
	if text == "" {
		return
	}
	scale := dst.scale
	if scale <= 0 {
		// 直达调用（测试与未来非布局入口）可能带着零值 layout：补齐缩放，
		// 保证背景/字形几何与完整布局路径一致。
		scale = hudScale(width, height)
		dst.scale = scale
	}
	// 墨迹测量与绘制共用同一套 advance/kerning 公式：宽度取推进宽度与字形
	// 墨宽的较大者，高度由 BearingY 与下伸量决定，保证背景完整包住字形。
	// 本调用点的文本上界是 `maxTooltipRunes`（8 rune 截断上限），与 WebView
	// 侧弹条/聊天行只共用「31 rune + 省略号」的截断口径，不共享预算。
	textWidth := textAdvanceWidth(atlas, text, scale)
	inkWidth := float32(0)
	inkAscent, inkDescent := float32(0), float32(0)
	for _, char := range text {
		glyph := atlas.Glyph(char)
		inkWidth = max(inkWidth, glyph.BearingX+glyph.Width)
		inkAscent = max(inkAscent, glyph.BearingY)
		inkDescent = max(inkDescent, glyph.Height-glyph.BearingY)
	}
	padding := tooltipPadding * scale
	gap := tooltipCursorGap * scale
	// 阴影层向右下偏移 1 design px，墨迹包围盒按含阴影计，保证背景完整包住
	// 两层字形。
	rectWidth := padding*2 + max(textWidth, inkWidth*scale+scale)
	rectHeight := padding*2 + (inkAscent+inkDescent)*scale + scale

	x := float32(tooltip.CursorX) + gap
	y := float32(tooltip.CursorY) + gap
	// 右下侧越出 framebuffer（右沿或下沿任一）即翻转到指针左上，随后整体
	// 夹回 framebuffer；极小 framebuffer 下矩形按上限对齐，不产生负坐标。
	if x+rectWidth > width || y+rectHeight > height {
		x = float32(tooltip.CursorX) - gap - rectWidth
		y = float32(tooltip.CursorY) - gap - rectHeight
	}
	x = max(0, min(x, width-rectWidth))
	y = max(0, min(y, height-rectHeight))

	expand := panelShadowExpand * scale
	dst.quads = append(dst.quads,
		hotbarInstance{
			X: x - expand, Y: y - expand,
			Width: rectWidth + 2*expand, Height: rectHeight + 2*expand,
			Color: panelShadow,
		},
		hotbarInstance{X: x, Y: y, Width: rectWidth, Height: rectHeight, Color: panelSurface},
	)
	// 基线放在「上内边距 + 最大上伸」处：任意字形的墨迹顶都落在上内边距上，
	// 墨迹底不越过下内边距。前景取 `textOnPanelFg`：背景是 `panelSurface` 暖
	// 羊皮纸，世界浮层的暖白字对比不足。
	baseline := y + padding + inkAscent*scale
	appendAlignedText(dst, atlas, text, x+padding, baseline, scale, textOnPanelFg)
}

// truncateVisibleRunes 把文本截断到 limit 个可见 rune：超长时保留前 limit-1
// rune 并以省略号收尾。这是容器 tooltip 显示名的截断约定，与 WebView 侧弹条/
// 聊天行沿用同一「31 rune + 省略号」口径。
func truncateVisibleRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	visibleEnd := 0
	runes := 0
	for index := range text {
		if runes == limit-1 {
			visibleEnd = index
			break
		}
		runes++
	}
	return text[:visibleEnd] + "…"
}

// appendAlignedText 以基线锚定、左对齐绘制双层文字：tooltip 悬停名取这套 pen
// 推进、kerning 与阴影偏移。前景取 `textOnPanelFg`（背景是 `panelSurface` 暖羊
// 皮纸），阴影层统一走 `textPrimaryShadow` 且向右下偏移 1 design px。
func appendAlignedText(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	text string,
	penOriginX, baseline, scale float32,
	foreground [4]float32,
) {
	for pass := range 2 {
		penX := penOriginX
		previous := rune(0)
		index := 0
		for _, char := range text {
			if index > 0 {
				penX += atlas.Kern(previous, char) * scale
			}
			glyph := atlas.Glyph(char)
			offset := float32(0)
			color := foreground
			if pass == 0 {
				offset = scale
				color = textPrimaryShadow
			}
			dst.glyphs = append(dst.glyphs, hotbarInstance{
				X:     penX + glyph.BearingX*scale + offset,
				Y:     baseline - glyph.BearingY*scale + offset,
				Width: glyph.Width * scale, Height: glyph.Height * scale,
				U0: glyph.U0, V0: glyph.V0, U1: glyph.U1, V1: glyph.V1, Color: color,
			})
			penX += glyph.Advance * scale
			previous = char
			index++
		}
	}
}

// textAdvanceWidth 按与绘制完全相同的 advance/kerning 公式测量文本推进宽度，
// 保证 tooltip 背景矩形不因测量与绘制分叉而漂移。调用方负责先按 `maxTooltipRunes`
// 截断，本函数不设上界。
func textAdvanceWidth(atlas render.GlyphSource, text string, scale float32) float32 {
	width := float32(0)
	previous := rune(0)
	index := 0
	for _, char := range text {
		if index > 0 {
			width += atlas.Kern(previous, char) * scale
		}
		width += atlas.Glyph(char).Advance * scale
		previous = char
		index++
	}
	return width
}

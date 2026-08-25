package hud

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

const (
	// 合成视图：面板、9 个网格格与产物格背景、最多 10 个双层物品色块与标题。
	// 个人 2×2 只画其中 4 个网格格，容量按 3×3 最坏组合锁定。
	craftingQuads = 1 + (core.CraftingGridSlots + 1) + (core.CraftingGridSlots+1)*2 + 1
	// 网格与产物各最多两位数量。
	craftingGlyphs = (core.CraftingGridSlots + 1) * 4
	// 熔炉视图：面板、三个栏位、双层物品色块、两条进度条底与填充。
	furnaceQuads = 1 + 3 + 3*2 + 4 + 1
	// 三个熔炉格各最多两位数量。
	furnaceGlyphs = 12
	// 箱子视图：面板、27 格背景，加最多 27 个双层物品色块。
	chestQuads = 1 + core.ChestSlots + core.ChestSlots*2 + 1
	// 箱子每格最多两位数量。
	chestGlyphs = core.ChestSlots * 4

	maxOverlayQuads  = max(craftingQuads, furnaceQuads, chestQuads)
	maxOverlayGlyphs = max(craftingGlyphs, furnaceGlyphs, chestGlyphs)

	// 合成区（网格与产物格）位于背包最上一行之上，与熔炉/箱子行共用同一行锚点。
	recipeRowGap = float32(16)
	// 熔炉三格与两条进度条排在背包最上一行之上。
	furnaceBarHeight = float32(10)
	furnaceBarGap    = float32(6)
)

// craftingGridSizeWorkbench 是合成网格的最大有效尺寸；与 `sim.CraftingGridSizeWorkbench`
// 同值。render 不依赖 sim（archcheck 依赖边界），两侧各自硬编码、由布局与命中
// 测试共同锁定。
const craftingGridSizeWorkbench = 3

// normalizeCraftingGridSize 把权威网格尺寸归一为 2 或 3：绘制与命中必须共用同一
// 个归一（未确认镜像的零值按个人 2×2 呈现），否则命中矩形会漂移到不画的格上。
func normalizeCraftingGridSize(size int) int {
	if size != craftingGridSizeWorkbench {
		return 2
	}
	return craftingGridSizeWorkbench
}

// containerSourceOrigin 返回来源高亮格的左上角像素坐标；索引落在当前打开的容器视图之外
// 时返回 false。overlay 与 chest 至多一个非 nil，分别把索引 36 之后解释为熔炉三格或
// 箱子 27 格；两者都为 nil 时是合成视图，索引按统一视图解释（网格 0..8、背包 9..44）。
func containerSourceOrigin(
	source int,
	crafting *CraftingOverlay,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
	width, height float32,
) (float32, float32, bool) {
	switch {
	case chest != nil:
		if source >= core.InventorySlots && source < core.ChestViewSlots {
			x, y := chestSlotOrigin(source-core.InventorySlots, width, height)
			return x, y, true
		}
	case overlay != nil:
		if source >= core.InventorySlots && source < core.FurnaceViewSlots {
			x, y := recipeSlotOrigin(source-core.InventorySlots, width, height)
			return x, y, true
		}
	default:
		// 合成视图的统一索引：网格 0..8（个人尺寸只认可 size*size 以内的格）、
		// 背包 9..44。
		if source >= 0 && source < core.CraftingGridSlots {
			size := normalizeCraftingGridSize(craftingSize(crafting))
			if source < size*size {
				x, y := craftingGridSlotOrigin(source, size, width, height)
				return x, y, true
			}
			return 0, 0, false
		}
		if source >= core.CraftingGridSlots && source < core.CraftingGridSlots+core.InventorySlots {
			x, y := inventorySlotOrigin(source-core.CraftingGridSlots, true, width, height)
			return x, y, true
		}
	}
	if source >= 0 && source < core.InventorySlots {
		x, y := inventorySlotOrigin(source, true, width, height)
		return x, y, true
	}
	return 0, 0, false
}

// craftingSize 读取合成叠加值的尺寸；未确认（nil）时按个人 2×2 呈现。
func craftingSize(crafting *CraftingOverlay) int {
	if crafting == nil {
		return 2
	}
	return normalizeCraftingGridSize(int(crafting.Size))
}

// FurnaceOverlay 是熔炉界面需要显示的全部权威值。
// 它是 render 本地值，由 app 从已确认镜像转换，渲染层不依赖协议类型。
type FurnaceOverlay struct {
	Input         core.ItemStack
	Fuel          core.ItemStack
	Output        core.ItemStack
	ProgressTicks uint8
	BurnTicks     uint16
}

// appendFurnaceRow 绘制熔炉的输入、燃料、输出三格与燃烧、熔炼两条进度条。
func appendFurnaceRow(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay FurnaceOverlay,
	width, height float32,
) {
	scale := hudScale(true, width, height)
	padding := hotbarPanelPadding * scale
	panelX, slotY := recipeSlotOrigin(0, width, height)
	_, barTop := furnaceBarOrigin(width, height)
	panelWidth := (3*hotbarSlotSize+2*hotbarSlotGap)*scale + 2*padding
	dst.quads = append(dst.quads, hotbarInstance{
		X: panelX - padding, Y: barTop - padding - containerHeaderHeight*scale,
		Width: panelWidth, Height: slotY + hotbarSlotSize*scale - barTop + 2*padding + containerHeaderHeight*scale,
		Color: [4]float32{0.035, 0.05, 0.065, 0.96},
	})
	stacks := [3]core.ItemStack{overlay.Input, overlay.Fuel, overlay.Output}
	slotUV := hotbarTextureUV(hotbarContainerSlotColumn)
	for index, stack := range stacks {
		x, y := recipeSlotOrigin(index, width, height)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: hotbarSlotSize * scale, Height: hotbarSlotSize * scale,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
		if stack.Item == core.ItemNone {
			continue
		}
		appendItemTile(dst, stack.Item, x, y, scale)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, scale)
	}

	// 两条进度条分别显示剩余燃烧量与当前熔炼进度。
	bars := [2]struct {
		fraction float32
		uv       [4]float32
	}{
		{float32(overlay.BurnTicks) / float32(core.FurnaceBurnTicks),
			hotbarTextureUV(hotbarFurnaceFlameColumn)},
		{float32(overlay.ProgressTicks) / float32(core.FurnaceSmeltTicks),
			hotbarTextureUV(hotbarFurnaceArrowColumn)},
	}
	barX, barTop := furnaceBarOrigin(width, height)
	barWidth := (3*hotbarSlotSize + 2*hotbarSlotGap) * scale
	for index, bar := range bars {
		y := barTop + float32(index)*(furnaceBarHeight+furnaceBarGap)*scale
		dst.quads = append(dst.quads, hotbarInstance{
			X: barX, Y: y, Width: barWidth, Height: furnaceBarHeight * scale,
			Color: [4]float32{0.05, 0.05, 0.06, 0.62},
		})
		fraction := min(bar.fraction, 1)
		if fraction <= 0 {
			continue
		}
		fill := hotbarInstance{U0: bar.uv[0], V0: bar.uv[1], U1: bar.uv[2], V1: bar.uv[3], Color: [4]float32{1, 1, 1, 1}}
		if index == 0 {
			fill.X, fill.Y = barX, y+furnaceBarHeight*scale*(1-fraction)
			fill.Width, fill.Height = barWidth, furnaceBarHeight*scale*fraction
			fill.V0 = bar.uv[1] + (bar.uv[3]-bar.uv[1])*(1-fraction)
		} else {
			fill.X, fill.Y = barX, y
			fill.Width, fill.Height = barWidth*fraction, furnaceBarHeight*scale
			fill.U1 = bar.uv[0] + (bar.uv[2]-bar.uv[0])*fraction
		}
		dst.quads = append(dst.quads, fill)
	}
	titleUV := hotbarTextureUV(hotbarFurnaceTitleColumn)
	dst.quads = append(dst.quads, hotbarInstance{
		X: panelX, Y: barTop - padding - containerHeaderHeight*scale + containerTitleGap*scale,
		Width: containerTitleSize * scale, Height: containerTitleSize * scale,
		U0: titleUV[0], V0: titleUV[1], U1: titleUV[2], V1: titleUV[3],
		Color: [4]float32{1, 1, 1, 1},
	})
}

// furnaceBarOrigin 返回两条进度条的左上角像素坐标。
func furnaceBarOrigin(width, height float32) (float32, float32) {
	x, y := recipeSlotOrigin(0, width, height)
	scale := hudScale(true, width, height)
	return x, y - (2*furnaceBarGap+2*furnaceBarHeight)*scale
}

// FurnaceSlotAt 把光标像素坐标映射为熔炉界面的统一索引 0..38。
// 它与绘制共用同一套几何常量；界外返回 false。
func FurnaceSlotAt(cursorX, cursorY float64, width, height uint32) (uint8, bool) {
	if slot, ok := InventorySlotAt(cursorX, cursorY, width, height); ok {
		return slot, true
	}
	if width == 0 || height == 0 {
		return 0, false
	}
	x, y := float32(cursorX), float32(cursorY)
	slotSize := hotbarSlotSize * hudScale(true, float32(width), float32(height))
	for index := range 3 {
		left, top := recipeSlotOrigin(index, float32(width), float32(height))
		if x >= left && x < left+slotSize && y >= top && y < top+slotSize {
			return core.InventorySlots + uint8(index), true
		}
	}
	return 0, false
}

// ChestOverlay 是箱子界面需要显示的全部权威值：27 个格子的物品。
// 它是 render 本地值，由 app 从已确认镜像转换，渲染层不依赖协议类型。
type ChestOverlay struct {
	Items [core.ChestSlots]core.ItemStack
}

// appendChestGrid 绘制箱子 27 格背景、物品色块与数量，按统一栏位 36..62 排布成 3 行 9 列。
func appendChestGrid(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay ChestOverlay,
	width, height float32,
) {
	scale := hudScale(true, width, height)
	padding := hotbarPanelPadding * scale
	left, bottomY := chestSlotOrigin(0, width, height)
	_, top := chestSlotOrigin(core.ChestSlots-core.HotbarSlots, width, height)
	totalWidth := (core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap) * scale
	dst.quads = append(dst.quads, hotbarInstance{
		X: left - padding, Y: top - padding - containerHeaderHeight*scale,
		Width: totalWidth + 2*padding, Height: bottomY + hotbarSlotSize*scale - top + 2*padding + containerHeaderHeight*scale,
		Color: [4]float32{0.035, 0.05, 0.065, 0.96},
	})
	slotUV := hotbarTextureUV(hotbarContainerSlotColumn)
	for index := range core.ChestSlots {
		x, y := chestSlotOrigin(index, width, height)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: hotbarSlotSize * scale, Height: hotbarSlotSize * scale,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
	for index, stack := range overlay.Items {
		if stack.Item == core.ItemNone {
			continue
		}
		x, y := chestSlotOrigin(index, width, height)
		appendItemTile(dst, stack.Item, x, y, scale)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, scale)
	}
	titleUV := hotbarTextureUV(hotbarChestTitleColumn)
	dst.quads = append(dst.quads, hotbarInstance{
		X: left, Y: top - padding - containerHeaderHeight*scale + containerTitleGap*scale,
		Width: containerTitleSize * scale, Height: containerTitleSize * scale,
		U0: titleUV[0], V0: titleUV[1], U1: titleUV[2], V1: titleUV[3],
		Color: [4]float32{1, 1, 1, 1},
	})
}

// chestSlotOrigin 返回箱子统一索引 0..26 对应格子的左上角像素坐标：3 行 9 列，
// 紧贴在背包最上一行之上，index 0 在最下面一行、与熔炉/合成行共用同一起点。
func chestSlotOrigin(index int, width, height float32) (float32, float32) {
	row := index / core.HotbarSlots
	column := index % core.HotbarSlots
	x, y := recipeSlotOrigin(column, width, height)
	return x, y - float32(row)*(hotbarSlotSize+hotbarSlotGap)*hudScale(true, width, height)
}

// ChestSlotAt 把光标像素坐标映射为箱子界面的统一索引 0..62。
// 它与绘制共用同一套几何常量；界外返回 false。
func ChestSlotAt(cursorX, cursorY float64, width, height uint32) (uint8, bool) {
	if slot, ok := InventorySlotAt(cursorX, cursorY, width, height); ok {
		return slot, true
	}
	if width == 0 || height == 0 {
		return 0, false
	}
	x, y := float32(cursorX), float32(cursorY)
	slotSize := hotbarSlotSize * hudScale(true, float32(width), float32(height))
	for index := range core.ChestSlots {
		left, top := chestSlotOrigin(index, float32(width), float32(height))
		if x >= left && x < left+slotSize && y >= top && y < top+slotSize {
			return core.InventorySlots + uint8(index), true
		}
	}
	return 0, false
}

// CraftingOverlay 是背包/合成界面需要显示的全部权威网格值：有效尺寸、统一 9 格
// 与服务端派生的产物。它是 render 本地值，由 app 从已确认镜像转换，渲染层不依赖
// 协议类型；Size 归一为 2 或 3，产物格内容只画镜像里的权威值，客户端不预测。
type CraftingOverlay struct {
	Size   uint8
	Slots  [core.CraftingGridSlots]core.ItemStack
	Output core.ItemStack
}

// appendCraftingGrid 绘制合成区：尺寸 × 尺寸 的网格格与一个独立产物格（设计上
// 与网格之间隔一列、垂直居中），替代既有十条固定配方行。网格格与产物格复用与
// 全部栏位相同的凹槽 cell 与 `appendItemTile` 双层物品色块；数量走既有数字流。
func appendCraftingGrid(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay CraftingOverlay,
	width, height float32,
) {
	size := normalizeCraftingGridSize(int(overlay.Size))
	scale := hudScale(true, width, height)
	padding := hotbarPanelPadding * scale
	// slot 0 在最上一行、最左一列；最下一行（row size-1）锚在容器行上。
	left, top := craftingGridSlotOrigin(0, size, width, height)
	_, bottomY := craftingGridSlotOrigin((size-1)*size, size, width, height)
	outputX, outputY := craftingOutputOrigin(size, width, height)
	dst.quads = append(dst.quads, hotbarInstance{
		X: left - padding, Y: top - padding - containerHeaderHeight*scale,
		Width:  outputX + hotbarSlotSize*scale - left + 2*padding,
		Height: bottomY + hotbarSlotSize*scale - top + 2*padding + containerHeaderHeight*scale,
		Color:  [4]float32{0.035, 0.05, 0.065, 0.96},
	})
	slotUV := hotbarTextureUV(hotbarContainerSlotColumn)
	appendCraftingCell := func(stack core.ItemStack, x, y float32) {
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: hotbarSlotSize * scale, Height: hotbarSlotSize * scale,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
		if stack.Item == core.ItemNone {
			return
		}
		appendItemTile(dst, stack.Item, x, y, scale)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, scale)
	}
	for slot := range size * size {
		x, y := craftingGridSlotOrigin(slot, size, width, height)
		appendCraftingCell(overlay.Slots[slot], x, y)
	}
	appendCraftingCell(overlay.Output, outputX, outputY)

	titleUV := hotbarTextureUV(hotbarCraftingTitleColumn)
	dst.quads = append(dst.quads, hotbarInstance{
		X: left, Y: top - padding - containerHeaderHeight*scale + containerTitleGap*scale,
		Width: containerTitleSize * scale, Height: containerTitleSize * scale,
		U0: titleUV[0], V0: titleUV[1], U1: titleUV[2], V1: titleUV[3],
		Color: [4]float32{1, 1, 1, 1},
	})
}

// craftingGridSlotOrigin 返回统一网格格 0..8 的左上角像素坐标：行主序、
// row 0 在最上一行，与形状表（design.md D2，顶排在先）的阅读方向一致——
// 工具类配方（镐/锄）因此以直立形态呈现在画面上，玩家按配方表摆放即所见；
// 网格最下一行与箱子行共用同一起点，个人尺寸 2 只有格 0..3 有意义。
func craftingGridSlotOrigin(slot, size int, width, height float32) (float32, float32) {
	size = normalizeCraftingGridSize(size)
	row := slot / size
	column := slot % size
	x, y := recipeSlotOrigin(column, width, height)
	return x, y - float32(size-1-row)*(hotbarSlotSize+hotbarSlotGap)*hudScale(true, width, height)
}

// craftingOutputOrigin 返回产物格的左上角像素坐标：在网格右侧隔一列、垂直居中
// （2×2 取底行、3×3 取中行），与网格格保持互不相交。
func craftingOutputOrigin(size int, width, height float32) (float32, float32) {
	size = normalizeCraftingGridSize(size)
	x, baseY := recipeSlotOrigin(size+1, width, height)
	y := baseY - float32((size-1)/2)*(hotbarSlotSize+hotbarSlotGap)*hudScale(true, width, height)
	return x, y
}

// CraftingSlotAt 把光标像素坐标映射为合成界面的统一索引：网格 0..8、背包
// 9..44。个人尺寸 2 的扩展格 4..8 既不画也不命中；界外返回 false。它与绘制共用
// 同一套几何常量与尺寸归一。
func CraftingSlotAt(cursorX, cursorY float64, width, height uint32, size int) (uint8, bool) {
	if slot, ok := InventorySlotAt(cursorX, cursorY, width, height); ok {
		return slot + core.CraftingGridSlots, true
	}
	if width == 0 || height == 0 {
		return 0, false
	}
	size = normalizeCraftingGridSize(size)
	x, y := float32(cursorX), float32(cursorY)
	slotSize := hotbarSlotSize * hudScale(true, float32(width), float32(height))
	for slot := range size * size {
		left, top := craftingGridSlotOrigin(slot, size, float32(width), float32(height))
		if x >= left && x < left+slotSize && y >= top && y < top+slotSize {
			return uint8(slot), true
		}
	}
	return 0, false
}

// CraftingOutputAt 报告光标是否命中产物格。产物格不是普通移动目标：两次点击
// 整堆移动经 `CraftingSlotAt` 组成，产物取出是独立的 `TakeCraftingOutput` 路径。
func CraftingOutputAt(cursorX, cursorY float64, width, height uint32, size int) bool {
	if width == 0 || height == 0 {
		return false
	}
	x, y := float32(cursorX), float32(cursorY)
	slotSize := hotbarSlotSize * hudScale(true, float32(width), float32(height))
	left, top := craftingOutputOrigin(size, float32(width), float32(height))
	return x >= left && x < left+slotSize && y >= top && y < top+slotSize
}

// recipeSlotOrigin 返回容器叠加区第 index 个格子的左上角像素坐标：熔炉三格、
// 箱子每行与合成网格列共用这一行锚点，index 0 与背包最上一行对齐。
func recipeSlotOrigin(index int, width, height float32) (float32, float32) {
	x, _ := inventorySlotOrigin(index, true, width, height)
	_, topRowY := inventorySlotOrigin(core.HotbarSlots, true, width, height)
	scale := hudScale(true, width, height)
	return x, topRowY - (recipeRowGap+hotbarSlotSize)*scale
}

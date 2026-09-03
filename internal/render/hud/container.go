package hud

import (
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	// 合成图式内容：9 个网格格与产物格凹槽、双层物品色块、静态箭头图示与产物
	// 格麦金轮廓底衬。个人 2×2 只画其中 4 个网格格，容量按 3×3 最坏组合锁定。
	craftingContentQuads = (core.CraftingGridSlots + 1) + (core.CraftingGridSlots+1)*2 +
		craftingArrowQuads + craftingOutputOutlineQuads
	craftingArrowQuads = 1
	// 网格与产物各最多两位数量。
	craftingGlyphs = (core.CraftingGridSlots + 1) * 4
	// 熔炉图式内容：三个栏位、双层物品色块、火焰与箭头两条进度的底衬和填充。
	furnaceContentQuads = 3 + 3*2 + 4
	// 三个熔炉格各最多两位数量。
	furnaceGlyphs = 12
	// 箱子图式内容：27 格凹槽加最多 27 个双层物品色块。面板族由
	// `appendContainerPanel` 统一追加，不计入视图内容。
	chestContentQuads = core.ChestSlots + core.ChestSlots*2
	// 箱子每格最多两位数量。
	chestGlyphs = core.ChestSlots * 4

	maxOverlayQuads  = max(craftingContentQuads+recipeColumnQuads, furnaceContentQuads, chestContentQuads)
	maxOverlayGlyphs = max(craftingGlyphs+recipeColumnGlyphs, furnaceGlyphs, chestGlyphs)
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

// inventoryRecipeIDs 是右侧配方栏展示的十条固定配方，顺序即 UI 下标：命中函数
// `RecipeButtonAt` 与绘制共用本表，两条锄头配方按 ID 顺序收在末尾——没有它们
// 玩家在 UI 里拿不到锄头，整条农业闭环在客户端不可达。
var inventoryRecipeIDs = [...]core.RecipeID{
	core.RecipeStoneBricks,
	core.RecipeFurnace,
	core.RecipeIronBlock,
	core.RecipeStonePickaxe,
	core.RecipeIronPickaxe,
	core.RecipeChest,
	core.RecipeOakPlanks,
	core.RecipeLightBlock,
	core.RecipeStoneHoe,
	core.RecipeIronHoe,
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
			x, y := furnaceSlotOrigin(source-core.InventorySlots, width, height)
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
			x, y := inventorySlotOrigin(source-core.CraftingGridSlots, width, height)
			return x, y, true
		}
	}
	if source >= 0 && source < core.InventorySlots {
		x, y := inventorySlotOrigin(source, width, height)
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

// appendFurnaceContent 绘制熔炉的原版图式：输入栏在上、燃料栏在下构成左列，
// 火焰图示居中其间并以自下向上裁剪表达剩余燃烧量；箭头图示横置指向右侧输出
// 栏，以自左向右裁剪表达熔炼进度。两条进度复用既有「底衬加填充」的 4 个
// quad，只挪坐标与尺寸，不新增进度实例。
func appendFurnaceContent(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay FurnaceOverlay,
	width, height float32,
) {
	frame := openPanelAnchor(width, height)
	slotUV := hotbarTextureUV(hotbarContainerSlotColumn)
	stacks := [3]core.ItemStack{overlay.Input, overlay.Fuel, overlay.Output}
	for index, stack := range stacks {
		x, y := furnaceSlotOrigin(index, width, height)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: hotbarSlotSize * frame.scale, Height: hotbarSlotSize * frame.scale,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
		if stack.Item == core.ItemNone {
			continue
		}
		appendItemTile(dst, stack.Item, x, y, frame.scale)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, frame.scale)
	}

	// 两条进度分别显示剩余燃烧量与当前熔炼进度：底衬铺满图示区域，填充按
	// 权威比例裁剪实例与 UV（火焰自下向上、箭头自左向右）。
	flameX, flameY := furnaceFlameOrigin(width, height)
	arrowX, arrowY := furnaceArrowOrigin(width, height)
	bars := [2]struct {
		fraction float32
		uv       [4]float32
		x, y     float32
		barWidth float32
		barSize  float32
		isFlame  bool
	}{
		{float32(overlay.BurnTicks) / float32(core.FurnaceBurnTicks),
			hotbarTextureUV(hotbarFurnaceFlameColumn),
			flameX, flameY, furnaceFlameSize * frame.scale, furnaceFlameSize * frame.scale, true},
		{float32(overlay.ProgressTicks) / float32(core.FurnaceSmeltTicks),
			hotbarTextureUV(hotbarFurnaceArrowColumn),
			arrowX, arrowY, furnaceArrowWidth * frame.scale, furnaceArrowHeight * frame.scale, false},
	}
	for _, bar := range bars {
		dst.quads = append(dst.quads, hotbarInstance{
			X: bar.x, Y: bar.y, Width: bar.barWidth, Height: bar.barSize,
			Color: miningTrackColor,
		})
		fraction := min(bar.fraction, 1)
		if fraction <= 0 {
			continue
		}
		fill := hotbarInstance{U0: bar.uv[0], V0: bar.uv[1], U1: bar.uv[2], V1: bar.uv[3], Color: [4]float32{1, 1, 1, 1}}
		if bar.isFlame {
			fill.X, fill.Y = bar.x, bar.y+bar.barSize*(1-fraction)
			fill.Width, fill.Height = bar.barWidth, bar.barSize*fraction
			fill.V0 = bar.uv[1] + (bar.uv[3]-bar.uv[1])*(1-fraction)
		} else {
			fill.X, fill.Y = bar.x, bar.y
			fill.Width, fill.Height = bar.barWidth*fraction, bar.barSize
			fill.U1 = bar.uv[0] + (bar.uv[2]-bar.uv[0])*fraction
		}
		dst.quads = append(dst.quads, fill)
	}
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
	slotSize := hotbarSlotSize * hudScale(float32(width), float32(height))
	for index := range 3 {
		left, top := furnaceSlotOrigin(index, float32(width), float32(height))
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

// appendChestContent 绘制箱子 27 格凹槽、物品色块与数量，按统一栏位 36..62
// 排布成 3 行 9 列铺在图示区。
func appendChestContent(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay ChestOverlay,
	width, height float32,
) {
	frame := openPanelAnchor(width, height)
	slotUV := hotbarTextureUV(hotbarContainerSlotColumn)
	for index := range core.ChestSlots {
		x, y := chestSlotOrigin(index, width, height)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: hotbarSlotSize * frame.scale, Height: hotbarSlotSize * frame.scale,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
	for index, stack := range overlay.Items {
		if stack.Item == core.ItemNone {
			continue
		}
		x, y := chestSlotOrigin(index, width, height)
		appendItemTile(dst, stack.Item, x, y, frame.scale)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, frame.scale)
	}
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
	slotSize := hotbarSlotSize * hudScale(float32(width), float32(height))
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

// appendCraftingContent 绘制合成图式：尺寸 × 尺寸 的网格格、静态箭头图示与
// 一个产物格（麦金轮廓底衬标记产物语义），右侧再加十条固定配方入口。网格格
// 与产物格复用与全部栏位相同的凹槽 cell 与 `appendItemTile` 双层物品色块；
// 数量走既有数字流。
func appendCraftingContent(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay CraftingOverlay,
	width, height float32,
) {
	size := normalizeCraftingGridSize(int(overlay.Size))
	frame := openPanelAnchor(width, height)
	slotUV := hotbarTextureUV(hotbarContainerSlotColumn)
	appendCraftingCell := func(stack core.ItemStack, x, y float32) {
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: hotbarSlotSize * frame.scale, Height: hotbarSlotSize * frame.scale,
			U0: slotUV[0], V0: slotUV[1], U1: slotUV[2], V1: slotUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
		if stack.Item == core.ItemNone {
			return
		}
		appendItemTile(dst, stack.Item, x, y, frame.scale)
		appendHotbarCountScaled(dst, atlas, stack.Count, x, y, frame.scale)
	}
	// 产物格麦金轮廓底衬：`accentProgress` 只标记产物语义，凹槽 cell 盖住中部
	// 后读作 1 design px 轮廓。
	outputX, outputY := craftingOutputOrigin(size, width, height)
	expand := craftingOutputOutlineExpand * frame.scale
	dst.quads = append(dst.quads, hotbarInstance{
		X: outputX - expand, Y: outputY - expand,
		Width:  hotbarSlotSize*frame.scale + 2*expand,
		Height: hotbarSlotSize*frame.scale + 2*expand,
		Color:  accentProgress,
	})
	for slot := range size * size {
		x, y := craftingGridSlotOrigin(slot, size, width, height)
		appendCraftingCell(overlay.Slots[slot], x, y)
	}
	appendCraftingCell(overlay.Output, outputX, outputY)
	// 箭头图示是静态指示（非进度、非交互），指向产物格。
	arrowUV := hotbarTextureUV(hotbarFurnaceArrowColumn)
	arrowX, arrowY := craftingArrowOrigin(size, width, height)
	dst.quads = append(dst.quads, hotbarInstance{
		X: arrowX, Y: arrowY,
		Width: craftingArrowSize * frame.scale, Height: craftingArrowSize * frame.scale,
		U0: arrowUV[0], V0: arrowUV[1], U1: arrowUV[2], V1: arrowUV[3],
		Color: [4]float32{1, 1, 1, 1},
	})
	appendRecipeColumn(dst, atlas, width, height)
}

// RecipeButtonAt 报告光标是否命中任一固定配方入口，命中时返回配方 ID。入口
// 只存在于个人/工作台合成面板；边界左上闭、右下开，与绘制共用同一套几何。
func RecipeButtonAt(cursorX, cursorY float64, width, height uint32) (core.RecipeID, bool) {
	if width == 0 || height == 0 {
		return 0, false
	}
	x, y := float32(cursorX), float32(cursorY)
	scale := hudScale(float32(width), float32(height))
	for row, recipe := range inventoryRecipeIDs {
		left, top := recipeButtonOrigin(row, float32(width), float32(height))
		if x >= left && x < left+recipeColumnWidth*scale && y >= top && y < top+recipeEntryHeight*scale {
			return recipe, true
		}
	}
	return 0, false
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
	slotSize := hotbarSlotSize * hudScale(float32(width), float32(height))
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
	slotSize := hotbarSlotSize * hudScale(float32(width), float32(height))
	left, top := craftingOutputOrigin(size, float32(width), float32(height))
	return x >= left && x < left+slotSize && y >= top && y < top+slotSize
}

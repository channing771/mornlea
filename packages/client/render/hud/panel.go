package hud

import (
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/core"
)

// panel.go 是四类容器浮动面板的几何单源：面板原点、面板族常量、图示区格、
// 产物格、熔炉流程图式与右侧配方栏的全部矩形都从这里推导，绘制与命中测试
// 共用同一组函数，禁止在别处出现第二份面板坐标常量。

const (
	// containerPanelQuads 是面板族的固定 quad 数：投影、表面、四边 1 design px
	// 描边与一个标题 atlas cell。
	containerPanelQuads = 7
	// hotbarRowWidth 是 9 格行的 design px 宽度：面板内容列、面板下段的快捷栏
	// 行与两条状态行共用同一中轴。
	hotbarRowWidth = core.HotbarSlots*hotbarSlotSize + (core.HotbarSlots-1)*hotbarSlotGap
	// containerPanelWidth 是基础面板宽度：9 格行宽加两侧内边距。
	containerPanelWidth = hotbarRowWidth + 2*hotbarPanelPadding
	// containerPanelHeight 是四类面板共用的 design px 高度（自上而下）：标题
	// header、容器图示区三行、行距、玩家背包三行、行距、快捷栏行与底部内边距。
	// 高度统一取最坏图示区（工作台 3×3 与箱子同为三行），四类视图共用同一
	// 垂直原点，36 个统一栏位的命中矩形不随视图切换漂移。
	containerPanelHeight = containerHeaderHeight +
		(overlayAreaRows*hotbarSlotSize + (overlayAreaRows-1)*hotbarSlotGap) +
		recipeRowGap +
		(overlayAreaRows*hotbarSlotSize + (overlayAreaRows-1)*hotbarSlotGap) +
		inventoryRowGap + hotbarSlotSize + hotbarPanelPadding

	// 右侧配方栏（个人面板的刻意分歧，见 design 记录）：十条固定配方入口竖排，
	// 行高紧凑，栏体悬挂在共享内容列右侧，内容列不随面板加宽而移动。
	recipeColumnWidth = float32(96)
	recipeColumnGap   = float32(8)
	recipeEntryHeight = float32(24)
	recipeEntryGap    = float32(8)
	recipeEntryInset  = float32(2)
	recipeEntryPitch  = recipeEntryHeight + recipeEntryGap
	recipeEntryCount  = len(inventoryRecipeIDs)
	recipeColumnSpan  = 2*(overlayAreaRows*hotbarSlotSize+(overlayAreaRows-1)*hotbarSlotGap) + recipeRowGap
	// recipeColumnSlack 是十条入口在图示区与背包段联合跨度内垂直居中的上余量。
	recipeColumnSlack = (recipeColumnSpan - float32(recipeEntryCount)*recipeEntryHeight -
		float32(recipeEntryCount-1)*recipeEntryGap) / 2
	recipePanelWidth   = containerPanelWidth + recipeColumnGap + recipeColumnWidth
	recipeColumnQuads  = recipeEntryCount * 3
	recipeColumnGlyphs = recipeEntryCount * 4

	// openHUDWidth 是打开态的宽度约束：配方栏面板向右伸出共享内容列
	// `recipeColumnGap+recipeColumnWidth`，对称等效宽度按两侧各伸出一份计
	// （内容列居中不动），再加两侧屏幕边距。
	openHUDWidth = hotbarRowWidth +
		2*(hotbarPanelPadding+recipeColumnGap+recipeColumnWidth+hudEdgeMargin)
	// openHUDHeight 是打开态从面板顶到 framebuffer 下沿的设计高度：统一面板
	// 高度 + 主状态行与氧气行（快捷栏下方既有避让关系）+ 面板下段快捷栏行
	// 外沿到主状态行的 `statusHotbarGap` 可见净空 + 底边距。贴条竖向外沿
	// `hotbarPanelPadding` 已计入 `containerPanelHeight` 的底部内边距，这里
	// 不再重复相加。面板在「顶边到状态栈上沿」剩余空间内垂直居中，该约束
	// 保证居中后仍有不小于屏幕边距的上下余量，面板与状态栈互不相交。
	openHUDHeight = containerPanelHeight + hotbarBottomMargin +
		2*(healthHeartSize+statusBarGap) + statusHotbarGap

	// containerTitleSize/Gap 与 containerHeaderHeight（间隙加标题 cell）构成
	// 面板顶部的标题行；overlayAreaRows 是图示区与背包段的统一行数：3×3 工作
	// 台网格与箱子同为三行，个人 2×2 与熔炉只占其一，高度按最坏组合统一收缩。
	containerTitleSize    = float32(16)
	containerTitleGap     = float32(4)
	containerHeaderHeight = float32(20)
	overlayAreaRows       = 3
	// recipeRowGap 是图示区与玩家背包三行之间的行距。
	recipeRowGap = float32(16)

	// panelShadowExpand 是面板投影层相对表面的外扩（design px）。
	panelShadowExpand = float32(2)
	// panelBorderWidth 是面板描边宽度：1 design px。
	panelBorderWidth = float32(1)

	// 合成图式的箭头图示与产物格间距（design px）。
	craftingArrowSize = float32(24)
	craftingArrowGap  = float32(8)
	// craftingOutputOutlineExpand 是产物格轮廓底衬向四周的外扩：底衬取强调色，
	// 被凹槽 cell 盖住中部后读作 1 px 轮廓。
	craftingOutputOutlineExpand = float32(1)
	craftingOutputOutlineQuads  = 1

	// 熔炉原版图式的火焰与箭头图示尺寸（design px）：火焰居中于输入栏与燃料
	// 栏之间，箭头横置指向右侧输出栏。
	furnaceFlameSize   = float32(32)
	furnaceArrowWidth  = float32(64)
	furnaceArrowHeight = float32(24)
	furnaceArrowGap    = float32(8)
)

// containerView 标识当前打开的容器视图：决定面板宽度（个人面板加配方栏）、
// 标题 cell 与图示区内容。三类互斥，优先级为箱子 > 熔炉 > 合成。
type containerView uint8

const (
	containerViewCrafting containerView = iota
	containerViewFurnace
	containerViewChest
)

// containerTitleColumn 返回视图对应的标题 atlas 列。
func containerTitleColumn(view containerView) int {
	switch view {
	case containerViewFurnace:
		return hotbarFurnaceTitleColumn
	case containerViewChest:
		return hotbarChestTitleColumn
	default:
		return hotbarCraftingTitleColumn
	}
}

// panelOrigin 返回容器浮动面板的左上角像素坐标。
//
// 水平方向：面板按基础面板宽居中，结果恰为「共享 9 格内容列左沿 - 内边距」，
// 与面板下段快捷栏行、两条状态行同一中轴；个人面板的右侧配方栏复用同一
// 原点向右展开——内容列不得随更宽的面板移动，因为统一栏位命中几何与状态行
// 锚点都是视图无关的。
// 垂直方向：在「framebuffer 顶边到打开态底部状态栈上沿」的剩余空间内居中；
// 四类面板共用同一 `containerPanelHeight`，居中结果与当前打开的容器无关。
// 状态栈上沿由 `openBottomStackTop` 推导（主状态行上沿），经同名参数传入。
func panelOrigin(viewportW, viewportH, panelW, panelH, bottomStackTop float32) (float32, float32) {
	x := (viewportW - panelW) * 0.5
	y := (bottomStackTop - panelH) * 0.5
	return x, y
}

// openBottomStackTop 返回打开态底部状态栈的上沿（主状态行顶）：主状态行与氧气
// 行的既有位置不随面板重排移动，面板在它上方的剩余空间内布局。生命/饥饿与氧气
// 两行已迁 WebView 组件呈现，GPU 保留面不再产生它们的实例，但这项预留仍是面板
// 垂直居中的唯一下界；各预留项与 `openHUDHeight` 同序，两处约束因此描述同一份
// 打开态构图。
func openBottomStackTop(width, height float32) float32 {
	scale := hudScale(width, height)
	bottomMargin := hotbarBottomMargin + 2*(healthHeartSize+statusBarGap) + statusHotbarGap + hotbarPanelPadding
	hotbarY := height - (bottomMargin+hotbarSlotSize)*scale
	return hotbarY + (hotbarSlotSize+hotbarPanelPadding+statusHotbarGap+statusBarGap)*scale
}

// containerPanelFrame 是一次打开态布局共享的面板矩形与关键行锚点：全部四类
// 面板的绘制、命中测试与 tooltip 都从这里取坐标。
type containerPanelFrame struct {
	x, y            float32 // 面板左上角（不含投影外扩）
	width, height   float32
	scale           float32
	contentLeft     float32 // 共享 9 格内容列左沿
	illustrationTop float32 // 图示区顶（标题 header 之下）
	backpackTop     float32 // 玩家背包 3×9 顶行
	hotbarY         float32 // 面板下段快捷栏行槽位顶
}

// openPanelAnchor 返回视图无关的面板锚点：原点、缩放与各行位置对四类视图
// 一致，统一栏位命中几何因此不随视图切换漂移。
func openPanelAnchor(width, height float32) containerPanelFrame {
	scale := hudScale(width, height)
	x, y := panelOrigin(width, height,
		containerPanelWidth*scale, containerPanelHeight*scale, openBottomStackTop(width, height))
	frame := containerPanelFrame{
		x: x, y: y,
		width:  containerPanelWidth * scale,
		height: containerPanelHeight * scale,
		scale:  scale,
	}
	frame.contentLeft = x + hotbarPanelPadding*scale
	frame.illustrationTop = y + containerHeaderHeight*scale
	frame.backpackTop = frame.illustrationTop +
		(overlayAreaRows*hotbarSlotSize+(overlayAreaRows-1)*hotbarSlotGap)*scale + recipeRowGap*scale
	frame.hotbarY = frame.backpackTop +
		(overlayAreaRows*hotbarSlotSize+(overlayAreaRows-1)*hotbarSlotGap)*scale + inventoryRowGap*scale
	return frame
}

// openContainerPanel 返回指定视图的面板矩形：与锚点同原点，个人合成面板向右
// 展开配方栏宽度。
func openContainerPanel(view containerView, width, height float32) containerPanelFrame {
	frame := openPanelAnchor(width, height)
	if view == containerViewCrafting {
		frame.width = recipePanelWidth * frame.scale
	}
	return frame
}

// appendContainerPanel 追加浮动面板族：外扩 2 design px 的投影、半透明表面、
// 四边 1 design px 深暖棕描边与一个标题 atlas cell，共 `containerPanelQuads` 个
// quad，零 glyph。调用方必须在任何栏位之前追加。
func appendContainerPanel(dst *hotbarLayout, view containerView, width, height float32) {
	frame := openContainerPanel(view, width, height)
	expand := panelShadowExpand * frame.scale
	edge := panelBorderWidth * frame.scale
	dst.quads = append(dst.quads,
		hotbarInstance{
			X: frame.x - expand, Y: frame.y - expand,
			Width: frame.width + 2*expand, Height: frame.height + 2*expand,
			Color: panelShadow,
		},
		hotbarInstance{X: frame.x, Y: frame.y, Width: frame.width, Height: frame.height, Color: panelSurface},
		hotbarInstance{X: frame.x, Y: frame.y, Width: frame.width, Height: edge, Color: panelBorderLight},
		hotbarInstance{X: frame.x, Y: frame.y + frame.height - edge, Width: frame.width, Height: edge, Color: panelBorderLight},
		hotbarInstance{X: frame.x, Y: frame.y, Width: edge, Height: frame.height, Color: panelBorderLight},
		hotbarInstance{X: frame.x + frame.width - edge, Y: frame.y, Width: edge, Height: frame.height, Color: panelBorderLight},
	)
	titleUV := hotbarTextureUV(containerTitleColumn(view))
	dst.quads = append(dst.quads, hotbarInstance{
		X: frame.contentLeft, Y: frame.y + containerTitleGap*frame.scale,
		Width: containerTitleSize * frame.scale, Height: containerTitleSize * frame.scale,
		U0: titleUV[0], V0: titleUV[1], U1: titleUV[2], V1: titleUV[3],
		Color: [4]float32{1, 1, 1, 1},
	})
}

// chestSlotOrigin 返回箱子统一格 0..26 的左上角像素坐标：3 行 9 列铺在图示区，
// 行 0 在最上一行，与玩家背包的行主序阅读方向一致。
func chestSlotOrigin(index int, width, height float32) (float32, float32) {
	frame := openPanelAnchor(width, height)
	row := index / core.HotbarSlots
	column := index % core.HotbarSlots
	return frame.contentLeft + float32(column)*(hotbarSlotSize+hotbarSlotGap)*frame.scale,
		frame.illustrationTop + float32(row)*(hotbarSlotSize+hotbarSlotGap)*frame.scale
}

// craftingGridSlotOrigin 返回统一网格格 0..8 的左上角像素坐标：行主序、row 0
// 在最上一行，与形状表（顶排在先）的阅读方向一致——工具类配方因此以直立
// 形态呈现在画面上；个人尺寸 2 只有格 0..3 有意义。
func craftingGridSlotOrigin(slot, size int, width, height float32) (float32, float32) {
	size = normalizeCraftingGridSize(size)
	frame := openPanelAnchor(width, height)
	row := slot / size
	column := slot % size
	return frame.contentLeft + float32(column)*(hotbarSlotSize+hotbarSlotGap)*frame.scale,
		frame.illustrationTop + float32(row)*(hotbarSlotSize+hotbarSlotGap)*frame.scale
}

// craftingGridRight 返回尺寸 × 尺寸 网格的右沿像素坐标，供箭头与产物格定位。
func craftingGridRight(size int, width, height float32) float32 {
	size = normalizeCraftingGridSize(size)
	frame := openPanelAnchor(width, height)
	return frame.contentLeft +
		(float32(size)*hotbarSlotSize+float32(size-1)*hotbarSlotGap)*frame.scale
}

// craftingOutputOrigin 返回产物格的左上角像素坐标：在网格右侧隔箭头图示、
// 垂直居中于网格（2×2 取底行、3×3 取中行），与网格格、箭头互不相交。
func craftingOutputOrigin(size int, width, height float32) (float32, float32) {
	size = normalizeCraftingGridSize(size)
	frame := openPanelAnchor(width, height)
	x := craftingGridRight(size, width, height) + (craftingArrowGap+craftingArrowSize+craftingArrowGap)*frame.scale
	y := frame.illustrationTop + float32((size-1)/2)*(hotbarSlotSize+hotbarSlotGap)*frame.scale
	return x, y
}

// craftingArrowOrigin 返回合成图式箭头图示（静态指示，非交互）的左上角像素
// 坐标：贴在网格与产物格之间，垂直对准产物格中心。
func craftingArrowOrigin(size int, width, height float32) (float32, float32) {
	size = normalizeCraftingGridSize(size)
	frame := openPanelAnchor(width, height)
	outputX, outputY := craftingOutputOrigin(size, width, height)
	return outputX - (craftingArrowGap+craftingArrowSize)*frame.scale,
		outputY + (hotbarSlotSize-craftingArrowSize)*frame.scale*0.5
}

// furnaceSlotOrigin 返回熔炉统一格下标（0 输入、1 燃料、2 输出）的左上角像素
// 坐标：原版图式里输入栏在上、燃料栏在下构成左列，输出栏在箭头图示右侧。
// 下标 36/37/38 的语义不变，只挪像素位置。
func furnaceSlotOrigin(index int, width, height float32) (float32, float32) {
	frame := openPanelAnchor(width, height)
	pitch := (hotbarSlotSize + hotbarSlotGap) * frame.scale
	switch index {
	case 0:
		return frame.contentLeft, frame.illustrationTop
	case 1:
		return frame.contentLeft, frame.illustrationTop + 2*pitch
	default:
		return frame.contentLeft +
				(hotbarSlotSize+furnaceArrowGap+furnaceArrowWidth+furnaceArrowGap)*frame.scale,
			frame.illustrationTop + pitch
	}
}

// furnaceFlameOrigin 返回燃烧进度火焰图示区域的左上角像素坐标：32 design px
// 见方，水平对齐输入栏中轴、垂直居中于输入栏底沿与燃料栏顶沿之间。
func furnaceFlameOrigin(width, height float32) (float32, float32) {
	frame := openPanelAnchor(width, height)
	pitch := (hotbarSlotSize + hotbarSlotGap) * frame.scale
	slotSize := hotbarSlotSize * frame.scale
	flameSize := furnaceFlameSize * frame.scale
	return frame.contentLeft + (slotSize-flameSize)*0.5,
		frame.illustrationTop + slotSize + (2*pitch-slotSize-flameSize)*0.5
}

// furnaceArrowOrigin 返回熔炼进度箭头图示区域的左上角像素坐标：横置在输入/
// 燃料左列与输出栏之间，垂直对准输出栏中心。
func furnaceArrowOrigin(width, height float32) (float32, float32) {
	frame := openPanelAnchor(width, height)
	_, outputY := furnaceSlotOrigin(2, width, height)
	return frame.contentLeft + (hotbarSlotSize+furnaceArrowGap)*frame.scale,
		outputY + (hotbarSlotSize*frame.scale-furnaceArrowHeight*frame.scale)*0.5
}

// recipeColumnLeft 返回右侧配方栏的左沿像素坐标：共享内容列右沿再加栏间隙。
func recipeColumnLeft(width, height float32) float32 {
	frame := openPanelAnchor(width, height)
	return frame.contentLeft + (hotbarRowWidth+recipeColumnGap)*frame.scale
}

// recipeButtonOrigin 返回第 row 条固定配方入口的左上角像素坐标：十条入口在
// 图示区与背包段的联合跨度内竖排、整体垂直居中，下标与既有固定配方表一致。
func recipeButtonOrigin(row int, width, height float32) (float32, float32) {
	frame := openPanelAnchor(width, height)
	return recipeColumnLeft(width, height),
		frame.illustrationTop + (recipeColumnSlack+float32(row)*recipeEntryPitch)*frame.scale
}

// appendRecipeColumn 绘制个人面板右侧的十条固定配方入口：每条由 `slotWell`
// 凹槽、`slotWellEdge` 上沿内高光与产物物品色块组成，数量数字沿用既有双层
// 数字流。入口是命中目标（`RecipeButtonAt`），不是普通移动目标。
func appendRecipeColumn(dst *hotbarLayout, atlas render.GlyphSource, width, height float32) {
	frame := openPanelAnchor(width, height)
	wellWidth := recipeColumnWidth * frame.scale
	wellHeight := recipeEntryHeight * frame.scale
	faceSize := (recipeEntryHeight - 2*recipeEntryInset) * frame.scale
	wellEdge := panelBorderWidth * frame.scale
	for row, recipeID := range inventoryRecipeIDs {
		recipe, ok := core.Recipe(recipeID)
		if !ok {
			continue
		}
		x, y := recipeButtonOrigin(row, width, height)
		dst.quads = append(dst.quads,
			hotbarInstance{X: x, Y: y, Width: wellWidth, Height: wellHeight, Color: slotWell},
			hotbarInstance{X: x, Y: y, Width: wellWidth, Height: wellEdge, Color: slotWellEdge},
		)
		// 产物物品色块：可放置方块采样注册表材质，其余用程序化色块。
		face := hotbarInstance{
			X: x + (wellWidth-faceSize)*0.5, Y: y + (wellHeight-faceSize)*0.5,
			Width: faceSize, Height: faceSize, Color: render.ItemColor(recipe.Output.Item),
		}
		if uv, ok := hotbarItemUV(recipe.Output.Item); ok {
			face.U0, face.V0, face.U1, face.V1 = uv[0], uv[1], uv[2], uv[3]
			face.Color = [4]float32{1, 1, 1, 1}
		}
		dst.quads = append(dst.quads, face)
		appendCountAtSize(dst, atlas, recipe.Output.Count, x, y, recipeEntryHeight, frame.scale)
	}
}

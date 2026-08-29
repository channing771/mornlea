package hud

import (
	"github.com/channing771/mornlea/internal/assets"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/mesh"
)

const (
	// HUD 图集前七格是代码生成的固定 survival 图标，随后六格是 container cell，
	// 其后从 `hotbarBlockColumnOffset` 开始按 `ItemID` 放置真实方块顶面。
	hotbarEmptyHeartColumn = iota
	hotbarHalfHeartColumn
	hotbarFullHeartColumn
	hotbarEmptyBubbleColumn
	hotbarFullBubbleColumn
	hotbarEmptyDrumstickColumn
	hotbarFullDrumstickColumn
	hotbarContainerSlotColumn
	hotbarCraftingTitleColumn
	hotbarChestTitleColumn
	hotbarFurnaceTitleColumn
	hotbarFurnaceFlameColumn
	hotbarFurnaceArrowColumn
	hotbarBlockColumnOffset

	hotbarTextureSize = 16
	// 列数按合法物品编号的独占上界 ItemIDMax 预留：追加新物品时图集自动扩出
	// 空白列，与 core 枚举末项守护（item_test.go）保持同一穷举界，不再依赖
	// 「某个具体物品恰为枚举末项」的脆弱假设。
	hotbarTextureColumns = hotbarBlockColumnOffset + int(core.ItemIDMax)
	hotbarTextureWidth   = hotbarTextureColumns * hotbarTextureSize
)

func buildHotbarTextureAtlas(registry *assets.Registry) []byte {
	pixels := make([]byte, hotbarTextureWidth*hotbarTextureSize*4)
	paintHotbarHeart(pixels, hotbarEmptyHeartColumn, heartEmpty)
	paintHotbarHeart(pixels, hotbarHalfHeartColumn, heartHalf)
	paintHotbarHeart(pixels, hotbarFullHeartColumn, heartFull)
	paintHotbarBubble(pixels, hotbarEmptyBubbleColumn, false)
	paintHotbarBubble(pixels, hotbarFullBubbleColumn, true)
	paintHotbarDrumstick(pixels, hotbarEmptyDrumstickColumn, false)
	paintHotbarDrumstick(pixels, hotbarFullDrumstickColumn, true)
	paintContainerSlot(pixels, hotbarContainerSlotColumn)
	paintContainerTitle(pixels, hotbarCraftingTitleColumn, craftingTitleMasks)
	paintContainerTitle(pixels, hotbarChestTitleColumn, chestTitleMasks)
	paintContainerTitle(pixels, hotbarFurnaceTitleColumn, furnaceTitleMasks)
	paintContainerFlame(pixels, hotbarFurnaceFlameColumn)
	paintContainerArrow(pixels, hotbarFurnaceArrowColumn)
	// 物品列按 ItemID 穷举到独占上界 ItemIDMax（与列数预留同一穷举界），
	// 不可放置的物品没有对应列内容，保持空白。
	for item := core.ItemStone; item < core.ItemIDMax; item++ {
		block, ok := core.ItemPlacement(item)
		if !ok {
			continue
		}
		layer := registry.Material(block, mesh.FacePosY)
		copyHotbarTextureCell(pixels, hotbarBlockColumnOffset+int(item), registry.LayerRGBA(int(layer)))
	}
	return pixels
}

var (
	craftingTitleMasks = [2][7]string{
		{".###...", "#...#..", "#####..", "#...#..", "#...#..", "#...#..", "......."},
		{"#....#.", ".#..#..", "..##...", ".##.#..", "#..#...", "#...#..", "......."},
	}
	chestTitleMasks = [2][7]string{
		{"#.#.#..", ".#.#...", "#####..", ".#.#...", "#####..", ".#.#...", "......."},
		{".###...", "...#...", ".###...", "...#...", "...#...", ".###...", "......."},
	}
	furnaceTitleMasks = [2][7]string{
		{"#...#..", "#####..", ".#.#...", "#####..", ".#.#...", "#...#..", "......."},
		{"#......", "#####..", "#.#....", "#.#....", "#.#....", "#.#....", "......."},
	}
)

// paintContainerSlot 以浅边、面色和深边绘制可复用的 16px 凹槽，不依赖外部美术。
func paintContainerSlot(dst []byte, column int) {
	for y := range hotbarTextureSize {
		for x := range hotbarTextureSize {
			color := [4]byte{40, 52, 58, 255}
			switch {
			case x == 0 || y == 0:
				color = [4]byte{148, 166, 174, 255}
			case x >= hotbarTextureSize-2 || y >= hotbarTextureSize-2:
				color = [4]byte{16, 24, 30, 255}
			case x == 1 || y == 1:
				color = [4]byte{90, 108, 116, 255}
			}
			paintContainerPixel(dst, column, x, y, color)
		}
	}
}

// paintContainerTitle 用两枚 7×7 原创掩码生成一个标题 cell，避免标题进入 glyph 流。
func paintContainerTitle(dst []byte, column int, masks [2][7]string) {
	color := [4]byte{232, 238, 222, 255}
	paintPixelMask(dst, column, 1, 4, masks[0], color)
	paintPixelMask(dst, column, 8, 4, masks[1], color)
}

// paintPixelMask 把 `#` 像素按给定偏移写进一个固定 cell；掩码只供 atlas 构建使用。
func paintPixelMask(dst []byte, column, offsetX, offsetY int, mask [7]string, color [4]byte) {
	for y, row := range mask {
		for x, pixel := range row {
			if pixel == '#' {
				paintContainerPixel(dst, column, offsetX+x, offsetY+y, color)
			}
		}
	}
}

// paintContainerFlame 以固定整数像素给熔炉燃烧进度一枚不依赖字体的原创图示。
func paintContainerFlame(dst []byte, column int) {
	rows := [...]struct{ left, right int }{
		{7, 8}, {6, 8}, {6, 9}, {5, 10}, {5, 10}, {6, 9}, {6, 9}, {7, 8},
	}
	for y, row := range rows {
		for x := row.left; x <= row.right; x++ {
			color := [4]byte{238, 116, 30, 255}
			if y >= 5 && x >= 7 && x <= 8 {
				color = [4]byte{255, 218, 86, 255}
			}
			paintContainerPixel(dst, column, x, y+3, color)
		}
	}
}

// paintContainerArrow 以固定整数像素给熔炼进度一枚右向图示。
func paintContainerArrow(dst []byte, column int) {
	color := [4]byte{102, 202, 238, 255}
	for y := 6; y <= 9; y++ {
		for x := 3; x <= 10; x++ {
			paintContainerPixel(dst, column, x, y, color)
		}
	}
	for y, width := range [...]int{1, 2, 3, 4, 3, 2, 1} {
		for x := 0; x < width; x++ {
			paintContainerPixel(dst, column, 10+x, y+4, color)
		}
	}
}

// paintContainerPixel 写入 atlas 中已知合法的固定坐标，所有容器 painter 共用它保持行距一致。
func paintContainerPixel(dst []byte, column, x, y int, color [4]byte) {
	offset := (y*hotbarTextureWidth + column*hotbarTextureSize + x) * 4
	copy(dst[offset:offset+4], color[:])
}
func copyHotbarTextureCell(dst []byte, column int, src []byte) {
	for y := range hotbarTextureSize {
		dstStart := (y*hotbarTextureWidth + column*hotbarTextureSize) * 4
		srcStart := y * hotbarTextureSize * 4
		copy(dst[dstStart:dstStart+hotbarTextureSize*4], src[srcStart:srcStart+hotbarTextureSize*4])
	}
}

// hotbarUVInsetTexels 是每列左右 UV 界向列内对称收进的亚纹素余量（1/256 纹素）。
//
// 为什么是 1/256：归一化 UV 在图集扩列时被 float32 重归一化，解码回纹素空间
// 的噪声上界为 `W·2^-24` 纹素且随宽度线性增长；适用域 `W <= 2^15` 纹素
// （2048 列）内，收进后解码界距列边界恒 >= delta spec 要求的 1/512 纹素。
// 当前真实宽度 `hotbarTextureWidth` 纹素下噪声 <= 6.3e-5 纹素，实际裕度比 >30×。物品表规模
// 一旦可能超出该适用域，必须重审余量而不是沿用本常量。
//
// 收进同时消除「采样点恰好落在列边界」的实现定义 tie-break：不收进时边界
// 采样归属哪一列取决于采样器的舍入方向，扩列即可翻转；收进后任何采样点
// 与列边界的距离恒大于重归一化噪声上界，归属完全确定。
//
// v 轴不收进：图集是单行、仅 16 纹素高，v ∈ [0,1] 内部不存在列界歧义，
// 上下边缘由采样器的 ClampToEdge 兜底，无需对称处理。
const hotbarUVInsetTexels = 1.0 / 256.0

// hotbarColumnUV 把「列纹素边界 ± 亚纹素收进，再 ÷ 图集宽度」的计算参数化为
// 任意宽度，返回该列的归一化 UV 区间 [`left`, 0, `right`, 1]。从 `hotbarTextureUV`
// 中提取这个纯函数是为了可测性：图集宽度随物品表追加自动扩列，稳定性属性
// 测试只有拿到宽度参数才能扫描「模拟未来扩列」的宽度集，在同一份计算上
// 机械验证「扩列不改变既有列采样纹素集合」「相邻列区间互不侵入」的性质。
// 生产路径不得绕过 `hotbarTextureUV` 直接调用本函数。
func hotbarColumnUV(column, width int) [4]float32 {
	left := (float32(column*hotbarTextureSize) + hotbarUVInsetTexels) / float32(width)
	right := (float32((column+1)*hotbarTextureSize) - hotbarUVInsetTexels) / float32(width)
	return [4]float32{left, 0, right, 1}
}

// hotbarTextureUV 是 UV 计算的唯一生产入口，签名 `(column int) [4]float32`
// 保持不变（消费方零改动）；它只是把当前图集宽度 `hotbarTextureWidth`
// 钉进 `hotbarColumnUV` 的薄包装。
func hotbarTextureUV(column int) [4]float32 {
	return hotbarColumnUV(column, hotbarTextureWidth)
}
func hotbarItemUV(item core.ItemID) ([4]float32, bool) {
	if _, ok := core.ItemPlacement(item); !ok {
		return [4]float32{}, false
	}
	return hotbarTextureUV(hotbarBlockColumnOffset + int(item)), true
}

func hotbarBubbleUV(full bool) [4]float32 {
	if full {
		return hotbarTextureUV(hotbarFullBubbleColumn)
	}
	return hotbarTextureUV(hotbarEmptyBubbleColumn)
}

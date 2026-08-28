// Package assets 提供方块注册表与程序化材质。
package assets

const texSize = 16

type rgb struct{ R, G, B uint8 }

func hash2(x, y, salt uint32) uint32 {
	h := x*374761393 + y*668265263 + salt*2246822519
	h = (h ^ (h >> 13)) * 1274126177
	return h ^ (h >> 16)
}

func noisyTexture(base rgb, spread int32, salt uint32) []byte {
	px := make([]byte, texSize*texSize*4)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			n := int32(hash2(uint32(x), uint32(y), salt)%uint32(2*spread+1)) - spread
			i := (y*texSize + x) * 4
			px[i] = clamp8(int32(base.R) + n)
			px[i+1] = clamp8(int32(base.G) + n)
			px[i+2] = clamp8(int32(base.B) + n)
			px[i+3] = 255
		}
	}
	return px
}

func stoneTexture() []byte {
	px := noisyTexture(rgb{R: 128, G: 128, B: 128}, 14, 0x2545)
	for _, point := range [][2]int{{2, 3}, {3, 3}, {3, 4}, {9, 10}, {10, 10}, {10, 11}, {11, 11}} {
		paint(px, point[0], point[1], rgb{R: 92, G: 94, B: 98})
	}
	return px
}

func dirtTexture() []byte {
	px := noisyTexture(rgb{R: 134, G: 96, B: 67}, 10, 0x1B87)
	for _, point := range [][2]int{{2, 2}, {3, 2}, {10, 5}, {10, 6}, {6, 11}, {7, 11}, {13, 14}, {14, 14}} {
		paint(px, point[0], point[1], rgb{R: 166, G: 111, B: 68})
	}
	return px
}

func grassTopTexture() []byte {
	px := noisyTexture(rgb{R: 88, G: 140, B: 60}, 14, 0x9E37)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			if hash2(uint32(x), uint32(y), 0x51ED)%5 != 0 {
				continue
			}
			i := (y*texSize + x) * 4
			px[i+1] = clamp8(int32(px[i+1]) + 30)
		}
	}
	for _, point := range [][2]int{
		{-1, 4}, {0, 4}, {1, 4},
		{6, 9}, {6, 10}, {11, 2}, {12, 2}, {13, 12}, {14, 12},
	} {
		paintWrapped(px, point[0], point[1], rgb{R: 60, G: 108, B: 45})
	}
	for _, point := range [][2]int{
		{4, 2}, {5, 2}, {5, 3},
		{8, 7}, {9, 7}, {8, 8},
		{2, 12}, {3, 12}, {3, 13},
		{10, -1}, {10, 0}, {10, 1},
	} {
		paintWrapped(px, point[0], point[1], rgb{R: 105, G: 174, B: 72})
	}
	return px
}

func grassSideTexture() []byte {
	px := dirtTexture()
	depths := [...]int{3, 4, 4, 5, 6, 6, 5, 4, 3, 3, 4, 5, 5, 4, 3, 3}
	for x := 0; x < texSize; x++ {
		depth := depths[x]
		for y := 0; y < depth; y++ {
			n := int32(hash2(uint32(x), uint32(y), 0x77C1)%25) - 12
			i := (y*texSize + x) * 4
			px[i] = clamp8(88 + n)
			px[i+1] = clamp8(140 + n)
			px[i+2] = clamp8(60 + n)
		}
		paint(px, x, depth-1, rgb{R: 60, G: 108, B: 45})
	}
	return px
}

func stoneBrickTexture() []byte {
	px := noisyTexture(rgb{R: 126, G: 122, B: 116}, 8, 0x77B1)
	mortar := rgb{R: 72, G: 72, B: 74}
	for x := 0; x < texSize; x++ {
		paint(px, x, 7, mortar)
		paint(px, x, 15, mortar)
	}
	for y := 0; y < 7; y++ {
		paint(px, 4, y, mortar)
	}
	for y := 8; y < 15; y++ {
		paint(px, 12, y, mortar)
	}
	return px
}

func oreTexture(ore rgb) []byte {
	px := noisyTexture(rgb{R: 124, G: 124, B: 126}, 12, 0x5C3D)
	points := [...][2]int{
		{2, 3}, {3, 3}, {3, 4}, {4, 4},
		{10, 2}, {10, 3}, {11, 3},
		{7, 9}, {8, 9}, {8, 10}, {9, 10},
		{3, 13}, {4, 13}, {4, 14}, {12, 12}, {12, 13},
	}
	for _, point := range points {
		paint(px, point[0], point[1], ore)
	}
	return px
}

func furnaceTexture() []byte {
	px := noisyTexture(rgb{R: 100, G: 98, B: 100}, 8, 0x41D7)
	frame := rgb{R: 122, G: 120, B: 124}
	for i := 0; i < texSize; i++ {
		paint(px, i, 0, frame)
		paint(px, i, 15, frame)
		paint(px, 0, i, frame)
		paint(px, 15, i, frame)
	}
	fill(px, 4, 5, 12, 13, rgb{R: 40, G: 40, B: 44})
	for x := 5; x < 11; x += 2 {
		paint(px, x, 11, rgb{R: 212, G: 94, B: 36})
	}
	return px
}

func ironBlockTexture() []byte {
	px := noisyTexture(rgb{R: 218, G: 220, B: 224}, 6, 0x2E95)
	frame := rgb{R: 154, G: 158, B: 166}
	for i := 0; i < texSize; i++ {
		paint(px, i, 0, frame)
		paint(px, i, 15, frame)
		paint(px, 0, i, frame)
		paint(px, 15, i, frame)
	}
	for _, point := range [][2]int{{2, 2}, {13, 2}, {2, 13}, {13, 13}} {
		paint(px, point[0], point[1], rgb{R: 118, G: 122, B: 132})
	}
	return px
}

func chestTexture() []byte {
	px := noisyTexture(rgb{R: 156, G: 108, B: 58}, 10, 0x9C4E)
	seam := rgb{R: 86, G: 54, B: 30}
	for x := 0; x < texSize; x++ {
		paint(px, x, 5, seam)
		paint(px, x, 11, seam)
	}
	fill(px, 7, 7, 9, 10, rgb{R: 214, G: 178, B: 74})
	return px
}

func lightBlockTexture() []byte {
	px := noisyTexture(rgb{R: 238, G: 196, B: 76}, 8, 0x4C17)
	frame := rgb{R: 164, G: 106, B: 30}
	for i := 0; i < texSize; i++ {
		paint(px, i, 0, frame)
		paint(px, i, texSize-1, frame)
		paint(px, 0, i, frame)
		paint(px, texSize-1, i, frame)
	}
	fill(px, 4, 4, 12, 12, rgb{R: 255, G: 226, B: 112})
	return px
}

func leavesTexture() []byte {
	px := make([]byte, texSize*texSize*4)
	colors := [...]rgb{
		{R: 48, G: 108, B: 44}, {R: 62, G: 126, B: 54},
		{R: 70, G: 136, B: 58}, {R: 78, G: 144, B: 62},
	}
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			color := colors[hash2(uint32(x/2), uint32(y/2), 0x1EA5)%uint32(len(colors))]
			paint(px, x, y, color)
			if hash2(uint32(x), uint32(y), 0x1EA5)%10 < 3 {
				px[(y*texSize+x)*4+3] = 0
			}
		}
	}
	return px
}

func glassTexture() []byte {
	px := make([]byte, texSize*texSize*4)
	frame := rgb{R: 188, G: 222, B: 226}
	for i := 0; i < texSize; i++ {
		paint(px, i, 0, frame)
		paint(px, i, texSize-1, frame)
		paint(px, 0, i, frame)
		paint(px, texSize-1, i, frame)
	}
	for _, p := range [][2]int{{0, 0}, {15, 0}, {0, 15}, {15, 15}} {
		paint(px, p[0], p[1], rgb{R: 142, G: 184, B: 190})
	}
	for i := 3; i < 7; i++ {
		paint(px, i, i, rgb{R: 224, G: 244, B: 246})
	}
	for i := 9; i < 13; i++ {
		paint(px, i, 14-i, rgb{R: 224, G: 244, B: 246})
	}
	return px
}

// waterAlpha 是水材质层的固定 alpha。
//
// 取 160/255：足够低使水底地形透过水面可见，又足够高让水面本身呈现可辨识
// 的水色。水面走 alpha blend 而非 cutout，因此这里**不能**是 0 或 255——
// 前者让整片水消失，后者让水变成不透明蓝方块。
const waterAlpha = 160

// waterTexture 生成半透明的蓝色水面材质。
//
// 结构：以噪声给出细碎的深浅变化（避免大面积同色带来的塑料感），再叠两条
// 错开的亮色波纹让水面在世界坐标 UV 下有可辨认的流向。全部像素共用
// waterAlpha，逐像素蓝色主导（B 严格大于 R 与 G），守卫见
// TestWaterTextureIsTranslucentBlue。
func waterTexture() []byte {
	px := noisyTexture(rgb{R: 42, G: 96, B: 186}, 12, 0x57A2)
	for _, point := range [][2]int{
		{1, 3}, {2, 3}, {3, 4}, {4, 4}, {5, 3}, {6, 3},
		{9, 10}, {10, 10}, {11, 11}, {12, 11}, {13, 10}, {14, 10},
	} {
		paint(px, point[0], point[1], rgb{R: 96, G: 158, B: 226})
	}
	for i := 3; i < len(px); i += 4 {
		px[i] = waterAlpha
	}
	return px
}

// torchTexture 生成火把五种形态共用的 cutout 材质：竖直窄木柄（中间两列、
// 自图像底部向上）＋顶部暖色火芯（外橙内黄），其余像素 alpha=0。
//
// 两处几何约束决定了像素位置：
//   - 世界坐标锁定 UV 下纹理第 0 行对应方块顶面、第 15 行对应底面，木柄
//     因此自底（第 15 行）向上生长，火芯落在图像顶部；
//   - 墙面形态的斜板顶缘只抬到 (13+1)/16，纹理前两行被几何裁掉，火芯必须
//     全部画在第 2 行及以下，四种墙面火把才看得到火焰。
//
// alpha 只取 0/255（terrain pass 的 `c.a < 0.5` discard），mip 链由
// `downsampleCutout` 保住窄柄覆盖率——守卫见 TestCutoutLayersUseBinaryAlpha
// 与 TestTorchFormsUseDedicatedCutoutLayer。
func torchTexture() []byte {
	px := make([]byte, texSize*texSize*4)
	// 木柄：第 7..15 行 × 第 7..8 列，棕色底带纵向木纹噪声。
	for y := 7; y < texSize; y++ {
		for _, x := range [...]int{7, 8} {
			n := int32(hash2(uint32(x), uint32(y), 0x70C1)%17) - 8
			paint(px, x, y, rgb{
				R: clamp8(118 + n),
				G: clamp8(82 + n),
				B: clamp8(46 + n),
			})
		}
	}
	// 火芯：第 2..5 行的暖色火苗——外圈橙、核心黄，第 6 行是暗色余烬收口。
	fill(px, 7, 2, 9, 3, rgb{R: 232, G: 120, B: 30})
	fill(px, 6, 3, 10, 5, rgb{R: 232, G: 120, B: 30})
	fill(px, 7, 3, 9, 4, rgb{R: 250, G: 170, B: 60})
	fill(px, 7, 4, 9, 6, rgb{R: 255, G: 220, B: 110})
	fill(px, 7, 6, 9, 7, rgb{R: 128, G: 52, B: 28})
	return px
}

func cobblestoneTexture() []byte {
	px := noisyTexture(rgb{R: 116, G: 118, B: 120}, 10, 0xC0B1)
	seam := rgb{R: 70, G: 72, B: 74}
	for x := 0; x < texSize; x++ {
		paint(px, x, 5, seam)
		paint(px, x, 11, seam)
	}
	for y := 0; y < 5; y++ {
		paint(px, 4, y, seam)
		paint(px, 11, y, seam)
	}
	for y := 6; y < 11; y++ {
		paint(px, 7, y, seam)
		paint(px, 13, y, seam)
	}
	for y := 12; y < texSize; y++ {
		paint(px, 3, y, seam)
		paint(px, 10, y, seam)
	}
	return px
}

func smoothStoneTexture() []byte {
	px := noisyTexture(rgb{R: 142, G: 142, B: 140}, 6, 0x5A10)
	for cellY := 0; cellY < 4; cellY++ {
		for cellX := 0; cellX < 4; cellX++ {
			x := cellX*4 + int(hash2(uint32(cellX), uint32(cellY), 0x5A10)%3)
			y := cellY*4 + int(hash2(uint32(cellY), uint32(cellX), 0x5A10)%3)
			paint(px, x, y, rgb{R: 132, G: 134, B: 134})
		}
	}
	return px
}

func sandTexture() []byte {
	px := noisyTexture(rgb{R: 218, G: 202, B: 146}, 8, 0x5A2D)
	bright := rgb{R: 240, G: 226, B: 168}
	for _, point := range [][2]int{
		{2, 3}, {3, 3}, {8, 7}, {8, 8}, {12, 12}, {13, 12},
		{5, 5}, {6, 5}, {10, 14}, {11, 14}, {14, 9}, {14, 10},
	} {
		paint(px, point[0], point[1], bright)
	}
	for _, point := range [][2]int{{5, 1}, {10, 4}, {4, 10}, {14, 6}, {1, 14}} {
		paint(px, point[0], point[1], rgb{R: 196, G: 180, B: 126})
	}
	return px
}

func gravelTexture() []byte {
	px := noisyTexture(rgb{R: 112, G: 108, B: 106}, 12, 0x6A41)
	for index, origin := range [][2]int{{2, 2}, {10, 3}, {5, 9}, {12, 12}} {
		color := rgb{R: 82, G: 80, B: 80}
		if index%2 != 0 {
			color = rgb{R: 144, G: 140, B: 136}
		}
		fill(px, origin[0], origin[1], origin[0]+2, origin[1]+2, color)
	}
	return px
}

func oakLogSideTexture() []byte {
	px := noisyTexture(rgb{R: 112, G: 76, B: 42}, 8, 0x0A61)
	for _, x := range []int{2, 6, 11, 14} {
		for y := 0; y < texSize; y++ {
			paint(px, x, y, rgb{R: 72, G: 46, B: 28})
		}
	}
	return px
}

func oakLogTopTexture() []byte {
	px := noisyTexture(rgb{R: 166, G: 126, B: 72}, 6, 0x0A62)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			radius := max(absInt(2*x-15), absInt(2*y-15))
			if radius == 5 || radius == 11 {
				paint(px, x, y, rgb{R: 202, G: 158, B: 90})
			}
		}
	}
	return px
}

func oakPlanksTexture() []byte {
	px := noisyTexture(rgb{R: 174, G: 124, B: 68}, 8, 0x0A63)
	seam := rgb{R: 100, G: 66, B: 38}
	for x := 0; x < texSize; x++ {
		paint(px, x, 5, seam)
		paint(px, x, 11, seam)
	}
	for y := 0; y < 5; y++ {
		paint(px, 5, y, seam)
	}
	for y := 6; y < 11; y++ {
		paint(px, 11, y, seam)
	}
	for y := 12; y < texSize; y++ {
		paint(px, 4, y, seam)
	}
	fill(px, 12, 2, 14, 4, rgb{R: 86, G: 54, B: 32})
	return px
}

// workbenchTopTexture 是工作台顶面：比素木板略深的木色台面，外框一圈深色
// 包边，台面中央用十字分割线分成 2×2 工作区——方块的功能是「把 2×2 个人
// 网格升到 3×3」，顶面像素直接呼应它的自举配方（2×2 木板 → 工作台）。
func workbenchTopTexture() []byte {
	px := noisyTexture(rgb{R: 158, G: 108, B: 58}, 8, 0x0A64)
	frame := rgb{R: 92, G: 58, B: 32}
	for i := 0; i < texSize; i++ {
		paint(px, i, 0, frame)
		paint(px, i, texSize-1, frame)
		paint(px, 0, i, frame)
		paint(px, texSize-1, i, frame)
	}
	split := rgb{R: 104, G: 66, B: 36}
	for i := 2; i < texSize-2; i++ {
		paint(px, i, 7, split)
		paint(px, i, 8, split)
		paint(px, 7, i, split)
		paint(px, 8, i, split)
	}
	// 每个工作区中心点一枚浅色划痕，避免大色块在 mip 链里糊成平面。
	scratch := rgb{R: 188, G: 140, B: 82}
	for _, point := range [][2]int{{4, 4}, {11, 4}, {4, 11}, {11, 11}} {
		paint(px, point[0], point[1], scratch)
	}
	return px
}

// workbenchSideTexture 是工作台侧面：上半是台面下的深色横带（带两枚工具挂
// 节），下半沿用木板竖缝语言但错位半板，与素橡木木板层拉开可辨识的距离。
func workbenchSideTexture() []byte {
	px := noisyTexture(rgb{R: 150, G: 102, B: 54}, 8, 0x0A65)
	seam := rgb{R: 96, G: 60, B: 34}
	for x := 0; x < texSize; x++ {
		paint(px, x, 4, seam)
		paint(px, x, 5, seam)
	}
	for y := 6; y < texSize; y++ {
		paint(px, 3, y, seam)
		paint(px, 11, y, seam)
	}
	pin := rgb{R: 120, G: 84, B: 52}
	fill(px, 2, 1, 3, 3, pin)
	fill(px, 12, 1, 13, 3, pin)
	return px
}

// workbenchBottomTexture 是工作台底面：纯箱底木板，只有横向双缝与两条半板
// 竖缝——底面永远朝下，刻意做成三层里最安静的一层。
func workbenchBottomTexture() []byte {
	px := noisyTexture(rgb{R: 146, G: 98, B: 52}, 8, 0x0A66)
	seam := rgb{R: 100, G: 64, B: 36}
	for x := 0; x < texSize; x++ {
		paint(px, x, 7, seam)
		paint(px, x, 8, seam)
	}
	for y := 0; y < 7; y++ {
		paint(px, 7, y, seam)
	}
	for y := 9; y < texSize; y++ {
		paint(px, 5, y, seam)
		paint(px, 10, y, seam)
	}
	return px
}

// doorTexture 生成木门材质：中褐底色、外框深色包边、中间横向门档与纵向面板线。
func doorTexture() []byte {
	px := noisyTexture(rgb{R: 158, G: 112, B: 58}, 8, 0x0D0A)
	frame := rgb{R: 96, G: 64, B: 36}
	for i := 0; i < texSize; i++ {
		paint(px, i, 0, frame)
		paint(px, i, texSize-1, frame)
		paint(px, 0, i, frame)
		paint(px, texSize-1, i, frame)
	}
	// 中横档与竖向面板缝
	bar := rgb{R: 108, G: 72, B: 40}
	for x := 1; x < texSize-1; x++ {
		paint(px, x, 7, bar)
		paint(px, x, 8, bar)
	}
	for y := 1; y < texSize-1; y++ {
		paint(px, 7, y, bar)
		paint(px, 8, y, bar)
	}
	// 门把手点
	paint(px, 11, 8, rgb{R: 196, G: 168, B: 84})
	return px
}

func brickTexture() []byte {
	px := noisyTexture(rgb{R: 154, G: 74, B: 58}, 8, 0xB21C)
	paintStaggeredSeams(px, rgb{R: 72, G: 66, B: 64})
	return px
}

func whiteWoolTexture() []byte {
	px := noisyTexture(rgb{R: 226, G: 222, B: 210}, 4, 0x7001)
	for index, origin := range [][2]int{{2, 2}, {8, 4}, {4, 10}, {11, 12}} {
		color := rgb{R: 214, G: 212, B: 204}
		if index%2 != 0 {
			color = rgb{R: 232, G: 228, B: 216}
		}
		fill(px, origin[0], origin[1], origin[0]+2, origin[1]+2, color)
	}
	return px
}

func roofTileTexture() []byte {
	px := noisyTexture(rgb{R: 138, G: 62, B: 46}, 6, 0x711E)
	paintStaggeredSeams(px, rgb{R: 72, G: 32, B: 28})
	for _, point := range [][2]int{{1, 3}, {2, 4}, {7, 3}, {8, 4}, {3, 9}, {4, 10}, {11, 9}, {12, 10}} {
		paint(px, point[0], point[1], rgb{R: 174, G: 78, B: 54})
	}
	return px
}

func clayTexture() []byte {
	px := noisyTexture(rgb{R: 132, G: 150, B: 158}, 6, 0xC1A7)
	for _, origin := range [][2]int{{2, 3}, {9, 2}, {5, 10}, {12, 12}} {
		fill(px, origin[0], origin[1], origin[0]+2, origin[1]+2, rgb{R: 126, G: 146, B: 156})
	}
	return px
}

func snowTopTexture() []byte {
	px := noisyTexture(rgb{R: 244, G: 246, B: 244}, 4, 0x5A09)
	for _, point := range [][2]int{{2, 3}, {7, 1}, {12, 4}, {4, 9}, {10, 11}, {14, 14}} {
		paint(px, point[0], point[1], rgb{R: 255, G: 255, B: 255})
	}
	return px
}

func snowSideTexture() []byte {
	px := noisyTexture(rgb{R: 214, G: 228, B: 236}, 5, 0x5A0D)
	for x := 0; x < texSize; x++ {
		paint(px, x, 5, rgb{R: 198, G: 216, B: 228})
		paint(px, x, 11, rgb{R: 202, G: 220, B: 230})
	}
	return px
}

func mossyCobblestoneTexture() []byte {
	px := cobblestoneTexture()
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			i := (y*texSize + x) * 4
			brightness := (int(px[i]) + int(px[i+1]) + int(px[i+2])) / 3
			if brightness >= 90 && hash2(uint32(x), uint32(y), 0xA055)%5 == 0 {
				paint(px, x, y, rgb{R: 86, G: 126, B: 70})
			}
		}
	}
	return px
}

func paintStaggeredSeams(px []byte, seam rgb) {
	for x := 0; x < texSize; x++ {
		paint(px, x, 5, seam)
		paint(px, x, 11, seam)
	}
	for y := 0; y < 5; y++ {
		paint(px, 5, y, seam)
		paint(px, 13, y, seam)
	}
	for y := 6; y < 11; y++ {
		paint(px, 9, y, seam)
	}
	for y := 12; y < texSize; y++ {
		paint(px, 3, y, seam)
		paint(px, 11, y, seam)
	}
}
func fill(px []byte, left, top, right, bottom int, color rgb) {
	for y := top; y < bottom; y++ {
		for x := left; x < right; x++ {
			paint(px, x, y, color)
		}
	}
}

func paint(px []byte, x, y int, color rgb) {
	i := (y*texSize + x) * 4
	px[i] = color.R
	px[i+1] = color.G
	px[i+2] = color.B
	px[i+3] = 255
}

func paintWrapped(px []byte, x, y int, color rgb) {
	x = (x%texSize + texSize) % texSize
	y = (y%texSize + texSize) % texSize
	paint(px, x, y, color)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func clamp8(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// farmlandDryTexture 生成干耕地的顶面材质：比泥土更深的褐色，加三道犁沟。
//
// 干湿两种耕地**只靠明暗区分**（与 Minecraft 的约定一致），因此这里的基色必须
// 明显亮于 farmlandWetTexture，守卫见 TestFarmlandWetIsDarkerThanDry。
func farmlandDryTexture() []byte {
	px := noisyTexture(rgb{R: 108, G: 74, B: 46}, 9, 0x7A31)
	furrow := rgb{R: 78, G: 52, B: 32}
	for _, y := range [...]int{2, 7, 12} {
		for x := 0; x < texSize; x++ {
			paint(px, x, y, furrow)
		}
	}
	return px
}

// farmlandWetTexture 生成湿耕地的顶面材质：近黑的深褐，犁沟同样更深。
func farmlandWetTexture() []byte {
	px := noisyTexture(rgb{R: 56, G: 36, B: 22}, 7, 0x7A32)
	furrow := rgb{R: 34, G: 22, B: 14}
	for _, y := range [...]int{2, 7, 12} {
		for x := 0; x < texSize; x++ {
			paint(px, x, y, furrow)
		}
	}
	return px
}

// wheatStageCount 是小麦的生长阶段数，与 core.WheatStage0ID..WheatStage7ID 一一对应。
const wheatStageCount = 8

// lerp8 在 [from, to] 上按 step/span 线性插值，全整数运算、不引入浮点。
func lerp8(from, to uint8, step, span int) uint8 {
	return uint8(int32(from) + (int32(to)-int32(from))*int32(step)/int32(span))
}

// wheatTexture 生成小麦第 stage（0..7）阶段的 cutout 材质。
//
// 结构：5 根等距麦秆自图像**第 0 行**向上生长。第 0 行在世界坐标 UV 下对应方块
// 底面（terrain.wgsl 的植物分支取 uv = (world.x, world.y)，v = 0 即格子底面），
// 于是「阶段越高长得越高」在画面上就是从地面往上长。秆高 h = 2 + 2*stage，
// 阶段 7 恰好长满 16 行；颜色自嫩芽绿线性插值到成熟金黄，阶段 >= 5 起在顶端
// 三行左右各加宽一列作为麦穗。
//
// 其余像素 alpha 为 0，走 terrain pass 既有的 alpha cutout（`c.a < 0.5` 即
// discard），因此**不得**出现中间 alpha——守卫见 TestCutoutLayersUseBinaryAlpha。
func wheatTexture(stage int) []byte {
	px := make([]byte, texSize*texSize*4)
	young := rgb{R: 74, G: 142, B: 56}
	ripe := rgb{R: 206, G: 174, B: 74}
	base := rgb{
		R: lerp8(young.R, ripe.R, stage, wheatStageCount-1),
		G: lerp8(young.G, ripe.G, stage, wheatStageCount-1),
		B: lerp8(young.B, ripe.B, stage, wheatStageCount-1),
	}
	height := 2 + 2*stage
	salt := 0x3D57 + uint32(stage)
	speck := func(x, y int) {
		n := int32(hash2(uint32(x), uint32(y), salt)%25) - 12
		paint(px, x, y, rgb{
			R: clamp8(int32(base.R) + n),
			G: clamp8(int32(base.G) + n),
			B: clamp8(int32(base.B) + n),
		})
	}
	for _, x := range [...]int{1, 4, 7, 10, 13} {
		for y := 0; y < height; y++ {
			speck(x, y)
		}
		if stage < 5 {
			continue
		}
		// 麦穗：成熟阶段在顶端三行向两侧各加宽一列，读起来才像结了穗而不是草。
		for y := height - 3; y < height; y++ {
			speck(x-1, y)
			speck(x+1, y)
		}
	}
	return px
}

// potatoTexture 生成马铃薯第 stage 阶段的 cutout 材质，复用小麦同形。
//
// 颜色自深绿向成熟黄绿插值，叶形与小麦同为 5 根直秆，阶段 >=5 同样加宽麦穗
// 效果以保持远处覆盖率；占位纯色但保持二值 alpha 契约。
func potatoTexture(stage int) []byte {
	px := make([]byte, texSize*texSize*4)
	young := rgb{R: 68, G: 132, B: 48}
	ripe := rgb{R: 96, G: 168, B: 58}
	base := rgb{
		R: lerp8(young.R, ripe.R, stage, wheatStageCount-1),
		G: lerp8(young.G, ripe.G, stage, wheatStageCount-1),
		B: lerp8(young.B, ripe.B, stage, wheatStageCount-1),
	}
	height := 2 + 2*stage
	salt := 0x3D58 + uint32(stage)
	speck := func(x, y int) {
		n := int32(hash2(uint32(x), uint32(y), salt)%25) - 12
		paint(px, x, y, rgb{
			R: clamp8(int32(base.R) + n),
			G: clamp8(int32(base.G) + n),
			B: clamp8(int32(base.B) + n),
		})
	}
	for _, x := range [...]int{1, 4, 7, 10, 13} {
		for y := 0; y < height; y++ {
			speck(x, y)
		}
		if stage < 5 {
			continue
		}
		for y := height - 3; y < height; y++ {
			speck(x-1, y)
			speck(x+1, y)
		}
	}
	return px
}

// carrotTexture 生成胡萝卜第 stage 阶段的 cutout 材质，复用小麦同形。
//
// 颜色自嫩绿向成熟橙绿插值，阶段越高橙色分量越重以示成熟，结构与小麦同形。
func carrotTexture(stage int) []byte {
	px := make([]byte, texSize*texSize*4)
	young := rgb{R: 74, G: 142, B: 52}
	ripe := rgb{R: 198, G: 132, B: 42}
	base := rgb{
		R: lerp8(young.R, ripe.R, stage, wheatStageCount-1),
		G: lerp8(young.G, ripe.G, stage, wheatStageCount-1),
		B: lerp8(young.B, ripe.B, stage, wheatStageCount-1),
	}
	height := 2 + 2*stage
	salt := 0x3D59 + uint32(stage)
	speck := func(x, y int) {
		n := int32(hash2(uint32(x), uint32(y), salt)%25) - 12
		paint(px, x, y, rgb{
			R: clamp8(int32(base.R) + n),
			G: clamp8(int32(base.G) + n),
			B: clamp8(int32(base.B) + n),
		})
	}
	for _, x := range [...]int{1, 4, 7, 10, 13} {
		for y := 0; y < height; y++ {
			speck(x, y)
		}
		if stage < 5 {
			continue
		}
		for y := height - 3; y < height; y++ {
			speck(x-1, y)
			speck(x+1, y)
		}
	}
	return px
}

// 床面的原创配色（床与睡眠功能行）：全部取自橡木/织物色域的原创像素，不
// 复用也绝不引入任何外部美术资源。struct 无法声明为 Go 常量，这里用包级
// var 表达「颜色表只在此一处」的同一意图。
var (
	// bedFrameColor 是床架包边：深橡木，与门框/工作台包边同一色域。
	bedFrameColor = rgb{R: 92, G: 62, B: 36}
	// bedMattressColor 是床垫基色：暖橡木织物的中调。
	bedMattressColor = rgb{R: 186, G: 144, B: 92}
	// bedSeamColor 是绗缝线：比床垫深一档的橡木褐。
	bedSeamColor = rgb{R: 152, G: 110, B: 62}
	// bedPillowColor 是床头层的枕头带：全层最亮的奶白，是「这头是床头」的
	// 主视觉锚点。
	bedPillowColor = rgb{R: 236, G: 228, B: 206}
	// bedBlanketColor 是床尾层的毯沿带：比床垫深而饱和，读作折过来的毯子。
	bedBlanketColor = rgb{R: 158, G: 112, B: 60}
	// bedBlanketEdgeColor 是毯沿外侧一线的折边高光。
	bedBlanketEdgeColor = rgb{R: 206, G: 166, B: 112}
)

// bedBand 沿床头朝向边、距床架包边内侧 offset 像素处刷一条 width 像素宽的
// 通带。
//
// 带的位置锁死在朝向上：顶面 UV 约定与 terrain.wgsl 的 face_uv 一致
// （列 = world.z、行 = world.x），因此南带在末列侧、北带在首列侧、东带在
// 末行侧、西带在首行侧——四个朝向的床面层因此逐两可辨。带只画包边内侧的
// 2..13 开区间，不覆盖床架包边。
func bedBand(px []byte, dir, offset, width int, color rgb) {
	for i := 2; i < texSize-2; i++ {
		for w := offset; w < offset+width; w++ {
			switch dir {
			case 0: // 南：床头在 +Z → 末列侧
				paint(px, texSize-3-w, i, color)
			case 1: // 西：床头在 −X → 首行侧
				paint(px, i, 2+w, color)
			case 2: // 北：床头在 −Z → 首列侧
				paint(px, 2+w, i, color)
			case 3: // 东：床头在 +X → 末行侧
				paint(px, i, texSize-3-w, color)
			}
		}
	}
}

// bedTopTexture 生成床第 head（false=床尾、true=床头）× dir（南 0/西 1/北 2/
// 东 3）形态的原创床面层。
//
// 结构自外向内：2px 深橡木床架包边；带噪声的暖橡木床垫；两道垂直于床头方向
// 的绗缝线；床头朝向边内侧 3px 亮带——床头层是枕头（奶白），床尾层是折过来的
// 毯沿（深橡木褐 + 内侧一线折边高光）。八个（head × dir）组合各用独立噪声盐，
// 加上带位置随朝向旋转，保证八张层互不相同。全部像素不透明（床面层是普通
// 固体层，不进 cutout 集合）。
func bedTopTexture(head bool, dir int) []byte {
	salt := 0x3E10 + uint32(dir)
	if head {
		salt += 0x20
	}
	px := noisyTexture(bedMattressColor, 8, salt)
	// 床架包边：外圈 2px。
	for i := 0; i < texSize; i++ {
		for _, b := range [...]int{0, 1, texSize - 2, texSize - 1} {
			paint(px, i, b, bedFrameColor)
			paint(px, b, i, bedFrameColor)
		}
	}
	// 绗缝线：垂直于床头方向的两道（床头在 z 轴向时缝线沿列，x 轴向时沿行）。
	for _, seam := range [...]int{6, 9} {
		for i := 2; i < texSize-2; i++ {
			if dir == 0 || dir == 2 {
				paint(px, seam, i, bedSeamColor)
			} else {
				paint(px, i, seam, bedSeamColor)
			}
		}
	}
	if head {
		bedBand(px, dir, 0, 3, bedPillowColor)
		return px
	}
	bedBand(px, dir, 0, 3, bedBlanketColor)
	// 毯沿内侧一线折边高光，让「毯子折过来」可读。
	bedBand(px, dir, 3, 1, bedBlanketEdgeColor)
	return px
}

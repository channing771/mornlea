// 牛四层的原创程序化回退像素：牛皮/牛头是不透明皮毛层，生熟牛肉是镂空
// 图标层。全部取值是本仓原创，不复用也不引入任何外部美术资源；`applyPack`
// 缺文件时这些像素即最终呈现（见 `TestCattleProceduralFallbackPixels`）。
package assets

// 牛皮/牛头两层的奶油风共用取色（对照包内棕色系：泥土 134,96,67、木板
// 174,124,68、耕地沟 78,52,32；斑块棕 R-B=82 与熟牛肉层 78 同暖）。
var (
	cowCream     = rgb{R: 234, G: 224, B: 205}
	cowBrown     = rgb{R: 130, G: 80, B: 48}
	cowEdge      = rgb{R: 182, G: 152, B: 126}
	cowHighlight = rgb{R: 246, G: 236, B: 217}
	cowHoof      = rgb{R: 76, G: 50, B: 34}
)

// cowPatchCell 报告 (x,y) 所在的 8×8 规整斑块格是否为棕斑。
func cowPatchCell(x, y int, salt uint32) bool {
	return hash2(uint32(x/8), uint32(y/8), salt)%4 == 0
}

// cowCoatBase 返回奶油风皮毛在 (x,y) 的底色：8×8 规整棕斑（异类邻格交界处
// 1px 软边混合）+ 非斑格的柔和高光。盐值不同即不同层，用它区分牛皮/牛头。
func cowCoatBase(x, y int, salt uint32) rgb {
	patched := cowPatchCell(x, y, salt)
	if !patched {
		if hash2(uint32(x/8), uint32(y/8), salt^0xC117)%3 == 0 {
			return cowHighlight
		}
		if cowPatchCell(x-1, y, salt) || cowPatchCell(x+1, y, salt) ||
			cowPatchCell(x, y-1, salt) || cowPatchCell(x, y+1, salt) {
			return cowEdge
		}
		return cowCream
	}
	if !cowPatchCell(x-1, y, salt) || !cowPatchCell(x+1, y, salt) ||
		!cowPatchCell(x, y-1, salt) || !cowPatchCell(x, y+1, salt) {
		return cowEdge
	}
	return cowBrown
}

// cowHideTexture 生成牛皮层：奶油底 + 规整棕斑 + 柔和高光 + 深暖蹄棕散点 +
// 细噪点。
//
// 蹄色只能以散点表达：单层贴满腿身六面，无法按腿底定位深色蹄块（几何与层映
// 射冻结），深暖点读作蹄色。全图不透明：牛身层不进 cutout 集合，透明像素会
// 被 discard 啃出破洞。
func cowHideTexture() []byte {
	px := make([]byte, texSize*texSize*4)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			base := cowCoatBase(x, y, 0xC07D)
			// 蹄色散点先于噪点落定，与皮毛同起伏、读作一体。
			if hash2(uint32(x), uint32(y), 0xC0F)%37 == 0 {
				base = cowHoof
			}
			n := int32(hash2(uint32(x), uint32(y), 0xC07E)%21) - 10
			paint(px, x, y, rgb{
				R: clamp8(int32(base.R) + n),
				G: clamp8(int32(base.G) + n),
				B: clamp8(int32(base.B) + n),
			})
		}
	}
	return px
}

// cowHeadTexture 生成牛头层：同族奶油皮毛底（不同盐，与牛皮层逐像素不
// 同）+ 双眼 + 吻部 + 嘴线。
//
// 双眼是上半区两枚深色像素带浅色高光，吻部是底部四行的粉棕横带配两枚鼻孔，
// 嘴线是横带底行的深暖收底——牛头采样在 Avatar 头部，与牛皮层必须一眼可辨。
// 全图不透明，理由同牛皮层。
func cowHeadTexture() []byte {
	px := make([]byte, texSize*texSize*4)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			base := cowCoatBase(x, y, 0xC0E4)
			if hash2(uint32(x), uint32(y), 0xC0F)%37 == 0 {
				base = cowHoof
			}
			n := int32(hash2(uint32(x), uint32(y), 0xC0E5)%21) - 10
			paint(px, x, y, rgb{
				R: clamp8(int32(base.R) + n),
				G: clamp8(int32(base.G) + n),
				B: clamp8(int32(base.B) + n),
			})
		}
	}
	// 双眼：第 5 行左右各一枚深色眼 + 右下一格高光。
	for _, x := range [...]int{4, 11} {
		paint(px, x, 5, rgb{R: 30, G: 22, B: 18})
		paint(px, x+1, 6, rgb{R: 245, G: 238, B: 228})
	}
	// 吻部：第 12..15 行粉棕横带，两枚鼻孔读作朝向，底行嘴线收底。
	fill(px, 2, 12, 14, 16, rgb{R: 198, G: 152, B: 134})
	paint(px, 5, 13, rgb{R: 110, G: 70, B: 58})
	paint(px, 10, 13, rgb{R: 110, G: 70, B: 58})
	for x := 4; x <= 11; x++ {
		paint(px, x, 15, rgb{R: 96, G: 62, B: 50})
	}
	return px
}

// beefMask 报告牛肉图标镂空形状内的像素：以 (8,8) 为心的椭圆加散列边缘抖动，
// 生熟两层共用同一剪影、只换配色，保证图标外形一致而颜色可辨。
func beefMask(x, y int) bool {
	dx, dy := int32(x-8), int32(y-8)
	// 椭圆判据全整数运算：(dx²·25 + dy²·36) ≤ 900 即 (dx/6)²+(dy/5)² ≤ 1。
	inside := dx*dx*25+dy*dy*36 <= 900
	if !inside {
		return false
	}
	edge := dx*dx*25 + dy*dy*36
	if edge > 700 {
		return hash2(uint32(x), uint32(y), 0x3EEF)%3 != 0
	}
	return true
}

// beefTexture 按配色生成牛肉图标：深色描边环 + 基色肉面 + 浅色脂肪点，背景
// 透明。alpha 只取 0/255，与牛肉层的 cutout 分类配套。
func beefTexture(base, edge, fat rgb, salt uint32) []byte {
	px := make([]byte, texSize*texSize*4)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			if !beefMask(x, y) {
				continue
			}
			dx, dy := int32(x-8), int32(y-8)
			color := base
			if dx*dx*25+dy*dy*36 > 650 {
				color = edge
			}
			n := int32(hash2(uint32(x), uint32(y), salt)%21) - 10
			paint(px, x, y, rgb{
				R: clamp8(int32(color.R) + n),
				G: clamp8(int32(color.G) + n),
				B: clamp8(int32(color.B) + n),
			})
		}
	}
	// 脂肪点：肉面内部三处 2×2 浅色块，读作肥肉花纹。
	for _, origin := range [][2]int{{5, 6}, {9, 9}, {6, 10}} {
		for _, offset := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
			x, y := origin[0]+offset[0], origin[1]+offset[1]
			if beefMask(x, y) {
				paint(px, x, y, fat)
			}
		}
	}
	return px
}

// rawBeefTexture 生成生牛肉回退图标：偏红肉面配浅粉脂肪。
func rawBeefTexture() []byte {
	return beefTexture(
		rgb{R: 178, G: 64, B: 48},
		rgb{R: 110, G: 38, B: 30},
		rgb{R: 232, G: 170, B: 150},
		0xBE4F,
	)
}

// cookedBeefTexture 生成熟牛肉回退图标：与生牛肉同剪影的偏棕配色。
func cookedBeefTexture() []byte {
	return beefTexture(
		rgb{R: 122, G: 76, B: 44},
		rgb{R: 74, G: 46, B: 28},
		rgb{R: 188, G: 140, B: 96},
		0xBE50,
	)
}

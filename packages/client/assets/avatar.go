package assets

// `originalHumanTexture` 在装配阶段生成原创像素服饰；四部件各占六个完整面，
// 保持每层独立 mip，避免分面小图在远处发生采样串色。
func originalHumanTexture(index int) []byte {
	palette, part, face := index/24, index%24/6, index%6
	skin, hair := rgb{221, 166, 127}, rgb{70, 50, 40}
	coat, seam := rgb{133, 153, 120}, rgb{96, 119, 88}
	if palette == 1 {
		skin = rgb{179, 123, 91}
		hair = rgb{49, 39, 37}
		coat = rgb{185, 124, 100}
		seam = rgb{142, 87, 71}
	}
	cream, pants, boot := rgb{242, 225, 188}, rgb{83, 96, 99}, rgb{54, 46, 43}
	px := make([]byte, texSize*texSize*4)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			c := coat
			switch part {
			case 0:
				c = skin
				if face == 2 || face == 4 || y < 3 || ((face == 0 || face == 1) && y < 7) {
					c = hair
				}
				if face == 5 {
					if (x < 3 && y < 6) || (x >= 10 && y < 4) || (x >= 6 && x <= 9 && y == 3) {
						c = hair
					}
					if y == 6 && ((x >= 3 && x <= 5) || (x >= 10 && x <= 12)) {
						c = rgb{92, 61, 47}
					}
					if y >= 7 && y <= 8 && ((x >= 3 && x <= 5) || (x >= 10 && x <= 12)) {
						c = cream
					}
					if y >= 7 && y <= 8 && (x == 4 || x == 11) {
						c = rgb{47, 53, 47}
					}
					if x == 8 && y >= 9 && y <= 10 {
						c = rgb{190, 130, 96}
					}
					if y == 12 && x >= 6 && x <= 9 {
						c = rgb{159, 94, 75}
					}
				}
				if (face == 0 || face == 1) && x >= 6 && x <= 8 && y >= 8 && y <= 10 {
					c = rgb{202, 142, 106}
				}
				if face == 3 {
					c = skin
				}
				if face == 4 && (x == 3 || x == 10) && y < 12 {
					c = rgb{82, 61, 47}
				}
			case 1:
				if y == 14 || x == 0 || x == 15 {
					c = seam
				}
				if face == 5 {
					if x >= 5 && x <= 10 {
						c = cream
					}
					if y < 3 && x >= 4 && x <= 11 {
						c = skin
					}
					if (y == 2 && (x == 4 || x == 11)) || (y == 3 && (x == 5 || x == 10)) {
						c = cream
					}
					if (x == 4 || x == 11) && y >= 4 {
						c = seam
					}
					if y >= 9 && y <= 11 && ((x >= 1 && x <= 3) || (x >= 12 && x <= 14)) {
						c = seam
					}
				}
				if face == 4 && y == 4 {
					c = seam
				}
				if face == 2 || face == 3 {
					c = seam
				}
			case 2:
				if x == 0 || x == 15 {
					c = seam
				}
				if y >= 11 && y <= 12 {
					c = cream
				}
				if y >= 13 {
					c = skin
				}
				if face == 3 {
					c = skin
				}
			case 3:
				c = pants
				if x == 0 || x == 15 {
					c = rgb{66, 78, 82}
				}
				if y >= 11 {
					c = boot
				}
				if y == 11 {
					c = rgb{88, 71, 56}
				}
				if face == 5 && y == 13 && x >= 3 && x <= 12 {
					c = rgb{114, 91, 68}
				}
				if y == 15 || face == 3 {
					c = rgb{39, 36, 34}
				}
			}
			// 细微同色明暗保留像素织物质感，顶底面也避免完全平涂。
			n := int32(hash2(uint32(x), uint32(y), uint32(index+1))%5) - 2
			c = rgb{clamp8(int32(c.R) + n), clamp8(int32(c.G) + n), clamp8(int32(c.B) + n)}
			i := (y*texSize + x) * 4
			px[i] = c.R
			px[i+1] = c.G
			px[i+2] = c.B
			px[i+3] = 255
		}
	}
	return px
}

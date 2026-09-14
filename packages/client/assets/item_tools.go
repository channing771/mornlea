package assets

import "github.com/channing771/mornlea/packages/shared/core"

// ItemToolPart 是以握点为原点的实心体素部件；尺寸与颜色缓存随图标一起刷新。
// 柄、柄套、刃脊和刃缘拥有不同截面，不能退化为图标的均匀挤出。
type ItemToolPart struct {
	Center, Size [3]float32
	Color        [4]float32
	// `RotationZ` 只用于弯折刃段，缓存局部转角不进入 ABI。
	RotationZ float32
}

// ItemToolParts 返回注册表拥有的只读工具模型；非工具继续使用原有图标或方块。
func (r *Registry) ItemToolParts(item core.ItemID) ([]ItemToolPart, bool) {
	if item >= core.ItemIDMax {
		return nil, false
	}
	parts := r.itemTools[item]
	return parts, len(parts) > 0
}

func buildItemToolParts(item core.ItemID, px []byte) []ItemToolPart {
	category, broken := 0, false
	switch item {
	case core.ItemBrokenWoodenSword, core.ItemBrokenStoneSword, core.ItemBrokenIronSword:
		category, broken = 1, true
	case core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword:
		category = 1
	case core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe:
		category, broken = 2, true
	case core.ItemStonePickaxe, core.ItemIronPickaxe:
		category = 2
	case core.ItemBrokenStoneHoe, core.ItemBrokenIronHoe:
		category, broken = 3, true
	case core.ItemStoneHoe, core.ItemIronHoe:
		category = 3
	default:
		return nil
	}
	// 从当前图稿的柄与刃区域取实际非透明色；覆盖改变轮廓时就近寻找，
	// 避免硬编码木石铁调色板或沿用旧材质。所有扫描只发生在刷新阶段。
	sample := func(x, y int) [4]float32 {
		best := 1000
		color := [4]float32{1, 1, 1, 1}
		for yy := 0; yy < 16; yy++ {
			for xx := 0; xx < 16; xx++ {
				i := (yy*16 + xx) * 4
				d := (xx-x)*(xx-x) + (yy-y)*(yy-y)
				if px[i+3] >= 128 && d < best {
					best = d
					color = [4]float32{float32(px[i]) / 255, float32(px[i+1]) / 255, float32(px[i+2]) / 255, float32(px[i+3]) / 255}
				}
			}
		}
		return color
	}
	handle := sample(5, 12)
	metal := sample(10, 7)
	edge := sample(9, 8)
	if category == 2 {
		metal, edge = sample(7, 5), sample(7, 4)
	}
	if category == 3 {
		metal, edge = sample(10, 4), sample(10, 3)
	}
	parts := make([]ItemToolPart, 0, 48)
	add := func(x, y, z, w, h, d float32, c [4]float32) {
		parts = append(parts, ItemToolPart{Center: [3]float32{x, y, z}, Size: [3]float32{w, h, d}, Color: c})
	}
	add(0, -.005, 0, .065, .41, .075, handle)
	shade := func(c [4]float32, f float32) [4]float32 {
		for i := 0; i < 3; i++ {
			c[i] = min(1, c[i]*f)
		}
		return c
	}
	// 连续实心柄保留握点，表面窄条形成有明暗变化的原创木纹。
	for i := 0; i < 12; i++ {
		y := float32(i)*.045 - .19
		add(.009, y, .039, .044, .022, .009, shade(handle, .73+float32(i%3)*.13))
		add(-.034, y+.012, 0, .007, .026, .06, shade(handle, .66+float32(i%2)*.12))
	}
	add(0, -.225, 0, .085, .04, .09, edge)
	if category == 1 {
		add(0, .20, 0, .25, .055, .12, shade(metal, .45))
		add(0, .245, 0, .105, .05, .115, shade(metal, .65))
		length := float32(.40)
		if broken {
			length = .18
		}
		add(0, .27+length/2, 0, .085, length, .11, shade(metal, .58))
		add(-.016, .27+length/2, .060, .032, length, .012, shade(metal, .83))
		add(-.055, .27+length/2, 0, .028, length, .05, edge)
		add(.055, .27+length/2, 0, .028, length, .05, edge)
		if broken {
			add(-.022, .27+length+.025, 0, .045, .05, .065, metal)
		} else {
			for i := 0; i < 4; i++ {
				width := .075 - float32(i)*.018
				add(0, .27+length+.012+float32(i)*.02, 0, width, .024, width, edge)
			}
		}
	} else {
		add(0, .28, 0, .065, .20, .075, handle)
		add(0, .37, 0, .115, .13, .125, shade(metal, .66))
		if category == 2 {
			// 逐段下弯的双臂保持实心截面，端部收窄；损坏形态移去右臂。
			for _, sign := range []float32{-1, 1} {
				if broken && sign > 0 {
					continue
				}
				for i := 0; i < 4; i++ {
					x := []float32{.07, .14, .205, .252}[i]
					y := []float32{.414, .389, .345, .285}[i]
					w := []float32{.105, .08, .065, .04}[i]
					add(sign*x, y, 0, w+.015, .085, .12-float32(i)*.009, shade(metal, .60))
					parts[len(parts)-1].RotationZ = -sign * (.15 + float32(i)*.22)
					add(sign*x, y+.032, .005, w+.010, .020, .10-float32(i)*.009, edge)
					parts[len(parts)-1].RotationZ = -sign * (.15 + float32(i)*.22)
				}
			}
		} else {
			length := float32(.20)
			if broken {
				length = .105
			}
			add(-length/2, .415, 0, length, .09, .13, shade(metal, .62))
			width := float32(.16)
			if broken {
				width = .09
			}
			add(-length, .325, 0, width, .15, .12, shade(metal, .72))
			add(-length, .325, .064, width-.025, .12, .012, shade(metal, .88))
			add(-length, .242, 0, width+.018, .024, .07, edge)
		}
	}
	return parts
}

package assets

import (
	"math"

	"github.com/channing771/mornlea/packages/shared/core"
)

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
	shade := func(c [4]float32, f float32) [4]float32 {
		for i := 0; i < 3; i++ {
			c[i] = min(1, c[i]*f)
		}
		return c
	}
	// 实心木纹分段首尾相接，宽色带覆盖四侧；采样更新传播到整个工具。
	handleTop := float32(.20)
	bandCount := 6
	if category != 1 {
		handleTop = .59
		bandCount = 8
		if category == 3 {
			handleTop += .025
		}
	}
	bandHeight := (handleTop + .21) / float32(bandCount)
	for i := 0; i < bandCount; i++ {
		y := -.21 + (float32(i)+.5)*bandHeight
		add(0, y, 0, .065, bandHeight, .075, shade(handle, [...]float32{.18, .29, .21, .33, .23, .30, .20, .32}[i]))
	}
	add(0, -.225, 0, .072, .035, .082, shade(metal, .25))
	if category == 1 {
		add(0, .175, 0, .25, .052, .12, shade(metal, .08))
		for _, sign := range []float32{-1, 1} {
			add(sign*.139, .175, 0, .045, .07, .13, shade(metal, .085))
			parts[len(parts)-1].RotationZ = -sign * .18
		}
		add(0, .175, .061, .25, .052, .002, shade(metal, .27))
		add(0, .220, 0, .105, .045, .115, shade(metal, .35))
		length := float32(.650)
		sections := 6
		if broken {
			length = .21
			sections = 3
		}
		// 刃脊比刃面深，沿长度逐级收窄；尖端连同厚度收窄，形成真正的实心尖刃。
		for i := 0; i < sections; i++ {
			h := length / float32(sections)
			width := [...]float32{.118, .112, .103, .093, .077, .055}[i]
			y := .242 + (float32(i)+.5)*h
			add(0, y, 0, width, h, .105, shade(metal, .075))
			add(-.006, y, .053, width-.012, h, .001, shade(metal, .55))
			add(-width/2+.004, y, .054, .008, h, .002, shade(edge, .94))
			add(-.009, y, .055, .012, h, .002, shade(metal, .75))
		}
		if broken {
			add(-.025, .242+length+.023, 0, .046, .046, .065, shade(metal, .50))
		} else {
			// 菱形上半部与末段直接相接，连续斜刃替代微小阶梯帽。
			const tipBase = float32(.892)
			add(0, tipBase-.005, 0, .039, .039, .105, shade(metal, .075))
			parts[len(parts)-1].RotationZ = math.Pi / 4
			add(-.003, tipBase-.004, .053, .035, .035, .002, shade(metal, .55))
			parts[len(parts)-1].RotationZ = math.Pi / 4
		}
	} else {
		const lift = float32(.22)
		socketY := .37 + lift
		if category == 2 {
			socketY += .04
		}
		if category == 3 {
			socketY += .045
		}
		add(0, socketY, 0, .09, .13, .125, shade(metal, .075))
		if category == 2 {
			// 相邻翼段共享端点并小幅相交，粗大斜面连续下垂，右翼明显更长。
			for _, sign := range []float32{-1, 1} {
				if broken && sign > 0 {
					continue
				}
				points := [][2]float32{{.025, .790}, {.17, .755}, {.30, .630}, {.39, .440}}
				if sign < 0 {
					points = [][2]float32{{.025, .790}, {.17, .745}, {.29, .670}, {.40, .575}}
				}
				for i := 0; i < 3; i++ {
					a, b := points[i], points[i+1]
					dx, dy := sign*(b[0]-a[0]), b[1]-a[1]
					length := float32(math.Hypot(float64(dx), float64(dy)))
					angle := float32(math.Atan2(float64(dy), float64(dx)))
					h, d := [...]float32{.105, .085, .055}[i], [...]float32{.13, .11, .075}[i]
					x, y := sign*(a[0]+b[0])/2, (a[1]+b[1])/2
					add(x, y, 0, length+.025, h, d, shade(metal, .08))
					parts[len(parts)-1].RotationZ = angle
					add(x, y, d/2+.001, length+.025, h, .002, shade(metal, .70))
					parts[len(parts)-1].RotationZ = angle
				}
				end := points[3]
				add(sign*(end[0]+.012), end[1]-.01, 0, .032, .020, .025, shade(metal, .35))
				parts[len(parts)-1].RotationZ = -sign * .8
			}
		} else {
			width := float32(.30)
			if broken {
				width = .14
			}
			// 宽刃沿柄套左侧斜出，厚度集中在刃背；不能堆成方锤头。
			add(-.045, .740, 0, .11, .100, .105, shade(metal, .31))
			add(-.18, .725, 0, width, .095, .13, shade(metal, .08))
			parts[len(parts)-1].RotationZ = .72
			add(-.18, .725, .071, width, .095, .012, shade(metal, .76))
			parts[len(parts)-1].RotationZ = .72
			tipX := -.18 - width*.38
			tipY := .725 - width*.33
			add(tipX, tipY, 0, .04, .025, .072, shade(metal, .56))
			parts[len(parts)-1].RotationZ = .72
		}
	}
	if category != 1 {
		y := float32(.59)
		if category == 2 {
			y += .04
		}
		if category == 3 {
			y += .045
		}
		if category == 2 {
			add(-.0455, y, 0, .001, .13, .125, shade(metal, .53))
		} else {
			add(0, y, .0635, .09, .13, .002, shade(metal, .53))
		}
	}
	if category == 3 {
		// 锄头上移时同步延长柄到套内，保持连接而不放大整件工具。
		for i := bandCount + 1; i < len(parts); i++ {
			parts[i].Center[1] += .025
		}
	}
	return parts
}

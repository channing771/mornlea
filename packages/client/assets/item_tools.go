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
		handleTop = .70
		bandCount = 8
	}
	bandHeight := (handleTop + .21) / float32(bandCount)
	for i := 0; i < bandCount; i++ {
		y := -.21 + (float32(i)+.5)*bandHeight
		add(0, y, 0, .065, bandHeight, .075, shade(handle, [...]float32{.30, .47, .36, .52, .38, .48, .35, .50}[i]))
	}
	add(0, -.225, 0, .072, .035, .082, shade(metal, .25))
	if category == 1 {
		add(0, .175, 0, .25, .052, .12, shade(metal, .28))
		for _, sign := range []float32{-1, 1} {
			add(sign*.139, .175, 0, .045, .07, .13, shade(metal, .24))
			parts[len(parts)-1].RotationZ = -sign * .18
		}
		add(0, .220, 0, .105, .045, .115, shade(metal, .35))
		length := float32(.535)
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
			add(0, y, 0, width, h, .105, shade(metal, .30))
			add(-.012, y, .058, width*.56, h, .011, shade(metal, .72))
			add(-width/2-.010, y, .019, .022, h, .055, shade(edge, .94))
			add(width/2+.007, y, -.008, .016, h, .07, shade(metal, .45))
		}
		if broken {
			add(-.025, .242+length+.023, 0, .046, .046, .065, shade(metal, .50))
		} else {
			// 斜置实心菱形的下半部接入末段，形成连续斜边而非细小方块帽。
			add(0, .811, 0, .055, .055, .080, shade(metal, .35))
			parts[len(parts)-1].RotationZ = math.Pi / 4
			add(0, .811, .045, .055, .055, .010, shade(metal, .72))
			parts[len(parts)-1].RotationZ = math.Pi / 4
			add(-.019, .830, .045, .011, .055, .014, shade(edge, .94))
			parts[len(parts)-1].RotationZ = -math.Pi / 4
		}
	} else {
		const lift = float32(.33)
		add(0, .37+lift, 0, .115, .13, .125, shade(metal, .32))
		if category == 2 {
			// 三段折线下弯后以两级尖端收束，避免恒厚弧段形成圆钩。
			for _, sign := range []float32{-1, 1} {
				if broken && sign > 0 {
					continue
				}
				for i := 0; i < 5; i++ {
					x := [...]float32{.075, .155, .220, .265, .292}[i]
					y := [...]float32{.421, .397, .347, .293, .251}[i] + lift
					w := [...]float32{.11, .10, .09, .064, .041}[i]
					h := [...]float32{.075, .070, .054, .034, .016}[i]
					d := [...]float32{.12, .105, .082, .052, .022}[i]
					add(sign*x, y, 0, w, h, d, shade(metal, .34))
					parts[len(parts)-1].RotationZ = -sign * ([...]float32{.20, .45, .72, .92, 1.04}[i])
					add(sign*x, y, .5*d+.004, w, h, .008, shade(metal, .78))
					parts[len(parts)-1].RotationZ = -sign * ([...]float32{.20, .45, .72, .92, 1.04}[i])
				}
			}
		} else {
			width := float32(.27)
			if broken {
				width = .14
			}
			// 宽刃沿柄套左侧斜出，厚度集中在刃背；不能堆成方锤头。
			add(-.045, .755, 0, .11, .060, .105, shade(metal, .31))
			add(-.17, .728, 0, width, .065, .13, shade(metal, .34))
			parts[len(parts)-1].RotationZ = .55
			add(-.17, .728, .071, width, .065, .012, shade(metal, .76))
			parts[len(parts)-1].RotationZ = .55
			tipX := -.17 - width*.44
			tipY := .728 - width*.27
			add(tipX, tipY, 0, .04, .025, .072, shade(metal, .56))
			parts[len(parts)-1].RotationZ = .55
		}
	}
	return parts
}

package assets

import "github.com/channing771/mornlea/packages/shared/core"

// ItemToolPart 是以握点为原点的实心体素部件；尺寸与颜色缓存随图标一起刷新。
// 柄、柄套、刃脊和刃缘拥有不同截面，不能退化为图标的均匀挤出。
type ItemToolPart struct {
	Center, Size [3]float32
	Color        [4]float32
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
	parts := make([]ItemToolPart, 0, 14)
	add := func(x, y, z, w, h, d float32, c [4]float32) {
		parts = append(parts, ItemToolPart{[3]float32{x, y, z}, [3]float32{w, h, d}, c})
	}
	add(0, .08, 0, .065, .28, .075, handle)
	add(0, -.065, 0, .085, .04, .09, edge)
	if category == 1 {
		add(0, .20, 0, .24, .055, .10, handle)
		add(0, .245, 0, .105, .05, .115, metal)
		length := float32(.34)
		if broken {
			length = .18
		}
		add(0, .27+length/2, 0, .09, length, .09, metal)
		add(-.055, .27+length/2, 0, .02, length, .04, edge)
		add(.055, .27+length/2, 0, .02, length, .04, edge)
		if broken {
			add(-.022, .27+length+.025, 0, .045, .05, .065, metal)
		} else {
			add(0, .27+length+.025, 0, .065, .05, .065, edge)
			add(0, .27+length+.06, 0, .035, .02, .035, edge)
		}
	} else {
		add(0, .28, 0, .065, .20, .075, handle)
		add(0, .37, 0, .115, .13, .125, metal)
		if category == 2 {
			add(-.09, .405, 0, .18, .085, .10, metal)
			add(-.20, .38, 0, .06, .075, .07, edge)
			add(-.235, .34, 0, .03, .065, .04, edge)
			if !broken {
				add(.10, .405, 0, .20, .085, .10, metal)
				add(.22, .38, 0, .06, .075, .07, edge)
				add(.255, .34, 0, .03, .065, .04, edge)
			}
		} else {
			length := float32(.19)
			if broken {
				length = .085
			}
			add(length/2, .415, 0, length, .075, .10, metal)
			add(length, .365, 0, .075, .10, .085, metal)
			if !broken {
				add(length, .285, 0, .08, .06, .04, edge)
			} else {
				add(length-.015, .30, 0, .045, .03, .04, edge)
			}
		}
	}
	return parts
}

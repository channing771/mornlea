package assets

import (
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// ItemIconLayer 返回透明轮廓物品在世界 atlas 中的同源层。完整立方体物品由
// 掉落渲染继续采样方块材质，因而不占独立物品层；未知物品明确返回 false。
func ItemIconLayer(item core.ItemID) (uint32, bool) {
	switch item {
	case core.ItemCoal:
		return uint32(LayerItemCoal), true
	case core.ItemRawIron:
		return uint32(LayerItemRawIron), true
	case core.ItemIronIngot:
		return uint32(LayerItemIronIngot), true
	case core.ItemStonePickaxe:
		return uint32(LayerItemStonePickaxe), true
	case core.ItemIronPickaxe:
		return uint32(LayerItemIronPickaxe), true
	case core.ItemBrokenStonePickaxe:
		return uint32(LayerItemBrokenStonePickaxe), true
	case core.ItemBrokenIronPickaxe:
		return uint32(LayerItemBrokenIronPickaxe), true
	case core.ItemStoneHoe:
		return uint32(LayerItemStoneHoe), true
	case core.ItemIronHoe:
		return uint32(LayerItemIronHoe), true
	case core.ItemBrokenStoneHoe:
		return uint32(LayerItemBrokenStoneHoe), true
	case core.ItemBrokenIronHoe:
		return uint32(LayerItemBrokenIronHoe), true
	case core.ItemWheatSeeds:
		return uint32(LayerItemWheatSeeds), true
	case core.ItemWheat:
		return uint32(LayerItemWheat), true
	case core.ItemBread:
		return uint32(LayerItemBread), true
	case core.ItemStick:
		return uint32(LayerItemStick), true
	case core.ItemBoneMeal:
		return uint32(LayerItemBoneMeal), true
	case core.ItemPotato:
		return uint32(LayerItemPotato), true
	case core.ItemCarrot:
		return uint32(LayerItemCarrot), true
	case core.ItemPoisonousPotato:
		return uint32(LayerItemPoisonousPotato), true
	case core.ItemDoor:
		return uint32(LayerItemDoor), true
	case core.ItemTorch:
		return uint32(LayerItemTorch), true
	case core.ItemRottenFlesh:
		return uint32(LayerItemRottenFlesh), true
	case core.ItemBed:
		return uint32(LayerItemBed), true
	case core.ItemWoodenSword:
		return uint32(LayerItemWoodenSword), true
	case core.ItemStoneSword:
		return uint32(LayerItemStoneSword), true
	case core.ItemIronSword:
		return uint32(LayerItemIronSword), true
	case core.ItemBrokenWoodenSword:
		return uint32(LayerItemBrokenWoodenSword), true
	case core.ItemBrokenStoneSword:
		return uint32(LayerItemBrokenStoneSword), true
	case core.ItemBrokenIronSword:
		return uint32(LayerItemBrokenIronSword), true
	case core.ItemRawBeef:
		return uint32(LayerRawBeef), true
	case core.ItemCookedBeef:
		return uint32(LayerCookedBeef), true
	default:
		return 0, false
	}
}

// ItemIconRGBA 返回 16×16 RGBA 图标的注册表只读缓存。调用方不得修改返回
// slice；同一注册表的重复查询共享底层字节，未知与空物品返回 false。
func (r *Registry) ItemIconRGBA(item core.ItemID) ([]byte, bool) {
	if item == core.ItemNone || item >= core.ItemIDMax || !core.RegisteredItem(item) {
		return nil, false
	}
	icon := r.itemIcons[item]
	return icon, len(icon) == texSize*texSize*4
}

// refreshItemIcons 在注册表装配或材质包原子替换后重建一次图标目录。轮廓物品
// 直接引用 atlas 层；立方体物品从当前顶面和侧面材质合成，因此用户材质覆盖会
// 同步反映到 UI，而帧循环无需生成或编码像素。
func (r *Registry) refreshItemIcons() {
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if layer, ok := ItemIconLayer(item); ok {
			r.itemIcons[item] = r.layers[int(layer)]
			continue
		}
		block, ok := core.ItemPlacement(item)
		if !ok {
			r.itemIcons[item] = nil
			continue
		}
		top := r.LayerRGBA(int(r.Material(block, mesh.FacePosY)))
		side := r.LayerRGBA(int(r.Material(block, mesh.FacePosX)))
		r.itemIcons[item] = blockItemTexture(top, side)
	}
}

func blockItemTexture(top, side []byte) []byte {
	px := make([]byte, texSize*texSize*4)
	topFace := [][2]int{{8, 1}, {14, 4}, {8, 7}, {2, 4}}
	leftFace := [][2]int{{2, 4}, {8, 7}, {8, 14}, {2, 11}}
	rightFace := [][2]int{{8, 7}, {14, 4}, {14, 11}, {8, 14}}
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			face := 0
			switch {
			case pointInPolygon(x, y, topFace):
				face = 1
			case pointInPolygon(x, y, leftFace):
				face = 2
			case pointInPolygon(x, y, rightFace):
				face = 3
			}
			if face == 0 {
				continue
			}
			if cubeEdge(x, y) {
				paint(px, x, y, rgb{R: 70, G: 54, B: 40})
				continue
			}
			source := side
			shade := uint16(190)
			if face == 1 {
				source, shade = top, 255
			} else if face == 3 {
				shade = 220
			}
			sx, sy := min(15, max(0, x*16/14)), min(15, max(0, y*16/14))
			offset := (sy*texSize + sx) * 4
			paint(px, x, y, rgb{
				R: uint8(uint16(source[offset]) * shade / 255),
				G: uint8(uint16(source[offset+1]) * shade / 255),
				B: uint8(uint16(source[offset+2]) * shade / 255),
			})
		}
	}
	return px
}

func pointInPolygon(x, y int, points [][2]int) bool {
	inside := false
	for i, j := 0, len(points)-1; i < len(points); j, i = i, i+1 {
		xi, yi := points[i][0], points[i][1]
		xj, yj := points[j][0], points[j][1]
		if (yi > y) != (yj > y) && x <= (xj-xi)*(y-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}

func cubeEdge(x, y int) bool {
	for _, segment := range [...][4]int{
		{8, 1, 14, 4}, {14, 4, 14, 11}, {14, 11, 8, 14},
		{8, 14, 2, 11}, {2, 11, 2, 4}, {2, 4, 8, 1},
		{2, 4, 8, 7}, {14, 4, 8, 7}, {8, 7, 8, 14},
	} {
		if pointOnSegment(x, y, segment) {
			return true
		}
	}
	return false
}

func pointOnSegment(x, y int, segment [4]int) bool {
	x0, y0, x1, y1 := segment[0], segment[1], segment[2], segment[3]
	cross := abs((x-x0)*(y1-y0) - (y-y0)*(x1-x0))
	return cross <= max(abs(x1-x0), abs(y1-y0))/2 &&
		x >= min(x0, x1) && x <= max(x0, x1) && y >= min(y0, y1) && y <= max(y0, y1)
}

type itemPalette struct {
	edge, base, light, accent rgb
}

type itemMask struct {
	shape  [texSize * texSize]bool
	accent [texSize * texSize]bool
}

func (m *itemMask) set(x, y int) {
	if x >= 0 && x < texSize && y >= 0 && y < texSize {
		m.shape[y*texSize+x] = true
	}
}

func (m *itemMask) setAccent(x, y int) {
	m.set(x, y)
	if x >= 0 && x < texSize && y >= 0 && y < texSize {
		m.accent[y*texSize+x] = true
	}
}

func (m *itemMask) line(x0, y0, x1, y1, radius int, accent bool) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		for oy := -radius; oy <= radius; oy++ {
			for ox := -radius; ox <= radius; ox++ {
				if accent {
					m.setAccent(x0+ox, y0+oy)
				} else {
					m.set(x0+ox, y0+oy)
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func originalItemTexture(item core.ItemID) []byte {
	m := itemMask{}
	palette := paletteForItem(item)
	switch item {
	case core.ItemCoal, core.ItemRawIron:
		for y, row := range [...]string{"..####..", ".######.", "########", "########", "########", ".#######", ".######.", "..####.."} {
			for x, cell := range row {
				if cell == '#' {
					m.set(x+4, y+4)
				}
			}
		}
		m.line(7, 6, 9, 6, 0, true)
	case core.ItemIronIngot:
		for y := 6; y <= 10; y++ {
			inset := abs(8-y) / 2
			for x := 3 + inset; x <= 12-inset; x++ {
				m.set(x, y)
			}
		}
		m.line(5, 7, 10, 7, 0, true)
	case core.ItemStonePickaxe, core.ItemIronPickaxe, core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe:
		broken := item == core.ItemBrokenStonePickaxe || item == core.ItemBrokenIronPickaxe
		m.line(4, 13, 10, 6, 1, false)
		if broken {
			m.line(3, 5, 8, 5, 1, true)
		} else {
			m.line(3, 5, 13, 5, 1, true)
			m.setAccent(2, 6)
			m.setAccent(14, 6)
		}
	case core.ItemStoneHoe, core.ItemIronHoe, core.ItemBrokenStoneHoe, core.ItemBrokenIronHoe:
		broken := item == core.ItemBrokenStoneHoe || item == core.ItemBrokenIronHoe
		m.line(4, 13, 10, 5, 1, false)
		if broken {
			m.line(9, 4, 11, 4, 1, true)
		} else {
			m.line(9, 4, 13, 4, 1, true)
			m.line(13, 4, 13, 8, 1, true)
		}
	case core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword,
		core.ItemBrokenWoodenSword, core.ItemBrokenStoneSword, core.ItemBrokenIronSword:
		broken := item == core.ItemBrokenWoodenSword || item == core.ItemBrokenStoneSword || item == core.ItemBrokenIronSword
		m.line(4, 13, 7, 10, 1, false)
		m.line(4, 9, 9, 14, 0, false)
		if broken {
			m.line(8, 9, 10, 7, 1, true)
		} else {
			m.line(8, 9, 13, 4, 1, true)
			m.setAccent(13, 3)
		}
	case core.ItemWheatSeeds:
		for _, p := range [...][2]int{{4, 10}, {7, 6}, {9, 11}, {12, 7}} {
			m.line(p[0], p[1], p[0]+2, p[1]+2, 1, false)
		}
	case core.ItemWheat:
		m.line(8, 14, 8, 2, 0, false)
		for _, grain := range [...]struct{ x, y int }{
			{3, 2}, {9, 4}, {3, 6}, {9, 8}, {3, 10},
		} {
			m.line(8, grain.y+2, grain.x+2, grain.y+1, 0, false)
			for y := grain.y; y < grain.y+3; y++ {
				for x := grain.x; x < grain.x+4; x++ {
					m.set(x, y)
				}
			}
			m.setAccent(grain.x+1, grain.y+1)
			m.setAccent(grain.x+2, grain.y+1)
		}
	case core.ItemBread:
		for y := 5; y <= 11; y++ {
			for x := 3; x <= 13; x++ {
				if (x-8)*(x-8)*3+(y-8)*(y-8)*7 <= 80 {
					m.set(x, y)
				}
			}
		}
		m.line(6, 6, 7, 8, 0, true)
		m.line(9, 5, 10, 7, 0, true)
	case core.ItemStick:
		m.line(4, 13, 12, 3, 1, false)
	case core.ItemBoneMeal:
		for y := 7; y <= 12; y++ {
			for x := 4; x <= 12; x++ {
				if (x-8)*(x-8)+(y-10)*(y-10) < 20 {
					m.set(x, y)
				}
			}
		}
		m.line(6, 7, 8, 4, 0, true)
		m.line(10, 7, 12, 5, 0, true)
	case core.ItemPotato, core.ItemPoisonousPotato:
		for y := 5; y <= 12; y++ {
			for x := 4; x <= 12; x++ {
				if (x-8)*(x-8)*4+(y-8)*(y-8)*3 <= 54 {
					m.set(x, y)
				}
			}
		}
		for _, p := range [...][2]int{{6, 8}, {10, 6}, {9, 11}} {
			m.setAccent(p[0], p[1])
		}
	case core.ItemCarrot:
		m.line(7, 5, 10, 12, 2, false)
		m.set(9, 13)
		m.line(7, 5, 4, 2, 0, true)
		m.line(8, 5, 9, 1, 0, true)
		m.line(8, 5, 12, 3, 0, true)
	case core.ItemDoor:
		for y := 1; y <= 14; y++ {
			for x := 3; x <= 12; x++ {
				m.set(x, y)
			}
		}
		for y := 3; y <= 6; y++ {
			for x := 5; x <= 10; x++ {
				m.setAccent(x, y)
			}
		}
		for x := 4; x <= 11; x++ {
			m.shape[8*texSize+x] = false
		}
		m.set(11, 10)
	case core.ItemTorch:
		m.line(7, 14, 8, 6, 1, false)
		m.line(6, 5, 8, 2, 1, true)
		m.setAccent(9, 4)
	case core.ItemRottenFlesh:
		for y := 5; y <= 12; y++ {
			for x := 4; x <= 12; x++ {
				if (x+y)%7 != 0 {
					m.set(x, y)
				}
			}
		}
		m.line(6, 7, 10, 10, 0, true)
	case core.ItemBed:
		for y := 5; y <= 11; y++ {
			for x := 1; x <= 14; x++ {
				m.set(x, y)
			}
		}
		m.line(2, 11, 2, 14, 1, false)
		m.line(13, 11, 13, 14, 1, false)
		for y := 6; y <= 9; y++ {
			for x := 2; x <= 6; x++ {
				m.setAccent(x, y)
			}
		}
	default:
		m.line(4, 11, 11, 4, 2, false)
	}
	return renderItemMask(m, palette, uint32(item))
}

func renderItemMask(mask itemMask, palette itemPalette, salt uint32) []byte {
	px := make([]byte, texSize*texSize*4)
	for y := 0; y < texSize; y++ {
		for x := 0; x < texSize; x++ {
			index := y*texSize + x
			if !mask.shape[index] {
				continue
			}
			color := palette.base
			edge := false
			for _, d := range [...][2]int{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || nx >= texSize || ny < 0 || ny >= texSize || !mask.shape[ny*texSize+nx] {
					edge = true
					break
				}
			}
			if edge {
				color = palette.edge
			} else if mask.accent[index] {
				color = palette.accent
			} else if hash2(uint32(x), uint32(y), salt)%5 == 0 {
				color = palette.light
			}
			paint(px, x, y, color)
		}
	}
	return px
}

func paletteForItem(item core.ItemID) itemPalette {
	wood := itemPalette{rgb{69, 48, 30}, rgb{151, 101, 55}, rgb{203, 151, 83}, rgb{238, 196, 92}}
	stone := itemPalette{rgb{58, 52, 48}, rgb{151, 101, 55}, rgb{203, 151, 83}, rgb{172, 171, 162}}
	iron := itemPalette{rgb{71, 67, 66}, rgb{151, 101, 55}, rgb{203, 151, 83}, rgb{230, 226, 213}}
	switch item {
	case core.ItemCoal:
		return itemPalette{rgb{35, 30, 31}, rgb{60, 55, 58}, rgb{103, 94, 96}, rgb{151, 157, 163}}
	case core.ItemRawIron:
		return itemPalette{rgb{83, 56, 45}, rgb{179, 119, 84}, rgb{229, 171, 123}, rgb{246, 217, 168}}
	case core.ItemIronIngot:
		return itemPalette{rgb{71, 67, 66}, rgb{171, 177, 180}, rgb{230, 226, 213}, rgb{248, 246, 235}}
	case core.ItemIronPickaxe, core.ItemIronHoe, core.ItemIronSword:
		return iron
	case core.ItemBrokenIronPickaxe, core.ItemBrokenIronHoe, core.ItemBrokenIronSword:
		return iron
	case core.ItemStonePickaxe, core.ItemStoneHoe, core.ItemStoneSword:
		return stone
	case core.ItemBrokenStonePickaxe, core.ItemBrokenStoneHoe, core.ItemBrokenStoneSword:
		return stone
	case core.ItemWoodenSword:
		return itemPalette{wood.edge, wood.base, wood.light, rgb{219, 163, 89}}
	case core.ItemBrokenWoodenSword:
		return itemPalette{wood.edge, wood.base, wood.light, rgb{219, 163, 89}}
	case core.ItemWheatSeeds:
		return itemPalette{rgb{61, 72, 34}, rgb{112, 132, 52}, rgb{160, 174, 75}, rgb{218, 189, 87}}
	case core.ItemWheat:
		return itemPalette{rgb{105, 71, 31}, rgb{203, 151, 55}, rgb{239, 202, 99}, rgb{245, 211, 112}}
	case core.ItemBread:
		return itemPalette{rgb{96, 55, 31}, rgb{190, 116, 50}, rgb{236, 175, 87}, rgb{255, 215, 132}}
	case core.ItemStick:
		return wood
	case core.ItemDoor:
		return itemPalette{rgb{69, 48, 30}, rgb{151, 101, 55}, rgb{203, 151, 83}, rgb{166, 210, 210}}
	case core.ItemBoneMeal:
		return itemPalette{rgb{102, 92, 78}, rgb{208, 198, 176}, rgb{247, 239, 218}, rgb{153, 178, 131}}
	case core.ItemPotato:
		return itemPalette{rgb{91, 62, 34}, rgb{174, 123, 62}, rgb{219, 169, 89}, rgb{119, 83, 42}}
	case core.ItemPoisonousPotato:
		return itemPalette{rgb{67, 63, 30}, rgb{145, 127, 53}, rgb{197, 177, 74}, rgb{77, 139, 54}}
	case core.ItemCarrot:
		return itemPalette{rgb{105, 55, 24}, rgb{218, 108, 32}, rgb{247, 155, 54}, rgb{73, 137, 50}}
	case core.ItemTorch:
		return itemPalette{rgb{77, 47, 28}, rgb{139, 91, 47}, rgb{201, 145, 70}, rgb{255, 194, 58}}
	case core.ItemRottenFlesh:
		return itemPalette{rgb{76, 51, 34}, rgb{126, 91, 52}, rgb{164, 126, 70}, rgb{100, 128, 59}}
	case core.ItemBed:
		return itemPalette{rgb{72, 49, 34}, rgb{177, 99, 82}, rgb{221, 144, 119}, rgb{244, 225, 193}}
	default:
		return wood
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

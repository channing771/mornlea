package assets

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/channing771/mornlea/packages/shared/core"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func TestItemIconsCoverRegistryAndKeepTransparentOutlines(t *testing.T) {
	registry := NewRegistry()
	seen := make(map[uint32]core.ItemID)
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if !core.RegisteredItem(item) {
			t.Fatalf("物品 %d 位于枚举范围内却未注册", item)
		}
		icon, ok := registry.ItemIconRGBA(item)
		if !ok {
			t.Fatalf("已注册物品 %d 缺少图标", item)
		}
		if len(icon) != texSize*texSize*4 {
			t.Fatalf("物品 %d 图标字节=%d，想要 %d", item, len(icon), texSize*texSize*4)
		}
		opaque, transparent := 0, 0
		for offset := 3; offset < len(icon); offset += 4 {
			switch icon[offset] {
			case 0:
				transparent++
			case 255:
				opaque++
			default:
				t.Fatalf("物品 %d 图标含半透明 alpha=%d", item, icon[offset])
			}
		}
		if opaque == 0 {
			t.Fatalf("物品 %d 图标全透明", item)
		}
		if layer, original := ItemIconLayer(item); original {
			if transparent == 0 {
				t.Fatalf("轮廓物品 %d 没有透明空隙", item)
			}
			if int(layer) >= registry.LayerCount() {
				t.Fatalf("物品 %d 图层=%d，越出 atlas %d", item, layer, registry.LayerCount())
			}
			if !bytes.Equal(icon, registry.LayerRGBA(int(layer))) {
				t.Fatalf("物品 %d 的 UI 图标与世界 atlas 层不同源", item)
			}
			if prior, duplicate := seen[layer]; duplicate {
				t.Fatalf("轮廓物品 %d 与 %d 复用图层 %d", item, prior, layer)
			}
			seen[layer] = item
		}
	}
	if _, ok := registry.ItemIconRGBA(core.ItemNone); ok {
		t.Fatal("空物品取得图标")
	}
	if _, ok := registry.ItemIconRGBA(core.ItemIDMax); ok {
		t.Fatal("未知物品取得图标")
	}
}

func TestTexturePackRefreshesBlockItemIconCache(t *testing.T) {
	registry := NewRegistry()
	before, _ := registry.ItemIconRGBA(core.ItemStone)
	before = bytes.Clone(before)
	_, stone := solidPNG(t, 16, 16, color.NRGBA{R: 24, G: 180, B: 210, A: 255})
	root := fstest.MapFS{
		"pack.json":          {Data: manifest(t, "图标刷新测试")},
		"textures/stone.png": {Data: stone},
	}
	if err := applyPack(registry, root); err != nil {
		t.Fatal(err)
	}
	first, ok := registry.ItemIconRGBA(core.ItemStone)
	if !ok || bytes.Equal(first, before) {
		t.Fatal("材质包替换后方块物品图标缓存未刷新")
	}
	second, _ := registry.ItemIconRGBA(core.ItemStone)
	if &first[0] != &second[0] {
		t.Fatal("刷新后的重复查询未共享缓存")
	}
}

func TestItemIconMaterialsAndBrokenFormsStayDistinct(t *testing.T) {
	registry := NewRegistry()
	pairs := [][2]core.ItemID{
		{core.ItemStone, core.ItemDirt},
		{core.ItemDirt, core.ItemGrass},
		{core.ItemStone, core.ItemStoneBrick},
		{core.ItemStonePickaxe, core.ItemIronPickaxe},
		{core.ItemStonePickaxe, core.ItemBrokenStonePickaxe},
		{core.ItemIronPickaxe, core.ItemBrokenIronPickaxe},
		{core.ItemStoneHoe, core.ItemIronHoe},
		{core.ItemStoneHoe, core.ItemBrokenStoneHoe},
		{core.ItemIronHoe, core.ItemBrokenIronHoe},
		{core.ItemWoodenSword, core.ItemStoneSword},
		{core.ItemStoneSword, core.ItemIronSword},
		{core.ItemWoodenSword, core.ItemBrokenWoodenSword},
		{core.ItemStoneSword, core.ItemBrokenStoneSword},
		{core.ItemIronSword, core.ItemBrokenIronSword},
		{core.ItemPotato, core.ItemPoisonousPotato},
		{core.ItemRawBeef, core.ItemCookedBeef},
	}
	for _, pair := range pairs {
		left, _ := registry.ItemIconRGBA(pair[0])
		right, _ := registry.ItemIconRGBA(pair[1])
		if bytes.Equal(left, right) {
			t.Fatalf("物品 %d 与 %d 图标相同", pair[0], pair[1])
		}
	}
	first, _ := registry.ItemIconRGBA(core.ItemIronIngot)
	second, _ := registry.ItemIconRGBA(core.ItemIronIngot)
	if len(first) == 0 || &first[0] != &second[0] {
		t.Fatal("重复查询没有返回同一份只读缓存")
	}
}

func TestItemIconContactSheet(t *testing.T) {
	path := os.Getenv("MORNLEA_ITEM_ICON_SHEET")
	if path == "" {
		t.Skip("未请求图标 contact sheet")
	}
	registry := NewDefaultRegistry()
	const scale, columns, cellWidth, cellHeight = 7, 6, 180, 162
	rows := (int(core.ItemIDMax) - 1 + columns - 1) / columns
	sheet := image.NewRGBA(image.Rect(0, 0, columns*cellWidth, rows*cellHeight))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{R: 222, G: 205, B: 169, A: 255}), image.Point{}, draw.Src)
	fontBytes, err := os.ReadFile("/System/Library/Fonts/Hiragino Sans GB.ttc")
	if err != nil {
		t.Fatal(err)
	}
	collection, err := opentype.ParseCollection(fontBytes)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := collection.Font(0)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 14, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	label := font.Drawer{Dst: sheet, Src: image.NewUniform(color.RGBA{R: 61, G: 46, B: 32, A: 255}), Face: face}
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		icon, ok := registry.ItemIconRGBA(item)
		if !ok {
			t.Fatalf("物品 %d 缺少图标", item)
		}
		cell := int(item) - 1
		cellX, cellY := cell%columns*cellWidth, cell/columns*cellHeight
		for y := cellY + 4; y < cellY+cellHeight-4; y++ {
			for x := cellX + 4; x < cellX+cellWidth-4; x++ {
				sheet.SetRGBA(x, y, color.RGBA{R: 251, G: 245, B: 228, A: 255})
			}
		}
		ox, oy := cellX+(cellWidth-texSize*scale)/2, cellY+6
		for y := 0; y < texSize; y++ {
			for x := 0; x < texSize; x++ {
				offset := (y*texSize + x) * 4
				sample := icon[offset : offset+4]
				if sample[3] == 0 {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						sheet.SetRGBA(ox+x*scale+dx, oy+y*scale+dy, color.RGBA{R: sample[0], G: sample[1], B: sample[2], A: sample[3]})
					}
				}
			}
		}
		name, _ := core.ItemDisplayName(item)
		label.Dot = fixed.P(cellX+12, cellY+132)
		label.DrawString("#" + strconv.Itoa(int(item)))
		label.Dot = fixed.P(cellX+12, cellY+151)
		label.DrawString(name)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, sheet); err != nil {
		t.Fatal(err)
	}
}

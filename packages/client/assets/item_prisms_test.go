package assets

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestItemIconPrismsPreservePixelsAndRefresh(t *testing.T) {
	r := NewRegistry()
	layer, _ := ItemIconLayer(core.ItemIronSword)
	px := make([]byte, 16*16*4)
	copy(px[0:16], []byte{12, 34, 56, 127, 23, 45, 67, 128, 23, 45, 67, 255, 98, 76, 54, 255})
	r.layers[layer] = px
	r.refreshItemIcons()
	parts, ok := r.ItemIconPrisms(core.ItemIronSword)
	if !ok || len(parts) != 2 {
		t.Fatalf("alpha 阈值或同行合并错误: %v", parts)
	}
	if parts[0].X != 1 || parts[0].Y != 0 || parts[0].Width != 2 || parts[0].Color != [4]float32{23.0 / 255, 45.0 / 255, 67.0 / 255, 1} {
		t.Fatalf("像素位置/颜色未保留: %+v", parts[0])
	}
	old := parts[0]
	replacement := make([]byte, 16*16*4)
	copy(replacement[0:4], []byte{220, 30, 10, 255})
	r.layers[layer] = replacement
	r.refreshItemIcons()
	updated, _ := r.ItemIconPrisms(core.ItemIronSword)
	if len(updated) != 1 || updated[0].X != 0 || updated[0].Color[0] != 220.0/255 || parts[0] != old {
		t.Fatal("刷新未替换缓存或修改了旧只读快照")
	}
}

func TestItemIconPrismsCoverDefaultRegistryWithinBudget(t *testing.T) {
	for _, r := range []*Registry{NewRegistry(), NewDefaultRegistry()} {
		for item := core.ItemID(1); item < core.ItemIDMax; item++ {
			if _, ok := ItemIconLayer(item); !ok {
				continue
			}
			px, _ := r.ItemIconRGBA(item)
			parts, ok := r.ItemIconPrisms(item)
			if !ok || len(parts) == 0 || len(parts) > 256 {
				t.Fatalf("item %d prism count=%d", item, len(parts))
			}
			var covered [256]bool
			for _, p := range parts {
				for x := p.X; x < p.X+p.Width; x++ {
					i := int(p.Y)*16 + int(x)
					if covered[i] || px[i*4+3] < 128 {
						t.Fatalf("item %d 多余/重叠像素 %d", item, i)
					}
					covered[i] = true
					for c := range 3 {
						if p.Color[c] != float32(px[i*4+c])/255 {
							t.Fatalf("item %d 颜色不一致", item)
						}
					}
				}
			}
			for i := range 256 {
				if covered[i] != (px[i*4+3] >= 128) {
					t.Fatalf("item %d 丢失轮廓像素 %d", item, i)
				}
			}
			again, _ := r.ItemIconPrisms(item)
			if &again[0] != &parts[0] {
				t.Fatal("重复读取重新生成缓存")
			}
		}
	}
}

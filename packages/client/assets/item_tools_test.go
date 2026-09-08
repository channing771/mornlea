package assets

import (
	"github.com/channing771/mornlea/packages/shared/core"
	"testing"
)

func TestSolidToolsHaveIndependentCrossSectionsAndRefresh(t *testing.T) {
	r := NewDefaultRegistry()
	for _, item := range []core.ItemID{core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword, core.ItemBrokenWoodenSword, core.ItemBrokenStoneSword, core.ItemBrokenIronSword, core.ItemStonePickaxe, core.ItemIronPickaxe, core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe, core.ItemStoneHoe, core.ItemIronHoe, core.ItemBrokenStoneHoe, core.ItemBrokenIronHoe} {
		parts, ok := r.ItemToolParts(item)
		if !ok || len(parts) < 5 || len(parts) > 256 {
			t.Fatalf("item %d solid parts=%d", item, len(parts))
		}
		depths := map[float32]bool{}
		for _, p := range parts {
			depths[p.Size[2]] = true
			if p.Size[2] <= 0 {
				t.Fatal("empty section")
			}
		}
		if len(depths) < 3 {
			t.Fatalf("item %d lacks handle, socket and edge depth", item)
		}
		old := parts[0].Color
		layer, _ := ItemIconLayer(item)
		px := append([]byte(nil), r.layers[layer]...)
		for i := 0; i < len(px); i += 4 {
			if px[i+3] >= 128 {
				px[i], px[i+1], px[i+2] = 17, 29, 43
			}
		}
		r.layers[layer] = px
		r.refreshItemIcons()
		updated, _ := r.ItemToolParts(item)
		if updated[0].Color == old || updated[0].Color != ([4]float32{17.0 / 255, 29.0 / 255, 43.0 / 255, 1}) {
			t.Fatalf("item %d stale palette %v", item, updated[0].Color)
		}
	}
	if _, ok := r.ItemToolParts(core.ItemBread); ok {
		t.Fatal("food must retain icon")
	}
}

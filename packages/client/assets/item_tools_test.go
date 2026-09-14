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

func TestApprovedToolsHaveWoodBandsAndMetalGuard(t *testing.T) {
	r := NewDefaultRegistry()
	for _, item := range []core.ItemID{core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe} {
		parts, _ := r.ItemToolParts(item)
		bands := map[[4]float32]bool{}
		for _, p := range parts {
			if p.Center[1] > .08 && p.Center[1] < .3 && p.Size[0] < .08 {
				bands[p.Color] = true
			}
		}
		if len(bands) < 3 {
			t.Errorf("tool %d lacks woodgrain bands: %d", item, len(bands))
		}
		if item == core.ItemIronSword {
			for _, p := range parts {
				if p.Size[0] > .2 && p.Center[1] < .3 && p.Color == parts[0].Color {
					t.Error("sword guard is wood handle color")
				}
			}
		}
	}
}

func TestApprovedPickCurveAndBroadHoeBlade(t *testing.T) {
	r := NewDefaultRegistry()
	pick, _ := r.ItemToolParts(core.ItemIronPickaxe)
	lowTips := 0
	for _, p := range pick {
		if (p.Center[0] < -.2 || p.Center[0] > .2) && p.Center[1] < .3 && p.Size[2] >= .07 {
			lowTips++
		}
	}
	if lowTips < 2 {
		t.Fatalf("pick has %d solid lowered arm tips", lowTips)
	}
	hoe, _ := r.ItemToolParts(core.ItemIronHoe)
	broad := false
	for _, p := range hoe {
		if p.Center[0] < -.15 && p.Center[1] < .37 && p.Size[0] >= .15 && p.Size[1] >= .12 && p.Size[2] >= .1 {
			broad = true
		}
	}
	if !broad {
		t.Fatal("hoe lacks broad offset solid blade")
	}
}

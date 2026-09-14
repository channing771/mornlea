package assets

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
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
		if updated[0].Color == old {
			t.Fatalf("item %d stale handle palette", item)
		}
		for _, p := range updated {
			// 单色覆盖的各个可见分面仅改变明暗，所有通道必须来自新素材。
			red, green, blue := p.Color[0]/17, p.Color[1]/29, p.Color[2]/43
			if absToolColor(red-green) > 1e-6 || absToolColor(red-blue) > 1e-6 {
				t.Fatalf("item %d stale visible facet palette %v", item, p.Color)
			}
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
			if p.Center[1] > -.21 && p.Center[1] < .70 && p.Size[0] < .08 && p.Size[1] < .2 {
				bands[p.Color] = true
			}
		}
		if len(bands) < 3 {
			t.Errorf("tool %d lacks woodgrain bands: %d", item, len(bands))
		}
		if item == core.ItemIronSword {
			guardFound := false
			for _, p := range parts {
				if p.Size[0] > .2 && p.Center[1] < .3 {
					guardFound = true
					if p.Color == parts[0].Color {
						t.Error("sword guard is wood handle color")
					}
				}
			}
			if !guardFound {
				t.Error("sword lacks a wide metal guard")
			}
		}
	}
}

func TestApprovedPickArmsDescendFromSocket(t *testing.T) {
	pick, _ := NewDefaultRegistry().ItemToolParts(core.ItemIronPickaxe)
	socketY := float32(0)
	for _, p := range pick {
		if p.Center[0] == 0 && p.Size[0] > .1 {
			socketY = max(socketY, p.Center[1])
		}
	}
	for _, sign := range []float32{-1, 1} {
		lowered := false
		for _, p := range pick {
			if p.Center[0]*sign > .2 && p.Center[1] < socketY-.10 && p.Size[2] > .01 {
				lowered = true
			}
		}
		if !lowered {
			t.Errorf("pick side %v lacks a solid descending tip", sign)
		}
	}
}

func absToolColor(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestApprovedToolHandleHasBroadDarkBands(t *testing.T) {
	parts, _ := NewDefaultRegistry().ItemToolParts(core.ItemIronPickaxe)
	broad := 0
	for _, p := range parts {
		if p.Center[1] >= -.21 && p.Center[1] <= .70 && p.Size[0] <= .08 && p.Size[1] < .2 {
			if p.Size[1] >= .055 {
				if p.Color[0]*.2126+p.Color[1]*.7152+p.Color[2]*.0722 > .35 || p.Color[0] <= p.Color[1] || p.Color[1] <= p.Color[2] {
					t.Errorf("handle band is not dark warm wood: %v", p.Color)
				}
				broad++
			}
		}
	}
	if broad < 5 || broad > 8 {
		t.Fatalf("broad handle bands=%d, want about six instead of ladder strips", broad)
	}
}

func TestApprovedPickEndsTaperToPoints(t *testing.T) {
	parts, _ := NewDefaultRegistry().ItemToolParts(core.ItemIronPickaxe)
	for _, sign := range []float32{-1, 1} {
		var end ItemToolPart
		for _, p := range parts {
			if p.Center[0]*sign > end.Center[0]*sign {
				end = p
			}
		}
		if end.Size[1] > .025 || end.Size[2] > .035 {
			t.Errorf("pick end remains blunt: %v", end.Size)
		}
	}
}

func TestApprovedHoeIsAngledSlabAndPickFrontIsLight(t *testing.T) {
	r := NewDefaultRegistry()
	hoe, _ := r.ItemToolParts(core.ItemIronHoe)
	slab := false
	for _, p := range hoe {
		if p.Center[0] < -.1 && p.Size[0] >= .20 && p.Size[1] <= .08 && p.Size[2] >= .07 && p.RotationZ > .25 {
			slab = true
		}
	}
	if !slab {
		t.Error("hoe head lacks the thin broad angled blade from reference")
	}
	pick, _ := r.ItemToolParts(core.ItemIronPickaxe)
	for _, front := range pick {
		if front.Center[2] > .05 && front.Center[1] > .5 {
			for _, body := range pick {
				if body.Center[2] == 0 && body.Center[0] == front.Center[0] && body.Center[1] == front.Center[1] {
					if front.Color[0]+front.Color[1]+front.Color[2] < 1.5*(body.Color[0]+body.Color[1]+body.Color[2]) {
						t.Error("pick front is darker than broad side")
					}
				}
			}
		}
	}
}

func TestApprovedSwordHasAngledPointAndUpperTaper(t *testing.T) {
	parts, _ := NewDefaultRegistry().ItemToolParts(core.ItemIronSword)
	highest, tipRotation := float32(0), float32(0)
	lowerWidth, upperWidth := float32(0), float32(0)
	for _, p := range parts {
		upper := p.Center[1] + float32(math.Abs(math.Sin(float64(p.RotationZ))))*p.Size[0]/2 + float32(math.Abs(math.Cos(float64(p.RotationZ))))*p.Size[1]/2
		if upper > highest {
			highest, tipRotation = upper, p.RotationZ
		}
		if p.Size[2] >= .1 && p.Center[1] > .28 && p.Center[1] < .36 {
			lowerWidth = max(lowerWidth, p.Size[0])
		}
		if p.Size[2] >= .1 && p.Center[1] > .7 {
			upperWidth = max(upperWidth, p.Size[0])
		}
	}
	if absToolColor(tipRotation) < .5 {
		t.Error("sword point is an axis-aligned stack instead of an angled point")
	}
	if lowerWidth == 0 || upperWidth == 0 || upperWidth > lowerWidth*.7 {
		t.Errorf("upper blade lacks progressive taper: %v -> %v", lowerWidth, upperWidth)
	}
}

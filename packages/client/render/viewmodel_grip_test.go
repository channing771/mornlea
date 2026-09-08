package render

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
)

func TestViewmodelSingleHandAndIconClassification(t *testing.T) {
	out := (&ViewmodelEncoder{}).EncodeViewmodelInstances(nil, &ViewmodelInput{})
	if len(out) != 96 {
		t.Fatalf("idle hand count=%d, want one", len(out)/96)
	}
	for _, item := range []core.ItemID{core.ItemTorch, core.ItemDoor, core.ItemBed, core.ItemWheatSeeds} {
		if ViewmodelHeldKindOf(core.ItemStack{Item: item, Count: 1}) != ViewmodelHeldItem {
			t.Errorf("item %d must use icon before placement", item)
		}
	}
}

func TestViewmodelBlockFacesMatchWorld(t *testing.T) {
	r := assets.NewDefaultRegistry()
	for _, item := range []core.ItemID{core.ItemGrass, core.ItemOakLog, core.ItemWorkbench} {
		parts := buildViewmodelParts(nil, &ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}}, 0)
		if len(parts) != 7 {
			t.Fatalf("item %d: parts=%d, want hand plus six faces", item, len(parts))
		}
		block, _ := core.ItemPlacement(item)
		for i, face := range []mesh.Face{mesh.FacePosX, mesh.FaceNegX, mesh.FacePosY, mesh.FaceNegY, mesh.FacePosZ, mesh.FaceNegZ} {
			if parts[i+1].material != uint32(r.Material(block, face)) {
				t.Errorf("item %d face %d wrong material", item, face)
			}
		}
	}
}

func TestViewmodelHeldGripRigidDuringSwing(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemStone, core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe} {
		input := &ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}}
		neutral := buildViewmodelParts(nil, input, 0)
		for _, angle := range []float32{-.7, -.3, .3, .7} {
			moved := buildViewmodelParts(nil, input, angle)
			for i := 1; i < len(neutral); i++ {
				want := neutral[0].transform.Inv().Mul4(neutral[i].transform)
				got := moved[0].transform.Inv().Mul4(moved[i].transform)
				for j := range 16 {
					if !approxEqual(want[j], got[j]) {
						t.Fatalf("item %d part %d detached at %f", item, i, angle)
					}
				}
			}
		}
	}
}

func TestViewmodelAllIconsVisibleAcrossCompleteSwings(t *testing.T) {
	registry := assets.NewDefaultRegistry()
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if _, ok := assets.ItemIconLayer(item); !ok {
			continue
		}
		stack := core.ItemStack{Item: item, Count: 1}
		_, period := ViewmodelSwingParams(ViewmodelTierOf(stack))
		var angles []float32
		for tick := uint64(0); tick <= period; tick++ {
			angles = append(angles, ViewmodelMiningAngle(tick, 0, ViewmodelTierOf(stack)))
		}
		for age := uint8(0); age <= ViewmodelAttackFrames; age++ {
			angles = append(angles, ViewmodelAttackAngle(age, ViewmodelTierOf(stack)))
		}
		for _, aspect := range []float32{16.0 / 9, 4.0 / 3} {
			projection := core.Perspective(viewmodelProjectionFovY, aspect, .1, 100)
			for _, angle := range angles {
				parts := buildViewmodelParts(nil, &ViewmodelInput{Selected: stack, Registry: registry}, angle)
				out := make([]byte, len(parts)*avatarInstanceBytes)
				encodeAvatarPartsInto(out, parts)
				hand := viewmodelInstanceScreenHull(t, out, 0, projection)
				visible, total := 0, 0
				prisms, _ := registry.ItemIconPrisms(item)
				for i, p := range prisms {
					center, w := projectToNDC(projection, decodedPartCenter(out, i+1))
					if w <= 0 || abs32(center[0]) > .98 || abs32(center[1]) > .98 {
						t.Fatalf("item %d angle %.3f aspect %.3f: icon outside screen %v", item, angle, aspect, center)
					}
					hull := viewmodelInstanceScreenHull(t, out, i+1, projection)
					if viewmodelHullCoversPoint(hull, mgl32.Vec2{}) {
						t.Fatalf("item %d angle %.3f: crosshair covered", item, angle)
					}
					total += int(p.Width)
					if !viewmodelHullCoversPoint(hand, mgl32.Vec2{center[0], center[1]}) {
						visible += int(p.Width)
					}
				}
				if visible*2 < total {
					t.Fatalf("item %d angle %.3f: visible pixels %d/%d", item, angle, visible, total)
				}
			}
		}
	}
}

func TestViewmodelIconsKeepDistinctGeometryAndZeroAlloc(t *testing.T) {
	encoder := &ViewmodelEncoder{}
	dst := make([]byte, 0, 257*96)
	seen := map[string]core.ItemID{}
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		if _, ok := assets.ItemIconLayer(item); !ok {
			continue
		}
		input := &ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}}
		out := encoder.EncodeViewmodelInstances(dst, input)
		if len(out) > 257*96 || len(out) <= 96 {
			t.Fatalf("item %d budget %d", item, len(out)/96)
		}
		key := string(out[96:])
		if prior, ok := seen[key]; ok {
			t.Fatalf("item %d indistinguishable from %d", item, prior)
		}
		seen[key] = item
		if allocs := testing.AllocsPerRun(10, func() { encoder.EncodeViewmodelInstances(dst, input) }); allocs != 0 {
			t.Fatalf("item %d allocs %f", item, allocs)
		}
	}
}

func TestViewmodelEveryItemTouchesMainHand(t *testing.T) {
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		parts := buildViewmodelParts(nil, &ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}}, 0)
		inverse := parts[0].transform.Inv()
		touches := false
		for _, part := range parts[1:] {
			local := inverse.Mul4(part.transform)
			lo, hi := mgl32.Vec3{100, 100, 100}, mgl32.Vec3{-100, -100, -100}
			for _, x := range []float32{-.5, .5} {
				for _, y := range []float32{-.5, .5} {
					for _, z := range []float32{-.5, .5} {
						p := local.Mul4x1(mgl32.Vec4{x, y, z, 1})
						for axis := range 3 {
							lo[axis] = min(lo[axis], p[axis])
							hi[axis] = max(hi[axis], p[axis])
						}
					}
				}
			}
			if lo[0] <= .5 && hi[0] >= -.5 && lo[1] <= .5 && hi[1] >= -.5 && lo[2] <= .5 && hi[2] >= -.5 {
				touches = true
			}
		}
		if !touches {
			t.Errorf("item %d floats clear of fist", item)
		}
	}
}

func TestViewmodelBlockFacesStayOnScreenAcrossCompleteSwings(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemGrass, core.ItemOakLog, core.ItemWorkbench, core.ItemStone} {
		for _, aspect := range []float32{16.0 / 9, 4.0 / 3} {
			projection := core.Perspective(viewmodelProjectionFovY, aspect, .1, 100)
			var angles []float32
			for tick := uint64(0); tick <= 14; tick++ {
				angles = append(angles, ViewmodelMiningAngle(tick, 0, ViewmodelTierBlock))
			}
			for age := uint8(0); age <= ViewmodelAttackFrames; age++ {
				angles = append(angles, ViewmodelAttackAngle(age, ViewmodelTierBlock))
			}
			for _, angle := range angles {
				parts := buildViewmodelParts(nil, &ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}}, angle)
				out := make([]byte, len(parts)*96)
				encodeAvatarPartsInto(out, parts)
				for i := 1; i < len(parts); i++ {
					hull := viewmodelInstanceScreenHull(t, out, i, projection)
					for _, p := range hull {
						if abs32(p[0]) >= 1 || abs32(p[1]) >= 1 {
							t.Fatalf("item %d face %d outside frame", item, i)
						}
					}
					if viewmodelHullCoversPoint(hull, mgl32.Vec2{}) {
						t.Fatalf("item %d face %d covers crosshair", item, i)
					}
				}
			}
		}
	}
}

func TestViewmodelArmEndRemainsClippedAcrossSwing(t *testing.T) {
	for _, aspect := range []float32{16.0 / 9, 4.0 / 3} {
		projection := core.Perspective(viewmodelProjectionFovY, aspect, .1, 100)
		for step := -14; step <= 14; step++ {
			angle := float32(step) * .05
			parts := buildViewmodelParts(nil, &ViewmodelInput{}, angle)
			out := make([]byte, len(parts)*96)
			encodeAvatarPartsInto(out, parts)
			for _, x := range []float32{-1, 1} {
				for _, z := range []float32{-1, 1} {
					p, w := projectToNDC(projection, viewmodelInstanceCorner(out, 0, x, -1, z))
					if w <= 0 || p[1] >= -1 {
						t.Fatalf("angle %.2f arm end visible: %v", angle, p)
					}
				}
			}
		}
	}
}

func TestViewmodelBlockFaceSlabsDoNotOverlap(t *testing.T) {
	parts := buildViewmodelParts(nil, &ViewmodelInput{Selected: core.ItemStack{Item: core.ItemGrass, Count: 1}}, 0)
	basis := parts[1].transform
	for col := range 3 {
		length := mgl32.Vec3{basis[col*4], basis[col*4+1], basis[col*4+2]}.Len()
		for row := range 3 {
			basis[col*4+row] /= length
		}
	}
	inverse := basis.Inv()
	for i := 1; i < len(parts); i++ {
		for j := i + 1; j < len(parts); j++ {
			a, b := inverse.Mul4(parts[i].transform), inverse.Mul4(parts[j].transform)
			overlap := true
			for axis := range 3 {
				ca, cb := a[12+axis], b[12+axis]
				sa, sb := abs32(a[axis*5])/2, abs32(b[axis*5])/2
				if min(ca+sa, cb+sb)-max(ca-sa, cb-sb) <= 1e-6 {
					overlap = false
				}
			}
			if overlap {
				t.Fatalf("block slabs %d and %d overlap, causing coplanar edge surfaces", i, j)
			}
		}
	}
}

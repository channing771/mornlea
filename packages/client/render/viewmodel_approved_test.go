package render

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
)

func TestViewmodelApprovedForearmAndFist(t *testing.T) {
	input := &ViewmodelInput{ViewportWidth: 1280, ViewportHeight: 720}
	parts := buildViewmodelParts(nil, input, 0)
	if len(parts) != 8 {
		t.Fatalf("hand parts=%d, want sleeve cuff palm thumb and four fingers", len(parts))
	}
	out := make([]byte, len(parts)*96)
	encodeAvatarPartsInto(out, parts)
	size := decodedPartSize(out, 0)
	if size[1] > .8 {
		t.Fatalf("long sleeve %v", size)
	}
	degrees := viewmodelSlantDegreesOf(out, 0)
	if degrees < 30 || degrees > 55 {
		t.Fatalf("forearm angle=%v, want diagonal", degrees)
	}
	projection := core.Perspective(viewmodelProjectionFovY, 1280.0/720, .1, 100)
	p, _ := projectToNDC(projection, decodedPartCenter(out, 2))
	x, y := (p[0]+1)/2, (1-p[1])/2
	if x < .82 || x > .86 || y < .76 || y > .82 {
		t.Fatalf("fist center=%v,%v", x, y)
	}
	if parts[2].color == parts[0].color || parts[3].color == parts[0].color {
		t.Fatal("fist/thumb still sleeve colored")
	}
	inv := parts[2].transform.Inv()
	thumb := inv.Mul4x1(parts[3].transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}))
	if abs32(thumb[0]) < .35 {
		t.Fatal("thumb silhouette absent")
	}
}

func TestViewmodelResizeDoesNotJumpAtLayoutBoundary(t *testing.T) {
	a := viewmodelGripRoot(&ViewmodelInput{ViewportWidth: 999, ViewportHeight: 720}, 0)
	b := viewmodelGripRoot(&ViewmodelInput{ViewportWidth: 1000, ViewportHeight: 720}, 0)
	if a.Col(3).Sub(b.Col(3)).Len() > .01 {
		t.Fatal("one-pixel resize visibly jumps hand")
	}
}

// 审核图按独立视口归一化：剑尖在握点右上方，护手位于拳面上方。
func TestViewmodelApprovedSwordScreenAnchors(t *testing.T) {
	input := &ViewmodelInput{ViewportWidth: 1280, ViewportHeight: 720, Selected: core.ItemStack{Item: core.ItemIronSword, Count: 1}}
	parts := buildViewmodelParts(nil, input, 0)
	out := make([]byte, len(parts)*96)
	encodeAvatarPartsInto(out, parts)
	projection := core.Perspective(viewmodelProjectionFovY, 1280.0/720, .1, 100)
	tip := mgl32.Vec2{0, 1}
	guard := mgl32.Vec2{}
	guardLeft, guardRight := float32(1), float32(0)
	models, _ := viewmodelDefaultRegistry.ItemToolParts(core.ItemIronSword)
	for i, p := range parts[8:] {
		for _, c := range viewmodelInstanceScreenHull(t, out, i+8, projection) {
			point := mgl32.Vec2{(c[0] + 1) / 2, (1 - c[1]) / 2}
			if models[i].Center[1] == .175 {
				guardLeft = min(guardLeft, point[0])
				guardRight = max(guardRight, point[0])
			}
			if point[1] < tip[1] {
				tip = point
			}
		}
		if models[i].Size[0] > .2 && models[i].Center[1] < .3 {
			c, _ := projectToNDC(projection, p.transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Vec3())
			guard = mgl32.Vec2{(c[0] + 1) / 2, (1 - c[1]) / 2}
		}
	}
	if tip[0] < .88 || tip[0] > .90 || tip[1] < .19 || tip[1] > .21 {
		t.Errorf("sword tip=%v, expected approved upper-right silhouette", tip)
	}
	if guard[0] < .83 || guard[0] > .85 || guard[1] < .67 || guard[1] > .69 {
		t.Errorf("guard=%v", guard)
	}
	tilt := math.Atan2(float64((tip[0]-guard[0])*1280), float64((guard[1]-tip[1])*720)) * 180 / math.Pi
	length := float32(math.Hypot(float64((tip[0]-guard[0])*1280), float64((tip[1]-guard[1])*720)) / 720)
	t.Logf("sword tip %v guard %v tilt %.3f length %.4f guard width %.4f", tip, guard, tilt, length, guardRight-guardLeft)
	if length < .47 || length > .51 || guardRight-guardLeft < .15 || guardRight-guardLeft > .18 {
		t.Errorf("sword blade/guard proportions differ: %.4f / %.4f", length, guardRight-guardLeft)
	}
	if tilt < 10 || tilt > 12 {
		t.Errorf("sword leans %.1f degrees right, expected 10–15", tilt)
	}
}

func TestViewmodelApprovedClosedFist(t *testing.T) {
	parts := buildViewmodelParts(nil, &ViewmodelInput{}, 0)
	inverse := parts[2].transform.Inv()
	// 亮面贴合实心掌面；不能再出现高于拳掌的独立板条盖。
	for _, i := range []int{4, 5} {
		local := inverse.Mul4(parts[i].transform)
		for _, x := range []float32{-.5, .5} {
			for _, y := range []float32{-.5, .5} {
				for _, z := range []float32{-.5, .5} {
					p := local.Mul4x1(mgl32.Vec4{x, y, z, 1})
					if abs32(p[0]) > .52 || abs32(p[1]) > .52 || abs32(p[2]) > .53 {
						t.Errorf("palm facet %d protrudes: %v", i, p)
					}
				}
			}
		}
	}
}

func TestViewmodelHelmetCheekSeatsInPalm(t *testing.T) {
	for _, size := range [][2]float32{{1280, 720}, {1600, 900}, {1280, 960}} {
		parts := buildViewmodelParts(nil, &ViewmodelInput{ViewportWidth: size[0], ViewportHeight: size[1], Selected: core.ItemStack{Item: core.ItemIronHelmet, Count: 1}}, 0)
		out := make([]byte, len(parts)*96)
		encodeAvatarPartsInto(out, parts)
		projection := core.Perspective(viewmodelProjectionFovY, size[0]/size[1], .1, 100)
		palm := viewmodelInstanceScreenHull(t, out, 2, projection)
		seated := false
		for i := 8; i < len(parts); i++ {
			cheek := viewmodelInstanceScreenHull(t, out, i, projection)
			for _, p := range cheek {
				if viewmodelHullCoversPoint(palm, p) {
					seated = true
				}
			}
		}
		if !seated {
			t.Errorf("helmet cheek floats above palm at %v", size)
		}
	}
}

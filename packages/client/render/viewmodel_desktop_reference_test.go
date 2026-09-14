package render

import (
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
	"math"
	"testing"
)

// 量取真实投影凸包与屏边的交点，不以根或中心点代替可见轮廓。
func TestViewmodelDesktopSleeveIntersections(t *testing.T) {
	for _, id := range []core.ItemID{0, core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe} {
		parts := buildViewmodelParts(nil, &ViewmodelInput{ViewportWidth: 1280, ViewportHeight: 720, Selected: core.ItemStack{Item: id, Count: 1}}, 0)
		out := make([]byte, len(parts)*96)
		encodeAvatarPartsInto(out, parts)
		projection := core.Perspective(viewmodelProjectionFovY, 1280.0/720, .1, 100)
		right, bottom := float32(10), float32(10)
		for _, idx := range []int{0, 1} {
			hull := viewmodelInstanceScreenHull(t, out, idx, projection)
			for i, a := range hull {
				b := hull[(i+1)%len(hull)]
				if (a[0]-1)*(b[0]-1) <= 0 && a[0] != b[0] {
					y := a[1] + (b[1]-a[1])*(1-a[0])/(b[0]-a[0])
					right = min(right, (1-y)/2)
				}
				if (a[1]+1)*(b[1]+1) <= 0 && a[1] != b[1] {
					x := a[0] + (b[0]-a[0])*(-1-a[1])/(b[1]-a[1])
					bottom = min(bottom, (1+x)/2)
				}
			}
		}
		t.Logf("item %d sleeve right %.4f bottom %.4f", id, right, bottom)
		if right < .95 || right > .98 || bottom < .83 || bottom > .87 {
			t.Errorf("item %d sleeve intersects right %.4f bottom %.4f; want .95–.98 / .83–.87", id, right, bottom)
		}
	}
}

func TestViewmodelDesktopPalmProfile(t *testing.T) {
	parts := buildViewmodelParts(nil, &ViewmodelInput{ViewportWidth: 1280, ViewportHeight: 720}, 0)
	out := make([]byte, len(parts)*96)
	encodeAvatarPartsInto(out, parts)
	projection := core.Perspective(viewmodelProjectionFovY, 1280.0/720, .1, 100)
	project := func(p mgl32.Vec3) mgl32.Vec2 {
		n, _ := projectToNDC(projection, p)
		return mgl32.Vec2{(n[0] + 1) * 640, (1 - n[1]) * 360}
	}
	a := project(viewmodelInstanceCorner(out, 2, 0, -1, 0))
	b := project(viewmodelInstanceCorner(out, 2, 0, 1, 0))
	axis := b.Sub(a).Normalize()
	cross := mgl32.Vec2{-axis[1], axis[0]}
	lo, hi := mgl32.Vec2{1e6, 1e6}, mgl32.Vec2{-1e6, -1e6}
	for _, p := range viewmodelInstanceScreenHull(t, out, 2, projection) {
		q := mgl32.Vec2{(p[0] + 1) * 640, (1 - p[1]) * 360}
		v := mgl32.Vec2{q.Dot(axis), q.Dot(cross)}
		for i := range 2 {
			lo[i] = min(lo[i], v[i])
			hi[i] = max(hi[i], v[i])
		}
	}
	length, width := (hi[0]-lo[0])/720, (hi[1]-lo[1])/720
	t.Logf("palm axis length %.4f width %.4f angle %.2f", length, width, math.Atan2(float64(axis[0]), float64(-axis[1]))*180/math.Pi)
	if length < .12 || length > .16 || width < .15 || width > .19 {
		t.Errorf("palm profile length %.4f width %.4f; want .12–.16 / .15–.19", length, width)
	}
}

func TestViewmodelDesktopToolHeadProportions(t *testing.T) {
	for _, id := range []core.ItemID{core.ItemIronPickaxe, core.ItemIronHoe} {
		parts := buildViewmodelParts(nil, &ViewmodelInput{ViewportWidth: 1280, ViewportHeight: 720, Selected: core.ItemStack{Item: id, Count: 1}}, 0)
		out := make([]byte, len(parts)*96)
		encodeAvatarPartsInto(out, parts)
		projection := core.Perspective(viewmodelProjectionFovY, 1280.0/720, .1, 100)
		lo, hi := mgl32.Vec2{1, 1}, mgl32.Vec2{}
		for i := 17; i < len(parts); i++ {
			for _, p := range viewmodelInstanceScreenHull(t, out, i, projection) {
				q := mgl32.Vec2{(p[0] + 1) / 2, (1 - p[1]) / 2}
				for k := range 2 {
					lo[k] = min(lo[k], q[k])
					hi[k] = max(hi[k], q[k])
				}
			}
		}
		width, height := hi[0]-lo[0], hi[1]-lo[1]
		t.Logf("tool %d head bbox %v .. %v width %.4f height %.4f", id, lo, hi, width, height)
		if id == core.ItemIronPickaxe {
			if width < .22 || width > .26 || height < .30 || height > .35 {
				t.Errorf("pick head too small or oversized: %.4f x %.4f", width, height)
			}
		} else {
			if width < .16 || width > .19 || height < .17 || height > .21 {
				t.Errorf("hoe head wrong proportions %.4f x %.4f", width, height)
			}
		}
		if id == core.ItemIronPickaxe {
			if lo[0] < .74 || lo[0] > .77 || hi[0] < .97 || hi[0] > .995 || lo[1] < .23 || lo[1] > .26 || hi[1] < .55 || hi[1] > .59 {
				t.Errorf("pick head corners differ from reference: %v .. %v", lo, hi)
			}
		} else {
			if lo[0] < .75 || lo[0] > .78 || hi[0] < .91 || hi[0] > .94 || lo[1] < .22 || lo[1] > .25 || hi[1] < .41 || hi[1] > .44 {
				t.Errorf("hoe head corners differ from reference: %v .. %v", lo, hi)
			}
		}
		models, _ := viewmodelDefaultRegistry.ItemToolParts(id)
		if id == core.ItemIronHoe {
			var sx0, sx1 float32 = 1, 0
			for _, p := range viewmodelInstanceScreenHull(t, out, 17, projection) {
				sx0 = min(sx0, (p[0]+1)/2)
				sx1 = max(sx1, (p[0]+1)/2)
			}
			t.Logf("hoe socket width %.4f", sx1-sx0)
			if sx1-sx0 < .045 || sx1-sx0 > .075 {
				t.Errorf("hoe socket oversized: %.4f", sx1-sx0)
			}
			for i, p := range models {
				if p.Size[0] > .25 && p.Size[2] > .05 && p.Center[0] < -.1 {
					a, _ := projectToNDC(projection, parts[i+8].transform.Mul4x1(mgl32.Vec4{-.5, 0, 0, 1}).Vec3())
					b, _ := projectToNDC(projection, parts[i+8].transform.Mul4x1(mgl32.Vec4{.5, 0, 0, 1}).Vec3())
					slope := math.Atan2(float64((b[1]-a[1])*720), float64((b[0]-a[0])*1280)) * 180 / math.Pi
					t.Logf("hoe blade slope %.2f", slope)
					if slope < 35 || slope > 40 {
						t.Errorf("hoe blade slope %.2f want 35–40 degrees", slope)
					}
				}
			}
		}
		socket := parts[17].transform
		foot, _ := projectToNDC(projection, socket.Mul4x1(mgl32.Vec4{0, -.5, 0, 1}).Vec3())
		palmCenter, _ := projectToNDC(projection, parts[2].transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Vec3())
		start := mgl32.Vec2{foot[0] * 640, foot[1] * 360}
		end := mgl32.Vec2{palmCenter[0] * 640, palmCenter[1] * 360}
		axis := end.Sub(start)
		fraction := float32(1)
		hull := viewmodelInstanceScreenHull(t, out, 2, projection)
		cross := func(a, b mgl32.Vec2) float32 { return a[0]*b[1] - a[1]*b[0] }
		for i, a := range hull {
			b := hull[(i+1)%len(hull)]
			a = mgl32.Vec2{a[0] * 640, a[1] * 360}
			b = mgl32.Vec2{b[0] * 640, b[1] * 360}
			edge := b.Sub(a)
			den := cross(axis, edge)
			if abs32(den) < 1e-6 {
				continue
			}
			u := cross(a.Sub(start), edge) / den
			v := cross(a.Sub(start), axis) / den
			if u >= 0 && u <= 1 && v >= 0 && v <= 1 {
				fraction = min(fraction, u)
			}
		}
		length := axis.Len() * fraction / 720
		t.Logf("tool %d socket %v exposed handle %.4f", id, models[9].Size, length)
		if length < .27 || length > .32 {
			t.Errorf("exposed handle %.4f want .27–.32H", length)
		}
	}
}

func TestViewmodelDesktopPalmFacetsFollowIdentity(t *testing.T) {
	colors := map[[4]float32]bool{}
	for n := byte(0); n < 16; n++ {
		parts := buildViewmodelParts(nil, &ViewmodelInput{Player: core.PlayerID{n}}, 0)
		colors[parts[4].color] = true
		light, dark := parts[4].color, parts[2].color
		if light[0] <= light[1] || light[1] <= light[2] || light[0] < dark[0]*1.8 {
			t.Fatalf("skin lost warm lit face and deep side: %v / %v", light, dark)
		}
		if parts[0].color == light || parts[1].color == light {
			t.Fatal("skin replaced identity sleeve")
		}
	}
	if len(colors) < 2 {
		t.Fatal("hand identity palettes collapsed")
	}
}

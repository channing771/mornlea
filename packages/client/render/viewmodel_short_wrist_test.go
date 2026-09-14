package render

import (
	"math"
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
)

var viewmodelToolVariants = [...]core.ItemID{
	core.ItemWoodenSword, core.ItemStoneSword, core.ItemIronSword,
	core.ItemBrokenWoodenSword, core.ItemBrokenStoneSword, core.ItemBrokenIronSword,
	core.ItemStonePickaxe, core.ItemIronPickaxe,
	core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe,
	core.ItemStoneHoe, core.ItemIronHoe,
	core.ItemBrokenStoneHoe, core.ItemBrokenIronHoe,
}

// 逐相位检验编码后的真实腕部尺寸，避免仅靠投影重叠掩盖细长皮肤桥。
func TestViewmodelToolsKeepShortPhysicalWrist(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	for _, item := range viewmodelToolVariants {
		var encoder ViewmodelEncoder
		input := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720, SwingActive: true}
		for step := 0; step <= 100; step++ {
			input.SwingPhase = float32(step) / 100
			out := encoder.EncodeViewmodelInstances(nil, &input)
			bottom := viewmodelInstanceCorner(out, 7, 0, -1, 0)
			top := viewmodelInstanceCorner(out, 7, 0, 1, 0)
			length := top.Sub(bottom).Len()
			if length > .12 {
				t.Fatalf("item %d phase %.2f skin bridge length %.3f exceeds short wrist", item, input.SwingPhase, length)
			}
			palmBottom := viewmodelInstanceCorner(out, 2, 0, -1, 0)
			palmTop := viewmodelInstanceCorner(out, 2, 0, 1, 0)
			if length > palmTop.Sub(palmBottom).Len()*1.1 {
				t.Fatalf("item %d phase %.2f skin bridge longer than fist", item, input.SwingPhase)
			}
			for _, pair := range [][2]int{{0, 6}, {6, 1}, {1, 7}, {7, 2}} {
				if !viewmodelPartsTouchOnCenterline(encoder.parts[pair[0]], encoder.parts[pair[1]]) {
					t.Fatalf("item %d phase %.2f physical arm gap between parts %d and %d", item, input.SwingPhase, pair[0], pair[1])
				}
			}
			palmNDC0, _ := projectToNDC(projection, palmBottom)
			palmNDC1, _ := projectToNDC(projection, palmTop)
			palmPixels := palmNDC1.Sub(palmNDC0).Len() * 360
			var exposed [2]mgl32.Vec3
			found := false
			for sample := 0; sample <= 32; sample++ {
				fraction := float32(sample) / 32
				point := bottom.Add(top.Sub(bottom).Mul(fraction))
				if viewmodelPointInPart(encoder.parts[1], point) || viewmodelPointInPart(encoder.parts[2], point) {
					continue
				}
				if !found {
					exposed[0] = point
					found = true
				}
				exposed[1] = point
			}
			if found {
				a, _ := projectToNDC(projection, exposed[0])
				b, _ := projectToNDC(projection, exposed[1])
				if pixels := b.Sub(a).Len() * 360; pixels > palmPixels*.5 {
					t.Fatalf("item %d phase %.2f exposed wrist %.1fpx exceeds half fist %.1fpx", item, input.SwingPhase, pixels, palmPixels)
				}
			}
		}
	}
}

// 皮肤棱体侧面可能比中心线露得更长；按实际深度可见的面像素量取腕轴跨度。
func TestViewmodelToolWristSideFacesStayCompact(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	toPixels := func(p mgl32.Vec2) mgl32.Vec2 { return mgl32.Vec2{(p[0] + 1) * 640, (1 - p[1]) * 360} }
	for _, item := range viewmodelToolVariants {
		var encoder ViewmodelEncoder
		input := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720, SwingActive: true}
		for _, phase := range []float32{0, .16, .24, .32, .40, .48, .56, .70, .85, 1} {
			input.SwingPhase = phase
			out := encoder.EncodeViewmodelInstances(nil, &input)
			palm0, _ := projectToNDC(projection, viewmodelInstanceCorner(out, 2, 0, -1, 0))
			palm1, _ := projectToNDC(projection, viewmodelInstanceCorner(out, 2, 0, 1, 0))
			palmLength := toPixels(mgl32.Vec2{palm1[0], palm1[1]}).Sub(toPixels(mgl32.Vec2{palm0[0], palm0[1]})).Len()
			wrist0, _ := projectToNDC(projection, viewmodelInstanceCorner(out, 7, 0, -1, 0))
			wrist1, _ := projectToNDC(projection, viewmodelInstanceCorner(out, 7, 0, 1, 0))
			axis := toPixels(mgl32.Vec2{wrist1[0], wrist1[1]}).Sub(toPixels(mgl32.Vec2{wrist0[0], wrist0[1]})).Normalize()
			// 横截面的投影厚度不能被误计为沿腕轴拉长的皮肤。
			crossSpan := float32(0)
			for _, y := range []float32{-1, 0, 1} {
				crossLo, crossHi := float32(1e6), float32(-1e6)
				for _, x := range []float32{-1, 1} {
					for _, z := range []float32{-1, 1} {
						point := encoder.parts[7].transform.Mul4x1(mgl32.Vec4{x * .5, y * .5, z * .5, 1}).Vec3()
						ndc, _ := projectToNDC(projection, point)
						along := toPixels(mgl32.Vec2{ndc[0], ndc[1]}).Dot(axis)
						crossLo, crossHi = min(crossLo, along), max(crossHi, along)
					}
				}
				crossSpan = max(crossSpan, crossHi-crossLo)
			}
			hull := viewmodelInstanceScreenHull(t, out, 7, projection)
			lo, hi := mgl32.Vec2{10, 10}, mgl32.Vec2{-10, -10}
			for _, p := range hull {
				lo[0], lo[1] = min(lo[0], p[0]), min(lo[1], p[1])
				hi[0], hi[1] = max(hi[0], p[0]), max(hi[1], p[1])
			}
			first, last := float32(1e6), float32(-1e6)
			for y := 0; y < 25; y++ {
				for x := 0; x < 25; x++ {
					p := mgl32.Vec2{lo[0] + (hi[0]-lo[0])*(float32(x)+.5)/25, lo[1] + (hi[1]-lo[1])*(float32(y)+.5)/25}
					if !viewmodelHullCoversPoint(hull, p) {
						continue
					}
					ray := mgl32.Vec3{p[0] * (1280.0 / 720) * float32(math.Tan(35*math.Pi/180)), p[1] * float32(math.Tan(35*math.Pi/180)), -1}
					nearest, depth := -1, float32(1e6)
					for index, part := range encoder.parts {
						if d, hit := viewmodelPartRayDepth(part, ray); hit && d < depth {
							nearest, depth = index, d
						}
					}
					if nearest == 7 {
						along := toPixels(p).Dot(axis)
						first, last = min(first, along), max(last, along)
					}
				}
			}
			if last >= first && last-first-crossSpan > palmLength*.5 {
				t.Fatalf("item %d phase %.2f visible wrist side length %.1fpx exceeds half fist %.1fpx", item, phase, last-first-crossSpan, palmLength)
			}
		}
	}
}

func viewmodelPointInPart(part avatarPart, point mgl32.Vec3) bool {
	q := part.transform.Inv().Mul4x1(mgl32.Vec4{point[0], point[1], point[2], 1}).Vec3()
	return abs32(q[0]) <= .5 && abs32(q[1]) <= .5 && abs32(q[2]) <= .5
}

func viewmodelPartsTouchOnCenterline(a, b avatarPart) bool {
	start := a.transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Vec3()
	end := b.transform.Mul4x1(mgl32.Vec4{0, 0, 0, 1}).Vec3()
	interval := func(part avatarPart) (float32, float32) {
		inverse := part.transform.Inv()
		x := inverse.Mul4x1(mgl32.Vec4{start[0], start[1], start[2], 1}).Vec3()
		y := inverse.Mul4x1(mgl32.Vec4{end[0], end[1], end[2], 1}).Vec3()
		lo, hi := float32(0), float32(1)
		for axis := range 3 {
			d := y[axis] - x[axis]
			if abs32(d) < 1e-6 {
				if abs32(x[axis]) > .5 {
					return 2, -1
				}
				continue
			}
			u, v := (-.5-x[axis])/d, (.5-x[axis])/d
			if u > v {
				u, v = v, u
			}
			lo, hi = max(lo, u), min(hi, v)
		}
		return lo, hi
	}
	a0, a1 := interval(a)
	b0, b1 := interval(b)
	return max(a0, b0) <= min(a1, b1)+1e-4
}

// 同一物理手臂应遵循共同轨迹；剑、镐和锄的区别由握点附近的工具刃口体现。
func TestViewmodelToolsShareArmStroke(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	var baseline [101][2]mgl32.Vec2
	for variant, item := range viewmodelToolVariants {
		var encoder ViewmodelEncoder
		input := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720, SwingActive: true}
		input.SwingPhase = 0
		neutral := encoder.EncodeViewmodelInstances(nil, &input)
		neutralCenter := [2]mgl32.Vec2{}
		for i := range neutralCenter {
			p, _ := projectToNDC(projection, decodedPartCenter(neutral, i))
			neutralCenter[i] = mgl32.Vec2{p[0], p[1]}
		}
		for step := 0; step <= 100; step++ {
			input.SwingPhase = float32(step) / 100
			out := encoder.EncodeViewmodelInstances(nil, &input)
			for i := range neutralCenter {
				p, _ := projectToNDC(projection, decodedPartCenter(out, i))
				delta := mgl32.Vec2{(p[0] - neutralCenter[i][0]) / 2, (p[1] - neutralCenter[i][1]) / 2}
				if variant == 0 {
					baseline[step][i] = delta
				} else if delta.Sub(baseline[step][i]).Len() > .02 {
					t.Fatalf("item %d phase %.2f sleeve part %d diverges from common path: %v / %v", item, input.SwingPhase, i, delta, baseline[step][i])
				}
			}
		}
	}
}

// 拳掌的真实射线可见面积在击出时仍占主体，袖口不能在前景吞掉握拳。
func TestViewmodelToolFistStaysVisibleThroughStroke(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	for _, item := range viewmodelToolVariants {
		var encoder ViewmodelEncoder
		input := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720, SwingActive: true}
		for _, phase := range []float32{0, .16, .24, .32, .40, .48, .56, .70, .85} {
			input.SwingPhase = phase
			out := encoder.EncodeViewmodelInstances(nil, &input)
			palm := viewmodelInstanceScreenHull(t, out, 2, projection)
			lo, hi := mgl32.Vec2{10, 10}, mgl32.Vec2{-10, -10}
			for _, p := range palm {
				lo[0], lo[1] = min(lo[0], p[0]), min(lo[1], p[1])
				hi[0], hi[1] = max(hi[0], p[0]), max(hi[1], p[1])
			}
			visible, total := 0, 0
			for y := 0; y < 25; y++ {
				for x := 0; x < 25; x++ {
					p := mgl32.Vec2{lo[0] + (hi[0]-lo[0])*(float32(x)+.5)/25, lo[1] + (hi[1]-lo[1])*(float32(y)+.5)/25}
					if !viewmodelHullCoversPoint(palm, p) {
						continue
					}
					ray := mgl32.Vec3{p[0] * (1280.0 / 720) * float32(math.Tan(35*math.Pi/180)), p[1] * float32(math.Tan(35*math.Pi/180)), -1}
					palmDepth, hit := viewmodelPartRayDepth(encoder.parts[2], ray)
					if !hit {
						continue
					}
					total++
					covered := false
					for _, sleeve := range []int{0, 1, 6} {
						if depth, hit := viewmodelPartRayDepth(encoder.parts[sleeve], ray); hit && depth < palmDepth-.001 {
							covered = true
							break
						}
					}
					if !covered {
						visible++
					}
				}
			}
			if total == 0 || visible*2 < total {
				t.Fatalf("item %d phase %.2f fist visible %d/%d projected samples", item, phase, visible, total)
			}
		}
	}
}

// 测量袖子真实凸包与屏幕边缘的交点，允许打击时从右下底边任一侧进入。
func TestViewmodelToolsEnterFromLowerRightAcrossStroke(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	for _, item := range viewmodelToolVariants {
		var encoder ViewmodelEncoder
		input := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720, SwingActive: true}
		for step := 0; step <= 100; step++ {
			input.SwingPhase = float32(step) / 100
			out := encoder.EncodeViewmodelInstances(nil, &input)
			right, bottom := viewmodelSleeveEntry(t, out, encoder.parts, projection)
			if !viewmodelSleeveEntryAccepted(right, bottom) {
				t.Fatalf("item %d phase %.2f sleeve enters from side: right %.3fH bottom %.3fW", item, input.SwingPhase, right, bottom)
			}
		}
	}
}

func viewmodelSleeveEntryAccepted(right, bottom float32) bool {
	if right < 10 {
		return right >= .90
	}
	return bottom < 10 && bottom >= .78
}

// 独立构造屏边缺失、屏外伪交点及仅布料连接段入侧边的编码实例，锁住判定器本身。
func TestViewmodelSleeveEntryOracleRejectsAbsentOrInvalidEdges(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	for _, probe := range []struct {
		name     string
		part     int
		position mgl32.Vec3
	}{
		{"no crossing", 6, mgl32.Vec3{0, 0, -1}},
		{"right crossing outside viewport", 0, mgl32.Vec3{1.22, -1.4, -1}},
		{"bridge crosses screen side", 6, mgl32.Vec3{1.22, 0, -1}},
	} {
		t.Run(probe.name, func(t *testing.T) {
			parts := make([]avatarPart, 8)
			for index := range parts {
				parts[index].transform = mgl32.Translate3D(0, 0, -1).Mul4(mgl32.Scale3D(.05, .05, .05))
			}
			parts[probe.part].transform = mgl32.Translate3D(probe.position[0], probe.position[1], probe.position[2]).Mul4(mgl32.Scale3D(.3, .3, .3))
			out := make([]byte, len(parts)*avatarInstanceBytes)
			encodeAvatarPartsInto(out, parts)
			if probe.name == "right crossing outside viewport" {
				hull := viewmodelInstanceScreenHull(t, out, probe.part, projection)
				outside := false
				for i, a := range hull {
					b := hull[(i+1)%len(hull)]
					if (a[0]-1)*(b[0]-1) > 0 || a[0] == b[0] {
						continue
					}
					y := a[1] + (b[1]-a[1])*(1-a[0])/(b[0]-a[0])
					outside = outside || y < -1 || y > 1
				}
				if !outside {
					t.Fatal("probe did not produce an off-viewport right-edge intersection")
				}
			}
			right, bottom := viewmodelSleeveEntry(t, out, parts, projection)
			if viewmodelSleeveEntryAccepted(right, bottom) {
				t.Fatalf("invalid sleeve edge accepted: right %.3f bottom %.3f", right, bottom)
			}
		})
	}
}

// 布料连接段在屏边被更近的拳掌遮住时，以实际可见的前臂轮廓判定入画位置。
func TestViewmodelSleeveEntryOracleIgnoresOccludedFabric(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	parts := make([]avatarPart, 8)
	for i := range parts {
		parts[i].transform = mgl32.Translate3D(0, 0, -1).Mul4(mgl32.Scale3D(.05, .05, .05))
	}
	parts[0].transform = mgl32.Translate3D(1.22, -.75, -1).Mul4(mgl32.Scale3D(.3, .3, .3))
	parts[6].transform = mgl32.Translate3D(1.22, 0, -1).Mul4(mgl32.Scale3D(.3, .3, .3))
	parts[2].transform = mgl32.Translate3D(.95, 0, -.65).Mul4(mgl32.Scale3D(.6, .6, .5))
	out := make([]byte, len(parts)*avatarInstanceBytes)
	encodeAvatarPartsInto(out, parts)
	right, bottom := viewmodelSleeveEntry(t, out, parts, projection)
	if !viewmodelSleeveEntryAccepted(right, bottom) {
		t.Fatalf("occluded fabric falsely counted as side entry: right %.3f bottom %.3f", right, bottom)
	}
}

func viewmodelSleeveEntry(t *testing.T, out []byte, parts []avatarPart, projection mgl32.Mat4) (right, bottom float32) {
	t.Helper()
	rightCuts, bottomCuts := []float32{0, 1}, []float32{0, 1}
	for index := range parts {
		hull := viewmodelInstanceScreenHull(t, out, index, projection)
		for i, a := range hull {
			b := hull[(i+1)%len(hull)]
			if (a[0]-1)*(b[0]-1) <= 0 && a[0] != b[0] {
				y := a[1] + (b[1]-a[1])*(1-a[0])/(b[0]-a[0])
				if y >= -1 && y <= 1 {
					rightCuts = append(rightCuts, (1-y)/2)
				}
			}
			if (a[1]+1)*(b[1]+1) <= 0 && a[1] != b[1] {
				x := a[0] + (b[0]-a[0])*(-1-a[1])/(b[1]-a[1])
				if x >= -1 && x <= 1 {
					bottomCuts = append(bottomCuts, (1+x)/2)
				}
			}
		}
	}
	slices.Sort(rightCuts)
	slices.Sort(bottomCuts)
	right = viewmodelVisibleFabricEdge(rightCuts, parts, projection.Inv(), true)
	bottom = viewmodelVisibleFabricEdge(bottomCuts, parts, projection.Inv(), false)
	return right, bottom
}

// 沿投影凸包在屏边的所有切分段抽样，取实际深度最前的布料联合轮廓。
// 用持物和拳掌的边界一并切段，避免被它们遮住的布料面误报侧边入画。
func viewmodelVisibleFabricEdge(cuts []float32, parts []avatarPart, projectionInverse mgl32.Mat4, rightEdge bool) float32 {
	for i := 0; i+1 < len(cuts); i++ {
		if cuts[i+1]-cuts[i] < 1e-5 {
			continue
		}
		position := (cuts[i] + cuts[i+1]) * .5
		x, y := position*2-1, float32(-1)+1e-4
		if rightEdge {
			x, y = 1-1e-4, 1-position*2
		}
		ray := projectionInverse.Mul4x1(mgl32.Vec4{x, y, 0, 1}).Vec3()
		nearest, nearestDepth := -1, float32(1e6)
		for index, part := range parts {
			if depth, hit := viewmodelPartRayDepth(part, ray); hit && depth < nearestDepth {
				nearest, nearestDepth = index, depth
			}
		}
		if nearest == 0 || nearest == 1 || nearest == 6 {
			return cuts[i]
		}
	}
	return 10
}

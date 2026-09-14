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
		if p.Center[0] == 0 && p.Size[0] >= .09 {
			socketY = max(socketY, p.Center[1])
		}
	}
	for _, sign := range []float32{-1, 1} {
		lowered := false
		for _, p := range pick {
			if p.Center[0]*sign > .2 && toolPartLowestY(p) < socketY-.015 && p.Size[2] > .01 {
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
		if p.Center[1] >= -.21 && p.Center[1] <= .70 && p.Size[0] >= .06 && p.Size[0] <= .08 && p.Center[0] == 0 && p.Size[1] < .2 {
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

// 最末实心段的截面本身必须收束，不检查可被宽翼遮住的小终端装饰。
func TestApprovedPickEndsTaperToPoints(t *testing.T) {
	parts, _ := NewDefaultRegistry().ItemToolParts(core.ItemIronPickaxe)
	for _, sign := range []float32{-1, 1} {
		var end ItemToolPart
		for _, p := range parts {
			if p.Center[0]*sign > end.Center[0]*sign {
				end = p
			}
		}
		corners := toolPartLocalCorners(end)
		var widths, depths [2]float32
		for i, fraction := range []float32{0, .45} {
			x := end.Size[0] * fraction
			lo, hi := [2]float32{1e6, 1e6}, [2]float32{-1e6, -1e6}
			for a := 0; a < 8; a++ {
				for axis := 0; axis < 3; axis++ {
					b := a ^ (1 << axis)
					if b < a {
						continue
					}
					p, q := corners[a], corners[b]
					if p[0] == q[0] || x < min(p[0], q[0]) || x > max(p[0], q[0]) {
						continue
					}
					f := (x - p[0]) / (q[0] - p[0])
					for k := range 2 {
						v := p[k+1] + (q[k+1]-p[k+1])*f
						lo[k] = min(lo[k], v)
						hi[k] = max(hi[k], v)
					}
				}
			}
			widths[i], depths[i] = hi[0]-lo[0], hi[1]-lo[1]
		}
		if widths[1] > widths[0]*.35 || depths[1] > depths[0]*.35 {
			t.Errorf("pick side %v has an untapered solid end: width %v depth %v", sign, widths, depths)
		}
	}
}

// 缓存几何的八角点用于独立量取局部截面与旋转后的最低端点。
func toolPartLocalCorners(p ItemToolPart) (corners [8][3]float32) {
	for i := range 8 {
		x, y, z := float32(i&1)-.5, float32((i>>1)&1)-.5, float32((i>>2)&1)-.5
		if p.Beveled {
			x, y, z = (.5*x-.5*y+.7071068*z)/1.7071068, (.7071068*x+.7071068*y)/1.4142136, (-.5*x+.5*y+.7071068*z)/1.7071068
		}
		corners[i] = [3]float32{x * p.Size[0], y * p.Size[1], z * p.Size[2]}
	}
	return
}

func toolPartLowestY(p ItemToolPart) float32 {
	lowest := float32(1e6)
	c, s := float32(math.Cos(float64(p.RotationZ))), float32(math.Sin(float64(p.RotationZ)))
	for _, v := range toolPartLocalCorners(p) {
		lowest = min(lowest, p.Center[1]+s*v[0]+c*v[1])
	}
	return lowest
}

func TestApprovedHoeIsAngledSlabAndPickFrontIsLight(t *testing.T) {
	r := NewDefaultRegistry()
	hoe, _ := r.ItemToolParts(core.ItemIronHoe)
	slab := false
	for _, p := range hoe {
		if p.Center[0] < -.1 && p.Size[0] >= .20 && p.Size[1] <= .10 && p.Size[2] >= .07 && p.RotationZ > .25 {
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

func TestApprovedSwordPointHasContinuousTaperedSupport(t *testing.T) {
	parts, _ := NewDefaultRegistry().ItemToolParts(core.ItemIronSword)
	lowerWidth, upperWidth := float32(0), float32(0)
	for _, p := range parts {
		if p.Size[2] >= .1 && p.Center[1] > .28 && p.Center[1] < .36 {
			lowerWidth = max(lowerWidth, p.Size[0])
		}
		if p.Size[2] >= .1 && p.Center[1] > .7 {
			upperWidth = max(upperWidth, p.Size[0])
		}
	}
	if lowerWidth == 0 || upperWidth == 0 || upperWidth > lowerWidth*.7 {
		t.Errorf("upper blade lacks progressive taper: %v -> %v", lowerWidth, upperWidth)
	}
	// 对实际旋转棱柱在刃尖横截面求交，捕获先缩成细颈又扩成菱形帽的轮廓。
	previousWidth := float32(1)
	previousLeft, previousRight := float32(-1), float32(1)
	for step := 0; step <= 27; step++ {
		y := float32(.886) + float32(step)*.001
		left, right := float32(1), float32(-1)
		for _, p := range parts {
			c, s := float32(math.Cos(float64(p.RotationZ))), float32(math.Sin(float64(p.RotationZ)))
			var corners [4][2]float32
			for i, xy := range [4][2]float32{{-.5, -.5}, {.5, -.5}, {.5, .5}, {-.5, .5}} {
				x, yy := xy[0]*p.Size[0], xy[1]*p.Size[1]
				corners[i] = [2]float32{p.Center[0] + c*x - s*yy, p.Center[1] + s*x + c*yy}
			}
			for i, a := range corners {
				b := corners[(i+1)%4]
				if y < min(a[1], b[1]) || y > max(a[1], b[1]) || a[1] == b[1] {
					continue
				}
				x := a[0] + (b[0]-a[0])*(y-a[1])/(b[1]-a[1])
				left, right = min(left, x), max(right, x)
			}
		}
		if left > right {
			t.Fatalf("sword point disconnected at y=%v", y)
		}
		width := right - left
		if width > previousWidth+.0005 {
			t.Fatalf("point widens above neck at y=%v: %v -> %v", y, previousWidth, width)
		}
		if left > previousRight || right < previousLeft {
			t.Fatalf("point lacks projected support at y=%v", y)
		}
		previousWidth, previousLeft, previousRight = width, left, right
	}
	if previousWidth > .025 {
		t.Fatalf("sword tip remains blunt: %v", previousWidth)
	}
}

func TestDesktopMetalFrontAndSideRemainDistinct(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe} {
		parts, _ := NewDefaultRegistry().ItemToolParts(item)
		pairs := 0
		for _, front := range parts {
			if min(front.Size[0], front.Size[2]) > .003 || front.Center[1] < .15 {
				continue
			}
			for _, body := range parts {
				if body.Size[0] < .01 || body.Center[2] != 0 || absToolColor(body.Center[0]-front.Center[0]) > body.Size[0]/2+.001 || absToolColor(body.Center[1]-front.Center[1]) > 1e-5 || body.Size[2] < .10 {
					continue
				}
				pairs++
				if front.Color[0] < body.Color[0]*3 {
					t.Errorf("item %d front and side wash together: %v / %v", item, front.Color, body.Color)
				}
			}
		}
		if pairs == 0 {
			t.Fatalf("item %d has no integrated bright front and dark solid side", item)
		}
	}
}

// 柄套必须与两翼根部共享真实体积；中立投影遮住的空气缝隙在转腕后仍会露出。
func TestPickSocketPhysicallyJoinsHead(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemStonePickaxe, core.ItemIronPickaxe, core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe} {
		parts, _ := NewDefaultRegistry().ItemToolParts(item)
		var socket ItemToolPart
		for _, p := range parts {
			if p.Center[0] == 0 && p.Size[0] == .09 {
				socket = p
			}
		}
		joined := 0
		for _, p := range parts {
			if absToolColor(p.Center[0]) < .05 || absToolColor(p.Center[0]) > .15 || p.Center[1] < .75 || p.Size[2] < .1 {
				continue
			}
			x := float32(.035)
			if p.Center[0] < 0 {
				x = -x
			}
			y := p.Center[1] + (x-p.Center[0])*float32(math.Tan(float64(p.RotationZ)))
			if absToolColor(x-socket.Center[0]) >= socket.Size[0]/2 || absToolColor(y-socket.Center[1]) >= socket.Size[1]/2-.002 {
				t.Errorf("item %d wing root (%v,%v) disconnected from socket %+v", item, x, y, socket)
			}
			joined++
		}
		if joined == 0 {
			t.Fatal("no physical wing roots checked")
		}
	}
}

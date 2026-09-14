package render

import (
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
	"math"
	"testing"
)

func TestViewmodelCompleteSweepClearsFullHUD(t *testing.T) {
	for _, viewport := range [][2]float32{{640, 360}, {1280, 720}, {800, 600}, {480, 360}} {
		w, h := viewport[0], viewport[1]
		scale := min((w-16)/476, (h-16)/180, float32(1))
		// 整条状态栈及最右选中格的抬升外框一并保守包围，避免只检测热栏中心。
		left, right, top := (w-476*scale)/2-8*scale, (w+476*scale)/2+8*scale, h-126*scale
		for _, fov := range []float32{30, 70, 110} {
			projection := core.Perspective(fov*math.Pi/180, w/h, .1, 100)
			for item := core.ItemID(0); item < core.ItemIDMax; item++ {
				for step := -14; step <= 14; step++ {
					parts := buildViewmodelParts(nil, &ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: w, ViewportHeight: h, FovY: fov * math.Pi / 180}, float32(step)*.05)
					out := make([]byte, len(parts)*96)
					encodeAvatarPartsInto(out, parts)
					hand := viewmodelInstanceScreenHull(t, out, 2, projection)
					for i := range parts {
						hull := viewmodelInstanceScreenHull(t, out, i, projection)
						if viewmodelHullCoversPoint(hull, mgl32.Vec2{}) {
							t.Fatalf("item %d part %d covers crosshair at %v", item, i, viewport)
						}
						if models, ok := viewmodelDefaultRegistry.ItemToolParts(item); ok && i >= 8 && models[i-8].Center[1]-models[i-8].Size[1]/2 > .20 {
							for _, corner := range hull {
								if abs32(corner[0]) >= 1 || abs32(corner[1]) >= 1 {
									t.Fatalf("tool %d head corner clipped at %v angle %v: %v", item, viewport, float32(step)*.05, corner)
								}
							}
							center, _ := projectToNDC(projection, decodedPartCenter(out, i))
							if abs32(center[0]) >= 1 || abs32(center[1]) >= 1 || viewmodelHullCoversPoint(hand, mgl32.Vec2{center[0], center[1]}) {
								t.Fatalf("tool %d head part %d unreadable at %v angle %v", item, i, viewport, float32(step)*.05)
							}
						}

						lo, hi := mgl32.Vec2{1e6, 1e6}, mgl32.Vec2{-1e6, -1e6}
						for _, p := range hull {
							x, y := (p[0]+1)*w/2, (1-p[1])*h/2
							lo[0] = min(lo[0], x)
							lo[1] = min(lo[1], y)
							hi[0] = max(hi[0], x)
							hi[1] = max(hi[1], y)
						}
						if viewmodelHUDIntersects(hull, left, right, top, h, w) {
							t.Fatalf("viewport %v fov %v item %d angle %v part %d overlaps HUD: %v..%v top %v", viewport, fov, item, float32(step)*.05, i, lo, hi, top)
						}
						if i >= 8 && (hi[0] < 0 || lo[0] > w || hi[1] < 0 || lo[1] > h) {
							t.Fatalf("viewport %v fov %v angle %v item %d part %d completely offscreen %v..%v", viewport, fov, float32(step)*.05, item, i, lo, hi)
						}
					}
					for _, x := range []float32{-1, 1} {
						for _, z := range []float32{-1, 1} {
							p, _ := projectToNDC(projection, viewmodelInstanceCorner(out, 0, x, -1, z))
							if abs32(p[0]) <= 1 && abs32(p[1]) <= 1 {
								t.Fatalf("arm root visible viewport %v fov %v angle %v point %v", viewport, fov, float32(step)*.05, p)
							}
						}
					}
				}
			}
		}
	}
}

// 凸投影与 HUD 矩形按分离轴相交；屏外臂根不应放大屏内遮挡包围。
func viewmodelHUDIntersects(hull []mgl32.Vec2, left, right, top, bottom, width float32) bool {
	rect := []mgl32.Vec2{{2*left/width - 1, 1 - 2*top/bottom}, {2*right/width - 1, 1 - 2*top/bottom}, {2*right/width - 1, -1}, {2*left/width - 1, -1}}
	for _, polygon := range [][]mgl32.Vec2{hull, rect} {
		for i, p := range polygon {
			edge := polygon[(i+1)%len(polygon)].Sub(p)
			axis := mgl32.Vec2{-edge[1], edge[0]}
			amin, amax, bmin, bmax := float32(1e6), float32(-1e6), float32(1e6), float32(-1e6)
			for _, v := range hull {
				d := v.Dot(axis)
				amin = min(amin, d)
				amax = max(amax, d)
			}
			for _, v := range rect {
				d := v.Dot(axis)
				bmin = min(bmin, d)
				bmax = max(bmax, d)
			}
			if amax <= bmin || bmax <= amin {
				return false
			}
		}
	}
	return true
}

package render

import (
	"bytes"
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
)

func TestViewmodelProductionPhaseContinuousEndpointsAndTravel(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemNone, core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe, core.ItemStone} {
		in := ViewmodelInput{ViewportWidth: 1280, ViewportHeight: 720, Selected: core.ItemStack{Item: item, Count: 1}}
		var encoder ViewmodelEncoder
		neutral := encoder.EncodeViewmodelInstances(nil, &in)
		projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
		rest, _ := projectToNDC(projection, decodedPartCenter(neutral, 2))
		in.SwingActive = true
		minX := rest[0]
		for step := 0; step <= 100; step++ {
			in.SwingPhase = float32(step) / 100
			out := encoder.EncodeViewmodelInstances(nil, &in)
			if (step == 0 || step == 100) && !bytes.Equal(neutral, out) {
				t.Fatalf("item %d phase %v endpoint discontinuity", item, in.SwingPhase)
			}
			p, _ := projectToNDC(projection, decodedPartCenter(out, 2))
			minX = min(minX, p[0])
		}
		travel := (rest[0] - minX) / 2
		if travel < .08 || travel > .16 {
			t.Errorf("item %d inward travel %.3f viewport widths", item, travel)
		}
		for _, join := range []float32{0, .16, .40, 1} {
			in.SwingPhase = join
			center := encoder.EncodeViewmodelInstances(nil, &in)
			for _, offset := range []float32{-.0001, .0001} {
				in.SwingPhase = max(0, min(1, join+offset))
				out := encoder.EncodeViewmodelInstances(nil, &in)
				for i := 0; i < len(out)/96; i++ {
					if decodedPartCenter(out, i).Sub(decodedPartCenter(center, i)).Len() > .001 {
						t.Fatalf("item %d discontinuity at %v", item, join)
					}
				}
			}
		}
	}
}

func TestViewmodelToolWristBridgesCuffAndPalm(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe} {
		var e ViewmodelEncoder
		for step := 0; step <= 100; step++ {
			e.EncodeViewmodelInstances(nil, &ViewmodelInput{SwingActive: true, SwingPhase: float32(step) / 100, Selected: core.ItemStack{Item: item, Count: 1}})
			// 腕部中心线必须穿过两个真实三维体积，投影重合或仅单轴包围相交均不足。
			for _, index := range []int{1, 2} {
				local := e.parts[index].transform.Inv().Mul4(e.parts[7].transform)
				a := local.Mul4x1(mgl32.Vec4{0, -.5, 0, 1}).Vec3()
				b := local.Mul4x1(mgl32.Vec4{0, .5, 0, 1}).Vec3()
				lo, hi := float32(0), float32(1)
				for axis := range 3 {
					d := b[axis] - a[axis]
					if abs32(d) < 1e-6 {
						if abs32(a[axis]) > .5 {
							lo = 2
						}
						continue
					}
					u, v := (-.5-a[axis])/d, (.5-a[axis])/d
					if u > v {
						u, v = v, u
					}
					lo = max(lo, u)
					hi = min(hi, v)
				}
				if lo > hi {
					t.Fatalf("item %d wrist has no physical connection to part %d", item, index)
				}
			}
		}
	}
}

func TestViewmodelProductionStrokeOrientation(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	for _, item := range []core.ItemID{core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe} {
		in := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720}
		var e ViewmodelEncoder
		axis := func(phase float32) (float64, mgl32.Vec3) {
			in.SwingActive = true
			in.SwingPhase = phase
			out := e.EncodeViewmodelInstances(nil, &in)
			a, _ := projectToNDC(projection, viewmodelInstanceCorner(out, 8, 0, -1, 0))
			b, _ := projectToNDC(projection, viewmodelInstanceCorner(out, 8, 0, 1, 0))
			return math.Atan2(float64((b[1]-a[1])*720), float64((b[0]-a[0])*1280)), decodedPartCenter(out, 8)
		}
		rest, _ := axis(0)
		peak, pos := axis(.40)
		degrees := math.Abs(peak-rest) * 180 / math.Pi
		t.Logf("item %d work rotation %.2f degrees, shaft center %v", item, degrees, pos)
		if degrees < 35 || degrees > 65 {
			t.Errorf("item %d insufficient/excessive projected working rotation %.2f", item, degrees)
		}
	}
}

func TestViewmodelPhaseVelocityJoinsAndWarmAllocation(t *testing.T) {
	var encoder ViewmodelEncoder
	dst := make([]byte, 0, ViewmodelMaxInstances*96)
	for item := core.ItemID(0); item < core.ItemIDMax; item++ {
		in := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, SwingActive: true, SwingPhase: .4}
		encoder.EncodeViewmodelInstances(dst, &in)
		if n := testing.AllocsPerRun(5, func() { encoder.EncodeViewmodelInstances(dst, &in) }); n != 0 {
			t.Fatalf("item %d moving allocations %v", item, n)
		}
		for _, join := range []float32{.16, .40} {
			const dt = float32(.001)
			in.SwingPhase = join - dt
			a := encoder.EncodeViewmodelInstances(nil, &in)
			in.SwingPhase = join
			b := encoder.EncodeViewmodelInstances(nil, &in)
			in.SwingPhase = join + dt
			c := encoder.EncodeViewmodelInstances(nil, &in)
			for part := 0; part < len(b)/96; part++ {
				before := decodedPartCenter(b, part).Sub(decodedPartCenter(a, part)).Mul(1 / dt)
				after := decodedPartCenter(c, part).Sub(decodedPartCenter(b, part)).Mul(1 / dt)
				if before.Sub(after).Len() > .10 {
					t.Fatalf("item %d part %d velocity discontinuity at %v: %v / %v", item, part, join, before, after)
				}
			}
		}
	}
}

func TestViewmodelCategoryWorkStrokeMovesTowardTarget(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	for _, item := range []core.ItemID{core.ItemNone, core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe} {
		in := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, SwingActive: true}
		var e ViewmodelEncoder
		sample := func(phase float32) mgl32.Vec3 {
			in.SwingPhase = phase
			out := e.EncodeViewmodelInstances(nil, &in)
			if item == core.ItemNone {
				return decodedPartCenter(out, 2)
			}
			models, _ := viewmodelDefaultRegistry.ItemToolParts(item)
			var sum mgl32.Vec3
			n := float32(0)
			for i, m := range models {
				if m.Center[1] > .3 {
					sum = sum.Add(decodedPartCenter(out, 8+i))
					n++
				}
			}
			return sum.Mul(1 / n)
		}
		wind, hit := sample(.16), sample(.40)
		a, _ := projectToNDC(projection, wind)
		b, _ := projectToNDC(projection, hit)
		t.Logf("item %d work displacement screen (%.3f,%.3f), depth %.3f", item, (b[0]-a[0])/2, (a[1]-b[1])/2, hit[2]-wind[2])
		if hit[2] >= wind[2]-.05 {
			t.Errorf("item %d work stroke has no targetward depth", item)
		}
		if item == core.ItemIronSword && b[0] >= a[0]-.25 {
			t.Error("sword does not slash inward")
		}
		if (item == core.ItemIronPickaxe || item == core.ItemIronHoe) && b[1] >= a[1]-.05 {
			t.Error("working head does not chop downward")
		}
	}
}

func TestViewmodelWorkStrokeIsFasterThanRecovery(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemNone, core.ItemIronSword, core.ItemIronPickaxe, core.ItemIronHoe, core.ItemStone} {
		in := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, SwingActive: true}
		var e ViewmodelEncoder
		speed := func(phase float32) float32 {
			in.SwingPhase = phase - .01
			a := e.EncodeViewmodelInstances(nil, &in)
			in.SwingPhase = phase + .01
			b := e.EncodeViewmodelInstances(nil, &in)
			var sum float32
			for i := 0; i < len(a)/96; i++ {
				sum += decodedPartCenter(a, i).Sub(decodedPartCenter(b, i)).Len()
			}
			return sum
		}
		work, recover := speed(.28), speed(.70)
		if work < recover*1.8 {
			t.Fatalf("item %d work segment is not decisively faster: %v / %v", item, work, recover)
		}
	}
}

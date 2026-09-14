package render

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
)

// 锄的落点取实际编码的自由刃端，避免柄套或整件模型中心冒充工作端。
func TestViewmodelHoeFreeEdgeLeadsWorkingApproach(t *testing.T) {
	projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
	toPixels := func(p mgl32.Vec3) mgl32.Vec2 {
		ndc, _ := projectToNDC(projection, p)
		return mgl32.Vec2{(ndc[0] + 1) * 640, (1 - ndc[1]) * 360}
	}
	for _, item := range []core.ItemID{core.ItemStoneHoe, core.ItemIronHoe, core.ItemBrokenStoneHoe, core.ItemBrokenIronHoe} {
		models, _ := viewmodelDefaultRegistry.ItemToolParts(item)
		cap, face, socket := -1, -1, -1
		for i, model := range models {
			switch {
			case model.Center[0] < -.22 && model.Size[0] == .04:
				cap = i + 8
			case model.Center[0] == -.18 && model.Center[2] > .06:
				face = i + 8
			case model.Center[0] == 0 && model.Size[0] == .09 && model.Size[2] == .125:
				socket = i + 8
			}
		}
		if cap < 0 || face < 0 || socket < 0 {
			t.Fatalf("item %d missing cutting cap, blade face or socket: %d/%d/%d", item, cap, face, socket)
		}
		var encoder ViewmodelEncoder
		input := ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720, SwingActive: true}
		sample := func(phase float32) (edge, outer, axis, hub mgl32.Vec3, faceFacing float32) {
			input.SwingPhase = phase
			out := encoder.EncodeViewmodelInstances(nil, &input)
			edge = decodedPartCenter(out, cap)
			hub = decodedPartCenter(out, socket)
			axis = edge.Sub(hub).Normalize()
			// 从编码棱体八角中选离柄套最远的自由刃端，击点不能由刃端中心代替。
			farthest := float32(-1e6)
			for _, x := range []float32{-1, 1} {
				for _, y := range []float32{-1, 1} {
					for _, z := range []float32{-1, 1} {
						vertex := viewmodelInstanceCorner(out, cap, x, y, z)
						if distance := vertex.Sub(hub).Dot(axis); distance > farthest {
							farthest, outer = distance, vertex
						}
					}
				}
			}
			faceNormal := viewmodelInstanceCorner(out, face, 0, 0, 1).Sub(viewmodelInstanceCorner(out, face, 0, 0, -1)).Normalize()
			faceFacing = faceNormal.Dot(decodedPartCenter(out, face).Mul(-1).Normalize())
			return
		}
		wind, _, _, _, _ := sample(.16)
		for _, phase := range []float32{.32, .36, .40} {
			edge, outer, axis, hub, faceFacing := sample(phase)
			approach := edge.Sub(wind).Normalize()
			pixel, outerPixel, hubPixel := toPixels(edge), toPixels(outer), toPixels(hub)
			t.Logf("item %d phase %.2f free edge center %v outer %v socket %v lead %.3f outer lead %.3f alignment %.3f face facing %.3f", item, phase, pixel, outerPixel, hubPixel, hub[2]-edge[2], hub[2]-outer[2], axis.Dot(approach), faceFacing)
			if outerPixel[0] < 1280*.48 || outerPixel[0] > 1280*.63 || outerPixel[1] < 720*.45 || outerPixel[1] > 720*.56 ||
				pixel[0] < 1280*.48 || pixel[0] > 1280*.63 || pixel[1] < 720*.45 || pixel[1] > 720*.56 {
				t.Errorf("item %d phase %.2f free cutting edge misses bounded crosshair work area: center %v outer %v", item, phase, pixel, outerPixel)
			}
			if outerPixel[0] >= hubPixel[0] || outerPixel[1] <= hubPixel[1] || hub[2]-outer[2] < .12 || hub[2]-edge[2] < .12 {
				t.Errorf("item %d phase %.2f free edge does not lead inboard, below and targetward of socket", item, phase)
			}
			if axis.Dot(approach) < .75 || approach[1] >= 0 || approach[2] >= 0 {
				t.Errorf("item %d phase %.2f free edge axis misses descending targetward approach", item, phase)
			}
			if faceFacing <= 0 {
				t.Errorf("item %d phase %.2f bright cutting face points away from camera", item, phase)
			}
		}
		hit, _, _, _, _ := sample(.40)
		if hit[1] >= wind[1] || hit[2] >= wind[2]-.08 || toPixels(hit)[1] <= toPixels(wind)[1]+30 {
			t.Errorf("item %d free edge does not descend forward: %v -> %v", item, wind, hit)
		}
	}
}

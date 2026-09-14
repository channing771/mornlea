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
		sample := func(phase float32) (edge, axis, hub, faceNormal mgl32.Vec3) {
			input.SwingPhase = phase
			out := encoder.EncodeViewmodelInstances(nil, &input)
			edge = decodedPartCenter(out, cap)
			hub = decodedPartCenter(out, socket)
			axis = edge.Sub(hub).Normalize()
			// 八个实际编码角点应围住自由刃端中心，避免只测试局部模型数据。
			for _, x := range []float32{-1, 1} {
				for _, y := range []float32{-1, 1} {
					for _, z := range []float32{-1, 1} {
						vertex := viewmodelInstanceCorner(out, cap, x, y, z)
						if vertex.Sub(edge).Len() > .05 {
							t.Fatalf("item %d phase %.2f cap vertex detached from center", item, phase)
						}
					}
				}
			}
			faceNormal = viewmodelInstanceCorner(out, face, 0, 0, 1).Sub(viewmodelInstanceCorner(out, face, 0, 0, -1)).Normalize()
			return
		}
		wind, _, _, _ := sample(.16)
		for _, phase := range []float32{.32, .36, .40} {
			edge, axis, hub, faceNormal := sample(phase)
			approach := edge.Sub(wind).Normalize()
			pixel, hubPixel := toPixels(edge), toPixels(hub)
			t.Logf("item %d phase %.2f free edge %v px %v socket %v lead %.3f alignment %.3f face normal %v", item, phase, edge, pixel, hubPixel, hub[2]-edge[2], axis.Dot(approach), faceNormal)
			if pixel[0] > 1280*.63 || pixel[1] < 720*.45 || pixel[1] > 720*.56 {
				t.Errorf("item %d phase %.2f free edge misses crosshair work area: %v", item, phase, pixel)
			}
			if pixel[0] >= hubPixel[0] || pixel[1] <= hubPixel[1] || hub[2]-edge[2] < .12 {
				t.Errorf("item %d phase %.2f free edge does not lead inboard, below and targetward of socket", item, phase)
			}
			if axis.Dot(approach) < .75 || approach[1] >= 0 || approach[2] >= 0 {
				t.Errorf("item %d phase %.2f free edge axis misses descending targetward approach", item, phase)
			}
			if faceNormal[2] <= 0 {
				t.Errorf("item %d phase %.2f bright cutting face points away from camera", item, phase)
			}
		}
		hit, _, _, _ := sample(.40)
		if hit[1] >= wind[1] || hit[2] >= wind[2]-.08 || toPixels(hit)[1] <= toPixels(wind)[1]+30 {
			t.Errorf("item %d free edge does not descend forward: %v -> %v", item, wind, hit)
		}
	}
}

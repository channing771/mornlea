package render

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/go-gl/mathgl/mgl32"
)

// 镐的工作端取所有损坏形态仍保留的左尖；由实际编码棱体顶点定位，避免根角度冒充凿击。
func TestViewmodelPickTerminalLeadsWorkingApproach(t *testing.T) {
	for _, item := range []core.ItemID{core.ItemStonePickaxe, core.ItemIronPickaxe, core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe} {
		models, _ := viewmodelDefaultRegistry.ItemToolParts(item)
		terminal, socket, opposite := -1, -1, -1
		for i, p := range models {
			if p.Beveled {
				if p.Center[0] < 0 {
					terminal = i + 8
				} else {
					opposite = i + 8
				}
			}
			if p.Center[0] == 0 && p.Size[0] == .09 {
				socket = i + 8
			}
		}
		if terminal < 0 || socket < 0 {
			t.Fatal("missing working geometry")
		}
		var e ViewmodelEncoder
		sample := func(phase float32) (tip, axis, hub, other mgl32.Vec3) {
			out := e.EncodeViewmodelInstances(nil, &ViewmodelInput{Selected: core.ItemStack{Item: item, Count: 1}, ViewportWidth: 1280, ViewportHeight: 720, SwingActive: true, SwingPhase: phase})
			center := decodedPartCenter(out, terminal)
			axis = center.Sub(decodedPartCenter(out, terminal-2)).Normalize()
			best := float32(-1e6)
			for _, x := range []float32{-1, 1} {
				for _, y := range []float32{-1, 1} {
					for _, z := range []float32{-1, 1} {
						p := viewmodelInstanceCorner(out, terminal, x, y, z)
						if d := p.Sub(center).Dot(axis); d > best {
							best, tip = d, p
						}
					}
				}
			}
			axis = tip.Sub(center).Normalize()
			hub = decodedPartCenter(out, socket)
			other = hub
			if opposite >= 0 {
				other = decodedPartCenter(out, opposite)
			}
			return
		}
		wind, _, _, _ := sample(.16)
		for step := 32; step <= 40; step++ {
			phase := float32(step) / 100
			tip, axis, hub, other := sample(phase)
			before, _, _, _ := sample(phase - .01)
			approach := tip.Sub(before).Normalize()
			t.Logf("item %d phase %.2f tip %v lead %.3f axis %v approach %v alignment %.3f", item, phase, tip, hub[2]-tip[2], axis, approach, axis.Dot(approach))
			// 至少一掌深度领先，尖轴前向分量过半，并在约 41° 内沿运动方向入射。
			if tip[2] > hub[2]-.12 || tip[2] > other[2]-.12 {
				t.Error("point does not lead socket and opposite terminal toward target")
			}
			if axis[2] > -.55 || axis.Dot(approach) < .75 {
				t.Error("terminal axis does not agree with targetward working approach")
			}
		}
		hit, _, _, _ := sample(.40)
		projection := core.Perspective(70*math.Pi/180, 1280.0/720, .1, 100)
		a, _ := projectToNDC(projection, wind)
		b, _ := projectToNDC(projection, hit)
		if hit[2] >= wind[2]-.08 || b[1] >= a[1]-.08 {
			t.Errorf("terminal does not descend targetward: %v -> %v screen %v -> %v", wind, hit, a, b)
		}
	}
}

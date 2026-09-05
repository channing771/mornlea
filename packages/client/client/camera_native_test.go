//go:build darwin

package client_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestNativeParity 把原生桥与纯 Go 参考实现逐项钉死：100 组随机位姿下
// `NativeViewProj` 的矩阵与视锥逐元相等；随机连通图下 `NativeVisibleSections`
// 的发射数量与顺序与 `mesh.VisibleSections` 逐项相等（含起点纵坐标越界与
// 负半径的空结果路径）。
func TestNativeParity(t *testing.T) {
	rng := rand.New(rand.NewSource(0x1CE5EED))
	for i := 0; i < 100; i++ {
		cam := client.Camera{
			Pos: mgl32.Vec3{
				float32(rng.NormFloat64() * 256),
				float32(40 + rng.Float64()*160),
				float32(rng.NormFloat64() * 256),
			},
			Yaw:    float32(rng.NormFloat64() * math.Pi),
			Pitch:  float32((rng.Float64()*2 - 1) * 1.2),
			FovY:   float32(0.6 + rng.Float64()*1.2),
			Aspect: float32(0.5 + rng.Float64()*2),
			Near:   0.1,
			Far:    float32(200 + rng.Float64()*1300),
		}
		vpArr, frustum := client.NativeViewProj(&cam)
		wantVP := cam.ViewProj()
		for j := range vpArr {
			if vpArr[j] != wantVP[j] {
				t.Fatalf("第 %d 组：视图投影第 %d 项 = %v，想要 %v", i, j, vpArr[j], wantVP[j])
			}
		}
		if mgl32.Mat4(vpArr) != wantVP {
			t.Fatalf("第 %d 组：`[16]float32` 回转 `mgl32.Mat4` 不一致", i)
		}
		if wantFrustum := core.FrustumFrom(wantVP); frustum != wantFrustum {
			t.Fatalf("第 %d 组：视锥不一致", i)
		}

		useFrustum := frustum
		if i%4 == 0 {
			useFrustum = mesh.EverythingVisible()
		}
		radius := rng.Intn(5)
		origin := core.SectionPos{
			X: int32(rng.Intn(65) - 32),
			Y: int32(rng.Intn(core.SectionsPerChunk)),
			Z: int32(rng.Intn(65) - 32),
		}
		if i%10 == 9 {
			origin.Y = int32(core.SectionsPerChunk)
		}
		conn := make(map[core.SectionPos]mesh.Connectivity)
		for x := origin.X - int32(radius); x <= origin.X+int32(radius); x++ {
			for y := int32(0); y < int32(core.SectionsPerChunk); y++ {
				for z := origin.Z - int32(radius); z <= origin.Z+int32(radius); z++ {
					if rng.Float64() < 0.3 {
						continue
					}
					var mask uint16
					switch rng.Intn(4) {
					case 1:
						mask = 0x7FFF
					case 2, 3:
						mask = uint16(rng.Intn(1 << 15))
					}
					conn[core.SectionPos{X: x, Y: y, Z: z}] = mesh.Connectivity(mask)
				}
			}
		}
		lookup := func(p core.SectionPos) (mesh.Connectivity, bool) {
			c, ok := conn[p]
			return c, ok
		}
		got := client.NativeVisibleSections(origin, radius, useFrustum, lookup)
		want := mesh.VisibleSections(origin, radius, useFrustum, lookup)
		if len(got) != len(want) {
			t.Fatalf("第 %d 组：可见数 = %d，想要 %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("第 %d 组：第 %d 个区段 = %+v，想要 %+v", i, j, got[j], want[j])
			}
		}
	}

	lookup := func(core.SectionPos) (mesh.Connectivity, bool) { return 0, false }
	if got := client.NativeVisibleSections(core.SectionPos{}, -1, mesh.EverythingVisible(), lookup); len(got) != 0 {
		t.Fatalf("负半径可见数 = %d，想要 0", len(got))
	}
}

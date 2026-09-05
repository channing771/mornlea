//go:build darwin

package client_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestNativeParity 把原生桥与纯 Go 参考实现钉死：100 组随机位姿下
// `NativeViewProj` 的矩阵与视锥逐元相等（含 `[16]float32` 回转一致性）。
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
	}
}

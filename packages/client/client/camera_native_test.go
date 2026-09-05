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

// TestNativeParity 是真正的跨语言 parity：100 组随机位姿下经 FFI 求解的
// NativeViewProj 与 Go 参考实现 Camera 的 ViewProj＋core 的 FrustumFrom
// 逐元比对。容差按量级缩放（1e-5·max(1,|want|)）；公式级分歧（行列错位、
// 符号、归一化缺失）偏差大数个量级，仍必被捕获。逐位一致由
// TestNativeViewProjBitwiseIdentity 钉死，本测试是其宽松烟雾层。
// FFI 执行证明见 Rust 侧同名出口的 bad-ABI 测试（Go 测试二进制不可直调
// cgo 入口；NativeViewProj 内除 FFI 外无回退路径）。
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
			tol := scaledEps(wantVP[j])
			if d := vpArr[j] - wantVP[j]; d < -tol || d > tol {
				t.Fatalf("第 %d 组：视图投影第 %d 项 = %v，想要 %v（差 %v）", i, j, vpArr[j], wantVP[j], d)
			}
		}
		wantFrustum := core.FrustumFrom(wantVP)
		for p := range frustum {
			for j := 0; j < 4; j++ {
				tol := scaledEps(wantFrustum[p][j])
				if d := frustum[p][j] - wantFrustum[p][j]; d < -tol || d > tol {
					t.Fatalf("第 %d 组：视锥平面 %d 第 %d 项 = %v，想要 %v（差 %v）", i, p, j, frustum[p][j], wantFrustum[p][j], d)
				}
			}
		}
	}
}

// scaledEps 返回量级缩放容差 1e-5·max(1,|want|)。
func scaledEps(want float32) float32 {
	if want < 0 {
		want = -want
	}
	return 1e-5 * max(1, want)
}

// TestNativeViewProjBitwiseIdentity 钉死跨语言逐位一致：20 万组随机位姿下
// FFI 出口与 Go 参考实现的矩阵＋视锥逐位相等（位模式整数比较）。
// 位级一致成立的前提是 Rust 内核逐字复现 Go 侧三处收窄（float64 三角／
// 开方、乘倒数归一化）与 Go 后端在帧循环内联上下文中的 FMA 融合形状；
// 任何一侧改动公式、Go 工具链升级改变融合、或 Rust 编译选项改变合约都会
// 让本测试变红——变红后先重跑 `make visual-check` 确认像素影响，再以权威
// 帧循环输出为基准重拟合各形状，不得放宽为容差比较。
func TestNativeViewProjBitwiseIdentity(t *testing.T) {
	rng := rand.New(rand.NewSource(0xB17))
	for i := 0; i < 200000; i++ {
		cam := client.Camera{
			Pos: mgl32.Vec3{
				float32(rng.NormFloat64() * 512),
				float32(rng.Float64()*400 - 64),
				float32(rng.NormFloat64() * 512),
			},
			Yaw:    float32(rng.NormFloat64() * math.Pi),
			Pitch:  float32((rng.Float64()*2 - 1) * 1.5),
			FovY:   float32(0.6 + rng.Float64()*1.2),
			Aspect: float32(0.5 + rng.Float64()*2),
			Near:   0.1,
			Far:    float32(200 + rng.Float64()*1300),
		}
		vpArr, frustum := client.NativeViewProj(&cam)
		wantVP := cam.ViewProj()
		for j := range vpArr {
			if math.Float32bits(vpArr[j]) != math.Float32bits(wantVP[j]) {
				t.Fatalf("第 %d 组：视图投影第 %d 项位模式 = %08x，想要 %08x", i, j, math.Float32bits(vpArr[j]), math.Float32bits(wantVP[j]))
			}
		}
		wantFrustum := core.FrustumFrom(wantVP)
		for p := range frustum {
			for j := 0; j < 4; j++ {
				if math.Float32bits(frustum[p][j]) != math.Float32bits(wantFrustum[p][j]) {
					t.Fatalf("第 %d 组：视锥平面 %d 第 %d 项位模式 = %08x，想要 %08x", i, p, j, math.Float32bits(frustum[p][j]), math.Float32bits(wantFrustum[p][j]))
				}
			}
		}
	}
}

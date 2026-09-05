package client_test

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
)

// TestCameraGoldenVectors 输出固定相机参数下的前向与投影数值，供 Rust
// 相机数学内核测试逐字抄入真值做跨语言 parity 断言。
func TestCameraGoldenVectors(t *testing.T) {
	cam := &client.Camera{Pos: mgl32.Vec3{1.5, 65.25, -3.75}, Yaw: 0.7, Pitch: -0.25, FovY: 1.22173, Aspect: 16.0 / 9.0, Near: 0.1, Far: 1536}
	t.Logf("FORWARD %.8f %.8f %.8f", cam.Forward()[0], cam.Forward()[1], cam.Forward()[2])
	vp := cam.ViewProj()
	t.Logf("VIEWPROJ %.8f %.8f %.8f %.8f", vp[0], vp[5], vp[10], vp[15])
}

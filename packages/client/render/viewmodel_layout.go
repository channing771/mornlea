package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// HUD 常显层在前端独占布局；这里仅镜像其安全包围与缩放分母，
// `TestViewmodelHUDFrontendParity` 钉住 geometry.ts / tokens.css，避免无解释漂移。
const (
	viewmodelHUDWidth        = float32(476)
	viewmodelHUDHeight       = float32(160)
	viewmodelHUDEdge         = float32(8)
	viewmodelHUDStatusHeight = float32(106)
)

// viewmodelGripRoot 在相机空间按逻辑 viewport 布局，XY 同比补偿世界 FOV，
// Z 不随 FOV 缩放以保持近裁面距离。臂从右侧出画，窄窗口向上让出完整状态栈。
func viewmodelGripRoot(input *ViewmodelInput, angle float32) mgl32.Mat4 {
	w, h := input.ViewportWidth, input.ViewportHeight
	if w <= 0 || h <= 0 {
		w, h = 1280, 720
	}
	fov := input.FovY
	if fov <= 0 {
		fov = 70 * math.Pi / 180
	}
	tangent := float32(math.Tan(float64(fov) / 2))
	scale := max(0, min((w-2*viewmodelHUDEdge)/viewmodelHUDWidth, (h-2*viewmodelHUDEdge)/viewmodelHUDHeight, float32(1)))
	y := min(float32(.35), 1-2*(viewmodelHUDStatusHeight*scale+80)/h)
	compensation := tangent / float32(math.Tan(35*math.Pi/180))
	return mgl32.Translate3D(.60*w/h*tangent*.95, -y*tangent*.95, -.95).
		Mul4(mgl32.Scale3D(compensation, compensation, 1)).
		Mul4(mgl32.HomogRotate3DZ(80*math.Pi/180 - angle*.30)).
		Mul4(mgl32.HomogRotate3DX(angle * .35))
}

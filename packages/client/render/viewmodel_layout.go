package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// HUD 常显层在前端独占布局；这里仅镜像其安全包围与缩放分母，
// `TestViewmodelHUDFrontendParity` 钉住 geometry.ts / tokens.css，避免无解释漂移。
const (
	viewmodelHUDWidth        = float32(476)
	viewmodelHUDHeight       = float32(180)
	viewmodelHUDEdge         = float32(8)
	viewmodelHUDStatusHeight = float32(126)
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
	// 护甲状态栈增高后，中立与预备动作略下移；下挥恢复净空以避开状态行。
	clearance := float32(68) + min(float32(12), max(angle, 0)*40)
	y := min(float32(.35), 1-2*(viewmodelHUDStatusHeight*scale+clearance)/h)
	y -= (max(0, 1.5-w/h) * .4) * max(angle, 0) / .7
	compensation := tangent / float32(math.Tan(35*math.Pi/180))
	return mgl32.Translate3D(.60*w/h*tangent*.95-max(angle, 0)*.12*compensation, -y*tangent*.95-(min(angle, 0)*.07+max(angle, 0)*.15)*compensation, -.95-max(angle, 0)*.22).
		Mul4(mgl32.Scale3D(compensation, compensation, 1)).
		Mul4(mgl32.HomogRotate3DZ(80*math.Pi/180 - angle*.12)).
		Mul4(mgl32.HomogRotate3DX(angle * .35))
}

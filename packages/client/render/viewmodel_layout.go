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
// Z 不随 FOV 缩放以保持近裁面距离。臂从右下底边出画，窄窗口向上让出完整状态栈。
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
	// 窄窗口收窄组合并向右上让出状态栈；缩放不改变手与物品的局部连接。
	compact := min(float32(1), w/1000)
	x := min(float32(.90), float32(.68)+(1-compact)*.55)
	y := min(float32(.58), 1-2*(viewmodelHUDStatusHeight*scale+55*compact+max(0, 640-w)*.25)/h)
	y += (.58 - y) * min(float32(1), max(float32(0), (w-800)/200))
	compensation := tangent / float32(math.Tan(35*math.Pi/180))
	depth := .95 + max(angle, 0)*.20
	return mgl32.Translate3D(x*w/h*tangent*depth-max(angle, 0)*.05*compensation, -y*tangent*depth-(min(angle, 0)*.035+max(angle, 0)*.06)*compensation, -depth).
		Mul4(mgl32.Scale3D(compensation*compact, compensation*compact, compact)).
		Mul4(mgl32.HomogRotate3DZ(50*math.Pi/180 - angle*.12)).
		Mul4(mgl32.HomogRotate3DX(angle * .20))
}

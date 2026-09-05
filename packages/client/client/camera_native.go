//go:build darwin

package client

// 本文件是相机查询的 Go 薄封装：`NativeViewProj` 复用纯 Go 相机与视锥
// 实现（`camera.go` 留作跨语言参考），供帧循环一次取齐视图投影矩阵与视锥。

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// NativeViewProj 返回相机视图投影矩阵（列主序）与对应视锥，供帧循环一次
// 取齐两个输入；数值与 Rust 相机数学内核同语义。
func NativeViewProj(cam *Camera) ([16]float32, core.Frustum) {
	viewProj := cam.ViewProj()
	return [16]float32(viewProj), core.FrustumFrom(viewProj)
}

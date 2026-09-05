//go:build darwin

package client

// 本文件是相机查询的 Go 薄封装：`NativeViewProj` 经无状态 FFI 出口调用
// Rust 相机数学内核，供帧循环一次取齐视图投影矩阵与视锥。
// 链接与 include 标志在 window.go 的 cgo 序言中声明，此处只补相机出口的
// 逃逸与回调指令。

/*
#cgo noescape mornlea_client_camera_viewproj
#cgo nocallback mornlea_client_camera_viewproj
#include "mornlea_client.h"
*/
import "C"

import (
	"unsafe"

	"github.com/channing771/mornlea/packages/shared/core"
)

// NativeViewProj 经 Rust 内核求解相机视图投影矩阵（列主序）与对应视锥，
// 供帧循环一次取齐两个输入；失败以稳定中文文案 panic（与既有窗口操作
// 口径一致），从不返回部分结果。
//
// 输出直写 Go 数组（`core.Frustum` 与 `[24]float32` 内存同构，末尾一次
// 重解释成型）：热路径零转换循环、零堆分配。
func NativeViewProj(cam *Camera) ([16]float32, core.Frustum) {
	pos := [3]float32{cam.Pos[0], cam.Pos[1], cam.Pos[2]}
	var viewProj [16]float32
	var frustumFlat [24]float32
	checkCameraStatus("viewproj", uint32(C.mornlea_client_camera_viewproj(
		C.MORNLEA_CLIENT_ABI_VERSION,
		(*C.float)(unsafe.Pointer(&pos[0])),
		C.float(cam.Yaw),
		C.float(cam.Pitch),
		C.float(cam.FovY),
		C.float(cam.Aspect),
		C.float(cam.Near),
		C.float(cam.Far),
		(*C.float)(unsafe.Pointer(&viewProj[0])),
		(*C.float)(unsafe.Pointer(&frustumFlat[0])),
	)))
	frustum := *(*core.Frustum)(unsafe.Pointer(&frustumFlat[0]))
	return viewProj, frustum
}

// checkCameraStatus 把相机查询的状态码折叠为稳定中文 panic，文案风格与
// `Renderer.check` 一致。
func checkCameraStatus(operation string, status uint32) {
	if status != uint32(C.MORNLEA_CLIENT_STATUS_OK) {
		panic("client: camera " + operation + " " + renderStatusText(status))
	}
}

//go:build darwin

package client

// 本文件是相机两段式查询的 Go 薄封装：`NativeViewProj` 复用纯 Go 相机与
// 视锥实现（`camera.go` 留作跨语言参考），`NativeVisibleSections` 把
// `mesh.Connectivity` 稠密化后经 `mornlea_client_camera_visible_len/fetch`
// 配对调用 Rust 可见性内核。链接与 include 标志在 window.go 的 cgo 序言中
// 声明，此处只补相机出口的逃逸与回调指令。

/*
#cgo noescape mornlea_client_camera_visible_len
#cgo nocallback mornlea_client_camera_visible_len
#cgo noescape mornlea_client_camera_visible_fetch
#cgo nocallback mornlea_client_camera_visible_fetch
#include "mornlea_client.h"
*/
import "C"

import (
	"runtime"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/shared/core"
)

// NativeViewProj 返回相机视图投影矩阵（列主序）与对应视锥，供帧循环一次
// 取齐两个输入；数值与 Rust 相机数学内核同语义。
func NativeViewProj(cam *Camera) ([16]float32, core.Frustum) {
	viewProj := cam.ViewProj()
	return [16]float32(viewProj), core.FrustumFrom(viewProj)
}

// NativeVisibleSections 经 Rust 内核求解可见区段，发射顺序与内容和
// `mesh.VisibleSections` 逐项一致。`lookup` 未覆盖的坐标视为未加载（保留
// 发射、跳过展开），与 Go 参考实现同语义。
func NativeVisibleSections(
	origin core.SectionPos,
	radius int,
	frustum core.Frustum,
	lookup func(core.SectionPos) (mesh.Connectivity, bool),
) []core.SectionPos {
	// 负半径与起点纵坐标越界按 `mesh.VisibleSectionsInto` 语义直接返回空，
	// 不进 FFI（FFI 对负半径报参数非法）。
	if radius < 0 || origin.Y < 0 || origin.Y >= core.SectionsPerChunk {
		return nil
	}
	var xyz []int32
	var mask []uint16
	for x := origin.X - int32(radius); x <= origin.X+int32(radius); x++ {
		for y := 0; y < core.SectionsPerChunk; y++ {
			for z := origin.Z - int32(radius); z <= origin.Z+int32(radius); z++ {
				pos := core.SectionPos{X: x, Y: int32(y), Z: z}
				conn, loaded := lookup(pos)
				if !loaded {
					continue
				}
				xyz = append(xyz, x, int32(y), z)
				mask = append(mask, uint16(conn))
			}
		}
	}
	var frustumArr [24]C.float
	for i := range frustum {
		for j := 0; j < 4; j++ {
			frustumArr[i*4+j] = C.float(frustum[i][j])
		}
	}
	var connXYZPtr *C.int32_t
	var connMaskPtr *C.uint16_t
	if len(mask) > 0 {
		connXYZPtr = (*C.int32_t)(unsafe.Pointer(&xyz[0]))
		connMaskPtr = (*C.uint16_t)(unsafe.Pointer(&mask[0]))
	}
	// Rust 结果缓存是调用线程局部的，Go goroutine 会在两次 cgo 调用之间
	// 迁移 OS 线程；`len→fetch` 配对必须钉在同一线程上完成。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var count C.uint32_t
	checkCameraStatus("visible len", uint32(C.mornlea_client_camera_visible_len(
		C.MORNLEA_CLIENT_ABI_VERSION,
		C.int32_t(origin.X),
		C.int32_t(origin.Y),
		C.int32_t(origin.Z),
		C.int32_t(radius),
		&frustumArr[0],
		C.size_t(len(frustumArr)),
		connXYZPtr,
		connMaskPtr,
		C.size_t(len(mask)),
		&count,
	)))
	if count == 0 {
		return nil
	}
	// 可见数的 3 倍恒为 3 的倍数，满足 `fetch` 的 `out_cap` 契约。
	out := make([]int32, uint(count)*3)
	var written C.size_t
	checkCameraStatus("visible fetch", uint32(C.mornlea_client_camera_visible_fetch(
		C.MORNLEA_CLIENT_ABI_VERSION,
		(*C.int32_t)(unsafe.Pointer(unsafe.SliceData(out))),
		C.size_t(len(out)),
		&written,
	)))
	visible := make([]core.SectionPos, 0, count)
	for i := uint32(0); i < uint32(count); i++ {
		visible = append(visible, core.SectionPos{X: out[3*i], Y: out[3*i+1], Z: out[3*i+2]})
	}
	return visible
}

// checkCameraStatus 把相机查询的状态码折叠为稳定中文 panic，文案风格与
// `Renderer.check` 一致。
func checkCameraStatus(operation string, status uint32) {
	if status != uint32(C.MORNLEA_CLIENT_STATUS_OK) {
		panic("client: camera " + operation + " " + renderStatusText(status))
	}
}

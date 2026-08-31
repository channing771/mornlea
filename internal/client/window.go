//go:build darwin

package client

// 本文件是 `mornlea_client` C ABI 的唯一 Go 调用方:窗口与输入采集的生产
// 实现位于 Rust winit(见 engine/crates/mornlea_client),Go 侧只保留领域
// API、快照解码与帧内缓存。`Poll` 每帧恰好一次 FFI 取回固定布局输入快照,
// 同帧内的按键/鼠标/光标/尺寸读取全部来自缓存,不产生额外窗口 FFI 调用。

/*
// client ABI v14: C header 常量与 Rust 动态库必须同批重建。
#cgo CFLAGS: -I${SRCDIR}/../../engine/include
#cgo LDFLAGS: -L${SRCDIR}/../../engine/target/release -lmornlea_client -Wl,-rpath,${SRCDIR}/../../engine/target/release
#cgo noescape mornlea_client_abi_version
#cgo nocallback mornlea_client_abi_version
#cgo noescape mornlea_client_window_create
#cgo nocallback mornlea_client_window_create
#cgo noescape mornlea_client_window_destroy
#cgo nocallback mornlea_client_window_destroy
#cgo noescape mornlea_client_window_poll
#cgo nocallback mornlea_client_window_poll
#cgo noescape mornlea_client_window_set_cursor_captured
#cgo nocallback mornlea_client_window_set_cursor_captured
#cgo noescape mornlea_client_window_set_content_size
#cgo nocallback mornlea_client_window_set_content_size
#cgo noescape mornlea_client_window_set_floating
#cgo nocallback mornlea_client_window_set_floating
#cgo noescape mornlea_client_window_focus
#cgo nocallback mornlea_client_window_focus
#cgo noescape mornlea_client_window_cancel_close
#cgo nocallback mornlea_client_window_cancel_close
#cgo noescape mornlea_client_ui_push_state
#cgo nocallback mornlea_client_ui_push_state
#cgo noescape mornlea_client_window_capture
#cgo nocallback mornlea_client_window_capture
#cgo noescape mornlea_client_render_drain_ui_events
#cgo nocallback mornlea_client_render_drain_ui_events
#include "mornlea_client.h"
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unsafe"
)

// Key 是主程序需要的最小按键集合。
type Key uint8

const (
	KeyW Key = iota
	KeyA
	KeyS
	KeyD
	KeySpace
	KeyLeftShift
	KeyLeftControl
	KeyEscape
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeyE
	KeyQ
	// 以下按键为调试面板交互追加，必须保持在末尾以免改变既有常量的 iota 取值。
	KeyF3
	KeyF5
	KeyF6
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyLeftAlt
	KeyBackspace
)

// 快照布局常量,必须与 Rust `input` 模块的布局文档逐字一致。
const (
	snapshotBytes         = int(C.MORNLEA_CLIENT_SNAPSHOT_BYTES)
	snapshotLayoutVersion = 1
	snapshotHeaderBytes   = 64
	textCapacity          = 1024
)

// ClientABIVersion 返回当前 client 库导出的 ABI 版本,供加载自检。
func ClientABIVersion() uint32 {
	return uint32(C.mornlea_client_abi_version())
}

// clientABIHeaderVersion 返回编译时 C header 的 ABI 常量，供三端一致性测试。
func clientABIHeaderVersion() uint32 {
	return uint32(C.MORNLEA_CLIENT_ABI_VERSION)
}

// snapshot 是一帧输入快照的解码结果。
type snapshot struct {
	keys              uint64
	mouse             uint32
	cursorX           float64
	cursorY           float64
	framebufferWidth  int
	framebufferHeight int
	contentWidth      int
	contentHeight     int
	shouldClose       bool
}

// decodeSnapshot 按固定布局解码快照头部;布局版本不符以稳定文案 panic。
// 文本段经 emit 逐字符回调,便于复用有界入队语义。
func decodeSnapshot(buf []byte, emit func(rune), overflow func()) snapshot {
	if len(buf) != snapshotBytes {
		panic("client: 输入快照长度非法")
	}
	if binary.LittleEndian.Uint32(buf[0:4]) != snapshotLayoutVersion {
		panic("client: 输入快照布局版本不匹配")
	}
	flags := binary.LittleEndian.Uint32(buf[4:8])
	decoded := snapshot{
		keys:              binary.LittleEndian.Uint64(buf[8:16]),
		mouse:             binary.LittleEndian.Uint32(buf[16:20]),
		cursorX:           math.Float64frombits(binary.LittleEndian.Uint64(buf[24:32])),
		cursorY:           math.Float64frombits(binary.LittleEndian.Uint64(buf[32:40])),
		framebufferWidth:  int(binary.LittleEndian.Uint32(buf[40:44])),
		framebufferHeight: int(binary.LittleEndian.Uint32(buf[44:48])),
		contentWidth:      int(binary.LittleEndian.Uint32(buf[48:52])),
		contentHeight:     int(binary.LittleEndian.Uint32(buf[52:56])),
		shouldClose:       flags&1 != 0,
	}
	textCount := binary.LittleEndian.Uint32(buf[20:24])
	if textCount > textCapacity {
		panic("client: 输入快照文本计数非法")
	}
	for index := 0; index < int(textCount); index++ {
		offset := snapshotHeaderBytes + index*4
		emit(rune(binary.LittleEndian.Uint32(buf[offset : offset+4])))
	}
	if flags&2 != 0 {
		overflow()
	}
	return decoded
}

// Window 封装 Rust winit 窗口:句柄、帧内快照缓存与有界文本队列。
//
// 文本队列语义与旧实现一致:跨 Poll 累积,上限 1024,溢出置标志并丢弃;
// DrainTextInput 追加返回并清空。
type Window struct {
	handle            uint64
	closed            bool
	captured          bool
	state             snapshot
	textInput         [textCapacity]rune
	textInputCount    int
	textInputOverflow bool
}

// NewWindow 创建 Rust winit 窗口并完成一次初始快照,保证创建后立即可读尺寸。
func NewWindow(width, height int, title string) (*Window, error) {
	titleBytes := []byte(title)
	var handle C.uint64_t
	var titlePtr *C.uint8_t
	if len(titleBytes) > 0 {
		titlePtr = (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(titleBytes)))
	}
	status := C.mornlea_client_window_create(
		C.MORNLEA_CLIENT_ABI_VERSION,
		C.uint32_t(width),
		C.uint32_t(height),
		titlePtr,
		C.size_t(len(titleBytes)),
		&handle,
	)
	if status != C.MORNLEA_CLIENT_STATUS_OK {
		return nil, fmt.Errorf("创建窗口: %s", windowStatusText(uint32(status)))
	}
	window := &Window{handle: uint64(handle)}
	window.Poll()
	return window, nil
}

// Poll 泵事件循环并取回本帧输入快照;每帧恰好一次窗口 FFI 调用。
func (w *Window) Poll() {
	var buf [snapshotBytes]byte
	status := C.mornlea_client_window_poll(
		C.MORNLEA_CLIENT_ABI_VERSION,
		C.uint64_t(w.handle),
		(*C.uint8_t)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if status != C.MORNLEA_CLIENT_STATUS_OK {
		panic("client: poll " + windowStatusText(uint32(status)))
	}
	w.state = decodeSnapshot(buf[:], w.enqueueTextInput, func() { w.textInputOverflow = true })
}

func (w *Window) enqueueTextInput(char rune) {
	if w.textInputCount == len(w.textInput) {
		w.textInputOverflow = true
		return
	}
	w.textInput[w.textInputCount] = char
	w.textInputCount++
}

// DrainTextInput 返回自上次 drain 后收到的字符与固定队列 overflow，并清空窗口队列。
func (w *Window) DrainTextInput(dst []rune) ([]rune, bool) {
	dst = append(dst, w.textInput[:w.textInputCount]...)
	overflow := w.textInputOverflow
	w.textInputCount = 0
	w.textInputOverflow = false
	return dst, overflow
}

// ShouldClose 报告本帧快照中的关闭请求。
func (w *Window) ShouldClose() bool { return w.state.shouldClose }

// CancelClose 撤销关闭请求并同步清除缓存标志。
func (w *Window) CancelClose() {
	w.call("cancel close", uint32(C.mornlea_client_window_cancel_close(
		C.MORNLEA_CLIENT_ABI_VERSION, C.uint64_t(w.handle))))
	w.state.shouldClose = false
}

// FramebufferSize 返回本帧快照中的物理像素尺寸。
func (w *Window) FramebufferSize() (int, int) {
	return w.state.framebufferWidth, w.state.framebufferHeight
}

// ContentSize 返回本帧快照中的逻辑点尺寸。
func (w *Window) ContentSize() (int, int) {
	return w.state.contentWidth, w.state.contentHeight
}

// SetContentSize 请求修改逻辑尺寸并立即刷新快照缓存,保证随后的读取一致。
func (w *Window) SetContentSize(width, height int) {
	w.call("set content size", uint32(C.mornlea_client_window_set_content_size(
		C.MORNLEA_CLIENT_ABI_VERSION, C.uint64_t(w.handle),
		C.uint32_t(width), C.uint32_t(height))))
	w.Poll()
}

// SetFloating 切换窗口置顶。
func (w *Window) SetFloating(floating bool) {
	w.call("set floating", uint32(C.mornlea_client_window_set_floating(
		C.MORNLEA_CLIENT_ABI_VERSION, C.uint64_t(w.handle), boolToU8(floating))))
}

// Focus 请求聚焦窗口。
func (w *Window) Focus() {
	w.call("focus", uint32(C.mornlea_client_window_focus(
		C.MORNLEA_CLIENT_ABI_VERSION, C.uint64_t(w.handle))))
}

// CursorPos 返回本帧快照中的光标位置;捕获期间为连续虚拟坐标。
func (w *Window) CursorPos() (float64, float64) {
	return w.state.cursorX, w.state.cursorY
}

// KeyDown 报告本帧快照中指定按键是否按下(含 repeat)。
func (w *Window) KeyDown(key Key) bool {
	if int(key) >= 64 {
		return false
	}
	return w.state.keys&(1<<key) != 0
}

// PrimaryButtonDown 报告本帧快照中鼠标主键是否按下。
func (w *Window) PrimaryButtonDown() bool { return w.state.mouse&1 != 0 }

// SecondaryButtonDown 报告本帧快照中鼠标副键是否按下。
func (w *Window) SecondaryButtonDown() bool { return w.state.mouse&2 != 0 }

// SetCursorCaptured 切换光标捕获;幂等。
func (w *Window) SetCursorCaptured(captured bool) {
	if captured == w.captured {
		return
	}
	w.captured = captured
	w.call("set cursor captured", uint32(C.mornlea_client_window_set_cursor_captured(
		C.MORNLEA_CLIENT_ABI_VERSION, C.uint64_t(w.handle), boolToU8(captured))))
}

// CursorCaptured 报告当前捕获状态。
func (w *Window) CursorCaptured() bool { return w.captured }

// PushUIState 把菜单层 UI 状态 JSON 推给挂在窗口上的 WebView(client ABI v12 引入、v14 保留
// 下行出口):Rust 侧缓存、按相位路由显隐并经 evaluateJavaScript 注入页面。
// 仅在状态变化时由 app 层调用;从未调用的进程(基准/capture)不创建 WebView。
// 首次调用惰性挂载;空 JSON 是调用方编程错误,panic(与既有窗口操作口径一致)。
func (w *Window) PushUIState(json []byte) {
	if len(json) == 0 {
		panic("client: push ui state 空 JSON")
	}
	w.call("push ui state", uint32(C.mornlea_client_ui_push_state(
		C.MORNLEA_CLIENT_ABI_VERSION,
		C.uint64_t(w.handle),
		(*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(json))),
		C.size_t(len(json)),
	)))
}

// ErrCaptureUnavailable 表示窗口合成捕获当前不可用:窗口号取不到、屏幕录制
// 授权未授予、系统返回空图等运行期预期条件。调用方应把它映射为可观察的
// 失败(如 HTTP 503),不得当作致命错误终止帧循环。
var ErrCaptureUnavailable = errors.New("client: 窗口合成捕获不可用")

// Capture 抓取当前窗口完整合成画面(世界 + wgpu HUD + WebView 菜单层),
// 返回自上而下、无行 padding 的 BGRA8 原始字节与合成图宽高,输出长度恰为
// width*height*4。
//
// client ABI v13 两段式容量协议:先以零容量调用拿所需字节数与尺寸(零容量
// 必然溢出,同时回填尺寸),再按 required 分配重试一次;第二次仍溢出说明
// 窗口尺寸在两次调用之间被外部改变,按契约违约 panic。捕获不可用返回
// `ErrCaptureUnavailable`;其余非 OK 状态以稳定中文文案 panic(与既有窗口
// 操作口径一致)。
//
// 线程约束:必须在窗口 poll 线程(与 `Poll` 同一线程)调用,Rust 侧窗口
// 句柄表是 thread-local 的。
func (w *Window) Capture() ([]byte, int, int, error) {
	status, required, width, height := w.captureInto(nil)
	switch status {
	case uint32(C.MORNLEA_CLIENT_STATUS_OK):
		panic("client: capture 零容量查询不应成功")
	case uint32(C.MORNLEA_CLIENT_STATUS_CAPTURE_UNAVAILABLE):
		return nil, 0, 0, ErrCaptureUnavailable
	case uint32(C.MORNLEA_CLIENT_STATUS_CAPTURE_OVERFLOW):
		// 预期路径:required 与尺寸已在出参回填。
	default:
		panic("client: capture " + windowStatusText(status))
	}
	pixels := make([]byte, required)
	status, required, width, height = w.captureInto(pixels)
	switch status {
	case uint32(C.MORNLEA_CLIENT_STATUS_OK):
	case uint32(C.MORNLEA_CLIENT_STATUS_CAPTURE_UNAVAILABLE):
		// 两次调用之间授权被收回等竞态:同样按不可用降级,不 panic。
		return nil, 0, 0, ErrCaptureUnavailable
	default:
		panic("client: capture " + windowStatusText(status))
	}
	if int(required) != len(pixels) || int(width)*int(height)*4 != len(pixels) {
		panic("client: capture 输出长度与合成图尺寸不一致")
	}
	return pixels, int(width), int(height), nil
}

// captureInto 以给定输出缓冲调用窗口捕获导出;out 为 nil 表示零容量查询。
// 无论溢出还是成功,都带回回填的所需字节数与合成图尺寸(更早的失败路径
// 由 Rust 侧保证出参原样,调用方只需按状态码分流)。
func (w *Window) captureInto(out []byte) (status uint32, required uint64, width, height uint32) {
	var outPtr *C.uint8_t
	if len(out) > 0 {
		outPtr = (*C.uint8_t)(unsafe.Pointer(&out[0]))
	}
	var requiredC C.uint64_t
	var widthC, heightC C.uint32_t
	status = uint32(C.mornlea_client_window_capture(
		C.MORNLEA_CLIENT_ABI_VERSION,
		C.uint64_t(w.handle),
		outPtr,
		C.uint64_t(len(out)),
		&requiredC,
		&widthC,
		&heightC,
	))
	return status, uint64(requiredC), uint32(widthC), uint32(heightC)
}

// Close 销毁窗口;重复调用安全。
func (w *Window) Close() {
	if w.closed {
		return
	}
	w.closed = true
	w.call("destroy", uint32(C.mornlea_client_window_destroy(
		C.MORNLEA_CLIENT_ABI_VERSION, C.uint64_t(w.handle))))
}

// call 把低频窗口操作的错误状态转为稳定中文 panic 文案。
func (w *Window) call(operation string, status uint32) {
	if status != uint32(C.MORNLEA_CLIENT_STATUS_OK) {
		panic("client: " + operation + " " + windowStatusText(status))
	}
}

func windowStatusText(status uint32) string {
	switch status {
	case uint32(C.MORNLEA_CLIENT_STATUS_ABI_VERSION):
		return "client ABI 版本不匹配"
	case uint32(C.MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT):
		return "client 参数非法"
	case uint32(C.MORNLEA_CLIENT_STATUS_WINDOW):
		return "client 窗口句柄无效或窗口操作失败"
	case uint32(C.MORNLEA_CLIENT_STATUS_PANIC):
		return "client Rust panic"
	case uint32(C.MORNLEA_CLIENT_STATUS_CAPTURE_OVERFLOW):
		return "client 窗口捕获输出缓冲溢出"
	case uint32(C.MORNLEA_CLIENT_STATUS_CAPTURE_UNAVAILABLE):
		return "client 窗口合成捕获不可用"
	default:
		return "client 未知状态"
	}
}

func boolToU8(value bool) C.uint8_t {
	if value {
		return 1
	}
	return 0
}

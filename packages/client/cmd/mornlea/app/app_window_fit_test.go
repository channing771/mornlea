//go:build darwin

package app

import "testing"

// sizingTestWindow 是 `fitFramebuffer` 的可控假窗口:尺寸与 `SetContentSize`
// 调用全可断言,其余方法嵌入包内共享的 `fakeInteractiveWindow`。
type sizingTestWindow struct {
	fakeInteractiveWindow
	contentWidth, contentHeight         int
	framebufferWidth, framebufferHeight int
	setWidth, setHeight                 int
	setCalls                            int
}

func (w *sizingTestWindow) ContentSize() (int, int) {
	return w.contentWidth, w.contentHeight
}
func (w *sizingTestWindow) FramebufferSize() (int, int) {
	return w.framebufferWidth, w.framebufferHeight
}
func (w *sizingTestWindow) SetContentSize(width, height int) {
	w.setCalls++
	w.setWidth, w.setHeight = width, height
}

// TestFitFramebufferNeverUpscales:帧缓冲小于目标分辨率时(1x 屏/小屏),
// `fitFramebuffer` 必须保持窗口现状,不得把内容放大到目标尺寸把窗口撑出屏幕。
func TestFitFramebufferNeverUpscales(t *testing.T) {
	window := &sizingTestWindow{
		contentWidth: 1280, contentHeight: 720,
		framebufferWidth: 1280, framebufferHeight: 720,
	}
	fitFramebuffer(window, 2560, 1440)
	if window.setCalls != 0 {
		t.Fatalf("fitFramebuffer 放大内容到 %dx%d,想要不调用 SetContentSize",
			window.setWidth, window.setHeight)
	}
}

// TestFitFramebufferShrinksOversizedFramebuffer:物理帧缓冲超过目标时
// 按比例收缩内容,使渲染分辨率不越出上限(超大缩放屏的兜底)。
func TestFitFramebufferShrinksOversizedFramebuffer(t *testing.T) {
	window := &sizingTestWindow{
		contentWidth: 2560, contentHeight: 1440,
		framebufferWidth: 5120, framebufferHeight: 2880,
	}
	fitFramebuffer(window, 2560, 1440)
	if window.setCalls != 1 || window.setWidth != 1280 || window.setHeight != 720 {
		t.Fatalf("fitFramebuffer 调用 SetContentSize %d 次,末次 (%d,%d),想要 1 次 (1280,720)",
			window.setCalls, window.setWidth, window.setHeight)
	}
}

// TestFitFramebufferTargetAlreadyMetIsNoop:帧缓冲恰好为目标分辨率时
// 不重复 `SetContentSize`(避免无意义的 Resize 与 `Poll`)。
func TestFitFramebufferTargetAlreadyMetIsNoop(t *testing.T) {
	window := &sizingTestWindow{
		contentWidth: 1280, contentHeight: 720,
		framebufferWidth: 2560, framebufferHeight: 1440,
	}
	fitFramebuffer(window, 2560, 1440)
	if window.setCalls != 0 {
		t.Fatalf("目标已满足仍调用 SetContentSize %d 次", window.setCalls)
	}
}

//go:build darwin

package devcapture

import (
	"image/color"
	"testing"
)

// TestBGRAToNRGBASwapsChannelsPerRow 钉住字节序交换与行进：2×2 输入的四个
// 像素各不相同，任何按单像素循环却忽略行推进的实现都会在第二行错位。
func TestBGRAToNRGBASwapsChannelsPerRow(t *testing.T) {
	pixels := []byte{
		10, 20, 30, 255, 40, 50, 60, 255, // 第一行
		70, 80, 90, 255, 100, 110, 120, 255, // 第二行
	}
	img := bgraToNRGBA(pixels, 2, 2)
	want := map[[2]int]color.NRGBA{
		{0, 0}: {R: 30, G: 20, B: 10, A: 255},
		{1, 0}: {R: 60, G: 50, B: 40, A: 255},
		{0, 1}: {R: 90, G: 80, B: 70, A: 255},
		{1, 1}: {R: 120, G: 110, B: 100, A: 255},
	}
	for pos, expected := range want {
		if got := img.NRGBAAt(pos[0], pos[1]); got != expected {
			t.Errorf("像素 (%d,%d) = %v，想要 %v", pos[0], pos[1], got, expected)
		}
	}
}

// TestBGRAToNRGBAPassesAlphaThrough 钉住 alpha 透传：窗口合成的 alpha 是
// 预乘语义，转换不做逐像素除法，原样交给 PNG；非 255 取值下交换仍只发生在
// B/R 两个通道。
func TestBGRAToNRGBAPassesAlphaThrough(t *testing.T) {
	pixels := []byte{1, 2, 3, 128}
	img := bgraToNRGBA(pixels, 1, 1)
	if got, want := (color.NRGBA)(img.NRGBAAt(0, 0)), (color.NRGBA{R: 3, G: 2, B: 1, A: 128}); got != want {
		t.Errorf("带 alpha 像素 = %v，想要 %v", got, want)
	}
}

//go:build darwin

package capture

import (
	"bytes"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"testing"
)

// gifFidelityFrames 返回草绿/牛肉红棕主导的合成帧：模拟牧场基线的真实色域，
// 固定调色板在此色域上损伤肉眼可见（青草地/粉泥土），自适应调色板必须保真。
func gifFidelityFrames() []*image.NRGBA {
	frames := make([]*image.NRGBA, 0, 2)
	for step := 0; step < 2; step++ {
		img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				offset := img.PixOffset(x, y)
				// 草绿基底带轻微明暗起伏（模拟光照），牛肉块居中。
				img.Pix[offset+0] = uint8(88 + (x+step)%5)
				img.Pix[offset+1] = uint8(140 + (y+step)%5)
				img.Pix[offset+2] = uint8(60 + (x+y+step)%4)
				img.Pix[offset+3] = 255
			}
		}
		for y := 6; y < 10; y++ {
			for x := 6; x < 10; x++ {
				offset := img.PixOffset(x, y)
				img.Pix[offset+0] = uint8(152 + (x+step)%4)
				img.Pix[offset+1] = uint8(76 + (y+step)%4)
				img.Pix[offset+2] = uint8(55 + (x+y)%3)
			}
		}
		frames = append(frames, img)
	}
	return frames
}

// TestGIFAdaptivePaletteBeatsFixedPaletteOnPastureColors 锁定自适应调色板语义：
// 同一帧 raw 经自适应编码再解码后，草绿/牛肉红棕与 raw 的通道差必须显著小于
// 固定调色板版本，且同输入逐字节确定输出。
func TestGIFAdaptivePaletteBeatsFixedPaletteOnPastureColors(t *testing.T) {
	frames := gifFidelityFrames()
	first, err := encodeGIF(frames)
	if err != nil {
		t.Fatalf("encodeGIF: %v", err)
	}
	second, err := encodeGIF(frames)
	if err != nil {
		t.Fatalf("encodeGIF: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("同输入两次编码输出不一致，想要逐字节确定")
	}
	adaptive, err := decodeGIF(first)
	if err != nil {
		t.Fatalf("decodeGIF: %v", err)
	}
	fixed, err := decodeGIF(mustEncodeGIFWithFixedPalette(t, frames))
	if err != nil {
		t.Fatalf("decodeGIF: %v", err)
	}
	adaptiveWorst := maxChannelDeltaToRaw(t, frames, adaptive)
	fixedWorst := maxChannelDeltaToRaw(t, frames, fixed)
	if adaptiveWorst >= fixedWorst {
		t.Fatalf("自适应最大通道差=%d，想要显著小于固定版本=%d", adaptiveWorst, fixedWorst)
	}
	if adaptiveWorst > 12 {
		t.Fatalf("自适应最大通道差=%d，想要草绿/牛肉保真（≤12）", adaptiveWorst)
	}
}

// TestGIFAdaptivePaletteIsDeterministicAndBounded 锁定调色板纪律：色数有界
// （≤256）、并列按色值升序决胜保证确定性。
func TestGIFAdaptivePaletteIsDeterministicAndBounded(t *testing.T) {
	frames := gifFidelityFrames()
	first := gifAdaptivePalette(frames)
	second := gifAdaptivePalette(frames)
	if len(first) == 0 || len(first) > 256 {
		t.Fatalf("调色板色数=%d，想要 1..256", len(first))
	}
	if len(first) != len(second) {
		t.Fatalf("两次取色色数=%d vs %d，想要一致", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("第 %d 色=%v vs %v，想要确定性并列决胜", index, first[index], second[index])
		}
	}
}

// mustEncodeGIFWithFixedPalette 用固定调色板编码作对照：只在测试内呈现旧损伤。
func mustEncodeGIFWithFixedPalette(t *testing.T, frames []*image.NRGBA) []byte {
	t.Helper()
	out := &gif.GIF{}
	for _, frame := range frames {
		paletted := image.NewPaletted(frame.Bounds(), palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, frame.Bounds(), frame, image.Point{})
		out.Image = append(out.Image, paletted)
		out.Delay = append(out.Delay, gifFrameDelay)
	}
	var buffer bytes.Buffer
	if err := gif.EncodeAll(&buffer, out); err != nil {
		t.Fatalf("固定调色板编码: %v", err)
	}
	return buffer.Bytes()
}

// maxChannelDeltaToRaw 返回解码帧相对 raw 的最大通道差。
func maxChannelDeltaToRaw(t *testing.T, raw, decoded []*image.NRGBA) int {
	t.Helper()
	if len(raw) != len(decoded) {
		t.Fatalf("帧数=%d vs %d，想要一致", len(raw), len(decoded))
	}
	worst := 0
	for index := range raw {
		if raw[index].Bounds() != decoded[index].Bounds() {
			t.Fatalf("第 %d 帧尺寸不一致", index)
		}
		for offset := 0; offset < len(raw[index].Pix); offset += 4 {
			for channel := 0; channel < 3; channel++ {
				delta := int(raw[index].Pix[offset+channel]) - int(decoded[index].Pix[offset+channel])
				if delta < 0 {
					delta = -delta
				}
				if delta > worst {
					worst = delta
				}
			}
		}
	}
	return worst
}

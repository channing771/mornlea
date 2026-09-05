//go:build darwin

package capture

// motion_break_burst_test.go：破碎 burst motion 演示的回归测试。演示产物只验
// 呈现（固定输入→固定 24 帧字节一致、可解码、帧数约定），链路正确性由
// 破碎 burst 的逐帧测试与 `RenderFrame` 接线测试承接。

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestBreakBurstMotionCaptures24FramesInTickOrder 钉住演示抓帧的帧约定：
// 24 帧、tick 逐帧 +1、同一固定序列两次抓取逐帧字节一致。
func TestBreakBurstMotionCaptures24FramesInTickOrder(t *testing.T) {
	capture := func(record *[]uint64) func(uint64) (*image.NRGBA, error) {
		return func(tick uint64) (*image.NRGBA, error) {
			*record = append(*record, tick)
			img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			img.Pix[0] = uint8(tick)
			return img, nil
		}
	}
	var firstTicks []uint64
	first, err := captureBreakBurstMotionFrames(capture(&firstTicks), breakBurstMotionTickBase)
	if err != nil {
		t.Fatalf("抓取 motion 帧序列: %v", err)
	}
	if len(first) != breakBurstMotionFrameCount {
		t.Fatalf("motion 帧数=%d，想要 %d", len(first), breakBurstMotionFrameCount)
	}
	for index, tick := range firstTicks {
		if want := breakBurstMotionTickBase + uint64(index); tick != want {
			t.Fatalf("第 %d 帧 tick=%d，想要 %d", index, tick, want)
		}
	}
	var secondTicks []uint64
	second, err := captureBreakBurstMotionFrames(capture(&secondTicks), breakBurstMotionTickBase)
	if err != nil {
		t.Fatalf("重抓 motion 帧序列: %v", err)
	}
	for index := range first {
		if !bytes.Equal(first[index].Pix, second[index].Pix) {
			t.Fatalf("第 %d 帧两次抓取字节不一致", index)
		}
	}
}

// TestBreakBurstMotionGIFDecodesTo24Frames 钉住编码产物可解码且帧数符合约定。
func TestBreakBurstMotionGIFDecodesTo24Frames(t *testing.T) {
	frames := make([]*image.NRGBA, 0, breakBurstMotionFrameCount)
	for index := range breakBurstMotionFrameCount {
		img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
		fill := color.NRGBA{R: uint8(index * 10), G: 120, B: 60, A: 255}
		for y := 0; y < 3; y++ {
			for x := 0; x < 4; x++ {
				img.SetNRGBA(x, y, fill)
			}
		}
		frames = append(frames, img)
	}
	data, err := encodeBreakBurstMotionGIF(frames)
	if err != nil {
		t.Fatalf("编码 motion GIF: %v", err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码 motion GIF: %v", err)
	}
	if len(decoded.Image) != breakBurstMotionFrameCount {
		t.Fatalf("解码帧数=%d，想要 %d", len(decoded.Image), breakBurstMotionFrameCount)
	}
	if decoded.Config.Width != 4 || decoded.Config.Height != 3 {
		t.Fatalf("解码尺寸=%dx%d，想要 4x3", decoded.Config.Width, decoded.Config.Height)
	}
}

// TestBreakBurstMotionGIFEncodingIsDeterministic 钉住固定输入→固定字节：
// 同一份 24 帧两次编码逐字节一致。
func TestBreakBurstMotionGIFEncodingIsDeterministic(t *testing.T) {
	frames := make([]*image.NRGBA, 0, breakBurstMotionFrameCount)
	for range breakBurstMotionFrameCount {
		frames = append(frames, image.NewNRGBA(image.Rect(0, 0, 4, 3)))
	}
	first, err := encodeBreakBurstMotionGIF(frames)
	if err != nil {
		t.Fatalf("首次编码 motion GIF: %v", err)
	}
	second, err := encodeBreakBurstMotionGIF(frames)
	if err != nil {
		t.Fatalf("重编码 motion GIF: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("同一 24 帧两次编码字节不一致（%d vs %d 字节）", len(first), len(second))
	}
}

// TestBreakBurstMotionGIFRefusesEmptyFrames 钉住空输入直接失败：
// 坏帧留在产物里只会制造假证据。
func TestBreakBurstMotionGIFRefusesEmptyFrames(t *testing.T) {
	if _, err := encodeBreakBurstMotionGIF(nil); err == nil {
		t.Fatal("空帧编码 motion GIF 想要报错，实际通过")
	}
}

// TestBreakBurstMotionDropIsDirtAtFixedBlock 钉住第 0 帧注入的合成掉落：
// 泥土物品（颜色经 `RenderFrame` 走 `ItemColor`）、合法 ID、锚定方块可还原。
func TestBreakBurstMotionDropIsDirtAtFixedBlock(t *testing.T) {
	upsert := breakBurstMotionDropUpsert()
	if err := upsert.Validate(); err != nil {
		t.Fatalf("合成掉落校验失败: %v", err)
	}
	if len(upsert.Drops) != 1 {
		t.Fatalf("合成掉落数=%d，想要 1", len(upsert.Drops))
	}
	drop := upsert.Drops[0]
	if drop.Item != core.ItemDirt {
		t.Fatalf("合成掉落物品=%d，想要泥土 %d", drop.Item, core.ItemDirt)
	}
	if !drop.ID.Valid() {
		t.Fatalf("合成掉落 ID=%+v 非法", drop.ID)
	}
	wantIndex, ok := world.ChunkBlockIndex(breakBurstMotionDropBlock)
	if !ok {
		t.Fatalf("合成掉落锚点 %+v 不在区块索引内", breakBurstMotionDropBlock)
	}
	if drop.BlockIndex != wantIndex {
		t.Fatalf("合成掉落块索引=%d，想要锚点还原值 %d", drop.BlockIndex, wantIndex)
	}
}

// TestBreakBurstMotionSceneStaysOutOfCaptureScenes 钉住演示场景不进正式表：
// 27 张 PNG 纪律与 `visual-check` 比对都不感知 motion 演示。
func TestBreakBurstMotionSceneStaysOutOfCaptureScenes(t *testing.T) {
	for _, scene := range captureScenes {
		lowered := strings.ToLower(scene.Name)
		if strings.Contains(lowered, "motion") || strings.Contains(lowered, "burst") {
			t.Fatalf("正式场景表混入演示场景 %q", scene.Name)
		}
	}
}

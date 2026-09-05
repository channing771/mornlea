//go:build darwin

package capture

import (
	"image"
	"testing"
)

// gifTestFrames 返回 3 张 8×8 合成帧：灰底 + 逐帧右移的红块（模拟死亡过渡
// 的逐帧变化），只测编解码与比对逻辑，不碰 GPU。
func gifTestFrames() []*image.NRGBA {
	frames := make([]*image.NRGBA, 0, 3)
	for step := 0; step < 3; step++ {
		img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				offset := img.PixOffset(x, y)
				img.Pix[offset+0] = 100
				img.Pix[offset+1] = 120
				img.Pix[offset+2] = 110
				img.Pix[offset+3] = 255
			}
		}
		for y := 2; y < 6; y++ {
			for x := step; x < step+3; x++ {
				offset := img.PixOffset(x, y)
				img.Pix[offset+0] = 200
				img.Pix[offset+1] = 40
				img.Pix[offset+2] = 30
			}
		}
		frames = append(frames, img)
	}
	return frames
}

func TestValidateGIFFramesRejectsOutOfBudgetBeforeCapture(t *testing.T) {
	for _, frames := range []int{0, -1, gifFrameBudget + 1, 100} {
		if err := validateGIFFrames(frames); err == nil {
			t.Fatalf("帧数 %d 被接受，想要在任何帧捕获之前拒绝", frames)
		}
	}
	for _, frames := range []int{1, 12, gifFrameBudget} {
		if err := validateGIFFrames(frames); err != nil {
			t.Fatalf("帧数 %d 被拒绝: %v", frames, err)
		}
	}
}

func TestGIFEncodeDecodeRoundtripKeepsFrames(t *testing.T) {
	frames := gifTestFrames()
	data, err := encodeGIF(frames)
	if err != nil {
		t.Fatalf("encodeGIF: %v", err)
	}
	decoded, err := decodeGIF(data)
	if err != nil {
		t.Fatalf("decodeGIF: %v", err)
	}
	if len(decoded) != len(frames) {
		t.Fatalf("解码帧数=%d，想要 %d", len(decoded), len(frames))
	}
	for index := range frames {
		if decoded[index].Bounds() != frames[index].Bounds() {
			t.Fatalf("第 %d 帧尺寸=%v，想要 %v", index, decoded[index].Bounds(), frames[index].Bounds())
		}
	}
}

func TestCompareGIFFramesUsesPerFrameDualThreshold(t *testing.T) {
	frames := gifTestFrames()
	data, err := encodeGIF(frames)
	if err != nil {
		t.Fatalf("encodeGIF: %v", err)
	}
	golden, err := decodeGIF(data)
	if err != nil {
		t.Fatalf("decodeGIF: %v", err)
	}
	repeat, err := decodeGIF(data)
	if err != nil {
		t.Fatalf("decodeGIF: %v", err)
	}
	if _, err := compareGIFFrames(repeat, golden, captureThresholds); err != nil {
		t.Fatalf("同源 GIF 比对失败: %v", err)
	}

	// 第 1 帧整体翻红：逐帧比对必须失败并指出帧号。
	changed := gifTestFrames()
	for i := range changed[1].Pix {
		if i%4 == 0 {
			changed[1].Pix[i] = 255
		}
	}
	changedData, err := encodeGIF(changed)
	if err != nil {
		t.Fatalf("encodeGIF: %v", err)
	}
	changedGIF, err := decodeGIF(changedData)
	if err != nil {
		t.Fatalf("decodeGIF: %v", err)
	}
	if _, err := compareGIFFrames(changedGIF, golden, captureThresholds); err == nil {
		t.Fatal("翻红帧通过了比对，想要失败")
	}

	// 帧数不一致直接失败，不逐帧比对。
	short, err := decodeGIF(mustEncodeGIF(t, frames[:2]))
	if err != nil {
		t.Fatalf("decodeGIF: %v", err)
	}
	if _, err := compareGIFFrames(short, golden, captureThresholds); err == nil {
		t.Fatal("帧数不一致通过了比对，想要失败")
	}
}

func mustEncodeGIF(t *testing.T, frames []*image.NRGBA) []byte {
	t.Helper()
	data, err := encodeGIF(frames)
	if err != nil {
		t.Fatalf("encodeGIF: %v", err)
	}
	return data
}

// TestCompareGIFAgainstGoldenRoundtripsFreshFrames 锁定比对管线的同量化语义：
// 实拍 raw 帧先经同一编码器往返量化再与基线比对——同源录制必须通过（若直接
// 比 raw 与量化基线，调色板损伤会让每一帧都超阈值）。
func TestCompareGIFAgainstGoldenRoundtripsFreshFrames(t *testing.T) {
	frames := gifTestFrames()
	goldenDir := t.TempDir()
	outDir := t.TempDir()
	if err := writeGIFFile(goldenDir, "roundtrip", mustEncodeGIF(t, frames)); err != nil {
		t.Fatalf("写入临时基线: %v", err)
	}
	if _, err := compareGIFAgainstGolden(goldenDir, outDir, "roundtrip", frames, captureThresholds); err != nil {
		t.Fatalf("同源实拍比对失败: %v", err)
	}
	if _, err := compareGIFAgainstGolden(goldenDir, outDir, "missing", frames, captureThresholds); err == nil {
		t.Fatal("缺失基线通过了比对，想要失败")
	}
}

// TestPassiveDeathGIFRegistryIsDecoupledFromScenes 锁定剧本注册表形状：四条
// 剧本（吃草/引诱/击杀/掉落）帧数有界、命名与 PNG 场景表不相交、Setup/Step
// 齐备——GIF 另起目录，绝不触碰场景表顺序与 PNG 基线。
func TestPassiveDeathGIFRegistryIsDecoupledFromScenes(t *testing.T) {
	want := map[string]int{"graze": 12, "lure": 16, "kill": 25, "beef-drop": 12}
	if len(passiveDeathGIFScripts) != len(want) {
		t.Fatalf("GIF 剧本=%d，想要 %d", len(passiveDeathGIFScripts), len(want))
	}
	sceneNames := make(map[string]struct{}, len(captureScenes))
	for _, scene := range captureScenes {
		sceneNames[scene.Name] = struct{}{}
	}
	for _, script := range passiveDeathGIFScripts {
		frames, ok := want[script.Name]
		if !ok {
			t.Fatalf("未知 GIF 剧本 %q", script.Name)
		}
		if script.Frames != frames {
			t.Fatalf("剧本 %s 帧数=%d，想要 %d", script.Name, script.Frames, frames)
		}
		if err := validateGIFFrames(script.Frames); err != nil {
			t.Fatalf("剧本 %s 帧预算: %v", script.Name, err)
		}
		if script.Setup == nil || script.Step == nil {
			t.Fatalf("剧本 %s 缺少 Setup 或 Step", script.Name)
		}
		if _, collides := sceneNames[script.Name]; collides {
			t.Fatalf("剧本 %s 与 PNG 场景表重名", script.Name)
		}
	}
}

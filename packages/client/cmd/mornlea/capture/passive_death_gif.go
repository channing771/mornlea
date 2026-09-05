package capture

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"sort"
)

// passiveDeathGoldenDir 是 GIF 动态基线的独立目录（相对仓库根）：与 PNG
// 场景表（`captureGoldenDir`）解耦，新增 GIF 不触碰 24 景顺序与 PNG 基线。
const passiveDeathGoldenDir = "testdata/visual-golden/passive-death"

// gifFrameBudget 是单条 GIF 基线的帧数上限（8fps×6s=48）：与 devcapture 的
// 录制上限纪律同源（总帧数有界），参数校验在首帧捕获之前。
const gifFrameBudget = 48

// gifFrameDelay 是 GIF 逐帧延迟（百分之一秒）：12 即约 8fps，与帧预算的
// 8fps×6s 同源。
const gifFrameDelay = 12

// validateGIFFrames 校验单条 GIF 的帧数：越界在任何帧捕获之前拒绝。
func validateGIFFrames(frames int) error {
	if frames < 1 || frames > gifFrameBudget {
		return fmt.Errorf("GIF 帧数 %d 越界，想要 1..%d", frames, gifFrameBudget)
	}
	return nil
}

// gifAdaptivePalette 按基线逐个构建自适应调色板：全部帧的 15 位量化色
// （每通道高 5 位）直方图取 Top-256，计数降序、同计数按量化色值升序决胜，
// 5 位回展 8 位。固定调色板是青草地/粉泥土的唯一成因（渲染器无辜），自适应
// 后 raw→gif 保真；同输入逐字节确定，不引入新依赖。
func gifAdaptivePalette(frames []*image.NRGBA) color.Palette {
	counts := make(map[uint32]int)
	for _, frame := range frames {
		for offset := 0; offset+4 <= len(frame.Pix); offset += 4 {
			key := uint32(frame.Pix[offset])>>3<<10 |
				uint32(frame.Pix[offset+1])>>3<<5 |
				uint32(frame.Pix[offset+2]>>3)
			counts[key]++
		}
	}
	keys := make([]uint32, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > 256 {
		keys = keys[:256]
	}
	palette := make(color.Palette, 0, len(keys))
	for _, key := range keys {
		r5 := uint32((key >> 10) & 0x1F)
		g5 := uint32((key >> 5) & 0x1F)
		b5 := uint32(key & 0x1F)
		palette = append(palette, color.RGBA{
			R: uint8(r5<<3 | r5>>2),
			G: uint8(g5<<3 | g5>>2),
			B: uint8(b5<<3 | b5>>2),
			A: 255,
		})
	}
	return palette
}

// encodeGIF 把逐帧 NRGBA 按标准库 `image/gif` 编码：逐基线自适应调色板 +
// Floyd-Steinberg 抖动，同输入逐字节确定；逐帧延迟固定为 `gifFrameDelay`。
func encodeGIF(frames []*image.NRGBA) ([]byte, error) {
	if err := validateGIFFrames(len(frames)); err != nil {
		return nil, err
	}
	palette := gifAdaptivePalette(frames)
	out := &gif.GIF{}
	for _, frame := range frames {
		paletted := image.NewPaletted(frame.Bounds(), palette)
		draw.FloydSteinberg.Draw(paletted, frame.Bounds(), frame, image.Point{})
		out.Image = append(out.Image, paletted)
		out.Delay = append(out.Delay, gifFrameDelay)
	}
	var buffer bytes.Buffer
	if err := gif.EncodeAll(&buffer, out); err != nil {
		return nil, fmt.Errorf("编码 GIF: %w", err)
	}
	return buffer.Bytes(), nil
}

// decodeGIF 解码 GIF 基线为逐帧 NRGBA：比对只读像素，不比较调色板与延迟。
func decodeGIF(data []byte) ([]*image.NRGBA, error) {
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解码 GIF: %w", err)
	}
	frames := make([]*image.NRGBA, 0, len(decoded.Image))
	for _, paletted := range decoded.Image {
		bounds := paletted.Bounds()
		frame := image.NewNRGBA(bounds)
		draw.Draw(frame, bounds, paletted, bounds.Min, draw.Src)
		frames = append(frames, frame)
	}
	return frames, nil
}

// compareGIFFrames 逐帧比对两份解码 GIF：帧数必须一致，每帧沿用 PNG 的
// `compareImages` + 双阈值；全部帧通过方为通过，返回最差帧的量化差异。
func compareGIFFrames(got, want []*image.NRGBA, threshold diffThreshold) (imageDiff, error) {
	if len(got) != len(want) {
		return imageDiff{}, fmt.Errorf("GIF 帧数不一致：实拍 %d，基线 %d", len(got), len(want))
	}
	var worst imageDiff
	for index := range got {
		diff, _, err := compareImages(got[index], want[index])
		if err != nil {
			return imageDiff{}, fmt.Errorf("第 %d 帧: %w", index, err)
		}
		if !diff.withinThreshold(threshold) {
			return diff, fmt.Errorf("第 %d 帧超出阈值：%s", index, diff)
		}
		worst.TotalPixels += diff.TotalPixels
		worst.DiffPixels += diff.DiffPixels
		if diff.MaxChannelDelta > worst.MaxChannelDelta {
			worst.MaxChannelDelta = diff.MaxChannelDelta
		}
	}
	if worst.TotalPixels > 0 {
		worst.DiffPixelRatio = float64(worst.DiffPixels) / float64(worst.TotalPixels)
	}
	return worst, nil
}

// compareGIFAgainstGolden 把实拍帧与 <goldenDir>/<name>.gif 基线逐帧比对：
// 实拍帧先经同一编码器往返量化（基线是量化后的像素，直接比 raw 必挂），再
// 逐帧解码比对（双阈值，全部帧通过方为通过）。
func compareGIFAgainstGolden(
	goldenDir, outDir, name string, frames []*image.NRGBA, threshold diffThreshold,
) (imageDiff, error) {
	goldenPath := filepath.Join(goldenDir, name+".gif")
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		return imageDiff{}, fmt.Errorf(
			"读取 GIF 基线 %s 失败（若是首次建立基线，先加 --update-golden）: %w",
			goldenPath, err)
	}
	want, err := decodeGIF(data)
	if err != nil {
		return imageDiff{}, fmt.Errorf("解码 GIF 基线 %s: %w", name, err)
	}
	// 同量化后再比：编码器（自适应调色板 + 抖动）是确定函数，同 raw 必得同
	// 量化帧；阈值裁的是录制漂移，不是量化损伤。
	roundTripped, err := decodeGIF(mustEncodeGIFFrames(frames))
	if err != nil {
		return imageDiff{}, fmt.Errorf("量化实拍帧: %w", err)
	}
	diff, err := compareGIFFrames(roundTripped, want, threshold)
	if err != nil {
		actual, encodeErr := encodeGIF(frames)
		if encodeErr != nil {
			return diff, err
		}
		actualPath := filepath.Join(outDir, name+"-actual.gif")
		if writeErr := os.WriteFile(actualPath, actual, 0o644); writeErr != nil {
			return diff, err
		}
		return diff, fmt.Errorf("超出阈值：%v（实拍见 %s）", err, actualPath)
	}
	return diff, nil
}

// mustEncodeGIFFrames 是测试与比对管线内的编码便捷入口：编码失败直接
// panic——调用方传入的都是已校验的内存帧，失败只可能是编码器 bug。
func mustEncodeGIFFrames(frames []*image.NRGBA) []byte {
	data, err := encodeGIF(frames)
	if err != nil {
		panic(fmt.Sprintf("capture: GIF 编码失败: %v", err))
	}
	return data
}

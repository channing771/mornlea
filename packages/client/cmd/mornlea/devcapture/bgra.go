//go:build darwin

package devcapture

import (
	"bytes"
	"errors"
	"image"
	"image/png"

	"github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
)

// bgraToNRGBA 把窗口合成捕获的 BGRA8 原始像素转成 PNG/GIF 编码所需的
// `*image.NRGBA`。
//
// 字节契约（`mornlea_client` 窗口捕获与离屏 readback 共用，见
// `packages/client/client` 的 `Window.Capture` 注释）：每像素 4 字节、32 位小端即
// B/G/R/A 字节序、自上而下、行紧凑无 padding，长度恰为 width*height*4；
// 契约未标注色彩空间。Go 消费侧因此只需交换 B/R 两个字节，行序无需翻转，
// 通道字节直拷、不做任何伽马换算——与 `packages/client/cmd/mornlea/capture` 侧 readback
// 转换同一口径：渲染管线产出的设备 RGB 原样进 PNG。
//
// alpha 有意按预乘值透传、不做逐像素除法：窗口合成画面实际不透明
// （alpha 恒 255），该取值下预乘与非预乘逐字一致；引入除法只会为不出现的
// 半透明取值增加成本与歧义。这与 `packages/client/cmd/mornlea/capture` 侧「alpha 恒写 255」
// 的取舍不同——离屏 readback 的 alpha 无管线约定，窗口合成的 alpha 则有
// 明确语义，透传即忠实。
//
// 有意不 import `packages/client/cmd/mornlea/capture` 复用其同名未导出助手：两侧契约来源
// 不同（离屏 readback vs 窗口合成捕获），为此登记跨包依赖或把助手提升为
// 共享导出面，都大于这十几行重复的成本（dev-capture design 的既定裁决）。
func bgraToNRGBA(pixels []byte, width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		src := pixels[y*width*4 : (y+1)*width*4]
		dst := img.Pix[y*img.Stride : (y+1)*img.Stride]
		for x := 0; x < width; x++ {
			s, d := x*4, x*4
			dst[d+0] = src[s+2] // R ← B
			dst[d+1] = src[s+1] // G
			dst[d+2] = src[s+0] // B ← R
			dst[d+3] = src[s+3] // A（预乘透传）
		}
	}
	return img
}

// fastPNG 是全包共用的 PNG 编码器：压缩级别取 png.BestSpeed。调试帧的
// 取舍是编码速度优先——默认压缩对 Retina 全分辨率（约 2784×1728，
// CGWindowListCreateImage 含窗口阴影包围盒的真实产出）单帧编码是秒级，
// BestSpeed 快数倍、产物约大 1.5-2 倍；localhost 调试图不进 golden 存档，
// 用体积换速度是录制约束（16-240 帧必须全部在总截止内完成）的前提。
// png.Encoder 在编码期只读自身字段、可变状态均为调用局部，/screenshot 与
// 录制两条路径并发调用安全。
var fastPNG = &png.Encoder{CompressionLevel: png.BestSpeed}

// encodePNG 编码一帧为 PNG 字节。/screenshot 与录制逐帧共用同一编码器，
// 两条路径的耗时特征与产物形态保持一致。
func encodePNG(img *image.NRGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := fastPNG.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// outcomeNRGBA 校验交付像素与尺寸一致后转为 `*image.NRGBA`。捕获桥契约
// 保证 `len(Pixels) == Width*Height*4`（违约在 `packages/client/client` 即 panic），
// 这里再核一次属纵深防御：坏帧按可观察错误收敛，绝不编码出与声明尺寸不符
// 的产物。
func outcomeNRGBA(outcome app.CaptureOutcome) (*image.NRGBA, error) {
	if outcome.Width <= 0 || outcome.Height <= 0 ||
		len(outcome.Pixels) != outcome.Width*outcome.Height*4 {
		return nil, errors.New("捕获交付的像素长度与尺寸不一致")
	}
	return bgraToNRGBA(outcome.Pixels, outcome.Width, outcome.Height), nil
}

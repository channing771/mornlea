// scripts/composite_grass_side.go 把内嵌 Pixel Perfection 材质包里的草方块侧面
// （textures/grass_side.png）合成成完全不透明的 16×16 图，并原位写回。
//
// 背景：Mornlea 图形客户端的内嵌默认材质直接采样自 Minetest 纹理包 Pixel
// Perfection 的 default/default_grass_side.png。该文件在 Minetest/Luanti 中是
// overlay 型纹理（与 dirt 用 ^ 合成后才显示），其自身只有顶部约 5 行完全不透明
// 的绿色草缘，其余像素 alpha 递减（下半部约 8 行 alpha 为 0）。而 Mornlea 的渲染
// 路径对每个材质层做直接采样：terrain.wgsl 的片段着色器对 alpha<0.5 的片段判
// discard，并且 `LayerGrassSide` 是不透明层（不在 `isCutoutLayer` 集合里，
// packages/client/assets/blocks.go），`applyPack` 只做像素替换、没有任何合成。于是草方块
// 侧面下半部被整段 discard，出现看穿/破洞，mip 降采样后远处整块侧面被丢弃。
//
// 修复：用直通 alpha 的 source-over（straight alpha source-over）把草缘 overlay
// 合成到本 pack 同目录的 dirt.png 之上，得到完全不透明的"泥土+草缘"侧面图。
//
// 算法（逐通道、8-bit 整数，out.A 恒为 255）：
//
//	out = (s*A + d*(255-A) + 127) / 255
//
// 其中 s 是草缘源像素、d 是泥土目标像素、A 是源像素 alpha；+127 是四舍五入的
// 取整规则（除以 255 前加一半），保证接近半值时向最近的整数收敛。每条通道独立
// 计算，R/G/B 互不串扰。alpha>=255 的像素（草缘顶部）退化为直接取源色，
// alpha=0 的像素（下半部）退化为直接取泥土色。
//
// 可复现性：脚本对同一对输入文件运行两次，输出文件必须逐字节一致（Go 的
// image/png 编码是确定性的；编码选项固定为默认 png.Encoder，不做任何压缩级别
// 调整）。运行方式：
//
//	go run scripts/composite_grass_side.go
//
// 输入文件路径相对于仓库根目录：packages/client/assets/packs/pixel_perfection/textures/
// 下的 grass_side.png 与 dirt.png。脚本校验两张图都是 16×16，否则报错退出。
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	dir     = "packages/client/assets/packs/pixel_perfection/textures/"
	texture = "grass_side.png"
	base    = "dirt.png"
	size    = 16
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "composite_grass_side: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 读取草缘 overlay 与泥土底图，并统一转成 16×16 的 8-bit RGBA。
	source, err := decodeRGBA(dir + texture)
	if err != nil {
		return fmt.Errorf("读取源图 %s: %w", texture, err)
	}
	dirt, err := decodeRGBA(dir + base)
	if err != nil {
		return fmt.Errorf("读取底图 %s: %w", base, err)
	}

	// 逐像素做 straight-alpha source-over 合成，输出 alpha 一律置为 255。
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < size*size; i++ {
		si, di, oi := i*4, i*4, i*4
		s := source.Pix[si : si+4]
		d := dirt.Pix[di : di+4]
		o := out.Pix[oi : oi+4]
		a := s[3]
		for ch := 0; ch < 3; ch++ {
			o[ch] = uint8((int(s[ch])*int(a) + int(d[ch])*(255-int(a)) + 127) / 255)
		}
		o[3] = 255
	}

	// 原位写回，保持与既有内嵌资产一致的默认 PNG 编码（确定性）。
	file, err := os.Create(dir + texture)
	if err != nil {
		return fmt.Errorf("打开目标 %s: %w", texture, err)
	}
	defer file.Close()
	if err := png.Encode(file, out); err != nil {
		return fmt.Errorf("编码 %s: %w", texture, err)
	}
	return file.Close()
}

// decodeRGBA 读取一张 PNG 并返回其 RGBA 像素（已经过颜色模型归一化到 8-bit）。
func decodeRGBA(path string) (*image.NRGBA, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoded, err := png.Decode(file)
	if err != nil {
		return nil, err
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != size || bounds.Dy() != size {
		return nil, fmt.Errorf("PNG 尺寸为 %dx%d，需要 %dx%d", bounds.Dx(), bounds.Dy(), size, size)
	}
	rgba := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			rgba.SetNRGBA(x, y, color.NRGBAModel.Convert(decoded.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA))
		}
	}
	return rgba, nil
}

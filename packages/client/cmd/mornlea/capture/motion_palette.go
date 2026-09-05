package capture

import (
	"image"
	"image/color"
	"slices"
)

// `motionPalette` 以固定 5-bit 直方图和确定性中位切分生成全片共享调色板。
// 只用于离线演示，避免 Plan9 抖色把服装与草地变成彩色颗粒；内存不随像素数增长。
func motionPalette(frames []*image.NRGBA) color.Palette {
	type bucket struct{ count, r, g, b uint64 }
	histogram := make([]bucket, 1<<15)
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y += 2 {
			for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x += 2 {
				pixel := frame.NRGBAAt(x, y)
				key := int(pixel.R>>3)<<10 | int(pixel.G>>3)<<5 | int(pixel.B>>3)
				h := &histogram[key]
				h.count++
				h.r += uint64(pixel.R)
				h.g += uint64(pixel.G)
				h.b += uint64(pixel.B)
			}
		}
	}
	type sample struct {
		pixel color.RGBA
		count uint64
	}
	points := make([]sample, 0, len(histogram))
	for _, h := range histogram {
		if h.count > 0 {
			points = append(points, sample{color.RGBA{uint8(h.r / h.count), uint8(h.g / h.count), uint8(h.b / h.count), 255}, h.count})
		}
	}
	if len(points) == 0 {
		return color.Palette{color.Black}
	}
	channel := func(s sample, axis int) uint8 {
		switch axis {
		case 0:
			return s.pixel.R
		case 1:
			return s.pixel.G
		default:
			return s.pixel.B
		}
	}
	boxes := [][]sample{points}
	for len(boxes) < 256 {
		chosen, axis, best := -1, 0, uint64(0)
		for i, box := range boxes {
			if len(box) < 2 {
				continue
			}
			var population uint64
			for _, p := range box {
				population += p.count
			}
			for a := 0; a < 3; a++ {
				lo, hi := uint8(255), uint8(0)
				for _, p := range box {
					v := channel(p, a)
					lo = min(lo, v)
					hi = max(hi, v)
				}
				score := uint64(hi-lo) * population
				if score > best {
					chosen, axis, best = i, a, score
				}
			}
		}
		if chosen < 0 {
			break
		}
		box := boxes[chosen]
		slices.SortStableFunc(box, func(a, b sample) int { return int(channel(a, axis)) - int(channel(b, axis)) })
		var total uint64
		for _, p := range box {
			total += p.count
		}
		var accumulated uint64
		split := 1
		for split < len(box) {
			accumulated += box[split-1].count
			if accumulated >= total/2 {
				break
			}
			split++
		}
		split = min(split, len(box)-1)
		boxes[chosen] = box[:split]
		boxes = append(boxes, box[split:])
	}
	result := make(color.Palette, 0, len(boxes))
	for _, box := range boxes {
		var r, g, b, n uint64
		for _, p := range box {
			n += p.count
			r += uint64(p.pixel.R) * p.count
			g += uint64(p.pixel.G) * p.count
			b += uint64(p.pixel.B) * p.count
		}
		result = append(result, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255})
	}
	return result
}

// `palettizeMotion` 用有界颜色查找表替代逐像素抖动，纯色区保持纯色。
func palettizeMotion(frame *image.NRGBA, palette color.Palette) *image.Paletted {
	out := image.NewPaletted(frame.Bounds(), palette)
	cache := make([]int16, 1<<15)
	for i := range cache {
		cache[i] = -1
	}
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			pixel := frame.NRGBAAt(x, y)
			key := int(pixel.R>>3)<<10 | int(pixel.G>>3)<<5 | int(pixel.B>>3)
			if cache[key] < 0 {
				cache[key] = int16(palette.Index(pixel))
			}
			out.SetColorIndex(x, y, uint8(cache[key]))
		}
	}
	return out
}

package hud

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

const (
	// 生命值 HUD：十颗空心和最多十颗填充爱心，不绘制背景面板。
	healthQuads        = healthSegmentCount * 2
	healthSegmentCount = 10
	healthHeartSize    = float32(16)
	healthHeartGap     = float32(1)
)

// heartFill 标识固定心形 cell 的填充状态。
type heartFill uint8

const (
	heartEmpty heartFill = iota
	heartHalf
	heartFull
)

// HealthOverlay 是服务端已确认的生命值。它是 render 本地值，由 app 从
// Predictor 的已确认镜像转换；Confirmed 为 false 时表示尚未收到权威状态，
// 渲染器不会画出任何生命值——绝不显示预测或陈旧的数值。
type HealthOverlay struct {
	Confirmed bool
	Value     uint8
}

// appendHealthBar 在 framebuffer 左下角绘制一排无背景的服务端确认爱心；
// 每颗两点，奇数值画半颗，打开背包不会改变其尺度或位置。
func appendHealthBar(dst *hotbarLayout, atlas render.GlyphSource, health HealthOverlay, width, height float32) {
	if !health.Confirmed || width <= 0 || height <= 0 {
		return
	}
	_ = atlas
	scale := hudScale(false, width, height)
	x := hudEdgeMargin * scale
	heartSize := healthHeartSize * scale
	heartGap := healthHeartGap * scale
	y := height - (hudEdgeMargin+healthHeartSize)*scale
	emptyUV := hotbarHeartUV(heartEmpty)
	for segment := range healthSegmentCount {
		dst.quads = append(dst.quads, hotbarInstance{
			X: x + float32(segment)*(heartSize+heartGap), Y: y,
			Width: heartSize, Height: heartSize,
			U0: emptyUV[0], V0: emptyUV[1], U1: emptyUV[2], V1: emptyUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
	value := min(health.Value, uint8(core.MaxHealth))
	filled := int(value) / 2
	fullUV := hotbarHeartUV(heartFull)
	for segment := range filled {
		dst.quads = append(dst.quads, hotbarInstance{
			X: x + float32(segment)*(heartSize+heartGap), Y: y,
			Width: heartSize, Height: heartSize,
			U0: fullUV[0], V0: fullUV[1], U1: fullUV[2], V1: fullUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
	if value%2 != 0 {
		halfUV := hotbarHeartUV(heartHalf)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x + float32(filled)*(heartSize+heartGap), Y: y,
			Width: heartSize, Height: heartSize,
			U0: halfUV[0], V0: halfUV[1], U1: halfUV[2], V1: halfUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
}

func hotbarHeartUV(fill heartFill) [4]float32 {
	return hotbarTextureUV(hotbarEmptyHeartColumn + int(fill))
}

func paintHotbarHeart(dst []byte, column int, fill heartFill) {
	for y := range hotbarTextureSize {
		for x := range hotbarTextureSize {
			if !hotbarHeartPixel(x, y) {
				continue
			}
			border := !hotbarHeartPixel(x-1, y) || !hotbarHeartPixel(x+1, y) ||
				!hotbarHeartPixel(x, y-1) || !hotbarHeartPixel(x, y+1)
			color := [4]byte{44, 20, 24, 255}
			if border {
				color = [4]byte{96, 28, 36, 255}
			} else if fill == heartFull || fill == heartHalf && x < hotbarTextureSize/2 {
				color = [4]byte{226, 42, 52, 255}
				if x <= 5 && y >= 4 && y <= 6 {
					color = [4]byte{255, 105, 112, 255}
				}
			}
			if fill != heartEmpty && border {
				color = [4]byte{128, 22, 30, 255}
			}
			offset := (y*hotbarTextureWidth + column*hotbarTextureSize + x) * 4
			copy(dst[offset:offset+4], color[:])
		}
	}
}

func paintHotbarBubble(dst []byte, column int, full bool) {
	for y := range hotbarTextureSize {
		for x := range hotbarTextureSize {
			if !hotbarBubblePixel(x, y) {
				continue
			}
			border := !hotbarBubblePixel(x-1, y) || !hotbarBubblePixel(x+1, y) ||
				!hotbarBubblePixel(x, y-1) || !hotbarBubblePixel(x, y+1)
			color := [4]byte{24, 52, 76, 255}
			if border {
				color = [4]byte{72, 148, 184, 255}
			} else if full {
				color = [4]byte{48, 172, 222, 255}
				if x <= 6 && y <= 7 {
					color = [4]byte{160, 232, 255, 255}
				}
			}
			offset := (y*hotbarTextureWidth + column*hotbarTextureSize + x) * 4
			copy(dst[offset:offset+4], color[:])
		}
	}
}

func hotbarBubblePixel(x, y int) bool {
	switch y {
	case 2:
		return x >= 6 && x <= 9
	case 3:
		return x >= 4 && x <= 11
	case 4, 5:
		return x >= 3 && x <= 12
	case 6, 7, 8, 9:
		return x >= 2 && x <= 13
	case 10, 11:
		return x >= 3 && x <= 12
	case 12:
		return x >= 4 && x <= 11
	case 13:
		return x >= 6 && x <= 9
	default:
		return false
	}
}
func hotbarHeartPixel(x, y int) bool {
	switch y {
	case 2:
		return x >= 2 && x <= 6 || x >= 9 && x <= 13
	case 3:
		return x >= 1 && x <= 14
	case 4, 5, 6, 7:
		return x >= 0 && x <= 15
	case 8:
		return x >= 1 && x <= 14
	case 9:
		return x >= 2 && x <= 13
	case 10:
		return x >= 3 && x <= 12
	case 11:
		return x >= 4 && x <= 11
	case 12:
		return x >= 5 && x <= 10
	case 13:
		return x >= 6 && x <= 9
	default:
		return false
	}
}

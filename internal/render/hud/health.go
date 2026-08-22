package hud

import "github.com/channing771/mornlea/internal/core"

const (
	// 生命值 HUD：十个槽位各自直接选择空、半或满心，不绘制背景面板。
	healthQuads        = healthSegmentCount
	healthSegmentCount = 10
	healthHeartSize    = float32(16)
	healthHeartGap     = float32(1)
	statusBarGap       = float32(4)
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

// appendHealthBar 以快捷栏左边沿为锚点绘制无背景的服务端确认爱心；关闭容器时
// 位于快捷栏上方，打开时移入快捷栏下方留白，不读取或推算任何游戏状态。
func appendHealthBar(dst *hotbarLayout, health HealthOverlay, open bool, width, height float32) {
	if !health.Confirmed || width <= 0 || height <= 0 {
		return
	}
	x, hotbarY, _, scale := hotbarRowBounds(open, width, height)
	heartSize := healthHeartSize * scale
	heartGap := healthHeartGap * scale
	y := hotbarY - (statusBarGap+healthHeartSize)*scale
	if open {
		y = hotbarY + (hotbarSlotSize+statusBarGap)*scale
	}
	value := min(health.Value, uint8(core.MaxHealth))
	for segment := range healthSegmentCount {
		fill := heartEmpty
		if segment < int(value)/2 {
			fill = heartFull
		} else if segment == int(value)/2 && value%2 != 0 {
			fill = heartHalf
		}
		uv := hotbarHeartUV(fill)
		dst.quads = append(dst.quads, hotbarInstance{
			X: x + float32(segment)*(heartSize+heartGap), Y: y,
			Width: heartSize, Height: heartSize,
			U0: uv[0], V0: uv[1], U1: uv[2], V1: uv[3],
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

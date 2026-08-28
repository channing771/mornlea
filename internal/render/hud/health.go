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

// appendHealthBar 从快捷栏左边缘向右绘制无背景的服务端确认爱心；关闭容器时
// 位于快捷栏上方，打开时移入快捷栏下方留白，不读取或推算任何游戏状态。
func appendHealthBar(dst *hotbarLayout, health HealthOverlay, open bool, width, height float32) {
	if !health.Confirmed || width <= 0 || height <= 0 {
		return
	}
	x, _, y, _, scale := statusBarBounds(open, width, height)
	heartSize := healthHeartSize * scale
	heartGap := healthHeartGap * scale
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
				// 左上叶内部的一小块高光：亮斑收进轮廓内 1 px，避免与描边
				// 混成一片，16 px 下也能读出体积。
				if x <= 4 && y >= 3 && y <= 4 {
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
				// 左上象限高光：气泡的透气感来自这块不对称亮斑，窗口收到
				// 轮廓内侧，边界像素仍由描边色统一。
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
	// 满幅圆盘：每行的左右界都关于 x=7.5 镜像，y=2/y=13 的短行把圆收圆。
	// 相比旧剪影外扩一圈填满 14 px 宽度，与心形鸡腿共用同一视觉字号；
	// 空/满两态只差配色，轮廓完全一致。
	switch y {
	case 2:
		return x >= 5 && x <= 10
	case 3:
		return x >= 3 && x <= 12
	case 4, 5:
		return x >= 2 && x <= 13
	case 6, 7, 8, 9:
		return x >= 1 && x <= 14
	case 10, 11:
		return x >= 2 && x <= 13
	case 12:
		return x >= 3 && x <= 12
	case 13:
		return x >= 5 && x <= 10
	default:
		return false
	}
}
func hotbarHeartPixel(x, y int) bool {
	// 剪影左右镜像、上宽下尖：两枚圆叶各占两行后在 y=5 合流，底部逐行收尖，
	// 并不上下对称。相比旧剪影去掉四行等宽的「方肩」，叶间裂口加深一行，
	// 心形在 16 px 下更易辨识；空/半/满三态共用同一轮廓，半心只按 x 中线换色。
	switch y {
	case 2:
		return x >= 2 && x <= 5 || x >= 10 && x <= 13
	case 3, 4:
		return x >= 1 && x <= 6 || x >= 9 && x <= 14
	case 5:
		return x >= 1 && x <= 14
	case 6, 7:
		return x >= 2 && x <= 13
	case 8:
		return x >= 3 && x <= 12
	case 9:
		return x >= 4 && x <= 11
	case 10:
		return x >= 5 && x <= 10
	case 11:
		return x >= 6 && x <= 9
	case 12:
		return x >= 7 && x <= 8
	default:
		return false
	}
}

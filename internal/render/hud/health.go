package hud

// health.go 是 HUD 图集的生命/氧气图标 painter：三枚心形 cell（空/半/满）与两枚
// 气泡 cell（空/满）的程序化像素源。心形与气泡的呈现已迁 WebView HUD 组件，但
// 图集的列布局是固定上传契约的一部分（`AtlasPixels` 把整张贴图交给渲染器），
// 这些 cell 因此继续随图集构建并上传，列下标不得移动。

const (
	// healthHeartSize 是状态行一格的 design px 边长，statusBarGap 是状态行之间
	// 的行堆叠间隙：两者都随状态行呈现迁往 WebView 组件的镜像常量，但打开态
	// 面板的垂直居中与高度约束（`openHUDHeight`/`openBottomStackTop`）仍按同一
	// 份两行状态栈构图预留空间，数值必须与前端逐值一致。
	healthHeartSize = float32(16)
	statusBarGap    = float32(4)
)

// heartFill 标识固定心形 cell 的填充状态。
type heartFill uint8

const (
	heartEmpty heartFill = iota
	heartHalf
	heartFull
)

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

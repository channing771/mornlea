package hud

import "github.com/channing771/mornlea/internal/core"

const (
	// 氧气 HUD 的合法最坏值是十个空槽叠加十个满气泡。
	oxygenQuads        = oxygenSegmentCount * 2
	oxygenSegmentCount = 10
	oxygenBarWidth     = oxygenSegmentCount*healthHeartSize + (oxygenSegmentCount-1)*healthHeartGap
)

// OxygenOverlay 是服务端已确认的氧气。它是 render 本地值，由 app 从 Predictor 的
// 已确认镜像转换；Confirmed 为 false 时表示尚未收到权威状态，渲染器不画任何氧气
// 元素——氧气是权威值，客户端绝不显示预测或陈旧的数值。
type OxygenOverlay struct {
	Confirmed bool
	Value     uint16
}

// appendOxygenBar 只在权威氧气耗损时，以快捷栏右边沿为锚点绘制十段气泡；
// 满氧与未确认值完全不占实例，打开容器只改变状态行位于快捷栏的上方或下方。
func appendOxygenBar(dst *hotbarLayout, oxygen OxygenOverlay, open bool, width, height float32) {
	if !oxygen.Confirmed || width <= 0 || height <= 0 {
		return
	}
	value := min(oxygen.Value, core.MaxOxygenTicks)
	if value >= core.MaxOxygenTicks {
		return
	}
	left, hotbarY, hotbarWidth, scale := hotbarRowBounds(open, width, height)
	barWidth := oxygenBarWidth * scale
	x := left + hotbarWidth - barWidth
	y := hotbarY - (statusBarGap+healthHeartSize)*scale
	if open {
		y = hotbarY + (hotbarSlotSize+statusBarGap)*scale
	}
	bubbleSize := healthHeartSize * scale
	bubbleGap := healthHeartGap * scale
	emptyUV := hotbarBubbleUV(false)
	for segment := range oxygenSegmentCount {
		dst.quads = append(dst.quads, hotbarInstance{
			X: x + float32(segment)*(bubbleSize+bubbleGap), Y: y,
			Width: bubbleSize, Height: bubbleSize,
			U0: emptyUV[0], V0: emptyUV[1], U1: emptyUV[2], V1: emptyUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
	filled := (int(value)*oxygenSegmentCount + int(core.MaxOxygenTicks) - 1) /
		int(core.MaxOxygenTicks)
	fullUV := hotbarBubbleUV(true)
	for segment := range filled {
		dst.quads = append(dst.quads, hotbarInstance{
			X: x + float32(segment)*(bubbleSize+bubbleGap), Y: y,
			Width: bubbleSize, Height: bubbleSize,
			U0: fullUV[0], V0: fullUV[1], U1: fullUV[2], V1: fullUV[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
}

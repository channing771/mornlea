package hud

import "github.com/channing771/mornlea/internal/core"

const (
	// 氧气 HUD 的十个槽位各自直接选择空或满气泡。
	oxygenQuads        = oxygenSegmentCount
	oxygenSegmentCount = 10
)

// OxygenOverlay 是服务端已确认的氧气。它是 render 本地值，由 app 从 Predictor 的
// 已确认镜像转换；Confirmed 为 false 时表示尚未收到权威状态，渲染器不画任何氧气
// 元素——氧气是权威值，客户端绝不显示预测或陈旧的数值。
type OxygenOverlay struct {
	Confirmed bool
	Value     uint16
}

// appendOxygenBar 只在权威氧气耗损时沿饥饿条右边缘绘制十段气泡；满氧与
// 未确认值不占实例，但永久次行仍由共享几何保留，打开容器只改变外扩方向。
func appendOxygenBar(dst *hotbarLayout, oxygen OxygenOverlay, open bool, width, height float32) {
	if !oxygen.Confirmed || width <= 0 || height <= 0 {
		return
	}
	value := min(oxygen.Value, core.MaxOxygenTicks)
	if value >= core.MaxOxygenTicks {
		return
	}
	_, right, _, y, scale := statusBarBounds(open, width, height)
	bubbleSize := healthHeartSize * scale
	bubbleGap := healthHeartGap * scale
	filled := (int(value)*oxygenSegmentCount + int(core.MaxOxygenTicks) - 1) /
		int(core.MaxOxygenTicks)
	for segment := range oxygenSegmentCount {
		uv := hotbarBubbleUV(segment < filled)
		// 从共享右边缘反推每格位置，避免缩放后末格右沿因累计加法漂移。
		x := right - float32(oxygenSegmentCount-segment)*bubbleSize -
			float32(oxygenSegmentCount-1-segment)*bubbleGap
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: y,
			Width: bubbleSize, Height: bubbleSize,
			U0: uv[0], V0: uv[1], U1: uv[2], V1: uv[3],
			Color: [4]float32{1, 1, 1, 1},
		})
	}
}

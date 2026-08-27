package hud

// eatingBarQuads 是进食进度条激活时追加的 quad 数上限：固定轨道加比例填充。
//
// 刻意不超过采掘条的 `miningBarQuads`+`miningWarningNotches`=5：进食条与采掘条
// 同锚点且互斥（采掘优先，见 `appendEatingBar`），布局的最坏情况 quad 数不变，
// scenario v19 已锁定的 `maxHotbarQuads`/`hotbarUploadBytes` 固定上传容量得以
// 保持（design D2）。
const eatingBarQuads = 2

// EatingOverlay 是客户端预测的进食进度。字段形态参照 `MiningOverlay`，但有两处
// 刻意差异：
//
//   - 没有 `Harvestable`：进食没有「可采/不可采」的二元状态，填充只有一种
//     固定颜色，因此既没有亮色末端标记（cap）也没有警示缺口（notch）；
//   - 进度是钳制到 0..1 的填充比例（float32 字段）而不是 ticks 二元组：权威侧
//     没有进食进度的 wire 字段（design D1），值来自 `client.EatingProgressTracker`
//     按帧间时长累积出的连续量，再量化回 ticks 只会丢精度。
//
// 与生命/氧气/饥饿不同，它不是权威确认值：纯呈现预测，只在输入中断时清零。
type EatingOverlay struct {
	Active   bool
	Progress float32
}

// eatingFillColor 是进食填充的固定暖金色：与 `miningHarvestableColor` 的绿、
// `miningBlockedColor` 的橙都拉开距离，玩家靠颜色即可分辨三条同锚点进度条
// （互斥保证它们永不同帧出现，颜色区分只为连续观看时的语义连贯）。
var eatingFillColor = [4]float32{0.92, 0.78, 0.42, 0.95}

// appendEatingBar 在永久预留的两行状态栈上方绘制进食进度条。
//
// 互斥判定在本函数内部：mining 激活时采掘优先，本函数一个实例都不追加——
// 调用方（`layoutInventory` 的关闭态分支）无需先做取舍，删掉这条短路会让
// 进食条与采掘条同帧叠加、突破既有最坏 quad 数。
//
// 几何全部复用 `appendMiningBar` 的既有常量：同一 `hotbarRowBounds` 居中、
// 同一 `miningBarWidth`/`miningBarHeight`/`miningBarGap` 与 `statusBarBounds`
// 上方锚定公式——`closedHUDHeight` 已为这条轨道预留高度，故这里不新增几何
// 常量、不改变 `hudScale`。进度先钳到 0..1 再取宽，超额值不会把填充推出轨道。
func appendEatingBar(dst *hotbarLayout, overlay EatingOverlay, mining MiningOverlay, width, height float32) {
	if !overlay.Active || mining.Active {
		return
	}
	left, _, totalWidth, scale := hotbarRowBounds(false, width, height)
	barWidth := miningBarWidth * scale
	barHeight := miningBarHeight * scale
	x := left + (totalWidth-barWidth)*0.5
	_, _, _, statusTop, _ := statusBarBounds(false, width, height)
	y := statusTop - (miningBarGap+miningBarHeight)*scale
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: y, Width: barWidth, Height: barHeight,
		Color: miningTrackColor,
	})
	fraction := min(max(overlay.Progress, 0), 1)
	if fraction <= 0 {
		return
	}
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: y, Width: barWidth * fraction, Height: barHeight,
		Color: eatingFillColor,
	})
}

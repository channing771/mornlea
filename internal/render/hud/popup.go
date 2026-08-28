package hud

import (
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/render"
)

// popup.go 实现选中栏位确认变化的物品名弹条：纯字形呈现（零 quad），文字与
// 截断约定和聊天行同源，锚点复用关闭态状态栈与采掘轨道的既有几何。

const (
	// popupDurationTicks 是弹条自确认变化所属权威 tick 起的持续窗口：硬切
	// 出现/消失，无淡入淡出，保证 golden 由 tick 而不是 wall-clock 决定。
	popupDurationTicks = 40
	// maxPopupRunes 是弹条可见 rune 上限（含省略号），与聊天行同一截断约定。
	maxPopupRunes = 32
	// popupGlyphs 是弹条最坏字形数：32 rune × 阴影/前景双层。弹条被容器打开
	// 抑制，与打开态 overlay 数量不同帧叠加，预算按关闭态分支计入。
	popupGlyphs = maxPopupRunes * 2
	// popupTrackGap 是弹条底边与采掘/进食进度轨道上沿的间隙（design px），
	// 与采掘条对状态行的间隙同构，保证两条元素同帧共存时互不遮挡。
	popupTrackGap = float32(6)
	// popupRowHeight 是关闭态高度预算中弹条行的 design px 高度：轨道上沿的
	// 6 px 间隙之外再预留一行文字空间。文字墨迹以行底为锚向上绘制，墨迹高度
	// 超出本行的部分由 `hudScale` 永久保留的 2×`hudEdgeMargin` 屏幕边距吸收
	// （响应式扫描测试见证任何正尺寸 framebuffer 内不越界）。
	popupRowHeight = float32(16)
)

// PopupOverlay 是物品名弹条的呈现输入，由应用层从已确认镜像组装：
//
//   - Text 是 `core.ItemDisplayName` 的结果；显示名缺省（空栏位、未注册物品）
//     时应用层置 Valid=false，渲染层不显示——「均缺省则不显示」。
//   - ShownAtTick 是确认变化所属的权威 tick；WorldTick 是本帧权威 tick。
//     可见性由 `(WorldTick - ShownAtTick) < popupDurationTicks` 判定，纯
//     tick 驱动，不读 wall-clock。
//   - 只消费已确认镜像与本地组装，零预测、零新协议字段。
type PopupOverlay struct {
	Text        string
	ShownAtTick uint64
	WorldTick   uint64
	Valid       bool
}

// visible 报告弹条是否处于 40 tick 窗口内。WorldTick 先于 ShownAtTick 属于
// 调用方缺陷，按不可见处理（无符号下自然溢出恰好给出该语义）。
func (overlay PopupOverlay) visible() bool {
	return overlay.Valid && overlay.WorldTick-overlay.ShownAtTick < popupDurationTicks
}

// popupVisibleText 把弹条文本截断到 `maxPopupRunes` 个 rune：超长时保留前
// 31 rune 并以省略号收尾，与聊天行的可见 rune 预算同构。
func popupVisibleText(text string) string {
	if utf8.RuneCountInString(text) <= maxPopupRunes {
		return text
	}
	visibleEnd := 0
	runes := 0
	for index := range text {
		if runes == maxPopupRunes-1 {
			visibleEnd = index
			break
		}
		runes++
	}
	return text[:visibleEnd] + "…"
}

// requestPopupText 为窗口内的可见弹条请求字形；容器打开抑制时不请求，避免
// 为不呈现的文本扩张字形图集。返回是否发生了请求，供 `Prepare` 决定是否
// 冲刷上传。
func requestPopupText(atlas render.GlyphSource, overlay PopupOverlay, open bool) bool {
	if open || !overlay.visible() {
		return false
	}
	visible := popupVisibleText(overlay.Text)
	if visible == "" {
		return false
	}
	atlas.Request(visible)
	return true
}

// appendPopupOverlay 在两行状态栈上方、进度轨道上沿之上 `popupTrackGap`
// design px 处绘制物品名弹条：快捷栏水平居中、阴影加前景双层 glyph、零 quad。
// 容器打开（open）抑制弹条——弹条锚在关闭态几何上，打开态还画会与浮动面板
// 层叠冲突（delta「容器与菜单抑制」；菜单相位由应用层不组装 Valid 弹条兜底）。
func appendPopupOverlay(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay PopupOverlay,
	open bool,
	width, height float32,
) {
	if open || width <= 0 || height <= 0 || !overlay.visible() {
		return
	}
	text := popupVisibleText(overlay.Text)
	if text == "" {
		return
	}
	left, _, totalWidth, scale := hotbarRowBounds(false, width, height)
	_, _, _, statusTop, _ := statusBarBounds(false, width, height)
	baseline := statusTop - (miningBarGap+miningBarHeight+popupTrackGap)*scale
	appendPopupText(dst, atlas, text, left+totalWidth*0.5, baseline, scale)
}

// appendPopupText 以基线锚定绘制双层居中文字：先阴影后前景，阴影向右下偏移
// 1 design px，与聊天文字同一套 pen 推进与 kerning 公式。
func appendPopupText(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	text string,
	centerX, baseline, scale float32,
) {
	width := popupTextWidth(atlas, text, scale)
	for pass := range 2 {
		penX := centerX - width*0.5
		previous := rune(0)
		index := 0
		for _, char := range text {
			if index > 0 {
				penX += atlas.Kern(previous, char) * scale
			}
			glyph := atlas.Glyph(char)
			offset := float32(0)
			color := textPrimaryFg
			if pass == 0 {
				offset = scale
				color = textPrimaryShadow
			}
			dst.glyphs = append(dst.glyphs, hotbarInstance{
				X:     penX + glyph.BearingX*scale + offset,
				Y:     baseline - glyph.BearingY*scale + offset,
				Width: glyph.Width * scale, Height: glyph.Height * scale,
				U0: glyph.U0, V0: glyph.V0, U1: glyph.U1, V1: glyph.V1, Color: color,
			})
			penX += glyph.Advance * scale
			previous = char
			index++
		}
	}
}

// popupTextWidth 按与绘制完全相同的 advance/kerning 公式测量文本宽度，保证
// 居中不因测量与绘制分叉而漂移。
func popupTextWidth(atlas render.GlyphSource, text string, scale float32) float32 {
	width := float32(0)
	previous := rune(0)
	index := 0
	for _, char := range text {
		if index > 0 {
			width += atlas.Kern(previous, char) * scale
		}
		width += atlas.Glyph(char).Advance * scale
		previous = char
		index++
	}
	return width
}

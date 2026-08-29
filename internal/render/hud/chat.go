package hud

import (
	"unicode/utf8"

	"github.com/channing771/mornlea/internal/render"
)

const (
	maxChatLines  = 6
	maxChatRunes  = 32
	maxChatQuads  = 2
	maxChatGlyphs = (maxChatLines + 1) * maxChatRunes * 2
	chatPadding   = float32(6)
	// 聊天与其他文字共用 24pt atlas；28px 可容纳 25px ink、1px 阴影并留出行隙。
	chatLineHeight      = float32(28)
	chatPanelGap        = float32(4)
	chatHealthClearance = float32(8)
	// `chatFitSlack` 比一个 framebuffer pixel 小 256 倍，正常尺寸不可见；它覆盖
	// 最多 32 个 glyph 的 float32 pen 累加及 panel 坐标乘加误差。
	chatFitSlack = float32(1.0 / 256.0)
)

// ChatOverlay 是调用方提供的固定聊天呈现值；Lines 只消费最后六条。
type ChatOverlay struct {
	Open  bool
	Input string
	Lines []string
}

func requestChatText(atlas render.GlyphSource, overlay ChatOverlay) bool {
	start := max(0, len(overlay.Lines)-maxChatLines)
	requested := false
	needsEllipsis := false
	for _, line := range overlay.Lines[start:] {
		truncated := requestVisibleChatText(atlas, line)
		requested = requested || line != ""
		needsEllipsis = needsEllipsis || truncated
	}
	if overlay.Open {
		truncated := requestVisibleChatText(atlas, overlay.Input)
		requested = requested || overlay.Input != ""
		needsEllipsis = needsEllipsis || truncated
	}
	if needsEllipsis {
		atlas.Request("…")
		requested = true
	}
	return requested
}

func requestVisibleChatText(atlas render.GlyphSource, text string) bool {
	if text == "" {
		return false
	}
	if utf8.RuneCountInString(text) <= maxChatRunes {
		atlas.Request(text)
		return false
	}
	visibleEnd := 0
	runes := 0
	for index := range text {
		if runes == maxChatRunes-1 {
			visibleEnd = index
			break
		}
		runes++
	}
	atlas.Request(text[:visibleEnd])
	return true
}

func appendChatOverlay(
	dst *hotbarLayout,
	atlas render.GlyphSource,
	overlay ChatOverlay,
	width, height float32,
) {
	if width <= 0 || height <= 0 {
		return
	}
	start := max(0, len(overlay.Lines)-maxChatLines)
	lines := overlay.Lines[start:]
	if len(lines) == 0 && !overlay.Open {
		return
	}
	_, _, _, statusTop, survivalScale := statusBarBounds(false, width, height)
	stackHeight := float32(0)
	if len(lines) > 0 {
		stackHeight = float32(len(lines))*chatLineHeight + 2*chatPadding
	}
	maxTextWidth := float32(0)
	for _, line := range lines {
		maxTextWidth = max(maxTextWidth, chatTextWidth(atlas, line, 1))
	}
	if overlay.Open {
		stackHeight += chatLineHeight + 2*chatPadding
		if len(lines) > 0 {
			stackHeight += chatPanelGap
		}
		maxTextWidth = max(maxTextWidth, chatTextWidth(atlas, overlay.Input, 1))
	}
	// 状态行仍使用 survival scale；聊天栈只能在此基础上继续缩小，
	// 且高度按本帧真实行数、宽度按最宽可见文本共同取界。
	scale := survivalScale
	if requiredHeight := stackHeight + chatHealthClearance; requiredHeight > 0 {
		if bound := max(statusTop-chatFitSlack, 0) / requiredHeight; bound < scale {
			scale = bound
		}
	}
	if requiredWidth := hudEdgeMargin + 2*chatPadding + maxTextWidth; requiredWidth > 0 {
		if bound := max(width-chatFitSlack, 0) / requiredWidth; bound < scale {
			scale = bound
		}
	}
	padding := chatPadding * scale
	lineHeight := chatLineHeight * scale
	x := hudEdgeMargin * scale
	// 聊天的整个面板栈从关闭态永久两行状态栈上方向上生长，不依赖是否恰好
	// 显示气泡实例，避免权威状态变化让已接受的聊天行突然被覆盖。
	bottom := statusTop - chatHealthClearance*scale
	inputHeight := lineHeight + 2*padding
	if overlay.Open {
		inputY := bottom - inputHeight
		dst.quads = append(dst.quads, hotbarInstance{
			X: x, Y: inputY, Width: chatTextWidth(atlas, overlay.Input, scale) + 2*padding,
			Height: inputHeight, Color: panelShadow,
		})
		appendChatText(dst, atlas, overlay.Input, x+padding, inputY+padding, scale)
		bottom = inputY - chatPanelGap*scale
	}
	if len(lines) == 0 {
		return
	}
	panelWidth := float32(0)
	for _, line := range lines {
		panelWidth = max(panelWidth, chatTextWidth(atlas, line, scale))
	}
	panelHeight := float32(len(lines))*lineHeight + 2*padding
	panelY := bottom - panelHeight
	dst.quads = append(dst.quads, hotbarInstance{
		X: x, Y: panelY, Width: panelWidth + 2*padding, Height: panelHeight,
		Color: panelSurface,
	})
	for index, line := range lines {
		appendChatText(dst, atlas, line, x+padding, panelY+padding+float32(index)*lineHeight, scale)
	}
}

func chatTextWidth(atlas render.GlyphSource, text string, scale float32) float32 {
	count := utf8.RuneCountInString(text)
	width := float32(0)
	previous := rune(0)
	index := 0
	for _, char := range text {
		if index == maxChatRunes {
			break
		}
		if index == maxChatRunes-1 && count > maxChatRunes {
			char = '…'
		}
		if index > 0 {
			width += atlas.Kern(previous, char) * scale
		}
		width += atlas.Glyph(char).Advance * scale
		previous = char
		index++
	}
	return width
}

func appendChatText(dst *hotbarLayout, atlas render.GlyphSource, text string, x, y, scale float32) {
	count := utf8.RuneCountInString(text)
	for pass := range 2 {
		penX := x
		previous := rune(0)
		index := 0
		for _, char := range text {
			if index == maxChatRunes {
				break
			}
			if index == maxChatRunes-1 && count > maxChatRunes {
				char = '…'
			}
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
				Y:     y + (chatLineHeight-glyph.BearingY)*scale + offset,
				Width: glyph.Width * scale, Height: glyph.Height * scale,
				U0: glyph.U0, V0: glyph.V0, U1: glyph.U1, V1: glyph.V1, Color: color,
			})
			penX += glyph.Advance * scale
			previous = char
			index++
		}
	}
}

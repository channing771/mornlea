package hud

import (
	"strconv"
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestOxygenBubbleUVUsesStableUICells 防止空、满气泡绕过 Task 2 固定 atlas cell。
func TestOxygenBubbleUVUsesStableUICells(t *testing.T) {
	for _, test := range []struct {
		name string
		full bool
		want [4]float32
	}{
		{"空气泡", false, hotbarTextureUV(hotbarEmptyBubbleColumn)},
		{"满气泡", true, hotbarTextureUV(hotbarFullBubbleColumn)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := hotbarBubbleUV(test.full); got != test.want {
				t.Fatalf("UV=%v，想要 %v", got, test.want)
			}
		})
	}
}

// TestOxygenBarUsesConfirmedCeilingSegments 防止未确认或满氧占实例、分段退回浮点
// 比例条，以及边界 tick 被向下取整。
func TestOxygenBarUsesConfirmedCeilingSegments(t *testing.T) {
	values := []uint16{0, 1, 30, 31, 60, 61, 90, 91, 120, 121, 150, 151,
		180, 181, 210, 211, 240, 241, 270, 271, core.MaxOxygenTicks - 1}
	for _, value := range values {
		t.Run("tick-"+strconv.Itoa(int(value)), func(t *testing.T) {
			var layout hotbarLayout
			appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: value}, false, 1280, 800)
			filled := (int(value)*oxygenSegmentCount + int(core.MaxOxygenTicks) - 1) /
				int(core.MaxOxygenTicks)
			if len(layout.quads) != oxygenSegmentCount || len(layout.glyphs) != 0 {
				t.Fatalf("氧气 %d：quads/glyphs=%d/%d，想要 %d/0", value,
					len(layout.quads), len(layout.glyphs), oxygenSegmentCount)
			}
			for index, bubble := range layout.quads {
				wantUV := hotbarBubbleUV(index < filled)
				if got := [4]float32{bubble.U0, bubble.V0, bubble.U1, bubble.V1}; got != wantUV {
					t.Fatalf("氧气 %d 实例 %d UV=%v，想要 %v", value, index, got, wantUV)
				}
				if bubble.Width != 16 || bubble.Height != 16 {
					t.Fatalf("氧气 %d 实例 %d=%+v，想要完整 16×16 cell", value, index, bubble)
				}
			}
		})
	}

	for _, test := range []struct {
		name    string
		overlay OxygenOverlay
	}{
		{"未确认", OxygenOverlay{Value: 10}},
		{"满氧", OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks}},
		{"越界高值按满氧处理", OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks + 50}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			appendOxygenBar(&layout, test.overlay, false, 1280, 800)
			if len(layout.quads) != 0 || len(layout.glyphs) != 0 {
				t.Fatalf("quads/glyphs=%d/%d，想要 0/0", len(layout.quads), len(layout.glyphs))
			}
		})
	}
}

// TestOxygenBarAnchorsToHotbar 防止气泡脱离饥饿条右边缘，或没有从快捷栏向外堆叠。
func TestOxygenBarAnchorsToHotbar(t *testing.T) {
	for _, test := range []struct {
		name string
		open bool
	}{
		{"关闭态位于饥饿上方", false},
		{"打开态位于饥饿下方", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			left, hotbarY, totalWidth, scale := hotbarRowBounds(test.open, 1280, 800)
			bubbleSize := healthHeartSize * scale
			bubbleGap := healthHeartGap * scale
			primaryY := hotbarY - (healthHeartSize+statusBarGap)*scale
			wantY := primaryY - (healthHeartSize+statusBarGap)*scale
			if test.open {
				primaryY = hotbarY + (hotbarSlotSize+statusBarGap)*scale
				wantY = primaryY + (healthHeartSize+statusBarGap)*scale
			}
			right := left + totalWidth
			var layout hotbarLayout
			appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: 1}, test.open, 1280, 800)
			if len(layout.quads) != oxygenSegmentCount {
				t.Fatalf("quads=%d，想要十个 resolved 气泡槽位", len(layout.quads))
			}
			for index, bubble := range layout.quads {
				wantX := right - float32(oxygenSegmentCount-index)*bubbleSize -
					float32(oxygenSegmentCount-1-index)*bubbleGap
				if bubble.X != wantX || bubble.Y != wantY {
					t.Fatalf("气泡 %d 锚点=(%v,%v)，想要饥饿右边缘外扩序列 (%v,%v)", index, bubble.X, bubble.Y, wantX, wantY)
				}
			}
		})
	}
}

// TestOxygenBarRejectsDegenerateFramebuffer 防止零尺寸 framebuffer 产生退化实例。
func TestOxygenBarRejectsDegenerateFramebuffer(t *testing.T) {
	for _, size := range [][2]float32{{0, 720}, {1280, 0}} {
		var layout hotbarLayout
		appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: 10}, false, size[0], size[1])
		if len(layout.quads) != 0 || len(layout.glyphs) != 0 {
			t.Fatalf("framebuffer %v：quads/glyphs=%d/%d，想要 0/0", size, len(layout.quads), len(layout.glyphs))
		}
	}
}

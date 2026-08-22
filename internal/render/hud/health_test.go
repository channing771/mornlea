package hud

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestHealthBarUsesConfirmedClampedHeartCells 防止未确认值泄漏、越界值溢出固定容量，
// 以及奇数生命退回半宽裁剪而不是完整半心 cell。
func TestHealthBarUsesConfirmedClampedHeartCells(t *testing.T) {
	for _, test := range []struct {
		name                string
		health              HealthOverlay
		wantEmpty, wantHalf int
		wantFull, wantTotal int
	}{
		{"未确认", HealthOverlay{Value: 12}, 0, 0, 0, 0},
		{"零血", HealthOverlay{Confirmed: true}, 10, 0, 0, 10},
		{"一点生命", HealthOverlay{Confirmed: true, Value: 1}, 9, 1, 0, 10},
		{"两点生命", HealthOverlay{Confirmed: true, Value: 2}, 9, 0, 1, 10},
		{"十九点生命", HealthOverlay{Confirmed: true, Value: 19}, 0, 1, 9, 10},
		{"满血", HealthOverlay{Confirmed: true, Value: core.MaxHealth}, 0, 0, 10, 10},
		{"越界值钳制", HealthOverlay{Confirmed: true, Value: 255}, 0, 0, 10, 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			appendHealthBar(&layout, test.health, false, 1280, 800)
			if len(layout.quads) != test.wantTotal || len(layout.glyphs) != 0 {
				t.Fatalf("quads/glyphs=%d/%d，想要 %d/0", len(layout.quads), len(layout.glyphs), test.wantTotal)
			}
			counts := map[[4]float32]int{}
			for _, quad := range layout.quads {
				if quad.Width != 16 || quad.Height != 16 {
					t.Fatalf("生命实例=%+v，想要完整 16×16 cell 且无背景或裁剪", quad)
				}
				counts[[4]float32{quad.U0, quad.V0, quad.U1, quad.V1}]++
			}
			if counts[hotbarHeartUV(heartEmpty)] != test.wantEmpty ||
				counts[hotbarHeartUV(heartHalf)] != test.wantHalf ||
				counts[hotbarHeartUV(heartFull)] != test.wantFull {
				t.Fatalf("空/半/满=%d/%d/%d，想要 %d/%d/%d", counts[hotbarHeartUV(heartEmpty)],
					counts[hotbarHeartUV(heartHalf)], counts[hotbarHeartUV(heartFull)],
					test.wantEmpty, test.wantHalf, test.wantFull)
			}
		})
	}
}

// TestHealthBarAnchorsToHotbar 防止生命行重新漂回 framebuffer 左下角，或打开容器
// 后继续覆盖快捷栏上方的可交互区域。
func TestHealthBarAnchorsToHotbar(t *testing.T) {
	for _, test := range []struct {
		name  string
		open  bool
		wantY float32
	}{
		{"关闭态位于快捷栏上方", false, 708},
		{"打开态位于快捷栏下方留白", true, 780},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: 1}, test.open, 1280, 800)
			if len(layout.quads) != healthSegmentCount {
				t.Fatalf("quads=%d，想要十个 resolved 心形槽位", len(layout.quads))
			}
			for index, heart := range layout.quads {
				wantX := float32(408 + index%10*17)
				if heart.X != wantX || heart.Y != test.wantY {
					t.Fatalf("爱心 %d 锚点=(%v,%v)，想要快捷栏左沿序列 (%v,%v)", index, heart.X, heart.Y, wantX, test.wantY)
				}
			}
		})
	}
}

// TestHealthBarRejectsDegenerateFramebuffer 防止零尺寸 framebuffer 产生退化实例。
func TestHealthBarRejectsDegenerateFramebuffer(t *testing.T) {
	for _, size := range [][2]float32{{0, 720}, {1280, 0}} {
		var layout hotbarLayout
		appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: 12}, false, size[0], size[1])
		if len(layout.quads) != 0 || len(layout.glyphs) != 0 {
			t.Fatalf("framebuffer %v：quads/glyphs=%d/%d，想要 0/0", size, len(layout.quads), len(layout.glyphs))
		}
	}
}

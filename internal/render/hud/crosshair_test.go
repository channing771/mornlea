package hud

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// TestCrosshairCentersOnViewportGeometryCenter 锁定准星与 viewport 几何中心的
// 重合关系：横臂 11×3、竖臂 3×11 design px，投影层相对前景层整体偏移
// +1,+1 design px，且先投影后前景共 4 个 quad、零 glyph。
func TestCrosshairCentersOnViewportGeometryCenter(t *testing.T) {
	const width, height = float32(1280), float32(800)
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, width, height)
	if len(got.quads) != crosshairQuads+4+core.HotbarSlots {
		t.Fatalf("关闭态空背包 quads=%d，想要准星 4 + 双层面板/选中 4 + 九格", len(got.quads))
	}
	if len(got.glyphs) != 0 {
		t.Fatalf("准星 glyphs=%d，想要 0（准星不进字形流）", len(got.glyphs))
	}

	crosshair := got.quads[:crosshairQuads]
	centerX, centerY := width*0.5, height*0.5
	for index, quad := range crosshair {
		wantWidth, wantHeight := crosshairArmLength, crosshairArmThickness
		if index%2 == 1 {
			wantWidth, wantHeight = crosshairArmThickness, crosshairArmLength
		}
		if quad.Width != wantWidth || quad.Height != wantHeight {
			t.Fatalf("准星臂 %d 尺寸=%v×%v，想要 %v×%v", index, quad.Width, quad.Height, wantWidth, wantHeight)
		}
		// 前景层中心与 viewport 几何中心重合；投影层整体偏移 +1,+1 design px。
		wantCX, wantCY := centerX, centerY
		wantColor := crosshairFg
		if index < 2 {
			wantColor = crosshairShadow
			wantCX += crosshairShadowOffset
			wantCY += crosshairShadowOffset
		}
		if gotCX, gotCY := quad.X+quad.Width*0.5, quad.Y+quad.Height*0.5; gotCX != wantCX || gotCY != wantCY {
			t.Fatalf("准星臂 %d 中心=(%v,%v)，想要 (%v,%v)", index, gotCX, gotCY, wantCX, wantCY)
		}
		if quad.Color != wantColor {
			t.Fatalf("准星臂 %d 颜色=%v，想要令牌 %v", index, quad.Color, wantColor)
		}
	}
}

// TestCrosshairScalesWithHudScale 锁定准星与快捷栏共用同一份 `hudScale`：
// 窄窗口下臂长按同一比例收缩，中心仍与 viewport 几何中心重合。
func TestCrosshairScalesWithHudScale(t *testing.T) {
	const width, height = float32(480), float32(300)
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, width, height)
	scale := hudScale(false, width, height)
	wantArm := crosshairArmLength * scale
	wantThick := crosshairArmThickness * scale
	for index, quad := range got.quads[:crosshairQuads] {
		wantWidth, wantHeight := wantArm, wantThick
		if index%2 == 1 {
			wantWidth, wantHeight = wantThick, wantArm
		}
		if quad.Width != wantWidth || quad.Height != wantHeight {
			t.Fatalf("缩放臂 %d=%v×%v，想要 %v×%v（scale=%v）", index, quad.Width, quad.Height, wantWidth, wantHeight, scale)
		}
		wantCX, wantCY := width*0.5, height*0.5
		if index < 2 {
			// 投影层随同一比例偏移。
			wantCX += crosshairShadowOffset * scale
			wantCY += crosshairShadowOffset * scale
		}
		if gotCX, gotCY := quad.X+quad.Width*0.5, quad.Y+quad.Height*0.5; gotCX != wantCX || gotCY != wantCY {
			t.Fatalf("缩放臂 %d 中心=(%v,%v)，想要 (%v,%v)", index, gotCX, gotCY, wantCX, wantCY)
		}
	}
}

// TestCrosshairEmitsNothingWhenHiddenOrDegenerate 见证三重门控：应用层相位
// 门控置 Visible=false、零宽与零高 framebuffer 都必须产生零实例。
func TestCrosshairEmitsNothingWhenHiddenOrDegenerate(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, test := range []struct {
		name          string
		overlay       CrosshairOverlay
		width, height float32
		wantQuads     int
	}{
		// 关闭态基线 = 双层面板/选中框 4 + 九格；不可见时准星贡献 0。
		{"菜单相位不可见", CrosshairOverlay{Visible: false}, 1280, 800, 4 + core.HotbarSlots},
		{"可见", CrosshairOverlay{Visible: true}, 1280, 800, crosshairQuads + 4 + core.HotbarSlots},
		{"零宽", CrosshairOverlay{Visible: true}, 0, 800, 0},
		{"零高", CrosshairOverlay{Visible: true}, 1280, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			got := layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
				MiningOverlay{}, EatingOverlay{}, test.overlay, test.width, test.height)
			if len(got.quads) != test.wantQuads {
				t.Fatalf("%s quads=%d，想要 %d", test.name, len(got.quads), test.wantQuads)
			}
		})
	}
}

// TestCrosshairPrecedesPanelInstances 锁定实例顺序契约：准星必须先于快捷栏
// 面板追加，容器打开时后画的面板才能遮挡二者重叠区域。
func TestCrosshairPrecedesPanelInstances(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, open := range []bool{false, true} {
		var chest *ChestOverlay
		if open {
			chest = fullChestOverlay()
		}
		var layout hotbarLayout
		got := layoutInventory(&layout, atlas, maxQuadTestInventory(), open, -1, nil, nil, chest,
			MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
		if len(got.quads) < crosshairQuads+4 {
			t.Fatalf("open=%t 实例不足：quads=%d", open, len(got.quads))
		}
		for index, quad := range got.quads[:crosshairQuads] {
			wantColor := crosshairFg
			if index < 2 {
				wantColor = crosshairShadow
			}
			if quad.Color != wantColor {
				t.Fatalf("open=%t quad %d 不是准星臂（颜色=%v）", open, index, quad.Color)
			}
		}
	}
}

// TestCrosshairDrawnWithoutConfirmedInventory 见证 `Prepare` 在物品镜像未确认
// 但 HUD 可见时的分支：快捷栏不布局，准星仍按相位门控绘制且位于实例流首位。
func TestCrosshairDrawnWithoutConfirmedInventory(t *testing.T) {
	renderer := newTestHotbarRenderer()
	if err := renderer.Prepare(
		core.Inventory{}, false, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{},
		HealthOverlay{}, OxygenOverlay{}, HungerOverlay{}, ChatOverlay{},
		PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 800, render.NewUploadBudget(1024),
	); err != nil {
		t.Fatalf("未确认物品 Prepare: %v", err)
	}
	if len(renderer.layout.quads) != crosshairQuads {
		t.Fatalf("未确认物品时 quads=%d，想要仅准星 %d", len(renderer.layout.quads), crosshairQuads)
	}
	if renderer.layout.quads[0].Color != crosshairShadow || renderer.layout.quads[3].Color != crosshairFg {
		t.Fatalf("准星双层顺序错误: %+v", renderer.layout.quads[:crosshairQuads])
	}
}

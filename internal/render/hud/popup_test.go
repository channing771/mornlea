package hud

import (
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// TestPopupOverlayDrawsShadowedCenteredText 锁定弹条几何：零 quad、阴影加前景
// 双层 glyph、水平居中于快捷栏行、文字基线锚在进度轨道上沿之上
// `popupTrackGap` design px 的弹条行底边。
func TestPopupOverlayDrawsShadowedCenteredText(t *testing.T) {
	const width, height = float32(1280), float32(800)
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, width, height)
	quadsBefore := len(layout.quads)
	appendPopupOverlay(&layout, atlas, PopupOverlay{
		Text: "中文", ShownAtTick: 100, WorldTick: 100, Valid: true,
	}, false, width, height)
	if len(layout.quads) != quadsBefore {
		t.Fatalf("弹条产生 %d 个 quad，想要 0（纯字形呈现）", len(layout.quads)-quadsBefore)
	}
	if len(layout.glyphs) != 4 {
		t.Fatalf("弹条 glyphs=%d，想要「中文」双层共 4", len(layout.glyphs))
	}

	_, _, totalWidth, scale := hotbarRowBounds(false, width, height)
	_, _, _, statusTop, _ := statusBarBounds(false, width, height)
	wantBaseline := statusTop - (miningBarGap+miningBarHeight+popupTrackGap)*scale
	centerX := (width-totalWidth)*0.5 + totalWidth*0.5
	// fakeNameTagGlyph：Advance=10、BearingY=10、BearingX=0，两个 rune 总 advance 20。
	wantPenX := centerX - 10
	for pass, wantColor := range map[int][4]float32{0: textPrimaryShadow, 1: textPrimaryFg} {
		// 阴影层相对前景层整体偏移 +1 design px（与 HUD 文字阴影同向）。
		offset := float32(0)
		if pass == 0 {
			offset = 1
		}
		for runeIndex := range 2 {
			glyph := layout.glyphs[pass*2+runeIndex]
			if glyph.Color != wantColor {
				t.Fatalf("pass %d rune %d 颜色=%v，想要令牌 %v", pass, runeIndex, glyph.Color, wantColor)
			}
			wantX := wantPenX + float32(runeIndex)*10 + offset
			wantY := wantBaseline - 10 + offset
			if glyph.X != wantX || glyph.Y != wantY {
				t.Fatalf("pass %d rune %d 位置=(%v,%v)，想要 (%v,%v)（基线 %v）",
					pass, runeIndex, glyph.X, glyph.Y, wantX, wantY, wantBaseline)
			}
		}
	}
	// 阴影层必须先于前景层（后画覆盖）。
	if layout.glyphs[0].Color != textPrimaryShadow || layout.glyphs[2].Color != textPrimaryFg {
		t.Fatalf("双层顺序错误: %+v", layout.glyphs)
	}
}

// TestPopupExpiresAfter40Ticks 锁定 40 权威 tick 窗口的硬切边界：窗口内最后一
// tick 仍可见，恰好在第 40 tick 时一个字形都不产生。
func TestPopupExpiresAfter40Ticks(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
	overlay := PopupOverlay{Text: "石头", ShownAtTick: 100, WorldTick: 139, Valid: true}
	appendPopupOverlay(&layout, atlas, overlay, false, 1280, 800)
	if len(layout.glyphs) != 4 {
		t.Fatalf("窗口内最后一 tick glyphs=%d，想要 4", len(layout.glyphs))
	}
	overlay.WorldTick = 140
	layout.glyphs = layout.glyphs[:0]
	appendPopupOverlay(&layout, atlas, overlay, false, 1280, 800)
	if len(layout.glyphs) != 0 {
		t.Fatalf("40 tick 后 glyphs=%d，想要 0", len(layout.glyphs))
	}
}

// TestPopupSuppressedByOpenContainerAndInvalidState 见证全部抑制与退化路径：
// 容器打开、Valid=false、空文本、零尺寸 framebuffer 都不产生字形。
func TestPopupSuppressedByOpenContainerAndInvalidState(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	for _, test := range []struct {
		name          string
		overlay       PopupOverlay
		open          bool
		width, height float32
	}{
		{"容器打开", PopupOverlay{Text: "石头", ShownAtTick: 1, WorldTick: 1, Valid: true}, true, 1280, 800},
		{"未确认变化", PopupOverlay{Text: "石头", Valid: false}, false, 1280, 800},
		{"空文本", PopupOverlay{Text: "", ShownAtTick: 1, WorldTick: 1, Valid: true}, false, 1280, 800},
		{"零宽", PopupOverlay{Text: "石头", ShownAtTick: 1, WorldTick: 1, Valid: true}, false, 0, 800},
		{"零高", PopupOverlay{Text: "石头", ShownAtTick: 1, WorldTick: 1, Valid: true}, false, 1280, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var layout hotbarLayout
			appendPopupOverlay(&layout, atlas, test.overlay, test.open, test.width, test.height)
			if len(layout.glyphs) != 0 {
				t.Fatalf("%s glyphs=%d，想要 0", test.name, len(layout.glyphs))
			}
		})
	}
}

// TestPopupTruncatesToBudget 锁定 32 rune 截断：超长文本保留前 31 rune 并以
// 省略号收尾，双层共 `popupGlyphs` 个字形，且必须落在固定 glyph 容量之内。
func TestPopupTruncatesToBudget(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{}, EatingOverlay{}, CrosshairOverlay{Visible: true}, 1280, 800)
	long := strings.Repeat("超", 40)
	appendPopupOverlay(&layout, atlas, PopupOverlay{
		Text: long, ShownAtTick: 1, WorldTick: 1, Valid: true,
	}, false, 1280, 800)
	if got := len(layout.glyphs); got != popupGlyphs {
		t.Fatalf("超长弹条 glyphs=%d，想要 %d（31 rune + 省略号，双层）", got, popupGlyphs)
	}
	if popupGlyphs > maxHotbarGlyphs {
		t.Fatalf("弹条字形预算 %d 超过固定容量 %d", popupGlyphs, maxHotbarGlyphs)
	}
}

// TestPopupCoexistsWithMiningBarWithoutOverlap 锁定弹条与采掘轨道同帧共存：
// 双方锚点相距 `popupTrackGap` design px，任何字形都不得与轨道矩形相交。
func TestPopupCoexistsWithMiningBarWithoutOverlap(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	var layout hotbarLayout
	got := layoutInventory(&layout, atlas, core.Inventory{}, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, EatingOverlay{},
		CrosshairOverlay{Visible: true}, 1280, 800)
	appendPopupOverlay(&got, atlas, PopupOverlay{
		Text: "石头", ShownAtTick: 1, WorldTick: 1, Valid: true,
	}, false, 1280, 800)
	var track hotbarInstance
	found := false
	for _, quad := range got.quads {
		if quad.Color == miningTrackColor {
			track, found = quad, true
		}
	}
	if !found {
		t.Fatal("夹具中找不到采掘轨道")
	}
	for index, glyph := range got.glyphs {
		if glyph.Y+glyph.Height > track.Y {
			t.Fatalf("弹条字形 %d 底边 %v 侵入采掘轨道上沿 %v: %+v", index, glyph.Y+glyph.Height, track.Y, glyph)
		}
	}
	// 锚点契约：文字基线钉在轨道上沿之上 popupTrackGap design px（墨迹相对
	// 基线的出入由字体度量决定，不改变锚点本身）。取前景层验锚点。
	_, _, _, statusTop, _ := statusBarBounds(false, 1280, 800)
	if wantBaseline := statusTop - (miningBarGap + miningBarHeight + popupTrackGap); got.glyphs[2].Y != wantBaseline-10 {
		t.Fatalf("弹条基线锚点漂移: fgY=%v，想要基线 %v", got.glyphs[2].Y, wantBaseline)
	}
}

// TestPopupRequestsVisibleText 见证字形请求与绘制同一份截断文本：可见时请求
// 截断结果，抑制或窗口外不请求。
func TestPopupRequestsVisibleText(t *testing.T) {
	source := &recordingChatGlyphSource{}
	if requestPopupText(source, PopupOverlay{
		Text: strings.Repeat("界", 40), ShownAtTick: 1, WorldTick: 1, Valid: true,
	}, false) {
		want := strings.Repeat("界", maxPopupRunes-1) + "…"
		if len(source.requests) != 1 || source.requests[0] != want {
			t.Fatalf("atlas requests=%q，想要 %q", source.requests, want)
		}
	} else {
		t.Fatal("可见弹条未请求字形")
	}
	source.requests = source.requests[:0]
	if requestPopupText(source, PopupOverlay{
		Text: "石头", ShownAtTick: 1, WorldTick: 1, Valid: true,
	}, true) {
		t.Fatal("容器打开抑制时仍请求字形")
	}
	if requestPopupText(source, PopupOverlay{
		Text: "石头", ShownAtTick: 1, WorldTick: 1, Valid: true,
	}, false) {
		if len(source.requests) != 1 || source.requests[0] != "石头" {
			t.Fatalf("atlas requests=%q，想要 [石头]", source.requests)
		}
	} else {
		t.Fatal("窗口内弹条未请求字形")
	}
	source.requests = source.requests[:0]
	if requestPopupText(source, PopupOverlay{
		Text: "石头", ShownAtTick: 1, WorldTick: 41, Valid: true,
	}, false) {
		t.Fatal("40 tick 窗口外仍请求字形")
	}
}

// TestPrepareLayoutsPopupGlyphs 见证 `Prepare` 的弹条接线：字形请求、布局与
// FrameStreams 前缀一致，弹条不产生任何 quad。
func TestPrepareLayoutsPopupGlyphs(t *testing.T) {
	renderer := newTestHotbarRenderer()
	if err := renderer.Prepare(
		core.Inventory{}, true, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{},
		HealthOverlay{}, OxygenOverlay{}, HungerOverlay{}, ChatOverlay{},
		PopupOverlay{Text: "石头", ShownAtTick: 1, WorldTick: 1, Valid: true},
		CrosshairOverlay{Visible: true}, 1280, 800, render.NewUploadBudget(1024),
	); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(renderer.layout.glyphs) == 0 {
		t.Fatal("Prepare 后弹条没有产生字形")
	}
	_, quads, glyphs := renderer.FrameStreams()
	if len(quads) != len(renderer.layout.quads)*hotbarInstanceBytes ||
		len(glyphs) != len(renderer.layout.glyphs)*hotbarInstanceBytes {
		t.Fatalf("FrameStreams 前缀与布局实例不一致: quads=%d glyphs=%d", len(quads), len(glyphs))
	}
}

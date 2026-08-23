package hud

import (
	"math"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

func TestChatOverlayShowsSixEventsAndInputWithinFixedCapacity(t *testing.T) {
	line := strings.Repeat("界", 33)
	lines := []string{line, line, line, line, line, line, line}
	renderer := &HotbarRenderer{
		atlas: newFakeNameTagAtlas(),
		layout: hotbarLayout{
			quads:  make([]hotbarInstance, 0, maxHotbarQuads),
			glyphs: make([]hotbarInstance, 0, maxHotbarGlyphs),
		},
		upload: make([]byte, hotbarUploadBytes),
	}
	if err := renderer.Prepare(
		core.Inventory{}, false, false, -1, nil, nil, MiningOverlay{}, HealthOverlay{}, OxygenOverlay{}, HungerOverlay{},
		ChatOverlay{Open: true, Input: line, Lines: lines},
		1280, 720, render.NewUploadBudget(1<<20),
	); err != nil {
		t.Fatal(err)
	}
	if len(renderer.layout.quads) != 2 || len(renderer.layout.glyphs) != 448 {
		t.Fatalf("chat layout quads=%d glyphs=%d", len(renderer.layout.quads), len(renderer.layout.glyphs))
	}
	if len(renderer.layout.quads) > maxHotbarQuads || len(renderer.layout.glyphs) > maxHotbarGlyphs {
		t.Fatalf("chat layout exceeds fixed capacity")
	}
}

func TestChatOverlayHotPathAllocations(t *testing.T) {
	source := &chatCountingGlyphSource{}
	renderer := &HotbarRenderer{
		atlas: source,
		layout: hotbarLayout{
			quads:  make([]hotbarInstance, 0, maxHotbarQuads),
			glyphs: make([]hotbarInstance, 0, maxHotbarGlyphs),
		},
		upload: make([]byte, hotbarUploadBytes),
	}
	overlay := ChatOverlay{
		Open:  true,
		Input: "@阿木 挖石头",
		Lines: []string{"Chen → 阿木：挖石头", "系统：未找到伙伴 阿树"},
	}
	budget := render.NewUploadBudget(1024)
	prepare := func() {
		if err := renderer.Prepare(
			core.Inventory{}, false, false, -1, nil, nil, MiningOverlay{}, HealthOverlay{}, OxygenOverlay{}, HungerOverlay{},
			overlay, 1280, 720, budget,
		); err != nil {
			panic(err)
		}
	}
	prepare()
	if allocations := testing.AllocsPerRun(1000, prepare); allocations != 0 {
		t.Fatalf("chat overlay allocations=%v", allocations)
	}
	source.flushes = 0
	prepare()
	if source.flushes != 1 {
		t.Fatalf("chat overlay atlas flushes=%d want=1", source.flushes)
	}
}

func TestChatOverlayRequestsOnlyVisibleRunes(t *testing.T) {
	source := &recordingChatGlyphSource{}
	linePrefix := strings.Repeat("甲", maxChatRunes-1)
	inputPrefix := strings.Repeat("丁", maxChatRunes-1)
	overlay := ChatOverlay{
		Open:  true,
		Input: inputPrefix + "戊己",
		Lines: []string{linePrefix + "乙丙"},
	}
	if !requestChatText(source, overlay) {
		t.Fatal("visible chat text was not requested")
	}
	if want := []string{linePrefix, inputPrefix, "…"}; !reflect.DeepEqual(source.requests, want) {
		t.Fatalf("atlas requests=%q want=%q", source.requests, want)
	}
}

func TestChatOverlayGlyphInkDoesNotOverlapOrEscapePanels(t *testing.T) {
	atlas := newFakeNameTagAtlas()
	atlas.tofu = render.Glyph{Advance: 10, BearingY: 25, Width: 8, Height: 24}
	layout := hotbarLayout{
		quads:  make([]hotbarInstance, 0, maxChatQuads),
		glyphs: make([]hotbarInstance, 0, 14),
	}
	appendChatOverlay(&layout, atlas, ChatOverlay{
		Open: true, Input: "输",
		Lines: []string{"甲", "乙", "丙", "丁", "戊", "己"},
	}, 1280, 720)
	if len(layout.quads) != 2 || len(layout.glyphs) != 14 {
		t.Fatalf("chat geometry quads/glyphs=%d/%d want=2/14", len(layout.quads), len(layout.glyphs))
	}
	assertChatGlyphPairInsidePanel(t, layout.glyphs[0:2], layout.quads[0])
	for line := range maxChatLines {
		start := 2 + line*2
		pair := layout.glyphs[start : start+2]
		assertChatGlyphPairInsidePanel(t, pair, layout.quads[1])
		if line == 0 {
			continue
		}
		previousShadow := layout.glyphs[start-2]
		currentForeground := pair[1]
		if previousShadow.Y+previousShadow.Height > currentForeground.Y {
			t.Fatalf("history lines %d/%d overlap: previousBottom=%v currentTop=%v",
				line-1, line, previousShadow.Y+previousShadow.Height, currentForeground.Y)
		}
	}
	if historyBottom, inputTop := layout.quads[1].Y+layout.quads[1].Height, layout.quads[0].Y; historyBottom > inputTop {
		t.Fatalf("history/input panels overlap: historyBottom=%v inputTop=%v", historyBottom, inputTop)
	}
}

func TestChatOverlayStaysInFramebufferAndAboveClosedSurvivalStatus(t *testing.T) {
	sizes := []struct {
		name          string
		width, height float32
	}{
		{"regular", 640, 360},
		{"short", 640, 300},
		{"small", 240, 40},
		{"narrow", 17, 800},
		{"flat", 800, 17},
		{"minimum_hud_margin", 16, 16},
		{"one_pixel", 1, 1},
	}
	line := strings.Repeat("界", maxChatRunes)
	lines := []string{line, line, line, line, line, line}
	for _, size := range sizes {
		for _, open := range []bool{false, true} {
			name := size.name + "/closed"
			if open {
				name = size.name + "/open"
			}
			t.Run(name, func(t *testing.T) {
				atlas := newFakeNameTagAtlas()
				atlas.tofu = render.Glyph{Advance: 10, BearingY: 25, Width: 8, Height: 24}
				layout := hotbarLayout{
					quads:  make([]hotbarInstance, 0, healthQuads+oxygenQuads+hungerQuads+maxChatQuads),
					glyphs: make([]hotbarInstance, 0, maxChatGlyphs),
				}
				appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: 12}, false, size.width, size.height)
				appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks / 2}, false, size.width, size.height)
				appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, false, size.width, size.height)
				statusCount := len(layout.quads)
				if statusCount != healthQuads+oxygenQuads+hungerQuads {
					t.Fatalf("status quads=%d want=%d", statusCount, healthQuads+oxygenQuads+hungerQuads)
				}
				_, hotbarY, _, survivalScale := hotbarRowBounds(false, size.width, size.height)
				primaryTop := hotbarY - (statusBarGap+healthHeartSize)*survivalScale
				statusTop := primaryTop - (statusBarGap+healthHeartSize)*survivalScale
				for index, quad := range layout.quads {
					wantTop := primaryTop
					if index >= healthQuads && index < healthQuads+oxygenQuads {
						wantTop = statusTop
					}
					if quad.Y != wantTop {
						t.Fatalf("status quad %d top=%v want reserved stack row %v", index, quad.Y, wantTop)
					}
				}

				appendChatOverlay(&layout, atlas, ChatOverlay{Open: open, Input: line, Lines: lines}, size.width, size.height)
				chatPanels := layout.quads[statusCount:]
				wantPanels := 1
				wantGlyphs := len(lines) * maxChatRunes * 2
				if open {
					wantPanels++
					wantGlyphs += maxChatRunes * 2
				}
				if len(chatPanels) != wantPanels || len(layout.glyphs) != wantGlyphs {
					t.Fatalf("chat panels/glyphs=%d/%d want=%d/%d",
						len(chatPanels), len(layout.glyphs), wantPanels, wantGlyphs)
				}
				for index, panel := range chatPanels {
					assertChatInstanceInFramebuffer(t, "panel", index, panel, size.width, size.height)
				}
				for index, glyph := range layout.glyphs {
					assertChatInstanceInFramebuffer(t, "glyph", index, glyph, size.width, size.height)
				}
				for statusIndex, status := range layout.quads[:statusCount] {
					for panelIndex, panel := range chatPanels {
						if chatInstancesOverlap(status, panel) {
							t.Fatalf("status quad %d overlaps chat panel %d: status=%+v panel=%+v",
								statusIndex, panelIndex, status, panel)
						}
					}
					for glyphIndex, glyph := range layout.glyphs {
						if chatInstancesOverlap(status, glyph) {
							t.Fatalf("status quad %d overlaps chat glyph %d: status=%+v glyph=%+v",
								statusIndex, glyphIndex, status, glyph)
						}
					}
				}
				if size.width == 640 && size.height == 360 {
					history := chatPanels[len(chatPanels)-1]
					wantHistoryHeight := (maxChatLines*chatLineHeight + 2*chatPadding) * survivalScale
					if history.Height != wantHistoryHeight {
						t.Fatalf("regular history height=%v want survival-scale height %v",
							history.Height, wantHistoryHeight)
					}
					lowerPanel := chatPanels[0]
					wantBottom := statusTop - chatHealthClearance*survivalScale
					if bottom := lowerPanel.Y + lowerPanel.Height; bottom != wantBottom {
						t.Fatalf("regular chat bottom=%v want shared status clearance anchor %v", bottom, wantBottom)
					}
				}
			})
		}
	}
}

// 杀死变异：删除或缩小亚像素余量、恢复只留一个 float32 ULP，会让真实字体在临界尺寸越界。
func TestChatOverlayEmbeddedFontStaysStrictlyInsideFitThresholds(t *testing.T) {
	atlas := newEmbeddedChatGlyphAtlas(t, "界i")

	for _, witness := range []struct {
		name          string
		char          string
		width, height float32
	}{
		{name: "wide_cjk", char: "界", width: 44, height: 40},
		{name: "narrow_latin", char: "i", width: 109, height: 40},
	} {
		t.Run(witness.name, func(t *testing.T) {
			assertEmbeddedChatStrictBounds(t, atlas, witness.char, witness.width, witness.height, true)
		})
	}

	for _, scan := range []struct {
		name, char   string
		centerWidth  int
		centerHeight int
	}{
		{name: "cjk", char: "界", centerWidth: 44, centerHeight: 40},
		{name: "latin", char: "i", centerWidth: 109, centerHeight: 40},
	} {
		t.Run(scan.name+"_scan", func(t *testing.T) {
			for width := scan.centerWidth - 2; width <= scan.centerWidth+2; width++ {
				for height := scan.centerHeight - 2; height <= scan.centerHeight+2; height++ {
					for _, open := range []bool{false, true} {
						assertEmbeddedChatStrictBounds(t, atlas, scan.char, float32(width), float32(height), open)
					}
				}
			}
		})
	}
}

func assertEmbeddedChatStrictBounds(
	t *testing.T,
	atlas render.GlyphSource,
	char string,
	width, height float32,
	open bool,
) {
	t.Helper()
	line := strings.Repeat(char, maxChatRunes)
	lines := []string{line, line, line, line, line, line}
	layout := hotbarLayout{
		quads:  make([]hotbarInstance, 0, healthQuads+oxygenQuads+hungerQuads+maxChatQuads),
		glyphs: make([]hotbarInstance, 0, maxChatGlyphs),
	}
	appendHealthBar(&layout, HealthOverlay{Confirmed: true, Value: 12}, false, width, height)
	appendOxygenBar(&layout, OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks / 2}, false, width, height)
	appendHungerBar(&layout, HungerOverlay{Confirmed: true, Value: core.MaxHunger}, false, width, height)
	statusCount := len(layout.quads)
	appendChatOverlay(&layout, atlas, ChatOverlay{Open: open, Input: line, Lines: lines}, width, height)

	panels := layout.quads[statusCount:]
	wantPanels := 1
	wantGlyphs := maxChatLines * maxChatRunes * 2
	if open {
		wantPanels++
		wantGlyphs += maxChatRunes * 2
	}
	if len(panels) != wantPanels || len(layout.glyphs) != wantGlyphs {
		t.Fatalf("%vx%v open=%t panels/glyphs=%d/%d want=%d/%d",
			width, height, open, len(panels), len(layout.glyphs), wantPanels, wantGlyphs)
	}
	for index, panel := range panels {
		assertChatInstanceInFramebuffer(t, "panel", index, panel, width, height)
	}
	for index, glyph := range layout.glyphs {
		assertChatInstanceInFramebuffer(t, "glyph", index, glyph, width, height)
		panel := panels[len(panels)-1]
		if open && index < maxChatRunes*2 {
			panel = panels[0]
		}
		if glyph.X < panel.X || glyph.Y < panel.Y ||
			glyph.X+glyph.Width > panel.X+panel.Width || glyph.Y+glyph.Height > panel.Y+panel.Height {
			t.Fatalf("%vx%v open=%t glyph %d escapes panel: glyph=%+v panel=%+v",
				width, height, open, index, glyph, panel)
		}
	}
	for statusIndex, status := range layout.quads[:statusCount] {
		for panelIndex, panel := range panels {
			if chatInstancesOverlap(status, panel) {
				t.Fatalf("%vx%v open=%t status %d overlaps panel %d: status=%+v panel=%+v",
					width, height, open, statusIndex, panelIndex, status, panel)
			}
		}
		for glyphIndex, glyph := range layout.glyphs {
			if chatInstancesOverlap(status, glyph) {
				t.Fatalf("%vx%v open=%t status %d overlaps glyph %d: status=%+v glyph=%+v",
					width, height, open, statusIndex, glyphIndex, status, glyph)
			}
		}
	}
}

func newEmbeddedChatGlyphAtlas(t *testing.T, text string) *render.GlyphAtlas {
	t.Helper()
	atlas, err := render.NewGlyphAtlasWithSink(chatGlyphSink{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(atlas.Release)
	atlas.Request(text)
	deadline := time.Now().Add(2 * time.Second)
	for _, char := range text {
		for atlas.Glyph(char).Slot == 0 {
			if time.Now().After(deadline) {
				t.Fatalf("production glyph %q was not ready", char)
			}
			if err := atlas.FlushUploads(render.NewUploadBudget(1024)); err != nil {
				t.Fatalf("flush production glyph %q: %v", char, err)
			}
			runtime.Gosched()
		}
	}
	return atlas
}

type chatGlyphSink struct{}

func (chatGlyphSink) WriteGlyphRect(uint32, uint32, uint32, uint32, []byte) {}

func assertChatInstanceInFramebuffer(
	t *testing.T,
	kind string,
	index int,
	instance hotbarInstance,
	width, height float32,
) {
	t.Helper()
	for _, value := range [...]float32{instance.X, instance.Y, instance.Width, instance.Height} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			t.Fatalf("chat %s %d contains non-finite geometry: %+v", kind, index, instance)
		}
	}
	if instance.X < 0 || instance.Y < 0 || instance.Width < 0 || instance.Height < 0 ||
		instance.X+instance.Width > width || instance.Y+instance.Height > height {
		t.Fatalf("chat %s %d escapes %vx%v framebuffer: %+v", kind, index, width, height, instance)
	}
}

func chatInstancesOverlap(a, b hotbarInstance) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width &&
		a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}

func assertChatGlyphPairInsidePanel(t *testing.T, pair []hotbarInstance, panel hotbarInstance) {
	t.Helper()
	if len(pair) != 2 {
		t.Fatalf("glyph pair len=%d want=2", len(pair))
	}
	foreground, shadow := pair[1], pair[0]
	if foreground.Y < panel.Y || shadow.Y+shadow.Height > panel.Y+panel.Height {
		t.Fatalf("glyph ink outside panel: foregroundTop=%v shadowBottom=%v panel=[%v,%v]",
			foreground.Y, shadow.Y+shadow.Height, panel.Y, panel.Y+panel.Height)
	}
}

type chatCountingGlyphSource struct {
	allocationGlyphSource
	flushes int
}

func (source *chatCountingGlyphSource) FlushUploads(*render.UploadBudget) error {
	source.flushes++
	return nil
}

type recordingChatGlyphSource struct {
	allocationGlyphSource
	requests []string
}

func (source *recordingChatGlyphSource) Request(text string) {
	source.requests = append(source.requests, text)
}

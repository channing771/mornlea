package hud

import (
	"math"
	"strings"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

func TestHotbarBufferRegionsDoNotOverlap(t *testing.T) {
	if hotbarQuadOffset%256 != 0 || hotbarGlyphOffset%256 != 0 {
		t.Fatalf("buffer offset 未按 256 字节对齐: quad=%d glyph=%d", hotbarQuadOffset, hotbarGlyphOffset)
	}
	quadEnd := hotbarQuadOffset + hotbarQuadSize
	if hotbarGlyphOffset < quadEnd {
		t.Fatalf("glyph offset=%d 落入 quad 区间 [%d,%d)", hotbarGlyphOffset, hotbarQuadOffset, quadEnd)
	}
}

// Mutation killed: reallocating layout or upload storage per frame would make
// the warmed HUD path allocate.
func TestHotbarPrepareReusesLayoutAndUploadStorage(t *testing.T) {
	source := &allocationGlyphSource{}
	renderer := &HotbarRenderer{
		atlas: source,
		layout: hotbarLayout{
			quads:  make([]hotbarInstance, 0, maxHotbarQuads),
			glyphs: make([]hotbarInstance, 0, maxHotbarGlyphs),
		},
		upload: make([]byte, hotbarUploadBytes),
	}
	inventory := fullTestInventory()
	health := HealthOverlay{Confirmed: true, Value: 7}
	// 氧气取未满值：只有这样预热路径才会真的走到氧气条，"零每帧分配、零新上传缓冲"
	// 这条断言才对它成立。传零值（未确认）会让氧气条整条被跳过、断言退化成空转。
	oxygen := OxygenOverlay{Confirmed: true, Value: 120}
	// 饥饿取奇数值：预热路径必须走到半格分支，否则「零每帧分配」对它只是空转。
	hunger := HungerOverlay{Confirmed: true, Value: 13}
	budget := render.NewUploadBudget(1024)
	if err := renderer.Prepare(inventory, true, true, 3, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, health, oxygen, hunger, ChatOverlay{}, 1280, 720, budget); err != nil {
		t.Fatalf("warm Prepare: %v", err)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		source.requestCount = 0
		if err := renderer.Prepare(inventory, true, true, 3, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, health, oxygen, hunger, ChatOverlay{}, 1280, 720, budget); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("warmed hotbar Prepare allocations=%v want=0", allocations)
	}
}

// TestHotbarPrepareClosedMiningEncodesLayeredFeedback 验证 `Prepare` 的关闭态输出
// 实际包含双层快捷栏与不可采采掘形状；删掉任一层或 warning notch 都会改变实例前缀。
func TestHotbarPrepareClosedMiningEncodesLayeredFeedback(t *testing.T) {
	renderer := &HotbarRenderer{
		atlas: &allocationGlyphSource{},
		layout: hotbarLayout{
			quads:  make([]hotbarInstance, 0, maxHotbarQuads),
			glyphs: make([]hotbarInstance, 0, maxHotbarGlyphs),
		},
		upload: make([]byte, hotbarUploadBytes),
	}
	if err := renderer.Prepare(
		core.Inventory{}, true, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, EatingOverlay{},
		HealthOverlay{}, OxygenOverlay{}, HungerOverlay{}, ChatOverlay{}, 1280, 800, render.NewUploadBudget(1024),
	); err != nil {
		t.Fatalf("关闭态 Prepare: %v", err)
	}

	const wantQuads = 2 + 2 + core.HotbarSlots + 2 + 3
	if got := len(renderer.layout.quads); got != wantQuads {
		t.Fatalf("关闭态采掘实例=%d，想要双层面板、双层选中、九格、轨道/填充和三个缺口共 %d", got, wantQuads)
	}
	panelCount, notchCount := 0, 0
	for _, quad := range renderer.layout.quads {
		if quad.Width > 432 && quad.Height > 48 {
			panelCount++
		}
		if quad.Width == 6 && quad.Height == 12 {
			notchCount++
		}
	}
	if panelCount != 2 || notchCount != 3 {
		t.Fatalf("关闭态面板=%d、warning notch=%d，想要 2/3", panelCount, notchCount)
	}
}

// TestHotbarFixedUploadLayoutMatchesScenarioVersion 把 Hotbar HUD 的**固定上传
// 布局**钉成数值断言。
//
// 这三个数不是内部细节：bounded-benchmark-workload 主规格用「固定 GPU 上传
// 布局、offset 与每帧写入字节数是否移动」来判定 benchmark scenario 要不要升版
// （v15→v16、v17→v18 与 v18→v19 都是因它而升）。没有这条断言，改动 HUD 布局时无人会
// 注意到 scenario 身份已经该动了——而那正是 v16 加氧气条那次发生过的事
// （quad 236→238 恰好没跨过 256 字节对齐边界，offset 与总容量才没变）。
//
// 数值随 HUD 结构增长是正常的；改动本测试的期望值时必须同时判定 scenario
// 版本要不要升，并把结论写进变更产物。
func TestHotbarFixedUploadLayoutMatchesScenarioVersion(t *testing.T) {
	for _, test := range []struct {
		name      string
		got, want int
	}{
		{"quad 容量", maxHotbarQuads, 267},
		{"glyph 容量", maxHotbarGlyphs, 700},
		{"glyph offset", hotbarGlyphOffset, 13312},
		{"总容量", hotbarUploadBytes, 46912},
	} {
		if test.got != test.want {
			t.Errorf("%s=%d，想要固定容量 %d", test.name, test.got, test.want)
		}
	}
}

// TestResponsiveHotbarPrepareKeepsEveryInstanceInFramebuffer 防止联合布局只缩放
// 快捷栏，遗漏状态行、采掘轨道或打开态最大容器 overlay。
func TestResponsiveHotbarPrepareKeepsEveryInstanceInFramebuffer(t *testing.T) {
	for _, size := range [][2]uint32{{1280, 720}, {640, 360}, {240, 40}, {17, 800}, {800, 17}, {16, 16}, {1, 1}} {
		for _, open := range []bool{false, true} {
			renderer := newTestHotbarRenderer()
			var chest *ChestOverlay
			mining := MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}
			if open {
				chest = fullChestOverlay()
			}
			if err := renderer.Prepare(maxQuadTestInventory(), true, open, 5, nil, nil, chest, mining, EatingOverlay{},
				HealthOverlay{Confirmed: true, Value: 7},
				OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1},
				HungerOverlay{Confirmed: true, Value: core.MaxHunger},
				ChatOverlay{}, size[0], size[1], render.NewUploadBudget(1024)); err != nil {
				t.Fatalf("framebuffer %v open=%v Prepare: %v", size, open, err)
			}
			for index, quad := range renderer.layout.quads {
				if !finiteRectangle(quad) || quad.X < 0 || quad.Y < 0 || quad.X+quad.Width > float32(size[0]) || quad.Y+quad.Height > float32(size[1]) {
					t.Fatalf("framebuffer %v open=%v quad %d 越界: %+v", size, open, index, quad)
				}
			}
			for index, glyph := range renderer.layout.glyphs {
				if !finiteRectangle(glyph) || glyph.X < 0 || glyph.Y < 0 || glyph.X+glyph.Width > float32(size[0]) || glyph.Y+glyph.Height > float32(size[1]) {
					t.Fatalf("framebuffer %v open=%v glyph %d 越界: %+v", size, open, index, glyph)
				}
			}
		}
	}

	for _, size := range [][2]uint32{{0, 720}, {1280, 0}, {0, 0}} {
		renderer := newTestHotbarRenderer()
		if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, nil, nil, fullChestOverlay(), MiningOverlay{}, EatingOverlay{},
			HealthOverlay{Confirmed: true, Value: 7}, OxygenOverlay{Confirmed: true, Value: 1},
			HungerOverlay{}, ChatOverlay{}, size[0], size[1], render.NewUploadBudget(1024)); err != nil {
			t.Fatalf("framebuffer %v Prepare: %v", size, err)
		}
		if len(renderer.layout.quads) != 0 || len(renderer.layout.glyphs) != 0 {
			t.Fatalf("framebuffer %v quads/glyphs=%d/%d，想要 0/0", size,
				len(renderer.layout.quads), len(renderer.layout.glyphs))
		}
	}
}

// TestHotbarMaximumBranchesAndEncodingContract 同时见证互斥分支容量、48-byte
// 编码、256-byte 对齐与 `FrameStreams` 的实际实例前缀。
func TestHotbarMaximumBranchesAndEncodingContract(t *testing.T) {
	if healthQuads != 10 || oxygenQuads != 10 || hungerQuads != 20 || hotbarInstanceBytes != 48 {
		t.Fatalf("health/oxygen/hunger/instance=%d/%d/%d/%d，想要 10/10/20/48",
			healthQuads, oxygenQuads, hungerQuads, hotbarInstanceBytes)
	}
	if hotbarViewportOffset%256 != 0 || hotbarQuadOffset%256 != 0 || hotbarGlyphOffset%256 != 0 ||
		hotbarViewportOffset+hotbarViewportBytes > hotbarQuadOffset ||
		hotbarQuadOffset+hotbarQuadSize > hotbarGlyphOffset ||
		hotbarGlyphOffset+hotbarGlyphSize != hotbarUploadBytes {
		t.Fatalf("上传区间异常: viewport=%d+%d quad=%d+%d glyph=%d+%d total=%d",
			hotbarViewportOffset, hotbarViewportBytes, hotbarQuadOffset, hotbarQuadSize,
			hotbarGlyphOffset, hotbarGlyphSize, hotbarUploadBytes)
	}

	chatLine := strings.Repeat("中", maxChatRunes)
	chat := ChatOverlay{Open: true, Input: chatLine,
		Lines: []string{chatLine, chatLine, chatLine, chatLine, chatLine, chatLine}}
	renderer := newTestHotbarRenderer()
	if err := renderer.Prepare(maxQuadTestInventory(), true, false, -1, nil, nil, nil,
		MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}, EatingOverlay{},
		HealthOverlay{Confirmed: true, Value: core.MaxHealth},
		OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1},
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat,
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("关闭最大分支 Prepare: %v", err)
	}
	closedWant := closedHotbarQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(renderer.layout.quads) != closedWant || closedWant != 96 || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("关闭分支 quads=%d，想要 96 且不超过 %d", len(renderer.layout.quads), maxHotbarQuads)
	}

	// 打开最大分支改由箱子见证（十条配方行删除后箱子 83 是最大 overlay）；
	// 合成视图的最坏组合（满 3×3 + 产物 + 来源高亮 + 满背包磨损）另起一段锁定。
	if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, nil, nil, fullChestOverlay(), MiningOverlay{}, EatingOverlay{},
		HealthOverlay{Confirmed: true, Value: core.MaxHealth},
		OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1},
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat,
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("打开最大分支 Prepare: %v", err)
	}
	openWant := openInventoryQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(renderer.layout.quads) != openWant || len(renderer.layout.quads) != 257 || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("较大打开分支 quads=%d，想要 257 且不超过固定上限 %d", len(renderer.layout.quads), maxHotbarQuads)
	}
	if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, fullCraftingOverlay(), nil, nil, MiningOverlay{}, EatingOverlay{},
		HealthOverlay{Confirmed: true, Value: core.MaxHealth},
		OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1},
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat,
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("打开合成最大分支 Prepare: %v", err)
	}
	craftingWant := openInventoryPanelQuads + 2 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + craftingQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(renderer.layout.quads) != craftingWant || craftingWant != 206 || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("打开合成分支 quads=%d，想要 206 且不超过固定上限 %d", len(renderer.layout.quads), maxHotbarQuads)
	}
	// maxQuadTestInventory 的九格工具数量为 1 不出数字，只有 27 个背包格贡献
	// 两位数量（各 4 实例）。
	if len(renderer.layout.glyphs) != (core.InventorySlots-core.HotbarSlots)*4+craftingGlyphs+maxChatGlyphs {
		t.Fatalf("打开合成分支 glyphs=%d，想要 %d", len(renderer.layout.glyphs),
			(core.InventorySlots-core.HotbarSlots)*4+craftingGlyphs+maxChatGlyphs)
	}
	viewport, quads, glyphs := renderer.FrameStreams()
	if len(viewport) != hotbarViewportBytes || len(quads) != len(renderer.layout.quads)*hotbarInstanceBytes ||
		len(glyphs) != len(renderer.layout.glyphs)*hotbarInstanceBytes {
		t.Fatalf("实际前缀 viewport/quads/glyphs=%d/%d/%d，实例=%d/%d 固定 quad 区=%d",
			len(viewport), len(quads), len(glyphs), len(renderer.layout.quads), len(renderer.layout.glyphs), hotbarQuadSize)
	}

	if err := renderer.Prepare(fullTestInventory(), true, true, 5, nil, nil, fullChestOverlay(), MiningOverlay{}, EatingOverlay{},
		HealthOverlay{Confirmed: true, Value: core.MaxHealth}, OxygenOverlay{Confirmed: true, Value: 0},
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat,
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("glyph 最大分支 Prepare: %v", err)
	}
	if len(renderer.layout.glyphs) != maxHotbarGlyphs || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("glyph 见证 glyphs/quads=%d/%d，上限=%d/%d", len(renderer.layout.glyphs),
			len(renderer.layout.quads), maxHotbarGlyphs, maxHotbarQuads)
	}

	if err := renderer.Prepare(core.Inventory{}, true, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{},
		HealthOverlay{}, OxygenOverlay{}, HungerOverlay{}, ChatOverlay{}, 1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("最小前缀 Prepare: %v", err)
	}
	_, quads, glyphs = renderer.FrameStreams()
	if len(quads) != len(renderer.layout.quads)*hotbarInstanceBytes || len(glyphs) != 0 || len(quads) >= hotbarQuadSize {
		t.Fatalf("最小实际前缀 quads/glyphs=%d/%d，实例=%d 固定 quad 区=%d",
			len(quads), len(glyphs), len(renderer.layout.quads), hotbarQuadSize)
	}
}

func newTestHotbarRenderer() *HotbarRenderer {
	return &HotbarRenderer{
		atlas: &allocationGlyphSource{},
		layout: hotbarLayout{
			quads:  make([]hotbarInstance, 0, maxHotbarQuads),
			glyphs: make([]hotbarInstance, 0, maxHotbarGlyphs),
		},
		upload: make([]byte, hotbarUploadBytes),
	}
}

func finiteRectangle(rect hotbarInstance) bool {
	if rect.Width < 0 || rect.Height < 0 {
		return false
	}
	for _, value := range [...]float32{rect.X, rect.Y, rect.Width, rect.Height} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return true
}

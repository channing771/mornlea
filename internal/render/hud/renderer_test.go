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
	if err := renderer.Prepare(inventory, true, true, 3, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, health, oxygen, hunger, ChatOverlay{}, false, PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 720, budget); err != nil {
		t.Fatalf("warm Prepare: %v", err)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		source.requestCount = 0
		if err := renderer.Prepare(inventory, true, true, 3, nil, nil, nil, MiningOverlay{}, EatingOverlay{}, health, oxygen, hunger, ChatOverlay{}, false, PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 720, budget); err != nil {
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
		HealthOverlay{}, OxygenOverlay{}, HungerOverlay{}, ChatOverlay{}, false,
		PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 800, render.NewUploadBudget(1024),
	); err != nil {
		t.Fatalf("关闭态 Prepare: %v", err)
	}

	const wantQuads = crosshairQuads + 2 + 2 + core.HotbarSlots + 2 + 3
	if got := len(renderer.layout.quads); got != wantQuads {
		t.Fatalf("关闭态采掘实例=%d，想要准星、双层面板、双层选中、九格、轨道/填充和三个缺口共 %d", got, wantQuads)
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
// 这四个数不是内部细节：bounded-benchmark-workload 主规格用「固定 GPU 上传
// 布局、offset 与每帧写入字节数是否移动」来判定 benchmark scenario 要不要升版
// （v15→v16、v17→v18 与 v18→v19 都是因它而升）。没有这条断言，改动 HUD 布局时无人会
// 注意到 scenario 身份已经该动了——而那正是 v16 加氧气条那次发生过的事
// （quad 236→238 恰好没跨过 256 字节对齐边界，offset 与总容量才没变）。
//
// 当前钉值是准星与物品名弹条落地后的重钉容量：quad 256→320（quad 区
// 12800→15616 恰为 glyph offset，256 对齐保持）、glyph 700→768（含弹条双层
// 与 tooltip 预留）。数值随 HUD 结构增长是正常的；改动本测试的期望值时必须
// 同时判定 scenario 版本要不要升，并把结论写进变更产物（benchmark 常量与
// 文档的 v19→v20 同步由 capture/benchmark 任务组落地）。
func TestHotbarFixedUploadLayoutMatchesScenarioVersion(t *testing.T) {
	for _, test := range []struct {
		name      string
		got, want int
	}{
		{"quad 容量", maxHotbarQuads, 320},
		{"glyph 容量", maxHotbarGlyphs, 768},
		{"glyph offset", hotbarGlyphOffset, 15616},
		{"总容量", hotbarUploadBytes, 52480},
	} {
		if test.got != test.want {
			t.Errorf("%s=%d，想要固定容量 %d", test.name, test.got, test.want)
		}
	}
}

// TestResponsiveHotbarPrepareKeepsEveryInstanceInFramebuffer 防止联合布局只缩放
// 快捷栏，遗漏状态行、采掘轨道或打开态最大容器 overlay。
func TestResponsiveHotbarPrepareKeepsEveryInstanceInFramebuffer(t *testing.T) {
	// 弹条全程可见：关闭态帧必须把弹条字形连同其余 HUD 矩形一起缩放进
	// framebuffer（打开态由容器抑制不画，夹具保持同值即可）。
	popup := PopupOverlay{
		Text: strings.Repeat("界", maxPopupRunes), ShownAtTick: 0, WorldTick: 0, Valid: true,
	}
	for _, size := range [][2]uint32{{1280, 720}, {640, 360}, {240, 40}, {17, 800}, {800, 17}, {16, 16}, {1, 1}} {
		for _, open := range []bool{false, true} {
			renderer := newTestHotbarRenderer()
			var chest *ChestOverlay
			mining := MiningOverlay{Active: true, ProgressTicks: 6, RequiredTicks: 15}
			tooltip := TooltipOverlay{}
			if open {
				chest = fullChestOverlay()
				// 打开帧携带有效悬停：指针落在箱子 0 号格（满箱非空）中心，tooltip
				// 的背景 quad 与双层字形随其余 HUD 矩形一起进入逐实例界内断言
				// （窄窗口场景明文含 tooltip）。格中心在任意缩放下都落在格内；
				// scale 为 0 的极小窗口没有任何可命中格，tooltip 与其余实例同为
				// 零尺寸退化。关闭帧由 Prepare 的打开态门控保持 tooltip 零实例。
				scale := hudScale(true, float32(size[0]), float32(size[1]))
				hoverX, hoverY := chestSlotOrigin(0, float32(size[0]), float32(size[1]))
				tooltip = TooltipOverlay{
					Valid:   true,
					CursorX: float64(hoverX + hotbarSlotSize*scale*0.5),
					CursorY: float64(hoverY + hotbarSlotSize*scale*0.5),
				}
			}
			if err := renderer.Prepare(maxQuadTestInventory(), true, open, 5, nil, nil, chest, mining, EatingOverlay{},
				HealthOverlay{Confirmed: true, Value: 7},
				OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1},
				HungerOverlay{Confirmed: true, Value: core.MaxHunger},
				ChatOverlay{}, false, popup, CrosshairOverlay{Visible: true}, tooltip, size[0], size[1], render.NewUploadBudget(1024)); err != nil {
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
			HungerOverlay{}, ChatOverlay{}, false, PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, size[0], size[1], render.NewUploadBudget(1024)); err != nil {
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
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat, false,
		PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("关闭最大分支 Prepare: %v", err)
	}
	closedWant := closedHotbarQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(renderer.layout.quads) != closedWant || closedWant != 100 || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("关闭分支 quads=%d，想要含准星的合法最坏 100 且不超过 %d", len(renderer.layout.quads), maxHotbarQuads)
	}

	// 打开最大分支改由箱子见证（面板族与箱子 81 内容 quad 是最大 overlay）；
	// 悬停 tooltip 背景计入合法最坏；合成视图的最坏组合另起一段锁定。
	hoverX, hoverY := chestSlotOrigin(0, 1280, 800)
	if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, nil, nil, fullChestOverlay(), MiningOverlay{}, EatingOverlay{},
		HealthOverlay{Confirmed: true, Value: core.MaxHealth},
		OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1},
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat, false,
		PopupOverlay{}, CrosshairOverlay{Visible: true},
		TooltipOverlay{Valid: true, CursorX: float64(hoverX) + 1, CursorY: float64(hoverY) + 1},
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("打开最大分支 Prepare: %v", err)
	}
	openWant := openInventoryQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(renderer.layout.quads) != openWant || len(renderer.layout.quads) != 264 || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("较大打开分支 quads=%d，想要含准星与 tooltip 的 264 且不超过固定上限 %d", len(renderer.layout.quads), maxHotbarQuads)
	}
	if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, fullCraftingOverlay(), nil, nil, MiningOverlay{}, EatingOverlay{},
		HealthOverlay{Confirmed: true, Value: core.MaxHealth},
		OxygenOverlay{Confirmed: true, Value: core.MaxOxygenTicks - 1},
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat, false,
		PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("打开合成最大分支 Prepare: %v", err)
	}
	craftingWant := crosshairQuads + containerPanelQuads + 2 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + craftingContentQuads + recipeColumnQuads + healthQuads + oxygenQuads + hungerQuads + maxChatQuads
	if len(renderer.layout.quads) != craftingWant || craftingWant != 243 || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("打开合成分支 quads=%d，想要含准星的 243 且不超过固定上限 %d", len(renderer.layout.quads), maxHotbarQuads)
	}
	// maxQuadTestInventory 的九格工具数量为 1 不出数字，只有 27 个背包格贡献
	// 两位数量（各 4 实例）；配方栏十条入口中三条产物数量为 4（各 1 位双层）。
	recipeDigitGlyphs := 0
	for _, recipeID := range inventoryRecipeIDs {
		if recipe, ok := core.Recipe(recipeID); ok && recipe.Output.Count > 1 {
			recipeDigitGlyphs += 2
		}
	}
	if len(renderer.layout.glyphs) != (core.InventorySlots-core.HotbarSlots)*4+craftingGlyphs+recipeDigitGlyphs+maxChatGlyphs {
		t.Fatalf("打开合成分支 glyphs=%d，想要 %d", len(renderer.layout.glyphs),
			(core.InventorySlots-core.HotbarSlots)*4+craftingGlyphs+recipeDigitGlyphs+maxChatGlyphs)
	}
	viewport, quads, glyphs := renderer.FrameStreams()
	if len(viewport) != hotbarViewportBytes || len(quads) != len(renderer.layout.quads)*hotbarInstanceBytes ||
		len(glyphs) != len(renderer.layout.glyphs)*hotbarInstanceBytes {
		t.Fatalf("实际前缀 viewport/quads/glyphs=%d/%d/%d，实例=%d/%d 固定 quad 区=%d",
			len(viewport), len(quads), len(glyphs), len(renderer.layout.quads), len(renderer.layout.glyphs), hotbarQuadSize)
	}

	if err := renderer.Prepare(fullTestInventory(), true, true, 5, nil, nil, fullChestOverlay(), MiningOverlay{}, EatingOverlay{},
		HealthOverlay{Confirmed: true, Value: core.MaxHealth}, OxygenOverlay{Confirmed: true, Value: 0},
		HungerOverlay{Confirmed: true, Value: core.MaxHunger}, chat, false,
		PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("glyph 最大分支 Prepare: %v", err)
	}
	openGlyphWant := core.InventorySlots*4 + maxOverlayGlyphs + maxChatGlyphs
	if len(renderer.layout.glyphs) != openGlyphWant || openGlyphWant > maxHotbarGlyphs ||
		len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("glyph 见证 glyphs/quads=%d/%d，分支公式=%d 固定上限=%d/%d", len(renderer.layout.glyphs),
			len(renderer.layout.quads), openGlyphWant, maxHotbarGlyphs, maxHotbarQuads)
	}

	if err := renderer.Prepare(core.Inventory{}, true, false, -1, nil, nil, nil, MiningOverlay{}, EatingOverlay{},
		HealthOverlay{}, OxygenOverlay{}, HungerOverlay{}, ChatOverlay{}, false,
		PopupOverlay{}, CrosshairOverlay{Visible: true}, TooltipOverlay{}, 1280, 800, render.NewUploadBudget(1024)); err != nil {
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

package hud

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/packages/shared/core"
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
	budget := render.NewUploadBudget(1024)
	warm := func() error {
		return renderer.Prepare(inventory, true, true, 3, nil, nil, nil, TooltipOverlay{}, 1280, 720, budget)
	}
	if err := warm(); err != nil {
		t.Fatalf("warm Prepare: %v", err)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		source.requestCount = 0
		if err := warm(); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("warmed hotbar Prepare allocations=%v want=0", allocations)
	}
}

// TestHotbarPrepareClosedProducesNoInstances 验证 `Prepare` 的关闭态输出恰好
// 零实例：常显层已迁 WebView，GPU 保留面只在容器界面打开时布局。
func TestHotbarPrepareClosedProducesNoInstances(t *testing.T) {
	renderer := newTestHotbarRenderer()
	if err := renderer.Prepare(
		maxQuadTestInventory(), true, false, -1, nil, nil, nil, TooltipOverlay{},
		1280, 800, render.NewUploadBudget(1024),
	); err != nil {
		t.Fatalf("关闭态 Prepare: %v", err)
	}
	if len(renderer.layout.quads) != 0 || len(renderer.layout.glyphs) != 0 {
		t.Fatalf("关闭态 quads/glyphs=%d/%d，想要 0/0", len(renderer.layout.quads), len(renderer.layout.glyphs))
	}
	_, quads, glyphs := renderer.FrameStreams()
	if len(quads) != 0 || len(glyphs) != 0 {
		t.Fatalf("关闭态实际前缀 quads/glyphs=%d/%d，想要 0/0", len(quads), len(glyphs))
	}

	// 物品镜像未确认时同样零实例：保留面不布局任何内容。
	if err := renderer.Prepare(
		core.Inventory{}, false, true, -1, nil, nil, fullChestOverlay(), TooltipOverlay{},
		1280, 800, render.NewUploadBudget(1024),
	); err != nil {
		t.Fatalf("镜像未确认 Prepare: %v", err)
	}
	if len(renderer.layout.quads) != 0 || len(renderer.layout.glyphs) != 0 {
		t.Fatalf("镜像未确认 quads/glyphs=%d/%d，想要 0/0", len(renderer.layout.quads), len(renderer.layout.glyphs))
	}
}

// TestHotbarFixedUploadLayoutMatchesScenarioVersion 把 Hotbar HUD 的**固定上传
// 布局**钉成数值断言。
//
// 这四个数不是内部细节：bounded-benchmark-workload 主规格用「固定 GPU 上传
// 布局、offset 与每帧写入字节数是否移动」来判定 benchmark scenario 要不要升版。
// 常显层退役只缩小了各分支的实际实例前缀，固定容量、offset 与对齐必须保持
// 不变（scenario 版本演进由 capture/benchmark 任务组统一收口）。
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

// TestResponsiveHotbarPrepareKeepsEveryInstanceInFramebuffer 防止打开态布局在
// 缩放中越界：面板、统一栏位、双层物品 tile、耐久条、箱子内容与 tooltip 背景
// 都必须留在 framebuffer 内，零尺寸 framebuffer 安全退化为零实例。
func TestResponsiveHotbarPrepareKeepsEveryInstanceInFramebuffer(t *testing.T) {
	for _, size := range [][2]uint32{{1280, 720}, {640, 360}, {240, 40}, {17, 800}, {800, 17}, {16, 16}, {1, 1}} {
		renderer := newTestHotbarRenderer()
		// 打开帧携带有效悬停：指针落在箱子 0 号格（满箱非空）中心，tooltip
		// 的背景 quad 与双层字形随其余实例一起进入逐实例界内断言。格中心在
		// 任意缩放下都落在格内；scale 为 0 的极小窗口没有任何可命中格，tooltip
		// 与其余实例同为零尺寸退化。
		scale := hudScale(float32(size[0]), float32(size[1]))
		hoverX, hoverY := chestSlotOrigin(0, float32(size[0]), float32(size[1]))
		tooltip := TooltipOverlay{
			Valid:   true,
			CursorX: float64(hoverX + hotbarSlotSize*scale*0.5),
			CursorY: float64(hoverY + hotbarSlotSize*scale*0.5),
		}
		if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, nil, nil, fullChestOverlay(),
			tooltip, size[0], size[1], render.NewUploadBudget(1024)); err != nil {
			t.Fatalf("framebuffer %v Prepare: %v", size, err)
		}
		for index, quad := range renderer.layout.quads {
			if !finiteRectangle(quad) || quad.X < 0 || quad.Y < 0 || quad.X+quad.Width > float32(size[0]) || quad.Y+quad.Height > float32(size[1]) {
				t.Fatalf("framebuffer %v quad %d 越界: %+v", size, index, quad)
			}
		}
		for index, glyph := range renderer.layout.glyphs {
			if !finiteRectangle(glyph) || glyph.X < 0 || glyph.Y < 0 || glyph.X+glyph.Width > float32(size[0]) || glyph.Y+glyph.Height > float32(size[1]) {
				t.Fatalf("framebuffer %v glyph %d 越界: %+v", size, index, glyph)
			}
		}
	}

	for _, size := range [][2]uint32{{0, 720}, {1280, 0}, {0, 0}} {
		renderer := newTestHotbarRenderer()
		if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, nil, nil, fullChestOverlay(),
			TooltipOverlay{}, size[0], size[1], render.NewUploadBudget(1024)); err != nil {
			t.Fatalf("framebuffer %v Prepare: %v", size, err)
		}
		if len(renderer.layout.quads) != 0 || len(renderer.layout.glyphs) != 0 {
			t.Fatalf("framebuffer %v quads/glyphs=%d/%d，想要 0/0", size,
				len(renderer.layout.quads), len(renderer.layout.glyphs))
		}
	}
}

// TestHotbarMaximumBranchesAndEncodingContract 同时见证互斥分支容量、48-byte
// 编码、256-byte 对齐与 `FrameStreams` 的实际实例前缀。常显层退役后关闭态零
// 实例，打开态由箱子视图见证 218，合成视图低于箱子不是见证分支。
func TestHotbarMaximumBranchesAndEncodingContract(t *testing.T) {
	if hotbarInstanceBytes != 48 {
		t.Fatalf("hotbarInstanceBytes=%d，想要 48", hotbarInstanceBytes)
	}
	if hotbarViewportOffset%256 != 0 || hotbarQuadOffset%256 != 0 || hotbarGlyphOffset%256 != 0 ||
		hotbarViewportOffset+hotbarViewportBytes > hotbarQuadOffset ||
		hotbarQuadOffset+hotbarQuadSize > hotbarGlyphOffset ||
		hotbarGlyphOffset+hotbarGlyphSize != hotbarUploadBytes {
		t.Fatalf("上传区间异常: viewport=%d+%d quad=%d+%d glyph=%d+%d total=%d",
			hotbarViewportOffset, hotbarViewportBytes, hotbarQuadOffset, hotbarQuadSize,
			hotbarGlyphOffset, hotbarGlyphSize, hotbarUploadBytes)
	}

	renderer := newTestHotbarRenderer()
	// 打开最大分支由箱子见证（面板族与箱子 81 内容 quad 是最大 overlay）；
	// 悬停 tooltip 背景计入合法最坏。
	longNameChest := fullChestOverlay()
	longNameChest.Items[0] = core.ItemStack{Item: core.ItemBrokenStonePickaxe, Count: core.MaxStackCount}
	hoverX, hoverY := chestSlotOrigin(0, 1280, 800)
	if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, nil, nil, fullChestOverlay(),
		TooltipOverlay{Valid: true, CursorX: float64(hoverX) + 1, CursorY: float64(hoverY) + 1},
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("打开最大分支 Prepare: %v", err)
	}
	if got := len(renderer.layout.quads); got != 218 || got > maxHotbarQuads {
		t.Fatalf("打开分支 quads=%d，想要含 tooltip 背景的 218 且不超过固定上限 %d", got, maxHotbarQuads)
	}
	// glyph 预算按 tooltip 8 rune 截断上限封顶；注册表实测最长名（5 rune 双层
	// 10）是更小的实测见证，两者都不得超过固定 768。
	if got := core.InventorySlots*4 + chestGlyphs + tooltipGlyphs; got != 268 {
		t.Fatalf("打开态 glyph 预算=%d，想要钉值 268", got)
	}

	// 合成视图（面板加配方栏与网格内容）低于箱子见证分支，但仍是合法组合：
	// 固定预算的逐项构成在这里按命名常量复核。
	if err := renderer.Prepare(maxQuadTestInventory(), true, true, 5, fullCraftingOverlay(), nil, nil,
		TooltipOverlay{}, 1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("打开合成最大分支 Prepare: %v", err)
	}
	craftingWant := containerPanelQuads + 2 + core.InventorySlots + core.InventorySlots*2 +
		core.HotbarSlots*2 + craftingContentQuads + recipeColumnQuads
	if got := len(renderer.layout.quads); got != craftingWant || got != 197 || got > maxHotbarQuads {
		t.Fatalf("打开合成分支 quads=%d，想要 %d（197）且不超过固定上限 %d", got, craftingWant, maxHotbarQuads)
	}
	// maxQuadTestInventory 的九格工具数量为 1 不出数字，只有 27 个背包格贡献
	// 两位数量（各 4 实例）；配方栏十条入口中数量大于一的产物各出两位双层。
	recipeDigitGlyphs := 0
	for _, recipeID := range inventoryRecipeIDs {
		if recipe, ok := core.Recipe(recipeID); ok && recipe.Output.Count > 1 {
			recipeDigitGlyphs += 2
		}
	}
	if got := len(renderer.layout.glyphs); got != (core.InventorySlots-core.HotbarSlots)*4+craftingGlyphs+recipeDigitGlyphs {
		t.Fatalf("打开合成分支 glyphs=%d，想要 %d", got,
			(core.InventorySlots-core.HotbarSlots)*4+craftingGlyphs+recipeDigitGlyphs)
	}
	viewport, quads, glyphs := renderer.FrameStreams()
	if len(viewport) != hotbarViewportBytes || len(quads) != len(renderer.layout.quads)*hotbarInstanceBytes ||
		len(glyphs) != len(renderer.layout.glyphs)*hotbarInstanceBytes {
		t.Fatalf("实际前缀 viewport/quads/glyphs=%d/%d/%d，实例=%d/%d 固定 quad 区=%d",
			len(viewport), len(quads), len(glyphs), len(renderer.layout.quads), len(renderer.layout.glyphs), hotbarQuadSize)
	}

	// glyph 见证分支：满格两位数量 + 满箱两位数量 + 悬停 tooltip，不得超过
	// 固定 768。
	if err := renderer.Prepare(fullTestInventory(), true, true, 5, nil, nil, fullChestOverlay(), TooltipOverlay{},
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("glyph 最大分支 Prepare: %v", err)
	}
	if got := len(renderer.layout.glyphs); got > maxHotbarGlyphs || len(renderer.layout.quads) > maxHotbarQuads {
		t.Fatalf("glyph 见证 glyphs/quads=%d/%d，固定上限=%d/%d", got,
			len(renderer.layout.quads), maxHotbarGlyphs, maxHotbarQuads)
	}

	// 最小前缀：空镜像（未确认）零实例，quad 前缀必须严格小于固定 quad 区。
	if err := renderer.Prepare(core.Inventory{}, false, true, -1, nil, nil, nil, TooltipOverlay{},
		1280, 800, render.NewUploadBudget(1024)); err != nil {
		t.Fatalf("最小前缀 Prepare: %v", err)
	}
	_, quads, glyphs = renderer.FrameStreams()
	if len(quads) != 0 || len(glyphs) != 0 || len(quads) >= hotbarQuadSize {
		t.Fatalf("最小实际前缀 quads/glyphs=%d/%d，固定 quad 区=%d", len(quads), len(glyphs), hotbarQuadSize)
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

package hud

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// HotbarRenderer 以固定容量绘制 9 格快捷栏 HUD。
// 它只消费已确认的权威快捷栏值，不做任何本地预测。
type HotbarRenderer struct {
	atlas render.GlyphSource

	layout hotbarLayout
	upload []byte
	// hudPixels 保留构建贴图时的像素,供 AtlasPixels 导出同一份内容。
	hudPixels []byte
}

func (renderer *HotbarRenderer) Prepare(
	inventory core.Inventory,
	inventoryConfirmed bool,
	open bool,
	source int,
	crafting *CraftingOverlay,
	overlay *FurnaceOverlay,
	chest *ChestOverlay,
	mining MiningOverlay,
	eating EatingOverlay,
	health HealthOverlay,
	oxygen OxygenOverlay,
	hunger HungerOverlay,
	chat ChatOverlay,
	popup PopupOverlay,
	crosshair CrosshairOverlay,
	tooltip TooltipOverlay,
	width, height uint32,
	budget *render.UploadBudget,
) error {
	textRequested := false
	if inventoryConfirmed {
		renderer.atlas.Request(hotbarDigits)
		textRequested = true
	}
	textRequested = requestChatText(renderer.atlas, chat) || textRequested
	textRequested = requestPopupText(renderer.atlas, popup, open) || textRequested
	textRequested = requestTooltipText(renderer.atlas, tooltip, inventory, crafting, overlay, chest,
		open, float32(width), float32(height)) || textRequested
	if textRequested {
		if err := renderer.atlas.FlushUploads(budget); err != nil {
			return err
		}
	}
	if inventoryConfirmed {
		layoutInventory(
			&renderer.layout, renderer.atlas, inventory, open, source, crafting, overlay, chest, mining, eating,
			crosshair, float32(width), float32(height),
		)
	} else {
		renderer.layout.quads = renderer.layout.quads[:0]
		renderer.layout.glyphs = renderer.layout.glyphs[:0]
		renderer.layout.scale = hudScale(open, float32(width), float32(height))
		renderer.layout.open = open
		// 物品镜像未确认时快捷栏与状态条都不布局，但准星只依赖 framebuffer
		// 几何与相位门控，HUD 可见即呈现（游戏相位登录早期等场景）。
		appendCrosshair(&renderer.layout, crosshair, float32(width), float32(height))
	}
	appendHealthBar(&renderer.layout, health, open, float32(width), float32(height))
	appendOxygenBar(&renderer.layout, oxygen, open, float32(width), float32(height))
	appendHungerBar(&renderer.layout, hunger, open, float32(width), float32(height))
	appendChatOverlay(&renderer.layout, renderer.atlas, chat, float32(width), float32(height))
	// 弹条最后布局：它只追加 glyph、锚在关闭态几何上，与物品镜像确认状态无关。
	appendPopupOverlay(&renderer.layout, renderer.atlas, popup, open, float32(width), float32(height))
	// tooltip 只在打开态布局：悬停解析与命中测试共用同一几何，实例追加在
	// 一切 HUD 内容之后，保证浮在最上层。
	if open && inventoryConfirmed {
		appendTooltipOverlay(&renderer.layout, renderer.atlas, tooltip, inventory, crafting, overlay, chest,
			float32(width), float32(height))
	}
	encodeHotbarViewport(
		renderer.upload[hotbarViewportOffset:hotbarViewportOffset+hotbarViewportBytes],
		float32(width), float32(height),
	)
	encodeHotbarInstances(
		renderer.upload[hotbarQuadOffset:hotbarQuadOffset+hotbarQuadSize],
		renderer.layout.quads,
	)
	encodeHotbarInstances(
		renderer.upload[hotbarGlyphOffset:hotbarUploadBytes],
		renderer.layout.glyphs,
	)
	return nil
}

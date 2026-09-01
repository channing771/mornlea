package hud

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/render"
)

// HotbarRenderer 以固定容量准备容器保留面（浮动面板/箱子/熔炉/合成/tooltip）
// 的实例。它只消费已确认的权威镜像值，不做任何本地预测；常显层（快捷栏贴条、
// 状态行、氧气、采掘/进食轨道、物品名弹条、准星、聊天呈现）已迁 WebView HUD
// 组件，不再进入本 pass。
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
	tooltip TooltipOverlay,
	width, height uint32,
	budget *render.UploadBudget,
) error {
	textRequested := false
	if inventoryConfirmed {
		renderer.atlas.Request(hotbarDigits)
		textRequested = true
	}
	// tooltip 只在打开态布局：悬停解析与命中测试共用同一几何。
	textRequested = requestTooltipText(renderer.atlas, tooltip, inventory, crafting, overlay, chest,
		open, float32(width), float32(height)) || textRequested
	if textRequested {
		if err := renderer.atlas.FlushUploads(budget); err != nil {
			return err
		}
	}
	if !inventoryConfirmed {
		// 物品镜像未确认时保留面不布局任何实例：固定上传区只写 viewport 前缀，
		// 实例前缀保持为零长度。
		renderer.layout.quads = renderer.layout.quads[:0]
		renderer.layout.glyphs = renderer.layout.glyphs[:0]
		renderer.layout.scale = hudScale(float32(width), float32(height))
	} else {
		layoutInventory(
			&renderer.layout, renderer.atlas, inventory, open, source, crafting, overlay, chest,
			float32(width), float32(height),
		)
		// tooltip 实例追加在一切 HUD 内容之后，保证浮在最上层。
		if open {
			appendTooltipOverlay(&renderer.layout, renderer.atlas, tooltip, inventory, crafting, overlay, chest,
				float32(width), float32(height))
		}
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

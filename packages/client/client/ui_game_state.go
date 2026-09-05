package client

import "github.com/channing771/mornlea/packages/shared/core"

// UIGameSlotRef 标识当前视图内的语义栏位，绝不表示像素位置。
type UIGameSlotRef struct {
	Area  string `json:"area"`
	Index int    `json:"index"`
}

// UIGameRecipe 是注册配方的只读材料预览。
type UIGameRecipe struct {
	Name   string       `json:"name"`
	Size   int          `json:"size"`
	Slots  [9]UIHudSlot `json:"slots"`
	Output UIHudSlot    `json:"output"`
}

// UIGameState 把镜像与本地视图状态一起下行；库存值始终等待权威确认。
type UIGameState struct {
	Token       uint64           `json:"token"`
	Kind        string           `json:"kind"`
	CursorFree  bool             `json:"cursorFree"`
	Confirmed   bool             `json:"confirmed"`
	Inventory   [36]UIHudSlot    `json:"inventory"`
	Grid        [9]UIHudSlot     `json:"grid"`
	GridSize    int              `json:"gridSize"`
	Output      UIHudSlot        `json:"output"`
	Chest       [27]UIHudSlot    `json:"chest"`
	Furnace     [3]UIHudSlot     `json:"furnace"`
	Progress    float32          `json:"progress"`
	Burn        float32          `json:"burn"`
	Recipes     [10]UIGameRecipe `json:"recipes"`
	RecipeIndex int              `json:"recipeIndex"`
	Source      *UIGameSlotRef   `json:"source,omitempty"`
}

// NewUIGameSlot 统一背包与快捷栏的名称、数量及耐久出口；图像由装配层缓存注入。
func NewUIGameSlot(stack core.ItemStack, sources ...UIItemMetadataSource) UIHudSlot {
	slot := newUIItemSlot(stack, firstUIItemMetadataSource(sources))
	if slot.Name == "" {
		slot.Name, _ = core.ItemDisplayName(stack.Item)
	}
	return slot
}

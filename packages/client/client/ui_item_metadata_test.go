package client

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

type uiMetadataStub struct{}

func (uiMetadataStub) UIItemMetadata(item core.ItemID) (UIItemMetadata, bool) {
	if item != core.ItemStone {
		return UIItemMetadata{}, false
	}
	return UIItemMetadata{Name: "缓存石头", Icon: "data:image/png;base64,c3RvbmU="}, true
}

func TestUISlotConstructorsUseSharedItemMetadata(t *testing.T) {
	stack := core.ItemStack{Item: core.ItemStone, Count: 3}
	game := NewUIGameSlot(stack, uiMetadataStub{})
	if game.Name != "缓存石头" || game.Icon != "data:image/png;base64,c3RvbmU=" {
		t.Fatalf("游戏栏位元数据=%+v", game)
	}
	hotbar := core.Hotbar{}
	hotbar.Slots[0] = stack
	hud := NewUIHudHotbar(hotbar, uiMetadataStub{})
	if hud == nil || hud.Slots[0].Name != game.Name || hud.Slots[0].Icon != game.Icon {
		t.Fatalf("HUD 与游戏栏位未消费同一元数据：hud=%+v game=%+v", hud, game)
	}
}

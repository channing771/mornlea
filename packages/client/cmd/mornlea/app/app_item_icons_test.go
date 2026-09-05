//go:build darwin

package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestItemIconCatalogEncodesOnceAndMatchesRegistryPixels(t *testing.T) {
	registry := assets.NewRegistry()
	calls := 0
	catalog, err := buildItemIconCatalog(registry, func(pixels []byte) (string, error) {
		calls++
		return encodeItemIconDataURI(pixels)
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != int(core.ItemIDMax)-1 {
		t.Fatalf("编码次数=%d，想要每个注册物品一次 %d", calls, int(core.ItemIDMax)-1)
	}
	for repeat := 0; repeat < 100; repeat++ {
		metadata, ok := catalog.UIItemMetadata(core.ItemIronPickaxe)
		if !ok || metadata.Name != "铁镐" || !strings.HasPrefix(metadata.Icon, itemIconDataURIPrefix) {
			t.Fatalf("缓存元数据=%+v,%v", metadata, ok)
		}
	}
	if calls != int(core.ItemIDMax)-1 {
		t.Fatalf("重复 HUD tick 触发重新编码：%d", calls)
	}
	metadata, _ := catalog.UIItemMetadata(core.ItemIronPickaxe)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(metadata.Icon, itemIconDataURIPrefix))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := registry.ItemIconRGBA(core.ItemIronPickaxe)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			r, g, b, a := decoded.At(x, y).RGBA()
			offset := (y*16 + x) * 4
			if [4]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)} != [4]byte(want[offset:offset+4]) {
				t.Fatalf("解码像素 (%d,%d) 与 registry 不同源", x, y)
			}
		}
	}
}

func TestHUDGameAndRecipesUseCachedIconsWithinBridgeLimit(t *testing.T) {
	registry := assets.NewRegistry()
	catalog, err := newItemIconCatalog(registry)
	if err != nil {
		t.Fatal(err)
	}
	app, _ := newHUDPushTestApplication(t)
	app.registry = registry
	app.itemIcons = catalog
	app.gameRecipeIndex = -1
	inventory := core.Inventory{}
	for index := 0; index < core.InventorySlots; index++ {
		var ok bool
		inventory, ok = inventory.SetSlot(uint8(index), core.ItemStack{Item: core.ItemStone, Count: 64})
		if !ok {
			t.Fatal("背包夹具")
		}
	}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	game := app.buildGameUIState()
	stoneMetadata, _ := catalog.UIItemMetadata(core.ItemStone)
	if game.Inventory[0].Icon == "" || game.Inventory[0].Icon != stoneMetadata.Icon {
		t.Fatalf("背包未消费石头缓存图标：inventory=%q cache=%q", game.Inventory[0].Icon, stoneMetadata.Icon)
	}
	recipeItem := game.Recipes[0].Output.Item
	recipeMetadata, _ := catalog.UIItemMetadata(recipeItem)
	if game.Recipes[0].Output.Icon != recipeMetadata.Icon {
		t.Fatalf("配方未消费物品 %d 的缓存图标", recipeItem)
	}
	hud := app.assembleHUDState()
	if hud.Hotbar == nil || hud.Hotbar.Slots[0].Icon != game.Inventory[0].Icon {
		t.Fatalf("HUD 与面板图标不同源：hud=%+v game=%q", hud.Hotbar, game.Inventory[0].Icon)
	}
	maxItem := core.ItemNone
	maxMetadata := client.UIItemMetadata{}
	for item := core.ItemID(1); item < core.ItemIDMax; item++ {
		metadata, ok := catalog.UIItemMetadata(item)
		if ok && len(metadata.Icon) > len(maxMetadata.Icon) {
			maxItem, maxMetadata = item, metadata
		}
	}
	maxSlot := client.UIHudSlot{Item: maxItem, Count: 64, Name: maxMetadata.Name, Icon: maxMetadata.Icon}
	for index := range hud.Hotbar.Slots {
		hud.Hotbar.Slots[index] = maxSlot
	}
	for index := range game.Inventory {
		game.Inventory[index] = maxSlot
	}
	for index := range game.Grid {
		game.Grid[index] = maxSlot
	}
	for index := range game.Chest {
		game.Chest[index] = maxSlot
	}
	for index := range game.Furnace {
		game.Furnace[index] = maxSlot
	}
	game.Output = maxSlot
	for index := range game.Recipes {
		game.Recipes[index].Output = maxSlot
		for slot := range game.Recipes[index].Slots {
			game.Recipes[index].Slots[slot] = maxSlot
		}
	}
	hudPayload, err := json.Marshal(hud)
	if err != nil {
		t.Fatal(err)
	}
	document := uiStateJSON{Phase: "game", Game: game, Hud: hudPayload}
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > client.MaxUIEnvelopeBytes {
		t.Fatalf("最坏面板 JSON=%d bytes，越过桥容量 %d", len(payload), client.MaxUIEnvelopeBytes)
	}
	t.Logf("最大图标物品=%d URI=%d bytes；最坏面板 JSON=%d bytes", maxItem, len(maxMetadata.Icon), len(payload))
}

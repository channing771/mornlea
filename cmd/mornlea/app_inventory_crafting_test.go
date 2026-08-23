//go:build darwin

package main

// app_inventory_crafting_test.go：背包点击移动与合成请求——只读已确认背包、界面关闭与会话清理。

import (
	"fmt"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render/hud"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestInventoryTwoClicksSendOneMoveRequest(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: true,
	})
	app.drainServerMessages(1)
	app.setInventoryOpen(true)

	width, height := uint32(1280), uint32(720)
	sourceX, sourceY := inventorySlotCenter(t, 1, width, height)
	targetX, targetY := inventorySlotCenter(t, 30, width, height)

	app.clickInventorySlot(sourceX, sourceY, width, height)
	if app.inventorySource != 1 {
		t.Fatalf("首次点击来源 = %d，想要 1", app.inventorySource)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)

	app.clickInventorySlot(targetX, targetY, width, height)
	if app.inventorySource != -1 {
		t.Fatalf("第二次点击后来源未清除: %d", app.inventorySource)
	}
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	if got, ok := message.(network.MoveInventoryStack); !ok || got.From != 1 || got.To != 30 {
		t.Fatalf("移动请求 = %#v，想要 1 → 30", message)
	}
}

func TestInventoryClickOutsideSlotsDoesNothing(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	app.setInventoryOpen(true)
	app.clickInventorySlot(0, 0, 1280, 720)
	if app.inventorySource != -1 {
		t.Fatalf("界外点击记录了来源: %d", app.inventorySource)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

// 杀死变异：跳过已确认背包检查、发送错误配方、预测结果或重复发送都会失败。
func TestCraftRecipeClickUsesConfirmedInventory(t *testing.T) {
	for _, test := range []struct {
		name   string
		recipe core.RecipeID
		input  core.ItemStack
	}{
		{"石砖", core.RecipeStoneBricks, core.ItemStack{Item: core.ItemStone, Count: 4}},
		{"熔炉", core.RecipeFurnace, core.ItemStack{Item: core.ItemStone, Count: 8}},
		{"铁块", core.RecipeIronBlock, core.ItemStack{Item: core.ItemIronIngot, Count: 9}},
		{"箱子", core.RecipeChest, core.ItemStack{Item: core.ItemStone, Count: 8}},
		{"橡木木板", core.RecipeOakPlanks, core.ItemStack{Item: core.ItemOakLog, Count: 1}},
		{"发光方块", core.RecipeLightBlock, core.ItemStack{Item: core.ItemGlass, Count: 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, serverEndpoint := newInteractiveTestApplication(t)
			var inventory core.Inventory
			inventory.Hotbar.Slots[0] = test.input
			if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
				t.Fatal(err)
			}
			app.inventorySource = 5
			width, height := uint32(1280), uint32(720)
			x, y := recipeButtonCenter(t, test.recipe, width, height)

			app.clickInventorySlot(x, y, width, height)
			message := receiveInteractiveClientMessage(t, serverEndpoint)
			craft, ok := message.(network.CraftRecipe)
			if !ok || craft.Recipe != test.recipe {
				t.Fatalf("合成请求 = %#v，想要 recipe %d", message, test.recipe)
			}
			assertNoInteractiveClientMessage(t, serverEndpoint)
			if app.inventorySource != -1 {
				t.Fatalf("合成后来源未清除: %d", app.inventorySource)
			}
			got, confirmed := app.inventory.State()
			if !confirmed || got != inventory {
				t.Fatalf("合成请求本地改写镜像: %+v, %v", got, confirmed)
			}
		})
	}
}

func TestUnavailableCraftRecipeClickDoesNothing(t *testing.T) {
	for _, recipe := range []core.RecipeID{
		core.RecipeStoneBricks, core.RecipeFurnace, core.RecipeIronBlock,
	} {
		t.Run(fmt.Sprintf("recipe_%d", recipe), func(t *testing.T) {
			app, serverEndpoint := newInteractiveTestApplication(t)
			inventory := core.Inventory{}
			if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
				t.Fatal(err)
			}
			x, y := recipeButtonCenter(t, recipe, 1280, 720)

			app.clickInventorySlot(x, y, 1280, 720)
			assertNoInteractiveClientMessage(t, serverEndpoint)
			got, confirmed := app.inventory.State()
			if !confirmed || got != inventory {
				t.Fatalf("不可用配方改写镜像: %+v, %v", got, confirmed)
			}
		})
	}

	t.Run("产物无容量", func(t *testing.T) {
		app, serverEndpoint := newInteractiveTestApplication(t)
		inventory := core.Inventory{}
		for slot := range inventory.Hotbar.Slots {
			inventory.Hotbar.Slots[slot] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount}
		}
		for slot := range inventory.Backpack {
			inventory.Backpack[slot] = core.ItemStack{Item: core.ItemDirt, Count: core.MaxStackCount}
		}
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 5}
		if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
			t.Fatal(err)
		}
		x, y := recipeButtonCenter(t, core.RecipeStoneBricks, 1280, 720)

		app.clickInventorySlot(x, y, 1280, 720)
		assertNoInteractiveClientMessage(t, serverEndpoint)
		got, confirmed := app.inventory.State()
		if !confirmed || got != inventory {
			t.Fatalf("产物无容量时改写镜像: %+v, %v", got, confirmed)
		}
	})
}

func TestCraftRecipeClickWaitsForConfirmedInventory(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	x, y := recipeButtonCenter(t, core.RecipeStoneBricks, 1280, 720)

	app.clickInventorySlot(x, y, 1280, 720)
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if app.sequence != 0 {
		t.Fatalf("未确认背包消耗了 sequence: %d", app.sequence)
	}
	if _, confirmed := app.inventory.State(); confirmed {
		t.Fatal("点击后空镜像被标记为已确认")
	}
}

func TestPlayerResetClearsInventorySource(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	app.inventoryOpen = true
	app.inventorySource = 8
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
	})

	app.drainServerMessages(1)
	if !app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("reset 后 open=%v source=%d，想要界面保持且来源清除", app.inventoryOpen, app.inventorySource)
	}
}

func TestClientSessionCloseClearsInventoryUIState(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	if err := app.furnace.Apply(network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.inventorySource = 8

	app.closeClientSession(nil)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("断线后 open=%v source=%d，想要界面关闭且来源清除", app.inventoryOpen, app.inventorySource)
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("断线后仍保留熔炉镜像")
	}
}

func TestInventoryCloseClearsSourceAndRecapturesCursor(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	app.setInventoryOpen(true)
	if window.CursorCaptured() {
		t.Fatal("打开背包后仍捕获鼠标")
	}
	width, height := uint32(1280), uint32(720)
	x, y := inventorySlotCenter(t, 5, width, height)
	app.clickInventorySlot(x, y, width, height)

	app.setInventoryOpen(false)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("关闭后 open=%v source=%d", app.inventoryOpen, app.inventorySource)
	}
	if !window.CursorCaptured() {
		t.Fatal("关闭背包后未恢复鼠标捕获")
	}
}

func inventorySlotCenter(t *testing.T, slot int, width, height uint32) (float64, float64) {
	t.Helper()
	for x := range int(width) {
		for y := range int(height) {
			got, ok := hud.InventorySlotAt(float64(x), float64(y), width, height)
			if ok && int(got) == slot {
				return float64(x), float64(y)
			}
		}
	}
	t.Fatalf("找不到栏位 %d 的像素", slot)
	return 0, 0
}

func recipeButtonCenter(t *testing.T, recipe core.RecipeID, width, height uint32) (float64, float64) {
	t.Helper()
	for y := range int(height) {
		for x := range int(width) {
			if got, ok := hud.RecipeButtonAt(float64(x), float64(y), width, height); ok &&
				got == recipe {
				return float64(x), float64(y)
			}
		}
	}
	t.Fatalf("找不到 recipe %d 的按钮像素", recipe)
	return 0, 0
}

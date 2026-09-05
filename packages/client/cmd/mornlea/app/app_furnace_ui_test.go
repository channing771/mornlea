//go:build darwin

package app

// app_furnace_ui_test.go：熔炉界面镜像生命周期——权威状态开 UI、两次点击一次移动、显式/服务端关闭与 reset 清理。

import (
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestAuthoritativeFurnaceStateOpensUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := network.FurnaceState{
		Furnace:       core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
		Input:         core.ItemStack{Item: core.ItemRawIron, Count: 2},
		Fuel:          core.ItemStack{Item: core.ItemCoal, Count: 3},
		ProgressTicks: 17, BurnTicks: 1599,
	}

	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	got, opened := app.furnace.State()
	if !opened || got != state || !app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("权威熔炉界面 state=%+v opened=%v ui=%v source=%v",
			got, opened, app.inventoryOpen, app.gameSource)
	}
	app.gameSource = &client.UIGameSlotRef{Area: "inventory", Index: 1}
	state.ProgressTicks++
	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	if app.gameSource == nil || app.gameSource.Index != 1 {
		t.Fatalf("连续权威更新清除了已选来源: %v", app.gameSource)
	}
}

func TestFurnaceTwoClicksSendOneMoveWithoutPrediction(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	var inventory core.Inventory
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemRawIron, Count: 2}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	state := network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Slot: 2, Generation: 3},
		Fuel:    core.ItemStack{Item: core.ItemCoal, Count: 1},
	}
	if err := app.furnace.Apply(state); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true

	app.menu.phase = MenuPhaseGame
	gameTestAction(app, "slot", "inventory", 1)
	assertNoInteractiveClientMessage(t, serverEndpoint)
	gameTestAction(app, "slot", "furnace", 0)

	message := receiveInteractiveClientMessage(t, serverEndpoint)
	want := network.MoveContainerStack{
		Sequence: 1, Container: state.Furnace, From: 1, To: core.FurnaceInputSlot,
	}
	if got, ok := message.(network.MoveContainerStack); !ok || got != want {
		t.Fatalf("跨容器移动 = %#v，想要 %+v", message, want)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if got, _ := app.inventory.State(); got != inventory {
		t.Fatalf("移动请求改写了物品镜像: %+v", got)
	}
	if got, _ := app.furnace.State(); got != state {
		t.Fatalf("移动请求改写了熔炉镜像: %+v", got)
	}
}

func TestExplicitFurnaceCloseClearsUIAndSendsOnce(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	state := network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}
	if err := app.furnace.Apply(state); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.gameSource = &client.UIGameSlotRef{Area: "furnace", Index: core.FurnaceFuelSlot - core.FurnaceInputSlot}

	app.setInventoryOpen(false)
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	if got, ok := message.(network.CloseContainer); !ok || got != (network.CloseContainer{Sequence: 1}) {
		t.Fatalf("关闭熔炉请求 = %#v", message)
	}
	app.setInventoryOpen(false)
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("关闭后 ui=%v source=%v", app.inventoryOpen, app.gameSource)
	}
	if !window.CursorCaptured() {
		t.Fatal("关闭熔炉后未恢复鼠标捕获")
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("显式关闭后仍保留熔炉镜像")
	}
}

func TestFurnaceClosedMessageClearsUIWithoutEcho(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Slot: 1, Generation: 2},
	}
	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	app.gameSource = &client.UIGameSlotRef{Area: "furnace", Index: core.FurnaceOutputSlot - core.FurnaceInputSlot}

	sendInteractiveServerMessage(t, serverEndpoint, network.ContainerClosed{Container: state.Furnace})
	app.DrainServerMessages(1)
	if app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("服务端关闭后 ui=%v source=%v", app.inventoryOpen, app.gameSource)
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("服务端关闭后仍保留熔炉镜像")
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

func TestPlayerResetClosesFurnaceUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.furnace.Apply(network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.gameSource = &client.UIGameSlotRef{Area: "furnace", Index: core.FurnaceInputSlot - core.FurnaceInputSlot}
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
	})

	app.DrainServerMessages(1)
	if app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("reset 后 ui=%v source=%v", app.inventoryOpen, app.gameSource)
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("reset 后仍保留熔炉镜像")
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

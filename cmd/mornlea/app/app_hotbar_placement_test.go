//go:build darwin

package app

// app_hotbar_placement_test.go：快捷栏选择与放置路径——只读已确认镜像、命中容器只发打开请求、会话重置。

import (
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestHotbarSelectionOnlySendsRequestAndKeepsMirror(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: true,
	})
	var confirmed core.Hotbar
	confirmed.Selected = 1
	confirmed.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 3}
	sendInteractiveServerMessage(t, serverEndpoint, network.InventoryState{Inventory: core.Inventory{Hotbar: confirmed}})
	app.DrainServerMessages(2)

	app.applyInteractiveInput(0, client.Movement{}, client.Actions{
		Select: true, SelectSlot: 7,
	}, true)

	message := receiveInteractiveClientMessage(t, serverEndpoint)
	if got, ok := message.(network.SelectHotbar); !ok ||
		got != (network.SelectHotbar{Sequence: 1, Slot: 7}) {
		t.Fatalf("选择请求=%#v，想要 Sequence 1 Slot 7", message)
	}
	if got, ok := app.inventory.Hotbar(); !ok || got != confirmed {
		t.Fatalf("未确认的选择改写了镜像: %+v, %v", got, ok)
	}
}

func TestHotbarPlaceUsesLastConfirmedSlot(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: true,
	})
	var confirmed core.Hotbar
	confirmed.Selected = 5
	confirmed.Slots[5] = core.ItemStack{Item: core.ItemDirt, Count: 2}
	sendInteractiveServerMessage(t, serverEndpoint, network.InventoryState{Inventory: core.Inventory{Hotbar: confirmed}})
	app.DrainServerMessages(2)

	app.applyInteractiveInput(0, client.Movement{}, client.Actions{Place: true}, true)

	message := receiveInteractiveClientMessage(t, serverEndpoint)
	place, ok := message.(network.PlaceBlock)
	if !ok || place.Slot != 5 {
		t.Fatalf("放置=%#v，想要引用已确认的栏位 5", message)
	}
	if got, ok := app.inventory.Hotbar(); !ok || got != confirmed {
		t.Fatalf("放置后镜像被本地预测修改: %+v, %v", got, ok)
	}
}

func TestHotbarPlaceWaitsForFirstConfirmedState(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: true,
	})
	app.DrainServerMessages(1)

	app.applyInteractiveInput(0, client.Movement{}, client.Actions{Place: true}, true)

	if app.sequence != 0 {
		t.Fatalf("尚未确认快捷栏就分配 sequence=%d", app.sequence)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

func TestPlaceOpensLocalMirrorFurnaceWithoutPredictingUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 3.5}, OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
	loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 10, Z: 0}, core.FurnaceID)

	app.placeBlock()
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	open, ok := message.(network.OpenContainer)
	if !ok || open != (network.OpenContainer{Sequence: 1}) {
		t.Fatalf("打开熔炉请求 = %#v，想要 sequence 1 与当前视角", message)
	}
	if app.inventoryOpen {
		t.Fatal("服务端确认前本地打开了熔炉界面")
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("打开请求本地改写了熔炉镜像")
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

func TestPlaceKeepsBlockRequestForNonFurnaceHit(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 3.5}, OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
	loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 10, Z: 0}, core.StoneID)
	var inventory core.Inventory
	inventory.Hotbar.Selected = 4
	inventory.Hotbar.Slots[4] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}

	app.placeBlock()
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	place, ok := message.(network.PlaceBlock)
	if !ok || place != (network.PlaceBlock{Sequence: 1, Slot: 4}) {
		t.Fatalf("非熔炉右键请求 = %#v，想要放置已确认栏位 4", message)
	}
}

func TestHotbarMirrorResetsWithClientSession(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	var confirmed core.Hotbar
	confirmed.Slots[0] = core.ItemStack{Item: core.ItemGrass, Count: 1}
	sendInteractiveServerMessage(t, serverEndpoint, network.InventoryState{Inventory: core.Inventory{Hotbar: confirmed}})
	app.DrainServerMessages(1)
	if _, ok := app.inventory.Hotbar(); !ok {
		t.Fatal("权威快捷栏未进入镜像")
	}

	app.CloseClientSession(nil)
	if hotbar, ok := app.inventory.Hotbar(); ok || hotbar != (core.Hotbar{}) {
		t.Fatalf("关闭会话后镜像=%+v, %v，想要空且未确认", hotbar, ok)
	}
}

func TestPlaceOpensLocalMirrorChestWithoutPredictingUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 3.5}, OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
	loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 10, Z: 0}, core.ChestID)

	app.placeBlock()
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	open, ok := message.(network.OpenContainer)
	if !ok || open != (network.OpenContainer{Sequence: 1}) {
		t.Fatalf("打开箱子请求 = %#v，想要 sequence 1 与当前视角", message)
	}
	if app.inventoryOpen {
		t.Fatal("服务端确认前本地打开了箱子界面")
	}
	if _, opened := app.chest.State(); opened {
		t.Fatal("打开请求本地改写了箱子镜像")
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

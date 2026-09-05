//go:build darwin

package app

// app_chest_ui_test.go：箱子界面镜像生命周期与熔炉/箱子互斥。

import (
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func chestTestState() network.ChestState {
	var state network.ChestState
	state.Chest = core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKindChest, Generation: 1}
	return state
}

func TestAuthoritativeChestStateOpensUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := chestTestState()
	state.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	state.Items[26] = core.ItemStack{Item: core.ItemCoal, Count: 1}

	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	got, opened := app.chest.State()
	if !opened || got != state || !app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("权威箱子界面 state=%+v opened=%v ui=%v source=%v",
			got, opened, app.inventoryOpen, app.gameSource)
	}
	app.gameSource = &client.UIGameSlotRef{Area: "inventory", Index: 1}
	state.Items[0].Count++
	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	if app.gameSource == nil || app.gameSource.Index != 1 {
		t.Fatalf("连续权威更新清除了已选来源: %v", app.gameSource)
	}
}

func TestChestTwoClicksSendOneMoveWithoutPrediction(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	var inventory core.Inventory
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemRawIron, Count: 2}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	state := chestTestState()
	state.Chest.Slot, state.Chest.Generation = 2, 3
	if err := app.chest.Apply(state); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true

	app.menu.phase = MenuPhaseGame
	gameTestAction(app, "slot", "inventory", 1)
	assertNoInteractiveClientMessage(t, serverEndpoint)
	gameTestAction(app, "slot", "chest", 5)

	message := receiveInteractiveClientMessage(t, serverEndpoint)
	want := network.MoveContainerStack{
		Sequence: 1, Container: state.Chest, From: 1, To: core.ChestFirstSlot + 5,
	}
	if got, ok := message.(network.MoveContainerStack); !ok || got != want {
		t.Fatalf("跨容器移动 = %#v，想要 %+v", message, want)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if got, _ := app.inventory.State(); got != inventory {
		t.Fatalf("移动请求改写了物品镜像: %+v", got)
	}
	if got, _ := app.chest.State(); got != state {
		t.Fatalf("移动请求改写了箱子镜像: %+v", got)
	}
}

func TestExplicitChestCloseClearsUIAndSendsOnce(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	state := chestTestState()
	if err := app.chest.Apply(state); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.gameSource = &client.UIGameSlotRef{Area: "chest", Index: core.ChestFirstSlot + 3 - core.ChestFirstSlot}

	app.setInventoryOpen(false)
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	if got, ok := message.(network.CloseContainer); !ok || got != (network.CloseContainer{Sequence: 1}) {
		t.Fatalf("关闭箱子请求 = %#v", message)
	}
	app.setInventoryOpen(false)
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("关闭后 ui=%v source=%v", app.inventoryOpen, app.gameSource)
	}
	if !window.CursorCaptured() {
		t.Fatal("关闭箱子后未恢复鼠标捕获")
	}
	if _, opened := app.chest.State(); opened {
		t.Fatal("显式关闭后仍保留箱子镜像")
	}
}

func TestChestClosedMessageClearsUIWithoutEcho(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := chestTestState()
	state.Chest.Slot, state.Chest.Generation = 1, 2
	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	app.gameSource = &client.UIGameSlotRef{Area: "chest", Index: core.ChestFirstSlot - core.ChestFirstSlot}

	sendInteractiveServerMessage(t, serverEndpoint, network.ContainerClosed{Container: state.Chest})
	app.DrainServerMessages(1)
	if app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("服务端关闭后 ui=%v source=%v", app.inventoryOpen, app.gameSource)
	}
	if _, opened := app.chest.State(); opened {
		t.Fatal("服务端关闭后仍保留箱子镜像")
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

// 杀死变异：熔炉与箱子镜像必须互斥；否则点击分流会用错容器，
// 或者渲染会同时按两种叠加值布局。
func TestNewContainerStateClearsStaleMirrorOfOtherKind(t *testing.T) {
	t.Run("箱子状态到达时清除旧熔炉镜像", func(t *testing.T) {
		app, serverEndpoint := newInteractiveTestApplication(t)
		furnaceState := network.FurnaceState{
			Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
		}
		sendInteractiveServerMessage(t, serverEndpoint, furnaceState)
		app.DrainServerMessages(1)
		if _, opened := app.furnace.State(); !opened {
			t.Fatal("熔炉状态未进入镜像")
		}

		chestState := chestTestState()
		chestState.Chest.Slot = 1
		sendInteractiveServerMessage(t, serverEndpoint, chestState)
		app.DrainServerMessages(1)
		if _, opened := app.furnace.State(); opened {
			t.Fatal("新箱子状态到达后仍保留过期熔炉镜像")
		}
		if got, opened := app.chest.State(); !opened || got != chestState {
			t.Fatalf("箱子镜像 = %+v, opened=%v", got, opened)
		}
	})

	t.Run("熔炉状态到达时清除旧箱子镜像", func(t *testing.T) {
		app, serverEndpoint := newInteractiveTestApplication(t)
		chestState := chestTestState()
		sendInteractiveServerMessage(t, serverEndpoint, chestState)
		app.DrainServerMessages(1)
		if _, opened := app.chest.State(); !opened {
			t.Fatal("箱子状态未进入镜像")
		}

		furnaceState := network.FurnaceState{
			Furnace: core.FurnaceRef{Dimension: core.Overworld, Slot: 1, Generation: 1},
		}
		sendInteractiveServerMessage(t, serverEndpoint, furnaceState)
		app.DrainServerMessages(1)
		if _, opened := app.chest.State(); opened {
			t.Fatal("新熔炉状态到达后仍保留过期箱子镜像")
		}
		if got, opened := app.furnace.State(); !opened || got != furnaceState {
			t.Fatalf("熔炉镜像 = %+v, opened=%v", got, opened)
		}
	})
}

func TestPlayerResetClosesChestUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.chest.Apply(chestTestState()); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.gameSource = &client.UIGameSlotRef{Area: "chest", Index: core.ChestFirstSlot - core.ChestFirstSlot}
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
	})

	app.DrainServerMessages(1)
	if app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("reset 后 ui=%v source=%v", app.inventoryOpen, app.gameSource)
	}
	if _, opened := app.chest.State(); opened {
		t.Fatal("reset 后仍保留箱子镜像")
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
}

func TestClientSessionCloseClearsChestMirror(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	if err := app.chest.Apply(chestTestState()); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.gameSource = &client.UIGameSlotRef{Area: "chest", Index: core.ChestFirstSlot - core.ChestFirstSlot}

	app.CloseClientSession(nil)
	if app.inventoryOpen || app.gameSource != nil {
		t.Fatalf("断线后 open=%v source=%v，想要界面关闭且来源清除", app.inventoryOpen, app.gameSource)
	}
	if _, opened := app.chest.State(); opened {
		t.Fatal("断线后仍保留箱子镜像")
	}
}

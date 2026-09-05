//go:build darwin

package app

// app_inventory_crafting_test.go：合成视图点击输入与权威镜像生命周期——
// 格点击组 MoveCraftingStack、产物格点击发 TakeCraftingOutput、确认前不本地
// 改写、工作台尺寸 3 的权威状态开/关界面与 use-key 交互。

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/go-gl/mathgl/mgl32"
)

// TestCraftingStateSizeThreeOpensWorkbenchUI 锁定工作台界面的权威驱动：收到
// 尺寸 3 的网格状态才打开 3×3 视图，连续尺寸 3 更新不清除已选来源，尺寸降级
// 视为服务端关闭通知——关闭界面、清除来源并重新捕获鼠标。
func TestCraftingStateSizeThreeOpensWorkbenchUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	app.setInventoryOpen(true)

	state := network.CraftingState{Size: 3}
	state.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	got, confirmed := app.crafting.State()
	if !confirmed || got != state {
		t.Fatalf("工作台状态 = %+v, %v，想要 %+v/true", got, confirmed, state)
	}
	if !app.inventoryOpen {
		t.Fatal("尺寸 3 的权威状态没有打开界面")
	}
	if window.CursorCaptured() {
		t.Fatal("工作台界面打开后仍捕获鼠标")
	}

	app.inventorySource = 7
	next := state
	next.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 3}
	sendInteractiveServerMessage(t, serverEndpoint, next)
	app.DrainServerMessages(1)
	if app.inventorySource != 7 {
		t.Fatalf("连续尺寸 3 更新清除了已选来源: %d", app.inventorySource)
	}
	if !app.inventoryOpen {
		t.Fatal("连续尺寸 3 更新关闭了界面")
	}

	// 尺寸降级 = 服务端关闭通知（关闭、断线或工作台失效后的权威回收）。
	app.inventorySource = 2
	sendInteractiveServerMessage(t, serverEndpoint, network.CraftingState{Size: 2})
	app.DrainServerMessages(1)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("尺寸降级后 open=%v source=%d，想要界面关闭且来源清除",
			app.inventoryOpen, app.inventorySource)
	}
	if !window.CursorCaptured() {
		t.Fatal("尺寸降级后未恢复鼠标捕获")
	}
	// 降级不清除个人网格镜像：随后的 2×2 状态仍是已确认的权威值。
	if got, confirmed := app.crafting.State(); !confirmed || got.Size != 2 {
		t.Fatalf("降级后网格镜像 = %+v, %v，想要保持已确认的尺寸 2", got, confirmed)
	}
}

// TestWorkbenchCloseSendsCloseContainerAndClearsUI 锁定显式关闭：工作台视图下
// 按 E/Escape 关闭必须发送 CloseContainer（服务端据此回收格 4..8 并降尺寸），
// 并清除本地界面状态与网格镜像（重新等待权威状态）。
func TestWorkbenchCloseSendsCloseContainerAndClearsUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	window := &fakeInteractiveWindow{}
	app.window = window
	if err := app.crafting.Apply(network.CraftingState{Size: 3}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.inventorySource = 4

	app.setInventoryOpen(false)
	message := receiveInteractiveClientMessage(t, serverEndpoint)
	if got, ok := message.(network.CloseContainer); !ok || got.Sequence != 1 {
		t.Fatalf("关闭请求 = %#v，想要 CloseContainer 序号 1", message)
	}
	assertNoInteractiveClientMessage(t, serverEndpoint)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("关闭后 open=%v source=%d", app.inventoryOpen, app.inventorySource)
	}
	if !window.CursorCaptured() {
		t.Fatal("关闭工作台后未恢复鼠标捕获")
	}
	if _, confirmed := app.crafting.State(); confirmed {
		t.Fatal("关闭工作台后仍保留尺寸 3 镜像")
	}
}

// TestPlayerResetClosesWorkbenchUI 锁定玩家状态 reset 对工作台界面的清理：
// reset 走既有 clearContainerUI 路径，界面关闭、来源清除且尺寸 3 镜像不复用。
func TestPlayerResetClosesWorkbenchUI(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	if err := app.crafting.Apply(network.CraftingState{Size: 3}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.inventorySource = 8
	sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
	})
	app.DrainServerMessages(1)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("reset 后 open=%v source=%d，想要界面关闭且来源清除",
			app.inventoryOpen, app.inventorySource)
	}
	if _, confirmed := app.crafting.State(); confirmed {
		t.Fatal("reset 后仍保留网格镜像")
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

	app.DrainServerMessages(1)
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
	if err := app.crafting.Apply(network.CraftingState{Size: 3}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen = true
	app.inventorySource = 8

	app.CloseClientSession(nil)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("断线后 open=%v source=%d，想要界面关闭且来源清除", app.inventoryOpen, app.inventorySource)
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("断线后仍保留熔炉镜像")
	}
	if _, confirmed := app.crafting.State(); confirmed {
		t.Fatal("断线后仍保留网格镜像")
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
	app.gameSource = &client.UIGameSlotRef{Area: "inventory", Index: 5}

	app.setInventoryOpen(false)
	if app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("关闭后 open=%v source=%d", app.inventoryOpen, app.inventorySource)
	}
	if !window.CursorCaptured() {
		t.Fatal("关闭背包后未恢复鼠标捕获")
	}
}

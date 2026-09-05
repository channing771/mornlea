//go:build darwin

package app

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestGameSemanticClicksWaitForAuthorityAndRejectStaleView(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	before := core.Inventory{}
	before.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	if err := a.inventory.Apply(network.InventoryState{Inventory: before}); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(true)
	state := a.buildGameUIState()
	a.handleGameAction(client.UIGameAction{Token: state.Token, Op: "slot", Area: "inventory", Index: 0})
	assertNoInteractiveClientMessage(t, endpoint)
	a.handleGameAction(client.UIGameAction{Token: state.Token, Op: "slot", Area: "inventory", Index: 10})
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.MoveInventoryStack); !ok || got.From != 0 || got.To != 10 {
		t.Fatalf("移动: %#v", got)
	}
	after, _ := a.inventory.State()
	if after != before {
		t.Fatal("点击改写权威镜像")
	}
	a.setInventoryOpen(false)
	a.setInventoryOpen(true)
	a.handleGameAction(client.UIGameAction{Token: state.Token, Op: "slot", Area: "inventory", Index: 1})
	if a.gameSource != nil {
		t.Fatal("过期来源泄漏")
	}
	a.menu.phase = menuPhasePaused
	a.handleGameAction(client.UIGameAction{Token: a.buildGameUIState().Token, Op: "hotbar", Index: 1})
	assertNoInteractiveClientMessage(t, endpoint)
}

func TestGameCraftingMovesAndOutputRemainAuthoritative(t *testing.T) {
	for _, size := range []uint8{2, 3} {
		t.Run(string(rune('0'+size)), func(t *testing.T) {
			a, endpoint := newInteractiveTestApplication(t)
			a.menu.phase = MenuPhaseGame
			if err := a.inventory.Apply(network.InventoryState{}); err != nil {
				t.Fatal(err)
			}
			grid := network.CraftingState{Size: size, Output: core.ItemStack{Item: core.ItemStoneBrick, Count: 4}}
			grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
			if err := a.crafting.Apply(grid); err != nil {
				t.Fatal(err)
			}
			a.setInventoryOpen(true)
			gameTestAction(a, "slot", "crafting", 0)
			assertNoInteractiveClientMessage(t, endpoint)
			gameTestAction(a, "slot", "inventory", 1)
			if got := receiveInteractiveClientMessage(t, endpoint); got != (network.MoveCraftingStack{Sequence: 1, From: 0, To: 10}) {
				t.Fatalf("网格移动: %#v", got)
			}
			gameTestAction(a, "slot", "crafting", 0)
			gameTestAction(a, "slot", "crafting", int(size*size-1))
			if got := receiveInteractiveClientMessage(t, endpoint); got != (network.MoveCraftingStack{Sequence: 2, From: 0, To: size*size - 1}) {
				t.Fatalf("网格内部移动: %#v", got)
			}
			gameTestAction(a, "take-output", "", 0)
			if got := receiveInteractiveClientMessage(t, endpoint); got != (network.TakeCraftingOutput{Sequence: 3}) {
				t.Fatalf("产物请求: %#v", got)
			}
			after, _ := a.crafting.State()
			if after != grid {
				t.Fatal("点击预测修改网格")
			}
			for i := int(size * size); i < 9; i++ {
				gameTestAction(a, "slot", "crafting", i)
				if a.gameSource != nil {
					t.Fatal("扩展格被选中")
				}
			}
			grid.Output = core.ItemStack{}
			if err := a.crafting.Apply(grid); err != nil {
				t.Fatal(err)
			}
			gameTestAction(a, "take-output", "", 0)
			gameTestAction(a, "slot", "output", 0)
			assertNoInteractiveClientMessage(t, endpoint)
		})
	}
}

func TestGameUnconfirmedClosedAndDebugPanelsRejectCommands(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	a.setInventoryOpen(true)
	gameTestAction(a, "slot", "inventory", 0)
	gameTestAction(a, "slot", "inventory", 1)
	gameTestAction(a, "take-output", "", 0)
	if a.gameSource != nil {
		t.Fatal("未确认时记录来源")
	}
	if err := a.inventory.Apply(network.InventoryState{}); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(false)
	gameTestAction(a, "slot", "inventory", 0)
	gameTestAction(a, "slot", "inventory", 1)
	if a.gameSource != nil {
		t.Fatal("关闭后记录来源")
	}
	a.clientSessionClosed = true
	gameTestAction(a, "inventory", "", 0)
	if a.inventoryOpen {
		t.Fatal("已结束会话打开面板")
	}
	assertNoInteractiveClientMessage(t, endpoint)
}

func TestGameFurnaceOutputRejectsDestinationButAllowsWithdrawal(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	if err := a.inventory.Apply(network.InventoryState{}); err != nil {
		t.Fatal(err)
	}
	furnace := network.FurnaceState{Furnace: core.FurnaceRef{Generation: 1}, Output: core.ItemStack{Item: core.ItemIronIngot, Count: 1}}
	if err := a.furnace.Apply(furnace); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(true)
	gameTestAction(a, "slot", "inventory", 0)
	gameTestAction(a, "slot", "furnace", 2)
	assertNoInteractiveClientMessage(t, endpoint)
	if a.gameSource == nil || a.gameSource.Area != "inventory" {
		t.Fatal("非法目标不得破坏来源")
	}
	gameTestAction(a, "slot", "inventory", 0)
	gameTestAction(a, "slot", "furnace", 2)
	gameTestAction(a, "slot", "inventory", 1)
	got := receiveInteractiveClientMessage(t, endpoint)
	if got != (network.MoveContainerStack{Sequence: 1, Container: furnace.Furnace, From: core.FurnaceOutputSlot, To: 1}) {
		t.Fatalf("熔炉取出: %#v", got)
	}
}

func TestGameBackpackOnlyConfirmationRepublishesState(t *testing.T) {
	a, window := newHUDPushTestApplication(t)
	a.menu.phase = MenuPhaseGame
	a.initHUDPush()
	a.syncHUDPushWindow()
	a.hudPush.Mark()
	a.flushHUDState()
	a.pushUIStateIfChanged()
	initial := len(window.pushedUIStates)
	// 快捷栏未变的权威背包更新仍必须到达独立游戏分节。
	inventory := core.Inventory{}
	inventory, ok := inventory.SetSlot(20, core.ItemStack{Item: core.ItemStone, Count: 3})
	if !ok {
		t.Fatal("夹具")
	}
	if err := a.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	a.gameUIDirty = true
	a.hudPush.Mark()
	a.flushHUDState()
	a.pushUIStateIfChanged()
	if len(window.pushedUIStates) <= initial {
		t.Fatal("纯背包变化被HUD去重吞掉")
	}
	state := a.buildGameUIState()
	if state.Inventory[20].Count != 3 {
		t.Fatal("权威背包未下行")
	}
}

func TestGameEventsDrainWithoutDeveloperPanel(t *testing.T) {
	a, _ := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	a.gameCursorFree = true
	token := a.buildGameUIState().Token
	source := &gameEventDrainer{events: []client.UIEvent{{Kind: client.UIEventGameAction, GameAction: client.UIGameAction{Token: token, Op: "inventory"}}}}
	a.drainGameUIEvents(source)
	if source.drains != 1 || !a.inventoryOpen {
		t.Fatal("非dev游戏未消费事件")
	}
}

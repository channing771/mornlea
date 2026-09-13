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

// TestGameRightClickSplitSendsPartialOnceAndWaitForAuthority 钉住右键两击的
// 部分移动语义：首击只记录来源，第二击按其 Shift 位在半组/单件两档间定档，
// 数量由服务端推导，确认前镜像逐格不变。
func TestGameRightClickSplitSendsPartialOnceAndWaitForAuthority(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	before := core.Inventory{}
	before.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 3}
	if err := a.inventory.Apply(network.InventoryState{Inventory: before}); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(true)
	// 右键首击：只记录来源，不发命令。
	gameTestPointerAction(a, "slot", "inventory", 0, "right", false)
	assertNoInteractiveClientMessage(t, endpoint)
	if a.gameSource == nil || a.gameSource.Area != "inventory" || a.gameSource.Index != 0 {
		t.Fatalf("右键首击未记录来源: %#v", a.gameSource)
	}
	// 右键二击（无 Shift）：半组档，背包内部走背包视图域。
	gameTestPointerAction(a, "slot", "inventory", 10, "right", false)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.MoveStackPartial); !ok ||
		got.View != network.StackViewInventory || got.From != 0 || got.To != 10 || got.Single {
		t.Fatalf("半组请求: %#v", got)
	}
	if a.gameSource != nil {
		t.Fatal("部分移动后来源未清除")
	}
	if after, _ := a.inventory.State(); after != before {
		t.Fatal("部分移动点击改写权威镜像")
	}
	// Shift+右键二击：单件档。
	gameTestPointerAction(a, "slot", "inventory", 0, "right", false)
	gameTestPointerAction(a, "slot", "inventory", 10, "right", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.MoveStackPartial); !ok ||
		got.View != network.StackViewInventory || got.From != 0 || got.To != 10 || !got.Single {
		t.Fatalf("单件请求: %#v", got)
	}
	// Shift+右键首击同样只记录来源：定档只由第二击决定。
	gameTestPointerAction(a, "slot", "inventory", 0, "right", true)
	assertNoInteractiveClientMessage(t, endpoint)
	if a.gameSource == nil {
		t.Fatal("Shift+右键首击未记录来源")
	}
	// 混合键序以第二击类型定档：右键选源后左键二击仍发整堆命令。
	gameTestPointerAction(a, "slot", "inventory", 10, "left", false)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.MoveInventoryStack); !ok || got.From != 0 || got.To != 10 {
		t.Fatalf("混合键序整堆请求: %#v", got)
	}
	// 右键二击同格：与整堆一致的取消语义，不发命令。
	gameTestPointerAction(a, "slot", "inventory", 0, "right", false)
	gameTestPointerAction(a, "slot", "inventory", 0, "right", false)
	assertNoInteractiveClientMessage(t, endpoint)
}

// TestGameRightClickPartialAcrossGridAndContainers 钉住部分移动的视图域分派：
// 触及网格的移动走合成视图，容器面板走容器视图并携带权威引用。
func TestGameRightClickPartialAcrossGridAndContainers(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	if err := a.inventory.Apply(network.InventoryState{}); err != nil {
		t.Fatal(err)
	}
	grid := network.CraftingState{Size: 3}
	grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	if err := a.crafting.Apply(grid); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(true)
	// 工作台：网格格 → 背包格走合成视图统一索引。
	gameTestPointerAction(a, "slot", "crafting", 0, "right", false)
	gameTestPointerAction(a, "slot", "inventory", 0, "right", false)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.MoveStackPartial); !ok ||
		got.View != network.StackViewCrafting || got.From != 0 || got.To != 9 {
		t.Fatalf("合成视图半组请求: %#v", got)
	}
	// 直接切换到箱子视图（容器开着的真实路径是权威下发新状态）：避免
	// 关闭面板触发 CloseContainer 挡在断言前面。
	chest := network.ChestState{Chest: core.ContainerRef{Kind: core.ContainerKindChest, Generation: 1}}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	if err := a.chest.Apply(chest); err != nil {
		t.Fatal(err)
	}
	gameTestPointerAction(a, "slot", "chest", 0, "right", true)
	gameTestPointerAction(a, "slot", "inventory", 0, "right", false)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.MoveStackPartial); !ok ||
		got.Container != chest.Chest || got.View != network.StackViewContainer || got.From != 36 || got.To != 0 {
		t.Fatalf("箱子视图单件请求: %#v", got)
	}
}

// TestGameRightClickPartialRejectsFurnaceOutputTarget 钉住部分移动沿既有熔炉
// 约束：输出格不得作为目标，非法目标不得破坏来源。
func TestGameRightClickPartialRejectsFurnaceOutputTarget(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	if err := a.inventory.Apply(network.InventoryState{}); err != nil {
		t.Fatal(err)
	}
	furnace := network.FurnaceState{Furnace: core.FurnaceRef{Generation: 1}}
	furnace.Input = core.ItemStack{Item: core.ItemRawIron, Count: 4}
	if err := a.furnace.Apply(furnace); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(true)
	gameTestPointerAction(a, "slot", "furnace", 0, "right", false)
	gameTestPointerAction(a, "slot", "furnace", 2, "right", false)
	assertNoInteractiveClientMessage(t, endpoint)
	if a.gameSource == nil || a.gameSource.Area != "furnace" {
		t.Fatal("非法目标破坏了部分移动来源")
	}
}

// TestGameShiftLeftClickQuickMoveClearsSourceAndMapsView 钉住 Shift+左键单击
// 的快捷搬运：无视既有来源直发一次请求，视图域按当前面板身份映射（容器
// 面板带权威引用；工作台用统一合成视图索引）。
func TestGameShiftLeftClickQuickMoveClearsSourceAndMapsView(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	if err := a.inventory.Apply(network.InventoryState{}); err != nil {
		t.Fatal(err)
	}
	grid := network.CraftingState{Size: 3}
	grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	if err := a.crafting.Apply(grid); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(true)
	// 预置来源：快捷搬运必须忽略并清除它。
	gameTestAction(a, "slot", "crafting", 1)
	if a.gameSource == nil {
		t.Fatal("夹具：来源未记录")
	}
	gameTestPointerAction(a, "slot", "crafting", 0, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.View != network.StackViewCrafting || got.From != 0 || got.Container != (core.ContainerRef{}) {
		t.Fatalf("工作台网格快捷搬运: %#v", got)
	}
	if a.gameSource != nil {
		t.Fatal("快捷搬运未清除既有来源")
	}
	// 背包格按统一合成视图 +9 映射。
	gameTestPointerAction(a, "slot", "inventory", 5, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.View != network.StackViewCrafting || got.From != 14 {
		t.Fatalf("工作台背包快捷搬运: %#v", got)
	}
	// 直接经权威状态切换视图（避免关闭面板触发 CloseContainer 挡在断言
	// 前面）：箱子优先于熔炉成为当前视图。
	chest := network.ChestState{Chest: core.ContainerRef{Kind: core.ContainerKindChest, Generation: 1}}
	if err := a.chest.Apply(chest); err != nil {
		t.Fatal(err)
	}
	furnace := network.FurnaceState{Furnace: core.FurnaceRef{Generation: 2}}
	if err := a.furnace.Apply(furnace); err != nil {
		t.Fatal(err)
	}
	gameTestPointerAction(a, "slot", "inventory", 0, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.Container != chest.Chest || got.View != network.StackViewContainer || got.From != 0 {
		t.Fatalf("箱子背包快捷搬运: %#v", got)
	}
	gameTestPointerAction(a, "slot", "chest", 3, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.Container != chest.Chest || got.View != network.StackViewContainer || got.From != 39 {
		t.Fatalf("箱内快捷搬运: %#v", got)
	}
	// 镜像层关闭箱子即切换到熔炉视图，不产生协议消息。
	if err := a.chest.Close(network.ContainerClosed{Container: chest.Chest}); err != nil {
		t.Fatal(err)
	}
	gameTestPointerAction(a, "slot", "furnace", 1, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.Container != furnace.Furnace || got.View != network.StackViewContainer || got.From != 37 {
		t.Fatalf("熔炉快捷搬运: %#v", got)
	}
	// 快捷搬运从输出格取回是合法来源方向，不受输出格目标约束影响。
	gameTestPointerAction(a, "slot", "furnace", 2, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.From != core.FurnaceOutputSlot {
		t.Fatalf("熔炉输出快捷搬运: %#v", got)
	}
}

// TestGamePersonalPanelQuickMoveRoutesInventoryView 钉住个人背包面板（2×2）
// 的快捷搬运视图域分派：背包/快捷栏格走背包视图域原始 0..35 索引（服务端
// 据此走纯背包面板互移），网格格仍走合成视图统一网格索引；工作台背包格
// 的合成视图 +9 映射不变。
func TestGamePersonalPanelQuickMoveRoutesInventoryView(t *testing.T) {
	a, endpoint := newInteractiveTestApplication(t)
	a.menu.phase = MenuPhaseGame
	if err := a.inventory.Apply(network.InventoryState{}); err != nil {
		t.Fatal(err)
	}
	grid := network.CraftingState{Size: 2}
	grid.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 4}
	if err := a.crafting.Apply(grid); err != nil {
		t.Fatal(err)
	}
	a.setInventoryOpen(true)
	if state := a.buildGameUIState(); state.Kind != "inventory" {
		t.Fatalf("夹具：视图身份 %q", state.Kind)
	}
	// 个人面板背包格：背包视图域原始索引，而非合成视图 +9 映射。
	gameTestPointerAction(a, "slot", "inventory", 12, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.View != network.StackViewInventory || got.From != 12 || got.Container != (core.ContainerRef{}) {
		t.Fatalf("个人面板背包快捷搬运: %#v", got)
	}
	// 个人面板快捷栏格同样走背包视图域。
	gameTestPointerAction(a, "slot", "inventory", 0, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.View != network.StackViewInventory || got.From != 0 {
		t.Fatalf("个人面板快捷栏快捷搬运: %#v", got)
	}
	// 个人面板 2×2 网格格：仍走合成视图统一网格索引。
	gameTestPointerAction(a, "slot", "crafting", 1, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.View != network.StackViewCrafting || got.From != 1 {
		t.Fatalf("个人面板网格快捷搬运: %#v", got)
	}
	// 工作台背包格：网格升级为 3×3 后保持合成视图 +9 统一映射不变。
	workbench := network.CraftingState{Size: 3}
	if err := a.crafting.Apply(workbench); err != nil {
		t.Fatal(err)
	}
	gameTestPointerAction(a, "slot", "inventory", 5, "left", true)
	if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.QuickMoveStack); !ok ||
		got.View != network.StackViewCrafting || got.From != 14 {
		t.Fatalf("工作台背包快捷搬运: %#v", got)
	}
}

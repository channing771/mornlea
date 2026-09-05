//go:build darwin

package app

import (
	"fmt"
	"log/slog"

	"github.com/channing771/mornlea/packages/client/audio"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

var gameRecipeIDs = [...]core.RecipeID{core.RecipeStoneBricks, core.RecipeFurnace, core.RecipeIronBlock, core.RecipeStonePickaxe, core.RecipeIronPickaxe, core.RecipeChest, core.RecipeOakPlanks, core.RecipeLightBlock, core.RecipeStoneHoe, core.RecipeIronHoe}

func (a *Application) invalidateGameView() {
	a.gameToken++
	a.gameSource = nil
	a.gameRecipeIndex = -1
	a.gameUIDirty = true
}

func (a *Application) gameViewIdentity() (string, string) {
	if !a.inventoryOpen {
		return "none", "none"
	}
	if state, ok := a.chest.State(); ok {
		return "chest", fmt.Sprintf("chest:%v", state.Chest)
	}
	if state, ok := a.furnace.State(); ok {
		return "furnace", fmt.Sprintf("furnace:%v", state.Furnace)
	}
	if state, ok := a.crafting.State(); ok && state.Size == 3 {
		return "workbench", "workbench"
	}
	if a.gameCharacter {
		return "character", "character"
	}
	return "inventory", "inventory"
}

func (a *Application) syncGameView() string {
	kind, identity := a.gameViewIdentity()
	if a.gameToken == 0 || identity != a.gameIdentity {
		a.invalidateGameView()
		a.gameIdentity = identity
	}
	return kind
}

func (a *Application) buildGameUIState() *client.UIGameState {
	kind := a.syncGameView()
	state := &client.UIGameState{Token: a.gameToken, Kind: kind, CursorFree: a.gameCursorFree || a.inventoryOpen, GridSize: 2, Source: a.gameSource, RecipeIndex: a.gameRecipeIndex}
	inventory, confirmed := a.inventory.State()
	state.Confirmed = confirmed
	if confirmed {
		for i := range state.Inventory {
			stack, _ := inventory.Slot(uint8(i))
			state.Inventory[i] = client.NewUIGameSlot(stack)
		}
	}
	if grid, ok := a.crafting.State(); ok {
		state.GridSize = int(grid.Size)
		for i, stack := range grid.Slots {
			state.Grid[i] = client.NewUIGameSlot(stack)
		}
		state.Output = client.NewUIGameSlot(grid.Output)
	}
	if chest, ok := a.chest.State(); ok {
		for i, stack := range chest.Items {
			state.Chest[i] = client.NewUIGameSlot(stack)
		}
	}
	if furnace, ok := a.furnace.State(); ok {
		state.Furnace = [3]client.UIHudSlot{client.NewUIGameSlot(furnace.Input), client.NewUIGameSlot(furnace.Fuel), client.NewUIGameSlot(furnace.Output)}
		state.Progress = float32(furnace.ProgressTicks) / float32(core.FurnaceSmeltTicks)
		state.Burn = float32(furnace.BurnTicks) / float32(core.FurnaceBurnTicks)
	}
	for i, id := range gameRecipeIDs {
		pattern, _ := core.Recipe(id)
		recipe := client.UIGameRecipe{Size: 3, Output: client.NewUIGameSlot(pattern.Output)}
		recipe.Name = recipe.Output.Name
		for j, item := range pattern.Cells {
			stack := core.ItemStack{Item: item}
			if item != 0 {
				stack.Count = 1
			}
			recipe.Slots[j] = client.NewUIGameSlot(stack)
		}
		state.Recipes[i] = recipe
	}
	return state
}

func (a *Application) setGameCursorFree(free bool) {
	if a.gameCursorFree == free {
		return
	}
	a.gameCursorFree = free
	a.invalidateGameView()
	if a.window != nil {
		a.window.SetCursorCaptured(!free)
	}
	a.applyInteractiveInput(0, client.Movement{}, client.Actions{}, false)
}

// handleGameAction 只把合法、当前视图的语义事件转换为既有权威请求。
func (a *Application) handleGameAction(action client.UIGameAction) {
	kind := a.syncGameView()
	if action.Token != a.gameToken || a.menu.phase != MenuPhaseGame || a.clientSessionClosed || a.panelVisible() || a.chatInput.open {
		return
	}
	switch action.Op {
	case "close":
		if a.inventoryOpen {
			a.setInventoryOpen(false)
		}
		return
	case "capture":
		if !a.inventoryOpen {
			a.setGameCursorFree(false)
		}
		return
	case "inventory", "character":
		if kind == "none" && action.Op == "inventory" {
			a.setInventoryOpen(true)
			return
		}
		if kind == "inventory" || kind == "character" {
			a.gameCharacter = action.Op == "character"
			a.invalidateGameView()
		}
		return
	}
	if _, confirmed := a.inventory.State(); !confirmed {
		return
	}
	if action.Op == "hotbar" {
		if (a.inventoryOpen || a.gameCursorFree) && action.Index >= 0 && action.Index < 9 {
			a.selectHotbarSlot(uint8(action.Index))
		}
		return
	}
	if !a.inventoryOpen || kind == "character" {
		return
	}
	if action.Op == "recipe" {
		if (kind == "inventory" || kind == "workbench") && action.Index >= 0 && action.Index < 10 {
			a.gameRecipeIndex = action.Index
			a.gameUIDirty = true
		}
		return
	}
	if action.Op == "take-output" {
		if kind != "inventory" && kind != "workbench" {
			return
		}
		grid, ok := a.crafting.State()
		if !ok || grid.Output.Item == 0 {
			return
		}
		a.gameSource = nil
		a.gameUIDirty = true
		a.sendGameCommand(network.TakeCraftingOutput{Sequence: a.nextSequence()})
		return
	}
	if action.Op != "slot" {
		return
	}
	target := client.UIGameSlotRef{Area: action.Area, Index: action.Index}
	to, valid := a.gameUnifiedSlot(kind, target)
	if !valid {
		return
	}
	if a.gameSource == nil {
		a.gameSource = &target
		a.gameUIDirty = true
		a.playLocalCue(audio.CueUIClick)
		return
	}
	from, valid := a.gameUnifiedSlot(kind, *a.gameSource)
	if !valid {
		a.gameSource = nil
		a.gameUIDirty = true
		return
	}
	if kind == "furnace" && to == core.FurnaceOutputSlot && from != to {
		return
	}
	a.gameSource = nil
	a.gameUIDirty = true
	if from == to {
		a.playLocalCue(audio.CueUIClick)
		return
	}
	if kind == "chest" {
		state, _ := a.chest.State()
		a.sendGameCommand(network.MoveContainerStack{Sequence: a.nextSequence(), Container: state.Chest, From: from, To: to})
		return
	}
	if kind == "furnace" {
		state, _ := a.furnace.State()
		a.sendGameCommand(network.MoveContainerStack{Sequence: a.nextSequence(), Container: state.Furnace, From: from, To: to})
		return
	}
	if from >= 9 && to >= 9 {
		a.sendGameCommand(network.MoveInventoryStack{Sequence: a.nextSequence(), From: from - 9, To: to - 9})
		return
	}
	a.sendGameCommand(network.MoveCraftingStack{Sequence: a.nextSequence(), From: from, To: to})
}

func (a *Application) gameUnifiedSlot(kind string, slot client.UIGameSlotRef) (uint8, bool) {
	if slot.Index < 0 {
		return 0, false
	}
	switch slot.Area {
	case "inventory":
		if slot.Index >= 36 {
			return 0, false
		}
		if kind == "inventory" || kind == "workbench" {
			return uint8(slot.Index + 9), true
		}
		return uint8(slot.Index), kind == "chest" || kind == "furnace"
	case "crafting":
		grid, ok := a.crafting.State()
		return uint8(slot.Index), ok && (kind == "inventory" || kind == "workbench") && slot.Index < int(grid.Size)*int(grid.Size)
	case "chest":
		return uint8(36 + slot.Index), kind == "chest" && slot.Index < 27
	case "furnace":
		return uint8(36 + slot.Index), kind == "furnace" && slot.Index < 3
	}
	return 0, false
}

func (a *Application) sendGameCommand(command network.ClientMessage) {
	if err := a.send(command); err != nil {
		slog.Warn("发送面板操作失败", "error", err)
		return
	}
	a.playLocalCue(audio.CueUIClick)
}

// drainGameUIEvents 在非开发游戏窗口同样消费语义输入，调试事件保留原序。
func (a *Application) drainGameUIEvents(source interface{ DrainUIEvents() []client.UIEvent }) []client.UIEvent {
	events := source.DrainUIEvents()
	for _, event := range events {
		if event.Kind == client.UIEventGameAction {
			a.handleGameAction(event.GameAction)
		}
	}
	return events
}

//go:build darwin

package main

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/go-gl/mathgl/mgl32"
)

// app_audio_test.go 覆盖本地音频只消费已确认客户端边界，绝不从预测或拒绝路径发声。

type audioCueRecorder []audio.Cue

func (recorder *audioCueRecorder) play(cue audio.Cue) {
	*recorder = append(*recorder, cue)
}

func (recorder audioCueRecorder) want(t *testing.T, cues ...audio.Cue) {
	t.Helper()
	if len(recorder) != len(cues) {
		t.Fatalf("cue=%v，想要 %v", recorder, cues)
	}
	for index := range cues {
		if recorder[index] != cues[index] {
			t.Fatalf("cue[%d]=%v，想要 %v", index, recorder[index], cues[index])
		}
	}
}

func TestLocalAudioUIClicksOnlyForEffectiveActions(t *testing.T) {
	app, endpoint := newInteractiveTestApplication(t)
	var recorder audioCueRecorder
	app.playCue = recorder.play
	width, height := uint32(1280), uint32(720)

	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 4}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	recipeX, recipeY := recipeButtonCenter(t, core.RecipeStoneBricks, width, height)
	app.clickInventorySlot(recipeX, recipeY, width, height)
	if _, ok := receiveInteractiveClientMessage(t, endpoint).(network.CraftRecipe); !ok {
		t.Fatal("可合成按钮未发送 CraftRecipe")
	}

	sourceX, sourceY := inventorySlotCenter(t, 1, width, height)
	targetX, targetY := inventorySlotCenter(t, 2, width, height)
	app.clickInventorySlot(sourceX, sourceY, width, height)
	app.clickInventorySlot(sourceX, sourceY, width, height)
	app.clickInventorySlot(sourceX, sourceY, width, height)
	app.clickInventorySlot(targetX, targetY, width, height)
	if _, ok := receiveInteractiveClientMessage(t, endpoint).(network.MoveInventoryStack); !ok {
		t.Fatal("有效第二次点击未发送 MoveInventoryStack")
	}
	recorder.want(t, audio.CueUIClick, audio.CueUIClick, audio.CueUIClick, audio.CueUIClick, audio.CueUIClick)

	app.clickInventorySlot(0, 0, width, height)
	if err := app.inventory.Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
		t.Fatal(err)
	}
	app.clickInventorySlot(recipeX, recipeY, width, height)
	app.inventorySource = core.FurnaceInputSlot
	if err := app.furnace.Apply(network.FurnaceState{
		Furnace: core.FurnaceRef{Dimension: core.Overworld, Generation: 1},
	}); err != nil {
		t.Fatal(err)
	}
	outputX, outputY := furnaceSlotCenter(t, core.FurnaceOutputSlot, width, height)
	app.clickInventorySlot(outputX, outputY, width, height)
	_ = app.clientEndpoint.Close()
	app.furnace.Reset()
	app.inventorySource = 1
	app.clickInventorySlot(targetX, targetY, width, height)
	recorder.want(t, audio.CueUIClick, audio.CueUIClick, audio.CueUIClick, audio.CueUIClick, audio.CueUIClick)

	silentApp, _ := newInteractiveTestApplication(t)
	var silent audioCueRecorder
	silentApp.playCue = silent.play
	silentApp.clickInventorySlot(recipeX, recipeY, width, height)
	silent.want(t)
}

func TestLocalAudioDamageOnlyAfterConfirmedHealthDecrease(t *testing.T) {
	app, endpoint := newInteractiveTestApplication(t)
	var recorder audioCueRecorder
	app.playCue = recorder.play

	for _, state := range []network.PlayerState{
		audioPlayerState(1, 20, 10, true),
		audioPlayerState(2, 20, 10, false),
		audioPlayerState(3, 20, 10, false),
		audioPlayerState(4, 19, 10, false),
		audioPlayerState(5, 20, 10, false),
		audioPlayerState(6, 20, 10, true),
	} {
		sendInteractiveServerMessage(t, endpoint, state)
		app.drainServerMessages(1)
	}
	recorder.want(t, audio.CueDamage)
}

func TestLocalAudioMiningOnlyAfterAppliedAirDeltaAtConfirmedTarget(t *testing.T) {
	app, endpoint := newInteractiveTestApplication(t)
	var recorder audioCueRecorder
	app.playCue = recorder.play
	target := core.BlockPos{X: 1, Y: 10, Z: 2}
	loadInteractiveBlock(t, app, target, core.StoneID)
	state := audioPlayerState(1, 20, 10, false)
	state.MiningActive = true
	state.MiningTarget = target
	state.MiningProgressTicks = 1
	state.MiningRequiredTicks = 2
	state.MiningHarvestable = true
	sendInteractiveServerMessage(t, endpoint, state)
	app.drainServerMessages(1)

	other := target
	other.X++
	sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
		Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 2, NewRevision: 3,
		Changes: []network.BlockChange{{Position: other, Block: core.AirID}},
	})
	app.drainServerMessages(1)
	recorder.want(t)

	sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
		Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 3, NewRevision: 4,
		Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
	})
	app.drainServerMessages(1)
	recorder.want(t, audio.CueMiningComplete)

	sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
		Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 4, NewRevision: 5,
		Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
	})
	app.drainServerMessages(1)
	recorder.want(t, audio.CueMiningComplete)
}

func TestLocalAudioMiningRequiresActuallyAppliedDelta(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*application, network.ServerEndpoint, core.BlockPos)
	}{
		{
			name: "revision 不匹配",
			setup: func(app *application, endpoint network.ServerEndpoint, target core.BlockPos) {
				sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
					Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 3, NewRevision: 4,
					Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
				})
				app.drainServerMessages(1)
				sendInteractiveServerMessage(t, endpoint, audioChunkSnapshot(target.Chunk(), 3))
				app.drainServerMessages(1)
			},
		},
		{
			name: "未加载区块",
			setup: func(app *application, endpoint network.ServerEndpoint, target core.BlockPos) {
				app.mirror = client.NewMirror()
				sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
					Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 1, NewRevision: 2,
					Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
				})
				app.drainServerMessages(1)
				sendInteractiveServerMessage(t, endpoint, audioChunkSnapshot(target.Chunk(), 1))
				app.drainServerMessages(1)
			},
		},
		{
			name: "已 desync 区块",
			setup: func(app *application, endpoint network.ServerEndpoint, target core.BlockPos) {
				sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
					Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 3, NewRevision: 4,
					Changes: []network.BlockChange{{Position: target, Block: core.StoneID}},
				})
				app.drainServerMessages(1)
				sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
					Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 2, NewRevision: 3,
					Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
				})
				app.drainServerMessages(1)
				sendInteractiveServerMessage(t, endpoint, audioChunkSnapshot(target.Chunk(), 2))
				app.drainServerMessages(1)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, endpoint := newInteractiveTestApplication(t)
			app.loadedChunks = make(map[core.ChunkPos]struct{})
			var recorder audioCueRecorder
			app.playCue = recorder.play
			target := core.BlockPos{X: 1, Y: 10, Z: 2}
			loadInteractiveBlock(t, app, target, core.StoneID)
			state := audioPlayerState(1, 20, 10, false)
			state.MiningActive = true
			state.MiningTarget = target
			state.MiningProgressTicks = 1
			state.MiningRequiredTicks = 2
			state.MiningHarvestable = true
			sendInteractiveServerMessage(t, endpoint, state)
			app.drainServerMessages(1)

			test.setup(app, endpoint, target)
			recorder.want(t)
			_, revision, loaded := app.mirror.Hash(core.Overworld, target.Chunk())
			if !loaded {
				t.Fatal("真实 delta 前区块未由 snapshot 恢复")
			}
			sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
				Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: revision, NewRevision: revision + 1,
				Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
			})
			app.drainServerMessages(1)
			recorder.want(t, audio.CueMiningComplete)
		})
	}
}

func TestLocalAudioPlacementRequiresBothConfirmedResults(t *testing.T) {
	for _, inventoryFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "InventoryState 先到", false: "BlockChanges 先到"}[inventoryFirst], func(t *testing.T) {
			app, endpoint, recorder, target := newAudioPlacementApplication(t)
			if inventoryFirst {
				sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemDirt, 1)})
				app.drainServerMessages(1)
				recorder.want(t)
				sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
			} else {
				sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
				app.drainServerMessages(1)
				recorder.want(t)
				sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemDirt, 1)})
			}
			app.drainServerMessages(1)
			recorder.want(t, audio.CueUIClick)
		})
	}
}

func TestLocalAudioPlacementUsesInitiatingSlotAfterSameFrameSelect(t *testing.T) {
	for _, inventoryFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "InventoryState 先到", false: "BlockChanges 先到"}[inventoryFirst], func(t *testing.T) {
			app, endpoint := newInteractiveTestApplication(t)
			var recorder audioCueRecorder
			app.playCue = recorder.play
			if err := app.predictor.Begin(audioPlayerState(1, 20, 10, false)); err != nil {
				t.Fatal(err)
			}
			app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
			loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 10, Z: 0}, core.StoneID)
			before := audioPlacementInventory(4, 2)
			if err := app.inventory.Apply(network.InventoryState{Inventory: before}); err != nil {
				t.Fatal(err)
			}
			app.applyInteractiveInput(0, client.Movement{}, client.Actions{
				Select: true, SelectSlot: 1, Place: true,
			}, true)
			if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.SelectHotbar); !ok ||
				got != (network.SelectHotbar{Sequence: 1, Slot: 1}) {
				t.Fatalf("SelectHotbar=%#v", got)
			}
			if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.PlaceBlock); !ok ||
				got != (network.PlaceBlock{Sequence: 2, Slot: 4}) {
				t.Fatalf("PlaceBlock=%#v", got)
			}
			target := core.BlockPos{X: 0, Y: 10, Z: 1}
			confirmed := audioPlacementInventory(4, 1)
			confirmed.Hotbar.Selected = 1
			if inventoryFirst {
				sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: confirmed})
				app.drainServerMessages(1)
				recorder.want(t)
				sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
			} else {
				sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
				app.drainServerMessages(1)
				recorder.want(t)
				sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: confirmed})
			}
			app.drainServerMessages(1)
			recorder.want(t, audio.CueUIClick)
		})
	}
}

func TestLocalAudioPlacementFailuresStaySilent(t *testing.T) {
	t.Run("拒绝和重复方块增量", func(t *testing.T) {
		app, endpoint, recorder, target := newAudioPlacementApplication(t)
		sendInteractiveServerMessage(t, endpoint, network.CommandRejected{Sequence: 1, Reason: network.RejectOccupied})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemDirt, 1)})
		app.drainServerMessages(1)
		recorder.want(t)

		app, endpoint, recorder, target = newAudioPlacementApplication(t)
		sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemDirt, 1)})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
		app.drainServerMessages(1)
		recorder.want(t, audio.CueUIClick)
	})

	t.Run("无关消息保留 pending 已见半边", func(t *testing.T) {
		app, endpoint, recorder, target := newAudioPlacementApplication(t)
		app.loadedChunks = make(map[core.ChunkPos]struct{})
		sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
		app.drainServerMessages(1)
		other := core.ChunkPos{X: 1}
		sendInteractiveServerMessage(t, endpoint, audioChunkSnapshot(other, 1))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
			Dimension: core.Overworld, Chunk: other, BaseRevision: 1, NewRevision: 2,
			Changes: []network.BlockChange{{Position: core.BlockPos{X: 16, Y: 10}, Block: core.StoneID}},
		})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemDirt, 2)})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.CommandRejected{Sequence: 2, Reason: network.RejectInvalidInput})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemDirt, 1)})
		app.drainServerMessages(1)
		recorder.want(t, audio.CueUIClick)
	})

	t.Run("同帧 Place 加 Drop 的无关拒绝", func(t *testing.T) {
		app, endpoint := newInteractiveTestApplication(t)
		var recorder audioCueRecorder
		app.playCue = recorder.play
		if err := app.predictor.Begin(audioPlayerState(1, 20, 10, false)); err != nil {
			t.Fatal(err)
		}
		app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
		loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 10, Z: 0}, core.StoneID)
		if err := app.inventory.Apply(network.InventoryState{Inventory: audioInventory(core.ItemDirt, 1)}); err != nil {
			t.Fatal(err)
		}
		app.applyInteractiveInput(0, client.Movement{}, client.Actions{Place: true, Drop: true}, true)
		if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.PlaceBlock); !ok || got.Sequence != 1 {
			t.Fatalf("PlaceBlock=%#v", got)
		}
		if got, ok := receiveInteractiveClientMessage(t, endpoint).(network.DropSelectedItem); !ok || got.Sequence != 2 {
			t.Fatalf("DropSelectedItem=%#v", got)
		}
		target := core.BlockPos{X: 0, Y: 10, Z: 1}
		sendInteractiveServerMessage(t, endpoint, network.CommandRejected{Sequence: 2, Reason: network.RejectInvalidInput})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemNone, 0)})
		app.drainServerMessages(1)
		recorder.want(t, audio.CueUIClick)
	})

	t.Run("reset 和关闭清除 pending", func(t *testing.T) {
		app, endpoint, recorder, target := newAudioPlacementApplication(t)
		sendInteractiveServerMessage(t, endpoint, audioPlayerState(2, 20, 10, true))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlacementDelta(target, 2, 3))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemDirt, 1)})
		app.drainServerMessages(1)
		recorder.want(t)

		app, _, recorder, _ = newAudioPlacementApplication(t)
		app.closeClientSession(nil)
		if app.audioFeedback.placement.active {
			t.Fatal("关闭会话后保留 pending placement")
		}
		recorder.want(t)
	})
}

func TestLocalAudioConfirmationFailuresStaySilent(t *testing.T) {
	t.Run("未确认采掘目标和非法增量", func(t *testing.T) {
		app, endpoint := newInteractiveTestApplication(t)
		var recorder audioCueRecorder
		app.playCue = recorder.play
		target := core.BlockPos{X: 1, Y: 10, Z: 2}
		loadInteractiveBlock(t, app, target, core.StoneID)
		sendInteractiveServerMessage(t, endpoint, network.BlockChanges{
			Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: 2, NewRevision: 3,
			Changes: []network.BlockChange{{Position: target, Block: core.AirID}},
		})
		app.drainServerMessages(1)
		recorder.want(t)
		if err := endpoint.Send(context.Background(), network.BlockChanges{}); err == nil {
			t.Fatal("非法 BlockChanges 被发送")
		}
		recorder.want(t)
	})

	t.Run("进食半匹配和选中格变化", func(t *testing.T) {
		app, endpoint := newInteractiveTestApplication(t)
		var recorder audioCueRecorder
		app.playCue = recorder.play
		before := audioInventory(core.ItemBread, 2)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: before})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlayerState(1, 20, 10, false))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemBread, 1)})
		app.drainServerMessages(1)
		recorder.want(t)
		sendInteractiveServerMessage(t, endpoint, audioPlayerState(2, 20, 10, false))
		app.drainServerMessages(1)
		recorder.want(t)

		selectedChanged := audioInventory(core.ItemBread, 1)
		selectedChanged.Hotbar.Selected = 1
		selectedChanged.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemBread, Count: 1}
		sendInteractiveServerMessage(t, endpoint, audioPlayerState(3, 20, 15, false))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: selectedChanged})
		app.drainServerMessages(1)
		recorder.want(t)
	})

	t.Run("非食物和会话重置", func(t *testing.T) {
		app, endpoint := newInteractiveTestApplication(t)
		var recorder audioCueRecorder
		app.playCue = recorder.play
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemStone, 2)})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlayerState(1, 20, 10, false))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlayerState(2, 20, 15, false))
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: audioInventory(core.ItemStone, 1)})
		app.drainServerMessages(1)
		recorder.want(t)

		sendInteractiveServerMessage(t, endpoint, network.CommandRejected{Reason: network.RejectInvalidInput})
		app.drainServerMessages(1)
		sendInteractiveServerMessage(t, endpoint, audioPlayerState(3, 20, 15, true))
		app.drainServerMessages(1)
		app.closeClientSession(nil)
		recorder.want(t)
	})
}

func TestLocalAudioEatingMatchesEitherConfirmedMessageOrder(t *testing.T) {
	for _, inventoryFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "InventoryState 先到", false: "PlayerState 先到"}[inventoryFirst], func(t *testing.T) {
			app, endpoint := newInteractiveTestApplication(t)
			var recorder audioCueRecorder
			app.playCue = recorder.play
			before := audioInventory(core.ItemBread, 2)
			after := audioInventory(core.ItemBread, 1)
			sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: before})
			app.drainServerMessages(1)
			sendInteractiveServerMessage(t, endpoint, audioPlayerState(1, 20, 10, false))
			app.drainServerMessages(1)
			if inventoryFirst {
				sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: after})
				app.drainServerMessages(1)
				sendInteractiveServerMessage(t, endpoint, audioPlayerState(2, 20, 15, false))
				app.drainServerMessages(1)
			} else {
				sendInteractiveServerMessage(t, endpoint, audioPlayerState(2, 20, 15, false))
				app.drainServerMessages(1)
				sendInteractiveServerMessage(t, endpoint, network.InventoryState{Inventory: after})
				app.drainServerMessages(1)
			}
			recorder.want(t, audio.CueEatingComplete)
		})
	}
}

func audioInventory(item core.ItemID, count uint8) core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: item, Count: count}
	return inventory
}

func audioPlayerState(tick uint64, health, hunger uint8, reset bool) network.PlayerState {
	return network.PlayerState{
		ServerTick: tick, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: reset, Health: health, Hunger: hunger,
	}
}

func audioChunkSnapshot(chunk core.ChunkPos, revision uint64) network.ChunkSnapshot {
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = network.SectionData{Y: int32(index), Storage: network.SectionSingle, Single: core.StoneID}
	}
	return network.ChunkSnapshot{
		Dimension: core.Overworld, Chunk: chunk, Revision: revision, Sections: sections,
	}
}

func newAudioPlacementApplication(t *testing.T) (*application, network.ServerEndpoint, *audioCueRecorder, core.BlockPos) {
	t.Helper()
	app, endpoint := newInteractiveTestApplication(t)
	recorder := new(audioCueRecorder)
	app.playCue = recorder.play
	if err := app.predictor.Begin(audioPlayerState(1, 20, 10, false)); err != nil {
		t.Fatal(err)
	}
	app.camera = client.Camera{Pos: mgl32.Vec3{0.5, 10.5, 3.5}}
	loadInteractiveBlock(t, app, core.BlockPos{X: 0, Y: 10, Z: 0}, core.StoneID)
	if err := app.inventory.Apply(network.InventoryState{Inventory: audioInventory(core.ItemDirt, 2)}); err != nil {
		t.Fatal(err)
	}
	app.placeBlock()
	if _, ok := receiveInteractiveClientMessage(t, endpoint).(network.PlaceBlock); !ok {
		t.Fatal("放置请求未发送")
	}
	return app, endpoint, recorder, core.BlockPos{X: 0, Y: 10, Z: 1}
}

func audioPlacementInventory(selected uint8, count uint8) core.Inventory {
	var inventory core.Inventory
	inventory.Hotbar.Selected = selected
	inventory.Hotbar.Slots[selected] = core.ItemStack{Item: core.ItemDirt, Count: count}
	inventory.Hotbar.Slots[1] = core.ItemStack{Item: core.ItemStone, Count: 1}
	return inventory
}

func audioPlacementDelta(target core.BlockPos, base, revision uint64) network.BlockChanges {
	return network.BlockChanges{
		Dimension: core.Overworld, Chunk: target.Chunk(), BaseRevision: base, NewRevision: revision,
		Changes: []network.BlockChange{{Position: target, Block: core.DirtID}},
	}
}

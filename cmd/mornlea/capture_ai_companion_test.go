package main

// capture_ai_companion_test.go 钉住 AI 伙伴视觉场景清除全部前序客户端呈现状态，
// 避免共享 `application` 把旧实体、容器、反馈或聊天缓存带入伙伴 golden。

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
)

func TestCaptureAICompanionClearsPriorClientState(t *testing.T) {
	app := newCaptureAICompanionState()
	if err := app.remotePlayers.Apply(network.RemotePlayerSpawn{
		PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: 1}, DisplayName: "旧玩家",
		ServerTick: 1, Position: mgl32.Vec3{1, 2, 3},
	}); err != nil {
		t.Fatal(err)
	}
	oldCompanionID := companion.ID{0: 1, 6: 0x40, 8: 0x80, 15: 2}
	if err := app.companions.ApplySpawn(network.CompanionSpawn{
		ID: oldCompanionID, Name: "旧友", Tick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{1, 2, 3},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.chatEvents.Apply(network.ChatEvent{
		EventID: 1, PlayerID: core.PlayerID{0: 1, 6: 0x40, 8: 0x80, 15: 3},
		PlayerName: "旧客", CompanionID: oldCompanionID, CompanionName: "旧友",
		Kind: network.ChatEventAccepted, Command: "旧命令",
	}); err != nil {
		t.Fatal(err)
	}
	app.chatInput.Open()
	for _, value := range "旧输入" {
		app.chatInput.Append(value)
	}
	app.chatInput.overflow = true
	app.chatEventBuffer[0] = network.ChatEvent{EventID: 99}
	app.chatLines[0], app.chatLineCount, app.formattedChatEventID = "旧缓存", 1, 99
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.inventory.Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	app.inventoryOpen, app.inventorySource, app.panel.visible = true, 7, true
	if err := app.furnace.Apply(network.FurnaceState{Furnace: core.FurnaceRef{Generation: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := app.chest.Apply(network.ChestState{Chest: core.ContainerRef{
		Kind: core.ContainerKindChest, Generation: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	app.miningOverlay = hud.MiningOverlay{Active: true, ProgressTicks: 3, RequiredTicks: 9}
	app.damageFeedback = damageFeedback{hasHealth: true, health: 7, remaining: 1}
	app.damageStrength = 0.75
	app.remotePresentations = []client.RemotePresentation{{DisplayName: "旧玩家"}}
	app.companionPresentations = []client.CompanionPresentation{{Name: "旧友"}}
	app.remoteAvatars = []render.Avatar{{Position: mgl32.Vec3{1, 2, 3}}}
	app.remoteNameTags = []render.NameTag{{Text: "旧标签"}}
	app.itemDropInstances = []render.ItemDrop{{}}
	app.blockTargetReset = true
	if err := app.itemDrops.Apply(network.ItemDropUpserts{Drops: []network.ItemDrop{{
		ID:         core.DropID{Dimension: core.Overworld, Generation: 1},
		BlockIndex: 1, Item: core.ItemStone, Count: 1,
	}}}); err != nil {
		t.Fatal(err)
	}
	app.worldTimeTicks = 18000
	app.camera = client.Camera{Pos: mgl32.Vec3{99, 99, 99}, Yaw: 1, Pitch: 1}

	scene := captureSceneByName(t, "ai-companion")
	if err := scene.Apply(app); err != nil {
		t.Fatal(err)
	}
	assertCaptureAICompanionState(t, app)
	if len(app.remotePlayers.Presentations()) != 0 || len(app.itemDrops.Presentations()) != 0 {
		t.Fatalf("旧实体未清空: remote=%v drops=%v",
			app.remotePlayers.Presentations(), app.itemDrops.Presentations())
	}
	if len(app.remotePresentations) != 0 || len(app.companionPresentations) != 0 ||
		len(app.remoteAvatars) != 0 || len(app.remoteNameTags) != 0 ||
		len(app.itemDropInstances) != 0 || app.blockTargetReset ||
		app.center != cameraChunk(app.camera.Pos) {
		t.Fatalf("派生呈现状态未重置: remote=%d companion=%d avatars=%d tags=%d drops=%d targetReset=%v center=%+v",
			len(app.remotePresentations), len(app.companionPresentations), len(app.remoteAvatars),
			len(app.remoteNameTags), len(app.itemDropInstances), app.blockTargetReset, app.center)
	}
	if app.chatEventBuffer != ([client.ChatEventCapacity]network.ChatEvent{}) || app.chatLines != ([6]string{}) ||
		app.chatLineCount != 0 || app.formattedChatEventID != 0 {
		t.Fatalf("旧聊天缓存未清空: buffer=%+v lines=%+v count=%d id=%d",
			app.chatEventBuffer, app.chatLines, app.chatLineCount, app.formattedChatEventID)
	}
	if _, confirmed := app.inventory.State(); confirmed || app.inventoryOpen || app.inventorySource != -1 {
		t.Fatalf("inventory 未重置: confirmed=%v open=%v source=%d",
			confirmed, app.inventoryOpen, app.inventorySource)
	}
	if app.panel.visible {
		t.Fatal("panel 未隐藏")
	}
	if _, opened := app.furnace.State(); opened {
		t.Fatal("furnace 未重置")
	}
	if _, opened := app.chest.State(); opened {
		t.Fatal("chest 未重置")
	}
	if app.miningOverlay != (hud.MiningOverlay{}) || app.damageFeedback != (damageFeedback{}) ||
		app.damageStrength != 0 {
		t.Fatalf("反馈状态未重置: mining=%+v damage=%+v strength=%v",
			app.miningOverlay, app.damageFeedback, app.damageStrength)
	}
}

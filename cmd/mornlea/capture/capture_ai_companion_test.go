package capture

// capture_ai_companion_test.go 钉住 AI 伙伴视觉场景清除全部前序客户端呈现状态，
// 避免共享 `app.Application` 把旧实体、容器、反馈或聊天缓存带入伙伴 golden。

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render"
	"github.com/channing771/mornlea/internal/render/hud"
)

func TestCaptureAICompanionClearsPriorClientState(t *testing.T) {
	app := newCaptureAICompanionState()
	if err := app.RemotePlayers().Apply(network.RemotePlayerSpawn{
		PlayerID: core.PlayerID{6: 0x40, 8: 0x80, 15: 1}, DisplayName: "旧玩家",
		ServerTick: 1, Position: mgl32.Vec3{1, 2, 3},
	}); err != nil {
		t.Fatal(err)
	}
	oldCompanionID := companion.ID{0: 1, 6: 0x40, 8: 0x80, 15: 2}
	if err := app.Companions().ApplySpawn(network.CompanionSpawn{
		ID: oldCompanionID, Name: "旧友", Tick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{1, 2, 3},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.ChatEvents().Apply(network.ChatEvent{
		EventID: 1, PlayerID: core.PlayerID{0: 1, 6: 0x40, 8: 0x80, 15: 3},
		PlayerName: "旧客", CompanionID: oldCompanionID, CompanionName: "旧友",
		Kind: network.ChatEventAccepted, Command: "旧命令",
	}); err != nil {
		t.Fatal(err)
	}
	app.ChatInput().Open()
	for _, value := range "旧输入" {
		app.ChatInput().Append(value)
	}
	app.ChatInput().SetOverflow(true)
	chatEventBuffer := app.ChatEventBuffer()
	chatEventBuffer[0] = network.ChatEvent{EventID: 99}
	app.SetChatEventBuffer(chatEventBuffer)
	chatLines := app.ChatLines()
	chatLines[0] = "旧缓存"
	app.SetChatLines(chatLines)
	app.SetChatLineCount(1)
	app.SetFormattedChatEventID(99)
	inventory := core.Inventory{}
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
	if err := app.Inventory().Apply(network.InventoryState{Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	app.SetInventoryOpen(true)
	app.SetInventorySource(7)
	if app.Panel() != nil {
		app.Panel().SetVisible(true)
	}
	if err := app.Furnace().Apply(network.FurnaceState{Furnace: core.FurnaceRef{Generation: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := app.Chest().Apply(network.ChestState{Chest: core.ContainerRef{
		Kind: core.ContainerKindChest, Generation: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	app.SetMiningOverlay(hud.MiningOverlay{Active: true, ProgressTicks: 3, RequiredTicks: 9})
	app.SetDamageFeedback(application.DamageFeedback{HasHealth: true, Health: 7, Remaining: 1})
	app.SetDamageStrength(0.75)
	app.SetRemotePresentations([]client.RemotePresentation{{DisplayName: "旧玩家"}})
	app.SetCompanionPresentations([]client.CompanionPresentation{{Name: "旧友"}})
	app.SetRemoteAvatars([]render.Avatar{{Position: mgl32.Vec3{1, 2, 3}}})
	app.SetRemoteNameTags([]render.NameTag{{Text: "旧标签"}})
	app.SetItemDropInstances([]render.ItemDrop{{}})
	app.SetBlockTargetReset(true)
	if err := app.ItemDrops().Apply(network.ItemDropUpserts{Drops: []network.ItemDrop{{
		ID:         core.DropID{Dimension: core.Overworld, Generation: 1},
		BlockIndex: 1, Item: core.ItemStone, Count: 1,
	}}}); err != nil {
		t.Fatal(err)
	}
	app.SetWorldTimeTicks(18000)
	*app.Camera() = client.Camera{Pos: mgl32.Vec3{99, 99, 99}, Yaw: 1, Pitch: 1}

	scene := captureSceneByName(t, "ai-companion")
	if err := scene.Apply(app); err != nil {
		t.Fatal(err)
	}
	assertCaptureAICompanionState(t, app)
	if len(app.RemotePlayers().Presentations()) != 0 || len(app.ItemDrops().Presentations()) != 0 {
		t.Fatalf("旧实体未清空: remote=%v drops=%v",
			app.RemotePlayers().Presentations(), app.ItemDrops().Presentations())
	}
	if len(app.RemotePresentations()) != 0 || len(app.CompanionPresentations()) != 0 ||
		len(app.RemoteAvatars()) != 0 || len(app.RemoteNameTags()) != 0 ||
		len(app.ItemDropInstances()) != 0 || app.BlockTargetReset() ||
		app.Center() != application.CameraChunk(app.Camera().Pos) {
		t.Fatalf("派生呈现状态未重置: remote=%d companion=%d avatars=%d tags=%d drops=%d targetReset=%v center=%+v",
			len(app.RemotePresentations()), len(app.CompanionPresentations()), len(app.RemoteAvatars()),
			len(app.RemoteNameTags()), len(app.ItemDropInstances()), app.BlockTargetReset(), app.Center())
	}
	if app.ChatEventBuffer() != ([client.ChatEventCapacity]network.ChatEvent{}) || app.ChatLines() != ([6]string{}) ||
		app.ChatLineCount() != 0 || app.FormattedChatEventID() != 0 {
		t.Fatalf("旧聊天缓存未清空: buffer=%+v lines=%+v count=%d id=%d",
			app.ChatEventBuffer(), app.ChatLines(), app.ChatLineCount(), app.FormattedChatEventID())
	}
	if _, confirmed := app.Inventory().State(); confirmed || app.InventoryOpen() || app.InventorySource() != -1 {
		t.Fatalf("inventory 未重置: confirmed=%v open=%v source=%d",
			confirmed, app.InventoryOpen(), app.InventorySource())
	}
	if app.Panel().Visible() {
		t.Fatal("panel 未隐藏")
	}
	if _, opened := app.Furnace().State(); opened {
		t.Fatal("furnace 未重置")
	}
	if _, opened := app.Chest().State(); opened {
		t.Fatal("chest 未重置")
	}
	if app.MiningOverlay() != (hud.MiningOverlay{}) || app.DamageFeedback() != (application.DamageFeedback{}) ||
		app.DamageStrength() != 0 {
		t.Fatalf("反馈状态未重置: mining=%+v damage=%+v strength=%v",
			app.MiningOverlay(), app.DamageFeedback(), app.DamageStrength())
	}
}

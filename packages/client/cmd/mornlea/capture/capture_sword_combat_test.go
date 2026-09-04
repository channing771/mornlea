package capture

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func isValidUUIDv4(id core.PlayerID) bool {
	// version 4: byte 6 high nibble 0100, variant: byte 8 high 2 bits 10
	if id[6]>>4 != 0x4 {
		return false
	}
	if id[8]>>6 != 0x2 {
		return false
	}
	return true
}

func TestSwordCombatCaptureState(t *testing.T) {
	scene := captureSceneByName(t, "sword-combat")
	if scene.Prepare == nil || scene.Apply == nil || scene.PinVolatile == nil || scene.WarmupFrames != 8 {
		t.Fatalf("sword-combat 场景不完整: %+v", scene)
	}
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	// 预置旧状态，确保清理生效
	if err := app.Inventory().Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
		t.Fatal(err)
	}
	if app.RemotePlayers() == nil {
		t.Fatal("需要 RemotePlayers")
	}
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 sword-combat: %v", err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 sword-combat: %v", err)
	}
	// 固定世界时间与相机
	if app.WorldTimeTicks() != 6000 {
		t.Fatalf("world time=%d want 6000", app.WorldTimeTicks())
	}
	wantCamera := client.Camera{
		Pos: mgl32.Vec3{5.5, 3.2, 9.5}, Yaw: 0, Pitch: -0.05,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	if *app.Camera() != wantCamera {
		t.Fatalf("camera=%+v want %+v", *app.Camera(), wantCamera)
	}
	if app.Center() != application.CameraChunk(app.Camera().Pos) {
		t.Fatalf("center not synced")
	}
	// 选中非满耐久铁剑 Durability 125
	inv, confirmed := app.Inventory().State()
	if !confirmed {
		t.Fatal("inventory not confirmed")
	}
	if inv.Hotbar.Selected != 2 {
		t.Fatalf("selected=%d want 2", inv.Hotbar.Selected)
	}
	stack := inv.Hotbar.Slots[2]
	if stack.Item != core.ItemIronSword || stack.Count != 1 || stack.Durability != 125 {
		t.Fatalf("sword stack=%+v want IronSword 125", stack)
	}
	// 合法 UUIDv4 且沿击退方向移动 0.35
	presentations := app.RemotePlayers().AppendPresentations(nil)
	if len(presentations) != 1 {
		t.Fatalf("remote players=%d want 1", len(presentations))
	}
	pres := presentations[0]
	if !isValidUUIDv4(pres.PlayerID) {
		t.Fatalf("PlayerID %v 不是合法 UUIDv4", pres.PlayerID)
	}
	if pres.Position != captureSwordCombatKnockedPos {
		t.Fatalf("knocked pos=%v want %v", pres.Position, captureSwordCombatKnockedPos)
	}
	// 验证击退距离 0.35
	dx := captureSwordCombatKnockedPos[0] - captureSwordCombatInitialPos[0]
	dz := captureSwordCombatKnockedPos[2] - captureSwordCombatInitialPos[2]
	dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	if math.Abs(float64(dist-0.35)) > 1e-5 {
		t.Fatalf("knockback dist=%v want 0.35", dist)
	}
	// marker 可见
	if !app.CombatMarkerVisible() {
		t.Fatalf("marker 不可见")
	}
	// 先让 marker 失效
	app.ResetCombatFeedback()
	if app.CombatMarkerVisible() {
		t.Fatalf("reset 后仍可见")
	}
	// PinVolatile 应重新武装
	if err := scene.PinVolatile(app); err != nil {
		t.Fatalf("PinVolatile: %v", err)
	}
	if !app.CombatMarkerVisible() {
		t.Fatalf("PinVolatile 后不可见")
	}
	// 清理：后续场景不应继承
	if err := resetCapturePresentation(app); err != nil {
		t.Fatalf("清理: %v", err)
	}
	if app.CombatMarkerVisible() {
		t.Fatalf("清理后仍可见")
	}
	if got := app.RemotePlayers().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("清理后仍有远端玩家")
	}
}

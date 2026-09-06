package capture

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件覆盖手持基线四景的装入状态：固定机位、确认背包与动作状态，以及收
// 敛后钉死（PinVolatile）的行为。

// newHandCaptureTestApp 装配手持场景测试的最小应用：与战斗场景测试同源。
func newHandCaptureTestApp(t *testing.T) *application.Application {
	t.Helper()
	mesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(mesher.Close)
	app := newCaptureAICompanionState()
	app.SetMirror(client.NewMirror())
	app.SetMesher(mesher)
	return app
}

// assertHandCaptureFraming 断言手持三景（工具/方块/打击）共用的机位与中心。
func assertHandCaptureFraming(t *testing.T, app *application.Application) {
	t.Helper()
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
		t.Fatal("center not synced")
	}
}

// assertHandCaptureBackpack 断言 2 号槽确认选中指定持物栈。
func assertHandCaptureBackpack(t *testing.T, app *application.Application, want core.ItemStack) {
	t.Helper()
	inv, confirmed := app.Inventory().State()
	if !confirmed {
		t.Fatal("inventory not confirmed")
	}
	if inv.Hotbar.Selected != 2 {
		t.Fatalf("selected=%d want 2", inv.Hotbar.Selected)
	}
	if stack := inv.Hotbar.Slots[2]; stack != want {
		t.Fatalf("held stack=%+v want %+v", stack, want)
	}
}

func TestHandToolCaptureState(t *testing.T) {
	scene := captureSceneByName(t, "hand-tool")
	if scene.Prepare == nil || scene.Apply == nil || scene.WarmupFrames != 8 {
		t.Fatalf("hand-tool 场景不完整: %+v", scene)
	}
	if scene.PinVolatile != nil {
		t.Fatal("hand-tool 为中立持握，不应带 PinVolatile")
	}
	app := newHandCaptureTestApp(t)
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 hand-tool: %v", err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 hand-tool: %v", err)
	}
	assertHandCaptureFraming(t, app)
	assertHandCaptureBackpack(t, app,
		core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 125})
	if overlay := app.MiningOverlay(); overlay.Active {
		t.Fatalf("hand-tool 不应带采掘镜像: %+v", overlay)
	}
	if app.CombatMarkerVisible() {
		t.Fatal("hand-tool 不应带命中标记")
	}
}

func TestHandBlockCaptureState(t *testing.T) {
	scene := captureSceneByName(t, "hand-block")
	if scene.Prepare == nil || scene.Apply == nil || scene.WarmupFrames != 8 {
		t.Fatalf("hand-block 场景不完整: %+v", scene)
	}
	if scene.PinVolatile != nil {
		t.Fatal("hand-block 为中立持握，不应带 PinVolatile")
	}
	app := newHandCaptureTestApp(t)
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 hand-block: %v", err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 hand-block: %v", err)
	}
	assertHandCaptureFraming(t, app)
	assertHandCaptureBackpack(t, app, core.ItemStack{Item: core.ItemDirt, Count: 1})
}

func TestHandMiningCaptureState(t *testing.T) {
	scene := captureSceneByName(t, "hand-mining")
	if scene.Prepare == nil || scene.Apply == nil || scene.PinVolatile == nil || scene.WarmupFrames != 8 {
		t.Fatalf("hand-mining 场景不完整: %+v", scene)
	}
	app := newHandCaptureTestApp(t)
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 hand-mining: %v", err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 hand-mining: %v", err)
	}
	// 裂纹机位与目标：与裂纹场景同一世界坐标。
	if app.Camera().Pos != captureCrackCameraPos {
		t.Fatalf("camera pos=%v want %v", app.Camera().Pos, captureCrackCameraPos)
	}
	assertHandCaptureBackpack(t, app,
		core.ItemStack{Item: core.ItemIronPickaxe, Count: 1, Durability: 125})
	overlay := app.MiningOverlay()
	if !overlay.Active || !overlay.HasTarget || overlay.Target != captureMiningCrackTarget ||
		overlay.ProgressTicks != 6 || overlay.RequiredTicks != 30 {
		t.Fatalf("mining overlay=%+v want 浅阶段 6/30", overlay)
	}
	// PinVolatile 只清挥动边沿，不碰采掘镜像：裂纹仍在，右手回上升沿。
	if err := scene.PinVolatile(app); err != nil {
		t.Fatalf("PinVolatile: %v", err)
	}
	if pinned := app.MiningOverlay(); pinned != overlay {
		t.Fatalf("PinVolatile 后 overlay=%+v want %+v", pinned, overlay)
	}
}

func TestHandAttackCaptureState(t *testing.T) {
	scene := captureSceneByName(t, "hand-attack")
	if scene.Prepare == nil || scene.Apply == nil || scene.PinVolatile == nil || scene.WarmupFrames != 8 {
		t.Fatalf("hand-attack 场景不完整: %+v", scene)
	}
	app := newHandCaptureTestApp(t)
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 hand-attack: %v", err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 hand-attack: %v", err)
	}
	assertHandCaptureFraming(t, app)
	assertHandCaptureBackpack(t, app,
		core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 125})
	if !app.CombatMarkerVisible() {
		t.Fatal("hand-attack 标记不可见")
	}
	// 打击基线是纯第一人称场景：不带受击远端玩家（带目标对照由战斗场景覆盖）。
	if got := app.RemotePlayers().AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("hand-attack 远端玩家=%d want 0", len(got))
	}
	app.ResetCombatFeedback()
	if err := scene.PinVolatile(app); err != nil {
		t.Fatalf("PinVolatile: %v", err)
	}
	if !app.CombatMarkerVisible() {
		t.Fatal("PinVolatile 后标记不可见")
	}
	// 清理：后继 far-horizon 不应继承标记与背包。
	if err := resetCapturePresentation(app); err != nil {
		t.Fatalf("清理: %v", err)
	}
	if app.CombatMarkerVisible() {
		t.Fatal("清理后标记仍可见")
	}
	inv, _ := app.Inventory().State()
	if inv.Hotbar.Slots[2] != (core.ItemStack{}) {
		t.Fatalf("清理后背包残留: %+v", inv.Hotbar.Slots[2])
	}
}

// TestHandMiningCleanup 挖掘镜像随公共清场熄灭：后继 far-horizon 不应继
// 承裂纹与持镐挥动。
func TestHandMiningCleanup(t *testing.T) {
	scene := captureSceneByName(t, "hand-mining")
	app := newHandCaptureTestApp(t)
	if err := scene.Prepare(app); err != nil {
		t.Fatalf("准备 hand-mining: %v", err)
	}
	if err := scene.Apply(app); err != nil {
		t.Fatalf("应用 hand-mining: %v", err)
	}
	if err := resetCapturePresentation(app); err != nil {
		t.Fatalf("清理: %v", err)
	}
	if overlay := app.MiningOverlay(); overlay.Active {
		t.Fatalf("清理后采掘镜像仍 active: %+v", overlay)
	}
}
